package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The bundle is baked into the binary, so its content is its version: the
// handler pairs a strong content ETag with Cache-Control: no-cache
// ("revalidate", not "do not store"). Without it embed.FS's zero ModTime left
// http.FileServer with no validator at all and every page load re-downloaded
// ~600 KB unconditionally.
func TestStaticAssetsAreConditionallyCacheable(t *testing.T) {
	h, err := Handler()
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}

	for _, path := range []string{"/app.js", "/app.css", "/hints.json", "/theme-init.js"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s = %d, want 200", path, rec.Code)
			}
			etag := rec.Header().Get("ETag")
			if etag == "" {
				t.Fatalf("GET %s: no ETag", path)
			}
			if !strings.HasPrefix(etag, `"`) || strings.HasPrefix(etag, `W/`) {
				t.Errorf("GET %s: ETag %q is not a strong validator", path, etag)
			}
			if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
				t.Errorf("GET %s: Cache-Control = %q, want %q", path, got, "no-cache")
			}
			if rec.Body.Len() == 0 {
				t.Errorf("GET %s served an empty body", path)
			}

			// The conditional request the browser makes on the next load.
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("If-None-Match", etag)
			rec2 := httptest.NewRecorder()
			h.ServeHTTP(rec2, req)
			if rec2.Code != http.StatusNotModified {
				t.Errorf("conditional GET %s = %d, want 304", path, rec2.Code)
			}
			if rec2.Body.Len() != 0 {
				t.Errorf("304 for %s carried a %d-byte body", path, rec2.Body.Len())
			}
			if got := rec2.Header().Get("ETag"); got != etag {
				t.Errorf("304 for %s: ETag = %q, want %q", path, got, etag)
			}

			// A stale validator still gets the bytes.
			req3 := httptest.NewRequest(http.MethodGet, path, nil)
			req3.Header.Set("If-None-Match", `"0000000000000000"`)
			rec3 := httptest.NewRecorder()
			h.ServeHTTP(rec3, req3)
			if rec3.Code != http.StatusOK {
				t.Errorf("GET %s with a stale ETag = %d, want 200", path, rec3.Code)
			}
		})
	}
}

// Two assets must have DIFFERENT ETags, or the validator is not derived from the
// content and a 304 would serve the wrong file from cache.
func TestAssetETagsAreContentDerived(t *testing.T) {
	h, err := Handler()
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}
	get := func(p string) string {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		return rec.Header().Get("ETag")
	}
	if a, b := get("/app.js"), get("/app.css"); a == b {
		t.Errorf("app.js and app.css share the ETag %q", a)
	}
	if a, b := get("/hints.json"), get("/app.js"); a == b {
		t.Errorf("hints.json and app.js share the ETag %q", a)
	}
}

// The SPA shell is the one thing a stale cache would pin, so it stays no-store
// and carries no validator.
func TestIndexIsNeverCached(t *testing.T) {
	h, err := Handler()
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}
	for _, path := range []string{"/", "/index.html", "/some/deep/spa/link"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200", path, rec.Code)
		}
		if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
			t.Errorf("GET %s: Cache-Control = %q, want no-store", path, got)
		}
	}
}

// The vendored fonts are served like any other asset, with the right type.
func TestVendoredFontsAreServed(t *testing.T) {
	h, err := Handler()
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}
	for _, f := range []string{
		"/fonts/inter-latin.woff2",
		"/fonts/space-grotesk-latin.woff2",
		"/fonts/jetbrains-mono-latin.woff2",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, f, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", f, rec.Code)
			continue
		}
		if rec.Body.Len() < 1024 {
			t.Errorf("GET %s served %d bytes - that is not a font", f, rec.Body.Len())
		}
		if rec.Header().Get("ETag") == "" {
			t.Errorf("GET %s: no ETag", f)
		}
	}
}
