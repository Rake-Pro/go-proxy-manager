package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"go.yaml.in/yaml/v3"
)

// TestDryRunSharesSaveValidation runs each object through DryRun and then Save:
// both must agree on accept/reject (with the same message), and DryRun must
// never write a file or move HEAD.
func TestDryRunSharesSaveValidation(t *testing.T) {
	dangling := sampleHost("app")
	dangling.TLS = model.TLSSettings{CertificateRef: "nope"}
	tests := []struct {
		name     string
		obj      model.Object
		noGeoDB  bool
		wantPath string
		wantErr  error // checked with errors.Is when set
		wantFail bool
	}{
		{"valid host", sampleHost("app"), false, "proxy-hosts/app.yaml", nil, false},
		{"placeholder secret", sampleIdP("idp", model.Secret("${ENV:IDP_SECRET}")), false, "identity-providers/idp.yaml", nil, false},
		{"invalid object", model.ProxyHost{ObjectMeta: model.ObjectMeta{Name: "app"}}, false, "proxy-hosts/app.yaml", nil, true},
		{"dangling reference", dangling, false, "proxy-hosts/app.yaml", nil, true},
		{"literal secret", sampleIdP("idp", model.Secret("plaintext-secret")), false, "identity-providers/idp.yaml", nil, true},
		{"geo rule without DB", geoAccessList("geoblock"), true, "access-lists/geoblock.yaml", ErrGeoDBUnavailable, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := newTestStore(t)
			ctx := context.Background()
			if tc.noGeoDB {
				st.SetGeoAvailability(func() bool { return false })
			}
			head, _ := st.Head(ctx)

			path, b, _, err := st.DryRun(ctx, tc.obj)
			if after, _ := st.Head(ctx); after != head {
				t.Fatalf("DryRun moved HEAD %q -> %q", head, after)
			}
			if _, statErr := os.Stat(filepath.Join(st.Dir(), tc.wantPath)); !os.IsNotExist(statErr) {
				t.Fatalf("DryRun wrote %s (stat err %v)", tc.wantPath, statErr)
			}
			_, saveErr := st.Save(ctx, tc.obj, Author{})

			if tc.wantFail {
				if err == nil || saveErr == nil || err.Error() != saveErr.Error() {
					t.Fatalf("DryRun err %v, Save err %v: want the same rejection", err, saveErr)
				}
				if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
					t.Fatalf("DryRun err %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil || saveErr != nil {
				t.Fatalf("DryRun err %v, Save err %v: want both to succeed", err, saveErr)
			}
			if path != tc.wantPath {
				t.Fatalf("path = %q, want %q", path, tc.wantPath)
			}
			var m map[string]any
			if err := yaml.Unmarshal(b, &m); err != nil || m["name"] != tc.obj.GetMeta().Name {
				t.Fatalf("yaml = %q (err %v), want a mapping named %q", b, err, tc.obj.GetMeta().Name)
			}
			if m["updatedAt"] == nil {
				t.Fatalf("yaml has no updatedAt stamp:\n%s", b)
			}
		})
	}
}

// TestDryRunKeepsCreatedAt previews an edit of an existing object: the stamped
// createdAt must be the stored one, exactly as Save would keep it.
func TestDryRunKeepsCreatedAt(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	if _, err := st.Save(ctx, sampleHost("app"), Author{}); err != nil {
		t.Fatalf("save: %v", err)
	}
	cfg, _, err := st.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	created := cfg.ProxyHosts[0].CreatedAt

	edit := sampleHost("app")
	edit.Upstream.Port = 9090
	_, b, exists, err := st.DryRun(ctx, edit)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !exists {
		t.Fatal("dry run of an edit must report exists=true")
	}
	var got model.ProxyHost
	if err := yaml.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.CreatedAt.Equal(created) || got.Upstream.Port != 9090 {
		t.Fatalf("createdAt %v port %d, want %v and 9090", got.CreatedAt, got.Upstream.Port, created)
	}
	cfg, _, _ = st.Load(ctx)
	if cfg.ProxyHosts[0].Upstream.Port != 8080 {
		t.Fatalf("dry run changed the stored object: port %d", cfg.ProxyHosts[0].Upstream.Port)
	}
}

// Create refuses a name that is already stored; Save keeps updating it.
func TestCreateRefusesExisting(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	first := sampleHost("app")
	if _, err := st.Create(ctx, first, Author{}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, _, exists, err := st.DryRun(ctx, first)
	if err != nil || !exists {
		t.Fatalf("dry run after create: exists=%v err=%v", exists, err)
	}
	head, _ := st.Head(ctx)
	again := sampleHost("app")
	again.Upstream.Port = 9090
	if _, err := st.Create(ctx, again, Author{}); !errors.Is(err, ErrExists) {
		t.Fatalf("second create: err=%v, want ErrExists", err)
	}
	if after, _ := st.Head(ctx); after != head {
		t.Fatalf("refused create moved HEAD %q -> %q", head, after)
	}
	if _, err := st.Save(ctx, again, Author{}); err != nil {
		t.Fatalf("save (update) after refused create: %v", err)
	}
}
