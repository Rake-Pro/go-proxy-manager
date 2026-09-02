package model

import (
	"reflect"
	"sort"
	"testing"
)

func TestUnknownYAMLKeys(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		out  any
		want []string
	}{
		{
			name: "no unknown keys",
			yaml: "name: app\ndomains: [app.example.com]\nupstream:\n  scheme: http\n  host: 10.0.0.5\n  port: 8080\n",
			out:  &ProxyHost{},
			want: nil,
		},
		{
			name: "top-level unknown key",
			yaml: "name: app\nfutureField: true\n",
			out:  &ProxyHost{},
			want: []string{"futureField"},
		},
		{
			name: "unknown key nested under a struct field",
			yaml: "name: app\nupstream:\n  scheme: http\n  host: x\n  port: 80\n  futureThing: 1\n",
			out:  &ProxyHost{},
			want: []string{"upstream.futureThing"},
		},
		{
			name: "unknown key inside a slice of structs is indexed",
			yaml: "name: app\nlocations:\n  - path: /\n    newField: 1\n  - path: /api\n    anotherNewField: 2\n",
			out:  &ProxyHost{},
			want: []string{"locations[0].newField", "locations[1].anotherNewField"},
		},
		{
			name: "unknown key on the embedded ObjectMeta is flattened, not nested",
			yaml: "name: app\ndisplayName: App\nbogus: 1\n",
			out:  &ProxyHost{},
			want: []string{"bogus"},
		},
		{
			name: "map[string]string-typed field is opaque",
			yaml: "name: app\nlabels:\n  anything: goes\n  arbitrary-key: also-fine\n",
			out:  &ProxyHost{},
			want: nil,
		},
		{
			name: "map of structs is opaque, including its nested keys",
			yaml: "name: app\nsecurityHeaders:\n  x-frame-options:\n    value: DENY\n    unknownNestedKey: true\n",
			out:  &ProxyHost{},
			want: nil,
		},
		{
			name: "unknown key behind a pointer-to-struct field",
			yaml: "name: app\nauth:\n  identityProvider: idp\n  bogusModeField: 1\n",
			out:  &ProxyHost{},
			want: []string{"auth.bogusModeField"},
		},
		{
			name: "multiple unknown keys across different files/fields",
			yaml: "name: app\ntopLevelBogus: 1\nupstream:\n  scheme: http\n  host: x\n  port: 80\n  nestedBogus: 2\n",
			out:  &ProxyHost{},
			want: []string{"topLevelBogus", "upstream.nestedBogus"},
		},
		{
			name: "settings.yaml: unknown key nested under a settings section",
			yaml: "schemaVersion: 1\nexternalBaseURL: https://gpm.example.com\nadminAuth:\n  providers: []\n  localLoginEnabled: true\n  newSsoField: true\n",
			out:  &Settings{},
			want: []string{"adminAuth.newSsoField"},
		},
		{
			name: "settings.yaml: no unknown keys",
			yaml: "schemaVersion: 1\nexternalBaseURL: https://gpm.example.com\nadminAuth:\n  localLoginEnabled: true\n",
			out:  &Settings{},
			want: nil,
		},
		{
			name: "empty file yields no warnings",
			yaml: "",
			out:  &ProxyHost{},
			want: nil,
		},
		{
			name: "unparsable YAML yields no warnings (the real Unmarshal already reports the parse failure)",
			yaml: "name: [unterminated\n",
			out:  &ProxyHost{},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UnknownYAMLKeys([]byte(tt.yaml), tt.out)
			sort.Strings(got)
			want := append([]string(nil), tt.want...)
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("UnknownYAMLKeys() = %v, want %v", got, want)
			}
		})
	}
}
