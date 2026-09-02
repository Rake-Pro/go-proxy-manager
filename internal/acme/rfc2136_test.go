package acme

import (
	"context"
	"crypto/hmac"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// ---------------------------------------------------------------------------
// In-process fake nameserver
//
// It speaks just enough DNS to be a real counterparty for the solver: it parses
// the UPDATE off the wire, recomputes the TSIG MAC from the shared secret and
// rejects a bad signature with NOTAUTH, and answers SOA queries so zone
// auto-detection has something to walk up to. No network beyond loopback.
// ---------------------------------------------------------------------------

// recordedRR is one RR the fake server saw in an update section.
type recordedRR struct {
	name  string
	typ   uint16
	class uint16
	ttl   uint32
	txt   string
}

// fakeDNS is a loopback UDP+TCP nameserver.
type fakeDNS struct {
	keyName string
	secret  []byte
	alg     tsigAlg
	// soaZones are the names the server will answer an SOA query for.
	soaZones []string
	// forceTruncate makes the first UDP reply set TC, exercising the TCP retry.
	forceTruncate bool

	udp *net.UDPConn
	tcp net.Listener

	mu      sync.Mutex
	updates []recordedRR
	zones   []string // zone name of each update received
}

func newFakeDNS(t *testing.T, keyName, secretB64 string, alg tsigAlg, soaZones ...string) *fakeDNS {
	t.Helper()
	secret, err := base64.StdEncoding.DecodeString(secretB64)
	if err != nil {
		t.Fatalf("decode test secret: %v", err)
	}
	f := &fakeDNS{keyName: strings.ToLower(keyName), secret: secret, alg: alg, soaZones: soaZones}

	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("resolve udp: %v", err)
	}
	f.udp, err = net.ListenUDP("udp", udpAddr)
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	// Bind TCP to the same port so one "host:port" reaches both transports.
	f.tcp, err = net.Listen("tcp", f.udp.LocalAddr().String())
	if err != nil {
		f.udp.Close()
		t.Fatalf("listen tcp: %v", err)
	}
	go f.serveUDP()
	go f.serveTCP()
	t.Cleanup(func() { f.udp.Close(); f.tcp.Close() })
	return f
}

func (f *fakeDNS) addr() string { return f.udp.LocalAddr().String() }

// setTruncate flips the "answer UDP with a truncated reply" switch under the
// same mutex serveUDP reads it with: the servers are already running by the time
// a test sets it.
func (f *fakeDNS) setTruncate(v bool) {
	f.mu.Lock()
	f.forceTruncate = v
	f.mu.Unlock()
}

func (f *fakeDNS) serveUDP() {
	buf := make([]byte, 4096)
	for {
		n, from, err := f.udp.ReadFromUDP(buf)
		if err != nil {
			return
		}
		req := append([]byte{}, buf[:n]...)
		f.mu.Lock()
		truncate := f.forceTruncate
		f.mu.Unlock()
		resp := f.handle(req)
		if truncate && len(resp) >= 12 {
			resp = append([]byte{}, resp[:12]...)
			resp[2] |= 0x02 // TC
		}
		if _, err := f.udp.WriteToUDP(resp, from); err != nil {
			return
		}
	}
}

func (f *fakeDNS) serveTCP() {
	for {
		conn, err := f.tcp.Accept()
		if err != nil {
			return
		}
		go func() {
			defer conn.Close()
			var lenBuf [2]byte
			if _, err := io.ReadFull(conn, lenBuf[:]); err != nil {
				return
			}
			req := make([]byte, binary.BigEndian.Uint16(lenBuf[:]))
			if _, err := io.ReadFull(conn, req); err != nil {
				return
			}
			resp := f.handle(req)
			out := binary.BigEndian.AppendUint16(nil, uint16(len(resp)))
			conn.Write(append(out, resp...))
		}()
	}
}

// handle dispatches on opcode: 0 is the SOA query used for zone detection, 5 is
// the dynamic update.
func (f *fakeDNS) handle(msg []byte) []byte {
	if len(msg) < 12 {
		return nil
	}
	id := binary.BigEndian.Uint16(msg[0:2])
	opcode := (binary.BigEndian.Uint16(msg[2:4]) >> 11) & 0xF
	switch opcode {
	case 0:
		return f.answerSOA(msg)
	case dnsOpcodeUpdate:
		req, err := parseFakeUpdate(msg)
		if err != nil {
			return dnsHeaderResponse(id, dnsOpcodeUpdate, 1) // FORMERR
		}
		if !f.verifyTSIG(msg, req) {
			return dnsHeaderResponse(id, dnsOpcodeUpdate, 9) // NOTAUTH
		}
		f.mu.Lock()
		f.updates = append(f.updates, req.rrs...)
		f.zones = append(f.zones, req.zone)
		f.mu.Unlock()
		return dnsHeaderResponse(id, dnsOpcodeUpdate, 0)
	default:
		return dnsHeaderResponse(id, opcode, 4) // NOTIMP
	}
}

func (f *fakeDNS) verifyTSIG(msg []byte, req *fakeUpdate) bool {
	if req.tsig == nil {
		return false
	}
	if req.tsig.keyName != f.keyName || req.tsig.algName != strings.TrimSuffix(f.alg.wire, ".") {
		return false
	}
	stripped := append([]byte{}, msg[:req.tsig.start]...)
	binary.BigEndian.PutUint16(stripped[10:12], req.arcount-1)

	vars, err := packName(nil, req.tsig.keyName)
	if err != nil {
		return false
	}
	vars = binary.BigEndian.AppendUint16(vars, dnsClassANY)
	vars = binary.BigEndian.AppendUint32(vars, 0)
	if vars, err = packName(vars, req.tsig.algName); err != nil {
		return false
	}
	vars = appendUint48(vars, req.tsig.timeSigned)
	vars = binary.BigEndian.AppendUint16(vars, req.tsig.fudge)
	vars = binary.BigEndian.AppendUint16(vars, 0)
	vars = binary.BigEndian.AppendUint16(vars, 0)

	h := hmac.New(f.alg.new, f.secret)
	h.Write(stripped)
	h.Write(vars)
	return hmac.Equal(h.Sum(nil), req.tsig.mac)
}

// answerSOA replies with an SOA record when the question name is one of the
// server's zones, and an empty NOERROR otherwise.
func (f *fakeDNS) answerSOA(msg []byte) []byte {
	r := &dnsReader{msg: msg, off: 12}
	qname, err := r.name()
	if err != nil {
		return dnsHeaderResponse(binary.BigEndian.Uint16(msg[0:2]), 0, 1)
	}
	question := msg[12:r.off]
	qtypeClass, err := r.take(4)
	if err != nil {
		return dnsHeaderResponse(binary.BigEndian.Uint16(msg[0:2]), 0, 1)
	}
	authoritative := false
	for _, z := range f.soaZones {
		if strings.EqualFold(z, qname) {
			authoritative = true
		}
	}
	out := append([]byte{}, msg[0:2]...)
	out = binary.BigEndian.AppendUint16(out, 1<<15|1<<10) // QR + AA
	out = binary.BigEndian.AppendUint16(out, 1)
	if authoritative {
		out = binary.BigEndian.AppendUint16(out, 1)
	} else {
		out = binary.BigEndian.AppendUint16(out, 0)
	}
	out = binary.BigEndian.AppendUint16(out, 0)
	out = binary.BigEndian.AppendUint16(out, 0)
	out = append(out, question...)
	out = append(out, qtypeClass...)
	if !authoritative {
		return out
	}
	out, _ = packName(out, qname)
	out = binary.BigEndian.AppendUint16(out, dnsTypeSOA)
	out = binary.BigEndian.AppendUint16(out, dnsClassIN)
	out = binary.BigEndian.AppendUint32(out, 3600)
	rdata, _ := packName(nil, "ns1."+qname)
	rdata, _ = packName(rdata, "hostmaster."+qname)
	for i := 0; i < 5; i++ {
		rdata = binary.BigEndian.AppendUint32(rdata, 3600)
	}
	out = binary.BigEndian.AppendUint16(out, uint16(len(rdata)))
	return append(out, rdata...)
}

func (f *fakeDNS) recorded() []recordedRR {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recordedRR{}, f.updates...)
}

func (f *fakeDNS) seenZones() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.zones...)
}

// dnsHeaderResponse builds a bare 12-byte reply with QR set.
func dnsHeaderResponse(id uint16, opcode uint16, rcode int) []byte {
	out := binary.BigEndian.AppendUint16(nil, id)
	out = binary.BigEndian.AppendUint16(out, 1<<15|opcode<<11|uint16(rcode))
	for i := 0; i < 4; i++ {
		out = binary.BigEndian.AppendUint16(out, 0)
	}
	return out
}

type fakeTSIG struct {
	start      int
	keyName    string
	algName    string
	timeSigned uint64
	fudge      uint16
	mac        []byte
}

type fakeUpdate struct {
	zone    string
	arcount uint16
	rrs     []recordedRR
	tsig    *fakeTSIG
}

// parseFakeUpdate reads an UPDATE message the way a nameserver would.
func parseFakeUpdate(msg []byte) (*fakeUpdate, error) {
	r := &dnsReader{msg: msg, off: 4}
	zocount, err := r.u16()
	if err != nil {
		return nil, err
	}
	prcount, err := r.u16()
	if err != nil {
		return nil, err
	}
	upcount, err := r.u16()
	if err != nil {
		return nil, err
	}
	arcount, err := r.u16()
	if err != nil {
		return nil, err
	}
	if zocount != 1 {
		return nil, fmt.Errorf("want 1 zone section, got %d", zocount)
	}
	u := &fakeUpdate{arcount: arcount}
	if u.zone, err = r.name(); err != nil {
		return nil, err
	}
	typ, err := r.u16()
	if err != nil {
		return nil, err
	}
	if typ != dnsTypeSOA {
		return nil, fmt.Errorf("zone section type = %d, want SOA", typ)
	}
	if _, err := r.u16(); err != nil { // class
		return nil, err
	}
	for i := 0; i < int(prcount)+int(upcount); i++ {
		rr, rdata, err := readFakeRR(r)
		if err != nil {
			return nil, err
		}
		if rr.typ == dnsTypeTXT && len(rdata) > 0 {
			rr.txt = string(rdata[1:])
		}
		if i >= int(prcount) {
			u.rrs = append(u.rrs, rr)
		}
	}
	for i := 0; i < int(arcount); i++ {
		start := r.off
		rr, rdata, err := readFakeRR(r)
		if err != nil {
			return nil, err
		}
		if rr.typ != dnsTypeTSIG {
			continue
		}
		rd := &dnsReader{msg: rdata}
		alg, err := rd.name()
		if err != nil {
			return nil, err
		}
		hi, err := rd.u16()
		if err != nil {
			return nil, err
		}
		lo, err := rd.u32()
		if err != nil {
			return nil, err
		}
		fudge, err := rd.u16()
		if err != nil {
			return nil, err
		}
		macSize, err := rd.u16()
		if err != nil {
			return nil, err
		}
		mac, err := rd.take(int(macSize))
		if err != nil {
			return nil, err
		}
		u.tsig = &fakeTSIG{
			start:      start,
			keyName:    rr.name,
			algName:    alg,
			timeSigned: uint64(hi)<<32 | uint64(lo),
			fudge:      fudge,
			mac:        append([]byte{}, mac...),
		}
	}
	return u, nil
}

func readFakeRR(r *dnsReader) (recordedRR, []byte, error) {
	var rr recordedRR
	var err error
	if rr.name, err = r.name(); err != nil {
		return rr, nil, err
	}
	if rr.typ, err = r.u16(); err != nil {
		return rr, nil, err
	}
	if rr.class, err = r.u16(); err != nil {
		return rr, nil, err
	}
	if rr.ttl, err = r.u32(); err != nil {
		return rr, nil, err
	}
	rdlen, err := r.u16()
	if err != nil {
		return rr, nil, err
	}
	rdata, err := r.take(int(rdlen))
	if err != nil {
		return rr, nil, err
	}
	return rr, rdata, nil
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

const testTSIGSecret = "c2VjcmV0LXRzaWcta2V5LWZvci11bml0LXRlc3Rz" // base64 of a fixed literal

func testRFC2136Config(addr string) RFC2136Config {
	return RFC2136Config{
		Server:    addr,
		Zone:      "example.com",
		KeyName:   "gpm-key",
		Secret:    testTSIGSecret,
		Algorithm: "hmac-sha256",
		TTL:       60,
		Timeout:   5 * time.Second,
	}
}

func TestRFC2136PresentAndCleanUp(t *testing.T) {
	for _, transport := range []string{"tcp", "udp"} {
		t.Run(transport, func(t *testing.T) {
			srv := newFakeDNS(t, "gpm-key", testTSIGSecret, tsigAlgorithms["hmac-sha256"])
			cfg := testRFC2136Config(srv.addr())
			cfg.Transport = transport
			s, err := NewRFC2136Solver(cfg)
			if err != nil {
				t.Fatalf("NewRFC2136Solver() = %v", err)
			}
			ctx := context.Background()
			if err := s.Present(ctx, "_acme-challenge.example.com", "token-value"); err != nil {
				t.Fatalf("Present() = %v", err)
			}
			if err := s.CleanUp(ctx, "_acme-challenge.example.com", "token-value"); err != nil {
				t.Fatalf("CleanUp() = %v", err)
			}
			got := srv.recorded()
			if len(got) != 2 {
				t.Fatalf("recorded %d RRs, want 2: %+v", len(got), got)
			}
			add, del := got[0], got[1]
			if add.name != "_acme-challenge.example.com" || add.typ != dnsTypeTXT {
				t.Errorf("add RR = %+v, want a TXT at _acme-challenge.example.com", add)
			}
			if add.class != dnsClassIN || add.ttl != 60 {
				t.Errorf("add RR class/ttl = %d/%d, want %d/60", add.class, add.ttl, dnsClassIN)
			}
			if add.txt != "token-value" {
				t.Errorf("add RR txt = %q, want %q", add.txt, "token-value")
			}
			if del.class != dnsClassNONE || del.ttl != 0 {
				t.Errorf("delete RR class/ttl = %d/%d, want %d/0 (RFC 2136 s2.5.4)", del.class, del.ttl, dnsClassNONE)
			}
			if del.txt != "token-value" {
				t.Errorf("delete RR txt = %q, want the exact value that was added", del.txt)
			}
			for _, z := range srv.seenZones() {
				if z != "example.com" {
					t.Errorf("zone section = %q, want example.com", z)
				}
			}
		})
	}
}

func TestRFC2136AllAlgorithms(t *testing.T) {
	for _, alg := range model.TSIGAlgorithms {
		t.Run(alg, func(t *testing.T) {
			srv := newFakeDNS(t, "gpm-key", testTSIGSecret, tsigAlgorithms[alg])
			cfg := testRFC2136Config(srv.addr())
			cfg.Algorithm = alg
			s, err := NewRFC2136Solver(cfg)
			if err != nil {
				t.Fatalf("NewRFC2136Solver() = %v", err)
			}
			if err := s.Present(context.Background(), "_acme-challenge.example.com", "v"); err != nil {
				t.Fatalf("Present() = %v", err)
			}
			if len(srv.recorded()) != 1 {
				t.Fatalf("server did not accept the %s signature", alg)
			}
		})
	}
}

func TestRFC2136WrongSecretIsRejected(t *testing.T) {
	srv := newFakeDNS(t, "gpm-key", testTSIGSecret, tsigAlgorithms["hmac-sha256"])
	cfg := testRFC2136Config(srv.addr())
	cfg.Secret = base64.StdEncoding.EncodeToString([]byte("a-different-key"))
	s, err := NewRFC2136Solver(cfg)
	if err != nil {
		t.Fatalf("NewRFC2136Solver() = %v", err)
	}
	err = s.Present(context.Background(), "_acme-challenge.example.com", "v")
	if err == nil {
		t.Fatal("Present() = nil, want a NOTAUTH error")
	}
	if !strings.Contains(err.Error(), "NOTAUTH") {
		t.Errorf("Present() = %q, want it to mention NOTAUTH", err)
	}
	if len(srv.recorded()) != 0 {
		t.Errorf("server recorded %d RRs, want 0 for a bad MAC", len(srv.recorded()))
	}
}

func TestRFC2136ZoneAutoDetect(t *testing.T) {
	srv := newFakeDNS(t, "gpm-key", testTSIGSecret, tsigAlgorithms["hmac-sha256"], "example.com")
	cfg := testRFC2136Config(srv.addr())
	cfg.Zone = ""
	s, err := NewRFC2136Solver(cfg)
	if err != nil {
		t.Fatalf("NewRFC2136Solver() = %v", err)
	}
	if err := s.Present(context.Background(), "_acme-challenge.app.example.com", "v"); err != nil {
		t.Fatalf("Present() = %v", err)
	}
	zones := srv.seenZones()
	if len(zones) != 1 || zones[0] != "example.com" {
		t.Fatalf("detected zones = %v, want [example.com]", zones)
	}
	// The detection result is cached: a second call must not re-query.
	if err := s.CleanUp(context.Background(), "_acme-challenge.app.example.com", "v"); err != nil {
		t.Fatalf("CleanUp() = %v", err)
	}
	if got := srv.seenZones(); len(got) != 2 {
		t.Fatalf("zones seen = %v, want two updates", got)
	}
}

func TestRFC2136ZoneAutoDetectNoMatch(t *testing.T) {
	srv := newFakeDNS(t, "gpm-key", testTSIGSecret, tsigAlgorithms["hmac-sha256"])
	cfg := testRFC2136Config(srv.addr())
	cfg.Zone = ""
	s, err := NewRFC2136Solver(cfg)
	if err != nil {
		t.Fatalf("NewRFC2136Solver() = %v", err)
	}
	err = s.Present(context.Background(), "_acme-challenge.example.com", "v")
	if err == nil || !strings.Contains(err.Error(), "set config.zone explicitly") {
		t.Fatalf("Present() = %v, want an error telling the operator to set config.zone", err)
	}
}

func TestRFC2136UDPTruncationFallsBackToTCP(t *testing.T) {
	srv := newFakeDNS(t, "gpm-key", testTSIGSecret, tsigAlgorithms["hmac-sha256"])
	srv.setTruncate(true)
	cfg := testRFC2136Config(srv.addr())
	cfg.Transport = "udp"
	s, err := NewRFC2136Solver(cfg)
	if err != nil {
		t.Fatalf("NewRFC2136Solver() = %v", err)
	}
	if err := s.Present(context.Background(), "_acme-challenge.example.com", "v"); err != nil {
		t.Fatalf("Present() = %v", err)
	}
	// One truncated UDP attempt plus the TCP retry: two updates recorded.
	if got := srv.recorded(); len(got) != 2 {
		t.Fatalf("recorded %d RRs, want 2 (truncated UDP try + TCP retry)", len(got))
	}
}

func TestRFC2136UnreachableServer(t *testing.T) {
	// Port 1 on loopback: nothing listens, and the dial fails fast.
	cfg := testRFC2136Config("127.0.0.1:1")
	cfg.Timeout = 2 * time.Second
	s, err := NewRFC2136Solver(cfg)
	if err != nil {
		t.Fatalf("NewRFC2136Solver() = %v", err)
	}
	if err := s.Present(context.Background(), "_acme-challenge.example.com", "v"); err == nil {
		t.Fatal("Present() = nil, want a dial error")
	}
}

func TestNewRFC2136SolverValidation(t *testing.T) {
	base := testRFC2136Config("ns1.example.com:53")
	cases := []struct {
		name    string
		mutate  func(*RFC2136Config)
		wantErr string
	}{
		{"missing server", func(c *RFC2136Config) { c.Server = "" }, "server is required"},
		{"missing key name", func(c *RFC2136Config) { c.KeyName = "" }, "tsigKeyName is required"},
		{"missing secret", func(c *RFC2136Config) { c.Secret = "" }, "tsigSecret is required"},
		{"bad secret", func(c *RFC2136Config) { c.Secret = "not base64!!" }, "not valid base64"},
		{"hmac-md5 refused", func(c *RFC2136Config) { c.Algorithm = "hmac-md5" }, "hmac-md5 is refused"},
		{"unknown algorithm", func(c *RFC2136Config) { c.Algorithm = "hmac-sha3" }, "unsupported tsigAlgorithm"},
		{"bad transport", func(c *RFC2136Config) { c.Transport = "quic" }, "transport must be tcp or udp"},
		{"ttl too large", func(c *RFC2136Config) { c.TTL = 90000 }, "ttl must be between"},
		{"ambiguous server", func(c *RFC2136Config) { c.Server = "ns1.example.com:53:53" }, "bracket an IPv6 literal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			tc.mutate(&cfg)
			_, err := NewRFC2136Solver(cfg)
			if err == nil {
				t.Fatalf("NewRFC2136Solver() = nil, want an error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("NewRFC2136Solver() = %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestNewRFC2136SolverDefaults(t *testing.T) {
	s, err := NewRFC2136Solver(RFC2136Config{Server: "ns1.example.com", KeyName: "k", Secret: testTSIGSecret})
	if err != nil {
		t.Fatalf("NewRFC2136Solver() = %v", err)
	}
	if s.server != "ns1.example.com:53" {
		t.Errorf("server = %q, want the default port appended", s.server)
	}
	if s.transport != "tcp" {
		t.Errorf("transport = %q, want tcp", s.transport)
	}
	if s.ttl != 60 {
		t.Errorf("ttl = %d, want 60", s.ttl)
	}
	if s.timeout != 30*time.Second {
		t.Errorf("timeout = %s, want 30s", s.timeout)
	}
	if s.alg.wire != "hmac-sha256." {
		t.Errorf("algorithm = %q, want hmac-sha256.", s.alg.wire)
	}
}

func TestNormalizeDNSServer(t *testing.T) {
	cases := []struct{ in, want string }{
		{"ns1.example.com", "ns1.example.com:53"},
		{"ns1.example.com:5353", "ns1.example.com:5353"},
		{"192.0.2.10", "192.0.2.10:53"},
		{"192.0.2.10:53", "192.0.2.10:53"},
		{"2001:db8::1", "[2001:db8::1]:53"},
		{"[2001:db8::1]:5353", "[2001:db8::1]:5353"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := normalizeDNSServer(tc.in)
			if err != nil {
				t.Fatalf("normalizeDNSServer(%q) = %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("normalizeDNSServer(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestPackName(t *testing.T) {
	got, err := packName(nil, "Example.COM.")
	if err != nil {
		t.Fatalf("packName() = %v", err)
	}
	want := []byte{7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 3, 'c', 'o', 'm', 0}
	if string(got) != string(want) {
		t.Errorf("packName() = %v, want %v (lowercased, length-prefixed)", got, want)
	}
	if _, err := packName(nil, "a..b"); err == nil {
		t.Error("packName(\"a..b\") = nil error, want an empty-label error")
	}
	if _, err := packName(nil, strings.Repeat("x", 64)+".example.com"); err == nil {
		t.Error("packName(64-byte label) = nil error, want a label-length error")
	}
}

func TestDNSReaderNameCompression(t *testing.T) {
	// "example.com" at offset 12, then a pointer back to it from offset 25.
	msg := make([]byte, 12)
	msg, _ = packName(msg, "example.com")
	msg = append(msg, 0xC0, 12)
	r := &dnsReader{msg: msg, off: 25}
	got, err := r.name()
	if err != nil {
		t.Fatalf("name() = %v", err)
	}
	if got != "example.com" {
		t.Errorf("name() = %q, want example.com", got)
	}
	if r.off != 27 {
		t.Errorf("offset after pointer = %d, want 27", r.off)
	}
	// A pointer to itself must not loop forever.
	loop := []byte{0xC0, 0}
	if _, err := (&dnsReader{msg: loop}).name(); err == nil {
		t.Error("name() on a self-referential pointer = nil error, want a bad-pointer error")
	}
	if _, err := (&dnsReader{msg: []byte{3, 'a'}}).name(); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("name() on a truncated label = %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestTSIGAlgorithmParity(t *testing.T) {
	if len(model.TSIGAlgorithms) != len(tsigAlgorithms) {
		t.Fatalf("model.TSIGAlgorithms has %d entries, solver supports %d", len(model.TSIGAlgorithms), len(tsigAlgorithms))
	}
	for _, name := range model.TSIGAlgorithms {
		if _, ok := tsigAlgorithms[name]; !ok {
			t.Errorf("model advertises %q but the solver has no implementation", name)
		}
	}
	for i, name := range supportedTSIGAlgorithms() {
		if model.TSIGAlgorithms[i] != name {
			t.Errorf("supportedTSIGAlgorithms()[%d] = %q, want %q", i, name, model.TSIGAlgorithms[i])
		}
	}
}
