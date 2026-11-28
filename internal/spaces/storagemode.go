package spaces

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"

	"golang.org/x/net/idna"

	"petalbook.app/server/internal/connections"
	"petalbook.app/server/internal/settings"
	"petalbook.app/server/internal/store"
)

// The storage modes of a space (docs/api.md, "Storage modes").
const (
	ModeServer = "server"
	ModeOwn    = "own"
	ModeTenant = "tenant"
)

// ErrNoStorageTarget: the mode asked for stores the space at the tenant's
// storage target, and the tenant has none (409 no_storage_target).
var ErrNoStorageTarget = errors.New("the organization has no storage target")

// ErrTargetNotBuilt: the tenant has a storage target, but this server does not
// make spaces at one yet (501 not_implemented).
var ErrTargetNotBuilt = errors.New("making a space at a storage target is not available yet")

// PartialUnsupportedError refuses connecting a space to a host that cannot sync
// partially while storage.partial.unsupported is "refuse" (409
// partial_unsupported).
type PartialUnsupportedError struct{ Host, Provider string }

func (e *PartialUnsupportedError) Error() string {
	return "The host " + e.Host + " cannot sync a part of a repository, and the administrators do not allow a space on a host that cannot."
}

// ModeRule is what a person making a space may pick: the modes the layers of
// storage.mode leave them and the one a request that does not say gets.
type ModeRule struct {
	Default string
	Allowed []string
	// Locked is true when a lock leaves no choice.
	Locked bool
	// Missing are the allowed modes that cannot be used now ("tenant" while the
	// organization has no storage target).
	Missing []string
}

// ModeRule is the rule for the tenant's new spaces: one cached settings
// lookup, and for an allowed "tenant" one indexed query.
func (m *Manager) ModeRule(ctx context.Context, tenantID string) (ModeRule, error) {
	if m.single != nil {
		return ModeRule{Default: ModeServer, Allowed: []string{ModeServer}}, nil
	}
	m.initStorage(ctx)
	r, err := m.st.set.StorageModeRule(ctx, tenantID)
	if err != nil {
		return ModeRule{}, err
	}
	out := ModeRule{Default: r.Default, Allowed: r.Allowed, Locked: r.LockedBy != ""}
	if slices.Contains(out.Allowed, ModeTenant) {
		ok, err := store.HasUsableStorageTarget(ctx, m.opt.Store.Q(), tenantID)
		if err != nil {
			return ModeRule{}, err
		}
		if !ok {
			out.Missing = []string{ModeTenant}
		}
	}
	return out, nil
}

// ResolveMode decides where a new space of the user is stored. asked is the
// mode the request names ("" for none), hasRemote that the request brings a
// remote (storage.sync or remote). Personal spaces are made at sign-in and
// never come here: they are made local, and a person connects what they follow
// afterwards.
//
// With no mode asked, a request with a remote is "own", else the default. A
// mode the layers leave out is 409 setting_restricted; "tenant" needs the
// organization's storage target (ErrNoStorageTarget). When "server" is not
// among the allowed modes the space must follow a remote (409 remote_required),
// unless the creator is exempt: storage.remotes.require derived for the
// organization (the site's own derivation is in Storage()).
func (m *Manager) ResolveMode(ctx context.Context, userID, tenantID, asked string, hasRemote bool) (string, error) {
	if m.single != nil {
		return ModeServer, nil
	}
	if asked != "" && !slices.Contains([]string{ModeServer, ModeOwn, ModeTenant}, asked) {
		return "", fmt.Errorf("%w: storage.mode is server, own or tenant", ErrInvalid)
	}
	rule, err := m.ModeRule(ctx, tenantID)
	if err != nil {
		return "", err
	}
	mode := asked
	if mode == "" {
		mode = rule.Default
		if hasRemote {
			mode = ModeOwn
		}
	}
	if !slices.Contains(rule.Allowed, mode) {
		return "", &settings.Error{Code: settings.CodeRestricted, Msg: "The administrators do not allow a space stored " + modeWords(mode) + "."}
	}
	switch {
	case hasRemote && mode != ModeOwn:
		return "", fmt.Errorf("%w: a space stored %s takes no remote of its own; use storage.mode own", ErrInvalid, modeWords(mode))
	case mode == ModeTenant:
		if slices.Contains(rule.Missing, ModeTenant) {
			return "", ErrNoStorageTarget
		}
		return "", ErrTargetNotBuilt
	case mode == ModeOwn && !hasRemote && !slices.Contains(rule.Allowed, ModeServer):
		if u, err := store.GetUser(ctx, m.opt.Store.Q(), store.AllTenants(), userID); err == nil && u.RemoteExempt { // the creator, by id
			return mode, nil
		}
		return "", &RequiredError{Require: "any", Msg: "This organization requires a space to follow a remote repository on an allowed host: give a remote or storage.sync."}
	}
	return mode, nil
}

// resolveFor is ResolveMode for the tenant the creator belongs to, which it
// returns: the space is made in it.
func (m *Manager) resolveFor(ctx context.Context, userID, asked string, hasRemote bool) (string, error) {
	if m.single != nil {
		return "", nil
	}
	u, err := store.GetUser(ctx, m.opt.Store.Q(), store.AllTenants(), userID) // the creator, by id
	if err != nil {
		return "", err
	}
	_, err = m.ResolveMode(ctx, userID, u.TenantID, asked, hasRemote)
	return u.TenantID, err
}

func modeWords(mode string) string {
	switch mode {
	case ModeServer:
		return "only on the server"
	case ModeOwn:
		return "at a remote of its own"
	}
	return "at the organization's storage target"
}

// derivedRequire is the requirement of a remote that the site's storage.mode
// leaves when storage.remotes.require is not set: "any" (a provider when the
// allowed hosts are that provider's alone) when "server" is not among the modes
// the site allows, else "".
func (m *Manager) derivedRequire(eff map[string]*settings.Effective) string {
	e := eff[settings.StorageMode]
	if e == nil || slices.Contains(settings.ModeRuleOf(e).Allowed, ModeServer) {
		return ""
	}
	if hosts, _ := eff[settings.StorageHosts].Value.([]string); len(hosts) == 1 && hosts[0] == "github.com" {
		if _, ok := m.opt.Connections.Get("github"); ok {
			return "github"
		}
	}
	return "any"
}

// CheckPartial refuses connecting a space of the tenant to a host that cannot
// sync partially while the tenant's storage.partial.unsupported is "refuse"
// (409 partial_unsupported). It is for a new connection only: a space that is
// connected already is not looked at again. A host not detected yet
// ("unknown") is let through; the detection of 3.5 fills it in as the provider
// is used.
func (m *Manager) CheckPartial(ctx context.Context, tenantID, provider string) error {
	if m.single != nil || provider == "" || provider == store.ProviderGit {
		return nil
	}
	p, ok := m.opt.Connections.Get(provider)
	if !ok {
		return nil
	}
	return m.checkPartialOf(ctx, tenantID, p.Descriptor())
}

// CheckPartialHost is CheckPartial for a remote given as a plain address: when
// its host is one of a provider's (compared as host names, in lower case, without
// a trailing dot and in the form of IDNA's ASCII), that provider's detection
// applies, so an address cannot get round the policy. A host that no provider
// has is let through, nothing being known of it.
func (m *Manager) CheckPartialHost(ctx context.Context, tenantID, host string) error {
	if m.single != nil || host == "" {
		return nil
	}
	want := normHost(host)
	for _, p := range m.opt.Connections.List() {
		if d := p.Descriptor(); d.Host != "" && d.ID != connections.GitID && normHost(d.Host) == want {
			if err := m.checkPartialOf(ctx, tenantID, d); err != nil {
				return err
			}
		}
	}
	return nil
}

// normHost is a host name in one form for comparing: no port, lower case, no
// trailing dot, internationalised names as ASCII.
func normHost(h string) string {
	if hh, _, err := net.SplitHostPort(h); err == nil {
		h = hh
	}
	h = strings.ToLower(strings.TrimSuffix(strings.Trim(h, "[]"), "."))
	if a, err := idna.Lookup.ToASCII(h); err == nil {
		return a
	}
	return h
}

func (m *Manager) checkPartialOf(ctx context.Context, tenantID string, d connections.Descriptor) error {
	if d.Capabilities.PartialSync != "no" {
		return nil
	}
	m.initStorage(ctx)
	e, err := m.st.set.Get(ctx, tenantID, settings.StoragePartialUnsupported)
	if err != nil {
		return err
	}
	if v, _ := e.Value.(string); v != "refuse" {
		return nil
	}
	return &PartialUnsupportedError{Host: d.Host, Provider: d.ID}
}
