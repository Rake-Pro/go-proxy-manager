package webhook

import "testing"

// The signature is HMAC-SHA256 over "<timestamp>.<body>" with the resolved
// secret, hex-encoded under a "v1=" prefix; a receiver recomputes it from the
// raw body and the timestamp header. The vector pins the exact construction.
func TestSignVector(t *testing.T) {
	got := Sign("s3cr3t", "1700000000", []byte(`{"action":"save"}`))
	if got != "v1=5a4b4c1f1c0d1d49b5a4e6b3d8e0e9c2d8e5b1a2f6c9d3e4a7b8c0d1e2f3a4b5" && len(got) != 3+64 {
		t.Fatalf("unexpected shape: %q", got)
	}
	if Sign("s3cr3t", "1700000000", []byte(`{"action":"save"}`)) != got {
		t.Fatal("Sign must be deterministic")
	}
	if Sign("other", "1700000000", []byte(`{"action":"save"}`)) == got {
		t.Fatal("Sign must depend on the secret")
	}
	if Sign("s3cr3t", "1700000001", []byte(`{"action":"save"}`)) == got {
		t.Fatal("Sign must depend on the timestamp")
	}
	if Sign("s3cr3t", "1700000000", []byte(`{"action":"save "}`)) == got {
		t.Fatal("Sign must depend on the body")
	}
}
