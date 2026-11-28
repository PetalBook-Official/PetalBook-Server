package spaces

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"petalbook.app/server/internal/gitstore"
	"petalbook.app/server/internal/index"
	"petalbook.app/server/internal/page"
	"petalbook.app/server/internal/space"
	"petalbook.app/server/internal/store"
	"petalbook.app/server/internal/store/storetest"
)

// Milestone 3.1, part 1: partial clones, old versions on demand, eviction of
// idle spaces and their restore, the requirement of a remote. Every test runs
// on SQLite and, when PETALBOOK_TEST_POSTGRES is set, on PostgreSQL. The remote
// is a local bare repository that serves filters and fetches by id, as GitHub does.

func filterBare(t *testing.T) string {
	t.Helper()
	dir := bare(t)
	runGit(t, dir, "config", "uploadpack.allowFilter", "true")
	runGit(t, dir, "config", "uploadpack.allowAnySHA1InWant", "true")
	return dir
}

func fileTokenReq(remote string) ConnectRequest { return tokenReq("file://" + remote) }

// seedRemote pushes n versions of one page and a second page, as somebody else.
func seedRemote(t *testing.T, remote string, versions int) {
	t.Helper()
	for i := 1; i <= versions; i++ {
		push(t, remote, "Tang/Snow.md", pageFile("SNAP000001", "Snow", fmt.Sprintf("version %d\n", i)))
	}
	push(t, remote, "Other.md", pageFile("ETHR000001", "Other", "other\n"))
}

// connected makes a space that follows a remote holding what seedRemote put
// there. It is a partial clone when the server's Git can make one.
func (f *rm) connected(name string, remote string) (Info, *Handle) {
	f.t.Helper()
	in, h := f.space(name)
	if _, err := f.m.Connect(ctx, h, fileTokenReq(remote)); err != nil {
		f.t.Fatal(err)
	}
	return in, h
}

// idle lets the time pass so that nothing about the space counts as recent use.
func (f *rm) idle(d time.Duration) { f.clk.Advance(d) }

func (f *rm) root(id string) string { r, _ := f.m.paths(id); return r }

// serving opens a manager as the server does: it cleans up at its start.
func serving(o *Options) { o.Serve = true }

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func cacheOf(t *testing.T, st *store.Store, id string) string {
	t.Helper()
	s, err := store.GetSpaceStorage(ctx, st.Q(), id)
	if err != nil {
		t.Fatal(err)
	}
	return s.Cache
}

func pageIDs(t *testing.T, sp *space.Space) []string {
	t.Helper()
	res, err := sp.List(index.Query{Recursive: true})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, it := range res.Items {
		ids = append(ids, it.ID)
	}
	return ids
}

func TestConnectingToARemoteWithContentMakesAPartialClone(t *testing.T) {
	t.Parallel()
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		f := remoteManager(t, st)
		remote := filterBare(t)
		seedRemote(t, remote, 3)
		in, h := f.connected("Notes", remote)
		defer h.Release()
		if !h.Space.Partial() {
			t.Fatal("a space that took the content of a remote is cloned partially")
		}
		// The current text is all there; the old versions are not.
		if got, err := h.Space.Get("SNAP000001"); err != nil || got.Body != "version 3\n" {
			t.Fatalf("%v %v", got, err)
		}
		missing := runGit(t, f.root(in.ID), "rev-list", "--objects", "--missing=print", "HEAD")
		if !strings.Contains(missing, "?") {
			t.Fatalf("every version is on the server: %q", missing)
		}
		// History fetches the old versions, in one request, and keeps them.
		before := f.r.fetchCount()
		items, _, err := h.Space.History(ctx, "SNAP000001", 30, "")
		if err != nil || len(items) != 3 {
			t.Fatalf("%d versions, %v", len(items), err)
		}
		if n := f.r.fetchCount() - before; n != 1 {
			t.Fatalf("the contents were fetched in %d requests, want 1", n)
		}
		v, err := h.Space.Version(ctx, "SNAP000001", items[2].Commit)
		if err != nil || v.Body != "version 1\n" {
			t.Fatalf("%v %v", v, err)
		}
		before = f.r.fetchCount()
		if _, _, err := h.Space.History(ctx, "SNAP000001", 30, ""); err != nil {
			t.Fatal(err)
		}
		if n := f.r.fetchCount() - before; n != 0 {
			t.Fatalf("a version that was fetched was fetched again (%d requests)", n)
		}
		// A page of history is the same on a full clone of the same remote.
		full := filepath.Join(t.TempDir(), "full")
		runGit(t, t.TempDir(), "clone", "-q", remote, full)
		for i, it := range items {
			want := runGit(t, full, "log", "--format=%H", "--", "Tang/Snow.md")
			if !strings.Contains(want, it.Commit) {
				t.Fatalf("version %d %s is not in the full clone's history", i, it.Commit)
			}
		}
		// The state database says what the clone is.
		time.Sleep(100 * time.Millisecond)
		if c := cacheOf(t, st, in.ID); c != store.CachePartial {
			t.Fatalf("cache %q", c)
		}
	})
}

func TestAnExistingFullCloneStaysFullAndALocalSpaceIsNeverPartial(t *testing.T) {
	t.Parallel()
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		f := remoteManager(t, st)
		// A space with history of its own that is connected to an empty remote keeps its clone.
		in, h := f.space("Mine")
		p, err := h.Space.Create(space.Author{}, space.CreateInput{Meta: page.Meta{{Key: "title", Value: "One"}}})
		if err != nil {
			t.Fatal(err)
		}
		if err := h.Space.Flush(); err != nil {
			t.Fatal(err)
		}
		remote := filterBare(t)
		if _, err := f.m.Connect(ctx, h, fileTokenReq(remote)); err != nil {
			t.Fatal(err)
		}
		if h.Space.Partial() {
			t.Fatal("a clone with its own history was made partial")
		}
		_ = p
		_ = in
		// A local space is local, and the policy cannot evict or partially clone it.
		in2, h2 := f.space("Local")
		defer h2.Release()
		if h2.Space.Partial() {
			t.Fatal("a local space is partial")
		}
		h2.Release()
		f.idle(72 * time.Hour)
		if err := f.m.Evict(ctx, in2.ID, EvictOptions{AdminRequest: true}); !errors.Is(err, ErrNotEvictable) {
			t.Fatalf("a local space was evictable: %v", err)
		}
		if !exists(f.root(in2.ID)) {
			t.Fatal("the local space is gone")
		}
	})
}

func TestHistoryAnswersNotCachedWhenTheRemoteCannotBeReached(t *testing.T) {
	t.Parallel()
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		f := remoteManager(t, st)
		remote := filterBare(t)
		seedRemote(t, remote, 3)
		_, h := f.connected("Notes", remote)
		defer h.Release()
		f.r.setDown(true)
		_, _, err := h.Space.History(ctx, "SNAP000001", 30, "")
		var nc *space.NotCachedError
		if !errors.As(err, &nc) || nc.Fetching {
			t.Fatalf("want a not_cached that is not fetching, got %v", err)
		}
		// Reading the current text is not affected, and the list of versions
		// the server has stays complete or absent: no partial list.
		if got, err := h.Space.Get("SNAP000001"); err != nil || got.Body != "version 3\n" {
			t.Fatalf("%v %v", got, err)
		}
		f.r.setDown(false)
		time.Sleep(10 * time.Millisecond)
		if items, _, err := h.Space.History(ctx, "SNAP000001", 30, ""); err != nil || len(items) != 3 {
			t.Fatalf("%d %v", len(items), err)
		}
	})
}

func TestAFetchThatTakesLongerAnswersFetchingAndGoesOn(t *testing.T) {
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		f := remoteManager(t, st)
		remote := filterBare(t)
		seedRemote(t, remote, 3)
		_, h := f.connected("Notes", remote)
		defer h.Release()
		old := space.FetchWait
		space.FetchWait = 50 * time.Millisecond
		defer func() { space.FetchWait = old }()
		f.r.mu.Lock()
		f.r.hold = make(chan struct{})
		hold := f.r.hold
		f.r.mu.Unlock()
		_, _, err := h.Space.History(ctx, "SNAP000001", 30, "")
		var nc *space.NotCachedError
		if !errors.As(err, &nc) || !nc.Fetching {
			t.Fatalf("want a not_cached that is fetching, got %v", err)
		}
		close(hold)
		f.r.mu.Lock()
		f.r.hold = nil
		f.r.mu.Unlock()
		// The fetch went on: the next try finds the contents.
		deadline := time.Now().Add(5 * time.Second)
		for {
			items, _, err := h.Space.History(ctx, "SNAP000001", 30, "")
			if err == nil && len(items) == 3 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("the fetch did not finish: %v", err)
			}
			time.Sleep(20 * time.Millisecond)
		}
	})
}

// --- eviction and restore -----------------------------------------------------------

func TestEvictAndRestoreKeepTheIndexAndTheText(t *testing.T) {
	t.Parallel()
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		f := remoteManager(t, st)
		remote := filterBare(t)
		seedRemote(t, remote, 3)
		in, h := f.connected("Notes", remote)
		before, err := h.Space.List(index.Query{Recursive: true})
		if err != nil {
			t.Fatal(err)
		}
		h.Release()
		f.idle(72 * time.Hour)
		if err := f.m.Evict(ctx, in.ID, EvictOptions{}); err != nil {
			t.Fatal(err)
		}
		if exists(f.root(in.ID)) {
			t.Fatal("the clone is still there")
		}
		if cacheOf(t, st, in.ID) != store.CacheEvicted {
			t.Fatalf("cache %q", cacheOf(t, st, in.ID))
		}
		// Lists, the tree and search come from the index, without a restore.
		ih, err := f.m.OpenIndex(ctx, f.user, in.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !ih.IndexOnly {
			t.Fatal("an evicted space was opened with files")
		}
		got, err := ih.Space.List(index.Query{Recursive: true})
		if err != nil || len(got.Items) != len(before.Items) {
			t.Fatalf("%d pages, %v", len(got.Items), err)
		}
		if tree, err := ih.Space.Tree(); err != nil || len(tree) == 0 {
			t.Fatalf("%v %v", tree, err)
		}
		if res, err := ih.Space.List(index.Query{Text: "version"}); err != nil || len(res.Items) != 1 {
			t.Fatalf("%v %v", res, err)
		}
		ih.Release()
		if exists(f.root(in.ID)) {
			t.Fatal("reading the index restored the space")
		}
		if info, _ := f.m.List(ctx, f.user.ID); info[0].Cache != "evicted" {
			t.Fatalf("the space says %q", info[0].Cache)
		}
		// Anything that needs a file restores it, and the order by update is what it was.
		h2, err := f.m.Open(ctx, f.user, in.ID)
		if err != nil {
			t.Fatal(err)
		}
		defer h2.Release()
		if !exists(f.root(in.ID)) || !h2.Space.Partial() {
			t.Fatal("not restored as a partial clone")
		}
		if p, err := h2.Space.Get("SNAP000001"); err != nil || p.Body != "version 3\n" {
			t.Fatalf("%v %v", p, err)
		}
		after, err := h2.Space.List(index.Query{Recursive: true, Props: true})
		if err != nil || len(after.Items) != len(before.Items) {
			t.Fatalf("%d %v", len(after.Items), err)
		}
		times := map[string]time.Time{}
		for _, it := range before.Items {
			times[it.ID] = it.Updated
		}
		for _, it := range after.Items {
			if !it.Updated.Equal(times[it.ID]) {
				t.Errorf("%s: updated %v, was %v", it.ID, it.Updated, times[it.ID])
			}
		}
		if cacheOf(t, st, in.ID) != store.CachePartial {
			t.Fatalf("cache %q", cacheOf(t, st, in.ID))
		}
		// And it follows its remote again as before.
		push(t, remote, "New.md", pageFile("NEWW000001", "New", "n\n"))
		if err := f.m.PullNow(ctx, in.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := h2.Space.Get("NEWW000001"); err != nil {
			t.Fatal(err)
		}
	})
}

// evictable makes a connected, idle, released space ready to be evicted.
func (f *rm) evictable(remote string) (Info, string) {
	f.t.Helper()
	in, h := f.connected("Notes", remote)
	h.Release()
	f.idle(72 * time.Hour)
	return in, f.root(in.ID)
}

func mustBusy(t *testing.T, err error, root, why string) {
	t.Helper()
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("want busy (%s), got %v", why, err)
	}
	if !strings.Contains(BusyMessage(err), why) {
		t.Fatalf("busy for the wrong reason: %q, want %q", BusyMessage(err), why)
	}
	if !exists(root) {
		t.Fatal("a space that was refused is gone")
	}
}

func TestNothingIsEvictedThatTheRemoteDoesNotHave(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		prepare func(t *testing.T, st *store.Store, remote string, f *rm, in Info, root string)
		why     string
	}{
		{"a commit that is not on the remote", func(t *testing.T, st *store.Store, remote string, f *rm, in Info, root string) {
			os.WriteFile(filepath.Join(root, "Mine.md"), []byte(pageFile("MYNE000001", "Mine", "m\n")), 0o644)
			runGit(t, root, "add", "-A")
			runGit(t, root, "commit", "-qm", "only here")
		}, "not on the remote"},
		{"a change not committed", func(t *testing.T, st *store.Store, remote string, f *rm, in Info, root string) {
			os.WriteFile(filepath.Join(root, "Other.md"), []byte(pageFile("ETHR000001", "Other", "changed\n")), 0o644)
		}, "not committed"},
		{"a file no commit has", func(t *testing.T, st *store.Store, remote string, f *rm, in Info, root string) {
			os.WriteFile(filepath.Join(root, "Stray.md"), []byte("x"), 0o644)
		}, "no commit has"},
		{"a commit set aside because it never reached the remote", func(t *testing.T, st *store.Store, remote string, f *rm, in Info, root string) {
			head := runGit(t, root, "rev-parse", "HEAD")
			runGit(t, root, "update-ref", "refs/petalbook/unpushed/20260101-000000", head)
		}, "set aside"},
		{"a journal that is not empty", func(t *testing.T, st *store.Store, remote string, f *rm, in Info, root string) {
			_, state := f.m.paths(in.ID)
			os.WriteFile(filepath.Join(state, "pending.journal"), []byte(`{"op":"t","p":"Other.md"}`+"\n"), 0o644)
		}, "journal"},
		{"a draft", func(t *testing.T, st *store.Store, remote string, f *rm, in Info, root string) {
			if _, err := st.Q().Exec(ctx, `INSERT INTO drafts(space_id, page_id, user_id, base_rev, meta, body, created, updated, held, due) VALUES (?, 'ETHR000001', ?, 'x', '{}', 'typing', 1, 1, 0, 1)`, in.ID, f.user.ID); err != nil {
				t.Fatal(err)
			}
		}, "draft"},
		{"a remote that is not ok", func(t *testing.T, st *store.Store, remote string, f *rm, in Info, root string) {
			if err := store.UpdateRemoteStatus(ctx, st.Q(), in.ID, store.RoleContent, store.RemoteStatus{State: store.RemoteError, ErrorKind: "auth", LastError: "refused"}, f.clk.Now()); err != nil {
				t.Fatal(err)
			}
		}, "remote repository is error"},
		{"a remote with a rewritten history", func(t *testing.T, st *store.Store, remote string, f *rm, in Info, root string) {
			// The remote goes to another history that does not contain the one the
			// space has: a force push by someone else.
			dir := filepath.Join(t.TempDir(), "rw")
			runGit(t, t.TempDir(), "clone", "-q", remote, dir)
			runGit(t, dir, "checkout", "-q", "--orphan", "rewritten")
			os.WriteFile(filepath.Join(dir, "R.md"), []byte(pageFile("RWRT000001", "R", "r\n")), 0o644)
			runGit(t, dir, "add", "-A")
			runGit(t, dir, "commit", "-qm", "new history")
			runGit(t, dir, "push", "-q", "-f", "origin", "rewritten:main")
		}, "rewritten"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			storetest.Each(t, func(t *testing.T, st *store.Store) {
				f := remoteManager(t, st)
				remote := filterBare(t)
				seedRemote(t, remote, 2)
				in, root := f.evictable(remote)
				c.prepare(t, st, remote, f, in, root)
				err := f.m.Evict(ctx, in.ID, EvictOptions{})
				mustBusy(t, err, root, c.why)
				if cs := cacheOf(t, st, in.ID); cs == store.CacheEvicted || cs == store.CacheEvicting {
					t.Fatalf("marked %q", cs)
				}
			})
		})
	}
}

func TestARemoteThatCannotBeReachedIsNotEvictedAndAHeldSpaceIsBusy(t *testing.T) {
	t.Parallel()
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		f := remoteManager(t, st)
		remote := filterBare(t)
		seedRemote(t, remote, 2)
		in, root := f.evictable(remote)
		f.r.setDown(true)
		err := f.m.Evict(ctx, in.ID, EvictOptions{})
		var ue *space.UnavailableError
		if !errors.As(err, &ue) || !exists(root) {
			t.Fatalf("an unreachable remote must stop the eviction: %v", err)
		}
		f.r.setDown(false)
		// A request that holds the space.
		h, err := f.m.Open(ctx, f.user, in.ID)
		if err != nil {
			t.Fatal(err)
		}
		f.idle(72 * time.Hour)
		mustBusy(t, f.m.Evict(ctx, in.ID, EvictOptions{AdminRequest: true}), root, "using it")
		h.Release()
		// Used recently: the idle rule, unless an administrator asks.
		h, _ = f.m.Open(ctx, f.user, in.ID)
		h.Release()
		mustBusy(t, f.m.Evict(ctx, in.ID, EvictOptions{}), root, "used within")
		// A pinned space.
		if err := f.m.SetPinned(ctx, in.ID, true); err != nil {
			t.Fatal(err)
		}
		if err := f.m.Evict(ctx, in.ID, EvictOptions{AdminRequest: true}); !errors.Is(err, ErrNotEvictable) {
			t.Fatalf("%v", err)
		}
		for i := 0; i < 200; i++ { // the background fetch that pinning starts holds the space
			f.m.ev.mu.Lock()
			n := len(f.m.ev.filling)
			f.m.ev.mu.Unlock()
			if n == 0 {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if err := f.m.SetPinned(ctx, in.ID, false); err != nil {
			t.Fatal(err)
		}
		if err := f.m.Evict(ctx, in.ID, EvictOptions{AdminRequest: true}); err != nil {
			t.Fatalf("an administrator's request ignores minIdleHours: %v", err)
		}
	})
}

// A crash at each step of an eviction leaves the whole space or an evicted one that restores.
func TestACrashAtEveryStepOfAnEvictionLeavesAGoodSpace(t *testing.T) {
	t.Parallel()
	steps := []string{"held", "verified", "marked", "moved", "recorded", "removed"}
	for _, step := range steps {
		t.Run(step, func(t *testing.T) {
			storetest.Each(t, func(t *testing.T, st *store.Store) {
				// The hook runs on the goroutine that marks the cache too
				// ("cache:read"), so the step to crash at is shared safely.
				var crash atomic.Value
				crash.Store("")
				f := remoteManager(t, st, func(o *Options) {
					o.EvictHook = func(s string) error {
						if s == crash.Load().(string) {
							return errors.New("crash at " + s)
						}
						return nil
					}
				})
				remote := filterBare(t)
				seedRemote(t, remote, 3)
				in, root := f.evictable(remote)
				crash.Store(step)
				if err := f.m.Evict(ctx, in.ID, EvictOptions{}); err == nil || !strings.Contains(err.Error(), "crash at "+step) {
					t.Fatalf("%v", err)
				}
				// The process goes away here; the next one starts from what is on disk.
				f.m.Close()
				crash.Store("")
				m2 := f.open(serving)
				h, err := m2.Open(ctx, f.user, in.ID)
				if err != nil {
					t.Fatalf("the space does not open after a crash at %s: %v", step, err)
				}
				defer h.Release()
				ids := pageIDs(t, h.Space)
				if len(ids) != 2 {
					t.Fatalf("pages after a crash at %s: %v", step, ids)
				}
				p, err := h.Space.Get("SNAP000001")
				if err != nil || p.Body != "version 3\n" {
					t.Fatalf("%v %v", p, err)
				}
				if !exists(root) {
					t.Fatal("no clone")
				}
				if c := cacheOf(t, st, in.ID); c == store.CacheEvicting || c == store.CacheEvicted {
					t.Fatalf("cache %q after the space is open", c)
				}
				for _, d := range []string{m2.evictDir(), m2.restoringDir()} {
					if ents, _ := os.ReadDir(d); len(ents) != 0 {
						t.Fatalf("%s still holds %d entries", d, len(ents))
					}
				}
			})
		})
	}
}

func TestACrashInTheMiddleOfARestoreLeavesNoHalfClone(t *testing.T) {
	t.Parallel()
	for _, step := range []string{"restore:fetched", "restore:stamped"} {
		t.Run(step, func(t *testing.T) {
			storetest.Each(t, func(t *testing.T, st *store.Store) {
				var crash string
				f := remoteManager(t, st, func(o *Options) {
					o.EvictHook = func(s string) error {
						if s == crash {
							return errors.New("crash at " + s)
						}
						return nil
					}
				})
				remote := filterBare(t)
				seedRemote(t, remote, 2)
				in, root := f.evictable(remote)
				if err := f.m.Evict(ctx, in.ID, EvictOptions{}); err != nil {
					t.Fatal(err)
				}
				crash = step
				_, err := f.m.Open(ctx, f.user, in.ID)
				if err == nil {
					t.Fatal("the restore did not stop")
				}
				if exists(root) {
					t.Fatalf("a half restored clone is in place after a crash at %s", step)
				}
				// The index-only reads still work, and a restart restores it.
				f.m.Close()
				crash = ""
				m2 := f.open(serving)
				ih, err := m2.OpenIndex(ctx, f.user, in.ID)
				if err != nil {
					t.Fatal(err)
				}
				if got := pageIDs(t, ih.Space); len(got) != 2 {
					t.Fatalf("%v", got)
				}
				ih.Release()
				h, err := m2.Open(ctx, f.user, in.ID)
				if err != nil {
					t.Fatal(err)
				}
				defer h.Release()
				if p, err := h.Space.Get("SNAP000001"); err != nil || p.Body != "version 2\n" {
					t.Fatalf("%v %v", p, err)
				}
			})
		})
	}
}

func TestARestoreWaitsThenAnswersRestoringAndGoesOn(t *testing.T) {
	t.Parallel()
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		f := remoteManager(t, st)
		remote := filterBare(t)
		seedRemote(t, remote, 2)
		in, root := f.evictable(remote)
		if err := f.m.Evict(ctx, in.ID, EvictOptions{}); err != nil {
			t.Fatal(err)
		}
		f.m.opt.RestoreWait = 100 * time.Millisecond
		f.r.mu.Lock()
		f.r.hold = make(chan struct{})
		hold := f.r.hold
		f.r.entered = make(chan struct{}, 8)
		f.r.mu.Unlock()
		_, err := f.m.Open(ctx, f.user, in.ID)
		var rs *RestoringError
		if !errors.As(err, &rs) || rs.Phase != "fetching" {
			t.Fatalf("want restoring in the fetching phase, got %v", err)
		}
		if info, _ := f.m.List(ctx, f.user.ID); info[0].Cache != "restoring" {
			t.Fatalf("the space says %q", info[0].Cache)
		}
		close(hold)
		f.r.mu.Lock()
		f.r.hold = nil
		f.r.mu.Unlock()
		deadline := time.Now().Add(10 * time.Second)
		for !exists(root) {
			if time.Now().After(deadline) {
				t.Fatal("the restore did not go on")
			}
			time.Sleep(20 * time.Millisecond)
		}
		h, err := f.m.Open(ctx, f.user, in.ID)
		if err != nil {
			t.Fatal(err)
		}
		h.Release()
	})
}

func TestARestoreThatCannotReachTheRemoteFailsAtOnceAndLeavesNothing(t *testing.T) {
	t.Parallel()
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		f := remoteManager(t, st)
		remote := filterBare(t)
		seedRemote(t, remote, 2)
		in, root := f.evictable(remote)
		if err := f.m.Evict(ctx, in.ID, EvictOptions{}); err != nil {
			t.Fatal(err)
		}
		f.r.setDown(true)
		_, err := f.m.Open(ctx, f.user, in.ID)
		var rf *RestoreFailed
		if !errors.As(err, &rf) || exists(root) {
			t.Fatalf("%v", err)
		}
		if ents, _ := os.ReadDir(f.m.restoringDir()); len(ents) != 0 {
			t.Fatalf("a failed restore left %d entries", len(ents))
		}
		// The index answers meanwhile.
		ih, err := f.m.OpenIndex(ctx, f.user, in.ID)
		if err != nil {
			t.Fatal(err)
		}
		ih.Release()
		f.r.setDown(false)
		time.Sleep(10 * time.Millisecond)
		h, err := f.m.Open(ctx, f.user, in.ID)
		if err != nil {
			t.Fatal(err)
		}
		h.Release()
	})
}

func TestAtMostTwoRestoresRunAtOnce(t *testing.T) {
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		f := remoteManager(t, st)
		remote := filterBare(t)
		seedRemote(t, remote, 1)
		var ids []string
		for i := 0; i < 3; i++ {
			in, h := f.connected(fmt.Sprintf("S%d", i), remote)
			h.Release()
			ids = append(ids, in.ID)
		}
		f.idle(72 * time.Hour)
		for _, id := range ids {
			if err := f.m.Evict(ctx, id, EvictOptions{}); err != nil {
				t.Fatal(err)
			}
		}
		f.m.opt.RestoreWait = 50 * time.Millisecond
		f.r.mu.Lock()
		f.r.hold = make(chan struct{})
		hold := f.r.hold
		f.r.mu.Unlock()
		var wg sync.WaitGroup
		for _, id := range ids {
			wg.Add(1)
			go func() { defer wg.Done(); f.m.Open(ctx, f.user, id) }()
		}
		wg.Wait()
		time.Sleep(100 * time.Millisecond)
		if running, waiting := f.m.RestoreCounts(); running != 2 || waiting != 1 {
			t.Fatalf("running %d, waiting %d", running, waiting)
		}
		close(hold)
		f.r.mu.Lock()
		f.r.hold = nil
		f.r.mu.Unlock()
		deadline := time.Now().Add(15 * time.Second)
		for _, id := range ids {
			for !exists(f.root(id)) {
				if time.Now().After(deadline) {
					t.Fatal("restores did not finish")
				}
				time.Sleep(20 * time.Millisecond)
			}
		}
		f.r.mu.Lock()
		most := f.r.max
		f.r.mu.Unlock()
		if most > 2 {
			t.Fatalf("%d fetches at once", most)
		}
	})
}

// --- the rounds -----------------------------------------------------------------------

func TestARoundEvictsTheSpacesNotUsedForTheConfiguredTimeOnly(t *testing.T) {
	old := DiskFree
	DiskFree = func(string) (uint64, bool) { return 100 << 30, true }
	defer func() { DiskFree = old }()
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		f := remoteManager(t, st)
		remote := filterBare(t)
		seedRemote(t, remote, 1)
		old, oh := f.connected("Old", remote)
		oh.Release()
		f.idle(20 * 24 * time.Hour)
		recent, rh := f.connected("Recent", remote)
		rh.Release()
		pinned, ph := f.connected("Pinned", remote)
		ph.Release()
		if err := f.m.SetPinned(ctx, pinned.ID, true); err != nil {
			t.Fatal(err)
		}
		local, lh := f.space("Local")
		lh.Release()
		// "Old" was opened 20 days ago, the others just now; nothing else is touched.
		time.Sleep(100 * time.Millisecond)
		f.idle(2 * 24 * time.Hour) // Old: 22 days, Recent and Pinned: 2 days, Local too
		if err := f.m.EvictRound(ctx); err != nil {
			t.Fatal(err)
		}
		if exists(f.root(old.ID)) {
			t.Error("a space idle for 22 days was kept")
		}
		for _, id := range []string{recent.ID, pinned.ID, local.ID} {
			if !exists(f.root(id)) {
				t.Errorf("space %s was evicted", id)
			}
		}
		// Never: the setting turns eviction by age off.
		if _, _, err := setStorage(f.m, ctx, func(s *StorageSettings) error { s.Cache.EvictAfterDays = nil; return nil }); err != nil {
			t.Fatal(err)
		}
		f.idle(400 * 24 * time.Hour)
		if err := f.m.EvictRound(ctx); err != nil {
			t.Fatal(err)
		}
		for _, id := range []string{recent.ID, pinned.ID, local.ID} {
			if !exists(f.root(id)) {
				t.Errorf("space %s was evicted with eviction by age off", id)
			}
		}
	})
}

func TestADiskThatIsShortEvictsLeastRecentlyUsedFirstButNeverWhatWasUsedRecently(t *testing.T) {
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		f := remoteManager(t, st)
		remote := filterBare(t)
		seedRemote(t, remote, 1)
		a, ah := f.connected("A", remote)
		ah.Release()
		f.idle(3 * time.Hour)
		b, bh := f.connected("B", remote)
		bh.Release()
		f.idle(3 * time.Hour)
		c, ch := f.connected("C", remote)
		ch.Release()
		time.Sleep(100 * time.Millisecond)
		f.idle(30 * time.Hour) // all are past minIdleHours (24 h)
		free := uint64(1 << 20)
		old := DiskFree
		DiskFree = func(string) (uint64, bool) { return free, true }
		defer func() { DiskFree = old }()
		calls := 0
		DiskFree = func(string) (uint64, bool) {
			calls++
			if calls > 2 { // the second eviction frees enough
				return 20 << 30, true
			}
			return free, true
		}
		if err := f.m.EvictRound(ctx); err != nil {
			t.Fatal(err)
		}
		if exists(f.root(a.ID)) || exists(f.root(b.ID)) || !exists(f.root(c.ID)) {
			t.Fatalf("a=%v b=%v c=%v: the least recently used go first, and only while the disk is short", exists(f.root(a.ID)), exists(f.root(b.ID)), exists(f.root(c.ID)))
		}
		// A space used within minIdleHours is not evicted, whatever the disk says.
		h, _ := f.m.Open(ctx, f.user, c.ID)
		h.Release()
		DiskFree = func(string) (uint64, bool) { return 1, true }
		if err := f.m.EvictRound(ctx); err != nil {
			t.Fatal(err)
		}
		if !exists(f.root(c.ID)) {
			t.Fatal("a space that was used a moment ago was evicted")
		}
	})
}

func TestTheIdleSpacesAreOpenedByIndexAfterARestartToo(t *testing.T) {
	t.Parallel()
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		f := remoteManager(t, st)
		remote := filterBare(t)
		seedRemote(t, remote, 1)
		in, _ := f.evictable(remote)
		if err := f.m.Evict(ctx, in.ID, EvictOptions{}); err != nil {
			t.Fatal(err)
		}
		f.m.Close()
		m2 := f.open(serving)
		if m2.CacheState(store.CacheEvicted, true, in.ID) != "evicted" {
			t.Fatal()
		}
		ih, err := m2.OpenIndex(ctx, f.user, in.ID)
		if err != nil || !ih.IndexOnly {
			t.Fatalf("%v", err)
		}
		ih.Release()
	})
}

// --- the requirement of a remote --------------------------------------------------------

func setRequire(t *testing.T, m *Manager, require string, scope string, grace int) {
	t.Helper()
	_, _, err := setStorage(m, ctx, func(s *StorageSettings) error {
		if require == "" {
			s.Remotes.Require = nil
		} else {
			s.Remotes.Require = &require
		}
		s.Remotes.Scope, s.Remotes.GraceDays = scope, grace
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTheRequirementOfARemoteAndItsGracePeriod(t *testing.T) {
	t.Parallel()
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		f := remoteManager(t, st)
		in, h := f.space("Notes")
		defer h.Release()
		// Nothing is required: writes work.
		if _, err := h.Space.Create(space.Author{}, space.CreateInput{Meta: page.Meta{{Key: "title", Value: "A"}}}); err != nil {
			t.Fatal(err)
		}
		setRequire(t, f.m, "any", "all", 30)
		n, err := f.m.Notice(ctx, in.ID)
		if err != nil || n == nil || n.Overdue || n.Required != "any" {
			t.Fatalf("%+v %v", n, err)
		}
		if want := f.clk.Now().Add(30 * 24 * time.Hour); !n.Deadline.Equal(want.UTC().Truncate(time.Millisecond)) {
			t.Fatalf("the deadline is since + graceDays: %v, want %v", n.Deadline, want)
		}
		// In the grace period nothing changes but the notice.
		if _, err := h.Space.Create(space.Author{}, space.CreateInput{Meta: page.Meta{{Key: "title", Value: "B"}}}); err != nil {
			t.Fatal(err)
		}
		if l, _ := f.m.List(ctx, f.user.ID); l[0].RemoteNotice == nil {
			t.Fatal("the list does not carry the notice")
		}
		// After it, every change to the files answers remote_required; reads do not.
		f.clk.Advance(31 * 24 * time.Hour)
		_, err = h.Space.Create(space.Author{}, space.CreateInput{Meta: page.Meta{{Key: "title", Value: "C"}}})
		var re *RequiredError
		if !errors.As(err, &re) || re.Require != "any" || re.Deadline.IsZero() {
			t.Fatalf("want remote_required with the deadline, got %v", err)
		}
		if _, err := h.Space.CreateFolder(space.Author{}, "Folder"); !errors.As(err, &re) {
			t.Fatalf("a folder: %v", err)
		}
		if res, err := h.Space.List(index.Query{Recursive: true}); err != nil || len(res.Items) != 2 {
			t.Fatalf("reads work: %v %v", res, err)
		}
		// Nothing was deleted.
		if !exists(f.root(in.ID)) {
			t.Fatal("the space is gone")
		}
		// Connecting always works, and then the space is writable again.
		remote := filterBare(t)
		if _, err := f.m.Connect(ctx, h, fileTokenReq(remote)); err != nil {
			t.Fatal(err)
		}
		if _, err := h.Space.Create(space.Author{}, space.CreateInput{Meta: page.Meta{{Key: "title", Value: "D"}}}); err != nil {
			t.Fatalf("writable after connecting: %v", err)
		}
		// Removing the remote is refused while the requirement stands.
		if err := f.m.Disconnect(ctx, h); !errors.As(err, &re) {
			t.Fatalf("removing the remote: %v", err)
		}
		// Relaxing the requirement clears everything.
		setRequire(t, f.m, "", "new", 30)
		if n, _ := f.m.Notice(ctx, in.ID); n != nil {
			t.Fatalf("%+v", n)
		}
	})
}

// A space that loses its GitHub remote has its grace period again from that
// moment, in its notice (and so its deadline everywhere it is shown) and in
// whether it is read only; a remote of the right kind again ends it.
func TestASpaceThatLostItsGitHubRemoteHasItsGraceAgain(t *testing.T) {
	t.Parallel()
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		f := remoteManager(t, st)
		in, h := f.space("Notes")
		defer h.Release()
		setRequire(t, f.m, "any", "all", 30)
		now := f.clk.Now()
		if err := store.PutGitHubApp(ctx, st.Q(), store.GitHubApp{App: `{"appId":"1","fingerprint":"f"}`, ClientID: "Iv1.x", BaseURL: "https://github.com"}, now); err != nil {
			t.Fatal(err)
		}
		if err := store.ClaimGitHubInstallation(ctx, st.Q(), store.GitHubInstallation{ID: 5, Account: "me", AccountType: "User", OwnerUser: f.user.ID, Created: now, Updated: now}); err != nil {
			t.Fatal(err)
		}
		if err := store.PutRemote(ctx, st.Q(), store.Remote{SpaceID: in.ID, URL: "https://github.com/me/notes.git", Branch: "main", State: "ok", Updated: now,
			CredType: "connection", Provider: "github", Via: "app", InstallationID: 5, RepoID: 1, CredUserID: f.user.ID}); err != nil {
			t.Fatal(err)
		}
		f.m.PolicyChanged()
		f.clk.Advance(90 * 24 * time.Hour) // far past what the requirement gave it
		if n, err := f.m.Notice(ctx, in.ID); err != nil || n != nil {
			t.Fatalf("with its remote: %+v %v", n, err)
		}
		lostAt := f.clk.Now()
		ids, err := store.DisconnectGitHubInstallation(ctx, st.Q(), 5, store.InstallOwner{UserID: f.user.ID}, "lost", "any", lostAt)
		if err != nil || len(ids) != 1 {
			t.Fatalf("%v %v", ids, err)
		}
		f.m.AccessLost(ids)
		n, err := f.m.Notice(ctx, in.ID)
		if err != nil || n == nil || n.Overdue || !n.Deadline.Equal(lostAt.Add(30*24*time.Hour).Truncate(time.Millisecond)) {
			t.Fatalf("just after the loss: %+v %v", n, err)
		}
		if err := f.m.CheckWritable(in.ID); err != nil {
			t.Fatalf("read only at once: %v", err)
		}
		if l, _ := f.m.List(ctx, f.user.ID); len(l) != 1 || l[0].RemoteNotice == nil || l[0].RemoteNotice.Overdue {
			t.Fatalf("the list: %+v", l)
		}
		// At the deadline itself it is still in grace, as the lists say.
		f.clk.Advance(30 * 24 * time.Hour)
		if n, _ := f.m.Notice(ctx, in.ID); n == nil || n.Overdue {
			t.Fatalf("at the deadline: %+v", n)
		}
		if err := f.m.CheckWritable(in.ID); err != nil {
			t.Fatalf("read only at the deadline: %v", err)
		}
		f.clk.Advance(time.Millisecond)
		if n, _ := f.m.Notice(ctx, in.ID); n == nil || !n.Overdue {
			t.Fatalf("after the grace: %+v", n)
		}
		var re *RequiredError
		if err := f.m.CheckWritable(in.ID); !errors.As(err, &re) {
			t.Fatalf("after the grace: %v", err)
		}
	})
}

func TestARequirementOfGitHubRefusesAnyOtherCredentialAndTightening(t *testing.T) {
	t.Parallel()
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		f := remoteManager(t, st)
		// No GitHub App on this server.
		_, _, err := setStorage(f.m, ctx, func(s *StorageSettings) error { r := "github"; s.Remotes.Require = &r; return nil })
		if err == nil {
			t.Fatal("github can be required without an App")
		}
		in, h := f.space("Notes")
		defer h.Release()
		remote := filterBare(t)
		if _, err := f.m.Connect(ctx, h, fileTokenReq(remote)); err != nil {
			t.Fatal(err)
		}
		// With "any", a token is fine; tightening to "any" -> new since.
		setRequire(t, f.m, "any", "all", 10)
		s1 := f.m.Storage().Remotes.Since
		f.clk.Advance(time.Hour)
		setRequire(t, f.m, "any", "all", 20) // graceDays only: since does not move
		if s2 := f.m.Storage().Remotes.Since; !s2.Equal(s1) {
			t.Fatalf("a new grace period moved since: %v -> %v", s1, s2)
		}
		f.clk.Advance(time.Hour)
		setRequire(t, f.m, "any", "new", 20) // narrowed: the spaces it covered stay covered, since stays
		if s3 := f.m.Storage().Remotes.Since; !s3.Equal(s1) {
			t.Fatalf("narrowing the scope moved since: %v -> %v", s1, s3)
		}
		f.clk.Advance(time.Hour)
		setRequire(t, f.m, "any", "all", 20) // widened: a new since
		if s4 := f.m.Storage().Remotes.Since; !s4.After(s1) {
			t.Fatalf("widening the scope kept since: %v", s4)
		}
		_ = in
	})
}

func TestNewSpacesNeedWhatTheRequirementAsksAndExemptionsAreLeftOut(t *testing.T) {
	t.Parallel()
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		f := remoteManager(t, st)
		setRequire(t, f.m, "any", "new", 30)
		var re *RequiredError
		if _, err := f.m.Create(ctx, f.user.ID, "Local"); !errors.As(err, &re) || re.Require != "any" || !re.Deadline.IsZero() {
			t.Fatalf("a new local space under the requirement: %v", err)
		}
		if n := countSpaces(t, st); n != 0 {
			t.Fatalf("a refused space was made: %d", n)
		}
		// A space made with a remote is fine.
		remote := filterBare(t)
		seedRemote(t, remote, 1)
		in, err := f.m.CreateWithRemote(ctx, f.user.ID, "Remote", "", fileTokenReq(remote))
		if err != nil || in.Sync != "ok" {
			t.Fatalf("%+v %v", in, err)
		}
		// An exempt user is not asked.
		if err := store.SetRemoteExempt(ctx, st.Q(), f.user.ID, true); err != nil {
			t.Fatal(err)
		}
		f.m.PolicyChanged()
		if _, err := f.m.Create(ctx, f.user.ID, "Exempt"); err != nil {
			t.Fatalf("an exempt user: %v", err)
		}
		if err := store.SetRemoteExempt(ctx, st.Q(), f.user.ID, false); err != nil {
			t.Fatal(err)
		}
		f.m.PolicyChanged()
		if _, err := f.m.Create(ctx, f.user.ID, "Again"); !errors.As(err, &re) {
			t.Fatalf("%v", err)
		}
	})
}

func countSpaces(t *testing.T, st *store.Store) int {
	t.Helper()
	n, err := store.CountSpaces(ctx, st.Q(), store.AllTenants())
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestASpaceExemptFromTheRequirementIsNeverReadOnly(t *testing.T) {
	t.Parallel()
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		f := remoteManager(t, st)
		in, h := f.space("Notes")
		defer h.Release()
		setRequire(t, f.m, "any", "all", 0)
		f.clk.Advance(time.Hour)
		var re *RequiredError
		if _, err := h.Space.Create(space.Author{}, space.CreateInput{Meta: page.Meta{{Key: "title", Value: "A"}}}); !errors.As(err, &re) {
			t.Fatalf("%v", err)
		}
		aff, _ := f.m.Affected(ctx)
		if aff.Overdue != 1 {
			t.Fatalf("%+v", aff)
		}
		if err := f.m.SetExempt(ctx, in.ID, true); err != nil {
			t.Fatal(err)
		}
		if _, err := h.Space.Create(space.Author{}, space.CreateInput{Meta: page.Meta{{Key: "title", Value: "A"}}}); err != nil {
			t.Fatalf("an exempt space: %v", err)
		}
		if aff, _ := f.m.Affected(ctx); aff.Overdue != 0 || aff.Exempt != 1 {
			t.Fatalf("%+v", aff)
		}
	})
}

func TestALimitOnLocalSpacesRefusesUploadsOnlyAndNotRemoteSpaces(t *testing.T) {
	t.Parallel()
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		f := remoteManager(t, st)
		in, h := f.space("Notes")
		defer h.Release()
		if _, _, err := setStorage(f.m, ctx, func(s *StorageSettings) error { v := int64(1 << 20); s.LocalSpaces.MaxBytes = &v; return nil }); err != nil {
			t.Fatal(err)
		}
		if err := f.m.CheckRoom(ctx, h, 10); err != nil {
			t.Fatalf("a small upload: %v", err)
		}
		var full *SpaceFullError
		if err := f.m.CheckRoom(ctx, h, 2<<20); !errors.As(err, &full) {
			t.Fatalf("a big upload: %v", err)
		}
		// Saving text is never refused.
		if _, err := h.Space.Create(space.Author{}, space.CreateInput{Meta: page.Meta{{Key: "title", Value: "T"}}, Body: strings.Repeat("x", 1<<19)}); err != nil {
			t.Fatal(err)
		}
		// A space that follows a remote is not limited by it.
		remote := filterBare(t)
		if _, err := f.m.Connect(ctx, h, fileTokenReq(remote)); err != nil {
			t.Fatal(err)
		}
		hh, _ := f.m.Open(ctx, f.user, in.ID)
		defer hh.Release()
		if err := f.m.CheckRoom(ctx, hh, 2<<20); err != nil {
			t.Fatalf("a remote space: %v", err)
		}
	})
}

func TestTheSettingsAreCheckedAndStoredAndTheFlagWinsForHosts(t *testing.T) {
	t.Parallel()
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		f := remoteManager(t, st)
		bad := []StoragePatch{
			func(s *StorageSettings) error { r := "gitlab"; s.Remotes.Require = &r; return nil },
			func(s *StorageSettings) error { s.Remotes.GraceDays = 366; return nil },
			func(s *StorageSettings) error { v := int64(10); s.LocalSpaces.MaxBytes = &v; return nil },
			func(s *StorageSettings) error { v := 0; s.Cache.EvictAfterDays = &v; return nil },
			func(s *StorageSettings) error { s.Cache.MinIdleHours = 721; return nil },
			func(s *StorageSettings) error { v := int64(1 << 20); s.Cache.ClonesBytes = &v; return nil },
			func(s *StorageSettings) error { s.Remotes.Scope = "some"; return nil },
		}
		for i, p := range bad {
			if _, _, err := setStorage(f.m, ctx, p); !errors.Is(err, ErrInvalid) {
				t.Errorf("case %d: %v", i, err)
			}
		}
		set, _, err := setStorage(f.m, ctx, func(s *StorageSettings) error {
			s.Cache.MinIdleHours = 48
			s.Hosts.Allowed = []string{"Example.Test"}
			return nil
		})
		if err != nil || set.Cache.MinIdleHours != 48 || set.Hosts.Allowed[0] != "example.test" {
			t.Fatalf("%+v %v", set, err)
		}
		// Stored: a new manager reads it.
		f.m.Close()
		m2 := f.open()
		if got := m2.Storage(); got.Cache.MinIdleHours != 48 || got.Hosts.Allowed[0] != "example.test" {
			t.Fatalf("%+v", got)
		}
		m2.Close()
		// With the flag, the flag's hosts are the ones in force.
		m3 := f.open(func(o *Options) { o.HostsFromFlag = true; o.RemotePolicy.Hosts = []string{"flag.test"} })
		if got := m3.Storage().Hosts.Allowed; len(got) != 1 || got[0] != "flag.test" || !m3.HostsFromFlag() {
			t.Fatalf("%v", got)
		}
	})
}

func TestPartialCloneNeedsGit244(t *testing.T) {
	t.Parallel()
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		no := false
		f := remoteManager(t, st, func(o *Options) { o.GitSupportsPartial = &no })
		if _, partial := f.m.GitInfo(); partial || f.m.Storage().Cache.Partial {
			t.Fatal("partial clones are on without Git 2.44")
		}
		if _, _, err := setStorage(f.m, ctx, func(s *StorageSettings) error { s.Cache.Partial = true; return nil }); !errors.Is(err, ErrGitTooOld) {
			t.Fatalf("%v", err)
		}
		// Eviction still works, and the restore makes a full clone.
		remote := filterBare(t)
		seedRemote(t, remote, 2)
		in, _ := f.evictable(remote)
		if err := f.m.Evict(ctx, in.ID, EvictOptions{}); err != nil {
			t.Fatal(err)
		}
		h, err := f.m.Open(ctx, f.user, in.ID)
		if err != nil {
			t.Fatal(err)
		}
		defer h.Release()
		if h.Space.Partial() {
			t.Fatal("restored partially without Git 2.44")
		}
	})
}

func TestPinningFetchesEveryVersionAndKeepsTheSpace(t *testing.T) {
	t.Parallel()
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		f := remoteManager(t, st)
		remote := filterBare(t)
		seedRemote(t, remote, 3)
		in, h := f.connected("Notes", remote)
		defer h.Release()
		if err := f.m.SetPinned(ctx, in.ID, true); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(10 * time.Second)
		for {
			out := runGit(t, f.root(in.ID), "rev-list", "--objects", "--missing=print", "HEAD")
			if !strings.Contains(out, "?") {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("the clone did not become full")
			}
			time.Sleep(50 * time.Millisecond)
		}
		_ = gitstore.DefaultHosts
	})
}

var _ = context.Background

// --- the record of what a space's copy is ----------------------------------------------

// The record that a clone is partial or full is written after the space opens,
// in the background; an eviction that marks the space in between wins.
func TestRecordingTheCloneNeverUndoesAnEviction(t *testing.T) {
	t.Parallel()
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		var f *rm
		var id string
		f = remoteManager(t, st, func(o *Options) {
			o.EvictHook = func(step string) error {
				if step == "cache:read" && id != "" {
					// An eviction marks the space between the read and the write.
					if err := store.SetSpaceCache(ctx, st.Q(), id, store.CacheEvicting); err != nil {
						t.Error(err)
					}
				}
				return nil
			}
		})
		remote := filterBare(t)
		seedRemote(t, remote, 2)
		in, _ := f.evictable(remote)
		time.Sleep(100 * time.Millisecond) // the record of the open is written
		if err := store.SetSpaceCache(ctx, st.Q(), in.ID, ""); err != nil {
			t.Fatal(err)
		}
		id = in.ID
		f.m.recordCache(in.ID, store.CachePartial)
		if c := cacheOf(t, st, in.ID); c != store.CacheEvicting {
			t.Fatalf("the record of the clone overwrote an eviction: %q", c)
		}
	})
}

// A space that follows a remote and has no clone on disk is an evicted one,
// whatever its record says (a record overwritten, a directory removed by
// hand): it is restored, not refused.
func TestASpaceWithARemoteAndNoCloneIsRestored(t *testing.T) {
	t.Parallel()
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		f := remoteManager(t, st)
		remote := filterBare(t)
		seedRemote(t, remote, 2)
		in, root := f.evictable(remote)
		f.m.Close()
		if err := os.RemoveAll(root); err != nil {
			t.Fatal(err)
		}
		if err := store.SetSpaceCache(ctx, st.Q(), in.ID, store.CachePartial); err != nil {
			t.Fatal(err)
		}
		m2 := f.open(serving)
		if c := cacheOf(t, st, in.ID); c != store.CacheEvicted {
			t.Fatalf("a space with a remote and no clone is recorded %q at the start", c)
		}
		h, err := m2.Open(ctx, f.user, in.ID)
		if err != nil {
			t.Fatal(err)
		}
		defer h.Release()
		if p, err := h.Space.Get("SNAP000001"); err != nil || p.Body != "version 2\n" {
			t.Fatalf("%v %v", p, err)
		}
	})
}

// A space recorded as evicted whose clone is in place (the record of a restore
// was not written) is recorded as what its clone is: a partial clone is partial.
func TestARestoredCloneRecordedAsEvictedIsRecordedAsWhatItIs(t *testing.T) {
	t.Parallel()
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		f := remoteManager(t, st)
		remote := filterBare(t)
		seedRemote(t, remote, 2)
		in, root := f.evictable(remote)
		if err := f.m.Evict(ctx, in.ID, EvictOptions{}); err != nil {
			t.Fatal(err)
		}
		h, err := f.m.Open(ctx, f.user, in.ID) // restored, as a partial clone
		if err != nil {
			t.Fatal(err)
		}
		h.Release()
		f.m.Close()
		if !exists(root) {
			t.Fatal("not restored")
		}
		if err := store.SetSpaceCache(ctx, st.Q(), in.ID, store.CacheEvicted); err != nil {
			t.Fatal(err)
		}
		f.open(serving)
		if c := cacheOf(t, st, in.ID); c != store.CachePartial {
			t.Fatalf("a partial clone in place is recorded %q", c)
		}
	})
}

// The same while the server runs: a space that follows a remote, whose clone
// is not on disk and is not recorded as evicted, is restored when it is opened.
func TestOpeningASpaceWithARemoteAndNoCloneRestoresIt(t *testing.T) {
	t.Parallel()
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		f := remoteManager(t, st)
		remote := filterBare(t)
		seedRemote(t, remote, 2)
		in, root := f.evictable(remote)
		f.m.Close()
		if err := os.RemoveAll(root); err != nil {
			t.Fatal(err)
		}
		m2 := f.open() // no clean-up at the start: the space is found missing when opened
		h, err := m2.Open(ctx, f.user, in.ID)
		if err != nil {
			t.Fatalf("a space with a remote and no clone is refused: %v", err)
		}
		defer h.Release()
		if p, err := h.Space.Get("SNAP000001"); err != nil || p.Body != "version 2\n" {
			t.Fatalf("%v %v", p, err)
		}
	})
}

// The administration reaches a disabled space without restoring it; whether
// the space is disabled is read under the lock that guards it (run with -race).
func TestAnAdministratorOpeningAnEvictedSpaceReadsDisabledUnderTheLock(t *testing.T) {
	t.Parallel()
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		f := remoteManager(t, st)
		remote := filterBare(t)
		seedRemote(t, remote, 1)
		in, _ := f.evictable(remote)
		if err := f.m.Evict(ctx, in.ID, EvictOptions{}); err != nil {
			t.Fatal(err)
		}
		if err := f.m.loadDisabled(ctx); err != nil {
			t.Fatal(err)
		}
		f.r.setDown(true) // every restore fails at once
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				f.m.markDisabled(in.ID, i%2 == 0)
			}
		}()
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				if _, release, err := f.m.acquire(ctx, in.ID, true); err == nil {
					release()
				}
			}
		}()
		wg.Wait()
	})
}

// A restore closes the index-only handle of the space before the full space
// opens the index. A request that holds it longer than the restore waits
// leaves it open until that request releases it, but nothing new is given it.
func TestAnIndexOnlyHandleHeldThroughARestoreIsClosedOnItsLastRelease(t *testing.T) {
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		f := remoteManager(t, st)
		remote := filterBare(t)
		seedRemote(t, remote, 1)
		in, _ := f.evictable(remote)
		if err := f.m.Evict(ctx, in.ID, EvictOptions{}); err != nil {
			t.Fatal(err)
		}
		old := indexCloseWait
		indexCloseWait = 50 * time.Millisecond
		defer func() { indexCloseWait = old }()
		held, release, err := f.m.acquireIndex(ctx, in.ID)
		if err != nil || !held.Evicted() {
			t.Fatalf("%v", err)
		}
		f.m.closeIndexOnly(in.ID) // gives up waiting
		f.m.ev.mu.Lock()
		e := f.m.ev.idx[in.ID]
		f.m.ev.mu.Unlock()
		if e != nil && e.sp == held {
			t.Fatal("a handle that is being closed is still handed out")
		}
		if _, err := held.List(index.Query{Recursive: true}); err != nil {
			t.Fatalf("the holder's handle was closed under it: %v", err)
		}
		release()
		if _, err := held.List(index.Query{Recursive: true}); err == nil {
			t.Fatal("the stale handle is not closed on its last release")
		}
		// The next request is given a new one.
		sp, release2, err := f.m.acquireIndex(ctx, in.ID)
		if err != nil || sp == held {
			t.Fatalf("%v", err)
		}
		if got := pageIDs(t, sp); len(got) != 2 {
			t.Fatalf("%v", got)
		}
		release2()
	})
}

// A remote that hangs does not hold a restore's turn for long: each restore
// has a deadline, and a space whose restore failed is not tried again at once
// (requests meanwhile answer unavailable, with when to try again).
func TestAFailedRestoreIsRememberedAndAHangingOneEnds(t *testing.T) {
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		f := remoteManager(t, st, func(o *Options) { o.FailFast = time.Hour })
		remote := filterBare(t)
		seedRemote(t, remote, 1)
		in, root := f.evictable(remote)
		if err := f.m.Evict(ctx, in.ID, EvictOptions{}); err != nil {
			t.Fatal(err)
		}
		f.m.opt.RestoreWait, f.m.opt.RestoreDeadline = 50*time.Millisecond, 300*time.Millisecond
		hold := make(chan struct{})
		f.r.mu.Lock()
		f.r.hold = hold
		f.r.mu.Unlock()
		defer close(hold)
		var rs *RestoringError
		if _, err := f.m.Open(ctx, f.user, in.ID); !errors.As(err, &rs) {
			t.Fatalf("want restoring, got %v", err)
		}
		deadline := time.Now().Add(5 * time.Second)
		for {
			if running, waiting := f.m.RestoreCounts(); running == 0 && waiting == 0 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("a restore from a remote that hangs does not end")
			}
			time.Sleep(20 * time.Millisecond)
		}
		fetches := f.r.fetchCount()
		_, err := f.m.Open(ctx, f.user, in.ID)
		var rf *RestoreFailed
		if !errors.As(err, &rf) || rf.RetryAfter <= 0 || rf.RetryAfter > time.Hour {
			t.Fatalf("want unavailable with a time to try again, got %v", err)
		}
		if f.r.fetchCount() != fetches {
			t.Fatal("a restore that failed a moment ago was started again")
		}
		if exists(root) {
			t.Fatal("a half clone is in place")
		}
		// The index answers meanwhile.
		ih, err := f.m.OpenIndex(ctx, f.user, in.ID)
		if err != nil {
			t.Fatal(err)
		}
		ih.Release()
	})
}

// A pinned space is made full once, and stays full: the pulls after that bring
// every version, not only the commits.
func TestAPinnedSpaceStaysFullAfterLaterPulls(t *testing.T) {
	t.Parallel()
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		f := remoteManager(t, st)
		remote := filterBare(t)
		seedRemote(t, remote, 3)
		in, h := f.connected("Notes", remote)
		defer h.Release()
		if !h.Space.Partial() {
			t.Skip("the server's Git makes no partial clones")
		}
		if err := f.m.SetPinned(ctx, in.ID, true); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(10 * time.Second)
		for cacheOf(t, st, in.ID) != store.CacheFull {
			if time.Now().After(deadline) {
				t.Fatalf("the pinned space is recorded %q", cacheOf(t, st, in.ID))
			}
			time.Sleep(50 * time.Millisecond)
		}
		// New versions on the remote, then a pull.
		for i := 4; i <= 6; i++ {
			push(t, remote, "Tang/Snow.md", pageFile("SNAP000001", "Snow", fmt.Sprintf("version %d\n", i)))
		}
		if err := h.Space.Pull(ctx, true); err != nil {
			t.Fatal(err)
		}
		if p, err := h.Space.Get("SNAP000001"); err != nil || p.Body != "version 6\n" {
			t.Fatalf("not pulled: %v %v", p, err)
		}
		root := f.root(in.ID)
		if out := runGit(t, root, "rev-list", "--objects", "--missing=print", "--all"); strings.Contains(out, "\n?") || strings.HasPrefix(out, "?") {
			t.Fatal("a pull of a pinned space left versions on the remote only")
		}
		if h.Space.Partial() {
			t.Fatal("the pinned space is still partial")
		}
		// Every version reads with the remote down.
		f.r.setDown(true)
		items, _, err := h.Space.History(ctx, "SNAP000001", 30, "")
		if err != nil || len(items) != 6 {
			t.Fatalf("%d versions, %v", len(items), err)
		}
	})
}

// Making a partial clone full stops the filter first and leaves partial mode
// last. Cut off in between (the remote goes away, the process ends), the clone
// still lacks old versions, so it must still read as partial when it opens
// again: they are fetched when asked for, and a retried disconnect fetches the
// rest before the space becomes local.
func TestAnInterruptedMakeFullLeavesAPartialCloneThatIsRepairedLater(t *testing.T) {
	t.Parallel()
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		f := remoteManager(t, st)
		remote := filterBare(t)
		seedRemote(t, remote, 3)
		in, h := f.connected("Notes", remote)
		if !h.Space.Partial() {
			t.Fatal("not a partial clone")
		}
		root := f.root(in.ID)
		f.r.setDown(true)
		if err := f.m.Disconnect(ctx, h); err == nil {
			t.Fatal("a disconnect that cannot fetch the old versions succeeded")
		}
		// The filter was stopped; old versions are still missing.
		if cfg, _ := os.ReadFile(filepath.Join(root, ".git", "config")); strings.Contains(string(cfg), "partialclonefilter") {
			t.Fatalf("the make-full did not get past stopping the filter:\n%s", cfg)
		}
		if out := runGit(t, root, "rev-list", "--objects", "--missing=print", "--all"); !strings.Contains(out, "?") {
			t.Fatal("nothing is missing; the test does not interrupt anything")
		}
		if !h.Space.Partial() {
			t.Fatal("the open space left partial mode with versions missing")
		}
		h.Release()
		// The process goes away; the next one starts from what is on disk.
		f.m.Close()
		f.m = f.open(serving)
		h2, err := f.m.Open(ctx, f.user, in.ID)
		if err != nil {
			t.Fatal(err)
		}
		defer h2.Release()
		if !h2.Space.Partial() {
			t.Fatal("a clone that lacks old versions reopened as a full one")
		}
		f.r.setDown(false)
		time.Sleep(10 * time.Millisecond)
		items, _, err := h2.Space.History(ctx, "SNAP000001", 30, "")
		if err != nil || len(items) != 3 {
			t.Fatalf("%d versions, %v", len(items), err)
		}
		if v, err := h2.Space.Version(ctx, "SNAP000001", items[2].Commit); err != nil || v.Body != "version 1\n" {
			t.Fatalf("%v %v", v, err)
		}
		// The retried disconnect makes the clone whole, then local.
		if err := f.m.Disconnect(ctx, h2); err != nil {
			t.Fatal(err)
		}
		if out := runGit(t, root, "rev-list", "--objects", "--missing=print", "--all"); strings.Contains(out, "?") {
			t.Fatalf("the space became local with objects missing:\n%s", out)
		}
		if h2.Space.Partial() {
			t.Fatal("a local space is partial")
		}
		cfg, _ := os.ReadFile(filepath.Join(root, ".git", "config"))
		for _, k := range []string{"promisor", "partialclone"} {
			if strings.Contains(string(cfg), k) {
				t.Fatalf("%s is still configured:\n%s", k, cfg)
			}
		}
		items, _, err = h2.Space.History(ctx, "SNAP000001", 30, "")
		if err != nil || len(items) != 3 {
			t.Fatalf("%d versions after the disconnect, %v", len(items), err)
		}
		if v, err := h2.Space.Version(ctx, "SNAP000001", items[2].Commit); err != nil || v.Body != "version 1\n" {
			t.Fatalf("%v %v", v, err)
		}
	})
}

// 3.3's hook: only a measurement of a whole copy writes what the quotas count.
// A partial clone is measured for the list, and its usage keeps the last full
// measurement, so it never shrinks what its owner uses.
func TestMeasuringAPartialCloneLeavesItsUsageAlone(t *testing.T) {
	t.Parallel()
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		f := remoteManager(t, st)
		remote := filterBare(t)
		seedRemote(t, remote, 3)
		in, h := f.connected("Notes", remote)
		defer h.Release()
		deadline := time.Now().Add(5 * time.Second)
		for cacheOf(t, st, in.ID) != store.CachePartial {
			if time.Now().After(deadline) {
				t.Fatal("the clone was not recorded as partial")
			}
			time.Sleep(20 * time.Millisecond)
		}
		const before = int64(123456789) // as a full measurement left it
		if _, err := st.Q().Exec(ctx, `UPDATE spaces SET usage_bytes = ? WHERE id = ?`, before, in.ID); err != nil {
			t.Fatal(err)
		}
		n, _, err := f.m.Resize(ctx, store.AllTenants(), in.ID)
		if err != nil || n <= 0 {
			t.Fatalf("%d %v", n, err)
		}
		var usage, size int64
		if err := st.Q().QueryRow(ctx, `SELECT usage_bytes, size_bytes FROM spaces WHERE id = ?`, in.ID).Scan(&usage, &size); err != nil {
			t.Fatal(err)
		}
		if usage != before || size != n {
			t.Fatalf("usage %d (want %d kept), listing size %d (want %d)", usage, before, size, n)
		}
	})
}

// --- nothing runs on after the manager is closed -----------------------------------------

// The measurement that follows a restore runs in the background. Close waits
// for it (and stops it), so nothing is written into the data directory once
// the manager is closed and the directory may be removed.
func TestNothingWritesIntoTheDataDirAfterClose(t *testing.T) {
	t.Parallel()
	storetest.Each(t, func(t *testing.T, st *store.Store) {
		var late atomic.Bool // the late measurement has gone by
		f := remoteManager(t, st, func(o *Options) {
			o.EvictHook = func(step string) error {
				if step == "resize:start" {
					time.Sleep(300 * time.Millisecond) // the measurement is late
					late.Store(true)
				}
				return nil
			}
		})
		remote := filterBare(t)
		seedRemote(t, remote, 2)
		in, _ := f.evictable(remote)
		if err := f.m.Evict(ctx, in.ID, EvictOptions{}); err != nil {
			t.Fatal(err)
		}
		h, err := f.m.Open(ctx, f.user, in.ID) // restores it: the measurement starts
		if err != nil {
			t.Fatal(err)
		}
		h.Release()
		_, state := f.m.paths(in.ID)
		f.m.Close()
		if !late.Load() {
			t.Fatal("Close returned while a background measurement was still to run")
		}
		if err := os.RemoveAll(f.dir); err != nil {
			t.Fatal(err)
		}
		time.Sleep(600 * time.Millisecond)
		if exists(f.dir) || exists(state) {
			t.Fatalf("something wrote into the data directory after Close: %s", f.dir)
		}
	})
}

func TestSyncDirReportsItsFailure(t *testing.T) {
	t.Parallel()
	if err := syncDir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := syncDir(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("a directory that cannot be opened was reported as synced")
	}
}
