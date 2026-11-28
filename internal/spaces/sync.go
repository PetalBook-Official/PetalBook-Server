package spaces

// Spaces that follow a repository on a provider (GitHub's App, ...): connecting
// an existing space, and making a space together with a new repository.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"petalbook.app/server/internal/connections"
	"petalbook.app/server/internal/gitstore"
	"petalbook.app/server/internal/perm"
	"petalbook.app/server/internal/space"
	"petalbook.app/server/internal/store"
)

// provider is the provider with the id, or the "not configured" answer.
func (m *Manager) provider(id string) (connections.Provider, error) {
	p, ok := m.opt.Connections.Get(id)
	if ok && id != connections.GitID {
		return p, nil
	}
	name := id
	if ok {
		name = p.Descriptor().Name
	}
	return nil, connections.Errorf(id, http.StatusConflict, connections.CodeNotConfigured, "This server has no %s connection set up.", name)
}

// hostAllowed holds a provider's host to the allowed hosts (--remote-host, or
// the storage setting hosts), as every other remote is: a connection is no way
// around the administrator's list. Only the host is checked here (the address
// itself is the provider's own).
func (m *Manager) hostAllowed(p connections.Provider) error {
	err := m.HostAllowed(p.Descriptor().Host)
	var he *gitstore.HostError
	if errors.As(err, &he) {
		return he
	}
	if err != nil {
		return invalidAt(p.Descriptor().ID, "the address of this provider is not usable")
	}
	return nil
}

// HostAllowed holds a host (with its port, if it has one) to the allowed hosts:
// a *gitstore.HostError when the administrator has not allowed it. A provider
// an administrator sets up (GitLab, Gitea) is held to it the same way.
func (m *Manager) HostAllowed(host string) error {
	_, err := gitstore.ParseURL("https://"+host+"/owner/name.git", gitstore.Policy{Hosts: m.hostPolicy().Hosts})
	return err
}

// resolveConnection reads the address of a space's repository and decides how
// it syncs: the address is stored in https form on the provider's host.
func (m *Manager) resolveConnection(ctx context.Context, sc connections.SpaceScope, req ConnectRequest) (gitstore.Remote, *connections.Cred, error) {
	if req.Provider == "" {
		return gitstore.Remote{}, nil, invalidAt("", "credential.provider is required for a connection")
	}
	p, err := m.provider(req.Provider)
	if err != nil {
		return gitstore.Remote{}, nil, err
	}
	d := p.Descriptor()
	owner, name, ok := p.ParseRepoURL(req.URL)
	if !ok {
		return gitstore.Remote{}, nil, invalidAt(d.ID, "a connection goes with a repository on %s (https://%s/owner/name.git or git@%s:owner/name.git)", d.Name, d.Host, d.Host)
	}
	if req.Via != connections.ViaApp && req.Via != connections.ViaUser {
		return gitstore.Remote{}, nil, invalidAt(d.ID, "credential.via must be app or user")
	}
	if err := m.hostAllowed(p); err != nil {
		return gitstore.Remote{}, nil, err
	}
	cred, repo, err := resolveExisting(ctx, p, sc, req.UserID, owner, name, req.Via)
	if err != nil {
		return gitstore.Remote{}, nil, err
	}
	u := p.RepoURL(repo.Owner, repo.Name)
	scheme := "https"
	if strings.HasPrefix(u, "http://") {
		scheme = "http"
	}
	return gitstore.Remote{URL: u, Scheme: scheme, Host: strings.ToLower(d.Host)}, cred, nil
}

// SyncCreate is a new space's repository on a provider.
type SyncCreate struct {
	Provider    string
	Owner, Name string
	Private     bool
	Description string
	// Create makes the repository; false connects the space to the one that is
	// there already.
	Create bool
	// Assets asks for an attachment repository made (or found) beside the
	// space's, with the same owner, visibility and credential; nil asks for none.
	Assets *SyncAssets
	// Mode is the storage mode asked for; "" for none.
	Mode string
}

// SyncAssets is the attachment repository of a new space on a provider. Default
// is for a request that did not say: the administrator's assets.newSpaces
// decides. Name is "" for <name>-assets.
type SyncAssets struct {
	Name    string
	Default bool
}

// CreateWithSync makes a space together with its repository on a provider, in
// the order of docs/api.md ("Creating a space with a repository"): nothing is
// visible, and nothing stays on the server, unless every step works. An empty
// repository left on the platform by a failure after it was made is named in
// the error, so the same request with Create false can use it.
func (m *Manager) CreateWithSync(ctx context.Context, userID, name, inbox string, g SyncCreate) (Info, error) {
	if m.single != nil {
		return Info{}, ErrNotFound
	}
	gh, err := m.provider(g.Provider)
	if err != nil {
		return Info{}, err
	}
	if inbox == "" {
		inbox = store.DefaultInbox
	}
	if inbox, err = ValidInbox(inbox); err != nil {
		return Info{}, err
	}
	if name, err = ValidName(name); err != nil {
		return Info{}, err
	}
	if _, err := m.resolveFor(ctx, userID, g.Mode, true); err != nil {
		return Info{}, err
	}
	if err := m.CheckNewSpace(ctx, userID, CredConnection, g.Provider); err != nil {
		return Info{}, err
	}
	if err := m.hostAllowed(gh); err != nil {
		return Info{}, err
	}
	if !gh.ValidRepoName(g.Name) {
		return Info{}, invalidAt(gh.Descriptor().ID, "storage.sync.name is 1 to 100 characters of letters, digits, '.', '_' and '-', not '.' or '..' and not ending in .git")
	}
	if utf8.RuneCountInString(g.Description) > 350 || !utf8.ValidString(g.Description) ||
		strings.IndexFunc(g.Description, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return Info{}, invalidAt(gh.Descriptor().ID, "storage.sync.description is at most 350 characters, without control characters")
	}
	// The name of the attachment repository, when the request could have one. A
	// name that cannot be a repository's is an error for a request that gave it
	// and only a reason to keep the attachments in the space for one that did not.
	assetsName := ""
	if a := g.Assets; a != nil {
		assetsName = a.Name
		if assetsName == "" {
			assetsName = g.Name + "-assets"
		}
		if !gh.ValidRepoName(assetsName) || strings.EqualFold(assetsName, g.Name) {
			if !a.Default {
				return Info{}, invalidAt(gh.Descriptor().ID, "storage.sync.assets names a repository other than the space's, in the form of storage.sync.name")
			}
			assetsName = ""
		}
	}
	// The limit counts the spaces being made as well as the ones that exist.
	// The space is the creator's tenant's, as Create's is.
	m.createMu.Lock()
	u, err := store.GetUser(ctx, m.opt.Store.Q(), store.AllTenants(), userID) // the creator, by id
	if err != nil {
		m.createMu.Unlock()
		return Info{}, err
	}
	if err := m.checkSpaceQuota(ctx, *u); err != nil {
		m.createMu.Unlock()
		return Info{}, err
	}
	tenantID := u.TenantID
	// Whose space it is: the creator's own in the default tenant, else their organisation's.
	sc := connections.ScopeOfSpace(tenantID, false)
	m.reserved[userID]++
	m.reservedT[tenantID]++
	m.createMu.Unlock()
	held := true
	unreserve := func() { // under createMu
		m.reserved[userID]--
		if m.reserved[userID] == 0 {
			delete(m.reserved, userID)
		}
		m.reservedT[tenantID]--
		if m.reservedT[tenantID] == 0 {
			delete(m.reservedT, tenantID)
		}
		held = false
	}
	release := func() { // gives the place back: under createMu, once
		if held {
			m.createMu.Lock()
			unreserve()
			m.createMu.Unlock()
		}
	}
	defer release()

	// The work goes on if the client goes away.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Minute)
	defer cancel()

	// Whether the space gets an attachment repository: the request if it said,
	// else the settings' default for the tenant, inside what the settings leave
	// the owner. One more settings lookup (a map read) per creation.
	pol, err := m.assetsPolicyOf(ctx, tenantID)
	if err != nil {
		return Info{}, err
	}
	if err := m.CheckPartial(ctx, tenantID, g.Provider); err != nil {
		return Info{}, err
	}
	assetsOptional := false // a repository the request did not ask for: the space does without one that cannot be had
	switch a := g.Assets; {
	case a == nil:
		if err := pol.check("space"); err != nil {
			return Info{}, err
		}
	case a.Default:
		// A lock to the repository is not a default the space can do without.
		assetsOptional = !pol.Locked
		if pol.Default != "repository" {
			assetsName = ""
		}
	default:
		if err := pol.check("repository"); err != nil {
			return Info{}, err
		}
	}

	var plan *connections.Plan
	var cred *connections.Cred
	var repo *connections.Repo
	if g.Create {
		if plan, err = planCreate(ctx, gh, sc, userID, g.Owner); err != nil {
			return Info{}, err
		}
	} else {
		if cred, repo, err = resolveExisting(ctx, gh, sc, userID, g.Owner, g.Name, ""); err != nil {
			return Info{}, err
		}
	}

	stg, err := m.Stage(name, IdentityOf(*u))
	if err != nil {
		return Info{}, err
	}
	committed := false
	defer func() {
		if !committed {
			stg.Abort()
		}
	}()

	created := false
	if g.Create {
		if repo, err = gh.CreateRepo(ctx, plan, connections.NewRepoSpec{Name: g.Name, Private: g.Private, Description: g.Description}); err != nil {
			return Info{}, err
		}
		created = true
		m.hook("repo-created")
	}
	// From here on the repository exists: a failure leaves it empty on GitHub
	// and says so (the attachment repository too, when one was made).
	var arepo *connections.Repo
	var acred *connections.Cred
	acreated := false
	failed := func(code string, status int, err error) error {
		if !created && !acreated {
			return err
		}
		// A reminder the person sees later, so the repository is not forgotten
		// (each repository that was made: the space's and its attachments').
		for _, r := range []*connections.Repo{repo, arepo} {
			if r == nil || (r == repo && !created) || (r == arepo && !acreated) {
				continue
			}
			if rerr := gh.RecordLeftover(ctx, userID, r, name); rerr != nil {
				m.log.Warn("could not remember a repository left empty by a failed space", "repo", r.FullName, "error", rerr)
			}
		}
		// The provider's own refusal (with its code and its wait) is passed on
		// as it is; anything else is the step's failure.
		var ge *connections.Error
		var he *connections.Error
		if errors.As(err, &ge) {
			c := *ge
			he = &c
		} else {
			he = &connections.Error{Provider: g.Provider, Code: code, Status: status, Msg: err.Error(), Err: err}
		}
		if created && he.Repo == nil {
			he.Repo = repo.Ref()
		}
		if acreated && he.AssetsRepo == nil {
			he.AssetsRepo = arepo.Ref()
		}
		return he
	}
	// Both repositories are made before either is checked for the credential, so
	// that a failure of the check names both and the retry can use them.
	if assetsName != "" && g.Create {
		if arepo, err = gh.CreateRepo(ctx, plan, connections.NewRepoSpec{Name: assetsName, Private: g.Private, Description: "Attachments of " + g.Name}); err != nil {
			if !assetsOptional || !withoutSecondRepo(err) {
				return Info{}, failed(connections.CodePushFailed, http.StatusBadGateway, err)
			}
			arepo, assetsName = nil, "" // the default does not fail a space over its second repository
		} else {
			acreated = true
		}
	}
	if created {
		if cred, err = gh.ChooseCredential(ctx, plan, repo); err != nil {
			return Info{}, failed(connections.CodePushFailed, http.StatusBadGateway, err)
		}
	}
	if assetsName != "" {
		// The credential of the attachment repository is chosen the same way (an
		// installation with selected repositories gets it added).
		if g.Create {
			if acred, err = gh.ChooseCredential(ctx, plan, arepo); err != nil {
				if !assetsOptional || !withoutSecondRepo(err) {
					return Info{}, failed(connections.CodePushFailed, http.StatusBadGateway, err)
				}
				// Made but unusable: remembered for the person, and the space goes on without it.
				if rerr := gh.RecordLeftover(ctx, userID, arepo, name); rerr != nil {
					m.log.Warn("could not remember an attachment repository left empty", "repo", arepo.FullName, "error", rerr)
				}
				arepo, acred, acreated, assetsName = nil, nil, false, ""
			}
		} else if acred, arepo, err = resolveExisting(ctx, gh, sc, userID, g.Owner, assetsName, ""); err != nil && assetsOptional && withoutSecondRepo(err) {
			arepo, acred, assetsName = nil, nil, "" // there is none beside the space's repository: the attachments stay in the space
		} else if err != nil {
			// The space's repository is there; say which one is not (a retry
			// after the attachment repository could not be made).
			var ge *connections.Error
			if !errors.As(err, &ge) {
				return Info{}, err
			}
			c := *ge
			c.Msg = fmt.Sprintf("the attachment repository %s/%s cannot be used (%s); the space's repository %s can", g.Owner, assetsName, ge.Msg, repo.FullName)
			c.Repo = repo.Ref()
			return Info{}, &c
		}
	}
	id := stg.ID()
	m.mu.Lock()
	m.staging[id] = true
	m.mu.Unlock()
	endStaging := func() { m.mu.Lock(); delete(m.staging, id); m.mu.Unlock() }
	defer endStaging()
	row := store.Remote{SpaceID: id, URL: gh.RepoURL(repo.Owner, repo.Name), Branch: "main", CredType: CredConnection, Provider: g.Provider, State: store.RemoteOK,
		Via: cred.Via, InstallationID: cred.InstallationID, RepoID: cred.RepoID, CredUserID: cred.UserID, Updated: m.opt.Now()}
	if err := m.connectRetry(ctx, stg.sp, *m.remoteOptions(id, &row)); err != nil {
		return Info{}, failed(connections.CodePushFailed, http.StatusBadGateway, err)
	}
	m.hook("connected")
	var arow *store.Remote
	if arepo != nil {
		// The attachment repository is looked at with the credential it will keep:
		// the first fetch proves it works, before the space is recorded.
		arow = &store.Remote{SpaceID: id, Role: store.RoleAssets, URL: gh.RepoURL(arepo.Owner, arepo.Name), Branch: "main", CredType: CredConnection, Provider: g.Provider, State: store.RemoteOK,
			Via: acred.Via, InstallationID: acred.InstallationID, RepoID: acred.RepoID, CredUserID: acred.UserID, Updated: m.opt.Now()}
		if err := m.probeAssetsRepo(ctx, id, arow); err != nil {
			os.RemoveAll(m.assetsDir(id))
			return Info{}, failed(connections.CodePushFailed, http.StatusBadGateway, err)
		}
	}
	st, _ := stg.sp.Remote()
	row.State, row.LastGood, row.LastSync = st.State, st.LastGood, st.LastSync
	now := m.opt.Now()
	m.createMu.Lock()
	unreserve() // the row below takes the place
	err = m.opt.Store.Tx(ctx, func(q store.Q) error {
		// The provider's row first, before InsertSpace and AddMember write
		// usage: its deletion locks the row and then usage (lockUnusedProvider,
		// then settling who is connected), and one order means neither waits
		// on the other in a circle. A provider deleted meanwhile takes no space.
		if err := store.LockProvider(ctx, q, g.Provider); err != nil {
			return err
		}
		if err := store.InsertSpace(ctx, q, tenantID, id, stg.Name(), "", inbox, now); err != nil {
			return err
		}
		if err := store.AddMember(ctx, q, id, userID, perm.Owner, now); err != nil {
			return err
		}
		if arow != nil {
			if err := store.PutRemote(ctx, q, *arow); err != nil {
				return err
			}
		}
		return store.PutRemote(ctx, q, row)
	})
	if errors.Is(err, store.ErrProviderGone) {
		os.RemoveAll(m.assetsDir(id))
		m.createMu.Unlock()
		return Info{}, failed(connections.CodeNotConfigured, http.StatusConflict,
			connections.Errorf(g.Provider, http.StatusConflict, connections.CodeNotConfigured, "The provider was removed while the space was being made; nothing was kept."))
	}
	if err != nil {
		os.RemoveAll(m.assetsDir(id))
		m.createMu.Unlock()
		m.log.Error("could not record a space made with a repository on a provider", "error", err)
		return Info{}, failed(connections.CodeInternal, http.StatusInternalServerError, errors.New("the space could not be recorded; nothing was kept"))
	}
	m.createMu.Unlock()
	endStaging() // the space has its member now: the checks apply from here on
	committed = true
	stg.Commit(userID)
	m.usageAdded(ctx, store.Touched{UserID: userID, TenantID: tenantID})
	m.setSync(id, row.State)
	m.markCache(id, stg.sp)
	if arow != nil {
		m.noteConnected(id, true)
		if r, _, err := m.assetRepo(ctx, id); err == nil && r != nil {
			stg.sp.SetAssets(backend{m, id, r})
		}
	}
	m.schedulePull(id)
	return Info{ID: id, Name: stg.Name(), Role: perm.Owner, Perms: perm.All, Inbox: inbox, Sync: row.State, Assets: m.assetsModeOf(arow != nil)}, nil
}

func (m *Manager) hook(step string) {
	if m.opt.SyncHook != nil {
		m.opt.SyncHook(step)
	}
}

// connectRetry connects a new space to its repository. A repository GitHub has
// just made is sometimes not found for a few seconds, so "not found" and a
// failure of GitHub's own are tried again after the delays of
// Options.PushRetry, all within Options.PushDeadline: the deadline covers each
// push as well as the waits, and a try that would start after it is not made.
// GitHub also answers "not found" for a credential it does not accept on a
// private repository, which cannot be told apart here; the deadline is what
// bounds that case. Every other refusal of the credential, and a credential
// that could not be had at all, is not tried again.
func (m *Manager) connectRetry(ctx context.Context, sp *space.Space, ro space.RemoteOptions) error {
	ctx, cancel := context.WithTimeout(ctx, m.opt.PushDeadline)
	defer cancel()
	delays := m.opt.PushRetry
	for i := 0; ; i++ {
		err := sp.Connect(ctx, ro)
		if err == nil || i >= len(delays) || !retryable(err) {
			return err
		}
		if dl, ok := ctx.Deadline(); ok && time.Until(dl) <= delays[i] {
			return err // the next try would start after the deadline
		}
		t := time.NewTimer(delays[i])
		select {
		case <-ctx.Done():
			t.Stop()
			return err
		case <-t.C:
		}
	}
}

// retryable: the repository is "not found" (perhaps not yet there), GitHub
// failed, or it could not be reached. Never a credential that could not be had.
func retryable(err error) bool {
	var ce *gitstore.CredentialError
	if errors.As(err, &ce) {
		return false
	}
	var ue *space.UnavailableError
	if !errors.As(err, &ue) {
		return false
	}
	var ge *gitstore.GitError
	text := ue.Msg
	if errors.As(err, &ge) {
		text = ge.Stderr
	}
	low := strings.ToLower(text)
	for _, s := range []string{"not found", "returned error: 5", "could not reach"} {
		if strings.Contains(low, s) {
			return true
		}
	}
	return false
}

// ConnectionFixed tells the manager that what spaces with a connection
// credential were waiting on is fixed (userID connected again; "" the
// provider was saved): the open spaces that were in one of those states are
// fetched now, without waiting for their turn.
func (m *Manager) ConnectionFixed(userID string) {
	if m.single != nil {
		return
	}
	// A GitHub installation connected again restores the spaces that lost it: what
	// the storage requirement knows of them is read again (one cache, rebuilt
	// lazily).
	m.dropFacts("")
	var due []*opening
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	for _, e := range m.open {
		select {
		case <-e.done:
		default:
			continue
		}
		if e.closing != nil || e.sp == nil {
			continue
		}
		if st, ok := e.sp.Remote(); ok && st.State == space.RemoteError && space.IsConnectionKind(st.Kind) {
			e.refs++
			due = append(due, e)
		}
	}
	m.mu.Unlock()
	for _, e := range due {
		go func() {
			defer func() { m.mu.Lock(); e.refs--; m.mu.Unlock() }()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			var ue *space.UnavailableError
			if err := e.sp.Pull(ctx, true); err != nil && !errors.As(err, &ue) && !errors.Is(err, space.ErrClosed) {
				m.log.Warn("fetching a space again after its connection was fixed failed", "error", err)
			}
		}()
	}
}

// invalidAt is a 400 of a connection route: it carries the provider's id like
// every error of a connection, and still is an ErrInvalid.
func invalidAt(provider, format string, a ...any) error {
	e := connections.Errorf(provider, http.StatusBadRequest, connections.CodeInvalid, format, a...)
	e.Err = ErrInvalid
	return e
}

// CreateWithRemote makes a space that follows an existing repository on an
// allowed host from its first moment (milestone 3.1, POST /api/spaces with
// "remote"): the repository is connected before the space becomes visible,
// under the same all-or-nothing rule as a space made with a repository on a
// provider. Only a token can be given here: an SSH key has to be added to the
// repository first, which needs a space to hold the key.
func (m *Manager) CreateWithRemote(ctx context.Context, userID, name, inbox string, req ConnectRequest) (Info, error) {
	if m.single != nil {
		return Info{}, ErrNotFound
	}
	if req.CredType != CredToken {
		return Info{}, fmt.Errorf("%w: a new space can be connected with a token; make the space first to use an SSH key", ErrInvalid)
	}
	box, err := m.box()
	if err != nil {
		return Info{}, err
	}
	if inbox == "" {
		inbox = store.DefaultInbox
	}
	if inbox, err = ValidInbox(inbox); err != nil {
		return Info{}, err
	}
	if name, err = ValidName(name); err != nil {
		return Info{}, err
	}
	remote, err := gitstore.ParseURL(req.URL, m.hostPolicy())
	if err != nil {
		return Info{}, err
	}
	if remote.Scheme != "https" && remote.Scheme != "http" && remote.Scheme != "file" {
		return Info{}, fmt.Errorf("%w: a token goes with an https address", ErrInvalid)
	}
	branch := strings.TrimSpace(req.Branch)
	if branch == "" {
		branch = "main"
	}
	if err := gitstore.ValidBranch(branch); err != nil {
		return Info{}, fmt.Errorf("%w: %s is not a usable branch name", ErrInvalid, branch)
	}
	user := strings.TrimSpace(req.Username)
	if user == "" {
		user = "x-access-token"
	}
	if strings.ContainsAny(user, " \t\r\n:\x00") || req.Token == "" || strings.ContainsAny(req.Token, "\r\n\x00") || len(req.Token) > 4096 {
		return Info{}, fmt.Errorf("%w: the user name or the token is not usable", ErrInvalid)
	}
	creator, err := m.resolveFor(ctx, userID, req.Mode, true)
	if err != nil {
		return Info{}, err
	}
	if err := m.CheckNewSpace(ctx, userID, CredToken, store.ProviderGit); err != nil {
		return Info{}, err
	}
	if err := m.CheckPartialHost(ctx, creator, remote.Host); err != nil {
		return Info{}, err
	}
	// The quota counts the spaces being made as well as the ones that exist;
	// the space is the creator's tenant's, as Create's is.
	m.createMu.Lock()
	u, err := store.GetUser(ctx, m.opt.Store.Q(), store.AllTenants(), userID) // the creator, by id
	if err != nil {
		m.createMu.Unlock()
		return Info{}, err
	}
	if err := m.checkSpaceQuota(ctx, *u); err != nil {
		m.createMu.Unlock()
		return Info{}, err
	}
	tenantID := u.TenantID
	m.reserved[userID]++
	m.reservedT[tenantID]++
	m.createMu.Unlock()
	held := true
	unreserve := func() { // under createMu
		m.reserved[userID]--
		if m.reserved[userID] == 0 {
			delete(m.reserved, userID)
		}
		m.reservedT[tenantID]--
		if m.reservedT[tenantID] == 0 {
			delete(m.reservedT, tenantID)
		}
		held = false
	}
	defer func() {
		if held {
			m.createMu.Lock()
			unreserve()
			m.createMu.Unlock()
		}
	}()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Minute)
	defer cancel()
	stg, err := m.Stage(name, IdentityOf(*u))
	if err != nil {
		return Info{}, err
	}
	committed := false
	defer func() {
		if !committed {
			stg.Abort()
		}
	}()
	id := stg.ID()
	enc, err := box.Seal(id, "token", []byte(req.Token))
	if err != nil {
		return Info{}, err
	}
	row := store.Remote{SpaceID: id, URL: remote.URL, Branch: branch, CredType: CredToken, Provider: store.ProviderGit, Username: user, TokenEnc: enc, State: store.RemoteOK, Updated: m.opt.Now()}
	if err := stg.sp.Connect(ctx, *m.remoteOptions(id, &row)); err != nil {
		return Info{}, err
	}
	st, _ := stg.sp.Remote()
	row.State, row.LastGood, row.LastSync = st.State, st.LastGood, st.LastSync
	now := m.opt.Now()
	m.createMu.Lock()
	unreserve() // the row below takes the place
	err = m.opt.Store.Tx(ctx, func(q store.Q) error {
		if err := store.InsertSpace(ctx, q, tenantID, id, stg.Name(), "", inbox, now); err != nil {
			return err
		}
		if err := store.AddMember(ctx, q, id, userID, perm.Owner, now); err != nil {
			return err
		}
		return store.PutRemote(ctx, q, row)
	})
	m.createMu.Unlock()
	if err != nil {
		return Info{}, err
	}
	committed = true
	stg.Commit(userID)
	m.usageAdded(ctx, store.Touched{UserID: userID, TenantID: tenantID})
	m.setSync(id, row.State)
	m.markCache(id, stg.sp)
	m.schedulePull(id)
	return Info{ID: id, Name: stg.Name(), Role: perm.Owner, DirectRole: perm.Owner, Perms: perm.All, Inbox: inbox, Sync: row.State}, nil
}

// planCreate and resolveExisting ask a provider for a space of the scope: one
// that tells a person's spaces from an organisation's (connections.Scoped) is
// given the scope, any other the tenant.
func planCreate(ctx context.Context, p connections.Provider, sc connections.SpaceScope, userID, owner string) (*connections.Plan, error) {
	if sp, ok := p.(connections.Scoped); ok {
		return sp.PlanCreateFor(ctx, sc, userID, owner)
	}
	return p.PlanCreate(ctx, userID, owner)
}

func resolveExisting(ctx context.Context, p connections.Provider, sc connections.SpaceScope, userID, owner, name, via string) (*connections.Cred, *connections.Repo, error) {
	if sp, ok := p.(connections.Scoped); ok {
		return sp.ResolveExistingFor(ctx, sc, userID, owner, name, via)
	}
	return p.ResolveExistingIn(ctx, sc.TenantID, userID, owner, name, via)
}

// AccessLost tells the manager that these spaces lost the GitHub access they
// synced with (their installation was disconnected): what the storage
// requirement knows of them is read again, and the open ones are fetched now,
// so they show it at once rather than at their next turn.
func (m *Manager) AccessLost(ids []string) {
	if m.single != nil {
		return
	}
	for _, id := range ids {
		m.dropFacts(id)
	}
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	var due []*opening
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	for id, e := range m.open {
		if !want[id] {
			continue
		}
		select {
		case <-e.done:
		default:
			continue
		}
		if e.closing != nil || e.sp == nil {
			continue
		}
		e.refs++
		due = append(due, e)
	}
	m.mu.Unlock()
	for _, e := range due {
		go func() {
			defer func() { m.mu.Lock(); e.refs--; m.mu.Unlock() }()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			_ = e.sp.Pull(ctx, true) // it fails with access_lost, which the space records
		}()
	}
}
