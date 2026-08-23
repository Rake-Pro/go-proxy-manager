package dataplane

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

// buildClientHello assembles a minimal but well-formed ClientHello handshake
// message (no record framing), carrying serverName as SNI when non-empty.
func buildClientHello(serverName string) []byte {
	var body []byte
	body = append(body, 0x03, 0x03)             // legacy_version TLS 1.2
	body = append(body, make([]byte, 32)...)    // random
	body = append(body, 0x00)                   // legacy_session_id (empty)
	body = append(body, 0x00, 0x02, 0x13, 0x01) // cipher_suites: one suite
	body = append(body, 0x01, 0x00)             // compression: one method (null)

	var exts []byte
	if serverName != "" {
		host := []byte(serverName)
		var sni []byte
		sni = append(sni, 0x00, 0x00) // server_name_list length (patched below)
		sni = append(sni, 0x00)       // name_type: host_name
		sni = append(sni, 0x00, 0x00) // host_name length (patched below)
		sni = append(sni, host...)
		binary.BigEndian.PutUint16(sni[0:2], uint16(len(sni)-2))
		binary.BigEndian.PutUint16(sni[3:5], uint16(len(host)))

		exts = append(exts, 0x00, 0x00) // extension_type: server_name
		exts = append(exts, 0x00, 0x00) // extension_data length (patched below)
		binary.BigEndian.PutUint16(exts[2:4], uint16(len(sni)))
		exts = append(exts, sni...)
	}
	// A supported_versions extension after SNI, so the parser has to walk past
	// an extension it does not care about rather than assuming SNI is last.
	exts = append(exts, 0x00, 0x2b, 0x00, 0x03, 0x02, 0x03, 0x04)

	extLen := make([]byte, 2)
	binary.BigEndian.PutUint16(extLen, uint16(len(exts)))
	body = append(body, extLen...)
	body = append(body, exts...)

	hs := []byte{handshakeClientHello, byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}
	return append(hs, body...)
}

// tlsRecords frames hs into handshake records of at most size bytes each, so a
// ClientHello split across records can be exercised.
func tlsRecords(hs []byte, size int) []byte {
	var out []byte
	for len(hs) > 0 {
		n := size
		if n > len(hs) {
			n = len(hs)
		}
		out = append(out, recordTypeHandshake, 0x03, 0x01, byte(n>>8), byte(n))
		out = append(out, hs[:n]...)
		hs = hs[n:]
	}
	return out
}

// corruptExtLen overstates the extensions vector length of a ClientHello, the
// classic truncation an attacker uses to walk the parser off the end.
func corruptExtLen(hs []byte) []byte {
	out := append([]byte{}, hs...)
	// 4 handshake header + 2 version + 32 random + 1 session id + 4 ciphers + 2 compression
	off := 4 + 34 + 1 + 4 + 2
	binary.BigEndian.PutUint16(out[off:off+2], 0xffff)
	return out
}

// feedConn serves wire over a real socket so peekClientHello is exercised
// against an actual net.Conn (deadlines included).
func feedConn(t *testing.T, wire []byte) net.Conn {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	ch := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		ch <- c
	}()
	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	go func() {
		_, _ = client.Write(wire)
	}()
	server := <-ch
	t.Cleanup(func() { server.Close() })
	return server
}

// TestPeekClientHelloVectors covers what a stream listener has to survive on a
// port anyone can connect to: well-formed hellos of both shapes, a hello split
// across records, and the malformed / non-TLS inputs that must be rejected
// rather than routed somewhere arbitrary.
func TestPeekClientHelloVectors(t *testing.T) {
	cases := []struct {
		name    string
		wire    []byte
		want    string
		wantErr bool
	}{
		{
			name: "sni present",
			wire: tlsRecords(buildClientHello("db.example.com"), 4096),
			want: "db.example.com",
		},
		{
			name: "sni is lower-cased and the trailing dot dropped",
			wire: tlsRecords(buildClientHello("DB.Example.COM."), 4096),
			want: "db.example.com",
		},
		{
			name: "no sni extension",
			wire: tlsRecords(buildClientHello(""), 4096),
			want: "",
		},
		{
			name: "hello split across records",
			wire: tlsRecords(buildClientHello("split.example.com"), 24),
			want: "split.example.com",
		},
		{
			name:    "not a TLS handshake at all",
			wire:    []byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n"),
			wantErr: true,
		},
		{
			name:    "handshake record that is not a ClientHello",
			wire:    tlsRecords([]byte{0x02, 0x00, 0x00, 0x02, 0x03, 0x03}, 4096),
			wantErr: true,
		},
		{
			name: "handshake body too short to be a ClientHello",
			// A well-framed handshake message whose declared length holds far
			// less than the fixed version+random prefix.
			wire:    tlsRecords(append([]byte{handshakeClientHello, 0x00, 0x00, 0x0a}, make([]byte, 10)...), 4096),
			wantErr: true,
		},
		{
			name:    "extension block longer than the hello body",
			wire:    tlsRecords(corruptExtLen(buildClientHello("trunc.example.com")), 4096),
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := feedConn(t, tc.wire)
			name, raw, err := peekClientHello(c, 2*time.Second)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got sni %q", name)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if name != tc.want {
				t.Fatalf("sni = %q, want %q", name, tc.want)
			}
			// Everything consumed must be handed back so passthrough can replay
			// the handshake byte-for-byte.
			if !bytes.Equal(raw, tc.wire) {
				t.Fatalf("peeked %d bytes, want the whole %d-byte hello returned verbatim", len(raw), len(tc.wire))
			}
		})
	}
}

// TestPeekClientHelloAgainstCryptoTLS proves the hand-written parser agrees with
// a hello produced by the standard library, extensions and all.
func TestPeekClientHelloAgainstCryptoTLS(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	go func() {
		defer client.Close()
		//nolint:errcheck // the handshake never completes; only the hello matters
		_ = tls.Client(client, &tls.Config{ServerName: "real.example.com", InsecureSkipVerify: true}).Handshake()
	}()
	name, raw, err := peekClientHello(server, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if name != "real.example.com" {
		t.Fatalf("sni = %q, want real.example.com", name)
	}
	if len(raw) == 0 {
		t.Fatal("the consumed bytes must be returned for replay")
	}
}

// TestPeekClientHelloTimesOut proves a client that connects and says nothing is
// cut loose rather than holding the goroutine forever.
func TestPeekClientHelloTimesOut(t *testing.T) {
	c := feedConn(t, nil)
	start := time.Now()
	if _, _, err := peekClientHello(c, 150*time.Millisecond); err == nil {
		t.Fatal("a silent client must time out")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("returned after %v, want the ~150ms deadline", elapsed)
	}
}

// TestStreamRoutesMatch covers SNI selection: exact beats wildcard, a wildcard
// covers exactly one label, and an unknown or absent name has no route (the
// connection is dropped rather than sent to an arbitrary backend).
func TestStreamRoutesMatch(t *testing.T) {
	exact := &streamTarget{name: "exact"}
	wild := &streamTarget{name: "wild"}
	rs := &streamRoutes{
		sni:   true,
		exact: map[string]*streamTarget{"db.example.com": exact},
		wild:  []wildcardRoute{{suffix: ".example.com", target: wild}},
	}
	cases := []struct {
		name string
		want *streamTarget
	}{
		{"db.example.com", exact},
		{"other.example.com", wild},
		{"deep.other.example.com", nil}, // a wildcard covers one label only
		{"example.com", nil},
		{"db.example.org", nil},
		{"", nil}, // no SNI at all
	}
	for _, tc := range cases {
		if got := rs.match(tc.name); got != tc.want {
			t.Errorf("match(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}

	// Without SNI routing the single blind route always wins.
	blind := &streamRoutes{def: exact}
	if got := blind.match(""); got != exact {
		t.Errorf("a non-SNI port must always use its single route, got %v", got)
	}
}
