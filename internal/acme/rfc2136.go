package acme

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

// DNS wire-format constants used by the RFC 2136 solver. Only the handful of
// codepoints a dynamic UPDATE needs are defined; there is no general-purpose DNS
// library here on purpose, since pulling one in for one solver would cost more
// dependency surface than the ~200 lines of packing below.
const (
	dnsTypeSOA  = 6
	dnsTypeTXT  = 16
	dnsTypeTSIG = 250

	dnsClassIN   = 1
	dnsClassNONE = 254 // RFC 2136 s2.5.4: "delete an individual RR"
	dnsClassANY  = 255 // TSIG RRs are class ANY

	dnsOpcodeUpdate = 5

	// tsigFudge is the accepted clock skew in seconds between this process and
	// the nameserver (RFC 8945 s5.2.1). 300 is the universal default.
	tsigFudge = 300

	// dnsUDPBufferSize bounds a UDP response read. Update replies are tiny; a
	// larger answer means the server set TC and we retry over TCP.
	dnsUDPBufferSize = 4096
)

// tsigAlg binds a TSIG algorithm identifier to its on-the-wire name and hash.
type tsigAlg struct {
	wire string
	new  func() hash.Hash
}

// tsigAlgorithms are the MAC algorithms this solver will sign with. HMAC-MD5 is
// deliberately absent: RFC 8945 keeps it only for backwards compatibility and it
// is refused with an explicit error rather than silently downgraded.
var tsigAlgorithms = map[string]tsigAlg{
	"hmac-sha1":   {"hmac-sha1.", sha1.New},
	"hmac-sha224": {"hmac-sha224.", sha256.New224},
	"hmac-sha256": {"hmac-sha256.", sha256.New},
	"hmac-sha384": {"hmac-sha384.", sha512.New384},
	"hmac-sha512": {"hmac-sha512.", sha512.New},
}

// dnsRcodes names the response codes an UPDATE can come back with, so a failure
// reads as "REFUSED" rather than "rcode 5".
var dnsRcodes = map[int]string{
	0: "NOERROR", 1: "FORMERR", 2: "SERVFAIL", 3: "NXDOMAIN", 4: "NOTIMP",
	5: "REFUSED", 6: "YXDOMAIN", 7: "YXRRSET", 8: "NXRRSET", 9: "NOTAUTH",
	10: "NOTZONE",
}

// RFC2136Config configures an RFC2136Solver. Only Server, KeyName and Secret are
// required; Zone is optional and auto-detected with SOA queries when empty.
type RFC2136Config struct {
	Server    string // host, host:port or [v6]:port; port defaults to 53
	Zone      string // zone to send the UPDATE to; auto-detected when empty
	KeyName   string // TSIG key name, exactly as configured on the nameserver
	Secret    string // base64 TSIG secret, as printed by tsig-keygen
	Algorithm string // one of tsigAlgorithms; default hmac-sha256
	TTL       int    // TTL of the challenge TXT record; default 60
	Transport string // "tcp" (default) or "udp"
	Timeout   time.Duration
}

// RFC2136Solver implements DNSSolver with RFC 2136 dynamic updates authenticated
// by a TSIG key (RFC 8945). It works against any nameserver that speaks dynamic
// update - BIND, Knot, PowerDNS, NSD with a helper - so a zone with no REST API
// still gets DNS-01, including wildcards.
type RFC2136Solver struct {
	server    string
	zone      string
	keyName   string
	secret    []byte
	alg       tsigAlg
	ttl       uint32
	transport string
	timeout   time.Duration

	mu    sync.Mutex
	zones map[string]string // challenge fqdn -> detected zone
}

// NewRFC2136Solver validates cfg and builds a solver. It performs no network I/O.
func NewRFC2136Solver(cfg RFC2136Config) (*RFC2136Solver, error) {
	server, err := normalizeDNSServer(cfg.Server)
	if err != nil {
		return nil, err
	}
	keyName := strings.TrimSpace(cfg.KeyName)
	if keyName == "" {
		return nil, errors.New("rfc2136: tsigKeyName is required")
	}
	secret := strings.TrimSpace(cfg.Secret)
	if secret == "" {
		return nil, errors.New("rfc2136: tsigSecret is required")
	}
	key, err := decodeTSIGSecret(secret)
	if err != nil {
		return nil, err
	}
	algName := strings.ToLower(strings.TrimSpace(cfg.Algorithm))
	if algName == "" {
		algName = "hmac-sha256"
	}
	algName = strings.TrimSuffix(algName, ".")
	if algName == "hmac-md5" || algName == "hmac-md5.sig-alg.reg.int" {
		return nil, errors.New("rfc2136: tsigAlgorithm hmac-md5 is refused; regenerate the key with hmac-sha256")
	}
	alg, ok := tsigAlgorithms[algName]
	if !ok {
		return nil, fmt.Errorf("rfc2136: unsupported tsigAlgorithm %q, want one of %s", cfg.Algorithm, strings.Join(supportedTSIGAlgorithms(), ", "))
	}
	transport := strings.ToLower(strings.TrimSpace(cfg.Transport))
	if transport == "" {
		transport = "tcp"
	}
	if transport != "tcp" && transport != "udp" {
		return nil, fmt.Errorf("rfc2136: transport must be tcp or udp, got %q", cfg.Transport)
	}
	ttl := cfg.TTL
	if ttl == 0 {
		ttl = 60
	}
	if ttl < 1 || ttl > 86400 {
		return nil, fmt.Errorf("rfc2136: ttl must be between 1 and 86400 seconds, got %d", cfg.TTL)
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &RFC2136Solver{
		server:    server,
		zone:      strings.TrimSuffix(strings.TrimSpace(cfg.Zone), "."),
		keyName:   strings.TrimSuffix(strings.ToLower(keyName), "."),
		secret:    key,
		alg:       alg,
		ttl:       uint32(ttl),
		transport: transport,
		timeout:   timeout,
		zones:     map[string]string{},
	}, nil
}

// supportedTSIGAlgorithms lists the accepted algorithm identifiers in a stable
// order, for error messages and for the model-side validation parity test.
func supportedTSIGAlgorithms() []string {
	return []string{"hmac-sha1", "hmac-sha224", "hmac-sha256", "hmac-sha384", "hmac-sha512"}
}

// decodeTSIGSecret accepts the padded base64 tsig-keygen prints, and unpadded
// base64 for operators who trimmed it.
func decodeTSIGSecret(s string) ([]byte, error) {
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	b, err := base64.RawStdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("rfc2136: tsigSecret is not valid base64: %w", err)
	}
	return b, nil
}

// normalizeDNSServer appends the default port 53 when cfg.Server carries none.
func normalizeDNSServer(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New("rfc2136: server is required (host or host:port)")
	}
	if _, port, err := net.SplitHostPort(s); err == nil && port != "" {
		return s, nil
	}
	// A bare IPv6 literal splits badly above; bracket it here.
	if ip := net.ParseIP(s); ip != nil {
		return net.JoinHostPort(s, "53"), nil
	}
	if strings.Contains(s, ":") {
		return "", fmt.Errorf("rfc2136: server %q is not a valid host or host:port (bracket an IPv6 literal, e.g. [2001:db8::1]:53)", s)
	}
	return net.JoinHostPort(s, "53"), nil
}

// Present adds the challenge TXT record with an RFC 2136 UPDATE. The record is
// added, never replaced, so an apex + wildcard order sharing one name works.
func (s *RFC2136Solver) Present(ctx context.Context, name, value string) error {
	return s.update(ctx, name, value, true)
}

// CleanUp deletes exactly the TXT RR that Present added (class NONE), leaving any
// other value at the same name alone.
func (s *RFC2136Solver) CleanUp(ctx context.Context, name, value string) error {
	return s.update(ctx, name, value, false)
}

func (s *RFC2136Solver) update(ctx context.Context, name, value string, add bool) error {
	zone, err := s.zoneFor(ctx, name)
	if err != nil {
		return err
	}
	msg, id, err := s.buildUpdate(zone, name, value, add, time.Now())
	if err != nil {
		return err
	}
	resp, err := s.exchange(ctx, msg)
	if err != nil {
		return err
	}
	return checkUpdateResponse(resp, id, zone)
}

// zoneFor returns the configured zone, or detects the closest enclosing zone by
// walking up the name with SOA queries and caches the result.
func (s *RFC2136Solver) zoneFor(ctx context.Context, name string) (string, error) {
	if s.zone != "" {
		return s.zone, nil
	}
	s.mu.Lock()
	zone, ok := s.zones[name]
	s.mu.Unlock()
	if ok {
		return zone, nil
	}
	zone, err := s.detectZone(ctx, name)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.zones[name] = zone
	s.mu.Unlock()
	return zone, nil
}

// detectZone queries SOA for each suffix of name until the server answers with
// one, which identifies the zone apex it is authoritative for.
func (s *RFC2136Solver) detectZone(ctx context.Context, name string) (string, error) {
	labels := strings.Split(strings.TrimSuffix(name, "."), ".")
	for i := 0; i+1 < len(labels); i++ {
		candidate := strings.Join(labels[i:], ".")
		msg, id, err := buildSOAQuery(candidate)
		if err != nil {
			return "", err
		}
		resp, err := s.exchange(ctx, msg)
		if err != nil {
			return "", err
		}
		owner, err := soaOwnerFromAnswer(resp, id)
		if err != nil {
			return "", err
		}
		if owner == "" {
			continue
		}
		// The reply is unauthenticated (a plain query, no TSIG on either side),
		// but the owner name it carries is packed into the zone section of a
		// TSIG-SIGNED update. Accept it only if it is actually a zone the name
		// being solved lives in: equal to the name or one of its parent
		// suffixes, and never the root. Otherwise a spoofed or hostile answer
		// picks the zone gpm signs an UPDATE for.
		if !zoneCoversName(owner, name) {
			return "", fmt.Errorf("rfc2136: SOA lookup for %q answered with zone %q, which is not a suffix of that name; refusing to send an update for it (set config.zone explicitly)", candidate, owner)
		}
		return owner, nil
	}
	return "", fmt.Errorf("rfc2136: could not detect the zone for %q from %s; set config.zone explicitly", name, s.server)
}

// zoneCoversName reports whether zone is a plausible zone for name: a non-root
// name equal to it or one of its parent suffixes. Comparison is case-insensitive
// and trailing dots are ignored, which is how DNS names compare.
func zoneCoversName(zone, name string) bool {
	z := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(zone), "."))
	n := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
	if z == "" || n == "" {
		return false
	}
	return n == z || strings.HasSuffix(n, "."+z)
}

// buildUpdate assembles and TSIG-signs an UPDATE message. add=true appends the
// TXT RR (class IN, configured TTL); add=false deletes that exact RR (class NONE,
// TTL 0, per RFC 2136 s2.5.4).
func (s *RFC2136Solver) buildUpdate(zone, name, value string, add bool, now time.Time) ([]byte, uint16, error) {
	if len(value) > 255 {
		return nil, 0, fmt.Errorf("rfc2136: TXT value for %q is %d bytes, over the 255-byte character-string limit", name, len(value))
	}
	id, err := dnsMessageID()
	if err != nil {
		return nil, 0, err
	}
	buf := make([]byte, 0, 512)
	buf = binary.BigEndian.AppendUint16(buf, id)
	buf = binary.BigEndian.AppendUint16(buf, uint16(dnsOpcodeUpdate)<<11)
	buf = binary.BigEndian.AppendUint16(buf, 1) // ZOCOUNT: one zone
	buf = binary.BigEndian.AppendUint16(buf, 0) // PRCOUNT: no prerequisites
	buf = binary.BigEndian.AppendUint16(buf, 1) // UPCOUNT: one RR
	buf = binary.BigEndian.AppendUint16(buf, 0) // ADCOUNT: signTSIG bumps this

	if buf, err = packName(buf, zone); err != nil {
		return nil, 0, err
	}
	buf = binary.BigEndian.AppendUint16(buf, dnsTypeSOA)
	buf = binary.BigEndian.AppendUint16(buf, dnsClassIN)

	if buf, err = packName(buf, name); err != nil {
		return nil, 0, err
	}
	class, ttl := uint16(dnsClassIN), s.ttl
	if !add {
		class, ttl = dnsClassNONE, 0
	}
	buf = binary.BigEndian.AppendUint16(buf, dnsTypeTXT)
	buf = binary.BigEndian.AppendUint16(buf, class)
	buf = binary.BigEndian.AppendUint32(buf, ttl)
	buf = binary.BigEndian.AppendUint16(buf, uint16(len(value)+1)) // RDLENGTH
	buf = append(buf, byte(len(value)))
	buf = append(buf, value...)

	signed, err := s.signTSIG(buf, now)
	if err != nil {
		return nil, 0, err
	}
	return signed, id, nil
}

// signTSIG appends a TSIG RR to msg. The MAC covers the message as it stands
// (ADCOUNT still excluding the TSIG) followed by the "TSIG variables" of RFC
// 8945 s4.3.3; ADCOUNT is incremented only after the MAC is computed.
func (s *RFC2136Solver) signTSIG(msg []byte, now time.Time) ([]byte, error) {
	if len(msg) < 12 {
		return nil, errors.New("rfc2136: message too short to sign")
	}
	timeSigned := uint64(now.Unix())

	vars, err := packName(nil, s.keyName)
	if err != nil {
		return nil, err
	}
	vars = binary.BigEndian.AppendUint16(vars, dnsClassANY)
	vars = binary.BigEndian.AppendUint32(vars, 0) // TTL
	if vars, err = packName(vars, s.alg.wire); err != nil {
		return nil, err
	}
	vars = appendUint48(vars, timeSigned)
	vars = binary.BigEndian.AppendUint16(vars, tsigFudge)
	vars = binary.BigEndian.AppendUint16(vars, 0) // error
	vars = binary.BigEndian.AppendUint16(vars, 0) // other len

	mac := hmac.New(s.alg.new, s.secret)
	mac.Write(msg)
	mac.Write(vars)
	sum := mac.Sum(nil)

	rdata, err := packName(nil, s.alg.wire)
	if err != nil {
		return nil, err
	}
	rdata = appendUint48(rdata, timeSigned)
	rdata = binary.BigEndian.AppendUint16(rdata, tsigFudge)
	rdata = binary.BigEndian.AppendUint16(rdata, uint16(len(sum)))
	rdata = append(rdata, sum...)
	rdata = binary.BigEndian.AppendUint16(rdata, binary.BigEndian.Uint16(msg[0:2])) // original ID
	rdata = binary.BigEndian.AppendUint16(rdata, 0)                                 // error
	rdata = binary.BigEndian.AppendUint16(rdata, 0)                                 // other len

	out := append([]byte{}, msg...)
	if out, err = packName(out, s.keyName); err != nil {
		return nil, err
	}
	out = binary.BigEndian.AppendUint16(out, dnsTypeTSIG)
	out = binary.BigEndian.AppendUint16(out, dnsClassANY)
	out = binary.BigEndian.AppendUint32(out, 0)
	out = binary.BigEndian.AppendUint16(out, uint16(len(rdata)))
	out = append(out, rdata...)
	binary.BigEndian.PutUint16(out[10:12], 1) // ADCOUNT now counts the TSIG
	return out, nil
}

// buildSOAQuery builds an unsigned SOA query for zone detection.
func buildSOAQuery(name string) ([]byte, uint16, error) {
	id, err := dnsMessageID()
	if err != nil {
		return nil, 0, err
	}
	buf := make([]byte, 0, 64)
	buf = binary.BigEndian.AppendUint16(buf, id)
	buf = binary.BigEndian.AppendUint16(buf, 0) // standard query, no recursion
	buf = binary.BigEndian.AppendUint16(buf, 1) // QDCOUNT
	buf = binary.BigEndian.AppendUint16(buf, 0)
	buf = binary.BigEndian.AppendUint16(buf, 0)
	buf = binary.BigEndian.AppendUint16(buf, 0)
	if buf, err = packName(buf, name); err != nil {
		return nil, 0, err
	}
	buf = binary.BigEndian.AppendUint16(buf, dnsTypeSOA)
	buf = binary.BigEndian.AppendUint16(buf, dnsClassIN)
	return buf, id, nil
}

// exchange sends msg and returns the raw response. UDP falls back to TCP when the
// answer is truncated.
func (s *RFC2136Solver) exchange(ctx context.Context, msg []byte) ([]byte, error) {
	if s.transport == "udp" {
		resp, truncated, err := s.exchangeUDP(ctx, msg)
		if err != nil {
			return nil, err
		}
		if !truncated {
			return resp, nil
		}
	}
	return s.exchangeTCP(ctx, msg)
}

func (s *RFC2136Solver) exchangeUDP(ctx context.Context, msg []byte) ([]byte, bool, error) {
	conn, err := s.dial(ctx, "udp")
	if err != nil {
		return nil, false, err
	}
	defer conn.Close()
	if _, err := conn.Write(msg); err != nil {
		return nil, false, fmt.Errorf("rfc2136: write to %s: %w", s.server, err)
	}
	buf := make([]byte, dnsUDPBufferSize)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, false, fmt.Errorf("rfc2136: read from %s: %w", s.server, err)
	}
	if n < 12 {
		return nil, false, fmt.Errorf("rfc2136: short response from %s (%d bytes)", s.server, n)
	}
	return buf[:n], buf[2]&0x02 != 0, nil
}

func (s *RFC2136Solver) exchangeTCP(ctx context.Context, msg []byte) ([]byte, error) {
	conn, err := s.dial(ctx, "tcp")
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	framed := binary.BigEndian.AppendUint16(make([]byte, 0, len(msg)+2), uint16(len(msg)))
	if _, err := conn.Write(append(framed, msg...)); err != nil {
		return nil, fmt.Errorf("rfc2136: write to %s: %w", s.server, err)
	}
	var lenBuf [2]byte
	if _, err := io.ReadFull(conn, lenBuf[:]); err != nil {
		return nil, fmt.Errorf("rfc2136: read from %s: %w", s.server, err)
	}
	resp := make([]byte, binary.BigEndian.Uint16(lenBuf[:]))
	if _, err := io.ReadFull(conn, resp); err != nil {
		return nil, fmt.Errorf("rfc2136: read from %s: %w", s.server, err)
	}
	if len(resp) < 12 {
		return nil, fmt.Errorf("rfc2136: short response from %s (%d bytes)", s.server, len(resp))
	}
	return resp, nil
}

func (s *RFC2136Solver) dial(ctx context.Context, network string) (net.Conn, error) {
	d := net.Dialer{Timeout: s.timeout}
	conn, err := d.DialContext(ctx, network, s.server)
	if err != nil {
		return nil, fmt.Errorf("rfc2136: dial %s %s: %w", network, s.server, err)
	}
	deadline := time.Now().Add(s.timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		conn.Close()
		return nil, fmt.Errorf("rfc2136: set deadline: %w", err)
	}
	return conn, nil
}

// checkUpdateResponse validates the reply header. The response TSIG is not
// verified: a forged NOERROR could only hide a failure the DNS propagation check
// catches before the CA is ever asked to validate the challenge.
func checkUpdateResponse(resp []byte, id uint16, zone string) error {
	if len(resp) < 12 {
		return fmt.Errorf("rfc2136: short response (%d bytes)", len(resp))
	}
	if got := binary.BigEndian.Uint16(resp[0:2]); got != id {
		return fmt.Errorf("rfc2136: response id %d does not match request id %d", got, id)
	}
	// QR must be set and the opcode must still be UPDATE: a message that is not
	// a reply to this request (a stray query, an all-zero datagram that happens
	// to carry the right ID) must never be read as "the update succeeded".
	if resp[2]&0x80 == 0 {
		return errors.New("rfc2136: server answered with a message that is not a response (QR=0)")
	}
	if op := int(resp[2]>>3) & 0x0F; op != dnsOpcodeUpdate {
		return fmt.Errorf("rfc2136: response opcode %d is not an update (%d)", op, dnsOpcodeUpdate)
	}
	rcode := int(resp[3] & 0x0F)
	if rcode == 0 {
		return nil
	}
	name, ok := dnsRcodes[rcode]
	if !ok {
		name = fmt.Sprintf("rcode %d", rcode)
	}
	switch rcode {
	case 5: // REFUSED
		return fmt.Errorf("rfc2136: update for zone %q refused (%s): check the server's update-policy/allow-update for this TSIG key", zone, name)
	case 9: // NOTAUTH
		return fmt.Errorf("rfc2136: update for zone %q rejected (%s): the TSIG key name, secret, algorithm or the local clock is wrong", zone, name)
	case 10: // NOTZONE
		return fmt.Errorf("rfc2136: update rejected (%s): the record is not inside zone %q", name, zone)
	default:
		return fmt.Errorf("rfc2136: update for zone %q failed: %s", zone, name)
	}
}

// dnsMessageID draws a message ID from crypto/rand so an off-path attacker cannot
// predict it and forge a reply.
func dnsMessageID() (uint16, error) {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, fmt.Errorf("rfc2136: generate message id: %w", err)
	}
	return binary.BigEndian.Uint16(b[:]), nil
}

func appendUint48(dst []byte, v uint64) []byte {
	return append(dst, byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// packName appends name in DNS wire format (length-prefixed labels, root
// terminator). Names are lowercased, which is also the canonical form the TSIG
// MAC is computed over.
func packName(dst []byte, name string) ([]byte, error) {
	name = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
	if name == "" {
		return append(dst, 0), nil
	}
	total := 1
	for _, label := range strings.Split(name, ".") {
		if label == "" {
			return nil, fmt.Errorf("dns: empty label in name %q", name)
		}
		if len(label) > 63 {
			return nil, fmt.Errorf("dns: label %q in %q exceeds 63 bytes", label, name)
		}
		total += len(label) + 1
		if total > 255 {
			return nil, fmt.Errorf("dns: name %q exceeds 255 bytes", name)
		}
		dst = append(dst, byte(len(label)))
		dst = append(dst, label...)
	}
	return append(dst, 0), nil
}

// dnsReader walks a DNS message, resolving compression pointers.
type dnsReader struct {
	msg []byte
	off int
}

func (r *dnsReader) u16() (uint16, error) {
	b, err := r.take(2)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(b), nil
}

func (r *dnsReader) u32() (uint32, error) {
	b, err := r.take(4)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(b), nil
}

func (r *dnsReader) take(n int) ([]byte, error) {
	if n < 0 || r.off+n > len(r.msg) {
		return nil, io.ErrUnexpectedEOF
	}
	b := r.msg[r.off : r.off+n]
	r.off += n
	return b, nil
}

// name reads a (possibly compressed) domain name and returns it lowercased with
// no trailing dot. The root is "".
func (r *dnsReader) name() (string, error) {
	var labels []string
	off, ret, jumps := r.off, -1, 0
	for {
		if off >= len(r.msg) {
			return "", io.ErrUnexpectedEOF
		}
		n := int(r.msg[off])
		switch {
		case n == 0:
			off++
			if ret < 0 {
				ret = off
			}
			r.off = ret
			return strings.Join(labels, "."), nil
		case n&0xC0 == 0xC0:
			if off+1 >= len(r.msg) {
				return "", io.ErrUnexpectedEOF
			}
			ptr := int(binary.BigEndian.Uint16(r.msg[off:off+2]) & 0x3FFF)
			off += 2
			if ret < 0 {
				ret = off
			}
			jumps++
			if jumps > 16 || ptr >= len(r.msg) {
				return "", errors.New("dns: bad compression pointer")
			}
			off = ptr
		case n&0xC0 != 0:
			return "", errors.New("dns: reserved label type")
		default:
			if off+1+n > len(r.msg) {
				return "", io.ErrUnexpectedEOF
			}
			labels = append(labels, strings.ToLower(string(r.msg[off+1:off+1+n])))
			off += 1 + n
		}
	}
}

// soaOwnerFromAnswer returns the owner name of the first SOA in the answer
// section, or "" when there is none (which means "keep walking up").
func soaOwnerFromAnswer(resp []byte, id uint16) (string, error) {
	if len(resp) < 12 {
		return "", fmt.Errorf("rfc2136: short SOA response (%d bytes)", len(resp))
	}
	if got := binary.BigEndian.Uint16(resp[0:2]); got != id {
		return "", fmt.Errorf("rfc2136: SOA response id %d does not match request id %d", got, id)
	}
	if resp[2]&0x80 == 0 {
		return "", errors.New("rfc2136: SOA lookup answered with a message that is not a response (QR=0)")
	}
	if rcode := int(resp[3] & 0x0F); rcode != 0 && rcode != 3 {
		// NXDOMAIN is a normal "not this suffix"; anything else is a real failure.
		name, ok := dnsRcodes[rcode]
		if !ok {
			name = fmt.Sprintf("rcode %d", rcode)
		}
		return "", fmt.Errorf("rfc2136: SOA lookup failed: %s", name)
	}
	qd := int(binary.BigEndian.Uint16(resp[4:6]))
	an := int(binary.BigEndian.Uint16(resp[6:8]))
	r := &dnsReader{msg: resp, off: 12}
	for i := 0; i < qd; i++ {
		if _, err := r.name(); err != nil {
			return "", err
		}
		if _, err := r.take(4); err != nil {
			return "", err
		}
	}
	for i := 0; i < an; i++ {
		owner, err := r.name()
		if err != nil {
			return "", err
		}
		typ, err := r.u16()
		if err != nil {
			return "", err
		}
		if _, err := r.take(6); err != nil { // class + ttl
			return "", err
		}
		rdlen, err := r.u16()
		if err != nil {
			return "", err
		}
		if _, err := r.take(int(rdlen)); err != nil {
			return "", err
		}
		if typ == dnsTypeSOA {
			return owner, nil
		}
	}
	return "", nil
}
