package spaces

// Remote repositories of spaces: connecting, disconnecting, status, and the
// puller.
//
// Where pulling lives. Spaces are opened lazily and closed when idle, so there
// is no goroutine per space (and none per closed space). One loop in the
// manager (RunPull) looks, every few seconds, at the spaces that are open right
// now and follow a remote, and pulls those whose turn it is (every
// --pull-interval, jittered so spaces do not all fetch together), a few at a
// time. A space that is closed is not pulled: nobody is looking at it, it costs
// nothing, and the pull that every opening starts brings it up to date at the
// moment someone is. A pull never counts as use, so a space that is only being
// pulled still goes idle and closes.

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"petalbook.app/server/internal/connections"
	"petalbook.app/server/internal/gitstore"
	"petalbook.app/server/internal/space"
	"petalbook.app/server/internal/store"
)

// DefaultPullInterval is the flag's default.
const DefaultPullInterval = time.Minute

// pullWorkers bounds how many remotes are fetched at the same time.
const pullWorkers = 4

var (
	// ErrConnected: the space follows a remote; this is only possible while it is local.
	ErrConnected = errors.New("the space follows a remote repository; disconnect it first")
	// ErrNoRemote: the space is local.
	ErrNoRemote = space.ErrNoRemote
)

// Credential types.
const (
	CredSSH        = "ssh"
	CredToken      = "token"
	CredConnection = store.CredConnection
)

// RemoteView is a remote as the API shows it.
type RemoteView struct {
	URL, Branch string
	CredType    string
	Username    string // token
	PublicKey   string // ssh
	Fingerprint string // ssh: of the public key
	HostKey     string // ssh: fingerprint of the host key trusted on first use
	State       string
	LastSync    time.Time
	Error       string
	ErrorCode   string // what an error state needs, for programs
	Rewritten   *Rewritten
	Provider    string          // the provider the remote goes through; "git" for a plain address
	Connection  *ConnectionView // CredType "connection"
}

// ConnectionView is the connection credential of a remote, as the API shows it.
type ConnectionView struct {
	Via            string
	InstallationID int64
	Account        string
	RepoID         int64
	UserID         string
	Username       string
	Login          string
}

// Rewritten says which commits the paused remote is about.
type Rewritten struct{ LastGood, Remote string }

// ConnectRequest is what the owner gives to connect a space.
type ConnectRequest struct {
	URL, Branch string
	CredType    string
	Username    string
	Token       string // empty keeps the stored one
	// A connection: Provider is the provider's id, Via is "app" or "user" (or ""
	// to let the server choose, for a new space), UserID the caller.
	Provider, Via, UserID string
	// Mode is the storage mode asked for a new space (CreateWithRemote); "" for none.
	Mode string
}

func (m *Manager) knownHosts(id string) string {
	return filepath.Join(m.opt.DataDir, "known_hosts", id)
}

func (m *Manager) box() (interface {
	Seal(spaceID, purpose string, plaintext []byte) (string, error)
	Open(spaceID, purpose, sealed string) ([]byte, error)
}, error) {
	if m.opt.Secrets == nil {
		return nil, errors.New("this server has no key for storing credentials")
	}
	return m.opt.Secrets, nil
}

// credential builds what git runs with from a stored row. The secrets stay
// encrypted until a command needs them.
func (m *Manager) credential(id string, r *store.Remote) gitstore.Credential {
	switch r.CredType {
	case CredToken:
		enc := r.TokenEnc
		return gitstore.Token{Username: r.Username, URL: r.URL, Get: func() (string, error) {
			b, err := m.opt.Secrets.Open(id, "token", enc)
			return string(b), err
		}}
	case CredSSH:
		enc := r.PrivKeyEnc
		return gitstore.SSH{KnownHosts: m.knownHosts(id), Key: func() ([]byte, error) {
			return m.opt.Secrets.Open(id, "ssh-key", enc)
		}}
	case CredConnection:
		row := *r
		prov := r.Provider
		user := m.connectionGitUser(prov)
		return gitstore.Token{Username: user, URL: r.URL, Get: func() (string, error) {
			p, ok := m.opt.Connections.Get(prov)
			if !ok {
				return "", &gitstore.CredentialError{Code: connections.CodeNotConfigured, Msg: "This server has no connection to " + prov + " any more."}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			m.mu.Lock()
			staging := m.staging[id]
			m.mu.Unlock()
			return p.GitToken(ctx, &row, staging)
		}}
	}
	return nil
}

// connectionGitUser is the user name Git is given with a provider's token. A
// provider that is not loaded (it is loaded after a start, or was removed) is
// asked of what is stored about it, never answered with GitHub's name: GitLab
// refuses that. For one nobody knows it is "": the token cannot be had either,
// and says so.
func (m *Manager) connectionGitUser(prov string) string {
	if p, ok := m.opt.Connections.Get(prov); ok {
		if gu, ok := p.(connections.GitUser); ok {
			return gu.GitUsername()
		}
		return "x-access-token"
	}
	if prov == "" || prov == store.ProviderGitHub {
		return connections.GitUsernameForType("github")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if row, err := store.GetConnProviderRow(ctx, m.opt.Store.Q(), prov); err == nil {
		return connections.GitUsernameForType(row.Type)
	}
	return ""
}

func statusOf(r *store.Remote) space.RemoteStatus {
	return space.RemoteStatus{State: r.State, Kind: r.ErrorKind, Error: r.LastError, LastGood: r.LastGood, RemoteTip: r.RemoteTip, LastSync: r.LastSync}
}

// remoteOptions is what a space is opened (or connected) with.
func (m *Manager) remoteOptions(id string, r *store.Remote) *space.RemoteOptions {
	return &space.RemoteOptions{
		URL: r.URL, Branch: r.Branch, Cred: m.credential(id, r), Status: statusOf(r),
		OnStatus:   m.persistStatus(id),
		NetTimeout: m.opt.NetTimeout, FetchFresh: m.opt.FetchFresh, FailFast: m.opt.FailFast,
		Runner: m.opt.RemoteRunner, AllowUnsafe: m.opt.RemotePolicy.AllowUnsafe,
		Partial: m.single == nil && m.Storage().Cache.Partial,
	}
}

// persistStatus is the callback that keeps the database and the list of
// spaces' sync states in line with a space's remote.
func (m *Manager) persistStatus(id string) func(space.RemoteStatus) {
	return func(st space.RemoteStatus) {
		prev := ""
		if m.single == nil {
			m.mu.Lock()
			known := m.syncs != nil
			prev = m.syncs[id]
			m.mu.Unlock()
			if !known {
				// The states are read once before the first change is noted, so that
				// the first change of a run is told from the state it was in.
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				prev = m.syncStates(ctx)[id]
				cancel()
			}
		}
		m.setSync(id, st.State)
		defer m.syncNotice(id, prev, st)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := store.UpdateRemoteStatus(ctx, m.opt.Store.Q(), id, store.RoleContent, store.RemoteStatus{
			State: st.State, ErrorKind: st.Kind, LastError: st.Error, LastGood: st.LastGood, RemoteTip: st.RemoteTip, LastSync: st.LastSync,
		}, m.opt.Now())
		if err != nil {
			m.log.Warn("could not store the state of a remote", "space", id, "error", err)
		}
	}
}

// --- sync states ----------------------------------------------------------------

// syncState is what Info.Sync says for a space: "local" or the remote's state.
func (m *Manager) syncStates(ctx context.Context) map[string]string {
	m.mu.Lock()
	if m.syncs != nil {
		out := m.syncs
		m.mu.Unlock()
		return out
	}
	m.mu.Unlock()
	states, err := store.RemoteStates(ctx, m.opt.Store.Q())
	if err != nil {
		m.log.Warn("could not read the states of the remotes", "error", err)
		return map[string]string{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.syncs == nil {
		m.syncs = states
	}
	return m.syncs
}

func (m *Manager) setSync(id, state string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.syncs == nil {
		return // loaded from the database on first use
	}
	if state == "" {
		delete(m.syncs, id)
	} else {
		m.syncs[id] = state
	}
}

// withSync fills in Info.Sync. The list of spaces is cached; the sync state
// changes by itself, so it is read at the end, not cached with it.
func (m *Manager) withSync(ctx context.Context, in []Info) []Info {
	if m.single != nil {
		return in
	}
	states := m.syncStates(ctx)
	m.loadDisabled(ctx) // on failure nothing is shown as disabled here; Open still refuses
	m.mu.Lock()
	out := make([]Info, len(in))
	for i, x := range in {
		x.Sync = "local"
		if st, ok := states[x.ID]; ok {
			x.Sync = st
		}
		x.Disabled = m.disabled[x.ID]
		out[i] = x
	}
	m.mu.Unlock()
	for i := range out {
		out[i].Cache = m.cacheOf(out[i].ID, out[i].Sync != "local")
		out[i].Assets = m.AssetsMode(ctx, out[i].ID)
		if n, err := m.Notice(ctx, out[i].ID); err == nil {
			out[i].RemoteNotice = n
		}
	}
	return out
}

// --- the owner's operations ------------------------------------------------------

func (m *Manager) viewOf(h *Handle, r *store.Remote) *RemoteView {
	v := &RemoteView{URL: r.URL, Branch: r.Branch, CredType: r.CredType, Username: r.Username, State: r.State, LastSync: r.LastSync, Error: r.LastError}
	if r.State == store.RemoteError {
		v.ErrorCode = r.ErrorKind
	}
	if st, ok := h.Space.Remote(); ok {
		v.State, v.LastSync, v.Error = st.State, st.LastSync, st.Error
		v.ErrorCode = ""
		if st.State == space.RemoteError {
			v.ErrorCode = st.Kind
		}
		if st.State == space.RemoteRewritten {
			v.Rewritten = &Rewritten{LastGood: st.LastGood, Remote: st.RemoteTip}
		}
	}
	v.Provider = r.Provider
	if v.Provider == "" {
		v.Provider = store.ProviderGit
	}
	if r.CredType == CredConnection {
		v.Connection = m.connectionView(r)
	}
	if r.CredType == CredSSH {
		v.PublicKey = r.PubKey
		v.Fingerprint, _ = gitstore.Fingerprint(r.PubKey)
		v.HostKey = gitstore.HostKeyFingerprint(m.knownHosts(r.SpaceID))
	}
	return v
}

// connectionView says what a connection credential is: no secret, only where it points.
func (m *Manager) connectionView(r *store.Remote) *ConnectionView {
	g := &ConnectionView{Via: r.Via, InstallationID: r.InstallationID, RepoID: r.RepoID, UserID: r.CredUserID}
	if u, err := url.Parse(r.URL); err == nil {
		// The account is the namespace: a person's, an organisation's, or a group
		// with its subgroups.
		full := strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), ".git")
		g.Account = full[:max(strings.LastIndex(full, "/"), 0)]
	}
	if r.Via == connections.ViaUser && r.CredUserID != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// The person is looked up in the space's tenant only: a credential of
		// someone elsewhere (which connecting refuses) shows no name.
		if sp, err := store.GetSpace(ctx, m.opt.Store.Q(), store.AllTenants(), r.SpaceID); err == nil { // the space being viewed, by id
			if u, err := store.GetUser(ctx, m.opt.Store.Q(), store.InTenant(sp.TenantID), r.CredUserID); err == nil {
				g.Username = u.Username
				if r.Provider == store.ProviderGitHub || r.Provider == "" {
					if a, err := store.GetRepoLink(ctx, m.opt.Store.Q(), store.ProviderGitHub, r.CredUserID); err == nil {
						g.Login = a.Login
					}
				} else if p, ok := m.opt.Connections.Get(r.Provider); ok {
					if c, ok := p.(connections.Connected); ok {
						g.Login = c.Login(ctx, r.CredUserID)
					}
				}
			}
		}
	}
	return g
}

// Remote returns the remote of the space, or ErrNoRemote when it is local.
func (m *Manager) Remote(ctx context.Context, h *Handle) (*RemoteView, error) {
	r, err := store.GetRemote(ctx, m.opt.Store.Q(), h.Info.ID, store.RoleContent)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !r.Connected()) {
		return nil, ErrNoRemote
	}
	if err != nil {
		return nil, err
	}
	return m.viewOf(h, r), nil
}

// SSHKey makes (or returns) the SSH key pair of the space. rotate replaces it,
// only while the space is local.
func (m *Manager) SSHKey(ctx context.Context, h *Handle, rotate bool) (publicKey, fingerprint string, err error) {
	if m.single != nil {
		return "", "", ErrNotFound
	}
	box, err := m.box()
	if err != nil {
		return "", "", err
	}
	id := h.Info.ID
	m.remoteMu.Lock()
	defer m.remoteMu.Unlock()
	r, err := store.GetRemote(ctx, m.opt.Store.Q(), id, store.RoleContent)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return "", "", err
	}
	if err == nil && r.PubKey != "" && !rotate {
		fp, _ := gitstore.Fingerprint(r.PubKey)
		return r.PubKey, fp, nil
	}
	if err == nil && r.Connected() {
		return "", "", ErrConnected
	}
	k, err := gitstore.GenerateSSHKey("petalbook-" + id)
	if err != nil {
		return "", "", err
	}
	enc, err := box.Seal(id, "ssh-key", k.Private)
	if err != nil {
		return "", "", err
	}
	row := store.Remote{SpaceID: id, Branch: "main", CredType: CredSSH, PrivKeyEnc: enc, PubKey: k.Public, State: store.RemoteLocal, Updated: m.opt.Now()}
	if err := store.PutRemote(ctx, m.opt.Store.Q(), row); err != nil {
		return "", "", err
	}
	return k.Public, k.Fingerprint, nil
}

// sameTenant refuses a via "user" credential of someone outside the space's
// tenant: a person's GitHub connection syncs only spaces of their own tenant.
// It can only come up for a site administrator, who may open a space of any
// tenant; everyone else reaches only spaces of their own. Two lookups by key.
func (m *Manager) sameTenant(ctx context.Context, provider, tenantID, userID string) error {
	_, err := store.GetUser(ctx, m.opt.Store.Q(), store.InTenant(tenantID), userID)
	if errors.Is(err, store.ErrNotFound) {
		return &connections.Error{Provider: provider, Code: connections.CodeForbidden, Status: http.StatusForbidden,
			Msg: "A connection syncs only spaces of its own tenant: use the App (via app), or let someone of the space's tenant connect it."}
	}
	return err
}

// spaceTenant is the tenant a space belongs to.
func (m *Manager) spaceTenant(ctx context.Context, id string) (string, error) {
	sp, err := store.GetSpace(ctx, m.opt.Store.Q(), store.AllTenants(), id) // the space the caller has open, by id
	if err != nil {
		return "", err
	}
	return sp.TenantID, nil
}

// providerOf is the provider a connect request goes through: its own for a
// connection, "git" for a plain address.
func providerOf(req ConnectRequest) string {
	if req.CredType == CredConnection {
		return req.Provider
	}
	return store.ProviderGit
}

// Connect makes the space follow a remote repository. It does not return
// until the space and the remote agree (see space.Connect).
func (m *Manager) Connect(ctx context.Context, h *Handle, req ConnectRequest) (*RemoteView, error) {
	if m.single != nil {
		return nil, ErrNotFound
	}
	box, err := m.box()
	if err != nil {
		return nil, err
	}
	id := h.Info.ID
	if err := m.CheckCredential(ctx, id, req.CredType, providerOf(req)); err != nil {
		return nil, err
	}
	var remote gitstore.Remote
	var ghCred *connections.Cred
	if req.CredType == CredConnection {
		sp, err := store.GetSpace(ctx, m.opt.Store.Q(), store.AllTenants(), id) // the space the caller has open, by id
		if err != nil {
			return nil, err
		}
		if err := m.CheckPartial(ctx, sp.TenantID, req.Provider); err != nil {
			return nil, err
		}
		if req.Via == connections.ViaUser {
			if err := m.sameTenant(ctx, req.Provider, sp.TenantID, req.UserID); err != nil {
				return nil, err
			}
		}
		if remote, ghCred, err = m.resolveConnection(ctx, connections.ScopeOfSpace(sp.TenantID, sp.Personal), req); err != nil {
			return nil, err
		}
	} else {
		if remote, err = gitstore.ParseURL(req.URL, m.hostPolicy()); err != nil {
			return nil, err
		}
		tenant, err := m.spaceTenant(ctx, id)
		if err != nil {
			return nil, err
		}
		if err := m.CheckPartialHost(ctx, tenant, remote.Host); err != nil {
			return nil, err
		}
	}
	branch := strings.TrimSpace(req.Branch)
	if branch == "" {
		branch = "main"
	}
	if err := gitstore.ValidBranch(branch); err != nil {
		return nil, fmt.Errorf("%w: %s is not a usable branch name", ErrInvalid, branch)
	}
	m.remoteMu.Lock()
	defer m.remoteMu.Unlock()
	old, err := store.GetRemote(ctx, m.opt.Store.Q(), id, store.RoleContent)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	restoreHosts, dropOldHosts := func() {}, func() {}
	row := store.Remote{SpaceID: id, URL: remote.URL, Branch: branch, State: store.RemoteOK}
	if old != nil {
		row.PrivKeyEnc, row.PubKey, row.TokenEnc, row.Username = old.PrivKeyEnc, old.PubKey, old.TokenEnc, old.Username
	}
	switch req.CredType {
	case CredToken:
		if remote.Scheme != "https" && remote.Scheme != "http" && remote.Scheme != "file" { // http and file only exist in tests
			return nil, fmt.Errorf("%w: a token goes with an https address; use an ssh key for %s", ErrInvalid, remote.URL)
		}
		row.CredType = CredToken
		if u := strings.TrimSpace(req.Username); u != "" {
			row.Username = u
		} else if row.Username == "" {
			row.Username = "x-access-token"
		}
		if strings.ContainsAny(row.Username, " \t\r\n:\x00") {
			return nil, fmt.Errorf("%w: the user name is not usable", ErrInvalid)
		}
		if req.Token != "" {
			if strings.ContainsAny(req.Token, "\r\n\x00") || len(req.Token) > 4096 {
				return nil, fmt.Errorf("%w: the token is not usable", ErrInvalid)
			}
			if row.TokenEnc, err = box.Seal(id, "token", []byte(req.Token)); err != nil {
				return nil, err
			}
		} else if old == nil || old.TokenEnc == "" {
			return nil, fmt.Errorf("%w: a token is required", ErrInvalid)
		} else if prev, perr := gitstore.ParseURL(old.URL, m.hostPolicy()); perr != nil || prev.Scheme != remote.Scheme || prev.Host != remote.Host || gitstore.CredentialScope(prev.URL) != gitstore.CredentialScope(remote.URL) {
			// A stored token was given for one host and port. It is not sent to
			// another because the address was edited.
			return nil, fmt.Errorf("%w: the address is on another host now; enter the token for it", ErrInvalid)
		}
	case CredSSH:
		if remote.Scheme != "ssh" {
			return nil, fmt.Errorf("%w: an ssh key goes with an ssh address (git@host:path); use a token for %s", ErrInvalid, remote.URL)
		}
		row.CredType = CredSSH
		if row.PrivKeyEnc == "" {
			k, err := gitstore.GenerateSSHKey("petalbook-" + id)
			if err != nil {
				return nil, err
			}
			if row.PrivKeyEnc, err = box.Seal(id, "ssh-key", k.Private); err != nil {
				return nil, err
			}
			row.PubKey = k.Public
			// Kept even if connecting fails, so the public key can be added to the
			// repository. For a space that is connected already (it is changing
			// from a token to a key) only the key is added to its row: the row
			// keeps saying what the space follows until the new connection works.
			keyOnly := row
			keyOnly.URL, keyOnly.State, keyOnly.Updated = "", store.RemoteLocal, m.opt.Now()
			if old != nil && old.Connected() {
				keyOnly = *old
				keyOnly.PrivKeyEnc, keyOnly.PubKey = row.PrivKeyEnc, row.PubKey
			}
			if err := store.PutRemote(ctx, m.opt.Store.Q(), keyOnly); err != nil {
				return nil, err
			}
		}
		// Host keys are trusted per connection: a changed address starts afresh.
		// The old file is set aside, not deleted, until the new connection works:
		// if it fails the space keeps following the old remote with the host key
		// it trusted.
		if old != nil && old.URL != row.URL {
			kh := m.knownHosts(id)
			if err := os.Rename(kh, kh+".old"); err == nil {
				restoreHosts = func() { os.Rename(kh+".old", kh) }
				dropOldHosts = func() { os.Remove(kh + ".old") }
			}
		}
	case CredConnection:
		// The credential holds no secret of its own: whatever the space had
		// before (a deploy key, a token) goes, once the new connection works.
		row.CredType, row.Username, row.Provider = CredConnection, "", req.Provider
		row.PrivKeyEnc, row.PubKey, row.TokenEnc = "", "", ""
		row.Via, row.InstallationID, row.RepoID, row.CredUserID = ghCred.Via, ghCred.InstallationID, ghCred.RepoID, ghCred.UserID
		if old != nil && old.CredType == CredSSH {
			dropOldHosts = func() { os.Remove(m.knownHosts(id)) }
		}
	default:
		return nil, fmt.Errorf("%w: credential.type must be ssh, token or connection", ErrInvalid)
	}
	row.Updated = m.opt.Now()
	if err := h.Space.Connect(ctx, *m.remoteOptions(id, &row)); err != nil {
		restoreHosts()
		return nil, err
	}
	dropOldHosts()
	st, _ := h.Space.Remote()
	row.State, row.LastGood, row.LastSync = st.State, st.LastGood, st.LastSync
	if err := store.PutRemote(ctx, m.opt.Store.Q(), row); err != nil {
		if derr := h.Space.Disconnect(); derr != nil {
			m.log.Error("could not disconnect a space whose remote could not be stored", "space", id, "error", derr)
		}
		return nil, err
	}
	m.setSync(id, row.State)
	m.dropFacts(id) // it follows a remote now: the requirement may be met
	m.markCache(id, h.Space)
	m.schedulePull(id)
	return m.viewOf(h, &row), nil
}

// schedulePull makes an open space due for its first pull one interval from
// now (it has just been synced by connecting).
func (m *Manager) schedulePull(id string) {
	m.mu.Lock()
	if e := m.open[id]; e != nil && m.opt.PullInterval > 0 {
		e.nextPull = m.opt.Now().Add(jittered(m.opt.PullInterval))
	}
	m.mu.Unlock()
}

// Disconnect makes the space local again. Nothing is changed on the remote;
// the credential is deleted.
func (m *Manager) Disconnect(ctx context.Context, h *Handle) error {
	if m.single != nil {
		return ErrNotFound
	}
	id := h.Info.ID
	if err := m.CheckRemoval(ctx, id); err != nil {
		return err
	}
	m.remoteMu.Lock()
	defer m.remoteMu.Unlock()
	// Attachments in their own repository are read through the space's remote;
	// without one the space could not serve them.
	if row, err := store.GetRemote(ctx, m.opt.Store.Q(), id, store.RoleAssets); err == nil && row.URL != "" {
		return assetsConflict("The space has an attachment repository; disconnect that first.")
	}
	// A local space is the complete copy: a partial clone is made whole first
	// (it waits for the fetch like any restore, and fails with the space still
	// following its remote).
	if err := h.Space.MakeFull(ctx); err != nil {
		return err
	}
	if err := h.Space.Disconnect(); err != nil {
		return err
	}
	if err := store.DeleteRemote(ctx, m.opt.Store.Q(), id, store.RoleContent); err != nil {
		return err
	}
	os.Remove(m.knownHosts(id))
	m.setSync(id, "")
	m.dropFacts(id)
	return nil
}

// Sync fetches the remote now.
func (m *Manager) Sync(ctx context.Context, h *Handle) (*RemoteView, error) {
	r, err := m.stored(ctx, h)
	if err != nil {
		return nil, err
	}
	h.Space.ClearNotice()
	// An unreachable remote is not an error of this call: the view says so.
	var ue *space.UnavailableError
	if err := h.Space.Pull(ctx, true); err != nil && !errors.As(err, &ue) {
		return nil, err
	}
	return m.viewOf(h, r), nil
}

// Accept accepts a rewritten remote history.
func (m *Manager) Accept(ctx context.Context, h *Handle, commit string) (*RemoteView, error) {
	r, err := m.stored(ctx, h)
	if err != nil {
		return nil, err
	}
	if err := h.Space.Accept(ctx, commit); err != nil {
		return nil, err
	}
	return m.viewOf(h, r), nil
}

func (m *Manager) stored(ctx context.Context, h *Handle) (*store.Remote, error) {
	r, err := store.GetRemote(ctx, m.opt.Store.Q(), h.Info.ID, store.RoleContent)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !r.Connected()) {
		return nil, ErrNoRemote
	}
	return r, err
}

// --- the puller -------------------------------------------------------------------

// RunPull pulls the open spaces that follow a remote, every PullInterval each
// (about: it is jittered), until ctx ends. It does nothing when the interval is
// not positive.
func (m *Manager) RunPull(ctx context.Context) {
	if m.single != nil || m.opt.PullInterval <= 0 {
		return
	}
	tick := m.opt.PullInterval / 4
	if tick > 5*time.Second {
		tick = 5 * time.Second
	}
	if tick < 20*time.Millisecond {
		tick = 20 * time.Millisecond
	}
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.pullWake:
		case <-t.C:
		}
		m.pullDue(ctx)
	}
}

func (m *Manager) wakePull() {
	select {
	case m.pullWake <- struct{}{}:
	default:
	}
}

// jittered is d plus or minus 20 percent.
func jittered(d time.Duration) time.Duration {
	return d - d/5 + time.Duration(rand.Int64N(int64(d)*2/5+1))
}

// pullDue pulls the spaces whose turn has come. The spaces are pinned while
// it works, so none is closed under it, but their last-use time is left alone.
func (m *Manager) pullDue(ctx context.Context) {
	now := m.opt.Now()
	var due []*opening
	ids := map[*opening]string{}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	for id, e := range m.open {
		if e.closing != nil || e.nextPull.IsZero() || now.Before(e.nextPull) {
			continue
		}
		select {
		case <-e.done: // still being opened otherwise: e.sp is not settled
		default:
			continue
		}
		if e.sp == nil {
			continue
		}
		if _, ok := e.sp.Remote(); !ok {
			e.nextPull = time.Time{}
			continue
		}
		e.nextPull = now.Add(jittered(m.opt.PullInterval))
		e.refs++
		due = append(due, e)
		ids[e] = id
	}
	m.mu.Unlock()
	if len(due) == 0 {
		return
	}
	sem := make(chan struct{}, pullWorkers)
	var wg sync.WaitGroup
	for _, e := range due {
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() {
				<-sem
				m.mu.Lock()
				e.refs--
				m.mu.Unlock()
				wg.Done()
			}()
			if ctx.Err() != nil {
				return
			}
			var ue *space.UnavailableError
			if err := e.sp.Pull(ctx, false); err != nil && !errors.As(err, &ue) && ctx.Err() == nil && !errors.Is(err, space.ErrClosed) {
				m.log.Warn("pulling a remote failed", "error", err)
			}
			if e.sp.Assets() != nil {
				m.refreshAssets(ids[e], e.sp)
			}
			// A space whose connection does not work is looked at again
			// only now and then; fixing the cause wakes it (ConnectionFixed).
			if st, ok := e.sp.Remote(); ok && st.State == space.RemoteError && space.IsConnectionKind(st.Kind) {
				m.mu.Lock()
				e.nextPull = m.opt.Now().Add(jittered(m.opt.ConnectionRetry))
				m.mu.Unlock()
			}
		}()
	}
	wg.Wait()
}

// PullNow pulls one open space immediately (tests, and "sync now").
func (m *Manager) PullNow(ctx context.Context, id string) error {
	sp, release, err := m.Acquire(ctx, id)
	if err != nil {
		return err
	}
	defer release()
	return sp.Pull(ctx, true)
}
