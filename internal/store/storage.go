package store

// Storage records of milestone 3.1 (see docs/api.md, "Server storage"): when a
// space was last used, what its copy on the server is, the administrators'
// pin and exemption, and the queries the storage policy and the eviction ask.
// Everything the administration lists or sums comes from these columns, in one
// query: nothing here looks at a directory.

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Cache states of a space's copy on the server, as stored. "" is a space that
// has not been looked at since the column was added: a full clone if it
// follows a remote.
const (
	CacheFull    = "full"
	CachePartial = "partial"
	CacheEvicted = "evicted"
	// CacheEvicting is written before the clone is moved away and replaced by
	// CacheEvicted after it; a crash in between is finished or undone at the
	// next start (spaces.Manager.recoverEvictions).
	CacheEvicting = "evicting"
)

// SpaceStorage is what the storage milestone records about one space.
type SpaceStorage struct {
	LastUsed   time.Time // zero: never recorded
	Cache      string
	Pinned     bool
	Exempt     bool
	IndexBytes int64
	// AssetsRepo: the space has an attachment repository.
	AssetsRepo bool
}

// GetSpaceStorage reads the storage columns of a space.
func GetSpaceStorage(ctx context.Context, q Q, id string) (SpaceStorage, error) {
	var s SpaceStorage
	var used, pinned, exempt, assets int64
	err := q.QueryRow(ctx, `SELECT last_used, cache_state, pinned, exempt, index_bytes,
		(SELECT COUNT(*) FROM remotes ar WHERE ar.space_id = spaces.id AND ar.role = 'assets' AND ar.url <> '') FROM spaces WHERE id = ?`, id).
		Scan(&used, &s.Cache, &pinned, &exempt, &s.IndexBytes, &assets)
	if errors.Is(err, sql.ErrNoRows) {
		return s, ErrNotFound
	}
	if used > 0 {
		s.LastUsed = fromMS(used)
	}
	s.Pinned, s.Exempt, s.AssetsRepo = pinned != 0, exempt != 0, assets != 0
	return s, err
}

// TouchSpaceUsed records that the space was used at this time (never moving it back).
func TouchSpaceUsed(ctx context.Context, q Q, id string, at time.Time) error {
	_, err := q.Exec(ctx, `UPDATE spaces SET last_used = ? WHERE id = ? AND last_used < ?`, ms(at), id, ms(at))
	return err
}

// SetSpaceCache records the state of the space's copy ("" for unknown).
func SetSpaceCache(ctx context.Context, q Q, id, state string) error {
	return withSpace(ctx, q, id, func(q Q) error {
		_, err := q.Exec(ctx, `UPDATE spaces SET cache_state = ? WHERE id = ?`, state, id)
		return err
	})
}

// SetSpaceCacheKept records the state of the space's copy unless the space is
// evicted or being evicted: the record that a clone is partial or full, which
// is written after the space opens, never undoes an eviction marked
// meanwhile. It reports whether it wrote.
func SetSpaceCacheKept(ctx context.Context, q Q, id, state string) (wrote bool, err error) {
	err = withSpace(ctx, q, id, func(q Q) error {
		res, err := q.Exec(ctx, `UPDATE spaces SET cache_state = ? WHERE id = ? AND cache_state NOT IN (?, ?)`, state, id, CacheEvicted, CacheEvicting)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		wrote = n > 0
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return false, nil // a space that is gone is not written
	}
	return wrote, err
}

// SetSpacePinned sets the pin of a space.
func SetSpacePinned(ctx context.Context, q Q, id string, v bool) error {
	return withSpace(ctx, q, id, func(q Q) error {
		_, err := q.Exec(ctx, `UPDATE spaces SET pinned = ? WHERE id = ?`, b2i(v), id)
		return err
	})
}

// SetSpaceExempt sets whether the requirement of a remote leaves the space out.
func SetSpaceExempt(ctx context.Context, q Q, id string, v bool) error {
	res, err := q.Exec(ctx, `UPDATE spaces SET exempt = ? WHERE id = ?`, b2i(v), id)
	return oneRow(res, err)
}

// SetSpaceIndexBytes stores a measurement of the space's index.
func SetSpaceIndexBytes(ctx context.Context, q Q, id string, n int64) error {
	return withSpace(ctx, q, id, func(q Q) error {
		_, err := q.Exec(ctx, `UPDATE spaces SET index_bytes = ? WHERE id = ?`, n, id)
		return err
	})
}

// SpacesInCacheState maps the id of each space in one of these cache states to
// its state (the evicted ones, and the ones whose eviction a crash cut short).
func SpacesInCacheState(ctx context.Context, q Q, states ...string) (map[string]string, error) {
	if len(states) == 0 {
		return map[string]string{}, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(states)), ",")
	args := make([]any, len(states))
	for i, s := range states {
		args[i] = s
	}
	rows, err := q.Query(ctx, `SELECT id, cache_state FROM spaces WHERE cache_state IN (`+ph+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, st string
		if err := rows.Scan(&id, &st); err != nil {
			return nil, err
		}
		out[id] = st
	}
	return out, rows.Err()
}

// CountDrafts is how many drafts the space has (written, not yet committed
// or discarded).
func CountDrafts(ctx context.Context, q Q, spaceID string) (int, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT COUNT(*) FROM drafts WHERE space_id = ?`, spaceID).Scan(&n)
	return n, err
}

// RemotePolicy is the part of the storage policy a query needs: which remote
// is required, and when the requirement began to apply (see PolicyBase).
type RemotePolicy struct {
	Require string // "", "any" or the id of a provider (2.7b's connections: "github")
	All     bool   // scope "all": spaces that existed before Since are asked too
	Since   time.Time
	Grace   time.Duration
	Now     time.Time
}

// Active reports whether anything is required.
func (p RemotePolicy) Active() bool { return p.Require != "" }

// PolicyBase is when the grace period of a space made at created begins: its
// creation when it was made under the requirement, else Since when the
// requirement is for every space; and when the space lost its GitHub remote
// (lostAt, zero when it did not), if that is later. ok is false for a space
// the requirement does not cover (made before a requirement of new spaces only).
func (p RemotePolicy) PolicyBase(created, lostAt time.Time) (time.Time, bool) {
	base, ok := time.Time{}, true
	switch {
	case !created.Before(p.Since):
		base = created
	case p.All:
		base = p.Since
	default:
		return time.Time{}, false
	}
	if lostAt.After(base) {
		base = lostAt
	}
	return base, ok
}

// Deadline is the end of the space's grace period; ok is false when the
// requirement does not cover the space.
func (p RemotePolicy) Deadline(created, lostAt time.Time) (time.Time, bool) {
	base, ok := p.PolicyBase(created, lostAt)
	if !ok {
		return time.Time{}, false
	}
	return base.Add(p.Grace), true
}

// deadlineSQL is the deadline expression of a space s as SQL, with the
// arguments of its placeholders: the later of its start (see PolicyBase) and
// remote_lost_at, plus the grace period; NULL for a space that is not covered.
func deadlineSQL(p RemotePolicy) (string, []any) {
	since, all := ms(p.Since), b2i(p.All)
	return `(CASE WHEN s.created >= CAST(? AS BIGINT) THEN (CASE WHEN s.remote_lost_at > s.created THEN s.remote_lost_at ELSE s.created END)
		WHEN CAST(? AS BIGINT) <> 0 THEN (CASE WHEN s.remote_lost_at > CAST(? AS BIGINT) THEN s.remote_lost_at ELSE CAST(? AS BIGINT) END) END + CAST(? AS BIGINT))`,
		[]any{since, all, since, since, p.Grace.Milliseconds()}
}

// lacksRemote is the SQL that says a space s does not follow a remote of the
// kind the policy requires, with the arguments of its placeholders: any remote,
// or one that goes through the required provider's connection. A remote that
// lost its GitHub access (its installation is no longer its owner's) is none.
func lacksRemote(p RemotePolicy) (string, []any) {
	if p.Require == "any" {
		return `NOT EXISTS (SELECT 1 FROM remotes r WHERE r.space_id = s.id AND r.role = 'content' AND r.url <> '' AND ` + usableRemote + `)`, nil
	}
	return `NOT EXISTS (SELECT 1 FROM remotes r WHERE r.space_id = s.id AND r.role = 'content' AND r.url <> '' AND r.cred_type = 'connection' AND r.provider = ? AND ` + usableRemote + `)`, []any{p.Require}
}

// policyConds is the SQL that says a space s is asked for a remote it does not
// have and is not exempt, and its deadline expression; askedArgs and dlArgs
// are for the placeholders of each, in order.
func policyConds(p RemotePolicy) (asked string, askedArgs []any, deadline string, dlArgs []any) {
	lacks, askedArgs := lacksRemote(p)
	exempt := `s.exempt = 0 AND NOT EXISTS (SELECT 1 FROM space_members om JOIN users ou ON ou.id = om.user_id WHERE om.space_id = s.id AND om.role = 'owner' AND ou.remote_exempt <> 0)`
	// The deadline: creation (or Since for an older space under scope "all", or
	// the loss of its remote, if later) plus the grace period; NULL for a space
	// that is not covered.
	deadline, dlArgs = deadlineSQL(p)
	return lacks + ` AND ` + exempt, askedArgs, deadline, dlArgs
}

// PolicyCounts counts the spaces the policy asks something of now: in their
// grace period, past their deadline, and exempt from a requirement they would
// otherwise be asked for.
func PolicyCounts(ctx context.Context, q Q, p RemotePolicy) (inGrace, overdue, exempt int, err error) {
	if !p.Active() {
		return 0, 0, 0, nil
	}
	asked, aargs, dl, dargs := policyConds(p)
	row := q.QueryRow(ctx, `SELECT
		COALESCE(SUM(CASE WHEN `+dl+` IS NOT NULL AND `+dl+` >= CAST(? AS BIGINT) THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN `+dl+` IS NOT NULL AND `+dl+` < CAST(? AS BIGINT) THEN 1 ELSE 0 END), 0)
		FROM spaces s WHERE `+asked,
		concat(dargs, dargs, ms(p.Now), dargs, dargs, ms(p.Now), aargs)...)
	if err = row.Scan(&inGrace, &overdue); err != nil {
		return
	}
	lacks, largs := lacksRemote(p)
	err = q.QueryRow(ctx, `SELECT COUNT(*) FROM spaces s WHERE `+lacks+` AND (s.exempt <> 0 OR EXISTS (SELECT 1 FROM space_members om JOIN users ou ON ou.id = om.user_id WHERE om.space_id = s.id AND om.role = 'owner' AND ou.remote_exempt <> 0))`, largs...).Scan(&exempt)
	return
}

func concat(parts ...any) []any {
	var out []any
	for _, p := range parts {
		if l, ok := p.([]any); ok {
			out = append(out, l...)
		} else {
			out = append(out, p)
		}
	}
	return out
}

// StorageSums is what the status of the server adds up, in one query.
type StorageSums struct {
	Local, Full, Partial, Evicted int
	CloneBytes, IndexBytes        int64
}

// SumStorage counts the spaces by what their copy is and adds up the stored
// measurements.
func SumStorage(ctx context.Context, q Q) (StorageSums, error) {
	var s StorageSums
	err := q.QueryRow(ctx, `SELECT
		COALESCE(SUM(CASE WHEN NOT EXISTS (SELECT 1 FROM remotes r WHERE r.space_id = s.id AND r.role = 'content' AND r.url <> '') THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN s.cache_state = 'evicted' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN s.cache_state = 'partial' AND EXISTS (SELECT 1 FROM remotes r WHERE r.space_id = s.id AND r.role = 'content' AND r.url <> '') THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN s.cache_state NOT IN ('evicted', 'partial') AND EXISTS (SELECT 1 FROM remotes r WHERE r.space_id = s.id AND r.role = 'content' AND r.url <> '') THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN s.cache_state = 'evicted' THEN 0 ELSE COALESCE(s.size_bytes, 0) END), 0),
		COALESCE(SUM(s.index_bytes), 0)
		FROM spaces s`).Scan(&s.Local, &s.Evicted, &s.Partial, &s.Full, &s.CloneBytes, &s.IndexBytes)
	return s, err
}

// Candidate is a space the eviction may look at.
type Candidate struct {
	ID         string
	LastUsed   time.Time
	CloneBytes int64
}

// EvictionCandidates lists the spaces that follow a remote whose remote is ok,
// that are not pinned, are not evicted and were last used before idleBefore,
// the least recently used first. It is the one query of a round of eviction
// (the checks that need the clone come after it, for one space at a time).
// A space that was never used has last_used 0 and goes first.
func EvictionCandidates(ctx context.Context, q Q, idleBefore time.Time, limit int) ([]Candidate, error) {
	rows, err := q.Query(ctx, `SELECT s.id, s.last_used, COALESCE(s.size_bytes, 0)
		FROM spaces s JOIN remotes r ON r.space_id = s.id AND r.role = 'content'
		WHERE r.url <> '' AND r.state = 'ok' AND s.pinned = 0 AND s.cache_state NOT IN ('evicted', 'evicting') AND s.last_used < ?
		ORDER BY s.last_used, s.id LIMIT ?`, ms(idleBefore), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Candidate
	for rows.Next() {
		var c Candidate
		var used int64
		if err := rows.Scan(&c.ID, &used, &c.CloneBytes); err != nil {
			return nil, err
		}
		if used > 0 {
			c.LastUsed = fromMS(used)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CloneBytesOfRemoteSpaces adds up the stored sizes of the clones of spaces
// that follow a remote and are not evicted: what clonesBytes is measured against.
func CloneBytesOfRemoteSpaces(ctx context.Context, q Q) (int64, error) {
	var n int64
	err := q.QueryRow(ctx, `SELECT COALESCE(SUM(COALESCE(s.size_bytes, 0)), 0) FROM spaces s
		WHERE s.cache_state NOT IN ('evicted') AND EXISTS (SELECT 1 FROM remotes r WHERE r.space_id = s.id AND r.role = 'content' AND r.url <> '')`).Scan(&n)
	return n, err
}

// PolicyFacts is what the requirement of a remote looks at in one space.
type PolicyFacts struct {
	Created     time.Time
	RemoteLost  time.Time // when its GitHub remote was lost; zero if not
	TenantID    string
	Personal    bool
	Exempt      bool   // the space is exempt
	OwnerExempt bool   // an owner is exempt
	CredType    string // of the remote it follows; "" when it follows none
	Provider    string // the provider that remote goes through ("git" or a provider id)
}

// GetPolicyFacts reads them: one query on the space's primary key.
func GetPolicyFacts(ctx context.Context, q Q, id string) (PolicyFacts, error) {
	var f PolicyFacts
	var created, exempt, ownerExempt, lost int64
	var personal sql.NullString
	err := q.QueryRow(ctx, `SELECT s.created, s.remote_lost_at, s.personal_of, s.exempt, s.tenant_id,
		(SELECT COUNT(*) FROM space_members om JOIN users ou ON ou.id = om.user_id WHERE om.space_id = s.id AND om.role = 'owner' AND ou.remote_exempt <> 0),
		COALESCE((SELECT r.cred_type FROM remotes r WHERE r.space_id = s.id AND r.role = 'content' AND r.url <> '' AND `+usableRemote+`), ''),
		COALESCE((SELECT r.provider FROM remotes r WHERE r.space_id = s.id AND r.role = 'content' AND r.url <> '' AND `+usableRemote+`), '')
		FROM spaces s WHERE s.id = ?`, id).Scan(&created, &lost, &personal, &exempt, &f.TenantID, &ownerExempt, &f.CredType, &f.Provider)
	if errors.Is(err, sql.ErrNoRows) {
		return f, ErrNotFound
	}
	f.Created, f.Personal, f.Exempt, f.OwnerExempt = fromMS(created), personal.Valid, exempt != 0, ownerExempt != 0
	if lost > 0 {
		f.RemoteLost = fromMS(lost)
	}
	return f, err
}

// SpaceTenants is the tenant of each of the spaces by id (a space that is gone is
// not in the answer). It is for a cache's own list of spaces, which names them
// by id, so it takes no scope.
func SpaceTenants(ctx context.Context, q Q, ids []string) (map[string]string, error) {
	out := make(map[string]string, len(ids))
	for len(ids) > 0 {
		n := min(len(ids), 500)
		args := make([]any, n)
		for i, id := range ids[:n] {
			args[i] = id
		}
		rows, err := q.Query(ctx, `SELECT id, tenant_id FROM spaces WHERE id IN (?`+strings.Repeat(",?", n-1)+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, tenant string
			if err := rows.Scan(&id, &tenant); err != nil {
				rows.Close()
				return nil, err
			}
			out[id] = tenant
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		ids = ids[n:]
	}
	return out, nil
}

// PinnedSpaceIDs lists the spaces an administrator keeps on the server.
func PinnedSpaceIDs(ctx context.Context, q Q) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT id FROM spaces WHERE pinned <> 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
