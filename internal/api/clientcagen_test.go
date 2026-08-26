package api_test

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/api"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/store"
	pkcs12 "software.sslmate.com/src/go-pkcs12"
)

// genHandler builds an API handler over a fresh store plus a cert store, and
// returns both directories so a test can inspect what generation put on disk.
func genHandler(t *testing.T) (http.Handler, string, string) {
	t.Helper()
	dir := t.TempDir()
	certDir := t.TempDir()
	st := store.New(dir, store.NewExecGit(dir))
	if err := st.Init(context.Background()); err != nil {
		t.Fatalf("store init: %v", err)
	}
	return api.New(api.Deps{Store: st, CertDir: certDir}), dir, certDir
}

// TestGenerateClientCAEndToEnd is the whole point of the feature: from an empty
// config, one request produces a committed ClientCA object whose key is on disk
// and which can immediately issue a client certificate that verifies against it -
// with no openssl and no hand-placed file anywhere.
func TestGenerateClientCAEndToEnd(t *testing.T) {
	h, _, certDir := genHandler(t)

	w := do(t, h, "POST", "/client-cas/corp/generate",
		`{"commonName":"Corp Device CA","validityDays":365,"organization":"Corp Ltd"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("generate want 200 got %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("X-Config-Commit") == "" {
		t.Fatal("generate is a config mutation and must report a commit")
	}

	// The response is the created object, like a PUT - and carries no key.
	var created model.ClientCA
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode generate response: %v", err)
	}
	if created.Name != "corp" {
		t.Fatalf("created name = %q", created.Name)
	}
	if created.CAKeyFile != "client-cas/corp.key" {
		t.Fatalf("caKeyFile = %q", created.CAKeyFile)
	}
	assertNoKeyMaterial(t, w.Body.String(), "generate response")

	// The certificate is a usable, correctly shaped CA.
	block, _ := pem.Decode([]byte(created.CAPEM))
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatalf("caPEM is not a PEM certificate: %q", created.CAPEM)
	}
	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse generated CA: %v", err)
	}
	if !caCert.IsCA || !caCert.BasicConstraintsValid {
		t.Fatal("generated certificate is not a CA")
	}
	if caCert.MaxPathLen != 0 || !caCert.MaxPathLenZero {
		t.Fatalf("generated CA must be pathlen:0, got MaxPathLen=%d zero=%v", caCert.MaxPathLen, caCert.MaxPathLenZero)
	}
	if caCert.KeyUsage != x509.KeyUsageCertSign|x509.KeyUsageCRLSign {
		t.Fatalf("KeyUsage = %v", caCert.KeyUsage)
	}
	if caCert.Subject.CommonName != "Corp Device CA" {
		t.Fatalf("CN = %q", caCert.Subject.CommonName)
	}
	if len(caCert.Subject.Organization) != 1 || caCert.Subject.Organization[0] != "Corp Ltd" {
		t.Fatalf("Organization = %v", caCert.Subject.Organization)
	}
	if caCert.SerialNumber.Sign() <= 0 {
		t.Fatal("serial must be positive")
	}
	if !caCert.NotBefore.Before(time.Now()) {
		t.Fatal("NotBefore must be backdated for clock skew")
	}
	if got := caCert.NotAfter.Sub(caCert.NotBefore); got < 364*24*time.Hour {
		t.Fatalf("validity %v did not honour validityDays", got)
	}
	if bits := caCert.PublicKey.(interface{ Size() int }).Size() * 8; bits != 4096 {
		t.Fatalf("generated CA key is %d bits, want 4096", bits)
	}

	// The private key landed in the cert store, private.
	keyPath := filepath.Join(certDir, "client-cas", "corp.key")
	fi, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat generated key: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("generated key mode %v, want 0600", fi.Mode().Perm())
	}

	// It is in the config, with a history entry, like any other saved object.
	w = do(t, h, "GET", "/client-cas/corp", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET the generated CA: %d %s", w.Code, w.Body.String())
	}
	assertNoKeyMaterial(t, w.Body.String(), "GET /client-cas/corp")
	w = do(t, h, "GET", "/config", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /config: %d", w.Code)
	}
	assertNoKeyMaterial(t, w.Body.String(), "GET /config")
	w = do(t, h, "GET", "/client-cas/corp/history", "")
	if w.Code != http.StatusOK {
		t.Fatalf("history: %d %s", w.Code, w.Body.String())
	}
	var commits []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &commits); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(commits) == 0 {
		t.Fatal("a generated CA must appear in object history like any other save")
	}

	// And it can immediately issue - the point of generating rather than pasting.
	w = do(t, h, "POST", "/client-cas/corp/issue",
		`{"commonName":"phone-01","password":"a-long-enough-pw"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("issue against the generated CA want 200 got %d: %s", w.Code, w.Body.String())
	}
	_, leaf, chain, err := pkcs12.DecodeChain(w.Body.Bytes(), "a-long-enough-pw")
	if err != nil {
		t.Fatalf("decode issued p12: %v", err)
	}
	if len(chain) != 1 || !chain[0].Equal(caCert) {
		t.Fatal("the issued bundle must carry the generated CA")
	}
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}); err != nil {
		t.Fatalf("certificate issued by the generated CA does not verify against it: %v", err)
	}
}

// TestGenerateClientCADefaults proves an empty body is valid: the object name
// becomes the common name and the validity is the ten-year default.
func TestGenerateClientCADefaults(t *testing.T) {
	h, _, _ := genHandler(t)
	w := do(t, h, "POST", "/client-cas/lab/generate", `{}`)
	if w.Code != http.StatusOK {
		t.Fatalf("generate want 200 got %d: %s", w.Code, w.Body.String())
	}
	var created model.ClientCA
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode([]byte(created.CAPEM))
	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if caCert.Subject.CommonName != "lab" {
		t.Fatalf("CN defaults to the object name, got %q", caCert.Subject.CommonName)
	}
	if len(caCert.Subject.Organization) != 0 {
		t.Fatalf("no organization was asked for, got %v", caCert.Subject.Organization)
	}
	if got := caCert.NotAfter.Sub(caCert.NotBefore); got < 3649*24*time.Hour {
		t.Fatalf("default validity %v, want about ten years", got)
	}
}

// TestGenerateClientCAConflicts covers the two things generation must never do:
// replace a configured CA, or overwrite a key file that may still be signing for
// certificates already on devices.
func TestGenerateClientCAConflicts(t *testing.T) {
	t.Run("existing CA name", func(t *testing.T) {
		h, _, certDir := genHandler(t)
		caPEM, _, _ := throwawayCA(t, "byo")
		body, err := json.Marshal(map[string]string{"caPEM": caPEM})
		if err != nil {
			t.Fatal(err)
		}
		if w := do(t, h, "PUT", "/client-cas/byo", string(body)); w.Code != http.StatusOK {
			t.Fatalf("seed: %d %s", w.Code, w.Body.String())
		}
		w := do(t, h, "POST", "/client-cas/byo/generate", `{}`)
		if w.Code != http.StatusConflict {
			t.Fatalf("want 409 got %d: %s", w.Code, w.Body.String())
		}
		// Refused before generating: nothing was written to the cert store.
		if _, err := os.Stat(filepath.Join(certDir, "client-cas", "byo.key")); !os.IsNotExist(err) {
			t.Fatal("a refused generate must not write a key file")
		}
		// The existing object is untouched.
		w = do(t, h, "GET", "/client-cas/byo", "")
		var got model.ClientCA
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.CAPEM != caPEM || got.CAKeyFile != "" {
			t.Fatalf("the existing CA was modified: %+v", got)
		}
	})

	t.Run("key file another CA is still using", func(t *testing.T) {
		h, storeDir, certDir := genHandler(t)
		plantKey(t, certDir, "corp.key", "IN-USE KEY")
		// A differently named ClientCA points at the very path generation would
		// write. That key is live - it is what that CA issues and signs CRLs with.
		caPEM, _, _ := throwawayCA(t, "other")
		body, err := json.Marshal(map[string]string{"caPEM": caPEM, "caKeyFile": "./client-cas/corp.key"})
		if err != nil {
			t.Fatal(err)
		}
		if w := do(t, h, "PUT", "/client-cas/other", string(body)); w.Code != http.StatusOK {
			t.Fatalf("seed referrer: %d %s", w.Code, w.Body.String())
		}

		w := do(t, h, "POST", "/client-cas/corp/generate", `{}`)
		if w.Code != http.StatusConflict {
			t.Fatalf("want 409 got %d: %s", w.Code, w.Body.String())
		}
		// The refusal names the CA still using it, so the operator knows why.
		if msg := errMessage(t, w); !strings.Contains(msg, `"other"`) {
			t.Fatalf("the conflict must name the referring CA, got %s", msg)
		}
		// The key file is byte-for-byte untouched...
		if got := readKey(t, certDir, "corp.key"); got != "IN-USE KEY" {
			t.Fatalf("a key still in use must not be touched, got %q", got)
		}
		// ...and no config object was created.
		if w := do(t, h, "GET", "/client-cas/corp", ""); w.Code != http.StatusNotFound {
			t.Fatalf("a refused generate must create no object, got %d", w.Code)
		}
		if entries, err := os.ReadDir(filepath.Join(storeDir, "client-cas")); err == nil && len(entries) != 1 {
			t.Fatalf("only the seeded referrer should exist, found %d objects", len(entries))
		}
	})

	t.Run("orphaned key file is reclaimed", func(t *testing.T) {
		h, _, certDir := genHandler(t)
		// The residue of a crash between the key write and the config save, or of
		// a ClientCA someone deleted (a delete keeps the key on purpose). Nothing
		// references it, so refusing forever would make the name permanently
		// unusable from the UI with no way to fix it.
		plantKey(t, certDir, "corp.key", "ORPHANED KEY")

		w := do(t, h, "POST", "/client-cas/corp/generate", `{}`)
		if w.Code != http.StatusOK {
			t.Fatalf("an unreferenced orphan must be reclaimed, got %d: %s", w.Code, w.Body.String())
		}
		if got := readKey(t, certDir, "corp.key"); got == "ORPHANED KEY" {
			t.Fatal("the orphan was not replaced by the freshly generated key")
		}
		// The generated pair is real and the object is saved and usable.
		var created model.ClientCA
		if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
			t.Fatal(err)
		}
		if created.CAKeyFile != "client-cas/corp.key" {
			t.Fatalf("caKeyFile = %q", created.CAKeyFile)
		}
		if w := do(t, h, "POST", "/client-cas/corp/issue",
			`{"commonName":"phone-01","password":"a-long-enough-pw"}`); w.Code != http.StatusOK {
			t.Fatalf("the reclaimed CA must be able to issue, got %d: %s", w.Code, w.Body.String())
		}
	})
}

// plantKey writes a fake pre-existing CA key into the certificate store.
func plantKey(t *testing.T, certDir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(certDir, "client-cas"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(certDir, "client-cas", name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readKey(t *testing.T, certDir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(certDir, "client-cas", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestGenerateClientCACleansUpAfterFailedSave proves the rollback: when the
// config write fails after the key landed, the key file is removed, so the
// no-overwrite rule does not permanently block retrying the same name.
func TestGenerateClientCACleansUpAfterFailedSave(t *testing.T) {
	h, storeDir, certDir := genHandler(t)

	// Make the object's config directory unwritable so Store.Save fails after
	// GenerateCA has already written the key. Restored so t.TempDir can clean up.
	objDir := filepath.Join(storeDir, "client-cas")
	if err := os.MkdirAll(objDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(objDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(objDir, 0o700) })

	w := do(t, h, "POST", "/client-cas/corp/generate", `{}`)
	if w.Code == http.StatusOK {
		t.Fatal("expected the config save to fail with the object dir read-only")
	}
	keyPath := filepath.Join(certDir, "client-cas", "corp.key")
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Fatalf("the key file must be cleaned up when the config save fails, stat err = %v", err)
	}

	// Retrying the same name once the store is writable again works, which is
	// exactly what the cleanup exists to preserve.
	if err := os.Chmod(objDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if w := do(t, h, "POST", "/client-cas/corp/generate", `{}`); w.Code != http.StatusOK {
		t.Fatalf("retry after cleanup want 200 got %d: %s", w.Code, w.Body.String())
	}
}

// TestGenerateClientCAValidation is the request table. Every case here is
// refused BEFORE any key is generated, which is why it is cheap to run.
func TestGenerateClientCAValidation(t *testing.T) {
	h, _, certDir := genHandler(t)
	cases := []struct {
		name string
		path string
		body string
		want int
	}{
		{"invalid object name", "/client-cas/Corp!/generate", `{}`, http.StatusBadRequest},
		{"over-long common name", "/client-cas/corp/generate",
			`{"commonName":"` + longName(65) + `"}`, http.StatusBadRequest},
		{"control character in common name", "/client-cas/corp/generate",
			`{"commonName":"a\nb"}`, http.StatusBadRequest},
		{"over-long organization", "/client-cas/corp/generate",
			`{"organization":"` + longName(65) + `"}`, http.StatusBadRequest},
		{"negative validity", "/client-cas/corp/generate", `{"validityDays":-1}`, http.StatusBadRequest},
		{"validity over the cap", "/client-cas/corp/generate", `{"validityDays":7301}`, http.StatusBadRequest},
		{"malformed body", "/client-cas/corp/generate", `{`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := do(t, h, "POST", tc.path, tc.body)
			if w.Code != tc.want {
				t.Fatalf("status %d, want %d: %s", w.Code, tc.want, w.Body.String())
			}
		})
	}
	// Not one of those refusals reached the generator.
	if entries, err := os.ReadDir(filepath.Join(certDir, "client-cas")); err == nil && len(entries) != 0 {
		t.Fatalf("a refused generate must write nothing, found %d files", len(entries))
	}
}

// assertNoKeyMaterial fails if a payload carries anything that looks like the
// generated private key. The CA certificate is public and expected; the key is
// not, in any response, ever.
func assertNoKeyMaterial(t *testing.T, payload, where string) {
	t.Helper()
	for _, forbidden := range []string{"PRIVATE KEY", "caKeyPEM"} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("%s leaks %q", where, forbidden)
		}
	}
}

// TestGenerateClientCAUnconfinedName proves a name whose derived key path would
// escape the certificate store is refused with a message about the NAME. An error
// naming caKeyFile would send an operator hunting for a field they never filled in.
func TestGenerateClientCAUnconfinedName(t *testing.T) {
	h, _, certDir := genHandler(t)
	// An unreferenced key file is normally reclaimed. A request that is going to
	// be refused must not reach that reclaim, so plant one and prove it survives.
	plantKey(t, certDir, "a..b.key", "PLANTED KEY")

	w := do(t, h, "POST", "/client-cas/a..b/generate", `{}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d: %s", w.Code, w.Body.String())
	}
	msg := errMessage(t, w)
	if !strings.Contains(msg, `"a..b"`) {
		t.Fatalf("the refusal must name the offending CA name, got %s", msg)
	}
	if !strings.Contains(msg, "choose a name without") {
		t.Fatalf("the refusal must say what to do about it, got %s", msg)
	}
	if strings.Contains(msg, "caKeyFile") {
		t.Fatalf("the refusal must not blame caKeyFile, which the caller never sent: %s", msg)
	}
	if got := readKey(t, certDir, "a..b.key"); got != "PLANTED KEY" {
		t.Fatalf("a refused generate must not touch the store, key is now %q", got)
	}
}

// TestGenerateClientCARejectedRequestLeavesStoreUntouched is the ordering
// invariant the reclaim could silently break: an unreferenced key file is
// reclaimed (deleted) on a good request, so a request that is going to be REFUSED
// must never reach that code. Every refusal below happens on request validation,
// with an orphan sitting in the store the whole time.
func TestGenerateClientCARejectedRequestLeavesStoreUntouched(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"validity over the cap", `{"validityDays":99999}`},
		{"negative validity", `{"validityDays":-1}`},
		{"over-long common name", `{"commonName":"` + longName(65) + `"}`},
		{"control characters in common name", `{"commonName":"a
b"}`},
		{"over-long organization", `{"organization":"` + longName(65) + `"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, storeDir, certDir := genHandler(t)
			plantKey(t, certDir, "corp.key", "ORPHANED KEY")

			w := do(t, h, "POST", "/client-cas/corp/generate", tc.body)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("want 400 got %d: %s", w.Code, w.Body.String())
			}
			if got := readKey(t, certDir, "corp.key"); got != "ORPHANED KEY" {
				t.Fatalf("a rejected request reclaimed the orphan anyway, key is now %q", got)
			}
			if entries, err := os.ReadDir(filepath.Join(storeDir, "client-cas")); err == nil && len(entries) != 0 {
				t.Fatalf("a rejected request must create no object, found %d", len(entries))
			}
		})
	}

	// The reclaim still runs for a request that IS good, so the fix did not just
	// disable it.
	t.Run("valid request still reclaims", func(t *testing.T) {
		h, _, certDir := genHandler(t)
		plantKey(t, certDir, "corp.key", "ORPHANED KEY")
		if w := do(t, h, "POST", "/client-cas/corp/generate", `{"validityDays":1}`); w.Code != http.StatusOK {
			t.Fatalf("want 200 got %d: %s", w.Code, w.Body.String())
		}
		if got := readKey(t, certDir, "corp.key"); got == "ORPHANED KEY" {
			t.Fatal("a valid request must still reclaim the orphan")
		}
	})
}

// errMessage decodes the uniform {"error": "..."} body so an assertion can read
// the message as written, not as JSON-escaped.
func errMessage(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body %q: %v", w.Body.String(), err)
	}
	return body.Error
}
