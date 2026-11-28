package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"

	"petalbook.app/server/internal/auth"
	"petalbook.app/server/internal/authz"
	"petalbook.app/server/internal/settings"
	"petalbook.app/server/internal/spaces"
	"petalbook.app/server/internal/store"
)

// The layered settings of docs/api.md ("Settings by layer").

// failSetting answers a refusal of the settings package.
func (s *server) failSetting(c *call, err error) {
	var se *settings.Error
	if !errors.As(err, &se) {
		s.fail(c.w, err)
		return
	}
	status := http.StatusBadRequest
	switch se.Code {
	case settings.CodeNotFound:
		status = http.StatusNotFound
	case settings.CodeConflict, settings.CodeLocked, settings.CodeRestricted, "mail_not_configured", "sso_only_with_registration", "needs_tenancy", "needs_mail":
		status = http.StatusConflict
	}
	writeError(c.w, status, se.Code, se.Msg)
}

type siteSettingResp struct {
	Key    string            `json:"key"`
	Kind   settings.Kind     `json:"kind"`
	Layers []settings.Layer  `json:"layers"`
	Site   settings.SiteView `json:"site"`
	// Choices are the values of a choice setting, loosest first.
	Choices []string `json:"choices,omitempty"`
	// A setting with no tenant layer has one of these: ForTenant, a site
	// administrator may set it for one tenant (siteRule or siteLock on the
	// tenant's settings); ServerWide, why it is the same for every tenant.
	ForTenant  bool   `json:"forTenant,omitempty"`
	ServerWide string `json:"serverWide,omitempty"`
	// Strictness says what stricter means for the key (higher, lower, order,
	// subset), which the lower mode tighten needs; none: it cannot be tightened.
	Strictness settings.Strictness `json:"strictness,omitempty"`
	// OwnPeopleOnly: the site's own value governs the site's own people and
	// holds no organisation, so the site's layer takes no rule (only allow); a
	// rule for one tenant holds it.
	OwnPeopleOnly bool `json:"ownPeopleOnly,omitempty"`
}

type tenantSettingResp struct {
	Key       string               `json:"key"`
	Kind      settings.Kind        `json:"kind"`
	Layers    []settings.Layer     `json:"layers"`
	Site      tenantSiteView       `json:"site"`
	Tenant    *settings.TenantView `json:"tenant"`
	Effective any                  `json:"effective"`
	// Choices are the values of a choice setting, loosest first.
	Choices    []string            `json:"choices,omitempty"`
	Strictness settings.Strictness `json:"strictness,omitempty"`
	// Reason says why the tenant's layer cannot be set freely: the rule it is
	// held to (the site's, or its rule for this tenant) with the site's note;
	// absent when the site allows everything.
	Reason *reasonView `json:"reason,omitempty"`
}

// reasonView is the rule above a tenant's layer as the tenant is shown it:
// who made it, its mode, options or max and note, and under tighten the floor,
// the value the tenant's own must be at least as strict as.
type reasonView struct {
	// By is "site" when the site's own layer holds the tenant, "rule" when the
	// site's rule for this tenant does.
	By string `json:"by"`
	settings.Lower
	Floor any `json:"floor,omitempty"`
}

// tenantSettingOf is a setting as the API shows it for a tenant. Who set a rule
// (a site administrator's id) is shown to site administrators only: for
// anyone else the rules are copies without it (the cached view is shared).
func tenantSettingOf(e *settings.Effective, showSetBy bool) tenantSettingResp {
	d := e.Def
	tv := e.Tenant
	if !showSetBy && tv != nil && slices.ContainsFunc(tv.Rules, func(r settings.RuleView) bool { return r.SetBy != "" }) {
		cp := *tv
		cp.Rules = slices.Clone(tv.Rules)
		for i := range cp.Rules {
			cp.Rules[i].SetBy = ""
		}
		cp.SiteRule = nil
		tv = &cp
	}
	var reason *reasonView
	if e.Above.Mode != settings.Allow {
		reason = &reasonView{By: "site", Lower: e.Above}
		if e.ByRule {
			reason.By = "rule"
		}
		if e.Above.Mode == settings.Tighten {
			reason.Floor = e.Floor
		}
	}
	return tenantSettingResp{d.Key, d.Kind, d.Layers, tenantSiteView{e.Site.Value, e.Site.Lower}, tv, e.Value, d.Choices, d.Strict, reason}
}

// tenantSiteView is what a tenant is shown of the site's layer: the value and
// the rule, not whether a flag set it.
type tenantSiteView struct {
	Value any            `json:"value"`
	Lower settings.Lower `json:"lower"`
}

func (s *server) adminGetSettings(c *call) {
	eff, err := s.settings.For(c.r.Context(), "")
	if err != nil {
		s.fail(c.w, err)
		return
	}
	out := []siteSettingResp{}
	for _, d := range s.settings.Defs() {
		e := eff[d.Key]
		out = append(out, siteSettingResp{d.Key, d.Kind, d.Layers, e.Site, d.Choices, d.ForTenant, d.ServerWide, d.Strict, d.TenantDefault != nil})
	}
	writeJSON(c.w, http.StatusOK, struct {
		Settings []siteSettingResp `json:"settings"`
	}{out})
}

// readChange reads {"value": …, "lower": {…}} and, for a tenant's layer,
// "siteLock": which of them were given.
func (s *server) readChange(c *call, tenantLayer bool) (settings.Change, string, bool) {
	var req map[string]json.RawMessage
	if !s.readJSONMax(c.w, c.r, &req, false, authBodyLimit) {
		return settings.Change{}, "", false
	}
	var ch settings.Change
	for k := range req {
		if k != "value" && k != "lower" && k != "confirm" && !(tenantLayer && (k == "siteLock" || k == "siteRule")) {
			writeError(c.w, http.StatusBadRequest, "invalid", "Unknown field "+k+".")
			return ch, "", false
		}
	}
	// confirm is the confirmation of switching off a feature that is in use (capabilities.go).
	var tok string
	if raw, ok := req["confirm"]; ok && json.Unmarshal(raw, &tok) != nil {
		writeError(c.w, http.StatusBadRequest, "invalid", "Confirm is the token of POST /api/admin/confirm.")
		return ch, "", false
	}
	if raw, ok := req["value"]; ok {
		ch.HasValue = true
		if err := json.Unmarshal(raw, &ch.Value); err != nil {
			writeError(c.w, http.StatusBadRequest, "invalid", "Value is not JSON.")
			return ch, "", false
		}
	}
	if raw, ok := req["lower"]; ok {
		ch.HasLower = true
		if err := json.Unmarshal(raw, &ch.Lower); err != nil {
			writeError(c.w, http.StatusBadRequest, "invalid", "Lower is {\"mode\": …}.")
			return ch, "", false
		}
	}
	if raw, ok := req["siteLock"]; ok {
		ch.HasSiteLock = true
		if err := json.Unmarshal(raw, &ch.SiteLock); err != nil {
			writeError(c.w, http.StatusBadRequest, "invalid", "SiteLock is true or false.")
			return ch, "", false
		}
	}
	if raw, ok := req["siteRule"]; ok {
		// {"value": …, "lower": {…}, "from": …, "until": …}, any of them; null
		// removes the manual rule. from and until are RFC 3339 times.
		ch.HasSiteRule = true
		if err := json.Unmarshal(raw, &ch.SiteRule); err != nil {
			writeError(c.w, http.StatusBadRequest, "invalid", "SiteRule is {\"value\": …, \"lower\": {…}, \"from\": …, \"until\": …} or null, the times in RFC 3339.")
			return ch, "", false
		}
	}
	if !ch.HasValue && !ch.HasLower && !ch.HasSiteLock && !ch.HasSiteRule {
		writeError(c.w, http.StatusBadRequest, "invalid", "Give value, lower, siteLock, siteRule or any of them.")
		return ch, "", false
	}
	return ch, tok, true
}

// beforeOpening is the checks of a setting that opens something: organisation
// creation needs tenants and mail, and a plan must exist. The registration
// mode needs none: an account made by a sign-in needs no mail, and password
// sign-up checks mail when it is used.
func (s *server) beforeOpening(ctx context.Context, key string, ch settings.Change, site bool) error {
	switch key {
	case settings.FeatureOrgCreation:
		return s.beforeOrgCreation(ctx, ch)
	case settings.OrgCreationPlan:
		// A plan that does not exist is refused, as a tenant's plan is.
		if slug, _ := ch.Value.(string); ch.HasValue && slug != "" {
			if _, err := store.GetPlanBySlug(ctx, s.opt.Store.Q(), slug); errors.Is(err, store.ErrNotFound) {
				return &settings.Error{Code: settings.CodeInvalid, Msg: "There is no plan with that slug."}
			} else if err != nil {
				return err
			}
		}
		return nil
	}
	return nil
}

// beforeOrgCreation is the check of a value that turns organisation creation
// on: it needs tenants (features.tenancy) and mail, which the registration
// that makes one is confirmed by.
func (s *server) beforeOrgCreation(ctx context.Context, ch settings.Change) error {
	if !(ch.HasValue && ch.Value == true) {
		return nil
	}
	if on, err := s.settings.Get(ctx, "", settings.FeatureTenancy); err != nil {
		return err
	} else if b, _ := on.Value.(bool); !b {
		return &settings.Error{Code: "needs_tenancy", Msg: "Organizations people create need the multiple tenants feature; switch it on first."}
	}
	if !s.registrationReady() {
		return &settings.Error{Code: "needs_mail", Msg: "Organizations people create are confirmed by mail, and mail is not enabled."}
	}
	if s.auth.SSOOnlyStored(ctx) {
		return &settings.Error{Code: "sso_only_with_registration", Msg: "Password sign-in is refused (ssoOnly), so nobody could finish registering; switch ssoOnly off first."}
	}
	return nil
}

// closedRegistration cancels the pending registrations of tenants that are not
// open now, after a change that may have closed one.
func (s *server) closedRegistration(ctx context.Context, key string) {
	if key != settings.RegistrationMode && key != settings.FeatureOrgCreation && key != settings.FeatureTenancy {
		return
	}
	if err := s.auth.CancelClosedRegistrations(ctx); err != nil {
		s.log.Warn("could not cancel pending registrations", "error", err)
	}
}

func (s *server) adminPutSetting(c *call) {
	key := c.r.PathValue("key")
	ch, tok, ok := s.readChange(c, false)
	if !ok {
		return
	}
	ctx := c.r.Context()
	if _, ok := s.settings.Def(key); !ok {
		s.failSetting(c, &settings.Error{Code: settings.CodeNotFound, Msg: "No such setting."})
		return
	}
	if err := s.beforeOpening(ctx, key, ch, true); err != nil {
		s.failSetting(c, err)
		return
	}
	if !s.capabilityGate(c, key, "", ch, tok) {
		return
	}
	before, err := s.settings.Get(ctx, "", key)
	if err != nil {
		s.failSetting(c, err)
		return
	}
	ch.By = c.user().ID
	affected, err := s.settings.SetSite(ctx, key, ch)
	if err != nil {
		s.failSetting(c, err)
		return
	}
	s.closedRegistration(ctx, key)
	s.quotaSettingChanged(ctx, key, store.AllTenants())
	after, err := s.settings.Get(ctx, "", key)
	if err != nil {
		s.failSetting(c, err)
		return
	}
	s.audit(c, auth.AuditEvent{Kind: "setting_changed", Actor: c.user().ID, Tenant: "-",
		Detail: noted(map[string]any{"key": key, "layer": "site", "source": "manual", "from": before.Site, "to": after.Site}, ch.Lower, ch.HasLower)})
	d, _ := s.settings.Def(key)
	out := struct {
		siteSettingResp
		Affected *int `json:"affected,omitempty"`
		// Remotes is how many spaces the requirement of a remote now asks
		// something of, with a change of storage.remotes.*.
		Remotes *spaces.Affected `json:"remotes,omitempty"`
	}{siteSettingResp: siteSettingResp{d.Key, d.Kind, d.Layers, after.Site, d.Choices, d.ForTenant, d.ServerWide, d.Strict, d.TenantDefault != nil}}
	if ch.HasLower {
		out.Affected = &affected
	}
	// storage.mode's modes allowed derive the requirement when none is set.
	if (ch.HasValue && (strings.HasPrefix(key, "storage.remotes.") || key == settings.StorageHosts) || key == settings.StorageMode && (ch.HasValue || ch.HasLower)) && s.spaces != nil {
		// The requirement changed: what is cached of it goes, the people's counts
		// follow it.
		aff, err := s.spaces.StoragePolicyChanged(ctx)
		if err != nil {
			s.failSetting(c, err)
			return
		}
		out.Remotes = &aff
	}
	writeJSON(c.w, http.StatusOK, out)
}

func (s *server) tenantSettingsOf(ctx context.Context, tenant string, showSetBy bool) ([]tenantSettingResp, error) {
	eff, err := s.settings.For(ctx, tenant)
	if err != nil {
		return nil, err
	}
	out := []tenantSettingResp{}
	for _, d := range s.settings.Defs() {
		if !d.PerTenant() {
			continue
		}
		e := eff[d.Key]
		out = append(out, tenantSettingOf(e, showSetBy))
	}
	return out, nil
}

func (s *server) adminGetTenantSettings(c *call) {
	t, ok := s.visibleTenant(c)
	if !ok {
		return
	}
	out, err := s.tenantSettingsOf(c.r.Context(), t.ID, c.siteWide(authz.TenantSettingsWrite))
	if err != nil {
		s.fail(c.w, err)
		return
	}
	writeJSON(c.w, http.StatusOK, struct {
		Settings []tenantSettingResp `json:"settings"`
	}{out})
}

func (s *server) adminPutTenantSetting(c *call) {
	t, ok := s.visibleTenant(c)
	if !ok {
		return
	}
	key := c.r.PathValue("key")
	ch, tok, ok := s.readChange(c, true)
	if !ok {
		return
	}
	ctx := c.r.Context()
	before, err := s.settings.Get(ctx, t.ID, key)
	if err != nil {
		s.failSetting(c, err)
		return
	}
	// What the site locks is refused as locked, before anything else is asked of it.
	if before.Site.Lower.Mode != settings.Lock {
		if err := s.beforeOpening(ctx, key, ch, false); err != nil {
			s.failSetting(c, err)
			return
		}
	}
	ch.Site, ch.By = c.siteWide(authz.TenantSettingsWrite), c.user().ID
	if ch.Site && !s.capabilityGate(c, key, t.ID, ch, tok) {
		return
	}
	res, err := s.settings.SetTenantResult(ctx, t.ID, key, ch)
	if err != nil {
		s.failSetting(c, err)
		return
	}
	s.closedRegistration(ctx, key)
	s.quotaSettingChanged(ctx, key, store.InTenant(t.ID))
	after, err := s.settings.Get(ctx, t.ID, key)
	if err != nil {
		s.failSetting(c, err)
		return
	}
	detail := map[string]any{"key": key, "layer": "tenant", "from": before.Tenant, "to": after.Tenant}
	if ch.HasSiteRule {
		// A rule for the tenant: its source and window.
		detail["source"] = "manual"
		if r := ch.SiteRule; r != nil && (!r.From.IsZero() || !r.Until.IsZero()) {
			detail["window"] = map[string]any{"from": optTime(r.From), "until": optTime(r.Until)}
		}
	}
	if ch.HasSiteRule && ch.SiteRule != nil {
		noted(detail, ch.SiteRule.Lower, true)
	} else {
		noted(detail, ch.Lower, ch.HasLower)
	}
	s.audit(c, auth.AuditEvent{Kind: "setting_changed", Actor: c.user().ID, Tenant: t.ID, Detail: detail})
	out := struct {
		tenantSettingResp
		Warning *warningResp `json:"warning,omitempty"`
	}{tenantSettingResp: tenantSettingOf(after, ch.Site)}
	if res.UndatedReplaced {
		out.Warning = &warningResp{"undated_value_replaced", []string{key},
			"There is one manual rule for each setting. A dated rule replaced the undated one: when its window ends, the value is not the undated one but what is below it (billing, the plan, the site)."}
	}
	writeJSON(c.w, http.StatusOK, out)
}

// noted adds the note a change sent with its rule to the audit detail (the
// rule itself, note included, is in from and to).
func noted(detail map[string]any, l *settings.Lower, given bool) map[string]any {
	if given && l != nil && l.NoteSet {
		detail["note"] = l.Note
	}
	return detail
}

// warningResp is a warning that comes with a change that went through.
type warningResp struct {
	Code    string   `json:"code"`
	Keys    []string `json:"keys"`
	Message string   `json:"message"`
}

// mySettings is GET /api/settings: what the caller may know of the settings in
// their context. features is every feature (features.*, mail.ownSender) as it
// stands for the caller's tenant, true or false: what the web app shows or hides
// (useFeature). settings is what each setting that has a space or person layer
// comes to for the caller (and for a space, when one is given), and whether that
// layer may change it, so a client greys out "set by the administrators".
//
// It is made from the tenant's cached settings (no query), and carries their
// version as its ETag: a client that sends it back is answered 304, without
// the work of making the answer.
func (s *server) mySettings(c *call) {
	ctx := c.r.Context()
	if sp := c.r.URL.Query().Get("space"); sp != "" {
		h, err := s.spaces.Open(ctx, c.user(), sp)
		if err != nil {
			s.fail(c.w, err)
			return
		}
		h.Release()
		if c.viewAs != nil && !s.viewAsShows(c, &h.Info) {
			writeError(c.w, http.StatusNotFound, "not_found", "No such space.")
			return
		}
	}
	tenant := c.tenant()
	ver, err := s.settings.Version(ctx, tenant)
	if err != nil {
		s.fail(c.w, err)
		return
	}
	// Single-user mode has every feature on, and a version of its own.
	if s.single {
		ver += "s"
	}
	h := c.w.Header()
	h.Set("ETag", `"`+ver+`"`)
	h.Set("Cache-Control", "private, no-cache")
	if noneMatch(c.r.Header.Get("If-None-Match"), ver) {
		c.w.WriteHeader(http.StatusNotModified)
		return
	}
	eff, err := s.settings.For(ctx, tenant)
	if err != nil {
		s.fail(c.w, err)
		return
	}
	type layerView struct {
		CanChange bool `json:"canChange"`
		InEffect  bool `json:"inEffect"`
		// Source is the layer that keeps it from changing: a lock, or a value that
		// switches it off ("site" or "tenant"); absent when nothing does.
		Source settings.Layer `json:"source,omitempty"`
	}
	type item struct {
		Key       string     `json:"key"`
		Effective any        `json:"effective"`
		Space     *layerView `json:"space,omitempty"`
		Person    *layerView `json:"person,omitempty"`
		// Allowed and Locked are for storage.assets: what the person may ask for when they make a space,
		// and whether a lock leaves them no choice.
		Allowed []string `json:"allowed,omitempty"`
		Locked  *bool    `json:"locked,omitempty"`
		// Missing, for storage.mode, are the allowed modes that cannot be used
		// now (tenant, while the organization has no storage target).
		Missing []string `json:"missing,omitempty"`
	}
	out := []item{}
	features := map[string]bool{}
	for _, d := range s.settings.Defs() {
		if d.IsFeature() {
			on, _ := eff[d.Key].Value.(bool)
			features[d.Key] = on || s.single
		}
		if d.Key == settings.StorageAssets {
			r, err := s.assetsRule(ctx, c.tenant())
			if err != nil {
				s.fail(c.w, err)
				return
			}
			locked := r.LockedBy != ""
			out = append(out, item{Key: d.Key, Effective: r.Default, Allowed: r.Allowed, Locked: &locked})
			continue
		}
		if d.Key == settings.StorageMode && s.spaces != nil {
			r, err := s.spaces.ModeRule(ctx, c.tenant())
			if err != nil {
				s.fail(c.w, err)
				return
			}
			out = append(out, item{Key: d.Key, Effective: r.Default, Allowed: r.Allowed, Locked: &r.Locked, Missing: r.Missing})
			continue
		}
		if !d.Has(settings.Space) && !d.Has(settings.Person) || strings.HasPrefix(d.Key, "quota.") {
			continue // the quotas' person layer is an administrator's override, not the person's own
		}
		e := eff[d.Key]
		it := item{Key: d.Key, Effective: e.Value}
		// A boolean is what the layers below may do: when it is off they cannot
		// use it, and what they saved stays without effect.
		on, _ := e.Value.(bool)
		lv := &layerView{CanChange: on && e.Below.Mode != settings.Lock, InEffect: on}
		switch {
		case e.LockedBy != "":
			lv.Source = e.LockedBy
		case !on:
			lv.Source = e.Source
		}
		if d.Has(settings.Space) {
			it.Space = lv
		}
		if d.Has(settings.Person) {
			it.Person = lv
		}
		out = append(out, it)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(struct {
		Features map[string]bool `json:"features"`
		Settings []item          `json:"settings"`
	}{features, out}); err != nil {
		s.fail(c.w, err)
		return
	}
	h.Set("Content-Type", "application/json; charset=utf-8")
	c.w.WriteHeader(http.StatusOK)
	c.w.Write(buf.Bytes())
}

var _ = store.DefaultTenantID
