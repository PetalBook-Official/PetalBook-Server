// Package api is the HTTP API of docs/api.md. The web app talks to the server
// only through it.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"petalbook.app/server/internal/auth"
	"petalbook.app/server/internal/authz"
	"petalbook.app/server/internal/connections"
	"petalbook.app/server/internal/dailyfetch"
	"petalbook.app/server/internal/domains"
	"petalbook.app/server/internal/drafts"
	"petalbook.app/server/internal/github"
	"petalbook.app/server/internal/gitstore"
	"petalbook.app/server/internal/index"
	"petalbook.app/server/internal/jobs"
	"petalbook.app/server/internal/oauth"
	"petalbook.app/server/internal/page"
	"petalbook.app/server/internal/quota"
	"petalbook.app/server/internal/ratelimit"
	"petalbook.app/server/internal/secret"
	"petalbook.app/server/internal/secrets"
	"petalbook.app/server/internal/sentence"
	"petalbook.app/server/internal/settings"
	"petalbook.app/server/internal/space"
	"petalbook.app/server/internal/spaces"
	"petalbook.app/server/internal/store"
	"petalbook.app/server/internal/tokens"
)

// SpaceID is the id of the one space of single-user mode.
const SpaceID = spaces.SingleID

// DefaultMaxBody caps a request body. A page is text; 8 MiB is hundreds of
// thousands of words.
const DefaultMaxBody = 8 << 20

// DefaultMaxUpload caps an uploaded file.
const DefaultMaxUpload = 10 << 20

// DefaultMaxImport caps the body of an import.
const DefaultMaxImport = 100 << 20

// authBodyLimit caps the bodies of the sign-in and account endpoints, which
// anyone can reach.
const authBodyLimit = 64 << 10

// Options configures the handler.
type Options struct {
	Version string
	// BuildTime is when the binary was built, injected at build time
	// (-X main.buildTime=...); empty when it was not. The admin status then
	// shows it instead of the commit's time.
	BuildTime string
	MaxBody   int64 // bytes; DefaultMaxBody when 0
	// MaxUpload is the largest file accepted as an upload, in bytes;
	// DefaultMaxUpload when 0.
	MaxUpload int64
	// MaxImport is the largest body of an import, in bytes; DefaultMaxImport when 0.
	MaxImport int64
	// Store, Spaces and Drafts are required.
	Store  *store.Store
	Spaces *spaces.Manager
	Drafts *drafts.Service
	// Auth is set in accounts mode and nil in single-user mode, where every
	// request acts as the built-in user.
	Auth *auth.Service
	// PublicURL is the address people use in accounts mode; its origin is
	// accepted as the server's own on any listen address.
	PublicURL string
	// RestrictHost is loopback-only mode: it refuses requests whose Host header
	// is not a loopback name (or the host of an allowed origin), which stops a
	// web page on another site from reaching the server through DNS rebinding,
	// and it makes loopback origins on the request's port the server's own.
	// With it off there is no implicit own origin: browsers need AllowOrigins
	// (or, in accounts mode, the public URL).
	RestrictHost bool
	// AllowOrigins are origins (scheme://host[:port]) besides the server's own
	// that may make requests, for a development server on another port that
	// proxies to this one. Cross-site checks stay on whatever else is set.
	AllowOrigins []string
	Logger       *slog.Logger
	// Tokens is the service of API tokens (NewTokens); made with the defaults
	// when nil. The caller runs its Run.
	Tokens *tokens.Service
	// OAuth is the authorization server for MCP clients; nil when there is none
	// (single-user mode, or no public URL). The caller runs its Run.
	OAuth *oauth.Service
	// DailyFetch gets the daily text from the external interfaces an
	// administrator configured; the one that refuses private addresses when nil.
	DailyFetch *dailyfetch.Fetcher
	// Connections are the server's providers of repositories (accounts mode); the
	// provider "git" is added when it is missing.
	Connections *connections.Registry
	// MaxSpaces is --max-spaces when the flag was given: then it is the site's
	// quota.spacesPerPerson, read only. Nil leaves the stored setting (3).
	MaxSpaces *int
	// Now is the clock (default time.Now), for what has an end: confirmations and
	// views of spaces.
	Now func() time.Time
	// NoMCP switches /mcp off in single-user mode (serve --mcp=false). In
	// accounts mode the administrators' setting decides.
	NoMCP bool
	// PrefsQuiet is how long a person's first preference write waits for others
	// before they are committed together (DefaultPrefsQuiet when 0).
	PrefsQuiet time.Duration
	// Domains is the tenants' email domains (accounts mode): the caller gives
	// its map to auth (OrgDomains, ManagedDomain) and runs its recheck (Run).
	// Made here, with the system's resolver and no recheck, when nil.
	Domains *domains.Service
}

type server struct {
	opt    Options
	log    *slog.Logger
	auth   *auth.Service
	spaces *spaces.Manager
	drafts *drafts.Service
	tokens *tokens.Service
	oauth  *oauth.Service
	// settings are the layered settings (site, tenant; see internal/settings).
	settings *settings.Service
	// domains is the tenants' email domains.
	domains *domains.Service
	// featureFault, set by tests only, fails a route gate's read of a feature.
	featureFault func(tenant, feature string) error
	// vault is the secrets of the site and of tenants.
	vault *secrets.Service
	// quota knows which limits apply and notices crossings (accounts mode).
	quota *quota.Service
	// localAccess is what the built-in user of single-user mode holds.
	localAccess authz.Subject
	single      bool
	started     time.Time
	local       store.User // the built-in user of single-user mode
	origins     map[string]bool
	// importing lets one import run at a time (see importMarkdown).
	importing chan struct{}
	// confirms are the tokens of the confirmations; views the views of spaces
	// that administrators opened (see confirm.go and adminview.go).
	confirms *confirmStore
	views    *viewTable
	// viewsAs are the views as a person that administrators opened (viewas.go).
	viewsAs *viewAsTable
	// consoles throttles the record of entering a tenant's scope (scope.go).
	consoles *onceAnHour
	// activities keeps the optional audit rows of reads and downloads to one an
	// hour for each person and thing (activity.go).
	activities *onceAnHour
	// jobs runs the background jobs (internal/jobs; the bulk jobs, see
	// bulk.go).
	jobs *jobs.Runner
	// prefs holds the preferences written and not committed yet (see prefs.go).
	prefs prefsHub
	// peopleGate limits the people search per person, batchGate the rows of
	// batches per person (see batch.go).
	peopleGate, batchGate *ratelimit.Bucket
	// Background work (the bulk jobs) is started with goBg: it runs with bgCtx,
	// which Close cancels, and Close waits for it on bg.
	bgCtx    context.Context
	bgCancel context.CancelFunc
	bgMu     sync.Mutex // guards bgClosed and the Adds to bg
	bgClosed bool       // Close has begun: no background work starts any more
	bg       sync.WaitGroup
	// now is the clock of what has an end (confirmations, views).
	now func() time.Time
	// originHosts are the hosts of the allowed origins: a request addressed to
	// one of them (through a reverse proxy, say) passes the Host restriction.
	originHosts map[string]bool
}

// Handler is the API: the handler of every endpoint, and the work it runs in
// the background (the bulk jobs), which Close stops and waits for.
type Handler struct {
	h http.Handler
	s *server
}

// ServeHTTP serves a request. A request for a space whose index is still being
// rebuilt answers restoring after a short wait (spaces.WithRestoringAnswer);
// work started in the background does not carry the mark and waits.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.h.ServeHTTP(w, r.WithContext(spaces.WithRestoringAnswer(r.Context())))
}

// Close stops the background work the handler started and waits for it to
// end. Call it after the HTTP server has stopped and before the spaces, the
// services and the database under it are closed; it may be called again.
func (h *Handler) Close() { h.s.close() }

// goBg runs f in the background, tracked: Close cancels its context and waits
// for it, so nothing runs on after the handler is closed (into spaces and a
// database that are closed next). It reports false, without running f, when
// Close has begun.
func (s *server) goBg(f func(ctx context.Context)) bool {
	s.bgMu.Lock()
	if s.bgClosed {
		s.bgMu.Unlock()
		return false
	}
	s.bg.Add(1)
	s.bgMu.Unlock()
	go func() {
		defer s.bg.Done()
		f(s.bgCtx)
	}()
	return true
}

func (s *server) close() {
	s.settings.Close()
	s.flushAllPrefs()
	s.bgMu.Lock()
	s.bgClosed = true
	s.bgMu.Unlock()
	s.bgCancel()
	s.jobs.Close() // stops the running jobs and waits for them
	s.bg.Wait()
}

// New returns the handler for every endpoint.
func New(opt Options) *Handler {
	if opt.MaxBody <= 0 {
		opt.MaxBody = DefaultMaxBody
	}
	if opt.MaxUpload <= 0 {
		opt.MaxUpload = DefaultMaxUpload
	}
	if opt.MaxImport <= 0 {
		opt.MaxImport = DefaultMaxImport
	}
	if opt.Version == "" {
		opt.Version = "dev"
	}
	if opt.Tokens == nil {
		opt.Tokens = NewTokens(tokens.Config{Store: opt.Store, Logger: opt.Logger}, opt.Auth)
	}
	if opt.DailyFetch == nil {
		opt.DailyFetch = dailyfetch.New(NewDailyBacking(opt.Store))
	}
	// The spaces that synced with a person's tokens lose access when the tokens
	// go (a disconnection, an expiry): they show it at once. (The token services
	// keep the registry they were made with.)
	wireLoss := func() {
		if opt.Connections == nil || opt.Spaces == nil {
			return
		}
		opt.Connections.OnAccessLost(opt.Spaces.AccessLost)
		opt.Connections.SetRequire(func() string {
			if r := opt.Spaces.Storage().Remotes.Require; r != nil {
				return *r
			}
			return ""
		})
	}
	wireLoss()
	if _, ok := opt.Connections.Get(connections.GitID); !ok {
		ps := opt.Connections.List()
		opt.Connections = connections.NewRegistry(append(ps, connections.NewGit())...)
	}
	wireLoss()
	// The requirement of a remote as the settings now make it (a rule on
	// storage.mode written before this server started, say) begins its grace
	// period now, not when the rule was written.
	if opt.Spaces != nil {
		if err := opt.Spaces.SettlePolicy(context.Background()); err != nil && opt.Logger != nil {
			opt.Logger.Error("could not settle when the remote requirement took its form", "error", err)
		}
	}
	// The GitHub row is both a sign-in provider and where repositories are
	// connected: sign-in reads it again when the App changes, and connecting
	// repositories with another account never takes a person's last way in.
	if gh, ok := opt.Connections.Get(github.ProviderID); ok && opt.Auth != nil {
		if svc, ok := gh.(*github.Service); ok {
			au := opt.Auth
			svc.SetProvidersChanged(func(ctx context.Context) {
				if err := au.ReloadProviders(ctx); err != nil && opt.Logger != nil {
					opt.Logger.Error("could not read the sign-in providers again", "error", err)
				}
			})
			svc.SetSigninGuard(au.SigninLinkRemovable)
			// The spaces that lose GitHub access when an installation is
			// disconnected show it at once.
			if opt.Spaces != nil {
				svc.OnAccessLost(opt.Spaces.AccessLost)
			}
		}
	}
	s := &server{opt: opt, log: opt.Logger, settings: settings.Of(opt.Store), auth: opt.Auth, spaces: opt.Spaces, drafts: opt.Drafts, tokens: opt.Tokens, oauth: opt.OAuth, single: opt.Auth == nil, started: time.Now(),
		origins: map[string]bool{}, originHosts: map[string]bool{}, importing: make(chan struct{}, 1),
		confirms: newConfirmStore(), views: newViewTable(), viewsAs: newViewAsTable(), consoles: newOnceAnHour(maxConsoleAudit), activities: newOnceAnHour(maxActivityAudit), now: opt.Now}
	if s.now == nil {
		s.now = time.Now
	}
	now := func() time.Time { return s.now() }
	s.peopleGate = ratelimit.NewBucket(maxGateKeys, peoplePerMinute, peoplePerWindow, now)
	s.batchGate = ratelimit.NewBucket(maxGateKeys, batchRowsPerMinute, batchRowsPerWindow, now)
	s.bgCtx, s.bgCancel = context.WithCancel(context.Background())
	if s.log == nil {
		s.log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	allow := append([]string(nil), opt.AllowOrigins...)
	if !s.single && opt.PublicURL != "" {
		allow = append(allow, OriginOf(opt.PublicURL))
	}
	for _, o := range allow {
		s.origins[NormalizeOrigin(o)] = true
		if u, err := url.Parse(NormalizeOrigin(o)); err == nil && u.Host != "" {
			s.originHosts[u.Host] = true
		}
	}
	if s.single {
		s.local = store.User{ID: store.LocalUserID, Username: "local", Name: "Local user", Role: "admin", SiteAdmin: true, TenantID: store.DefaultTenantID, Created: time.Now().UTC()}
		if u, err := store.GetUser(context.Background(), opt.Store.Q(), store.AllTenants(), store.LocalUserID); err == nil {
			s.local = *u
		}
		s.localAccess = auth.AccessOf(s.local)
	}
	s.settings.SetClock(s.now)
	// Billing does not switch off a feature a tenant uses (capabilities.go).
	s.settings.UseGuard(s.billingGuard)
	s.domains = opt.Domains
	if s.domains == nil {
		s.domains = domains.New(domains.Options{Store: opt.Store, Settings: s.settings, SiteHost: SiteHostOf(opt.PublicURL), Now: now, Logger: s.log})
	}
	if !s.single {
		if err := s.domains.Load(context.Background()); err != nil {
			s.log.Error("could not read the verified email domains", "error", err)
		}
	}
	s.vault = secrets.Of(opt.Store, boxOf(opt))
	s.settings.SetFileCeiling(opt.MaxUpload)
	s.bindRecycle()
	s.bindDraftFileLimit()
	s.jobs = jobs.New(jobs.Options{Store: opt.Store, Logger: s.log, Now: now, WriteFault: jobWriteFault,
		MaxRunning: func() int { return s.jobLimit("", settings.JobsMaxRunning, 2) },
		PerTenant:  func(tenant string) int { return s.jobLimit(tenant, settings.JobsPerTenant, 1) }})
	s.jobs.Register(bulkKindName, bulkKind{s})
	s.jobs.Register(store.JobAuditPurge, auditPurgeKind{s})
	if opt.Spaces != nil && opt.Store != nil {
		// The exports, the actions of the recycle bin and the moves of
		// attachments run on the same runner.
		opt.Spaces.UseJobs(s.jobs)
		opt.Spaces.SetRecycleEnded(s.purgeEnded)
	}
	s.jobs.Start()
	s.bindDailySettings()
	if s.auth != nil {
		s.auth.UseSettings(s.settings)
		s.bindQuota()
		// The starts and ends of the windows of rules are audited, and the
		// quotas looked at again, by one timer (reads are right without it).
		s.settings.Watch(s.bgCtx, s.ruleBoundaries)
	}
	if s.spaces != nil && !s.single {
		s.bindAssetsPolicy()
	}
	mux := http.NewServeMux()
	uploads := map[string]bool{}
	for _, rt := range routeTable {
		mux.HandleFunc(rt.method+" "+rt.pattern, s.handle(rt))
		if rt.upload {
			uploads[rt.method+" "+rt.pattern] = true
		}
	}
	s.mountOAuth(mux)
	s.mountMCP(mux)
	// A path that exists with other methods is 405 in the API's error shape
	// (the mux would answer in plain text), anything else 404. This cannot be
	// method-less patterns in the mux: they conflict with wildcard routes such
	// as /api/invites/{id} next to /api/invites/accept.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if methods := s.methodsFor(r.URL.Path); len(methods) > 0 {
			allow := strings.Join(methods, ", ")
			w.Header().Set("Allow", allow)
			writeError(w, http.StatusMethodNotAllowed, "invalid", "Method "+r.Method+" is not allowed here; use "+allow+".")
			return
		}
		writeError(w, http.StatusNotFound, "not_found", "No such endpoint.")
	})
	// The body of a request that changes something must be JSON, or, on an
	// upload route, a file: asked of the mux, so the route table decides.
	bodyType := func(r *http.Request) string {
		if _, pattern := mux.Handler(r); uploads[pattern] {
			return "application/octet-stream"
		} else if formRoute(pattern) {
			return "application/x-www-form-urlencoded"
		}
		return "application/json"
	}
	return &Handler{h: s.wrap(mux, bodyType), s: s}
}

// methodsFor lists the methods of the routes a path matches, preferring the
// most literal match (so /api/invites/accept is not also /api/invites/{id}).
func (s *server) methodsFor(path string) []string {
	all := strings.Split(path, "/")
	best := -1
	var out []string
	for _, rt := range routeTable {
		segs := all
		if rt.accountsOnly && s.single {
			continue
		}
		ps := strings.Split(rt.pattern, "/")
		if last := ps[len(ps)-1]; strings.HasSuffix(last, "...}") && len(segs) >= len(ps) {
			segs = append(append([]string{}, segs[:len(ps)-1]...), strings.Join(segs[len(ps)-1:], "/"))
		}
		if len(ps) != len(segs) {
			continue
		}
		wild, ok := 0, true
		for i := range ps {
			if strings.HasPrefix(ps[i], "{") {
				if segs[i] == "" {
					ok = false
				}
				wild++
			} else if ps[i] != segs[i] {
				ok = false
			}
		}
		if !ok {
			continue
		}
		if best < 0 || wild < best {
			best, out = wild, nil
		}
		if wild == best {
			out = append(out, rt.method)
		}
	}
	return out
}

// OriginOf returns scheme://host[:port], lower case, of a URL.
func OriginOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

// wrap adds the Host check, panic recovery and access logging.
func (s *server) wrap(next http.Handler, bodyType func(*http.Request) string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				s.log.Error("panic in handler", "path", r.URL.Path, "panic", v)
				if sw.status == 0 {
					writeError(sw, http.StatusInternalServerError, "internal", "Internal error.")
				}
			}
			s.log.Debug("request", "method", r.Method, "path", r.URL.Path, "status", sw.status, "ms", time.Since(start).Milliseconds())
		}()
		sw.Header().Set("X-Content-Type-Options", "nosniff")
		if s.opt.RestrictHost && !loopbackHost(r.Host) && !s.originHosts[strings.ToLower(r.Host)] {
			writeError(sw, http.StatusForbidden, "forbidden", "This server only answers requests addressed to localhost.")
			return
		}
		// The identity provider's redirect is a cross-site navigation by
		// nature. It changes nothing unless its state matches a sign-in this
		// browser started, so the cross-site rules do not apply to it.
		// The endpoints of the authorization server are called, or the browser is
		// sent to them, by other sites by design (see oauthCrossSite), and so are
		// the sign-in endpoints an identity provider sends the browser to, and
		// A platform's return from connecting an account for repos (connectionReturn).
		if !fromIdentityProvider(r) && !connectionReturn(r) && !oauthCrossSite(r) {
			if why := s.crossSite(r); why != "" {
				if !oauthRefuse(sw, r, http.StatusForbidden, "invalid_request", why) {
					writeError(sw, http.StatusForbidden, "forbidden", why)
				}
				return
			}
		}
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
			// Even without a body: a plain HTML form cannot send this type, so
			// requiring it keeps pages on other sites from making changes.
			// A file upload is the one exception, and its type cannot be sent by a form either.
			want := bodyType(r)
			if oauthForm(r) {
				// The token endpoint takes a form, as RFC 6749 says; it checks the type itself.
				next.ServeHTTP(sw, r)
				return
			}
			if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != want {
				msg := "send Content-Type: application/json with every request that changes something"
				if want == "application/x-www-form-urlencoded" {
					msg = "send the form with Content-Type: " + want
				} else if want != "application/json" {
					msg = "send the file with Content-Type: " + want
				}
				if r.URL.Path == oauth.PathRegister {
					oauthRefuse(sw, r, http.StatusBadRequest, "invalid_client_metadata", "send the client metadata with Content-Type: application/json")
					return
				}
				if !oauthRefuse(sw, r, http.StatusUnsupportedMediaType, "invalid_request", msg) {
					writeError(sw, http.StatusUnsupportedMediaType, "invalid", msg)
				}
				return
			}
		}
		next.ServeHTTP(sw, r)
	})
}

// fromIdentityProvider is true for the requests an identity provider's site
// sends the browser to, or the provider itself makes: the redirects of
// OpenID Connect and GitHub, the form posted to a SAML ACS or by form_post, and the metadata.
// None changes anything unless it matches a sign-in this browser started.
func fromIdentityProvider(r *http.Request) bool {
	p := r.URL.Path
	switch r.Method {
	case http.MethodGet:
		if rest, ok := strings.CutPrefix(p, "/api/auth/"); ok {
			parts := strings.Split(rest, "/")
			if len(parts) == 3 && parts[1] != "" && (parts[0] == "oidc" || parts[0] == "github" || parts[0] == "oauth2") && parts[2] == "callback" {
				return true
			}
			if len(parts) == 3 && parts[0] == "saml" && parts[1] != "" && parts[2] == "metadata" {
				return true
			}
		}
	case http.MethodPost:
		if rest, ok := strings.CutPrefix(p, "/api/auth/saml/"); ok {
			parts := strings.Split(rest, "/")
			return len(parts) == 2 && parts[0] != "" && parts[1] == "acs"
		}
		// An OpenID Connect provider's answer posted by form_post (Apple's).
		if rest, ok := strings.CutPrefix(p, "/api/auth/oidc/"); ok {
			parts := strings.Split(rest, "/")
			return len(parts) == 2 && parts[0] != "" && parts[1] == "callback"
		}
	}
	return false
}

// NormalizeOrigin lower-cases an origin and drops a trailing slash.
func NormalizeOrigin(o string) string {
	return strings.ToLower(strings.TrimRight(strings.TrimSpace(o), "/"))
}

// crossSite returns why a request must be refused as coming from another
// site, or "". Browsers state where a request comes from in Sec-Fetch-Site and
// Origin; a request without either (curl, a script) is not a browser acting for
// a web page and passes.
//
// An Origin is accepted only if it is in the allow list, or if the server is in
// loopback-only mode and the Origin is a plain-http loopback address on the
// port the request was sent to (the Host header, which has already passed the
// loopback check). In particular there is no "same as Host" rule otherwise:
// with the Host restriction off, a page on a rebound DNS name would have
// Origin equal to Host, and a TLS-terminating proxy hides the scheme.
func (s *server) crossSite(r *http.Request) string {
	const msg = "cross-site requests are not allowed"
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		return msg
	}
	o := r.Header.Get("Origin")
	if o == "" {
		return ""
	}
	u, err := url.Parse(o)
	if o == "null" || err != nil || u.Scheme == "" || u.Host == "" {
		return msg
	}
	if s.origins[NormalizeOrigin(o)] {
		return ""
	}
	if s.opt.RestrictHost && u.Scheme == "http" && loopbackHost(u.Host) && hostPort(u.Host) == hostPort(r.Host) {
		return ""
	}
	return msg
}

// hostPort returns the port of a host[:port] (80 when absent, the http default).
func hostPort(h string) string {
	if _, port, err := net.SplitHostPort(h); err == nil {
		return port
	}
	return "80"
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func loopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.ToLower(strings.Trim(host, "[]"))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// --- JSON -------------------------------------------------------------------

// oneStop is a message with one full stop at its end: an error made of another
// error's sentence and a full stop of its own would end with two.
func oneStop(m string) string {
	if strings.HasSuffix(m, "..") && !strings.HasSuffix(m, "...") {
		return m[:len(m)-1]
	}
	return m
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	if e, ok := v.(errorBody); ok {
		e.Message = oneStop(e.Message)
		v = e
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		http.Error(w, `{"error":"internal","message":"Could not encode the response."}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	w.Write(buf.Bytes())
}

type errorBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Current any    `json:"current,omitempty"`
	// Provider is the id of the provider an error of a connection is about.
	Provider string `json:"provider,omitempty"`
	// Host is the host a refused connection is about (partial_unsupported).
	Host string `json:"host,omitempty"`
	// Repo is the empty repository a failed space creation left on the platform.
	Repo *repoRefResp `json:"repo,omitempty"`
	// AssetsRepo is the attachment repository made with the space's (milestone 3.1).
	AssetsRepo *repoRefResp `json:"assetsRepo,omitempty"`
	// InstallURL is where the App is installed (app_not_installed).
	InstallURL string `json:"installUrl,omitempty"`
	// Permission is the first permission an App lacks (app_missing_permission),
	// or the permission key a refused route needs (forbidden, feature_off).
	Permission string `json:"permission,omitempty"`
	// Feature is the tenant feature that is off (feature_off).
	Feature string `json:"feature,omitempty"`
	// What some refusals are about (docs/api.md, "Daily text refusals"): a
	// request header, a field of an interface.
	Header string `json:"header,omitempty"`
	Field  string `json:"field,omitempty"`
	// Accounts is how many accounts a refused provider deletion would have left without a way in.
	Accounts *int `json:"accounts,omitempty"`
	// Plan is what a change would stop, by tenant (confirm_required).
	Plan []any `json:"plan,omitempty"`
	// Organization is the organisation that manages an email domain (domain_managed).
	Organization *auth.OrgRef `json:"organization,omitempty"`
	// SSO is where an sso_required answer sends the person: the answer of
	// POST /api/auth/discover for the address (absent when it routes nowhere).
	SSO *auth.Discovered `json:"sso,omitempty"`
	// What a refused group request is about: how many spaces and provider
	// entries keep a group, the ids of people that do not exist.
	Spaces *int `json:"spaces,omitempty"`
	// Reason says what is wrong with a configuration (invalid, of a provider).
	Reason string `json:"reason,omitempty"`
	// Connections is how many people are connected through a provider that cannot
	// be removed (provider_in_use).
	Connections *int     `json:"connections,omitempty"`
	Mappings    *int     `json:"mappings,omitempty"`
	Missing     []string `json:"missing,omitempty"`
	// What a refused quota is about (quota_exceeded): which one, its limit, and
	// what is used.
	Quota string `json:"quota,omitempty"`
	Limit *int64 `json:"limit,omitempty"`
	Used  *int64 `json:"used,omitempty"`
	// What file_too_large is about: the size of the file (absent when the body
	// was cut off at the limit) and its name; the limit is Limit.
	Size *int64 `json:"size,omitempty"`
	Name string `json:"name,omitempty"`
	// What the storage refusals say (see "Server storage"): remote_required has
	// the requirement and, for an existing space, its deadline; not_cached says
	// whether a fetch goes on; restoring says how far the restore is; an
	// unavailable restore carries the remote's errorCode.
	Require   string `json:"require,omitempty"`
	Deadline  string `json:"deadline,omitempty"`
	Fetching  *bool  `json:"fetching,omitempty"`
	Phase     string `json:"phase,omitempty"`
	ErrorCode string `json:"errorCode,omitempty"`
}

// codedError is a 400 whose error code is code, with what it is about.
type codedError struct{ code, header, field, msg string }

func (e *codedError) Error() string { return e.msg }

func (e *codedError) body() errorBody {
	return errorBody{Error: e.code, Message: e.msg, Header: e.header, Field: e.field}
}

func writeCoded(w http.ResponseWriter, e *codedError) { writeJSON(w, http.StatusBadRequest, e.body()) }

// draftConflictBody is the 409 of the draft endpoints: the draft there is (or
// null) and the page as the file has it. Unlike errorBody, draft is always present.
type draftConflictBody struct {
	Error   string     `json:"error"`
	Message string     `json:"message"`
	Draft   *draftResp `json:"draft"`
	Current pageResp   `json:"current"`
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	// Whatever the message was made from, it goes out as a sentence.
	writeJSON(w, status, errorBody{Error: code, Message: sentence.End(msg)})
}

// readJSON decodes the request body into v. An empty body is accepted when
// optional is true. Invalid UTF-8 is refused outright: encoding/json would
// silently replace it, and the API promises 400 for it.
func (s *server) readJSON(w http.ResponseWriter, r *http.Request, v any, optional bool) bool {
	return s.readJSONMax(w, r, v, optional, s.opt.MaxBody)
}

func (s *server) readJSONMax(w http.ResponseWriter, r *http.Request, v any, optional bool, max int64) bool {
	ct := r.Header.Get("Content-Type")
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, max))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "invalid", fmt.Sprintf("The request body is larger than %d bytes.", max))
		} else {
			writeError(w, http.StatusBadRequest, "invalid", "Could not read the request body.")
		}
		return false
	}
	if len(bytes.TrimSpace(raw)) == 0 && optional {
		return true
	}
	if mt, _, err := mime.ParseMediaType(ct); err != nil || mt != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "invalid", "Send the body as application/json.")
		return false
	}
	if !utf8.Valid(raw) {
		writeError(w, http.StatusBadRequest, "invalid", "The request body is not valid UTF-8.")
		return false
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(v); err != nil {
		var inv *space.InvalidError
		if errors.As(err, &inv) && inv.Code != "" {
			writeCoded(w, &codedError{code: inv.Code, msg: inv.Msg})
			return false
		}
		writeError(w, http.StatusBadRequest, "invalid", "The request body is not valid JSON for this endpoint: "+err.Error()+".")
		return false
	}
	if dec.More() {
		writeError(w, http.StatusBadRequest, "invalid", "The request body has data after the JSON value.")
		return false
	}
	return true
}

// --- errors -----------------------------------------------------------------

func (s *server) fail(w http.ResponseWriter, err error) {
	var (
		inv   *space.InvalidError
		bq    badQuery
		ce    *space.ConflictError
		tce   *space.TypeConflictError
		tpce  *space.TemplateConflictError
		tple  *space.TemplateError
		fixe  *space.FixError
		vce   *space.ViewsConflictError
		ae    *auth.Error
		te    *tokens.Error
		dc    *drafts.Conflict
		ue    *space.UnavailableError
		urle  *gitstore.URLError
		hoste *gitstore.HostError
		qe    *spaces.QuotaError
		ftl   *space.FileTooLargeError
		nrs   *spaces.NotRestorable
		hre   *heirRequiredError
	)
	var coded *codedError
	var ghe *connections.Error
	var req *spaces.RequiredError
	var pue *spaces.PartialUnsupportedError
	var full *spaces.SpaceFullError
	var setErr *settings.Error
	var secErr *secrets.Error
	var nc *space.NotCachedError
	var rst *spaces.RestoringError
	var rf *spaces.RestoreFailed
	switch {
	case errors.As(err, &hre):
		s.failHeir(w, hre)
	case errors.Is(err, spaces.ErrExportsFull):
		writeError(w, http.StatusInsufficientStorage, "exports_full", "The exports already take the room they may; delete some, or wait for them to expire.")
	case errors.Is(err, spaces.ErrOwnerRequired):
		writeError(w, http.StatusConflict, "owner_required", "Nobody who owned the space can be put back: name an owner.")
	case errors.Is(err, store.ErrPeopleLimit):
		writeError(w, http.StatusConflict, "people_limit", "This organization has reached the number of people it may have.")
	case errors.Is(err, spaces.ErrBusyItem):
		writeError(w, http.StatusConflict, "busy", "The item is being moved, restored or purged right now.")
	case errors.Is(err, store.ErrJobLocked):
		writeError(w, http.StatusConflict, "busy", "Another job is already working on this: wait for it to end.")
	case errors.Is(err, spaces.ErrRestoreConflict):
		writeError(w, http.StatusConflict, "conflict", "A space with that id exists.")
	case errors.Is(err, spaces.ErrNeedsPerson):
		writeError(w, http.StatusConflict, "conflict", "A personal space is restored with its person: restore the person.")
	case errors.As(err, &nrs):
		writeError(w, http.StatusConflict, "conflict", nrs.Error())
	case errors.As(err, &ftl):
		var size int64
		if ftl.Size > 0 {
			size = ftl.Size
		}
		tooLarge(w, ftl.Limit, size, ftl.Name)
	case errors.As(err, &ghe):
		s.failConnection(w, ghe)
	case errors.As(err, &req):
		body := errorBody{Error: "remote_required", Message: req.Msg, Require: req.Require}
		if !req.Deadline.IsZero() {
			body.Deadline = rfc3339(req.Deadline)
		}
		writeJSON(w, http.StatusConflict, body)
	case errors.As(err, &pue):
		writeJSON(w, http.StatusConflict, errorBody{Error: "partial_unsupported", Message: pue.Error(), Provider: pue.Provider, Host: pue.Host})
	case errors.Is(err, spaces.ErrNoStorageTarget):
		writeError(w, http.StatusConflict, "no_storage_target", "This organization has no storage target to make a space at: ask its administrators to set one up, or pick another storage mode.")
	case errors.Is(err, spaces.ErrTargetNotBuilt):
		writeError(w, http.StatusNotImplemented, "not_implemented", err.Error()+".")
	case errors.As(err, &full):
		writeError(w, http.StatusInsufficientStorage, "space_full", full.Error())
	case errors.As(err, &nc):
		retry := "60"
		if nc.Fetching {
			retry = "5"
		}
		w.Header().Set("Retry-After", retry)
		f := nc.Fetching
		writeJSON(w, http.StatusServiceUnavailable, errorBody{Error: "not_cached", Message: nc.Error(), Fetching: &f})
	case errors.As(err, &rst):
		w.Header().Set("Retry-After", "5")
		writeJSON(w, http.StatusServiceUnavailable, errorBody{Error: "restoring", Message: rst.Error(), Phase: rst.Phase})
	case errors.As(err, &rf):
		if rf.RetryAfter > 0 {
			// A restore failed a moment ago and is not tried again before this.
			w.Header().Set("Retry-After", strconv.Itoa(int((rf.RetryAfter+time.Second-1)/time.Second)))
		}
		writeJSON(w, http.StatusServiceUnavailable, errorBody{Error: "unavailable", Message: rf.Msg, ErrorCode: rf.Code})
	case errors.Is(err, spaces.ErrAssetsConflict):
		writeError(w, http.StatusConflict, "conflict", strings.TrimPrefix(err.Error(), spaces.ErrAssetsConflict.Error()+": "))
	case errors.Is(err, spaces.ErrNotEvictable):
		writeError(w, http.StatusConflict, "conflict", strings.TrimPrefix(err.Error(), spaces.ErrNotEvictable.Error()+": "))
	case errors.Is(err, spaces.ErrGitTooOld), errors.Is(err, spaces.ErrConflictFlag):
		writeError(w, http.StatusConflict, "conflict", err.Error())
	case errors.As(err, &setErr):
		// A rule of the layered settings that the request goes against.
		status := http.StatusBadRequest
		if setErr.Code == settings.CodeLocked || setErr.Code == settings.CodeRestricted {
			status = http.StatusConflict
		}
		writeError(w, status, setErr.Code, setErr.Msg)
	case errors.As(err, &secErr):
		status := http.StatusBadRequest
		if secErr.Code == "not_found" {
			status = http.StatusNotFound
		}
		writeError(w, status, secErr.Code, secErr.Msg)
	case errors.As(err, &coded):
		writeCoded(w, coded)
	case errors.As(err, &te):
		s.failToken(w, te)
	case errors.As(err, &ae):
		s.failAuth(w, ae)
	case errors.As(err, &dc):
		var draft *draftResp
		if dc.Draft != nil {
			draft = toDraftResp(dc.Draft)
		}
		writeJSON(w, http.StatusConflict, draftConflictBody{Error: "conflict",
			Message: "The draft on the server is not the one you meant; here is what there is.", Draft: draft, Current: toPage(dc.Current)})
	case errors.As(err, &ue):
		writeError(w, http.StatusServiceUnavailable, "unavailable", ue.Msg)
	case errors.As(err, &urle):
		writeError(w, http.StatusBadRequest, "invalid", urle.Msg)
	case errors.As(err, &hoste):
		writeError(w, http.StatusForbidden, "forbidden", hoste.Error())
	case errors.Is(err, spaces.ErrNoAssets):
		writeError(w, http.StatusNotFound, "not_found", "This space has no attachment repository.")
	case errors.Is(err, space.ErrNoRemote):
		writeError(w, http.StatusNotFound, "not_found", "This space has no remote repository.")
	case errors.Is(err, space.ErrDiverged):
		writeError(w, http.StatusConflict, "conflict", "Both the space and the repository have content of their own, and the server will not merge two histories; make a new space for that repository.")
	case errors.Is(err, space.ErrNotTip), errors.Is(err, space.ErrNothingToAccept):
		writeError(w, http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, spaces.ErrConnected):
		writeError(w, http.StatusConflict, "conflict", err.Error())
	case errors.As(err, &qe):
		writeJSON(w, http.StatusForbidden, errorBody{Error: "quota_exceeded", Message: strings.TrimPrefix(qe.Error(), spaces.ErrLimit.Error()+": "),
			Quota: qe.Quota, Limit: &qe.Limit, Used: &qe.Used})
	case errors.Is(err, spaces.ErrLimit):
		writeError(w, http.StatusForbidden, "forbidden", strings.TrimPrefix(err.Error(), spaces.ErrLimit.Error()+": "))
	case s.failGroup(w, err):
	case errors.Is(err, spaces.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "No such space.")
	case errors.Is(err, spaces.ErrDisabled):
		writeError(w, http.StatusForbidden, "space_disabled", "This space has been disabled by an administrator.")
	case errors.Is(err, spaces.ErrBusy):
		msg := "The space is in use right now; try again in a moment."
		if why := spaces.BusyMessage(err); why != "" {
			msg = "The space is not evicted: " + why + "."
		}
		writeError(w, http.StatusConflict, "busy", msg)
	case errors.Is(err, spaces.ErrPersonalSpace):
		writeError(w, http.StatusConflict, "personal_space", "A personal space belongs to one person and cannot be transferred or deleted.")
	case errors.Is(err, spaces.ErrUserDisabled):
		writeError(w, http.StatusConflict, "user_disabled", "That user is disabled and cannot own a space.")
	case errors.Is(err, spaces.ErrForbidden):
		writeError(w, http.StatusForbidden, "no_permission", "Your role in this space does not allow that.")
	case errors.Is(err, spaces.ErrTakeover):
		writeError(w, http.StatusForbidden, "takeover_required", "Only a member of the space may change its owners, give an administrator a role in it, or change its remote; an administrator takes the space over first.")
	case errors.Is(err, spaces.ErrUserNotFound):
		writeError(w, http.StatusNotFound, "user_not_found", "There is no such user.")
	case errors.Is(err, spaces.ErrNotMember):
		writeError(w, http.StatusNotFound, "not_found", "That user is not a member of this space.")
	case errors.Is(err, spaces.ErrAlreadyMember):
		writeError(w, http.StatusConflict, "exists", "That user is a member of this space already.")
	case errors.Is(err, spaces.ErrPersonal):
		writeError(w, http.StatusConflict, "personal_space", "A personal space belongs to one person and cannot be shared.")
	case errors.Is(err, spaces.ErrLastOwner):
		writeError(w, http.StatusConflict, "last_owner", "A space needs at least one owner; make someone else an owner first.")
	case errors.Is(err, spaces.ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid", strings.TrimPrefix(err.Error(), spaces.ErrInvalid.Error()+": "))
	case errors.Is(err, space.ErrClosed):
		writeError(w, http.StatusInternalServerError, "internal", "The server is shutting down; try again when it is back.")
	case errors.Is(err, space.ErrNoVersion):
		writeError(w, http.StatusNotFound, "not_found", "That commit is not a version of this page.")
	case errors.Is(err, space.ErrNoFolder):
		writeError(w, http.StatusNotFound, "not_found", "No such folder.")
	case errors.Is(err, space.ErrNotEmpty):
		writeError(w, http.StatusConflict, "conflict", "The folder holds pages; remove them first, or ask for them to go with it.")
	case errors.Is(err, space.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "No such page.")
	case errors.Is(err, space.ErrExists), errors.Is(err, fs.ErrExist):
		writeError(w, http.StatusConflict, "exists", "It already exists.")
	case errors.As(err, &vce):
		writeJSON(w, http.StatusConflict, viewsConflict(vce))
	case errors.As(err, &fixe):
		writeJSON(w, http.StatusBadRequest, errorBody{Error: fixe.Code, Message: fixe.Msg})
	case errors.As(err, &tple):
		writeJSON(w, http.StatusBadRequest, errorBody{Error: tple.Code, Message: tple.Msg})
	case errors.As(err, &tpce):
		body := errorBody{Error: "conflict", Message: "The template changed since you opened it, or is not there; here is how it is now."}
		if tpce.Current != nil {
			body.Current = toTemplate(*tpce.Current, true)
		}
		writeJSON(w, http.StatusConflict, body)
	case errors.As(err, &tce):
		body := errorBody{Error: "conflict", Message: "The type changed since you opened it, or is not there; here is how it is now."}
		if tce.Current != nil {
			body.Current = toType(*tce.Current)
		}
		writeJSON(w, http.StatusConflict, body)
	case errors.As(err, &ce):
		writeJSON(w, http.StatusConflict, errorBody{Error: "conflict",
			Message: "The page changed since you opened it; here is the current version.", Current: toPage(ce.Current)})
	case errors.As(err, &bq):
		writeError(w, http.StatusBadRequest, "invalid", bq.Error())
	case errors.As(err, &inv):
		if inv.Code != "" {
			writeCoded(w, &codedError{code: inv.Code, msg: inv.Msg})
			break
		}
		writeError(w, http.StatusBadRequest, "invalid", inv.Msg)
	case errors.Is(err, index.ErrInvalidQuery):
		writeError(w, http.StatusBadRequest, "invalid", strings.TrimPrefix(err.Error(), index.ErrInvalidQuery.Error()+": "))
	default:
		s.log.Error("request failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "Something went wrong on the server.")
	}
}

type repoRefResp struct {
	ID       int64  `json:"id"`
	FullName string `json:"fullName"`
	HTMLURL  string `json:"htmlUrl"`
}

func (s *server) failConnection(w http.ResponseWriter, e *connections.Error) {
	body := errorBody{Error: e.Code, Message: e.Msg, Provider: e.Provider}
	if e.Repo != nil {
		body.Repo = &repoRefResp{e.Repo.ID, e.Repo.FullName, e.Repo.HTMLURL}
	}
	if e.AssetsRepo != nil {
		body.AssetsRepo = &repoRefResp{e.AssetsRepo.ID, e.AssetsRepo.FullName, e.AssetsRepo.HTMLURL}
	}
	if len(e.Missing) > 0 {
		body.Missing = e.Missing
		if e.Code == connections.CodeMissingPermission {
			body.Permission = e.Missing[0]
		}
	}
	body.InstallURL = e.InstallURL
	if e.Counts != nil {
		c, sp := e.Counts["connections"], e.Counts["spaces"]
		body.Connections, body.Spaces = &c, &sp
	}
	body.Reason = e.Reason
	if e.Code == connections.CodeRateLimited {
		secs := int((e.RetryAfter + time.Second - 1) / time.Second)
		w.Header().Set("Retry-After", strconv.Itoa(max(secs, 1)))
	}
	if e.Status >= 500 && e.Code == connections.CodeInternal {
		s.log.Error("a space made with a repository on a provider could not be recorded", "error", e.Err)
	}
	writeJSON(w, e.Status, body)
}

func (s *server) failAuth(w http.ResponseWriter, e *auth.Error) {
	switch e.Kind {
	case auth.Coded:
		if e.Status == http.StatusTooManyRequests {
			w.Header().Set("Retry-After", strconv.Itoa(max(int((e.RetryAfter+time.Second-1)/time.Second), 1)))
		}
		writeJSON(w, e.Status, errorBody{Error: e.Code, Message: e.Msg, Accounts: e.Accounts, Organization: e.Organization, SSO: e.SSO})
	case auth.Invalid:
		writeError(w, http.StatusBadRequest, "invalid", e.Msg)
	case auth.Unauthenticated:
		// Reason is there only when the cookie was a session that ended.
		writeJSON(w, http.StatusUnauthorized, errorBody{Error: "unauthenticated", Message: e.Msg, Reason: e.Reason})
	case auth.Forbidden:
		writeError(w, http.StatusForbidden, "forbidden", e.Msg)
	case auth.NotFound:
		writeError(w, http.StatusNotFound, "not_found", e.Msg)
	case auth.Conflict:
		writeError(w, http.StatusConflict, "conflict", e.Msg)
	case auth.Exists:
		writeError(w, http.StatusConflict, "exists", e.Msg)
	case auth.RateLimited:
		secs := int((e.RetryAfter + time.Second - 1) / time.Second)
		w.Header().Set("Retry-After", strconv.Itoa(max(secs, 1)))
		writeError(w, http.StatusTooManyRequests, "rate_limited", e.Msg)
	default:
		writeError(w, http.StatusInternalServerError, "internal", e.Msg)
	}
}

// --- resources --------------------------------------------------------------

func rfc3339(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05Z") }

type pageResp struct {
	ID      string    `json:"id"`
	Path    string    `json:"path"`
	Folder  string    `json:"folder"`
	Rev     string    `json:"rev"`
	Updated string    `json:"updated"`
	Meta    page.Meta `json:"meta"`
	Header  string    `json:"header"`
	Body    string    `json:"body"`
	// EffectiveType is the type the page has: its header's, else its folder's.
	EffectiveType string `json:"effectiveType"`
	// PartialProps are the keys of meta that a listing holds only in part
	// (the same list as the summary's); left out when there are none.
	PartialProps []string `json:"partialProps,omitempty"`
	// TextRev is in the answer to a save and a commit only: the rev the text
	// that was sent stands for, before the link rewrites the save made
	// (docs/api.md, "Typing during a save"). Equal to Rev when there were none.
	TextRev string `json:"textRev,omitempty"`
}

// toSaved is toPage for the answer to a save or a commit, with textRev. A save
// that wrote nothing (the draft was the file already) rewrote nothing either.
func toSaved(p *space.Page) pageResp {
	r := toPage(p)
	r.TextRev = p.TextRev
	if r.TextRev == "" {
		r.TextRev = p.Rev
	}
	return r
}

func toPage(p *space.Page) pageResp {
	m := p.Meta
	if m == nil {
		m = page.Meta{}
	}
	return pageResp{ID: p.ID, Path: p.Path, Folder: p.Folder, Rev: p.Rev, Updated: rfc3339(p.Updated), Meta: m, Header: p.Header, Body: p.Body, EffectiveType: p.EffectiveType, PartialProps: p.PartialProps}
}

type summaryResp struct {
	ID       string   `json:"id"`
	Path     string   `json:"path"`
	Folder   string   `json:"folder"`
	Title    string   `json:"title,omitempty"`
	Subtitle string   `json:"subtitle,omitempty"`
	Source   []string `json:"source"`
	Label    string   `json:"label"`
	Untitled bool     `json:"untitled"`
	Tags     []string `json:"tags"`
	Type     string   `json:"type"`
	Created  string   `json:"created,omitempty"`
	Updated  string   `json:"updated"`
	Excerpt  string   `json:"excerpt"`
	// Icon and Color are the page's own (a Lucide name or an emoji, one of the eight colours), from its header.
	Icon  string `json:"icon,omitempty"`
	Color string `json:"color,omitempty"`
	// EffectiveType is the type the page has: its header's, else its folder's.
	EffectiveType string `json:"effectiveType"`
	// Props and Rev are there when the listing was asked for them (props=1).
	Props json.RawMessage `json:"props,omitempty"`
	Rev   string          `json:"rev,omitempty"`
	// Body is the page's Markdown, there when the listing was asked for it
	// (body=1).
	Body *string `json:"body,omitempty"`
	// PartialProps are the keys of props that are not the whole of the value
	// in the file (see the index); left out when there are none.
	PartialProps []string `json:"partialProps,omitempty"`
}

func toSummary(s index.Summary, types space.TypeResolver) summaryResp {
	return summaryResp{ID: s.ID, Path: s.Path, Folder: s.Folder, Title: s.Title, Subtitle: s.Subtitle, Source: nonNil(s.Sources), Label: s.Label,
		Untitled: s.Untitled, Tags: s.Tags, Type: s.Type, Created: s.Created, Updated: rfc3339(s.Updated), Excerpt: s.Excerpt, Icon: s.Icon, Color: s.Color,
		EffectiveType: types.For(s.Folder, s.Type), Props: s.Props, Rev: s.Rev, PartialProps: s.PartialProps}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// boxOf is the server's box of sealed secrets: the space manager's (nil in
// single-user mode, which has no secrets to keep).
func boxOf(opt Options) *secret.Box {
	if opt.Spaces == nil {
		return nil
	}
	return opt.Spaces.Secrets()
}
