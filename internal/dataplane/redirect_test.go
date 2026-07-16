package dataplane

import (
	"net/http"
	"net/url"
	"testing"
)

func TestRewriteUpstreamRedirect(t *testing.T) {
	target := &url.URL{Scheme: "http", Host: "192.0.2.20:8080"}

	newResp := func(location, fwdProto, reqHost string) *http.Response {
		req, _ := http.NewRequest(http.MethodGet, "http://"+target.Host+"/admin", nil)
		req.Host = reqHost
		if fwdProto != "" {
			req.Header.Set("X-Forwarded-Proto", fwdProto)
		}
		h := http.Header{}
		if location != "" {
			h.Set("Location", location)
		}
		return &http.Response{Header: h, Request: req}
	}

	tests := []struct {
		name     string
		location string
		fwdProto string
		reqHost  string
		want     string // expected Location after rewrite
	}{
		{
			name:     "upstream absolute redirect rewritten to public scheme+host",
			location: "http://192.0.2.20:8080/admin/",
			fwdProto: "https",
			reqHost:  "dns.example.com",
			want:     "https://dns.example.com/admin/",
		},
		{
			name:     "cross-domain redirect (IdP) left untouched",
			location: "https://auth.example.com/application/o/authorize/?x=1",
			fwdProto: "https",
			reqHost:  "go.example.com",
			want:     "https://auth.example.com/application/o/authorize/?x=1",
		},
		{
			name:     "relative redirect left untouched",
			location: "/admin/login",
			fwdProto: "https",
			reqHost:  "dns.example.com",
			want:     "/admin/login",
		},
		{
			name:     "missing X-Forwarded-Proto defaults to https",
			location: "http://192.0.2.20:8080/admin/",
			fwdProto: "",
			reqHost:  "dns.example.com",
			want:     "https://dns.example.com/admin/",
		},
		{
			name:     "no Location header is a no-op",
			location: "",
			fwdProto: "https",
			reqHost:  "dns.example.com",
			want:     "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := newResp(tc.location, tc.fwdProto, tc.reqHost)
			rewriteUpstreamRedirect(resp)
			if got := resp.Header.Get("Location"); got != tc.want {
				t.Fatalf("Location = %q, want %q", got, tc.want)
			}
		})
	}
}
