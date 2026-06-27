package auth

import (
	"net/http/httptest"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

func TestMapRole(t *testing.T) {
	rm := &model.RoleMapping{
		AdminGroups: []string{"proxy-admins"},
		UserGroups:  []string{"gpm-users"},
		DefaultRole: "",
	}
	tests := []struct {
		name   string
		groups []string
		rm     *model.RoleMapping
		want   Role
	}{
		{"admin group wins", []string{"gpm-users", "proxy-admins"}, rm, RoleAdmin},
		{"user group", []string{"gpm-users"}, rm, RoleUser},
		{"no match -> deny (empty default)", []string{"random"}, rm, RoleNone},
		{"nil mapping -> deny", []string{"proxy-admins"}, nil, RoleNone},
		{"default role user", []string{"x"}, &model.RoleMapping{DefaultRole: "user"}, RoleUser},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := MapRole(tc.groups, tc.rm); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestMFASatisfied(t *testing.T) {
	tests := []struct {
		name string
		id   Identity
		want bool
	}{
		{"amr mfa", Identity{AMR: []string{"pwd", "mfa"}}, true},
		{"amr otp", Identity{AMR: []string{"otp"}}, true},
		{"amr only pwd", Identity{AMR: []string{"pwd"}}, false},
		{"acr step-up", Identity{ACR: "urn:okta:loa:2fa"}, true},
		{"acr level 0", Identity{ACR: "0"}, false},
		{"nothing", Identity{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := MFASatisfied(tc.id); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func forwardAuthSpec() model.ForwardAuthSpec {
	return model.ForwardAuthSpec{
		TrustedProxies:  []string{"10.0.0.0/8"},
		UserHeader:      "X-Authentik-Username",
		EmailHeader:     "X-Authentik-Email",
		NameHeader:      "X-Authentik-Name",
		GroupsHeader:    "X-Authentik-Groups",
		GroupsDelimiter: "|",
	}
}

func TestForwardAuthTrustBoundary(t *testing.T) {
	fa := CompileForwardAuth(forwardAuthSpec(), "authentik")

	// Untrusted peer asserting headers -> no identity (anti-spoof).
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "203.0.113.9:5000"
	r.Header.Set("X-Authentik-Username", "attacker")
	if _, ok := fa.Identity(r); ok {
		t.Fatal("must NOT trust identity headers from an untrusted peer")
	}

	// Trusted peer -> identity extracted.
	r = httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.1.2.3:5000"
	r.Header.Set("X-Authentik-Username", "admin")
	r.Header.Set("X-Authentik-Email", "admin@example.com")
	r.Header.Set("X-Authentik-Groups", "proxy-admins|staff")
	id, ok := fa.Identity(r)
	if !ok {
		t.Fatal("expected identity from trusted peer")
	}
	if id.Subject != "admin" || id.Email != "admin@example.com" {
		t.Fatalf("unexpected identity: %+v", id)
	}
	if len(id.Groups) != 2 || id.Groups[0] != "proxy-admins" || id.Groups[1] != "staff" {
		t.Fatalf("unexpected groups: %v", id.Groups)
	}
	// Without a configured AMR header, no MFA is asserted - we never fabricate it.
	if MFASatisfied(id) {
		t.Fatal("forward-auth must not assert MFA without an amrHeader")
	}
}

func TestForwardAuthAMRHeader(t *testing.T) {
	spec := forwardAuthSpec()
	spec.AMRHeader = "X-Authentik-Amr"
	fa := CompileForwardAuth(spec, "authentik")

	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.1.2.3:5000"
	r.Header.Set("X-Authentik-Username", "admin")
	r.Header.Set("X-Authentik-Amr", "pwd, mfa")
	id, ok := fa.Identity(r)
	if !ok {
		t.Fatal("expected identity from trusted peer")
	}
	if len(id.AMR) != 2 || id.AMR[0] != "pwd" || id.AMR[1] != "mfa" {
		t.Fatalf("expected AMR [pwd mfa] from the header, got %v", id.AMR)
	}
	if !MFASatisfied(id) {
		t.Fatal("an asserted mfa AMR token should satisfy MFA")
	}

	// Strip must also remove the AMR header from an untrusted request.
	r2 := httptest.NewRequest("GET", "/", nil)
	r2.Header.Set("X-Authentik-Amr", "mfa")
	fa.Strip(r2)
	if r2.Header.Get("X-Authentik-Amr") != "" {
		t.Fatal("Strip must remove the AMR header")
	}
}

func TestForwardAuthStrip(t *testing.T) {
	fa := CompileForwardAuth(forwardAuthSpec(), "authentik")
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Authentik-Username", "spoof")
	r.Header.Set("X-Authentik-Groups", "admins")
	fa.Strip(r)
	if r.Header.Get("X-Authentik-Username") != "" || r.Header.Get("X-Authentik-Groups") != "" {
		t.Fatal("Strip must remove all identity headers to prevent upstream spoofing")
	}
}

func TestForwardAuthNoUsername(t *testing.T) {
	fa := CompileForwardAuth(forwardAuthSpec(), "authentik")
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:1234" // trusted, but no username asserted
	if _, ok := fa.Identity(r); ok {
		t.Fatal("no identity should be returned when the username header is absent")
	}
}
