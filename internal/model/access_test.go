package model

import (
	"strings"
	"testing"
)

func TestAccessListGeoValidate(t *testing.T) {
	tests := []struct {
		name    string
		al      AccessList
		wantErr string
	}{
		{
			name: "no geo block is fine",
			al:   AccessList{ObjectMeta: ObjectMeta{Name: "plain"}},
		},
		{
			name: "valid countryDeny + onUnknown",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "no-sanctioned"},
				Geo:        &AccessListGeo{CountryDeny: []string{"CN", "RU", "KP"}, OnUnknown: ActionAllow},
			},
		},
		{
			name: "valid countryAllow",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "us-only"},
				Geo:        &AccessListGeo{CountryAllow: []string{"US"}},
			},
		},
		{
			name: "empty geo block is fine",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "empty-geo"},
				Geo:        &AccessListGeo{},
			},
		},
		{
			name: "lowercase country code rejected",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "bad"},
				Geo:        &AccessListGeo{CountryDeny: []string{"cn"}},
			},
			wantErr: `invalid geo countryDeny code "cn"`,
		},
		{
			name: "three-letter country code rejected",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "bad"},
				Geo:        &AccessListGeo{CountryAllow: []string{"USA"}},
			},
			wantErr: `invalid geo countryAllow code "USA"`,
		},
		{
			name: "bad onUnknown rejected",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "bad"},
				Geo:        &AccessListGeo{CountryDeny: []string{"CN"}, OnUnknown: "sometimes"},
			},
			wantErr: "geo onUnknown must be allow|deny",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.al.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected success, got: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestAccessListGeoHasRules(t *testing.T) {
	var nilGeo *AccessListGeo
	if nilGeo.HasRules() {
		t.Fatal("nil *AccessListGeo must report HasRules() == false")
	}
	if (&AccessListGeo{}).HasRules() {
		t.Fatal("an AccessListGeo with no country lists must report HasRules() == false")
	}
	if !(&AccessListGeo{CountryAllow: []string{"US"}}).HasRules() {
		t.Fatal("countryAllow alone must report HasRules() == true")
	}
	if !(&AccessListGeo{CountryDeny: []string{"CN"}}).HasRules() {
		t.Fatal("countryDeny alone must report HasRules() == true")
	}
}
