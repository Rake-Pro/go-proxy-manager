package api_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/api"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/store"
	"github.com/Rake-Pro/go-proxy-manager/internal/webhook"
)

// viewerHandler builds the API with a scope gate that grants everything except
// the admin scope - the read-only `user` role, which holds "*:read" and
// therefore "settings:read".
func viewerHandler(t *testing.T) http.Handler {
	t.Helper()
	dir := t.TempDir()
	st := store.New(dir, store.NewExecGit(dir))
	if err := st.Init(context.Background()); err != nil {
		t.Fatalf("store init: %v", err)
	}
	return api.New(api.Deps{
		Store: st,
		Runtime: api.RuntimeConfig{
			ConfigDir:       "/data/config",
			CertDir:         "/data/certs",
			SessionDB:       "/data/sessions.db",
			SecretFileRoots: []string{"/run/secrets"},
		},
		RequireScope: func(_ *http.Request, required string) error {
			if required == model.ScopeAdmin {
				return errors.New("the \"user\" role is read-only")
			}
			return nil
		},
		WebhookStatus: func() any {
			return []webhook.Delivery{{Name: "ci", URL: "https://discord.example.com/api/webhooks/123/s3cr3t-token"}}
		},
		NotificationStatus: func() any {
			return []webhook.Delivery{{Name: "phone", URL: "https://ntfy.example.com/my-private-topic"}}
		},
	})
}

// TestViewerDoesNotSeeDeploymentPaths: GET /runtime rides "settings:read", which
// the read-only viewer role holds. The filesystem layout is admin data, so it is
// omitted for a caller without the admin scope.
func TestViewerDoesNotSeeDeploymentPaths(t *testing.T) {
	h := viewerHandler(t)
	body := do(t, h, "GET", "/runtime", "").Body.String()
	for _, want := range []string{"/data/config", "/data/certs", "/data/sessions.db", "/run/secrets"} {
		if strings.Contains(body, want) {
			t.Fatalf("GET /runtime disclosed %q to a non-admin caller: %s", want, body)
		}
	}
	if !strings.Contains(body, `"secretFileRoots": []`) {
		t.Fatalf("secretFileRoots must stay an empty list rather than vanish: %s", body)
	}
}

// TestViewerDoesNotSeeWebhookURLs: for a Discord, Slack or ntfy receiver the URL
// path IS the credential, so the status routes keep scheme and host and redact
// the rest for a caller without the admin scope.
func TestViewerDoesNotSeeWebhookURLs(t *testing.T) {
	h := viewerHandler(t)
	tests := []struct {
		route  string
		secret string
		host   string
	}{
		{"/webhooks/status", "s3cr3t-token", "discord.example.com"},
		{"/notifications/status", "my-private-topic", "ntfy.example.com"},
	}
	for _, tc := range tests {
		t.Run(tc.route, func(t *testing.T) {
			body := do(t, h, "GET", tc.route, "").Body.String()
			if strings.Contains(body, tc.secret) {
				t.Fatalf("%s disclosed the target URL credential: %s", tc.route, body)
			}
			if !strings.Contains(body, tc.host) {
				t.Fatalf("%s should still name the target host: %s", tc.route, body)
			}
		})
	}
}
