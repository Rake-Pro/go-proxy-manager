package dataplane

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

func gzipReq(method string) *http.Request {
	r := httptest.NewRequest(method, "/", nil)
	r.Header.Set("Accept-Encoding", "gzip, deflate, br")
	return r
}

func largeHTML() string {
	return "<html><body>" + strings.Repeat("hello world ", 200) + "</body></html>" // well over 1024 bytes
}

func TestCompressionCompressesEligible(t *testing.T) {
	body := largeHTML()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})
	h := compressionHandler(model.Compression{Enabled: true}, next)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, gzipReq(http.MethodGet))

	resp := rec.Result()
	if got := resp.Header.Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if got := resp.Header.Get("Vary"); got != "Accept-Encoding" {
		t.Fatalf("Vary = %q, want Accept-Encoding", got)
	}
	if resp.Header.Get("Content-Length") != "" {
		t.Fatal("Content-Length must be stripped when compressing")
	}
	gr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	out, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("reading gzip body: %v", err)
	}
	if string(out) != body {
		t.Fatalf("decompressed body mismatch: got %d bytes, want %d", len(out), len(body))
	}
}

func TestCompressionHEADUntouched(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if _, ok := w.(*gzipResponseWriter); ok {
			t.Fatal("HEAD request must bypass the compression wrapper entirely")
		}
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
	})
	h := compressionHandler(model.Compression{Enabled: true}, next)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, gzipReq(http.MethodHead))
	if !called {
		t.Fatal("handler was not called")
	}
	if got := rec.Result().Header.Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty for HEAD", got)
	}
}

func TestCompressionSkipsExclusions(t *testing.T) {
	body := largeHTML()
	cases := []struct {
		name string
		next http.HandlerFunc
		// wantEncoding is the Content-Encoding the response must end up with:
		// empty for a normal skip, or the upstream's own value when compression
		// must leave an already-encoded response untouched rather than strip it.
		wantEncoding string
	}{
		{
			name:         "content-encoding already set",
			wantEncoding: "br",
			next: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				w.Header().Set("Content-Encoding", "br")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(body))
			},
		},
		{
			name: "non-matching content type",
			next: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "image/png")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(body))
			},
		},
		{
			name: "below minBytes",
			next: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("tiny"))
			},
		},
		{
			name: "204 no content",
			next: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusNoContent)
			},
		},
		{
			name: "304 not modified",
			next: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusNotModified)
			},
		},
		{
			name: "text/event-stream",
			next: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(body))
			},
		},
		{
			name: "streaming (early flush)",
			next: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("data: chunk\n\n"))
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
				_, _ = w.Write([]byte(body)) // even though this pushes past minBytes
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := compressionHandler(model.Compression{Enabled: true}, tc.next)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, gzipReq(http.MethodGet))
			if got := rec.Result().Header.Get("Content-Encoding"); got != tc.wantEncoding {
				t.Fatalf("Content-Encoding = %q, want %q (not gzip-compressed)", got, tc.wantEncoding)
			}
		})
	}
}

func TestCompressionSkipsWithoutAcceptEncoding(t *testing.T) {
	body := largeHTML()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})
	h := compressionHandler(model.Compression{Enabled: true}, next)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil)) // no Accept-Encoding
	if got := rec.Result().Header.Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty when client sends no Accept-Encoding", got)
	}
	if rec.Body.String() != body {
		t.Fatal("body must pass through unchanged")
	}
}

func TestCompressionCustomMinBytesAndTypes(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-custom")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(bytes.Repeat([]byte("x"), 50))
	})
	h := compressionHandler(model.Compression{Enabled: true, MinBytes: 10, Types: []string{"application/x-custom"}}, next)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, gzipReq(http.MethodGet))
	if got := rec.Result().Header.Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip with custom types/minBytes", got)
	}
}
