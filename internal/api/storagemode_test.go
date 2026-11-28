package api_test

import (
	"strings"
	"testing"
	"time"

	"petalbook.app/server/internal/forge/forgetest"
	"petalbook.app/server/internal/store"
)

// modeOf is what a person's client reads about where their new spaces may go.
func modeOf(t *testing.T, c *client) map[string]any {
	t.Helper()
	return settingOf(t, c.do("GET", "/api/settings", nil).want(t, 200, ""), "storage.mode")
}

func putMode(t *testing.T, c *client, path string, body map[string]any) {
	t.Helper()
	c.do("PUT", path, body).want(t, 200, "")
}

func addTarget(t *testing.T, a *acc, owner, provider string) {
	t.Helper()
	now := a.clock.Now()
	if err := store.PutStorageTarget(t.Context(), a.st.Q(), store.StorageTarget{ID: "TG" + owner, Owner: owner, ProviderID: provider, Namespace: "notes",
		State: store.TargetOK, InUse: true, Created: now, Updated: now}); err != nil {
		t.Fatal(err)
	}
}

func TestANewSpaceIsMadeInAModeTheSettingsAllow(t *testing.T) {
	t.Parallel()
	eachAccounts(t, fgOpts, func(t *testing.T, a *acc) {
		admin, member := fgSetup(t, a, "gitlab", a.gl)
		mk := func(name string, mode any, sync bool) result {
			st := map[string]any{}
			if mode != nil {
				st["mode"] = mode
			}
			if sync {
				st = fgSync("gitlab", map[string]any{"owner": "acme", "name": name, "create": true})
				if mode != nil {
					st["mode"] = mode
				}
			}
			return member.do("POST", "/api/spaces", map[string]any{"name": name, "storage": st})
		}
		// Everything is allowed to begin with, and the default is the server.
		if m := modeOf(t, member); m["effective"] != "server" || strings.Join(strs(m["allowed"]), ",") != "server,own,tenant" || m["locked"] != false ||
			strings.Join(strs(m["missing"]), ",") != "tenant" {
			t.Fatal(m)
		}
		mk("a", nil, false).want(t, 201, "")
		mk("b", "server", false).want(t, 201, "")
		mk("c", "own", false).want(t, 201, "")          // follows a remote it is given later
		mk("d", nil, true).want(t, 201, "")             // a remote with no mode is "own"
		mk("e", "own", true).want(t, 201, "")           // the same, said
		mk("f", "server", true).want(t, 400, "invalid") // a server space follows nothing
		mk("g", "elsewhere", false).want(t, 400, "invalid")
		// "tenant" needs the organisation's storage target.
		mk("h", "tenant", false).want(t, 409, "no_storage_target")
		addTarget(t, a, store.DefaultTenantID, "gitlab")
		// It has one: this server does not make spaces at it yet.
		mk("i", "tenant", false).want(t, 501, "not_implemented")
		if m := modeOf(t, member); m["missing"] != nil {
			t.Fatal(m)
		}

		// A restriction to "own": the server is out, and a space must follow a remote.
		putMode(t, admin, "/api/admin/settings/storage.mode", map[string]any{"value": "own", "lower": map[string]any{"mode": "restrict", "options": []string{"own", "tenant"}}})
		if m := modeOf(t, member); m["effective"] != "own" || strings.Join(strs(m["allowed"]), ",") != "own,tenant" {
			t.Fatal(m)
		}
		mk("j", "server", false).want(t, 409, "setting_restricted")
		mk("k", nil, false).want(t, 409, "remote_required") // the default is "own", with nothing to follow
		mk("l", "own", false).want(t, 409, "remote_required")
		mk("m", nil, true).want(t, 201, "")
		mk("n", "own", true).want(t, 201, "")
		// A person exempt from the requirement may make one without.
		a.st.Q().Exec(t.Context(), `UPDATE users SET remote_exempt = 1 WHERE username = ?`, "tang")
		mk("o", "own", false).want(t, 201, "")

		// A lock: one mode, and nothing else is offered.
		putMode(t, admin, "/api/admin/settings/storage.mode", map[string]any{"value": "server", "lower": map[string]any{"mode": "lock"}})
		if m := modeOf(t, member); m["effective"] != "server" || strings.Join(strs(m["allowed"]), ",") != "server" || m["locked"] != true {
			t.Fatal(m)
		}
		mk("p", "own", false).want(t, 409, "setting_restricted")
		mk("q", nil, true).want(t, 409, "setting_restricted") // a remote asks for "own"
		mk("r", nil, false).want(t, 201, "")
		if a.gl.RepoByName("acme/q") != nil || a.gl.RepoByName("acme/j") != nil {
			t.Fatal("a repository was made for a refused request")
		}
	})
}

func TestAnOrganisationsModesAreItsOwnAndPersonalSpacesAreNotAskedAboutThem(t *testing.T) {
	t.Parallel()
	eachAccounts(t, accOpts{}, func(t *testing.T, a *acc) {
		f := newTenFx(t, a)
		tpath := "/api/admin/tenants/" + f.tenB + "/settings/storage.mode"
		// The organisation narrows its own spaces to a remote of their own; the site's stay as they are.
		putMode(t, f.admB, tpath, map[string]any{"value": "own", "lower": map[string]any{"mode": "restrict", "options": []string{"own"}}})
		if m := modeOf(t, f.memB); m["effective"] != "own" || strings.Join(strs(m["allowed"]), ",") != "own" {
			t.Fatal(m)
		}
		if m := modeOf(t, f.memA); m["effective"] != "server" || strings.Join(strs(m["allowed"]), ",") != "server,own,tenant" {
			t.Fatal(m)
		}
		f.memB.do("POST", "/api/spaces", map[string]any{"name": "x"}).want(t, 409, "remote_required")
		f.memB.do("POST", "/api/spaces", map[string]any{"name": "x", "storage": map[string]any{"mode": "server"}}).want(t, 409, "setting_restricted")
		f.memA.do("POST", "/api/spaces", map[string]any{"name": "x"}).want(t, 201, "")
		// The organisation's own rule is not the server's: the site's requirement stays off.
		if r := f.root.do("GET", "/api/admin/storage", nil).want(t, 200, ""); r.body["remotes"].(map[string]any)["since"] != nil {
			t.Fatal(r.raw)
		}
		// A personal space is made local at sign-in whatever the modes say.
		hasPersonal := func(c *client) bool {
			for _, s := range c.do("GET", "/api/spaces", nil).want(t, 200, "").body["spaces"].([]any) {
				if s.(map[string]any)["personal"] == true {
					return true
				}
			}
			return false
		}
		if !hasPersonal(f.memB) {
			t.Fatal("no personal space under a restriction to own")
		}
		// The site's restriction beats the organisation's value: it cannot choose what the site left out.
		putMode(t, f.root, "/api/admin/settings/storage.mode", map[string]any{"lower": map[string]any{"mode": "restrict", "options": []string{"server", "own"}}})
		f.admA.do("PUT", "/api/admin/tenants/"+store.DefaultTenantID+"/settings/storage.mode", map[string]any{"value": "tenant"}).want(t, 409, "setting_restricted")
		invB := f.admB.do("POST", "/api/invites", map[string]any{"role": "member"}).want(t, 201, "").body
		late := a.newClient("")
		late.do("POST", "/api/invites/accept", map[string]any{"token": invB["token"], "username": "late-b", "name": "Late", "password": pw}).want(t, 201, "")
		if !hasPersonal(late) {
			t.Fatal("a person who joined later has no personal space")
		}
	})
}

func TestTheModesTheSiteAllowsDeriveTheRequirementOfARemote(t *testing.T) {
	t.Parallel()
	n := &netSwitch{}
	eachAccounts(t, storageOpts(n, nil), func(t *testing.T, a *acc) {
		admin := a.setup("kazuha", pw)
		// What the space of the administrator is asked for: the notice it carries.
		require := func() any {
			for _, s := range admin.do("GET", "/api/spaces", nil).body["spaces"].([]any) {
				if s.(map[string]any)["personal"] == true {
					if n, _ := s.(map[string]any)["storage"].(map[string]any)["remote"].(map[string]any); n != nil {
						return n["required"]
					}
				}
			}
			return nil
		}
		if require() != nil {
			t.Fatal("a requirement before any rule")
		}
		admin.putSetting("storage.remotes.scope", "all").want(t, 200, "")
		admin.putSetting("storage.remotes.graceDays", 10).want(t, 200, "")
		// A default of "own" with every mode still allowed implies nothing.
		admin.do("PUT", "/api/admin/settings/storage.mode", map[string]any{"value": "own"}).want(t, 200, "")
		if require() != nil {
			t.Fatal("a default alone made a requirement")
		}
		// "server" no longer allowed: a remote is required, with the grace period of the rule.
		r := admin.do("PUT", "/api/admin/settings/storage.mode", map[string]any{"lower": map[string]any{"mode": "restrict", "options": []string{"own", "tenant"}}}).want(t, 200, "")
		if require() != "any" || r.body["remotes"].(map[string]any)["inGrace"] != float64(1) {
			t.Fatal(require(), r.raw)
		}
		// A space with a remote is the only kind that can be made, and the notice names the requirement.
		admin.do("POST", "/api/spaces", map[string]any{"name": "Local"}).want(t, 409, "remote_required")
		var note map[string]any
		for _, s := range admin.do("GET", "/api/spaces", nil).body["spaces"].([]any) {
			if s.(map[string]any)["personal"] == true {
				note = s.(map[string]any)["storage"].(map[string]any)["remote"].(map[string]any)
			}
		}
		if note == nil || note["required"] != "any" || note["overdue"] != false {
			t.Fatalf("%v", note)
		}
		// Past the grace period the space is read only, as for any requirement.
		a.clock.Advance(11 * 24 * time.Hour)
		sid := admin.personalSpace()
		admin.do("POST", "/api/spaces/"+sid+"/pages", map[string]any{"meta": map[string]any{"title": "Late"}, "body": "x\n"}).want(t, 409, "remote_required")
		// A requirement of its own wins over the derived one.
		admin.putSetting("storage.remotes.require", "any").want(t, 200, "")
		admin.putSetting("storage.remotes.require", "").want(t, 200, "")
		if require() != "any" {
			t.Fatal("the derived requirement is gone while the modes still leave out the server")
		}
		// Allowing the server again lifts it.
		r = admin.do("PUT", "/api/admin/settings/storage.mode", map[string]any{"lower": map[string]any{"mode": "allow"}}).want(t, 200, "")
		if require() != nil || r.body["remotes"].(map[string]any)["overdue"] != float64(0) {
			t.Fatal(require(), r.raw)
		}
		admin.do("POST", "/api/spaces/"+sid+"/pages", map[string]any{"meta": map[string]any{"title": "Again"}, "body": "x\n"}).want(t, 201, "")
	})
}

func TestAHostThatCannotSyncPartiallyIsRefusedOnlyWhereThePolicySaysSoAndOnlyForNewConnections(t *testing.T) {
	t.Parallel()
	eachForge(t, func(t *testing.T, a *acc, kind string, f *forgetest.Fake) {
		f.NoPartial = true
		admin, member := fgSetup(t, a, kind, f)
		f.AddRepo("acme", "wiki", true)
		mkSync := func(name string) result {
			return member.do("POST", "/api/spaces", map[string]any{"name": name, "storage": fgSync(kind, map[string]any{"owner": "acme", "name": name, "create": true})})
		}
		connect := func(id string) result {
			return member.do("PUT", "/api/spaces/"+id+"/remote", map[string]any{"url": f.URL + "/acme/wiki.git", "credential": map[string]any{"type": "connection", "provider": kind, "via": "user"}})
		}
		// Allowed (the default): the host is found unable to filter by the first space, and the space is cloned in full.
		one := mkSync("one").want(t, 201, "").body["id"].(string)
		local := member.do("POST", "/api/spaces", map[string]any{"name": "Local"}).want(t, 201, "").body["id"].(string)
		if fgProvider(t, member, kind)["capabilities"].(map[string]any)["partialSync"] != "no" {
			t.Fatal(fgProvider(t, member, kind))
		}
		// The site refuses: a new connection answers 409 with the host, and nothing is made on the host.
		admin.putSetting("storage.partial.unsupported", "refuse").want(t, 200, "")
		r := mkSync("two").want(t, 409, "partial_unsupported")
		if r.body["host"] == nil || r.body["host"] == "" || r.body["provider"] != kind {
			t.Fatal(r.raw)
		}
		if f.RepoByName("acme/two") != nil {
			t.Fatal("a repository was made for a refused space")
		}
		connect(local).want(t, 409, "partial_unsupported")
		member.do("GET", "/api/spaces/"+local+"/remote", nil).want(t, 404, "not_found")
		// What is connected already keeps working.
		if rem := member.do("GET", "/api/spaces/"+one+"/remote", nil).want(t, 200, ""); rem.body["state"] != "ok" {
			t.Fatal(rem.raw)
		}
		member.do("POST", "/api/spaces/"+one+"/pages", map[string]any{"meta": map[string]any{"title": "Still here"}, "body": "x\n"}).want(t, 201, "")
		// A plain address is not a provider's host: nothing is known of it, so it is not refused.
		member.do("POST", "/api/spaces", map[string]any{"name": "Plain"}).want(t, 201, "")
		// Allowed at the site and refused by the organisation: the same.
		admin.putSetting("storage.partial.unsupported", "allow").want(t, 200, "")
		connect(local).want(t, 200, "")
		member.do("DELETE", "/api/spaces/"+local+"/remote", nil)
		admin.do("PUT", "/api/admin/tenants/"+store.DefaultTenantID+"/settings/storage.partial.unsupported", map[string]any{"value": "refuse"}).want(t, 200, "")
		connect(local).want(t, 409, "partial_unsupported")
	})
}
