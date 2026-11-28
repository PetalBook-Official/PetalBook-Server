package store_test

import (
	"testing"
	"time"

	"petalbook.app/server/internal/store"
	"petalbook.app/server/internal/store/storetest"
)

// A tenant has a storage target to make spaces at when its own target is in
// use and has not failed its check, or a site target shared with it is.
func TestHasUsableStorageTarget(t *testing.T) {
	t.Parallel()
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		q := st.Q()
		now := time.UnixMilli(1_700_000_000_000)
		const ten, other = "T000000017", "T000000018"
		for _, id := range []string{ten, other} {
			if err := st.CreateTenant(ctx, store.Tenant{ID: id, Slug: "t" + id, Name: id, Created: now, Updated: now}); err != nil {
				t.Fatal(err)
			}
		}
		if err := store.InsertConnProviderRow(ctx, q, store.ConnProviderRow{ID: "gl", Type: "gitlab", Host: "gitlab.example.test", Scheme: "https", Name: "GL", Owner: store.ConnProviderSite, Created: now, Updated: now}); err != nil {
			t.Fatal(err)
		}
		has := func(tenant string) bool {
			ok, err := store.HasUsableStorageTarget(ctx, q, tenant)
			if err != nil {
				t.Fatal(err)
			}
			return ok
		}
		if has(ten) {
			t.Fatal("a target before any was made")
		}
		own := store.StorageTarget{ID: "TG1", Owner: ten, ProviderID: "gl", Namespace: "n", State: store.TargetOK, InUse: true, Created: now, Updated: now}
		if err := store.PutStorageTarget(ctx, q, own); err != nil {
			t.Fatal(err)
		}
		if !has(ten) || has(other) {
			t.Fatal("a tenant's own target is its own")
		}
		for _, c := range []struct {
			state string
			inUse bool
			want  bool
		}{{store.TargetBroken, true, false}, {store.TargetChecking, true, true}, {store.TargetOK, false, false}} {
			own.State, own.InUse = c.state, c.inUse
			if err := store.PutStorageTarget(ctx, q, own); err != nil {
				t.Fatal(err)
			}
			if has(ten) != c.want {
				t.Fatalf("%+v: %v", c, !c.want)
			}
		}
		site := store.StorageTarget{ID: "TG2", Owner: "site", ProviderID: "gl", Namespace: "shared", State: store.TargetOK, InUse: true, Created: now, Updated: now}
		if err := store.PutStorageTarget(ctx, q, site); err != nil {
			t.Fatal(err)
		}
		if err := store.SetStorageTargetShares(ctx, q, "TG2", []string{other}); err != nil {
			t.Fatal(err)
		}
		if !has(other) || has(ten) {
			t.Fatal("a site target is for the tenants it is shared with")
		}
	})
}
