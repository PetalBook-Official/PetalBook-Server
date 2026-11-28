package settings

import (
	"context"
	"slices"
)

// AssetsInput is what the settings alone cannot say about a tenant when the
// attachments of a new space are decided.
type AssetsInput struct {
	// OwnStorage is true for a tenant that stores on a target of its own (its own
	// organisation or host connection, storage.mode "tenant"). Such a tenant is
	// not bound by the site's StorageAssets value or lock while the site's
	// StorageAssetsOwnExempt is on. Nothing sets it before the tenant targets of
	// 3.6 exist; the callers pass false.
	OwnStorage bool
}

// AssetsRule is where the attachments of a new space of a tenant go.
type AssetsRule struct {
	// Default is what a request that does not say gets.
	Default string
	// Allowed is what the space's owner may ask for, loosest first. A lock leaves
	// only Default.
	Allowed []string
	// LockedBy is the layer whose lock leaves the owner no choice ("" when none).
	LockedBy Layer
}

// Allows reports whether the owner may ask for attachments at v ("space" or
// "repository").
func (r AssetsRule) Allows(v string) bool { return slices.Contains(r.Allowed, v) }

// AssetsRule applies the layers of StorageAssets, widest first: the site's lock
// over everyone, the site's lock for this tenant, the site's default, the
// tenant's default; what is left is the space owner's to choose. Everything but
// the tenant's own value is one map lookup of the cached effective settings.
func (s *Service) AssetsRule(ctx context.Context, tenant string, in AssetsInput) (AssetsRule, error) {
	e, err := s.Get(ctx, tenant, StorageAssets)
	if err != nil {
		return AssetsRule{}, err
	}
	d := e.Def
	value, below, by := e.Value, e.Below, e.LockedBy
	if in.OwnStorage && e.Tenant != nil {
		x, err := s.Get(ctx, tenant, StorageAssetsOwnExempt)
		if err != nil {
			return AssetsRule{}, err
		}
		if on, _ := x.Value.(bool); on {
			// The site's value, its lock and its rule for this tenant do not
			// apply: the tenant's own value and its own rule do.
			value, below, by = d.Default, Lower{Mode: Allow}, ""
			if e.Tenant.Value != nil {
				value = e.Tenant.Value
			}
			if e.Tenant.Lower != nil {
				below = *e.Tenant.Lower
				if below.Mode == Lock {
					by = Tenant
				}
			}
		}
	}
	return choiceRule(d, value, below, by), nil
}

// choiceRule is what a lower layer may pick of a choice setting, given its value
// in effect, the rule below it and the layer whose lock leaves no choice.
func choiceRule(d *Def, value any, below Lower, by Layer) AssetsRule {
	v, _ := value.(string)
	var r AssetsRule
	for _, c := range d.Choices {
		if below.Mode != Lock && below.Allows(d, c) || below.Mode == Allow {
			r.Allowed = append(r.Allowed, c)
		}
	}
	switch {
	case below.Mode == Lock:
		r.Allowed, r.LockedBy = []string{v}, by
	case !slices.Contains(r.Allowed, v) && len(r.Allowed) > 0:
		v = r.Allowed[0]
	}
	r.Default = v
	return r
}

// StorageModeRule is where a new space of the tenant ("" for the site alone)
// may be stored, by the layers of StorageMode: Allowed are the modes the person
// who makes the space may pick, Default the one a request that does not say
// gets. A lock leaves only the locked value.
func (s *Service) StorageModeRule(ctx context.Context, tenant string) (AssetsRule, error) {
	e, err := s.Get(ctx, tenant, StorageMode)
	if err != nil {
		return AssetsRule{}, err
	}
	return ModeRuleOf(e), nil
}

// ModeRuleOf is the rule of an effective StorageMode (the site's own, from
// Service.Site, or a tenant's).
func ModeRuleOf(e *Effective) AssetsRule {
	return choiceRule(e.Def, e.Value, e.Below, e.LockedBy)
}
