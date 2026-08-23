package dataplane

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// PROXY protocol (HAProxy) inbound support, hand-written against the spec
// (https://www.haproxy.org/download/2.8/doc/proxy-protocol.txt) with nothing
// but the standard library.
//
// The whole point of the header is to replace the connection peer - an L4 load
// balancer - with the address of the real client, so every IP-based control gpm
// has (access lists, geo rules, guards, rate limits, the basic-auth lockout,
// X-Forwarded-For, the access log, the OIDC gate) evaluates the client rather
// than the balancer. That is done by overriding RemoteAddr on the accepted
// net.Conn, which is the single value all of those controls ultimately derive
// from, so there is no second code path that can be forgotten.
//
// It is also an UNAUTHENTICATED claim: anyone who can open the port can assert
// any source address. So the header is parsed only when the real TCP peer is
// inside the configured trustedCIDRs. From any other peer the bytes are left
// alone and treated as ordinary payload, and the peer address stands.

// v2Signature is the fixed 12-byte preamble of a PROXY protocol v2 header.
var v2Signature = []byte{0x0D, 0x0A, 0x0D, 0x0A, 0x00, 0x0D, 0x0A, 0x51, 0x55, 0x49, 0x54, 0x0A}

// v1Prefix is the fixed preamble of a (text) PROXY protocol v1 header.
var v1Prefix = []byte("PROXY ")

const (
	// maxProxyProtoV1 is the v1 spec's hard maximum header length, CRLF included.
	maxProxyProtoV1 = 107
	// maxProxyProtoHeader bounds a v2 header: the 16-byte fixed part plus the
	// largest address block its uint16 length field can describe (TLVs included,
	// which we consume and ignore). It is the ceiling on how much a trusted peer
	// can make us buffer before the payload starts.
	maxProxyProtoHeader = 16 + 65535
	// maxWarnedPeers bounds the once-per-peer warn set. Reaching it forgets every
	// peer rather than growing without bound; the only consequence is that a
	// long-lived, very wide set of peers may log a given peer more than once.
	maxWarnedPeers = 4096
)

// proxyProtoConfig is the compiled form of model.ProxyProtocolSettings.
type proxyProtoConfig struct {
	trusted []*net.IPNet
	timeout time.Duration
}

// compileProxyProtocol turns settings into the live listener config, or nil when
// the feature is off (or enabled with no trusted peer, which model validation
// rejects at write time and which must never mean "trust everyone" here).
func compileProxyProtocol(s *model.ProxyProtocolSettings) *proxyProtoConfig {
	if s == nil || !s.Enabled {
		return nil
	}
	c := &proxyProtoConfig{timeout: s.HeaderTimeout()}
	for _, cidr := range s.TrustedCIDRs {
		if n := parseNet(cidr); n != nil {
			c.trusted = append(c.trusted, n)
		}
	}
	if len(c.trusted) == 0 {
		log.Error().Msg("proxy protocol: enabled with no usable trustedCIDRs; refusing to accept headers from any peer")
		return nil
	}
	return c
}

// proxyProtoListener wraps a listener so every accepted connection parses a
// PROXY header before its first byte of payload. The config is read through a
// func on every Accept, so a settings change takes effect on the next connection
// with no listener restart (and a nil result restores the untouched conn).
type proxyProtoListener struct {
	net.Listener
	cfg func() *proxyProtoConfig
}

// wrapProxyProtocol returns ln unchanged when cfg is nil, otherwise a listener
// that strips and applies inbound PROXY headers.
func wrapProxyProtocol(ln net.Listener, cfg func() *proxyProtoConfig) net.Listener {
	if cfg == nil {
		return ln
	}
	return &proxyProtoListener{Listener: ln, cfg: cfg}
}

func (l *proxyProtoListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	cfg := l.cfg()
	if cfg == nil {
		return c, nil // feature off right now: the raw conn, zero overhead
	}
	return &proxyProtoConn{Conn: c, cfg: cfg}, nil
}

// proxyProtoConn is an accepted connection whose PROXY header is parsed lazily,
// on the first Read or RemoteAddr - never in Accept, so one stalled sender
// cannot hold up the accept loop for every other client.
//
// buf holds bytes read from the socket that the header parser did not consume;
// Read drains it before touching the socket again, so the payload is delivered
// byte-for-byte whether or not a header was present.
type proxyProtoConn struct {
	net.Conn
	cfg *proxyProtoConfig

	once   sync.Once
	err    error
	remote net.Addr
	buf    []byte
}

// header parses the PROXY header exactly once. Its error (a malformed header, a
// stalled sender) is sticky: the connection is closed and every subsequent
// operation fails, so no half-parsed header can be mistaken for payload.
func (c *proxyProtoConn) header() error {
	c.once.Do(func() {
		c.remote = c.Conn.RemoteAddr()
		c.err = c.readHeader()
		if c.err != nil {
			_ = c.Conn.Close()
		}
	})
	return c.err
}

func (c *proxyProtoConn) Read(p []byte) (int, error) {
	if err := c.header(); err != nil {
		return 0, err
	}
	if len(c.buf) > 0 {
		n := copy(p, c.buf)
		c.buf = c.buf[n:]
		return n, nil
	}
	return c.Conn.Read(p)
}

// RemoteAddr is the real client address once a header from a trusted peer has
// been parsed, and the connection peer otherwise. net/http reads it before the
// first request line, which is what makes the header take effect everywhere.
func (c *proxyProtoConn) RemoteAddr() net.Addr {
	if err := c.header(); err != nil {
		return c.Conn.RemoteAddr()
	}
	return c.remote
}

func (c *proxyProtoConn) readHeader() error {
	peer := addrIP(c.Conn.RemoteAddr())
	if peer == nil || !ipInNets(peer, c.cfg.trusted) {
		// Not a balancer we trust: whatever it sent is payload, and its own
		// address stands. Warned once per peer because with the feature on, a
		// direct connection is either a misconfigured route or an attempt to
		// assert a source address.
		if untrustedProxyPeers.first(peerKey(peer)) {
			log.Warn().Str("peer", peerKey(peer)).
				Msg("proxy protocol: connection from a peer outside trustedCIDRs; no header accepted, the peer address is the client IP")
		}
		return nil
	}
	if c.cfg.timeout > 0 {
		_ = c.Conn.SetReadDeadline(time.Now().Add(c.cfg.timeout))
		defer func() { _ = c.Conn.SetReadDeadline(time.Time{}) }()
	}
	// Every valid header of either version is at least 15 bytes, so 12 is always
	// available and never over-reads into the payload.
	if err := c.fill(len(v2Signature)); err != nil {
		return fmt.Errorf("proxy protocol: reading header from %s: %w", peerKey(peer), err)
	}
	switch {
	case bytes.Equal(c.buf[:len(v2Signature)], v2Signature):
		return c.readV2(peer)
	case bytes.HasPrefix(c.buf, v1Prefix):
		return c.readV1(peer)
	default:
		// A trusted peer that sent no header at all is not an error: load
		// balancers commonly health-check the port with a bare connection. The
		// bytes stay in buf and are delivered as payload, with the peer address
		// as the client IP.
		if headerlessProxyPeers.first(peerKey(peer)) {
			log.Warn().Str("peer", peerKey(peer)).
				Msg("proxy protocol: trusted peer sent no PROXY header; treating the bytes as payload and using the peer address as the client IP")
		}
		return nil
	}
}

// fill reads until buf holds exactly n bytes. It never reads past n, so no
// payload byte is consumed by header parsing.
func (c *proxyProtoConn) fill(n int) error {
	if n > maxProxyProtoHeader {
		return fmt.Errorf("header length %d exceeds the %d-byte maximum", n, maxProxyProtoHeader)
	}
	for len(c.buf) < n {
		if cap(c.buf) < n {
			grown := make([]byte, len(c.buf), n)
			copy(grown, c.buf)
			c.buf = grown
		}
		m, err := c.Conn.Read(c.buf[len(c.buf):n])
		c.buf = c.buf[:len(c.buf)+m]
		if err != nil {
			return err
		}
	}
	return nil
}

// readV1 parses the text header: "PROXY TCP4 <src> <dst> <sport> <dport>\r\n".
func (c *proxyProtoConn) readV1(peer net.IP) error {
	for n := len(v1Prefix) + 2; n <= maxProxyProtoV1; n++ {
		if err := c.fill(n); err != nil {
			return fmt.Errorf("proxy protocol v1: reading header from %s: %w", peerKey(peer), err)
		}
		if c.buf[n-2] != '\r' || c.buf[n-1] != '\n' {
			continue
		}
		line := string(c.buf[:n-2])
		c.buf = c.buf[n:]
		addr, err := parseProxyV1Line(line)
		if err != nil {
			return fmt.Errorf("proxy protocol v1 from %s: %w", peerKey(peer), err)
		}
		if addr != nil {
			c.remote = addr
		}
		return nil
	}
	return fmt.Errorf("proxy protocol v1 from %s: no CRLF within %d bytes", peerKey(peer), maxProxyProtoV1)
}

// parseProxyV1Line returns the client address a v1 header asserts, or nil for
// the UNKNOWN transport (the sender declining to name one), in which case the
// real peer stands.
func parseProxyV1Line(line string) (net.Addr, error) {
	f := strings.Split(line, " ")
	if len(f) < 2 || f[0] != "PROXY" {
		return nil, fmt.Errorf("malformed header %q", line)
	}
	if f[1] == "UNKNOWN" {
		return nil, nil
	}
	if len(f) != 6 {
		return nil, fmt.Errorf("malformed header %q: want 6 fields, got %d", line, len(f))
	}
	ip := net.ParseIP(f[2])
	if ip == nil {
		return nil, fmt.Errorf("malformed source address %q", f[2])
	}
	switch f[1] {
	case "TCP4":
		if ip.To4() == nil {
			return nil, fmt.Errorf("TCP4 header with non-IPv4 source %q", f[2])
		}
	case "TCP6":
		// An IPv4 literal is not a valid TCP6 source: net.ParseIP would accept
		// it and silently produce a v4-mapped address the operator never sent.
		if strings.Contains(f[2], ".") {
			return nil, fmt.Errorf("TCP6 header with non-IPv6 source %q", f[2])
		}
	default:
		return nil, fmt.Errorf("unsupported transport %q", f[1])
	}
	if net.ParseIP(f[3]) == nil {
		return nil, fmt.Errorf("malformed destination address %q", f[3])
	}
	port, err := strconv.Atoi(f[4])
	if err != nil || port < 0 || port > 65535 {
		return nil, fmt.Errorf("malformed source port %q", f[4])
	}
	if p, err := strconv.Atoi(f[5]); err != nil || p < 0 || p > 65535 {
		return nil, fmt.Errorf("malformed destination port %q", f[5])
	}
	return &net.TCPAddr{IP: ip, Port: port}, nil
}

// readV2 parses the binary header. Any TLV vector after the address block is
// consumed (so the payload starts where it should) and ignored.
func (c *proxyProtoConn) readV2(peer net.IP) error {
	if err := c.fill(16); err != nil {
		return fmt.Errorf("proxy protocol v2: reading header from %s: %w", peerKey(peer), err)
	}
	verCmd, family := c.buf[12], c.buf[13]
	length := int(binary.BigEndian.Uint16(c.buf[14:16]))
	if verCmd>>4 != 0x2 {
		return fmt.Errorf("proxy protocol v2 from %s: unsupported version %d", peerKey(peer), verCmd>>4)
	}
	if err := c.fill(16 + length); err != nil {
		return fmt.Errorf("proxy protocol v2: reading address block from %s: %w", peerKey(peer), err)
	}
	body := append([]byte(nil), c.buf[16:16+length]...)
	c.buf = c.buf[16+length:]

	switch verCmd & 0x0f {
	case 0x0:
		return nil // LOCAL: the sender's own health check, no address to apply
	case 0x1: // PROXY
	default:
		return fmt.Errorf("proxy protocol v2 from %s: unsupported command %d", peerKey(peer), verCmd&0x0f)
	}
	switch family {
	case 0x00: // AF_UNSPEC: no address asserted, the real peer stands
		return nil
	case 0x11, 0x12: // TCP/UDP over IPv4
		if len(body) < 12 {
			return fmt.Errorf("proxy protocol v2 from %s: IPv4 address block truncated (%d bytes)", peerKey(peer), len(body))
		}
		c.remote = &net.TCPAddr{
			IP:   net.IP(append([]byte(nil), body[0:4]...)),
			Port: int(binary.BigEndian.Uint16(body[8:10])),
		}
	case 0x21, 0x22: // TCP/UDP over IPv6
		if len(body) < 36 {
			return fmt.Errorf("proxy protocol v2 from %s: IPv6 address block truncated (%d bytes)", peerKey(peer), len(body))
		}
		c.remote = &net.TCPAddr{
			IP:   net.IP(append([]byte(nil), body[0:16]...)),
			Port: int(binary.BigEndian.Uint16(body[32:34])),
		}
	default:
		// AF_UNIX and anything undefined: this is an IP edge, so there is no
		// sensible client IP to derive. Fail closed rather than guess.
		return fmt.Errorf("proxy protocol v2 from %s: unsupported address family 0x%02x", peerKey(peer), family)
	}
	return nil
}

// addrIP extracts the IP from a net.Addr, or nil for an address with none.
func addrIP(a net.Addr) net.IP {
	switch v := a.(type) {
	case *net.TCPAddr:
		return v.IP
	case *net.UDPAddr:
		return v.IP
	case nil:
		return nil
	}
	host, _, err := net.SplitHostPort(a.String())
	if err != nil {
		return net.ParseIP(a.String())
	}
	return net.ParseIP(host)
}

// peerKey renders a peer IP for logging and for the warn-once set.
func peerKey(ip net.IP) string {
	if ip == nil {
		return "unknown"
	}
	return ip.String()
}

// peerWarnSet remembers which peers have already produced a given warning, so a
// steady stream of connections logs once rather than once per connection.
type peerWarnSet struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

// first reports whether key has not been warned about yet, recording it.
func (s *peerWarnSet) first(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.seen[key]; ok {
		return false
	}
	if s.seen == nil || len(s.seen) >= maxWarnedPeers {
		s.seen = map[string]struct{}{}
	}
	s.seen[key] = struct{}{}
	return true
}

var (
	untrustedProxyPeers  = &peerWarnSet{}
	headerlessProxyPeers = &peerWarnSet{}
)
