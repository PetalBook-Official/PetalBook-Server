package spaces

// The server's storage policy (milestone 3.1, docs/api.md "Server storage"):
// the site's settings, the requirement of a remote with its grace period, the
// limit on local spaces, and the record of when a space was used. The
// settings are layered settings (storage.*, cache.*; see internal/settings),
// worked out into one value for each set of their values: a request reads it
// with one atomic load, and a change applies to the next request. Eviction and restoring are in evict.go.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"petalbook.app/server/internal/connections"
	"petalbook.app/server/internal/gitstore"
	"petalbook.app/server/internal/settings"
	"petalbook.app/server/internal/store"
)

// Limits of the settings.
const (
	MinLocalMaxBytes   = 1 << 20
	MinClonesBytes     = 1 << 30
	DefaultEvictDays   = 14
	DefaultIdleHours   = 24
	DefaultFreeBytes   = 10 << 30
	DefaultAssetsBytes = 5 << 30
	MinAssetsBytes     = 64 << 20
	DefaultGraceDays   = 30
	maxGraceDays       = 365
	maxEvictDays       = 365
	maxIdleHours       = 720
	touchEvery         = time.Hour
	evictRoundEvery    = 10 * time.Minute
	maxRestoresAtOnce  = 2
)

// StorageSettings is the site's storage policy.
type StorageSettings struct {
	Remotes struct {
		Require   *string   `json:"require"` // nil, "any" or the id of a provider ("github")
		Scope     string    `json:"scope"`   // "new" or "all"
		GraceDays int       `json:"graceDays"`
		Since     time.Time `json:"since"` // when the requirement took its present form
	} `json:"remotes"`
	Hosts struct {
		Allowed []string `json:"allowed"`
	} `json:"hosts"`
	LocalSpaces struct {
		MaxBytes *int64 `json:"maxBytes"`
	} `json:"localSpaces"`
	Cache struct {
		Partial        bool   `json:"partial"`
		EvictAfterDays *int   `json:"evictAfterDays"`
		MinIdleHours   int    `json:"minIdleHours"`
		ClonesBytes    *int64 `json:"clonesBytes"`
		MinFreeBytes   *int64 `json:"minFreeBytes"`
		AssetsBytes    int64  `json:"assetsBytes"`
	} `json:"cache"`
}

// DefaultStorage is the policy of a server whose administrator has not
// changed anything. partial says whether Git can make partial clones.
func DefaultStorage(partialOK bool) StorageSettings {
	var s StorageSettings
	s.Remotes.Scope = "new"
	s.Remotes.GraceDays = DefaultGraceDays
	s.Hosts.Allowed = append([]string{}, gitstore.DefaultHosts...)
	s.Cache.Partial = partialOK
	d := DefaultEvictDays
	s.Cache.EvictAfterDays = &d
	s.Cache.MinIdleHours = DefaultIdleHours
	f := int64(DefaultFreeBytes)
	s.Cache.MinFreeBytes = &f
	s.Cache.AssetsBytes = DefaultAssetsBytes
	return s
}

// RemoteNotice is what a space shows of the requirement it does not meet.
type RemoteNotice struct {
	Required string
	Deadline time.Time
	Overdue  bool
}

// RequiredError is the refusal of a change the storage policy does not allow
// (409 remote_required).
type RequiredError struct {
	Require  string
	Deadline time.Time // zero for a new space or a removal
	Msg      string
}

func (e *RequiredError) Error() string { return e.Msg }

// SpaceFullError is the refusal of an upload or import over the limit on
// local spaces (507 space_full).
type SpaceFullError struct{ Limit, Size int64 }

func (e *SpaceFullError) Error() string {
	return fmt.Sprintf("this space would be over the limit of %d bytes the administrator has set for spaces that are not on a remote repository", e.Limit)
}

// ErrConflictFlag: a setting the command line decided.
var ErrConflictFlag = errors.New("this setting was given on the command line, which wins")

// ErrGitTooOld: partial clones need Git 2.44.
var ErrGitTooOld = errors.New("this server's Git is older than 2.44 and cannot make partial clones")

// storageState is the part of the manager the storage policy uses.
type storageState struct {
	once sync.Once
	set  *settings.Service
	// derived is the policy worked out from the settings, with the number of the
	// set of settings it was worked out from (see settings.Service.Site).
	derived atomic.Pointer[derivedStorage]
	buildMu sync.Mutex

	factsMu sync.Mutex
	facts   map[string]store.PolicyFacts

	touchMu sync.Mutex
	touched map[string]time.Time // space id -> when its use was last written to the database
	used    map[string]time.Time // space id -> its latest use

	gitVersion string
	gitPartial bool
}

type derivedStorage struct {
	serial uint64
	s      StorageSettings
}

// sinceRecord is the site record that keeps when the requirement of a remote
// took its present form: observed state, not a setting.
const sinceRecord = "storage"

// sinceRec is when the requirement in force took its present form, and which
// requirement that was (policyKey): the grace period counts from Since, so a
// change of anything the requirement is made of starts it afresh.
type sinceRec struct {
	Since   time.Time `json:"since"`
	Key     string    `json:"key"`
	Require string    `json:"require"`
	Scope   string    `json:"scope"`
}

// nextSince is when the requirement (require, scope) took its form, given the
// record of the one before: now when it begins, changes what is required or
// widens the scope to every space, nothing when it ends, and what it was when
// only the hosts changed or the scope narrowed, so the spaces it covered stay
// covered and no grace period is shortened or made longer by the change.
func nextSince(old sinceRec, require, scope string, now time.Time) time.Time {
	now = now.UTC().Truncate(time.Millisecond)
	switch {
	case require == "":
		return time.Time{}
	case old.Require == "", old.Require != require, old.Scope != "all" && scope == "all":
		return now
	}
	return old.Since
}

// initStorage reads the Git version and binds the storage settings; it is
// called once.
func (m *Manager) initStorage(ctx context.Context) {
	m.st.once.Do(func() {
		m.st.gitVersion, m.st.gitPartial = gitstore.GitVersion(ctx, m.opt.RemoteRunner)
		if m.opt.GitSupportsPartial != nil {
			m.st.gitPartial = *m.opt.GitSupportsPartial
		}
		if m.single == nil {
			m.st.set = settings.Of(m.opt.Store)
			m.bindSettings()
		}
	})
}

// hostsDefault is the hosts the server was started with (the default of
// --remote-host) as the starting point of storage.hosts.
func (m *Manager) hostsDefault() []string {
	if len(m.opt.RemotePolicy.Hosts) > 0 {
		return append([]string{}, m.opt.RemotePolicy.Hosts...)
	}
	return append([]string{}, gitstore.DefaultHosts...)
}

// bindSettings makes the layered settings check the storage policy as the
// policy needs, and keep its "since".
func (m *Manager) bindSettings() {
	set := m.st.set
	bad := func(f string, a ...any) error { return fmt.Errorf("%w: "+f, append([]any{ErrInvalid}, a...)...) }
	hosts := m.hostsDefault()
	set.SetDefault(settings.StorageHosts, hosts)
	hb := settings.SiteBinding{Check: func(_ context.Context, _ string, v any) (any, error) {
		in, _ := v.([]string)
		if len(in) == 0 {
			return nil, bad("storage.hosts needs at least one host (or *)")
		}
		out := make([]string, len(in))
		for i, h := range in {
			h = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(h, ".")))
			if h != "*" && (h == "" || strings.ContainsAny(h, " /:@\\") || len(h) > 253) {
				return nil, bad("storage.hosts: %q is not a host name", in[i])
			}
			out[i] = h
		}
		return out, nil
	}}
	if m.opt.HostsFromFlag {
		// The flag decides: the value is read-only.
		hb.Get = func(context.Context, store.Q) (any, error) { return hosts, nil }
		hb.Set = func(context.Context, store.Q, any) error { return ErrConflictFlag }
		hb.ReadOnly = func() bool { return true }
	}
	set.Bind(settings.StorageHosts, hb)

	set.Bind(settings.StorageRemotesRequire, settings.SiteBinding{
		Check: func(ctx context.Context, _ string, v any) (any, error) {
			r, _ := v.(string)
			r = strings.TrimSpace(r)
			if r == connections.GitID {
				return nil, bad(`storage.remotes.require is "", "any" or the id of a provider`)
			}
			if r != "" && r != "any" {
				// A provider this server has: requiring one it does not have would
				// leave nobody a way to meet the requirement.
				if _, ok := m.opt.Connections.Get(r); !ok {
					return nil, bad(`storage.remotes.require is "", "any" or the id of a provider of this server`)
				}
				p, err := m.provider(r)
				if err != nil {
					return nil, err
				}
				if in, err := p.Info(ctx, ""); err != nil {
					return nil, err
				} else if !in.Configured {
					d := p.Descriptor()
					return nil, connections.Errorf(d.ID, http.StatusConflict, connections.CodeNotConfigured, "This server has no %s connection set up; set it up before requiring it.", d.Name)
				}
			}
			return r, nil
		},
	})
	set.Bind(settings.CachePartial, settings.SiteBinding{Check: func(_ context.Context, _ string, v any) (any, error) {
		if v == true && !m.st.gitPartial {
			return nil, ErrGitTooOld
		}
		return v, nil
	}})
}

// policyKey names a requirement: what is required, of which spaces, on which
// hosts. Anything that changes it starts the grace period again.
func policyKey(require, scope string, hosts []string) string {
	return require + "|" + scope + "|" + strings.Join(hosts, ",")
}

func (m *Manager) readSince(ctx context.Context) (sinceRec, error) {
	var rec sinceRec
	raw, _, err := store.GetRecord(ctx, m.opt.Store.Q(), sinceRecord)
	if errors.Is(err, store.ErrNotFound) {
		return rec, nil
	}
	if err != nil {
		return rec, err
	}
	if err := json.Unmarshal([]byte(raw), &rec); err != nil {
		return sinceRec{}, nil
	}
	return rec, nil
}

// SettlePolicy keeps the record of when the requirement took its present form
// in step with the settings: when the requirement in force (derived from
// storage.mode or set, with its scope and hosts) is not the one the record
// names, the record is written for it, "now" for a requirement and nothing for
// none. A change never makes a space overdue sooner than the grace period the
// change itself gives. It runs at start and after a write of any setting the
// requirement is made of, outside any transaction (it reads settings).
func (m *Manager) SettlePolicy(ctx context.Context) error {
	if m.single != nil {
		return nil
	}
	m.initStorage(ctx)
	eff, _, err := m.st.set.Site(ctx)
	if err != nil {
		return err
	}
	require, _ := eff[settings.StorageRemotesRequire].Value.(string)
	if require == "" {
		require = m.derivedRequire(eff)
	}
	scope, _ := eff[settings.StorageRemotesScope].Value.(string)
	hosts, _ := eff[settings.StorageHosts].Value.([]string)
	key := policyKey(require, scope, hosts)
	rec, err := m.readSince(ctx)
	if err != nil {
		return err
	}
	if rec.Key == key {
		return nil
	}
	rec = sinceRec{Key: key, Require: require, Scope: scope, Since: nextSince(rec, require, scope, m.opt.Now())}
	raw, _ := json.Marshal(rec)
	if _, err := store.PutRecord(ctx, m.opt.Store.Q(), sinceRecord, string(raw), m.opt.Now()); err != nil {
		return err
	}
	m.st.derived.Store(nil)
	return nil
}

// Storage is the site's storage policy as it is now: worked out from the
// settings (storage.*, cache.*) once for each set of their values, so a request
// costs one atomic load and the settings' own cached read.
func (m *Manager) Storage() StorageSettings {
	if m.single != nil {
		return DefaultStorage(false)
	}
	m.initStorage(context.Background())
	ctx := context.Background()
	eff, serial, err := m.st.set.Site(ctx)
	if err != nil {
		m.log.Warn("could not read the storage settings; the last ones apply", "error", err)
		if d := m.st.derived.Load(); d != nil {
			return d.s
		}
		return m.defaultStorage()
	}
	if d := m.st.derived.Load(); d != nil && d.serial == serial {
		return d.s
	}
	m.st.buildMu.Lock()
	defer m.st.buildMu.Unlock()
	if d := m.st.derived.Load(); d != nil && d.serial == serial {
		return d.s
	}
	s, unsettled := m.buildStorage(ctx, eff)
	if unsettled {
		// The record of when the requirement took its form is for another
		// requirement (a change whose settling failed, or one made while the
		// server was down): settle it now, and take it as it then is. If that
		// fails too the answer is not kept, so the next call tries again.
		if err := m.SettlePolicy(ctx); err != nil {
			m.log.Warn("could not settle when the remote requirement took its form", "error", err)
			return s
		}
		s, _ = m.buildStorage(ctx, eff)
	}
	m.st.derived.Store(&derivedStorage{serial: serial, s: s})
	return s
}

// defaultStorage is DefaultStorage with the hosts the server was started with.
func (m *Manager) defaultStorage() StorageSettings {
	s := DefaultStorage(m.st.gitPartial)
	s.Hosts.Allowed = m.hostsDefault()
	return s
}

func (m *Manager) buildStorage(ctx context.Context, eff map[string]*settings.Effective) (StorageSettings, bool) {
	var s StorageSettings
	str := func(k string) string { v, _ := eff[k].Value.(string); return v }
	num := func(k string) *int64 {
		if n, ok := eff[k].Value.(int64); ok {
			return &n
		}
		return nil
	}
	if r := str(settings.StorageRemotesRequire); r != "" {
		s.Remotes.Require = &r
	}
	s.Remotes.Scope = str(settings.StorageRemotesScope)
	s.Remotes.GraceDays = int(*num(settings.StorageRemotesGraceDays))
	if s.Remotes.Require == nil {
		// No requirement of its own: the modes the site allows may imply one.
		if r := m.derivedRequire(eff); r != "" {
			s.Remotes.Require = &r
		}
	}
	hosts, _ := eff[settings.StorageHosts].Value.([]string)
	// When the requirement in force took its form. A record that is for another
	// requirement than the one the settings now make (a change not settled yet,
	// or a server that started with the settings changed) says "now": a grace
	// period never starts in the past.
	key := policyKey(deref(s.Remotes.Require), s.Remotes.Scope, hosts)
	rec, err := m.readSince(ctx)
	unsettled := false
	switch {
	case err != nil:
		m.log.Warn("could not read when the remote requirement took its form", "error", err)
		s.Remotes.Since = nextSince(sinceRec{}, deref(s.Remotes.Require), s.Remotes.Scope, m.opt.Now())
	case rec.Key != key:
		unsettled = true
		s.Remotes.Since = nextSince(rec, deref(s.Remotes.Require), s.Remotes.Scope, m.opt.Now())
	default:
		s.Remotes.Since = rec.Since
	}
	s.Hosts.Allowed, _ = eff[settings.StorageHosts].Value.([]string)
	s.Hosts.Allowed = append([]string{}, s.Hosts.Allowed...)
	s.LocalSpaces.MaxBytes = num(settings.StorageLocalMaxBytes)
	c := &s.Cache
	c.Partial, _ = eff[settings.CachePartial].Value.(bool)
	c.Partial = c.Partial && m.st.gitPartial
	if n := num(settings.CacheEvictAfterDays); n != nil {
		d := int(*n)
		c.EvictAfterDays = &d
	}
	c.MinIdleHours = int(*num(settings.CacheMinIdleHours))
	c.ClonesBytes = num(settings.CacheClonesBytes)
	c.MinFreeBytes = num(settings.CacheMinFreeBytes)
	c.AssetsBytes = *num(settings.CacheAssetsBytes)
	return s, unsettled
}

// StoragePolicyChanged tells the manager that the requirement of a remote
// changed: what is cached of it is dropped, and the people's counts of spaces
// by deadline follow the policy. It returns how many spaces the policy asks
// something of.
func (m *Manager) StoragePolicyChanged(ctx context.Context) (Affected, error) {
	if err := m.SettlePolicy(ctx); err != nil {
		return Affected{}, err
	}
	m.dropFacts("")
	if _, err := m.opt.Store.SettleDeadlines(ctx, m.remotePolicy()); err != nil {
		m.log.Warn("could not count the spaces by deadline", "error", err)
	}
	return m.Affected(ctx)
}

// GitInfo is the Git the server runs and whether it can make partial clones.
func (m *Manager) GitInfo() (version string, partial bool) {
	m.initStorage(context.Background())
	return m.st.gitVersion, m.st.gitPartial
}

// HostsFromFlag reports that --remote-host decided the allowed hosts.
func (m *Manager) HostsFromFlag() bool { return m.opt.HostsFromFlag }

// remotePolicy is the requirement of a remote, for the queries.
func (m *Manager) remotePolicy() store.RemotePolicy {
	s := m.Storage()
	p := store.RemotePolicy{Now: m.opt.Now()}
	if s.Remotes.Require != nil {
		p.Require = *s.Remotes.Require
	}
	p.All = s.Remotes.Scope == "all"
	p.Since = s.Remotes.Since
	p.Grace = time.Duration(s.Remotes.GraceDays) * 24 * time.Hour
	return p
}

// hostPolicy is the address policy in force: the hosts of the settings, unless
// the flag was given.
func (m *Manager) hostPolicy() gitstore.Policy {
	p := m.opt.RemotePolicy
	if !m.opt.HostsFromFlag && m.single == nil {
		p.Hosts = m.Storage().Hosts.Allowed
	}
	return p
}

// Affected is how many spaces the policy asks something of.
type Affected struct {
	InGrace int `json:"inGrace"`
	Overdue int `json:"overdue"`
	Exempt  int `json:"exempt"`
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// Affected counts the spaces the requirement asks something of now: one query.
func (m *Manager) Affected(ctx context.Context) (Affected, error) {
	g, o, e, err := store.PolicyCounts(ctx, m.opt.Store.Q(), m.remotePolicy())
	return Affected{g, o, e}, err
}

// --- the requirement of a remote --------------------------------------------------

// policyFacts reads what the requirement looks at in a space; cached, and
// dropped when a remote or an exemption changes.
func (m *Manager) policyFacts(ctx context.Context, id string) (store.PolicyFacts, error) {
	m.st.factsMu.Lock()
	f, ok := m.st.facts[id]
	m.st.factsMu.Unlock()
	if ok {
		return f, nil
	}
	f, err := store.GetPolicyFacts(ctx, m.opt.Store.Q(), id)
	if err != nil {
		return f, err
	}
	m.st.factsMu.Lock()
	if m.st.facts == nil || len(m.st.facts) > 50_000 {
		m.st.facts = map[string]store.PolicyFacts{}
	}
	m.st.facts[id] = f
	m.st.factsMu.Unlock()
	return f, nil
}

// dropFacts forgets what is cached about a space's requirement ("" for all).
func (m *Manager) dropFacts(id string) {
	m.st.factsMu.Lock()
	if id == "" {
		m.st.facts = nil
	} else {
		delete(m.st.facts, id)
	}
	m.st.factsMu.Unlock()
}

// PolicyChanged tells the manager that an exemption of a user changed.
func (m *Manager) PolicyChanged() { m.dropFacts("") }

// acceptable reports whether a remote meets the requirement: its credential
// type ("" for none) and the provider it goes through. "any" takes any remote;
// a provider's id takes a connection through that provider.
func acceptable(require, cred, provider string) bool {
	switch require {
	case "":
		return true
	case "any":
		return cred != ""
	}
	return cred == CredConnection && provider == require
}

// Notice says what requirement a space does not meet, with its deadline; nil
// when it meets it, is exempt, or is not asked.
func (m *Manager) Notice(ctx context.Context, id string) (*RemoteNotice, error) {
	if m.single != nil {
		return nil, nil
	}
	p := m.remotePolicy()
	if !p.Active() {
		return nil, nil
	}
	f, err := m.policyFacts(ctx, id)
	if err != nil {
		return nil, err
	}
	if f.Exempt || f.OwnerExempt || acceptable(p.Require, f.CredType, f.Provider) {
		return nil, nil
	}
	dl, ok := p.Deadline(f.Created, f.RemoteLost)
	if !ok {
		return nil, nil
	}
	return &RemoteNotice{Required: p.Require, Deadline: dl, Overdue: p.Now.After(dl)}, nil
}

// CheckWritable refuses a change to the files of a space that is past its
// deadline (409 remote_required): it is read only until it follows a remote of
// the required kind. Drafts do not come here; they are not files of the space.
func (m *Manager) CheckWritable(id string) error {
	if m.single != nil {
		return nil
	}
	if m.Storage().Remotes.Require == nil {
		return nil // the usual case: one load
	}
	n, err := m.Notice(context.Background(), id)
	if err != nil || n == nil || !n.Overdue {
		return nil // a failed lookup does not stop someone writing
	}
	return &RequiredError{Require: n.Required, Deadline: n.Deadline,
		Msg: "This space is past its deadline for following a remote repository (" + m.describeRequire(n.Required) + "), so it is read only until it does."}
}

func (m *Manager) describeRequire(r string) string {
	if r == "any" || r == "" {
		return "on an allowed host"
	}
	return "on " + m.providerName(r)
}

// providerName is the display name of a provider ("GitHub"), or its id.
func (m *Manager) providerName(id string) string {
	if p, ok := m.opt.Connections.Get(id); ok {
		return p.Descriptor().Name
	}
	return id
}

// CheckNewSpace refuses a new space that does not have what the policy
// requires, before anything is made. cred is the credential type the request
// brings ("" for none) and provider the provider it goes through; exempt
// users are not asked.
func (m *Manager) CheckNewSpace(ctx context.Context, userID, cred, provider string) error {
	if m.single != nil {
		return nil
	}
	p := m.remotePolicy()
	if !p.Active() || acceptable(p.Require, cred, provider) {
		return nil
	}
	if u, err := store.GetUser(ctx, m.opt.Store.Q(), store.AllTenants(), userID); err == nil && u.RemoteExempt { // the creator, by id
		return nil
	}
	msg := "This server requires a space to follow a remote repository"
	if p.Require != "any" {
		msg += " " + m.describeRequire(p.Require) + ": make it with storage.sync on " + m.providerName(p.Require)
	} else {
		msg += " on an allowed host: give a remote or storage.sync"
	}
	return &RequiredError{Require: p.Require, Msg: msg + "."}
}

// CheckRemoval refuses removing the remote of a space the policy asks for one.
//
// Besides the site's requirement, an organisation whose own rules leave out the
// "server" mode (so a space of its own must follow a remote) does not let one
// go back to the server: the space's tenant decides, exempt spaces and people
// excepted.
func (m *Manager) CheckRemoval(ctx context.Context, id string) error {
	if m.single != nil {
		return nil
	}
	f, err := m.policyFacts(ctx, id)
	if err != nil {
		return err
	}
	if f.Exempt || f.OwnerExempt {
		return nil
	}
	if !f.Personal { // the personal space is made local whatever the modes say
		m.initStorage(ctx)
		r, err := m.st.set.StorageModeRule(ctx, f.TenantID)
		if err != nil {
			return err
		}
		if !slices.Contains(r.Allowed, ModeServer) {
			return &RequiredError{Require: "any", Msg: "This organization requires a space to follow a remote repository; it cannot be removed."}
		}
	}
	p := m.remotePolicy()
	if !p.Active() {
		return nil
	}
	if _, ok := p.Deadline(f.Created, f.RemoteLost); !ok {
		return nil // not covered by a requirement of new spaces only
	}
	return &RequiredError{Require: p.Require, Msg: "This server requires the space to follow a remote repository; it cannot be removed."}
}

// CheckCredential refuses a credential the requirement does not accept: its
// type and the provider it goes through.
func (m *Manager) CheckCredential(ctx context.Context, id, cred, provider string) error {
	if m.single != nil {
		return nil
	}
	p := m.remotePolicy()
	if !p.Active() || acceptable(p.Require, cred, provider) {
		return nil
	}
	f, err := m.policyFacts(ctx, id)
	if err != nil {
		return err
	}
	if f.Exempt || f.OwnerExempt {
		return nil
	}
	if _, ok := p.Deadline(f.Created, f.RemoteLost); !ok {
		return nil // not covered by a requirement of new spaces only
	}
	return &RequiredError{Require: p.Require, Msg: "This server requires a remote repository " + m.describeRequire(p.Require) + "; use a connection to " + m.providerName(p.Require) + "."}
}

// --- a limit on local spaces -------------------------------------------------------

// CheckRoom refuses an upload or import of extra bytes into a local space that
// would take it over the limit (507 space_full). The size is the stored
// measurement, up to hours old; within 10 % of the limit the space is measured
// again first. A space that follows a remote is not limited: its copy is a cache.
func (m *Manager) CheckRoom(ctx context.Context, h *Handle, extra int64) error {
	if m.single != nil {
		return nil
	}
	s := m.Storage()
	limit := s.LocalSpaces.MaxBytes
	if limit == nil || h.Info.Sync != "local" {
		return nil
	}
	row, err := store.GetAdminSpace(ctx, m.opt.Store.Q(), store.AllTenants(), h.Info.ID) // the space the caller has open
	if err != nil {
		return err
	}
	var size int64
	if row.SizeBytes != nil {
		size = *row.SizeBytes
	}
	if row.SizeBytes == nil || size+extra > *limit || float64(size+extra) > 0.9*float64(*limit) {
		if n, _, err := m.Resize(ctx, store.AllTenants(), h.Info.ID); err == nil { // the space the caller has open
			size = n
		}
	}
	if size+extra > *limit {
		return &SpaceFullError{Limit: *limit, Size: size}
	}
	return nil
}

// --- when a space was used ---------------------------------------------------------

// noteUse records that a request opened the space. The database is written at
// most once an hour per space; the time in memory is what the eviction reads
// first.
func (m *Manager) noteUse(id string) {
	if m.single != nil {
		return
	}
	now := m.opt.Now()
	m.st.touchMu.Lock()
	if m.st.touched == nil {
		m.st.touched, m.st.used = map[string]time.Time{}, map[string]time.Time{}
	}
	if len(m.st.used) > 100_000 {
		m.st.used = map[string]time.Time{}
	}
	m.st.used[id] = now
	last, seen := m.st.touched[id]
	if seen && now.Sub(last) < touchEvery {
		m.st.touchMu.Unlock()
		return
	}
	if len(m.st.touched) > 100_000 {
		m.st.touched = map[string]time.Time{}
	}
	m.st.touched[id] = now
	m.st.touchMu.Unlock()
	m.goBg(func() {
		ctx, cancel := context.WithTimeout(m.closedCtx(), 10*time.Second)
		defer cancel()
		if err := store.TouchSpaceUsed(ctx, m.opt.Store.Q(), id, now); err != nil {
			m.log.Warn("could not record the use of a space", "space", id, "error", err)
		}
	})
}

// lastUsed is the time of the space's last use: the later of the database's
// and the one in memory.
func (m *Manager) lastUsed(db time.Time, id string) time.Time {
	m.st.touchMu.Lock()
	t := m.st.used[id]
	m.st.touchMu.Unlock()
	if t.After(db) {
		return t
	}
	return db
}

// RemotePolicy is the requirement of a remote as the queries use it, as of now.
func (m *Manager) RemotePolicy() store.RemotePolicy { return m.remotePolicy() }

// LastUsed is the time of the space's last use: the later of the stored one and
// the one in memory.
func (m *Manager) LastUsed(db time.Time, id string) time.Time { return m.lastUsed(db, id) }

// NoticeOfRow is Notice for a row the administration's list has read.
func (m *Manager) NoticeOfRow(r store.AdminSpace) *RemoteNotice {
	p := m.remotePolicy()
	if !p.Active() || r.Storage.Exempt || r.OwnerExempt || acceptable(p.Require, r.CredType, r.Provider) {
		return nil
	}
	dl, ok := p.Deadline(r.Created, r.RemoteLostAt)
	if !ok {
		return nil
	}
	return &RemoteNotice{Required: p.Require, Deadline: dl, Overdue: p.Now.After(dl)}
}
