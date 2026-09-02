package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// seedSatisfyAnyList writes an access list with satisfyAny and the given rules,
// plus a proxy host that references it host-wide and on a location.
func seedSatisfyAnyList(t *testing.T, h http.Handler, name string, rules []map[string]any) {
	t.Helper()
	list := map[string]any{
		"name":          name,
		"satisfyAny":    true,
		"defaultAction": "deny",
		"basicAuth":     []map[string]string{{"username": "alice", "passwordHash": bcryptFixture}},
		"rules":         rules,
	}
	body, _ := json.Marshal(list)
	if w := do(t, h, "PUT", "/access-lists/"+name, string(body)); w.Code != http.StatusOK {
		t.Fatalf("seed access list: %d %s", w.Code, w.Body.String())
	}
	host := map[string]any{
		"name":        "app",
		"domains":     []string{"app.example.com"},
		"upstream":    map[string]any{"scheme": "http", "host": "10.0.0.5", "port": 8080},
		"accessLists": []string{name},
		"locations": []map[string]any{
			{"path": "/admin", "accessLists": []string{name}},
		},
	}
	body, _ = json.Marshal(host)
	if w := do(t, h, "PUT", "/proxy-hosts/app", string(body)); w.Code != http.StatusOK {
		t.Fatalf("seed proxy host: %d %s", w.Code, w.Body.String())
	}
}

// TestMigrateBasicAuthDetachesFullyMovedList is the regression for the silent
// narrowing: with satisfyAny the list admitted on "IP match OR password", and
// after migration its credentials live in the middleware while allowFrom carries
// the same networks. Leaving the list attached would make those networks
// MANDATORY, so alice - who used to reach the host with a password from
// anywhere - would be refused before the password was ever asked for. Every rule
// moved here, so the list is detached from the host and the location.
func TestMigrateBasicAuthDetachesFullyMovedList(t *testing.T) {
	h, _ := newHandler(t)
	seedSatisfyAnyList(t, h, "staff", []map[string]any{
		{"action": "allow", "cidr": "10.0.0.0/8"},
	})

	w := do(t, h, "POST", "/access-lists/staff/migrate-basic-auth", "")
	if w.Code != http.StatusOK {
		t.Fatalf("apply: %d %s", w.Code, w.Body.String())
	}
	var p migrationPlan
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !p.DetachAccessList {
		t.Fatal("detachAccessList = false, want the fully-moved list detached")
	}

	w = do(t, h, "GET", "/proxy-hosts/app", "")
	var host model.ProxyHost
	if err := json.Unmarshal(w.Body.Bytes(), &host); err != nil {
		t.Fatalf("decode host: %v", err)
	}
	if len(host.AccessLists) != 0 {
		t.Fatalf("host still references the migrated list: %v", host.AccessLists)
	}
	if len(host.Middlewares) != 1 || host.Middlewares[0] != "staff-basic" {
		t.Fatalf("host middlewares = %v, want the new auth middleware", host.Middlewares)
	}
	if len(host.Locations) != 1 {
		t.Fatalf("locations = %+v", host.Locations)
	}
	if len(host.Locations[0].AccessLists) != 0 {
		t.Fatalf("location still references the migrated list: %v", host.Locations[0].AccessLists)
	}
	if len(host.Locations[0].Middlewares) != 1 || host.Locations[0].Middlewares[0] != "staff-basic" {
		t.Fatalf("location middlewares = %v", host.Locations[0].Middlewares)
	}
}

// TestMigrateBasicAuthKeepsListWithDenyRules is the other branch: a list that
// still carries a verdict allowFrom cannot express (a deny rule here) stays
// attached, and the plan says so explicitly rather than leaving an operator to
// discover the narrowing in production.
func TestMigrateBasicAuthKeepsListWithDenyRules(t *testing.T) {
	h, _ := newHandler(t)
	seedSatisfyAnyList(t, h, "mixed", []map[string]any{
		{"action": "allow", "cidr": "10.0.0.0/8"},
		{"action": "deny", "cidr": "203.0.113.0/24"},
	})

	w := do(t, h, "POST", "/access-lists/mixed/migrate-basic-auth?plan=1", "")
	if w.Code != http.StatusOK {
		t.Fatalf("plan: %d %s", w.Code, w.Body.String())
	}
	var p migrationPlan
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	if p.DetachAccessList {
		t.Fatal("detachAccessList = true, want a list with deny rules kept attached")
	}
	var warned bool
	for _, msg := range p.Warnings {
		if strings.Contains(msg, "stays attached") && strings.Contains(msg, "MANDATORY") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("warnings = %v, want an explicit warning that the remaining rules become mandatory", p.Warnings)
	}

	if w := do(t, h, "POST", "/access-lists/mixed/migrate-basic-auth", ""); w.Code != http.StatusOK {
		t.Fatalf("apply: %d %s", w.Code, w.Body.String())
	}
	w = do(t, h, "GET", "/proxy-hosts/app", "")
	var host model.ProxyHost
	if err := json.Unmarshal(w.Body.Bytes(), &host); err != nil {
		t.Fatalf("decode host: %v", err)
	}
	if len(host.AccessLists) != 1 || host.AccessLists[0] != "mixed" {
		t.Fatalf("host access lists = %v, want the list kept", host.AccessLists)
	}
	if len(host.Locations) != 1 || len(host.Locations[0].AccessLists) != 1 {
		t.Fatalf("location lost its access list: %+v", host.Locations)
	}
}
