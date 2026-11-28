// Package spaces knows which spaces exist, who is a member of them, and opens
// them: lazily, once, and for good until shutdown. In single-user mode it
// fronts the one space the server was started with.
package spaces

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"petalbook.app/server/internal/authz"
	"petalbook.app/server/internal/connections"
	"petalbook.app/server/internal/gitstore"
	"petalbook.app/server/internal/jobs"
	"petalbook.app/server/internal/page"
	"petalbook.app/server/internal/perm"
	"petalbook.app/server/internal/secret"
	"petalbook.app/server/internal/space"
	"petalbook.app/server/internal/store"
)

// SingleID is the id of the one space of single-user mode.
const SingleID = "default"

var (
	// ErrNotFound is the answer for a space that does not exist and for one the
	// user is not a member of: the two must look the same.
	ErrNotFound = errors.New("no such space")
	// ErrForbidden: a member whose role does not allow this.
	ErrForbidden = errors.New("not allowed")
	// ErrUserNotFound: no enabled user has that username.
	ErrUserNotFound = errors.New("no such user")
	// ErrNotMember: the user is not a member of the space.
	ErrNotMember = errors.New("not a member of this space")
	// ErrAlreadyMember: the user is a member already.
	ErrAlreadyMember = errors.New("already a member of this space")
	// ErrPersonal: a personal space is its person's alone.
	ErrPersonal = errors.New("a personal space cannot be shared")
	// ErrLastOwner: a space always keeps one owner.
	ErrLastOwner = errors.New("a space needs at least one owner")
	// ErrInvalid wraps a refused name.
	ErrInvalid = errors.New("invalid")
	// ErrLimit: the user owns as many spaces as they may (see QuotaError, which
	// is one).
	ErrLimit = errors.New("space limit reached")
	// ErrDisabled: an administrator has disabled the space; it is in its
	// members' lists, but nothing in it can be opened.
	ErrDisabled = errors.New("this space is disabled")
	// ErrBusy: requests still use the space, so it could not be closed now.
	ErrBusy = errors.New("the space is in use; try again in a moment")
	// ErrPersonalSpace: a personal space cannot be transferred or deleted.
	ErrPersonalSpace = errors.New("a personal space belongs to one person")
	// ErrDataDirInUse: another server runs on this data directory.
	ErrDataDirInUse = errors.New("another server uses this data directory (DATA/serve.lock is held)")
	// ErrTakeover: an administrator who is not a member asked for what only
	// taking the space over gives: to make or unmake an owner, to give
	// themselves (or a group of theirs) a role in it, or to change its remote.
	// Taking over is its own audited action (Manager.Transfer).
	ErrTakeover = errors.New("an administrator who is not a member takes the space over for that")
	// ErrUserDisabled: the user cannot sign in, so cannot own a space.
	ErrUserDisabled = errors.New("that user is disabled")
	errClosed       = errors.New("the server is shutting down")
)

// Info is a space as one user sees it.
type Info struct {
	ID   string
	Name string
	// Role is the built-in role whose permissions are the user's effective ones
	// (their direct role and their groups' roles together); "" for an
	// administrator who is not a member.
	Role string
	// DirectRole is the user's own role as a member, "" when they reach the
	// space only through groups; ViaGroups is whether a group of theirs is a
	// member of it.
	DirectRole string
	ViaGroups  bool
	// Perms is what the user may do here. Everything that decides access reads
	// this, never Role.
	Perms perm.Set
	// ViaAdmin: the user is no member but holds tenant.spaces.manage in the
	// space's tenant, and manages the space (AdminPerms) without belonging to it
	// (so it is in no list). Reading takes a view (Viewing); its owners and its
	// remote take a takeover.
	ViaAdmin bool
	// Viewing: an administrator reads the space in a view they opened (set by the
	// API, which holds the views); Perms then include read.
	Viewing  bool
	Personal bool
	// TenantID is the tenant the space belongs to.
	TenantID string
	// Disabled: an administrator has switched the space off. Perms are what the
	// user would have in it, but Open refuses it. Read when the list is asked
	// for, like Sync, not cached with it.
	Disabled bool
	// Inbox is the folder quick notes go to: a folder path, never empty.
	Inbox string
	// Sync is "local", or the state of the remote the space follows (ok,
	// offline, rewritten, error). It is read when the list is asked for, not cached.
	Sync string
	// Cache is what the space's copy on the server is: local, full, partial,
	// evicted or restoring; RemoteNotice is what the storage policy asks of it
	// that it does not have yet (nil if nothing). Both are read with Sync.
	Cache        string
	RemoteNotice *RemoteNotice
	// Assets is "repository" when the space has an attachment repository, else "space".
	Assets string
}

// AdminPerms is what a holder of tenant.spaces.manage who is not a member may
// do in a space: manage its settings and its people. Not read, and never
// write. Members here excludes owners and the administrator themselves
// (ErrTakeover), and the remote routes are for members only; both take the
// explicit takeover.
var AdminPerms = perm.Of(perm.Members, perm.Manage)

// Handle is an open space together with the caller's view of it. Release it
// when the request is done: a space in use is never closed.
type Handle struct {
	Space   *space.Space
	Info    Info
	release func()
	// IndexOnly: the space is evicted and was opened for a request that only
	// needs what its index answers (see OpenIndex); Space has no files.
	IndexOnly bool
}

// Release says the caller is done with the space.
func (h *Handle) Release() {
	if h.release != nil {
		h.release()
		h.release = nil
	}
}

// Options configures NewAccounts.
type Options struct {
	Store   *store.Store
	DataDir string // spaces live in DataDir/spaces/<id>, their index and journal in DataDir/cache/<id>
	// Serve says the manager is the server's (petalbook serve): it holds the
	// data directory's lock (DataDir/serve.lock) while it runs, and at its
	// start finishes what a crash left (evictions cut short, half restored
	// clones, half made spaces). A command run against the data directory of a
	// running server (petalbook user …) leaves Serve false and touches none of
	// that.
	Serve bool
	// Committer is the Git committer of commits ("Name <email>"): the service.
	Committer             string
	CommitIdle, CommitMax time.Duration
	Logger                *slog.Logger
	Now                   func() time.Time
	// CacheEntries bounds the membership cache by entries, one per space a
	// cached person can reach (default DefaultCacheEntries).
	CacheEntries int
	// MaxSpaces is how many spaces one user may own; 0 means no limit. The server
	// replaces it with the quota settings (SetQuota); it is what applies until
	// then, and what the tests of this package use.
	MaxSpaces int
	// IdleAfter is how long an open space may go unused before it is closed
	// (default 15 minutes). MemoryBudget is the memory the open spaces may
	// hold together, as space.MemoryCost estimates it (default
	// DefaultMemoryBudget), and MaxOpen how many may be open at once whatever
	// they cost (default DefaultMaxOpen); past either, the least recently used
	// idle ones are closed. A space in use is never closed. Closing flushes the
	// committer; the next request reopens the space.
	IdleAfter    time.Duration
	MemoryBudget int64
	MaxOpen      int
	// CloseWait is how long Close waits for spaces that requests still hold
	// (default 5 seconds) before closing them anyway.
	CloseWait time.Duration
	// OpenHook, if set, runs before a space is opened from disk (tests).
	OpenHook func(id string)

	// HostsFromFlag: the allowed hosts (RemotePolicy.Hosts) were given on the
	// command line, which wins over the setting of the administration page.
	HostsFromFlag bool
	// GitSupportsPartial, when set, replaces what the server's Git version says
	// about partial clones (tests).
	GitSupportsPartial *bool
	// EvictEvery is how often the eviction looks (default 10 minutes; negative
	// turns it off). EvictHook, if set, is called at each step of an eviction
	// and of a restore; an error it returns stops there as a crash would (tests).
	EvictEvery time.Duration
	EvictHook  func(step string) error
	// RestoreWait is how long a request that needs the files of an evicted
	// space waits for its restore before it answers 503 restoring (default 10
	// seconds).
	RestoreWait time.Duration
	// RestoreDeadline bounds one restore once it has its turn (default 5
	// minutes): a remote that hangs does not hold one of the two turns longer.
	RestoreDeadline time.Duration
	// RecycleEvery is how often the recycle bin's purge job looks (default an
	// hour; negative turns it off). PurgePause is the wait between two
	// directories being deleted (default 25 ms; negative: none). RecycleHook, if
	// set, is called at each step of a deletion, a restore and a purge
	// ("recycle:rows", "recycle:moved-space", "restore:marked", "purge:marked",
	// ...); an error it returns stops there as a crash would (tests).
	RecycleEvery time.Duration
	PurgePause   time.Duration
	RecycleHook  func(step string) error
	// Rename replaces os.Rename for the moves of the recycle bin (tests: a
	// rename that answers EXDEV, as one across filesystems does).
	Rename func(oldpath, newpath string) error
	// RemoveAll replaces os.RemoveAll for deleting the source of a move across
	// filesystems once its copy is in place (tests: a delete cut short).
	RemoveAll func(path string) error

	// AssetsHook is called at each step of moving a space's attachments into
	// its attachment repository; an error ends the move there, without
	// recording a failure, as a crash would (tests).
	AssetsHook func(step string) error

	// SizeEvery is how often the background sizer measures one space (default
	// DefaultSizeEvery; negative turns it off), and SizeAge how old a space's
	// measurement must be before it is measured again (default DefaultSizeAge).
	// See sizes.go for what that costs.
	SizeEvery, SizeAge time.Duration

	// RecountEvery is how often every usage row is recounted from the rows it
	// summarises (default DefaultRecountEvery; negative turns it off). The first
	// pass is made when the manager starts running.
	RecountEvery time.Duration

	// Secrets encrypts the credentials of remotes; by default a box keyed from
	// DataDir/secret.key or PETALBOOK_SECRET_KEY.
	Secrets *secret.Box
	// RemotePolicy says which remote addresses are accepted.
	RemotePolicy gitstore.Policy
	// PullInterval is how often a remote is fetched (default DefaultPullInterval
	// in the flag; 0 here means never by itself).
	PullInterval time.Duration
	// NetTimeout, FetchFresh and FailFast tune the remote commands (space.RemoteOptions).
	NetTimeout, FetchFresh, FailFast time.Duration
	// RemoteRunner replaces the git runner of remote spaces (tests).
	RemoteRunner gitstore.Runner

	// Connections are the server's providers; nil when it has none to offer
	// (spaces with a connection credential then show provider_not_configured).
	Connections *connections.Registry
	// ConnectionRetry is how often a space whose connection credential does not
	// work is tried again (default 10 minutes); a fix triggers it at once.
	ConnectionRetry time.Duration
	// PushRetry are the waits between the tries of the first push of a space
	// made with a new repository (default 1 s, 3 s, 7 s: four tries in 15 s).
	PushRetry []time.Duration
	// PushDeadline bounds the first push of such a space, tries, waits and
	// each push itself together (default 15 s).
	PushDeadline time.Duration
	// SyncHook, if set, is told the steps of making a space with a provider's
	// repository ("repo-created", "connected"), before the next one (tests).
	SyncHook func(step string)
}

// Defaults for closing spaces that are not in use.
const (
	DefaultIdleAfter = 15 * time.Minute
	// DefaultMemoryBudget is what the open spaces may hold together: a
	// quarter of the 4 GB a small server (4 cores, 6 GB) can give the process
	// beside the operating system's file cache and Git. An idle 50 000-page
	// space costs about 12 MB in that sum, a 200-page one about 2 MB (see
	// docs/design/database.md, §5.5).
	DefaultMemoryBudget = 1 << 30
	// DefaultMaxOpen bounds the open spaces whatever they cost: each keeps a
	// few files open.
	DefaultMaxOpen = 256
	// DefaultMaxSpaces is the flag's default; Options.MaxSpaces 0 is unlimited.
	DefaultMaxSpaces = 3
)

// opening is one space that is open, being opened, or being closed.
type opening struct {
	done chan struct{} // closed when the open has finished
	sp   *space.Space
	err  error

	// Guarded by Manager.mu.
	refs     int           // requests (and sweeps) using it right now
	last     time.Time     // when it was last acquired or released
	closing  chan struct{} // set while it is being closed; closed when that is done
	nextPull time.Time     // when the remote is pulled next; zero for a local space
	cost     int64         // its memory as last estimated (see refreshCosts)
	phase    string        // while it is opened: the phase of its index rebuild, if one is needed ("" if not)
}

// Manager is safe for concurrent use.
type Manager struct {
	opt Options
	// onMembers is told of MembershipChanged (see OnMembershipChanged).
	onMembers func(userIDs ...string)
	single    *space.Space
	// singleInbox is the inbox folder of the one space of single-user mode.
	singleInbox string
	log         *slog.Logger

	mu    sync.Mutex
	open  map[string]*opening
	cache *accessCache // who can reach which space, see cache.go
	// listGen counts membership changes. A list read from the database is cached
	// only if no change happened while it was being read, so a read that began
	// before a removal cannot put the removed member back.
	listGen uint64
	closed  bool

	authors authorCache // names for commits, by user id (see AuthorByID)

	heads map[string]string // space id -> the change cursor it had when it was last closed (guarded by mu; see heads.go)
	syncs map[string]string // space id -> state of its remote, for Info.Sync (guarded by mu; nil until first needed)
	// settled is the last state of each space's remote that was not "offline",
	// what a notice is told against (notify.go; guarded by mu).
	settled  map[string]string
	disabled map[string]bool // ids of the disabled spaces, and of deleted ones, which stay refused (guarded by mu; nil until first needed)
	adminMu  sync.Mutex      // one administrator action (disable, transfer, delete) at a time
	sizeSem  chan struct{}   // one directory walk at a time (see Resize)
	remoteMu sync.Mutex      // one connect, disconnect or key change at a time
	pullWake chan struct{}   // a space with a remote was just opened: pull it now

	st      storageState // the storage policy (storage.go) and its caches
	ev      evictState   // eviction and restoring (evict.go)
	cs      cacheStates  // cache states in memory (partial.go)
	as      assetState   // attachment repositories and their cache (assets.go)
	recycle recycleState // the recycle bin (recycle.go)
	exp     exportState  // exports (export.go)
	// jobs is the runner of the exports, the recycle actions and the moves of
	// attachments (jobs.go).
	jobsMu   sync.Mutex
	jobs     *jobs.Runner
	problems problemMemo // what each space said of its problems last (problems.go)

	trimMu    sync.Mutex // one trim at a time
	trimming  bool       // a background trim is queued or running (guarded by mu)
	trimAgain bool       // a space was opened while it ran: it runs once more (guarded by mu)
	// testCost, set by tests only, replaces space.MemoryCost.
	testCost  func(id string, sp *space.Space) int64
	createMu  sync.Mutex      // space creation is one at a time, so the limit cannot be passed by racing
	staging   map[string]bool // spaces being made with a repository on a provider, whose first push needs no member yet (guarded by mu)
	reserved  map[string]int  // spaces being made with a repository on a provider, by owner (guarded by createMu)
	reservedT map[string]int  // the same, by tenant (guarded by createMu)

	quota quotaHooks // see SetQuota

	// assetsPolicy decides where attachments go (SetAssetsPolicy); nil: separate.
	assetsPolicy func(ctx context.Context, tenantID string) (AssetsPolicy, error)

	closeCtx    context.Context // ends when the manager is closed (background work stops with it)
	closeCancel context.CancelFunc

	bgMu     sync.Mutex     // guards bgClosed and the Adds to bg
	bgClosed bool           // Close has begun: no background task starts any more
	bg       sync.WaitGroup // background tasks started by goBg; Close waits for them

	lock *os.File // DataDir/serve.lock, held while a server's manager runs (Options.Serve)

	// testHook, set by tests only, runs at named points inside transactions.
	testHook func(point string)
	// testID, set by tests only, picks a new group's id ("" for a random one).
	testID func() string
	// testOpened, set by tests only, is called with a space openSpace has just opened.
	testOpened func(*space.Space)
}

// at runs the test hook, if any, at a named point.
func (m *Manager) at(point string) {
	if m.testHook != nil {
		m.testHook(point)
	}
}

// NewAccounts returns the manager of accounts mode.
func NewAccounts(opt Options) (*Manager, error) {
	if opt.Store == nil || opt.DataDir == "" {
		return nil, errors.New("spaces: a store and a data directory are required")
	}
	abs, err := filepath.Abs(opt.DataDir)
	if err != nil {
		return nil, err
	}
	// The index database cannot live at a path with these (see index.Open);
	// better to say so now than to fail the first time a space is opened.
	if strings.ContainsAny(abs, "?#") {
		return nil, fmt.Errorf("the data directory %q: '?' and '#' in the path are not supported", abs)
	}
	opt.DataDir = abs
	if err := os.MkdirAll(filepath.Join(abs, "spaces"), 0o755); err != nil {
		return nil, err
	}
	if opt.Secrets == nil {
		opt.Secrets = secret.NewBox(abs)
	}
	var lock *os.File
	if opt.Serve {
		// Taken before anything is cleaned up, so that what one server has in
		// progress is never taken for what a crash left by another.
		if lock, err = lockDataDir(filepath.Join(abs, "serve.lock")); err != nil {
			return nil, err
		}
	}
	m := newManager(opt, nil)
	m.lock = lock
	m.initStorage(context.Background())
	if opt.Serve {
		// The exports and the recycle actions a crash cut short are taken up by
		// the jobs runner when it starts (each kind's Recover), and what a
		// failure left by the recycle job's first look (RunRecycle).
		if err := m.recoverEvictions(context.Background()); err != nil {
			m.log.Warn("could not finish the evictions and restores a crash cut short", "error", err)
		}
		if n, err := m.SweepStaged(context.Background()); err != nil {
			m.log.Warn("could not remove the spaces a crash left half made", "error", err)
		} else if n > 0 {
			m.log.Info("removed spaces a crash left half made", "count", n)
		}
	}
	if err := m.loadEvicted(context.Background()); err != nil {
		m.log.Warn("could not look at the spaces whose clone was removed", "error", err)
	}
	return m, nil
}

// Secrets is the box that encrypts the credentials the server keeps; nil in
// single-user mode.
func (m *Manager) Secrets() *secret.Box { return m.opt.Secrets }

// NewSingle returns the manager of single-user mode around sp, and makes sure
// the built-in user and space exist in the database (drafts refer to them).
func NewSingle(ctx context.Context, st *store.Store, sp *space.Space, log *slog.Logger) (*Manager, error) {
	if err := store.EnsureLocal(ctx, st.Q(), SingleID, sp.Name(), time.Now()); err != nil {
		return nil, err
	}
	return newManager(Options{Store: st, Logger: log}, sp), nil
}

func newManager(opt Options, single *space.Space) *Manager {
	if opt.Now == nil {
		opt.Now = time.Now
	}
	log := opt.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	if opt.IdleAfter <= 0 {
		opt.IdleAfter = DefaultIdleAfter
	}
	if opt.MaxOpen <= 0 {
		opt.MaxOpen = DefaultMaxOpen
	}
	if opt.MemoryBudget <= 0 {
		opt.MemoryBudget = DefaultMemoryBudget
	}
	if opt.CloseWait <= 0 {
		opt.CloseWait = 5 * time.Second
	}
	if opt.ConnectionRetry <= 0 {
		opt.ConnectionRetry = 10 * time.Minute
	}
	if opt.PushRetry == nil {
		opt.PushRetry = []time.Duration{time.Second, 3 * time.Second, 7 * time.Second}
	}
	if opt.PushDeadline <= 0 {
		opt.PushDeadline = 15 * time.Second
	}
	return &Manager{staging: map[string]bool{}, reserved: map[string]int{}, reservedT: map[string]int{}, opt: opt, single: single, singleInbox: store.DefaultInbox, log: log, open: map[string]*opening{}, cache: newAccessCache(opt.CacheEntries), pullWake: make(chan struct{}, 1), sizeSem: make(chan struct{}, 1)}
}

// SetSingleInbox sets the inbox folder of single-user mode (the --inbox flag).
// Call it before serving.
func (m *Manager) SetSingleInbox(folder string) error {
	folder, err := ValidInbox(folder)
	if err != nil {
		return err
	}
	m.singleInbox = folder
	return nil
}

// ValidInbox checks an inbox folder: a valid folder path that is not the root.
func ValidInbox(folder string) (string, error) {
	if folder == "" {
		return "", fmt.Errorf("%w: the inbox must be a folder, not the root of the space", ErrInvalid)
	}
	if err := space.ValidFolder(folder); err != nil {
		return "", fmt.Errorf("%w: %s", ErrInvalid, err.Error())
	}
	return folder, nil
}

// Single reports single-user mode.
func (m *Manager) Single() bool { return m.single != nil }

// List returns the spaces the user is a member of. After the first call for a
// user it does not touch the database until something about their spaces changes.
func (m *Manager) List(ctx context.Context, userID string) ([]Info, error) {
	l, err := m.list(ctx, userID)
	if err != nil {
		return nil, err
	}
	return m.withSync(ctx, l), nil
}

func (m *Manager) list(ctx context.Context, userID string) ([]Info, error) {
	if m.single != nil {
		if userID != store.LocalUserID {
			return nil, nil
		}
		return []Info{{ID: SingleID, Name: m.single.Name(), Role: perm.Owner, DirectRole: perm.Owner, Perms: perm.All, Personal: true, Inbox: m.singleInbox, Sync: "local"}}, nil
	}
	cells, err := m.cellsOf(ctx, userID)
	if err != nil {
		return nil, err
	}
	l := make([]Info, 0, len(cells))
	for _, cl := range cells {
		mt, err := m.metaFor(ctx, cl.id)
		if errors.Is(err, ErrNotFound) {
			continue // deleted since
		}
		if err != nil {
			return nil, err
		}
		l = append(l, cl.info(mt))
	}
	sortedInfos(l)
	return l, nil
}

// Open returns the space with what the user may do in it, and ErrNotFound if
// they may not even know it exists. This is the one place space access is
// decided. The caller must Release the handle.
//
// A member is answered from the cached list: no database query, and a scan of
// the spaces that one person belongs to. An administrator who is not a member
// costs one primary-key query to learn that the space exists; that is the only
// path that touches the database, and only administrators take it.
func (m *Manager) Open(ctx context.Context, user store.User, spaceID string) (*Handle, error) {
	return m.openForIndex(ctx, user, authz.Subject{}, false, spaceID, true, false)
}

// OpenAs is Open for a request whose caller holds access outside spaces (the
// session cache's subject, custom roles included): whether an administrator
// who is not a member reaches the space is asked of it.
func (m *Manager) OpenAs(ctx context.Context, user store.User, access authz.Subject, spaceID string) (*Handle, error) {
	return m.openForIndex(ctx, user, access, true, spaceID, true, false)
}

// AccessOf is what an account holds outside spaces by its built-in role, for
// a caller that has no subject of its own (the session cache's has its custom
// roles too).
func AccessOf(u store.User) authz.Subject {
	return authz.SubjectFor(u.Role, u.SiteAdmin, u.TenantID)
}

// OpenMember is Open for a credential that is not the person's own session
// (an API token): the administrator's way into spaces they are not a member of
// is not open to it, so a space the user is not a member of is ErrNotFound
// whatever their role. No query: the membership comes from the cached list.
func (m *Manager) OpenMember(ctx context.Context, user store.User, spaceID string) (*Handle, error) {
	return m.openForIndex(ctx, user, authz.Subject{}, false, spaceID, false, false)
}

// OpenIndex is Open for a request that only reads what the index answers (a
// list, the tree, tags, links, checks): an evicted space is opened by its
// index and is not restored. The handle says IndexOnly in that case.
func (m *Manager) OpenIndex(ctx context.Context, user store.User, spaceID string) (*Handle, error) {
	return m.openForIndex(ctx, user, authz.Subject{}, false, spaceID, true, true)
}

// OpenIndexAs is OpenIndex with the caller's subject (see OpenAs).
func (m *Manager) OpenIndexAs(ctx context.Context, user store.User, access authz.Subject, spaceID string) (*Handle, error) {
	return m.openForIndex(ctx, user, access, true, spaceID, true, true)
}

// OpenMemberIndex is OpenMember for such a request.
func (m *Manager) OpenMemberIndex(ctx context.Context, user store.User, spaceID string) (*Handle, error) {
	return m.openForIndex(ctx, user, authz.Subject{}, false, spaceID, false, true)
}

// openForIndex opens for user; access is what they hold outside spaces when
// haveAccess, else it is built from their built-in role, only on the path of
// an administrator who is not a member (a member costs nothing for it).
func (m *Manager) openForIndex(ctx context.Context, user store.User, access authz.Subject, haveAccess bool, spaceID string, viaAdmin, indexOnly bool) (*Handle, error) {
	var err error
	var in Info
	var found bool
	if m.single != nil {
		l, err := m.list(ctx, user.ID)
		if err != nil {
			return nil, err
		}
		if len(l) > 0 && l[0].ID == spaceID {
			in, found = l[0], true
		}
	} else {
		var err error
		if in, found, err = m.infoOf(ctx, user.ID, spaceID); err != nil {
			return nil, err
		}
	}
	if !found {
		if m.single != nil || !viaAdmin || !page.ValidID(spaceID) {
			return nil, ErrNotFound
		}
		// Who may manage a space they are not a member of holds
		// tenant.spaces.manage there: asked first in their own tenant (no query
		// for anyone else), then in the space's.
		if !haveAccess {
			access = AccessOf(user)
		}
		if !access.Can(authz.TenantSpacesManage, user.TenantID) && !access.SiteWide(authz.TenantSpacesManage) {
			return nil, ErrNotFound
		}
		sp, err := store.GetSpace(ctx, m.opt.Store.Q(), store.ScopeOf(user), spaceID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		if !access.Can(authz.TenantSpacesManage, sp.TenantID) {
			return nil, ErrNotFound
		}
		if m.opt.Store.TenantDisabled(sp.TenantID) {
			return nil, ErrDisabled // the spaces of a disabled tenant cannot be opened, by anyone
		}
		// A person's own space is not an organisation's: a tenant's administrator
		// does not reach it at all, as if it were not there (3.5; a site
		// administrator, who may read any file on the disk, does: the key
		// site.spaces.personal.meta).
		if sp.Personal && !access.Can(authz.SiteSpacesPersonalMeta, "") {
			return nil, ErrNotFound
		}
		// An administrator who is not a member manages the space (its members,
		// its remote, its settings) and does not read it: reading is a view the
		// administrator opens with a reason, which is recorded (see api
		// "Viewing a space"), and the caller adds read to these only then.
		in = Info{ID: sp.ID, Name: sp.Name, Perms: AdminPerms, ViaAdmin: true, Personal: sp.Personal, Inbox: sp.Inbox, TenantID: sp.TenantID}
	}
	in = m.withSync(ctx, []Info{in})[0]
	var sp *space.Space
	var release func()
	if indexOnly {
		sp, release, err = m.acquireIndex(ctx, spaceID)
	} else {
		sp, release, err = m.Acquire(ctx, spaceID)
	}
	if err != nil {
		return nil, err
	}
	m.noteUse(spaceID)
	return &Handle{Space: sp, Info: in, release: release, IndexOnly: sp != nil && sp.Evicted()}, nil
}

// OpenMembership is Open without opening the space: the handle carries what
// the user is in it (Info) and no Space. It is for leaving a space, which is a
// matter of the membership only and must work for a space that is switched off.
// ErrNotFound unless the user is a member.
func (m *Manager) OpenMembership(ctx context.Context, user store.User, spaceID string) (*Handle, error) {
	if m.single != nil {
		l, err := m.list(ctx, user.ID)
		if err != nil || len(l) == 0 || l[0].ID != spaceID {
			return nil, ErrNotFound
		}
		return &Handle{Info: m.withSync(ctx, []Info{l[0]})[0]}, nil
	}
	in, found, err := m.infoOf(ctx, user.ID, spaceID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrNotFound
	}
	return &Handle{Info: m.withSync(ctx, []Info{in})[0]}, nil
}

// Acquire opens a space by id without asking who is looking: for the draft
// sweeper, which acts for a draft's author. Request handlers use Open. The
// space stays open until release is called, and is opened again transparently
// if it was closed for being idle.
func (m *Manager) Acquire(ctx context.Context, id string) (*space.Space, func(), error) {
	if m.single != nil {
		if id != SingleID {
			return nil, nil, ErrNotFound
		}
		return m.single, func() {}, nil
	}
	if !page.ValidID(id) {
		return nil, nil, ErrNotFound
	}
	if err := m.loadDisabled(ctx); err != nil {
		return nil, nil, err
	}
	return m.acquire(ctx, id, false)
}

// acquire is Acquire; admin lets the administration reach a disabled space.
func (m *Manager) acquire(ctx context.Context, id string, admin bool) (*space.Space, func(), error) {
	noClone := false // the space was found without its clone once: restored, and tried once more
	for {
		// An evicted space has no files: they are restored first, outside the
		// lock (it waits for the network), and the request goes on as for any
		// space once they are there.
		if m.single == nil {
			if err := m.loadEvicted(ctx); err != nil {
				return nil, nil, err
			}
			m.mu.Lock()
			disabled := m.disabled[id]
			m.mu.Unlock()
			if m.isEvicted(id) && !(admin && disabled) {
				if err := m.ensureRestored(ctx, id); err != nil {
					return nil, nil, err
				}
			}
		}
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return nil, nil, errClosed
		}
		// Decided under the lock that takes the reference, so a space being
		// disabled has no new holders from this moment on.
		if !admin && m.disabled[id] {
			m.mu.Unlock()
			return nil, nil, ErrDisabled
		}
		e := m.open[id]
		if e != nil && e.closing != nil {
			// Being closed: wait, so two copies of one space never run at once.
			c := e.closing
			m.mu.Unlock()
			select {
			case <-c:
				continue
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			}
		}
		opener := false
		if e == nil {
			e = &opening{done: make(chan struct{})}
			m.open[id] = e
			opener = true
		}
		e.refs++
		e.last = m.opt.Now()
		m.mu.Unlock()
		if opener {
			// Opening can take as long as an index rebuild, and wait for a
			// rebuild slot of the server's first; it runs in the background, so
			// that it is not cut short because the request that started it went
			// away or was answered restoring (see awaitOpen).
			run := func() { m.finishOpen(context.WithoutCancel(ctx), id, e) }
			if !m.goBg(run) {
				run()
			}
		}
		if err := m.awaitOpen(ctx, e); err != nil {
			m.unref(e)
			return nil, nil, err
		}
		if e.err != nil {

			if errors.Is(e.err, errNoClone) && !noClone {
				// It follows a remote and its clone is gone without being
				// recorded as evicted: it is evicted, and restored as such.
				noClone = true
				m.ev.mu.Lock()
				if m.ev.evicted != nil {
					m.ev.evicted[id] = true
				}
				m.ev.mu.Unlock()
				continue
			}
			return nil, nil, e.err
		}
		var once sync.Once
		return e.sp, func() { once.Do(func() { m.unref(e) }) }, nil
	}
}

type restoringAnswerKey struct{}

// WithRestoringAnswer marks ctx as that of a request that answers restoring
// (503) when the space it needs is still having its index rebuilt after
// Options.RestoreWait (see awaitOpen). The HTTP layer sets it on every request;
// background work, exports, moves and every other caller wait for the open.
// The mark is a value of the context, so it survives context.WithoutCancel:
// work that outlives the request must not be started from c.r.Context() (the
// API's background work takes bgCtx), or it would answer restoring too.
func WithRestoringAnswer(ctx context.Context) context.Context {
	return context.WithValue(ctx, restoringAnswerKey{}, true)
}

func answersRestoring(ctx context.Context) bool {
	v, _ := ctx.Value(restoringAnswerKey{}).(bool)
	return v
}

// finishOpen opens the space of e and tells those waiting (e.done). An open
// waiting for a rebuild slot ends when the manager is closed. A panic while
// opening is the open's error: it runs in the background, where a panic
// would end the process.
func (m *Manager) finishOpen(ctx context.Context, id string, e *opening) {
	func() {
		defer func() {
			if r := recover(); r != nil {
				m.log.Error("opening a space panicked", "space", id, "panic", r, "stack", string(debug.Stack()))
				e.sp, e.err = nil, fmt.Errorf("opening space %s: panic: %v", id, r)
			}
		}()
		e.sp, e.err = m.openSpace(ctx, id, func(phase string) {
			m.mu.Lock()
			e.phase = phase
			m.mu.Unlock()
		})
	}()
	if e.err != nil {
		m.mu.Lock()
		delete(m.open, id)
		m.mu.Unlock()
	}
	if e.err == nil {
		if _, ok := e.sp.Remote(); ok {
			// Someone is about to look at it: bring it up to date now.
			m.mu.Lock()
			e.nextPull = m.opt.Now()
			m.mu.Unlock()
			m.wakePull()
		}
	}
	close(e.done)
	if e.err == nil {
		m.trimSoon()
	}
}

// awaitOpen waits for the open of e. For a request marked WithRestoringAnswer,
// an open whose index is rebuilt (or waits for one of the server's rebuild
// slots) answers *RestoringError once the request has waited
// Options.RestoreWait, as a restore does: the open goes on in the background
// and a later request finds the space open. Any other open, and every open for
// any other caller, is waited for, for as long as the caller's context lasts.
func (m *Manager) awaitOpen(ctx context.Context, e *opening) error {
	if !answersRestoring(ctx) {
		select {
		case <-e.done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	wait := m.opt.RestoreWait
	if wait <= 0 {
		wait = 10 * time.Second
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	for {
		select {
		case <-e.done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			m.mu.Lock()
			phase := e.phase
			m.mu.Unlock()
			if phase != "" {
				return &RestoringError{Phase: phase, Rebuild: true}
			}
			t.Reset(time.Second) // not a rebuild (yet): looked at again
		}
	}
}

func (m *Manager) unref(e *opening) {
	m.mu.Lock()
	e.refs--
	e.last = m.opt.Now()
	m.mu.Unlock()
}

func (m *Manager) paths(id string) (root, state string) {
	return filepath.Join(m.opt.DataDir, "spaces", id), filepath.Join(m.opt.DataDir, "cache", id)
}

// errNoClone: a space that follows a remote has no clone on disk, and is not
// recorded as evicted. acquire restores it.
var errNoClone = errors.New("the clone of a space that follows a remote is missing")

func (m *Manager) openSpace(ctx context.Context, id string, onRebuild func(phase string)) (*space.Space, error) {
	if m.opt.OpenHook != nil {
		m.opt.OpenHook(id)
	}
	root, state := m.paths(id)
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		// space.Open would quietly make an empty repository here. A space that
		// follows a remote is restored from it instead (see acquire).
		if r, rerr := store.GetRemote(ctx, m.opt.Store.Q(), id, store.RoleContent); rerr == nil && r.Connected() && errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: space %s", errNoClone, id)
		}
		return nil, fmt.Errorf("the directory of space %s is missing", id)
	}
	owner, name, err := store.SpaceOwner(ctx, m.opt.Store.Q(), id)
	if err != nil {
		return nil, err
	}
	var author string
	if owner != nil {
		author = IdentityOf(*owner).String()
	}
	var rem *space.RemoteOptions
	switch r, err := store.GetRemote(ctx, m.opt.Store.Q(), id, store.RoleContent); {
	case err == nil && r.Connected():
		rem = m.remoteOptions(id, r)
	case err != nil && !errors.Is(err, store.ErrNotFound):
		return nil, err
	}
	sp, err := space.Open(space.Options{
		Root: root, Name: name, StateDir: state, Committer: m.opt.Committer, Author: author,
		CommitIdle: m.opt.CommitIdle, CommitMax: m.opt.CommitMax, Logger: m.log.With("space", id), Remote: rem,
		WriteGate: func() error { return m.CheckWritable(id) }, OnRebuild: onRebuild, SlotContext: m.closedCtx(),
		// The server holds serve.lock: no other PetalBook process uses the clone.
		ClearStaleGitLocks: m.opt.Serve,
	})
	if err != nil {
		return nil, err
	}
	// A panic from here on (finishOpen recovers it) must not leave this copy
	// open: a later open would run a second one, with a second committer.
	defer func() {
		if r := recover(); r != nil {
			sp.Close()
			panic(r)
		}
	}()
	if m.testOpened != nil {
		m.testOpened(sp)
	}
	if rem != nil {
		m.markCache(id, sp)
		m.attachAssets(ctx, id, sp)
	}
	return sp, nil
}

// ValidName checks a space name: 1-80 characters once trimmed, no control characters.
func ValidName(name string) (string, error) {
	name = strings.TrimSpace(name)
	n := utf8.RuneCountInString(name)
	if n < 1 || n > 80 || !utf8.ValidString(name) {
		return "", fmt.Errorf("%w: a space name is 1 to 80 characters", ErrInvalid)
	}
	if strings.IndexFunc(name, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return "", fmt.Errorf("%w: a space name cannot contain control characters", ErrInvalid)
	}
	return name, nil
}

// Identity is a person as Git records them.
type Identity struct {
	Name, Email string
}

// String is the "Name <email>" form the space package takes.
func (i Identity) String() string { return i.Name + " <" + i.Email + ">" }

// IdentityOf is the Git author for a user: their name and email, with the
// email falling back to <username>@users.petalbook.invalid. Characters Git
// would mangle or that would break the "Name <email>" form are dropped.
func IdentityOf(u store.User) Identity {
	clean := func(s string) string {
		return strings.TrimSpace(strings.Map(func(r rune) rune {
			if r < 0x20 || r == 0x7f || r == '<' || r == '>' || r == '\n' {
				return -1
			}
			return r
		}, s))
	}
	name, email := clean(u.Name), clean(u.Email)
	if name == "" {
		name = clean(u.Username)
	}
	if name == "" {
		name = "unknown"
	}
	if email == "" || strings.ContainsAny(email, " \t") {
		email = u.Username + "@users.petalbook.invalid"
	}
	return Identity{Name: name, Email: email}
}

// Staged is a space whose repository exists on disk but that no one can see
// yet: it has no database row. Commit it in the same step that makes the row,
// or Abort it, so a failure part-way leaves nothing visible.
type Staged struct {
	m    *Manager
	id   string
	name string
	sp   *space.Space
	done bool
}

// ID is the id the space will have.
func (s *Staged) ID() string { return s.id }

// Name is the display name the row must be made with.
func (s *Staged) Name() string { return s.name }

// Stage creates the directory and Git repository of a new space, with author
// as the Git author of its commits. Nothing refers to it until Commit.
func (m *Manager) Stage(name string, author Identity) (*Staged, error) {
	if m.single != nil {
		return nil, errors.New("single-user mode has one space")
	}
	name, err := ValidName(name)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(m.stagingDir(), 0o755); err != nil {
		return nil, err
	}
	var id, root, state string
	for i := 0; ; i++ {
		id = page.NewID(m.opt.Now())
		root, state = m.paths(id)
		// The mark comes first, so whatever a crash leaves of this space on
		// disk is marked; see SweepStaged.
		mark, err := os.OpenFile(m.stagedMark(id), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			if os.IsExist(err) && i < 10 {
				continue
			}
			return nil, err
		}
		mark.Close()
		// Mkdir fails when the directory exists: it claims the id.
		if err = os.Mkdir(root, 0o755); err == nil {
			break
		}
		os.Remove(m.stagedMark(id))
		if !os.IsExist(err) || i >= 10 {
			return nil, err
		}
	}
	sp, err := space.Open(space.Options{
		Root: root, Name: name, StateDir: state, Committer: m.opt.Committer, Author: author.String(),
		CommitIdle: m.opt.CommitIdle, CommitMax: m.opt.CommitMax, Logger: m.log.With("space", id),
		WriteGate: func() error { return m.CheckWritable(id) },
	})
	if err != nil {
		os.RemoveAll(root)
		os.RemoveAll(state)
		os.Remove(m.stagedMark(id))
		return nil, err
	}
	return &Staged{m: m, id: id, name: name, sp: sp}, nil
}

// Commit makes the space known to the manager, after its database row exists.
func (s *Staged) Commit(ownerID string) {
	if s.done {
		return
	}
	s.done = true
	m := s.m
	os.Remove(m.stagedMark(s.id)) // the row exists now
	e := &opening{done: make(chan struct{}), sp: s.sp, last: m.opt.Now()}
	close(e.done)
	m.mu.Lock()
	closed := m.closed
	if !closed {
		m.open[s.id] = e
	}
	m.dropListsLocked(ownerID)
	m.mu.Unlock()
	if closed {
		s.sp.Close()
		return
	}
	m.trimSoon()
}

// Abort removes a space that was never committed.
func (s *Staged) Abort() {
	if s.done {
		return
	}
	s.done = true
	root, state := s.m.paths(s.id)
	s.sp.Close()
	os.RemoveAll(root)
	os.RemoveAll(state)
	os.Remove(s.m.stagedMark(s.id))
}

// stagedSweepAge is how old a staged space's mark must be before SweepStaged
// removes the space: well past the longest a request spends making one (three
// minutes), so a space another request is still making is never touched.
const stagedSweepAge = 15 * time.Minute

func (m *Manager) stagingDir() string { return filepath.Join(m.opt.DataDir, "staging") }

// stagedMark is the file that says the space id was staged and is not yet
// known to be recorded: made before its directory, removed by Commit or Abort.
func (m *Manager) stagedMark(id string) string { return filepath.Join(m.stagingDir(), id) }

// SweepStaged removes what a crash between Stage and Commit (or Abort) left:
// the directories of a staged space that has no row, once its mark is older
// than stagedSweepAge. Only a marked id is looked at, so a real space, or a
// directory this code did not stage, is never removed; a marked space that has
// its row (the crash came after the transaction) only loses the mark. It
// returns how many spaces it removed.
func (m *Manager) SweepStaged(ctx context.Context) (int, error) {
	if m.single != nil {
		return 0, nil
	}
	ents, err := os.ReadDir(m.stagingDir())
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range ents {
		id := e.Name()
		if !e.Type().IsRegular() || !page.ValidID(id) {
			continue
		}
		fi, err := e.Info()
		if err != nil || time.Since(fi.ModTime()) < stagedSweepAge {
			continue
		}
		_, err = store.GetSpace(ctx, m.opt.Store.Q(), store.AllTenants(), id) // background sweep: a staged id of any tenant
		switch {
		case err == nil:
			os.Remove(m.stagedMark(id))
			continue
		case !errors.Is(err, store.ErrNotFound):
			return n, err
		}
		root, state := m.paths(id)
		if err := os.RemoveAll(root); err != nil {
			return n, err
		}
		if err := os.RemoveAll(state); err != nil {
			return n, err
		}
		if err := os.RemoveAll(m.assetsDir(id)); err != nil { // the attachment clone is made before the row
			return n, err
		}
		os.Remove(m.stagedMark(id))
		n++
	}
	return n, nil
}

// Create makes a space owned by the user, unless they own as many as the
// server allows. The repository is made first and the row second, so a failure
// in either leaves nothing visible.
func (m *Manager) Create(ctx context.Context, userID, name string) (Info, error) {
	return m.CreateWith(ctx, userID, name, "")
}

// CreateWith is Create with the inbox folder given ("" is the default).
func (m *Manager) CreateWith(ctx context.Context, userID, name, inbox string) (Info, error) {
	return m.CreateWithMode(ctx, userID, name, inbox, "")
}

// CreateWithMode is CreateWith with the storage mode asked for ("" for none:
// the default in effect for the creator's tenant, see ResolveMode).
func (m *Manager) CreateWithMode(ctx context.Context, userID, name, inbox, mode string) (Info, error) {
	if m.single != nil {
		return Info{}, errors.New("single-user mode has one space")
	}
	if inbox == "" {
		inbox = store.DefaultInbox
	}
	inbox, err := ValidInbox(inbox)
	if err != nil {
		return Info{}, err
	}
	if _, err := ValidName(name); err != nil {
		return Info{}, err
	}
	if _, err := m.resolveFor(ctx, userID, mode, false); err != nil {
		return Info{}, err
	}
	if err := m.CheckNewSpace(ctx, userID, "", ""); err != nil {
		return Info{}, err
	}
	m.createMu.Lock()
	defer m.createMu.Unlock()
	u, err := store.GetUser(ctx, m.opt.Store.Q(), store.AllTenants(), userID) // the creator, by id
	if err != nil {
		return Info{}, err
	}
	if err := m.checkSpaceQuota(ctx, *u); err != nil {
		return Info{}, err
	}
	stg, err := m.Stage(name, IdentityOf(*u))
	if err != nil {
		return Info{}, err
	}
	now := m.opt.Now()
	err = m.opt.Store.Tx(ctx, func(q store.Q) error {
		if err := store.InsertSpace(ctx, q, u.TenantID, stg.ID(), stg.Name(), "", inbox, now); err != nil {
			return err
		}
		return store.AddMember(ctx, q, stg.ID(), userID, perm.Owner, now)
	})
	if err != nil {
		stg.Abort()
		return Info{}, err
	}
	stg.Commit(userID)
	m.usageAdded(ctx, store.Touched{UserID: userID, TenantID: u.TenantID})
	return Info{ID: stg.ID(), Name: stg.Name(), Role: perm.Owner, DirectRole: perm.Owner, Perms: perm.All, Inbox: inbox, Sync: "local"}, nil
}

// Rename changes a space's display name; whoever may manage the space.
func (m *Manager) Rename(ctx context.Context, user store.User, spaceID, name string) (Info, error) {
	return m.Update(ctx, user, spaceID, &name, nil)
}

// Update changes a space's display name, its inbox folder, or both (nil
// leaves one as it is); whoever may manage the space may. Changing the inbox
// moves nothing.
func (m *Manager) Update(ctx context.Context, user store.User, spaceID string, newName, newInbox *string) (Info, error) {
	if m.single != nil {
		return Info{}, ErrNotFound
	}
	h, err := m.Open(ctx, user, spaceID)
	if err != nil {
		return Info{}, err
	}
	defer h.Release()
	if !h.Info.Perms.Has(perm.Manage) {
		return Info{}, ErrForbidden
	}
	name, inbox := h.Info.Name, h.Info.Inbox
	if newName != nil {
		if name, err = ValidName(*newName); err != nil {
			return Info{}, err
		}
	}
	if newInbox != nil {
		if inbox, err = ValidInbox(*newInbox); err != nil {
			return Info{}, err
		}
	}
	err = m.opt.Store.Tx(ctx, func(q store.Q) error {
		if newName != nil {
			if err := store.RenameSpace(ctx, q, spaceID, name); err != nil {
				return err
			}
		}
		if newInbox != nil {
			if err := store.SetSpaceInbox(ctx, q, spaceID, inbox); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Info{}, err
	}
	// Names are kept once per space: nobody's list is dropped.
	m.spaceChanged(spaceID)
	h.Info.Name, h.Info.Inbox = name, inbox
	return h.Info, nil
}

// UserChanged tells the manager a user's name or email changed, so spaces whose
// default author they are (the first owner, as openSpace and SpaceOwner have it)
// and that are open commit under the new identity.
func (m *Manager) UserChanged(ctx context.Context, u store.User) {
	if m.single != nil {
		return
	}
	m.forgetAuthor(u.ID)
	l, err := m.List(ctx, u.ID)
	if err != nil {
		return
	}
	for _, in := range l {
		if in.DirectRole != perm.Owner {
			continue
		}
		m.mu.Lock()
		e := m.open[in.ID]
		m.mu.Unlock()
		if e == nil {
			continue
		}
		select {
		case <-e.done:
			if e.sp == nil {
				break
			}
			// Another owner's profile change is none of this space's business.
			if first, _, err := store.SpaceOwner(ctx, m.opt.Store.Q(), in.ID); err != nil || first == nil || first.ID != u.ID {
				break
			}
			id := IdentityOf(u)
			e.sp.SetAuthor(id.Name, id.Email)
		default:
		}
	}
}

// MembershipChanged drops what is cached about these users' spaces, so the
// next request of theirs reads the database. Call it after anything that adds,
// removes or re-roles a membership (a person's, or a group's: then every person
// in the group), with exactly the users it touched: no one else's cache is
// lost, so a change costs the people it touches one query each and everyone
// else nothing.
func (m *Manager) MembershipChanged(userIDs ...string) {
	m.mu.Lock()
	m.dropListsLocked(userIDs...)
	fn := m.onMembers
	m.mu.Unlock()
	if fn != nil {
		fn(userIDs...)
	}
}

// OnMembershipChanged registers fn, called with the people of every
// MembershipChanged after their lists are dropped: the session cache drops
// what they hold outside spaces, which a group may give (custom roles).
func (m *Manager) OnMembershipChanged(fn func(userIDs ...string)) {
	m.mu.Lock()
	m.onMembers = fn
	m.mu.Unlock()
}

func (m *Manager) dropListsLocked(userIDs ...string) {
	m.listGen++
	for _, id := range userIDs {
		m.cache.drop(id)
	}
}

// Close commits what the open spaces have pending and closes them. Call it
// after the HTTP server has stopped. Single-user mode's space belongs to the caller.
// A move of attachments still running when the runner's wait for it ends
// (jobs.Options.StopWait) is left running in its row and resumed at the next
// start: each of its steps is safe to cut short (see assets_move.go).
func (m *Manager) Close() error {
	// The jobs first (the API has closed the runner already when it was its):
	// a move stopped now is queued again for the next start, which it would not
	// be were the manager closed under it.
	m.closeJobs()
	m.mu.Lock()
	m.closed = true // from here on nothing new is acquired
	if m.closeCancel != nil {
		m.closeCancel()
	}
	m.mu.Unlock()
	m.bgMu.Lock()
	m.bgClosed = true
	m.bgMu.Unlock()
	// Spaces that requests still hold are given a few seconds to be released:
	// closing one under a running request would end it half done. If they are
	// not released in time they are closed anyway (changes made after that are
	// refused with space.ErrClosed, before anything is written).
	deadline := time.Now().Add(m.opt.CloseWait)
	for {
		m.mu.Lock()
		held := 0
		for _, e := range m.open {
			held += e.refs
		}
		m.mu.Unlock()
		if held == 0 {
			break
		}
		if time.Now().After(deadline) {
			m.log.Warn("closing spaces that requests still hold: they did not finish in time", "references", held, "waited", m.opt.CloseWait.String())
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	m.bg.Wait() // the background tasks end with the manager (their context ends with it)
	m.mu.Lock()
	open := m.open
	m.open = map[string]*opening{}
	m.mu.Unlock()
	var errs []error
	m.closeAssetRepos()
	for _, e := range open {
		if e.closing != nil {
			<-e.closing // already on its way out
			continue
		}
		<-e.done
		if e.sp != nil {
			if err := e.sp.Close(); err != nil {
				errs = append(errs, err)
			}
			m.mu.Lock()
			m.rememberHead(openID(open, e), e.sp.Head())
			m.mu.Unlock()
		}
	}
	m.mu.Lock()
	lock := m.lock
	m.lock = nil
	m.mu.Unlock()
	if lock != nil {
		unlockDataDir(lock) // releases the lock
	}
	return errors.Join(errs...)
}

// --- closing spaces that are not in use -------------------------------------

// Run closes idle spaces once a minute, and measures the sizes of spaces in
// the background, until ctx ends.
func (m *Manager) Run(ctx context.Context) {
	if m.single != nil {
		return
	}
	sized, counted := make(chan struct{}), make(chan struct{})
	go func() { defer close(sized); m.runSizer(ctx) }()
	go func() { defer close(counted); m.runRecount(ctx) }()
	defer func() { <-sized; <-counted }()
	evicted := make(chan struct{})
	go func() { defer close(evicted); m.RunEvict(ctx) }()
	defer func() { <-evicted }()
	recycled := make(chan struct{})
	go func() { defer close(recycled); m.RunRecycle(ctx) }()
	defer func() { <-recycled }()
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.Trim()
		}
	}
}

// OpenCount is how many spaces are open or being opened.
func (m *Manager) OpenCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.open)
}

// trimSoon, after a space was opened, closes in the background the least
// recently used idle spaces while the open ones are over the memory budget or
// the cap, so the request that opened one does not wait for another to be
// flushed. It closes none for being idle long (Trim does). A space opened
// while it runs makes it run once more: its cost was not counted yet.
func (m *Manager) trimSoon() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	if m.trimming {
		m.trimAgain = true
		m.mu.Unlock()
		return
	}
	m.trimming = true
	m.mu.Unlock()
	if !m.goBg(func() {
		for {
			m.trim(false)
			m.mu.Lock()
			again := m.trimAgain && !m.closed
			m.trimAgain = false
			if !again {
				m.trimming = false
			}
			m.mu.Unlock()
			if !again {
				return
			}
		}
	}) {
		m.mu.Lock()
		m.trimming = false
		m.mu.Unlock()
	}
}

// goBg runs f in the background, tracked: Close waits for it, so nothing runs
// on after the manager is closed (and writes into a data directory that is
// gone). Work that can take long must stop on closedCtx. It reports false,
// without running f, when Close has begun.
func (m *Manager) goBg(f func()) bool {
	m.bgMu.Lock()
	if m.bgClosed {
		m.bgMu.Unlock()
		return false
	}
	m.bg.Add(1)
	m.bgMu.Unlock()
	go func() {
		defer m.bg.Done()
		f()
	}()
	return true
}

// Trim closes the spaces that have been idle for IdleAfter and, while the open
// spaces together cost more than MemoryBudget or more than MaxOpen are open,
// the least recently used ones. A space with a request in flight is never
// closed, however much it costs. Each is flushed first, and one whose pending
// changes cannot be committed (the repository is in the middle of a merge, say)
// stays open and is tried again at the next trim: nothing is closed with
// uncommitted writes it could not flush.
func (m *Manager) Trim() { m.trim(true) }

// trim is Trim; idle false closes spaces only while over the budget or the
// cap (trimSoon).
func (m *Manager) trim(idle bool) {
	if m.single != nil {
		return
	}
	if idle {
		m.trimIndexOnly()
	}
	m.trimMu.Lock()
	defer m.trimMu.Unlock()
	m.refreshCosts()
	now := m.opt.Now()
	skip := map[*opening]bool{} // flush failed: not again in this trim
	for {
		id, e := m.pickIdle(now, idle, skip)
		if e == nil {
			return
		}
		if !m.closeIdle(id, e) {
			skip[e] = true
		}
	}
}

// refreshCosts estimates again what each open space holds. The estimate reads
// counters and stats a file, for each space; it runs outside mu, which it takes
// only to list the spaces and to store the results.
func (m *Manager) refreshCosts() {
	type item struct {
		id string
		e  *opening
	}
	m.mu.Lock()
	items := make([]item, 0, len(m.open))
	for id, e := range m.open {
		if e.closing == nil && opened(e) && e.sp != nil {
			items = append(items, item{id, e})
		}
	}
	m.mu.Unlock()
	costs := make([]int64, len(items))
	for i, it := range items {
		if m.testCost != nil {
			costs[i] = m.testCost(it.id, it.e.sp)
		} else {
			costs[i] = it.e.sp.MemoryCost()
		}
	}
	m.mu.Lock()
	for i, it := range items {
		it.e.cost = costs[i]
	}
	m.mu.Unlock()
}

// opened reports whether the open of e has finished (e.sp and e.err are set).
func opened(e *opening) bool {
	select {
	case <-e.done:
		return true
	default:
		return false
	}
}

// OpenCost is the memory the open spaces hold together, as last estimated.
func (m *Manager) OpenCost() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for _, e := range m.open {
		n += e.cost
	}
	return n
}

// pickIdle chooses the next space to close and pins it (so nobody starts using
// it while it is flushed); nil when there is none. With idle, a space unused
// for IdleAfter (or disabled) is closed whatever the budget.
func (m *Manager) pickIdle(now time.Time, idle bool, skip map[*opening]bool) (string, *opening) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return "", nil
	}
	over := len(m.open) > m.opt.MaxOpen
	var total int64
	for _, e := range m.open {
		total += e.cost
	}
	over = over || total > m.opt.MemoryBudget
	var bestID string
	var best *opening
	for id, e := range m.open {
		if e.refs > 0 || e.closing != nil || skip[e] || !opened(e) || e.sp == nil {
			continue
		}
		expired := idle && (now.Sub(e.last) >= m.opt.IdleAfter || m.disabled[id])
		if !expired && !over {
			continue
		}
		if best == nil || e.last.Before(best.last) {
			bestID, best = id, e
		}
	}
	if best != nil {
		best.refs++
	}
	return bestID, best
}

// closeIdle reports whether it closed the space.
func (m *Manager) closeIdle(id string, e *opening) bool {
	// Flush while pinned; a failure leaves the space open.
	if err := e.sp.Flush(); err != nil {
		m.log.Warn("not closing an idle space: its changes could not be committed yet", "space", id, "error", err)
		m.mu.Lock()
		e.refs-- // not a use: it stays as idle as it was
		m.mu.Unlock()
		return false
	}
	m.mu.Lock()
	e.refs--
	if e.refs > 0 || m.closed {
		m.mu.Unlock() // someone started using it meanwhile
		return true
	}
	e.closing = make(chan struct{})
	m.mu.Unlock()
	m.noteProblems(id, e.sp)
	if err := e.sp.Close(); err != nil {
		m.log.Warn("closing an idle space", "space", id, "error", err)
	}
	m.mu.Lock()
	m.rememberHead(id, e.sp.Head())
	if m.open[id] == e {
		delete(m.open, id)
	}
	close(e.closing)
	m.mu.Unlock()
	return true
}
