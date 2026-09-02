//lint:file-ignore SA1019 compat reads/writes of deprecated AccessList.BasicAuth/SatisfyAny during basic-auth-to-middleware migration
package api

import (
	"errors"
	"fmt"
	"net/http"
	"sort"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/store"
)

// basicAuthMiddlewareSuffix is appended to an access list's name to form the
// auth middleware the migration creates. It is fixed rather than caller-supplied
// so the endpoint is idempotent to look at: the name a plan reports is the name
// an apply writes, and a second apply collides visibly instead of forking a
// second copy of the same credentials.
const basicAuthMiddlewareSuffix = "-basic"

// basicAuthAttachment is one host (or host location) that references the access
// list and therefore gains the new auth middleware.
type basicAuthAttachment struct {
	Kind string `json:"kind"` // ProxyHost
	Name string `json:"name"`
	// Path is the location path when the reference is on a location, empty when
	// it is host-wide.
	Path string `json:"path,omitempty"`
}

// basicAuthMigration is the plan (and, after an apply, the result) of moving one
// access list's deprecated basicAuth users into an auth middleware.
type basicAuthMigration struct {
	AccessList string `json:"accessList"`
	Middleware string `json:"middleware"`
	// Users are the usernames carried over, in configuration order. Only the
	// bcrypt hashes move; no credential material is echoed here.
	Users []string `json:"users"`
	// AllowFrom are the list's literal allow CIDRs copied onto the middleware.
	// They are copied ONLY when the list set satisfyAny, which is the sole
	// configuration in which an IP match alone admitted a request.
	AllowFrom []string `json:"allowFrom,omitempty"`
	// SatisfyAny records what the list had, so a plan reader can see why
	// AllowFrom is (or is not) populated.
	SatisfyAny bool `json:"satisfyAny"`
	// AttachTo lists every host and location that referenced the list and will
	// therefore reference the new middleware.
	AttachTo []basicAuthAttachment `json:"attachTo"`
	// DetachAccessList reports whether the access list is REMOVED from those
	// hosts and locations. It is true only when the list set satisfyAny and
	// every rule it carries moved into the middleware's allowFrom: the list
	// evaluated as "IP match OR password", so leaving its now-mandatory rules
	// attached would turn that OR into an AND and lock out password users
	// outside the allow rules. It is false when the list still carries a
	// dimension the middleware cannot express (a deny rule, a source-backed
	// rule, or geo rules), in which case the list stays attached and Warnings
	// says what an operator has to check.
	DetachAccessList bool `json:"detachAccessList"`
	// Warnings name allow rules that could NOT be represented as allowFrom.
	Warnings []string `json:"warnings,omitempty"`
	// Plan is true for a dry run (?plan=1), which changes nothing.
	Plan bool `json:"plan"`
	// Commit is the commit an apply produced; empty for a plan.
	Commit string `json:"commit,omitempty"`
}

// handleMigrateBasicAuth converts one access list's deprecated basicAuth users
// into an auth middleware with `mode: basic`, attaches that middleware wherever
// the list is referenced, and clears basicAuth/satisfyAny from the list - all in
// a SINGLE commit, so the config is never briefly missing the gate.
//
// `?plan=1` is the dry run: it computes and returns exactly the same payload and
// writes nothing.
//
// It is admin-scoped rather than access-lists:write because one call rewrites
// access lists, middlewares AND proxy hosts; a token holding only the
// access-list scope must not be able to attach a middleware to a host through
// this route.
func (d Deps) handleMigrateBasicAuth(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := model.ValidateName(name); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	cfg, _, err := d.Store.Load(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	var list *model.AccessList
	for i := range cfg.AccessLists {
		if cfg.AccessLists[i].Name == name {
			list = &cfg.AccessLists[i]
			break
		}
	}
	if list == nil {
		writeErr(w, http.StatusNotFound, errNotFound("AccessList", name))
		return
	}
	if len(list.BasicAuth) == 0 {
		writeErr(w, http.StatusBadRequest,
			fmt.Errorf("access list %q has no basicAuth users to migrate", name))
		return
	}

	mwName := name + basicAuthMiddlewareSuffix
	if err := model.ValidateName(mwName); err != nil {
		writeErr(w, http.StatusBadRequest,
			fmt.Errorf("cannot derive a middleware name from access list %q: %w", name, err))
		return
	}
	for _, m := range cfg.Middlewares {
		if m.Name == mwName {
			writeErr(w, http.StatusConflict,
				fmt.Errorf("middleware %q already exists: rename or delete it, or migrate access list %q by hand", mwName, name))
			return
		}
	}

	plan := planBasicAuthMigration(cfg, *list, mwName)
	plan.Plan = r.URL.Query().Get("plan") == "1"
	if plan.Plan {
		writeJSON(w, http.StatusOK, plan)
		return
	}

	objs := basicAuthMigrationObjects(cfg, *list, mwName, plan)
	sha, err := d.Store.SaveBatch(r.Context(), objs,
		fmt.Sprintf("AccessList %q: migrate basicAuth to middleware %q", name, mwName), d.author(r))
	if err != nil {
		writeErr(w, migrateStatus(err), err)
		return
	}
	if !d.applyChange(w, "save", "AccessList", name, sha) {
		return
	}
	plan.Commit = sha
	w.Header().Set(commitHeader, sha)
	writeJSON(w, http.StatusOK, plan)
}

// migrateStatus maps a SaveBatch failure onto a status. A follower is already
// refused ahead of the handler (see followerReadOnly), so what is left is either
// a rejected object graph (the caller's config, 400) or a git/disk failure (500).
func migrateStatus(err error) int {
	if errors.Is(err, store.ErrNotFound) {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}

// planBasicAuthMigration computes what the migration would change, without
// touching cfg.
func planBasicAuthMigration(cfg model.Config, list model.AccessList, mwName string) basicAuthMigration {
	p := basicAuthMigration{
		AccessList: list.Name,
		Middleware: mwName,
		SatisfyAny: list.SatisfyAny,
		Users:      make([]string, 0, len(list.BasicAuth)),
		AttachTo:   []basicAuthAttachment{},
	}
	for _, u := range list.BasicAuth {
		p.Users = append(p.Users, u.Username)
	}
	// satisfyAny is the ONLY configuration in which an IP match alone admitted a
	// request, so it is the only one whose allow rules become an auth exemption.
	// Without it the list required BOTH the IP and the password, and copying the
	// networks into allowFrom would turn an AND into an OR - a real widening.
	if list.SatisfyAny {
		p.DetachAccessList = !listNeedsAttachment(list)
		for _, rule := range list.Rules {
			if rule.Action != model.ActionAllow {
				continue
			}
			switch {
			case rule.Source != "":
				p.Warnings = append(p.Warnings, fmt.Sprintf(
					"allow rule backed by source %q is not copied to allowFrom: an auth exemption is a fixed CIDR list, and a fetched feed is not - keep the access list attached if that feed must skip authentication", rule.Source))
			case rule.PathScoped():
				p.Warnings = append(p.Warnings, fmt.Sprintf(
					"allow rule for path(s) %v is not copied to allowFrom: an auth exemption covers the whole host, and copying a path-scoped rule would widen it", rule.Paths))
			default:
				p.AllowFrom = append(p.AllowFrom, rule.CIDR)
			}
		}
	}
	if list.SatisfyAny && !p.DetachAccessList {
		p.Warnings = append(p.Warnings, fmt.Sprintf(
			"access list %q stays attached because it also carries deny, source-backed or geo rules, which an auth exemption cannot express - but it set satisfyAny, so its remaining allow rules become MANDATORY once basicAuth moves out: a password user outside them is now refused before the password is ever asked for. Review the list's rules before relying on this host.", list.Name))
	}
	for _, h := range cfg.ProxyHosts {
		if containsString(h.AccessLists, list.Name) {
			p.AttachTo = append(p.AttachTo, basicAuthAttachment{Kind: "ProxyHost", Name: h.Name})
		}
		for _, loc := range h.Locations {
			if containsString(loc.AccessLists, list.Name) {
				p.AttachTo = append(p.AttachTo, basicAuthAttachment{Kind: "ProxyHost", Name: h.Name, Path: loc.Path})
			}
		}
	}
	return p
}

// listNeedsAttachment reports whether an access list must stay attached to its
// hosts after its basicAuth moves into a middleware, i.e. whether it still
// carries a verdict the middleware's allowFrom cannot represent: a deny rule, a
// rule backed by a fetched source, a source list, or geo rules. Only literal
// allow CIDRs move.
func listNeedsAttachment(list model.AccessList) bool {
	if len(list.Sources) > 0 || list.Geo.HasRules() {
		return true
	}
	for _, r := range list.Rules {
		if r.Action == model.ActionDeny || r.Source != "" {
			return true
		}
	}
	return false
}

// basicAuthMigrationObjects builds the objects SaveBatch writes: the new auth
// middleware, every proxy host that gains a reference to it, and the access list
// with basicAuth/satisfyAny cleared. Order does not matter - SaveBatch merges
// them all onto the current config and validates the result once - but the
// middleware is first so a reader of the commit sees what was created before
// what now references it.
func basicAuthMigrationObjects(cfg model.Config, list model.AccessList, mwName string, plan basicAuthMigration) []model.Object {
	mw := model.Middleware{
		ObjectMeta: model.ObjectMeta{Name: mwName},
		Type:       model.MWTypeAuth,
		Auth: &model.AuthMiddleware{
			Mode:      model.AuthModeBasic,
			AllowFrom: plan.AllowFrom,
			Basic: &model.BasicAuthSpec{
				Users: append([]model.BasicAuthUser(nil), list.BasicAuth...),
				Realm: list.Name,
			},
		},
	}
	objs := []model.Object{mw}

	// One updated copy per affected host, even when a host references the list
	// both host-wide and on a location: the middleware is attached wherever the
	// list was, and the host is written once.
	touched := map[string]*model.ProxyHost{}
	var order []string
	host := func(i int) *model.ProxyHost {
		h := cfg.ProxyHosts[i]
		if got, ok := touched[h.Name]; ok {
			return got
		}
		cp := h
		cp.Middlewares = append([]string(nil), h.Middlewares...)
		cp.AccessLists = append([]string(nil), h.AccessLists...)
		cp.Locations = append([]model.Location(nil), h.Locations...)
		touched[h.Name] = &cp
		order = append(order, h.Name)
		return &cp
	}
	for i := range cfg.ProxyHosts {
		if containsString(cfg.ProxyHosts[i].AccessLists, list.Name) {
			h := host(i)
			if !containsString(h.Middlewares, mwName) {
				h.Middlewares = append(h.Middlewares, mwName)
			}
			// The list's allow CIDRs are now the middleware's allowFrom, and the
			// list itself no longer holds the credentials that made its rules
			// optional. Leaving it attached would make those rules mandatory -
			// a silent narrowing that locks out every password user outside
			// them - so it is detached wherever the middleware lands.
			if plan.DetachAccessList {
				h.AccessLists = withoutString(h.AccessLists, list.Name)
			}
		}
		for j := range cfg.ProxyHosts[i].Locations {
			if !containsString(cfg.ProxyHosts[i].Locations[j].AccessLists, list.Name) {
				continue
			}
			h := host(i)
			loc := h.Locations[j]
			if plan.DetachAccessList {
				loc.AccessLists = withoutString(loc.AccessLists, list.Name)
			}
			if containsString(loc.Middlewares, mwName) {
				h.Locations[j] = loc
				continue
			}
			loc.Middlewares = append(append([]string(nil), loc.Middlewares...), mwName)
			h.Locations[j] = loc
		}
	}
	sort.Strings(order)
	for _, n := range order {
		objs = append(objs, *touched[n])
	}

	cleared := list
	cleared.BasicAuth = nil
	cleared.SatisfyAny = false
	objs = append(objs, cleared)
	return objs
}

// withoutString returns ss with every occurrence of drop removed, as a fresh
// slice (the caller's copy is shared with the loaded config).
func withoutString(ss []string, drop string) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if s != drop {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func containsString(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
