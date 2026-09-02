package auth

import (
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/session"
)

// TestRequireScopeRoleGate checks the two gates compose: the role decides the
// ceiling, the token scopes decide the token's share of it, and neither can
// raise the other.
func TestRequireScopeRoleGate(t *testing.T) {
	tests := []struct {
		name     string
		p        Principal
		required string
		wantOK   bool
	}{
		// Admin session: unconstrained, exactly as before.
		{"admin session read", Principal{Role: RoleAdmin}, "proxy-hosts:read", true},
		{"admin session write", Principal{Role: RoleAdmin}, "proxy-hosts:write", true},
		{"admin session admin scope", Principal{Role: RoleAdmin}, "admin", true},

		// User session: every read, no write, never the admin scope.
		{"user session resource read", Principal{Role: RoleUser}, "proxy-hosts:read", true},
		{"user session settings read", Principal{Role: RoleUser}, "settings:read", true},
		{"user session whole-config read", Principal{Role: RoleUser}, "*:read", true},
		{"user session metrics read", Principal{Role: RoleUser}, "metrics:read", true},
		{"user session resource write", Principal{Role: RoleUser}, "proxy-hosts:write", false},
		// API tokens are credentials, not configuration: the read-only role is
		// barred from them in both directions despite holding *:read.
		{"user session api-tokens read", Principal{Role: RoleUser}, "api-tokens:read", false},
		{"user session api-tokens write", Principal{Role: RoleUser}, "api-tokens:write", false},
		{"user session settings write", Principal{Role: RoleUser}, "certificates:write", false},
		{"user session admin scope", Principal{Role: RoleUser}, "admin", false},

		// Token principals: scope-limited as before.
		{"token with the scope", Principal{Role: RoleAdmin, IsToken: true, Scopes: []string{"proxy-hosts:write"}}, "proxy-hosts:write", true},
		{"token without the scope", Principal{Role: RoleAdmin, IsToken: true, Scopes: []string{"proxy-hosts:read"}}, "proxy-hosts:write", false},
		{"admin-scoped token", Principal{Role: RoleAdmin, IsToken: true, Scopes: []string{"admin"}}, "admin", true},

		// Composition: a write-scoped token presented on a read-only role is
		// still read-only, and a read-only token on an admin role is still
		// read-only. Neither gate can be raised by the other.
		{"write token on a user role", Principal{Role: RoleUser, IsToken: true, Scopes: []string{"admin"}}, "proxy-hosts:write", false},
		{"api-tokens token on a user role", Principal{Role: RoleUser, IsToken: true, Scopes: []string{"api-tokens:read"}}, "api-tokens:read", false},
		{"api-tokens read stays open to admin", Principal{Role: RoleAdmin}, "api-tokens:read", true},
		{"read token on a user role", Principal{Role: RoleUser, IsToken: true, Scopes: []string{"proxy-hosts:read"}}, "proxy-hosts:read", true},
		{"read token on a user role, write required", Principal{Role: RoleUser, IsToken: true, Scopes: []string{"proxy-hosts:read"}}, "proxy-hosts:write", false},
		{"narrow token on an admin role", Principal{Role: RoleAdmin, IsToken: true, Scopes: []string{"proxy-hosts:read"}}, "certificates:read", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := RequireScope(tc.p, tc.required)
			if (err == nil) != tc.wantOK {
				t.Fatalf("RequireScope(%q) err = %v, want ok = %v", tc.required, err, tc.wantOK)
			}
		})
	}
}

func TestRoleScopes(t *testing.T) {
	if got := RoleScopes(RoleAdmin); got != nil {
		t.Errorf("admin scopes = %v, want nil (no role-level limit)", got)
	}
	if got := RoleScopes(RoleUser); len(got) != 1 || got[0] != "*:read" {
		t.Errorf("user scopes = %v, want [*:read]", got)
	}
	// The wildcard grant is only half the rule; the exclusion list is the other.
	if len(ReadOnlyRoleExcludedSubjects) == 0 {
		t.Fatal("ReadOnlyRoleExcludedSubjects is empty; api-tokens must be excluded")
	}
	for _, required := range []string{"api-tokens:read", "api-tokens:write"} {
		if !readOnlyRoleExcludes(required) {
			t.Errorf("%q is not excluded from the read-only role", required)
		}
	}
	for _, required := range []string{"proxy-hosts:read", "*:read", "settings:read"} {
		if readOnlyRoleExcludes(required) {
			t.Errorf("%q was wrongly excluded from the read-only role", required)
		}
	}
}

// TestPrincipalReadOnly checks the flag the SPA reads off GET /api/me.
func TestPrincipalReadOnly(t *testing.T) {
	tests := []struct {
		role Role
		want bool
	}{
		{RoleAdmin, false},
		{RoleUser, true},
	}
	for _, tc := range tests {
		t.Run(string(tc.role), func(t *testing.T) {
			p := principalOf(&session.Session{Roles: []string{string(tc.role)}, CSRFToken: "t"})
			if p.ReadOnly != tc.want {
				t.Fatalf("readOnly = %v for role %q, want %v", p.ReadOnly, tc.role, tc.want)
			}
		})
	}
}
