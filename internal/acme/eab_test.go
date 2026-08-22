package acme

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

func TestExternalAccountBinding(t *testing.T) {
	want := []byte("super-secret-hmac-key")

	cases := map[string]string{
		"raw base64url": base64.RawURLEncoding.EncodeToString(want),
		"base64url":     base64.URLEncoding.EncodeToString(want),
		"std base64":    base64.StdEncoding.EncodeToString(want),
	}
	for name, encoded := range cases {
		t.Run(name, func(t *testing.T) {
			eab, err := externalAccountBinding(&model.EABSpec{KID: "kid-1", HMACKey: model.Secret(encoded)})
			if err != nil {
				t.Fatal(err)
			}
			if eab.KID != "kid-1" {
				t.Errorf("KID = %q", eab.KID)
			}
			if string(eab.Key) != string(want) {
				t.Errorf("Key = %q, want %q", eab.Key, want)
			}
		})
	}

	if eab, err := externalAccountBinding(nil); err != nil || eab != nil {
		t.Errorf("nil spec: got %v, %v; want nil, nil", eab, err)
	}
	if _, err := externalAccountBinding(&model.EABSpec{KID: " ", HMACKey: "aGk"}); err == nil {
		t.Error("expected an error for an empty kid")
	}
	if _, err := externalAccountBinding(&model.EABSpec{KID: "k", HMACKey: ""}); err == nil {
		t.Error("expected an error for an empty hmacKey")
	}
	if _, err := externalAccountBinding(&model.EABSpec{KID: "k", HMACKey: "not valid base64!!"}); err == nil {
		t.Error("expected an error for a non-base64 hmacKey")
	}
}

func TestAccountKeyPathPerEABAccount(t *testing.T) {
	dir := "/certs"
	url := "https://acme.example/directory"

	// No EAB: unchanged from the pre-EAB layout (one key per directory URL).
	base := accountKeyPath(dir, url, "")
	if base != accountKeyPath(dir, url, "") {
		t.Fatal("path is not stable")
	}
	// Two external accounts on the same CA must not share an account key.
	a := accountKeyPath(dir, url, "kid-a")
	b := accountKeyPath(dir, url, "kid-b")
	if a == b || a == base || b == base {
		t.Errorf("expected distinct account key paths, got base=%s a=%s b=%s", base, a, b)
	}
	if !strings.HasPrefix(a, dir) {
		t.Errorf("account key path escaped the cert dir: %s", a)
	}
}
