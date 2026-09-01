package model

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// A maintenance window has to survive a restart, so both halves of the switch
// must round-trip through the two encodings the store and the API use.
func TestMaintenanceRoundTrips(t *testing.T) {
	h := ProxyHost{ObjectMeta: ObjectMeta{Name: "grafana"}, Domains: []string{"grafana.example.com"}, Maintenance: true}
	yb, err := yaml.Marshal(h)
	if err != nil {
		t.Fatalf("yaml.Marshal(host): %v", err)
	}
	var fromYAML ProxyHost
	if err := yaml.Unmarshal(yb, &fromYAML); err != nil || !fromYAML.Maintenance {
		t.Fatalf("host yaml round-trip lost maintenance (err=%v):\n%s", err, yb)
	}
	jb, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("json.Marshal(host): %v", err)
	}
	var fromJSON ProxyHost
	if err := json.Unmarshal(jb, &fromJSON); err != nil || !fromJSON.Maintenance {
		t.Fatalf("host json round-trip lost maintenance (err=%v): %s", err, jb)
	}

	s := Settings{Maintenance: MaintenanceSettings{Enabled: true, RetryAfterSeconds: 900}}
	syb, err := yaml.Marshal(s)
	if err != nil {
		t.Fatalf("yaml.Marshal(settings): %v", err)
	}
	var sFromYAML Settings
	if err := yaml.Unmarshal(syb, &sFromYAML); err != nil {
		t.Fatalf("settings yaml round-trip: %v", err)
	}
	if !sFromYAML.Maintenance.Enabled || sFromYAML.Maintenance.RetryAfterSeconds != 900 {
		t.Fatalf("settings yaml round-trip lost maintenance:\n%s", syb)
	}
	sjb, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal(settings): %v", err)
	}
	var sFromJSON Settings
	if err := json.Unmarshal(sjb, &sFromJSON); err != nil || !sFromJSON.Maintenance.Enabled {
		t.Fatalf("settings json round-trip lost maintenance (err=%v): %s", err, sjb)
	}
}

// An unset flag must stay off the wire and out of the file: a host or settings
// object nobody has put into maintenance has to serialize byte-identically to
// what it did before the field existed.
func TestMaintenanceOmittedWhenUnset(t *testing.T) {
	hb, err := json.Marshal(ProxyHost{ObjectMeta: ObjectMeta{Name: "grafana"}, Domains: []string{"grafana.example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(hb), "maintenance") {
		t.Fatalf("an unflagged host carries a maintenance key: %s", hb)
	}
	sb, err := yaml.Marshal(DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sb), "maintenance") {
		t.Fatalf("default settings carry a maintenance key:\n%s", sb)
	}
}

func TestMaintenanceRetryAfterValidation(t *testing.T) {
	base := AdminAuthSettings{LocalLoginEnabled: true}
	for _, tc := range []struct {
		name    string
		seconds int
		wantErr bool
	}{
		{"unset", 0, false},
		{"in range", 900, false},
		{"at the cap", maxMaintenanceRetryAfter, false},
		{"negative", -1, true},
		{"over the cap", maxMaintenanceRetryAfter + 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := Settings{AdminAuth: base, Maintenance: MaintenanceSettings{RetryAfterSeconds: tc.seconds}}.Validate()
			if tc.wantErr && err == nil {
				t.Fatalf("retryAfterSeconds %d: expected a validation error", tc.seconds)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("retryAfterSeconds %d: unexpected error: %v", tc.seconds, err)
			}
		})
	}
}
