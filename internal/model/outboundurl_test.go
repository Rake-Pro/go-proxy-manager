package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A webhook or notification url that is wholly a placeholder passes write-time
// validation (it cannot be shape-checked before it is resolved); every other
// value is checked exactly as before.
func TestOutboundURLPlaceholderValidation(t *testing.T) {
	tests := []struct {
		name    string
		webhook string
		target  NotificationTarget
		wantErr bool
	}{
		{"literal webhook", "https://hooks.example.com/x", NotificationTarget{}, false},
		{"placeholder webhook", "${ENV:HOOK_URL}", NotificationTarget{}, false},
		{"file placeholder webhook", "${FILE:/run/secrets/hook_url}", NotificationTarget{}, false},
		{"mixed webhook without scheme", "${ENV:HOOK_BASE}/x", NotificationTarget{}, true},
		{"literal webhook bad scheme", "ftp://hooks.example.com/x", NotificationTarget{}, true},
		{"placeholder discord", "", NotificationTarget{Name: "d", Type: NotificationTypeDiscord, URL: "${ENV:DISCORD_URL}"}, false},
		{"placeholder ntfy", "", NotificationTarget{Name: "n", Type: NotificationTypeNtfy, URL: "${ENV:NTFY_URL}"}, false},
		{"literal discord without path", "", NotificationTarget{Name: "d", Type: NotificationTypeDiscord, URL: "https://discord.example.com/x"}, true},
		{"mixed discord without scheme", "", NotificationTarget{Name: "d", Type: NotificationTypeDiscord, URL: "${ENV:DISCORD_BASE}/api/webhooks/1/x"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := Settings{
				ExternalBaseURL: "https://gpm.example.com",
				AdminAuth:       AdminAuthSettings{LocalLoginEnabled: true},
			}
			if tc.webhook != "" {
				s.Webhooks = []WebhookConfig{{Name: "w", URL: tc.webhook}}
			}
			if tc.target.Name != "" {
				s.Notifications.Targets = []NotificationTarget{tc.target}
			}
			err := s.Validate()
			if tc.wantErr != (err != nil) {
				t.Fatalf("Validate() = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestOutboundURLResolve(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GPM_SECRET_FILE_ROOTS", dir)
	const token = "receiver-path-token"
	if err := os.WriteFile(filepath.Join(dir, "hook_url"), []byte("https://hooks.example.com/"+token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OUT_HOOK", "https://hooks.example.com/"+token)
	t.Setenv("OUT_DISCORD", "https://discord.example.com/api/webhooks/1/"+token)
	t.Setenv("OUT_DISCORD_NOPATH", "https://discord.example.com/"+token)
	t.Setenv("OUT_FTP", "ftp://hooks.example.com/"+token)
	t.Setenv("OUT_RELATIVE", "/"+token)

	tests := []struct {
		name    string
		resolve func() (string, error)
		want    string
		errHas  []string // substrings the error must carry
	}{
		{"literal unchanged", WebhookConfig{Name: "w", URL: "https://hooks.example.com/lit"}.ResolveURL, "https://hooks.example.com/lit", nil},
		{"mixed is a literal", WebhookConfig{Name: "w", URL: "https://hooks.example.com/${ENV:OUT_HOOK}"}.ResolveURL, "https://hooks.example.com/${ENV:OUT_HOOK}", nil},
		{"env placeholder", WebhookConfig{Name: "w", URL: "${ENV:OUT_HOOK}"}.ResolveURL, "https://hooks.example.com/" + token, nil},
		{"file placeholder", WebhookConfig{Name: "w", URL: "${FILE:" + filepath.Join(dir, "hook_url") + "}"}.ResolveURL, "https://hooks.example.com/" + token, nil},
		{"discord placeholder", NotificationTarget{Name: "d", Type: NotificationTypeDiscord, URL: "${ENV:OUT_DISCORD}"}.ResolveURL, "https://discord.example.com/api/webhooks/1/" + token, nil},
		{"missing env", WebhookConfig{Name: "w", URL: "${ENV:OUT_MISSING}"}.ResolveURL, "", []string{`webhook "w"`, "${ENV:OUT_MISSING}", "not set"}},
		{"file outside roots", NotificationTarget{Name: "n", Type: NotificationTypeNtfy, URL: "${FILE:/etc/hostname}"}.ResolveURL, "", []string{`notification target "n"`, "${FILE:/etc/hostname}", "outside the allowed secret roots"}},
		{"resolved non-http", WebhookConfig{Name: "w", URL: "${ENV:OUT_FTP}"}.ResolveURL, "", []string{`webhook "w"`, "${ENV:OUT_FTP}", "absolute http(s)"}},
		{"resolved relative", NotificationTarget{Name: "g", Type: NotificationTypeGeneric, URL: "${ENV:OUT_RELATIVE}"}.ResolveURL, "", []string{"${ENV:OUT_RELATIVE}", "absolute http(s)"}},
		{"resolved discord without path", NotificationTarget{Name: "d", Type: NotificationTypeDiscord, URL: "${ENV:OUT_DISCORD_NOPATH}"}.ResolveURL, "", []string{`notification target "d"`, "${ENV:OUT_DISCORD_NOPATH}", "/api/webhooks/"}},
		{"resolved ntfy without discord path", NotificationTarget{Name: "n", Type: NotificationTypeNtfy, URL: "${ENV:OUT_DISCORD_NOPATH}"}.ResolveURL, "https://discord.example.com/" + token, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.resolve()
			if tc.errHas == nil {
				if err != nil || got != tc.want {
					t.Fatalf("ResolveURL() = %q, %v; want %q", got, err, tc.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("ResolveURL() = %q, want an error", got)
			}
			if got != "" {
				t.Errorf("ResolveURL() returned %q alongside an error", got)
			}
			for _, s := range tc.errHas {
				if !strings.Contains(err.Error(), s) {
					t.Errorf("error %q does not mention %q", err, s)
				}
			}
			if strings.Contains(err.Error(), token) {
				t.Errorf("error repeats the resolved url: %q", err)
			}
		})
	}
}
