package dataplane

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

const testWSAccept = "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="

// upgradeRulesUpstream answers an upgrade with a raw 101 carrying the
// handshake headers plus two headers the rules act on, then echoes lines; any
// other request gets the same two headers on an ordinary 200.
func upgradeRulesUpstream(t *testing.T) model.Upstream {
	t.Helper()
	return upstreamFor(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isUpgradeRequest(r) {
			w.Header().Set("X-Leak", "1")
			w.Header().Set("X-Order", "upstream")
			_, _ = io.WriteString(w, "plain")
			return
		}
		conn, brw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		_, _ = brw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n" +
			"Sec-WebSocket-Accept: " + testWSAccept + "\r\nSec-WebSocket-Protocol: chat\r\nX-Leak: 1\r\nX-Order: upstream\r\n\r\n")
		_ = brw.Flush()
		line, err := brw.ReadString('\n')
		if err != nil {
			return
		}
		_, _ = brw.WriteString("echo:" + line)
		_ = brw.Flush()
	}))
}

// Two stacked headers middlewares must act on a 101 exactly as on an ordinary
// response (same rules, same order: the later-listed middleware wins), while
// the handshake headers survive rules that name them.
func TestHeadersMiddlewareAppliesToSwitchingProtocols(t *testing.T) {
	cfg := model.Config{
		Middlewares: []model.Middleware{
			{ObjectMeta: model.ObjectMeta{Name: "first"}, Type: model.MWTypeHeaders, Headers: &model.HeadersMiddleware{
				SetResponse: map[string]string{"X-Order": "first", "X-First-Only": "1", "Upgrade": "h2c"},
			}},
			{ObjectMeta: model.ObjectMeta{Name: "second"}, Type: model.MWTypeHeaders, Headers: &model.HeadersMiddleware{
				SetResponse: map[string]string{
					"X-Order": "second", "X-Set": "gpm",
					"Connection": "close", "sec-websocket-accept": "bogus",
				},
				RemoveResponse: []string{"X-Leak", "X-First-Only", "Connection", "Upgrade", "Sec-WebSocket-Accept", "Sec-WebSocket-Protocol", "Sec-WebSocket-Extensions"},
			}},
		},
		ProxyHosts: []model.ProxyHost{{
			ObjectMeta:  model.ObjectMeta{Name: "front"},
			Domains:     []string{"front.example"},
			Upstream:    upgradeRulesUpstream(t),
			Middlewares: []string{"first", "second"},
		}},
	}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}
	srv := frontServer(t, rt)

	plain, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	_ = plain.Body.Close()

	c, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	_, _ = io.WriteString(c, "GET /ws HTTP/1.1\r\nHost: front.example\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n"+
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n")
	br := bufio.NewReader(c)
	up, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("reading the upgrade response: %v", err)
	}
	if up.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade answered %d, want 101", up.StatusCode)
	}

	for _, tc := range []struct {
		name     string
		want     string
		ordinary bool // the ordinary response must carry the same value
	}{
		{name: "X-Set", want: "gpm", ordinary: true},
		{name: "X-Order", want: "second", ordinary: true},
		{name: "X-Leak", want: "", ordinary: true},
		{name: "X-First-Only", want: "", ordinary: true},
		{name: "Connection", want: "Upgrade"},
		{name: "Upgrade", want: "websocket"},
		{name: "Sec-WebSocket-Accept", want: testWSAccept},
		{name: "Sec-WebSocket-Protocol", want: "chat"},
	} {
		if got := up.Header.Get(tc.name); got != tc.want {
			t.Errorf("101 %s = %q, want %q", tc.name, got, tc.want)
		}
		if vs := up.Header.Values(tc.name); len(vs) > 1 {
			t.Errorf("101 %s carries %d values %q, want one", tc.name, len(vs), vs)
		}
		if tc.ordinary {
			if got := plain.Header.Get(tc.name); got != tc.want {
				t.Errorf("200 %s = %q, want %q (the 101 must match the ordinary response)", tc.name, got, tc.want)
			}
		}
	}

	_, _ = io.WriteString(c, "ping\n")
	if line, err := br.ReadString('\n'); err != nil || line != "echo:ping\n" {
		t.Fatalf("tunnel read = %q, %v; want the echo", line, err)
	}
}
