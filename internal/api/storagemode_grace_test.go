package api_test

import (
	"testing"
	"time"

	"petalbook.app/server/internal/forge/forgetest"
	"petalbook.app/server/internal/settings"
	"petalbook.app/server/internal/store"
)

const day = 24 * time.Hour

func restrictToOwn(t *testing.T, c *client) {
	t.Helper()
	c.do("PUT", "/api/admin/settings/storage.mode", map[string]any{"value": "own", "lower": map[string]any{"mode": "restrict", "options": []string{"own", "tenant"}}}).want(t, 200, "")
}

// writeLocal tries to change a page of the administrator's personal space: 201
// while the space may still be written, 409 remote_required once it is past its
// deadline.
func writeLocal(c *client) result {
	c.a.t.Helper()
	return c.do("POST", c.personalSpaceBase()+"/pages", map[string]any{"meta": map[string]any{"title": "x"}, "body": "x\n"})
}

func sinceOf(t *testing.T, c *client) string {
	t.Helper()
	s, _ := c.do("GET", "/api/admin/storage", nil).want(t, 200, "").body["remotes"].(map[string]any)["since"].(string)
	return s
}

// A change of what is required starts the grace period at the change, never at
// the time a rule it is made of was written.
func TestWideningTheScopeLongAfterTheModeRuleStartsAFreshGracePeriod(t *testing.T) {
	t.Parallel()
	eachAccounts(t, storageOpts(&netSwitch{}, nil), func(t *testing.T, a *acc) {
		admin := a.setup("kazuha", pw)
		admin.putSetting("storage.remotes.graceDays", 10).want(t, 200, "")
		restrictToOwn(t, admin) // scope "new": nothing is asked of the spaces that exist
		a.clock.Advance(20 * day)
		writeLocal(admin).want(t, 201, "")
		r := admin.putSetting("storage.remotes.scope", "all").want(t, 200, "")
		if aff := r.body["remotes"].(map[string]any); aff["inGrace"] != float64(1) || aff["overdue"] != float64(0) {
			t.Fatal(r.raw)
		}
		writeLocal(admin).want(t, 201, "")
		a.clock.Advance(11 * day)
		writeLocal(admin).want(t, 409, "remote_required")
	})
}

// Handing over from an explicit requirement to the one the modes imply, when
// they differ, is a change too.
func TestClearingAnExplicitRequirementStartsAFreshGracePeriodForTheDerivedOne(t *testing.T) {
	t.Parallel()
	eachAccounts(t, ghOpts, func(t *testing.T, a *acc) {
		admin, _ := ghSetup(t, a)
		admin.putSetting("storage.hosts", []string{"github.com", "example.test"}).want(t, 200, "") // the modes imply "any", not "github"
		admin.putSetting("storage.remotes.scope", "all").want(t, 200, "")
		admin.putSetting("storage.remotes.graceDays", 10).want(t, 200, "")
		restrictToOwn(t, admin)
		admin.putSetting("storage.remotes.require", "github").want(t, 200, "")
		a.clock.Advance(20 * day)
		writeLocal(admin).want(t, 409, "remote_required")
		admin.putSetting("storage.remotes.require", "").want(t, 200, "")
		writeLocal(admin).want(t, 201, "") // "any" begins now
		a.clock.Advance(11 * day)
		writeLocal(admin).want(t, 409, "remote_required")
	})
}

// A rule written long before this server started (or before this change was
// merged) begins its grace period when the server starts.
func TestARuleOlderThanTheGracePeriodDoesNotMakeSpacesOverdueAtStart(t *testing.T) {
	t.Parallel()
	eachAccounts(t, storageOpts(&netSwitch{}, nil), func(t *testing.T, a *acc) {
		admin := a.setup("kazuha", pw)
		ctx := t.Context()
		if _, err := settings.Of(a.st).SetSite(ctx, "storage.mode", settings.Change{HasLower: true, Lower: &settings.Lower{Mode: settings.Restrict, Options: []any{"own", "tenant"}}}); err != nil {
			t.Fatal(err)
		}
		a.setSetting(t, "storage.remotes.scope", "all")
		a.setSetting(t, "storage.remotes.graceDays", int64(10))
		start := a.clock.Now()
		a.clock.Advance(40 * day)
		a.restart(t)
		admin.do("POST", "/api/login", map[string]any{"username": "kazuha", "password": pw}).want(t, 200, "") // the session is 40 days old
		writeLocal(admin).want(t, 201, "")
		since, err := time.Parse(time.RFC3339, sinceOf(t, admin))
		if err != nil || since.Before(start.Add(39*day)) {
			t.Fatalf("since %v (%v), the rule was written %v", since, err, start)
		}
		a.clock.Advance(11 * day)
		writeLocal(admin).want(t, 409, "remote_required")
	})
}

// Only a widening of the requirement starts the grace period again: a change of
// the hosts that leaves what is required alone, and a narrowing of the scope,
// keep the time it began, so the spaces it covered stay covered.
func TestOnlyAWideningOfTheRequirementStartsAFreshGracePeriod(t *testing.T) {
	t.Parallel()
	eachAccounts(t, storageOpts(&netSwitch{}, nil), func(t *testing.T, a *acc) {
		admin := a.setup("kazuha", pw)
		admin.putSetting("storage.remotes.require", "any").want(t, 200, "")
		since := sinceOf(t, admin)
		a.clock.Advance(5 * day)
		admin.putSetting("storage.hosts", []string{"example.test"}).want(t, 200, "")
		admin.putSetting("storage.hosts", []string{"example.test", "other.test"}).want(t, 200, "")
		if sinceOf(t, admin) != since {
			t.Fatalf("a change of the hosts moved since from %q to %q", since, sinceOf(t, admin))
		}
		// Widening: scope "all" starts it now. Narrowing back does not move it.
		admin.putSetting("storage.remotes.scope", "all").want(t, 200, "")
		wide := sinceOf(t, admin)
		if wide == since {
			t.Fatal("widening the scope did not start a grace period")
		}
		a.clock.Advance(day)
		admin.putSetting("storage.remotes.scope", "new").want(t, 200, "")
		if sinceOf(t, admin) != wide {
			t.Fatal("narrowing the scope moved since")
		}
	})
}

// A change of the settings whose settling failed (the record still names the old
// requirement) is made good by the next rebuild: the grace period is counted from
// then once, not again at every rebuild.
func TestAnUnsettledRequirementIsSettledByTheNextRebuild(t *testing.T) {
	t.Parallel()
	eachAccounts(t, storageOpts(&netSwitch{}, nil), func(t *testing.T, a *acc) {
		admin := a.setup("kazuha", pw)
		a.setSetting(t, "storage.remotes.require", "any") // written without settling, as when the settle failed
		first := sinceOf(t, admin)
		if first == "" {
			t.Fatal("no since for the requirement in force")
		}
		a.clock.Advance(5 * day)
		admin.putSetting("cache.minIdleHours", 30).want(t, 200, "") // anything that rebuilds the policy
		if sinceOf(t, admin) != first {
			t.Fatalf("since moved from %q to %q: the record was not settled", first, sinceOf(t, admin))
		}
	})
}

// The personal space is made local whatever the modes say, so an organisation
// that leaves out the server does not stop its owner taking the remote off.
func TestAPersonalSpaceMayLoseItsRemoteWhereTheOrganisationLeavesOutTheServer(t *testing.T) {
	t.Parallel()
	eachAccounts(t, fgOpts, func(t *testing.T, a *acc) {
		admin, member := fgSetup(t, a, "gitlab", a.gl)
		a.gl.AddRepo("acme", "wiki", true)
		base := member.personalSpaceBase()
		member.do("PUT", base+"/remote", map[string]any{"url": a.gl.URL + "/acme/wiki.git", "credential": map[string]any{"type": "connection", "provider": "gitlab", "via": "user"}}).want(t, 200, "")
		admin.do("PUT", "/api/admin/tenants/"+store.DefaultTenantID+"/settings/storage.mode", map[string]any{"value": "own", "lower": map[string]any{"mode": "restrict", "options": []string{"own"}}}).want(t, 200, "")
		member.do("DELETE", base+"/remote", nil).want(t, 204, "")
	})
}

// An organisation that leaves out the server does not let a space go back to it.
func TestARemoteCannotBeRemovedWhereTheOrganisationLeavesOutTheServer(t *testing.T) {
	t.Parallel()
	eachAccounts(t, fgOpts, func(t *testing.T, a *acc) {
		admin, member := fgSetup(t, a, "gitlab", a.gl)
		mk := func(name string) string {
			return member.do("POST", "/api/spaces", map[string]any{"name": name, "storage": fgSync("gitlab", map[string]any{"owner": "acme", "name": name, "create": true, "assets": false})}).want(t, 201, "").body["id"].(string)
		}
		first, second := mk("one"), mk("two")
		// Allowed everywhere: a remote can go.
		a.gl.AddRepo("acme", "wiki", true)
		member.do("DELETE", "/api/spaces/"+first+"/remote", nil).want(t, 204, "")
		// The organisation's own rule, the site's staying open.
		admin.do("PUT", "/api/admin/tenants/"+store.DefaultTenantID+"/settings/storage.mode", map[string]any{"value": "own", "lower": map[string]any{"mode": "restrict", "options": []string{"own"}}}).want(t, 200, "")
		member.do("DELETE", "/api/spaces/"+second+"/remote", nil).want(t, 409, "remote_required")
		// Someone exempt may.
		if _, err := a.st.Q().Exec(t.Context(), `UPDATE users SET remote_exempt = 1 WHERE username = ?`, "tang"); err != nil {
			t.Fatal(err)
		}
		a.mgr.PolicyChanged()
		member.do("DELETE", "/api/spaces/"+second+"/remote", nil).want(t, 204, "")
	})
}

// A remote given as a plain address on a host that is a provider's is held to
// what is known of that provider.
func TestAPlainAddressOnAProvidersHostIsHeldToThePartialPolicy(t *testing.T) {
	t.Parallel()
	eachForge(t, func(t *testing.T, a *acc, kind string, f *forgetest.Fake) {
		f.NoPartial = true
		admin, member := fgSetup(t, a, kind, f)
		member.do("POST", "/api/spaces", map[string]any{"name": "one", "storage": fgSync(kind, map[string]any{"owner": "acme", "name": "one", "create": true})}).want(t, 201, "") // the host is found out
		local := member.do("POST", "/api/spaces", map[string]any{"name": "Local"}).want(t, 201, "").body["id"].(string)
		plain := map[string]any{"url": f.URL + "/acme/wiki.git", "credential": map[string]any{"type": "token", "token": "x"}}
		admin.putSetting("storage.partial.unsupported", "refuse").want(t, 200, "")
		member.do("PUT", "/api/spaces/"+local+"/remote", plain).want(t, 409, "partial_unsupported")
		r := member.do("POST", "/api/spaces", map[string]any{"name": "Two", "remote": plain}).want(t, 409, "partial_unsupported")
		if r.body["host"] == nil {
			t.Fatal(r.raw)
		}
	})
}

// A request that cannot be made is refused for what it is before the storage rules are asked.
func TestARequestIsValidatedBeforeTheStorageModeIsResolved(t *testing.T) {
	t.Parallel()
	eachAccounts(t, accOpts{}, func(t *testing.T, a *acc) {
		c := a.setup("kazuha", pw)
		c.do("POST", "/api/spaces", map[string]any{"name": "", "storage": map[string]any{"mode": "tenant"}}).want(t, 400, "invalid")
	})
}
