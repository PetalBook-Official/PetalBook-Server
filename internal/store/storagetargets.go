package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// The states of a storage target: a check is under way or has not run, the
// last check passed, or it failed (StateError says why).
const (
	TargetChecking = "checking"
	TargetOK       = "ok"
	TargetBroken   = "broken"
)

// StorageTarget is a place where an owner's spaces get repositories made
// (table storage_targets). Owner is "site" or a tenant's id. App is "site" or
// "tenant" for a GitHub target (whose App made the installation), "" for the
// others. InUse is the target new spaces go to; a tenant has one, and the old
// ones are kept for the spaces still on them. SharedWith are the tenants a
// site-owned target is offered to (GetStorageTarget fills it).
type StorageTarget struct {
	ID, Owner, ProviderID string
	InstallationID        int64
	Namespace             string
	NamespaceID           string
	App                   string
	Prefix                string
	State, StateError     string
	CheckedAt             time.Time
	InUse                 bool
	UpdatedBy             string
	Created, Updated      time.Time
	SharedWith            []string
}

const targetCols = `id, owner, provider_id, installation_id, namespace, namespace_id, app, prefix, state, state_error, checked_at, in_use, updated_by, created, updated`

func scanTarget(sc interface{ Scan(...any) error }) (StorageTarget, error) {
	var t StorageTarget
	var chk, inUse, cr, up int64
	err := sc.Scan(&t.ID, &t.Owner, &t.ProviderID, &t.InstallationID, &t.Namespace, &t.NamespaceID, &t.App, &t.Prefix,
		&t.State, &t.StateError, &chk, &inUse, &t.UpdatedBy, &cr, &up)
	if chk != 0 {
		t.CheckedAt = fromMS(chk)
	}
	t.InUse, t.Created, t.Updated = inUse != 0, fromMS(cr), fromMS(up)
	return t, err
}

// PutStorageTarget stores a target, replacing the one of the same id. An empty
// State is "checking". The tenants it is shared with are not written (see
// SetStorageTargetShares).
func PutStorageTarget(ctx context.Context, q Q, t StorageTarget) error {
	if t.State == "" {
		t.State = TargetChecking
	}
	var checked int64
	if !t.CheckedAt.IsZero() {
		checked = ms(t.CheckedAt)
	}
	_, err := q.Exec(ctx, `INSERT INTO storage_targets(`+targetCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET owner = excluded.owner, provider_id = excluded.provider_id, installation_id = excluded.installation_id,
		namespace = excluded.namespace, namespace_id = excluded.namespace_id, app = excluded.app, prefix = excluded.prefix,
		state = excluded.state, state_error = excluded.state_error, checked_at = excluded.checked_at, in_use = excluded.in_use,
		updated_by = excluded.updated_by, updated = excluded.updated`,
		t.ID, t.Owner, t.ProviderID, t.InstallationID, t.Namespace, t.NamespaceID, t.App, t.Prefix,
		t.State, t.StateError, checked, b2i(t.InUse), t.UpdatedBy, ms(t.Created), ms(t.Updated))
	return err
}

// GetStorageTarget returns a target with the tenants it is shared with, or
// ErrNotFound.
func GetStorageTarget(ctx context.Context, q Q, id string) (StorageTarget, error) {
	t, err := scanTarget(q.QueryRow(ctx, `SELECT `+targetCols+` FROM storage_targets WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	if err != nil {
		return t, err
	}
	rows, err := q.Query(ctx, `SELECT tenant_id FROM storage_target_shares WHERE target_id = ? ORDER BY tenant_id`, id)
	if err != nil {
		return t, err
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return t, err
		}
		t.SharedWith = append(t.SharedWith, s)
	}
	return t, rows.Err()
}

func queryTargets(ctx context.Context, q Q, query string, args ...any) ([]StorageTarget, error) {
	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StorageTarget
	for rows.Next() {
		t, err := scanTarget(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListStorageTargets returns the targets an owner has ("site" or a tenant's
// id), newest first, without the sharing.
func ListStorageTargets(ctx context.Context, q Q, owner string) ([]StorageTarget, error) {
	return queryTargets(ctx, q, `SELECT `+targetCols+` FROM storage_targets WHERE owner = ? ORDER BY created DESC, id`, owner)
}

// ListSharedStorageTargets returns the targets other owners share with a
// tenant, without the sharing.
func ListSharedStorageTargets(ctx context.Context, q Q, tenantID string) ([]StorageTarget, error) {
	return queryTargets(ctx, q, `SELECT `+targetCols+` FROM storage_targets
		WHERE id IN (SELECT target_id FROM storage_target_shares WHERE tenant_id = ?) ORDER BY created DESC, id`, tenantID)
}

// SetStorageTargetState records the result of a check: a state, the error of a
// failed one ("" otherwise) and when it ran. ErrNotFound when there is no such
// target.
func SetStorageTargetState(ctx context.Context, q Q, id, state, stateError string, at time.Time) error {
	res, err := q.Exec(ctx, `UPDATE storage_targets SET state = ?, state_error = ?, checked_at = ?, updated = ? WHERE id = ?`,
		state, stateError, ms(at), ms(at), id)
	return oneRow(res, err)
}

// SetStorageTargetShares makes the tenants a target is shared with exactly
// tenants: it deletes the others and adds the new ones (a tenant that does not
// exist is refused by the foreign key). Only a target of the site (owner
// "site") is shared: for another, or none, it is ErrNotFound and nothing
// changes. The caller runs it in a transaction.
func SetStorageTargetShares(ctx context.Context, q Q, id string, tenants []string) error {
	var owner string
	if err := q.QueryRow(ctx, `SELECT owner FROM storage_targets WHERE id = ?`, id).Scan(&owner); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if owner != "site" {
		return ErrNotFound
	}
	if _, err := q.Exec(ctx, `DELETE FROM storage_target_shares WHERE target_id = ?`, id); err != nil {
		return err
	}
	for _, t := range tenants {
		if _, err := q.Exec(ctx, `INSERT INTO storage_target_shares(target_id, tenant_id) VALUES (?, ?) ON CONFLICT DO NOTHING`, id, t); err != nil {
			return err
		}
	}
	return nil
}

// DeleteStorageTarget removes a target (and its sharing); there being none is
// not an error.
func DeleteStorageTarget(ctx context.Context, q Q, id string) error {
	_, err := q.Exec(ctx, `DELETE FROM storage_targets WHERE id = ?`, id)
	return err
}

// HasUsableStorageTarget reports whether new spaces of the tenant have a
// storage target to go to: the tenant's own target in use, or a site target
// shared with it, that has not failed its check. One indexed query.
func HasUsableStorageTarget(ctx context.Context, q Q, tenantID string) (bool, error) {
	var one int
	err := q.QueryRow(ctx, `SELECT 1 FROM storage_targets WHERE in_use <> 0 AND state <> ?
		AND (owner = ? OR id IN (SELECT target_id FROM storage_target_shares WHERE tenant_id = ?)) LIMIT 1`, TargetBroken, tenantID, tenantID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}
