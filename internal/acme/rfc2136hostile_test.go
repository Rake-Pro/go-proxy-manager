package acme

import (
	"context"
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"
)

// hostileSOAServer answers EVERY query with an SOA whose owner is a zone the
// caller never asked about. The SOA probes that drive zone auto-detection are
// plain, unauthenticated queries, so this is what an off-path spoofer (or a
// hostile resolver) gets to say - and the answer used to be packed straight into
// the zone section of a TSIG-SIGNED update.
func hostileSOAServer(t *testing.T, owner string) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 4096)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			msg := append([]byte{}, buf[:n]...)
			if len(msg) < 12 {
				continue
			}
			r := &dnsReader{msg: msg, off: 12}
			if _, err := r.name(); err != nil {
				continue
			}
			question := msg[12:r.off]
			qtypeClass, err := r.take(4)
			if err != nil {
				continue
			}
			out := append([]byte{}, msg[0:2]...)
			out = binary.BigEndian.AppendUint16(out, 1<<15|1<<10) // QR + AA
			out = binary.BigEndian.AppendUint16(out, 1)           // QDCOUNT
			out = binary.BigEndian.AppendUint16(out, 1)           // ANCOUNT
			out = binary.BigEndian.AppendUint16(out, 0)
			out = binary.BigEndian.AppendUint16(out, 0)
			out = append(out, question...)
			out = append(out, qtypeClass...)
			out, _ = packName(out, owner)
			out = binary.BigEndian.AppendUint16(out, dnsTypeSOA)
			out = binary.BigEndian.AppendUint16(out, dnsClassIN)
			out = binary.BigEndian.AppendUint32(out, 3600)
			rdata, _ := packName(nil, "ns1."+owner)
			rdata, _ = packName(rdata, "hostmaster."+owner)
			for i := 0; i < 5; i++ {
				rdata = binary.BigEndian.AppendUint32(rdata, 3600)
			}
			out = binary.BigEndian.AppendUint16(out, uint16(len(rdata)))
			out = append(out, rdata...)
			_, _ = pc.WriteTo(out, from)
		}
	}()
	return pc.LocalAddr().String()
}

// TestDetectZoneRefusesForeignSOAOwner: an SOA owner that is not a suffix of the
// name being solved is refused rather than signed for.
func TestDetectZoneRefusesForeignSOAOwner(t *testing.T) {
	addr := hostileSOAServer(t, "attacker.net")
	cfg := testRFC2136Config(addr)
	cfg.Zone = ""
	cfg.Transport = "udp"
	cfg.Timeout = 10 * time.Second
	s, err := NewRFC2136Solver(cfg)
	if err != nil {
		t.Fatalf("NewRFC2136Solver() = %v", err)
	}
	err = s.Present(context.Background(), "_acme-challenge.example.com", "value")
	if err == nil {
		t.Fatal("Present() = nil, want a refusal of the foreign SOA owner")
	}
	if !strings.Contains(err.Error(), "not a suffix") {
		t.Fatalf("Present() = %v, want an error naming the suffix check", err)
	}
}

// TestZoneCoversName is the suffix rule itself.
func TestZoneCoversName(t *testing.T) {
	tests := []struct {
		zone, name string
		want       bool
	}{
		{"example.com", "_acme-challenge.example.com", true},
		{"example.com.", "_acme-challenge.example.com.", true},
		{"EXAMPLE.com", "_acme-challenge.example.COM", true},
		{"_acme-challenge.example.com", "_acme-challenge.example.com", true},
		{"attacker.net", "_acme-challenge.example.com", false},
		{"notexample.com", "_acme-challenge.example.com", false},
		{"", "_acme-challenge.example.com", false},
		{".", "_acme-challenge.example.com", false},
		{"example.com", "", false},
	}
	for _, tc := range tests {
		if got := zoneCoversName(tc.zone, tc.name); got != tc.want {
			t.Errorf("zoneCoversName(%q, %q) = %v, want %v", tc.zone, tc.name, got, tc.want)
		}
	}
}

// TestCheckUpdateResponseRejectsNonResponse: a message that is not a reply to
// this UPDATE must never read as success, even when its ID matches - an all-zero
// datagram used to be accepted as NOERROR.
func TestCheckUpdateResponseRejectsNonResponse(t *testing.T) {
	header := func(flags uint16) []byte {
		out := binary.BigEndian.AppendUint16(nil, 0x1234)
		out = binary.BigEndian.AppendUint16(out, flags)
		for i := 0; i < 4; i++ {
			out = binary.BigEndian.AppendUint16(out, 0)
		}
		return out
	}
	tests := []struct {
		name    string
		msg     []byte
		wantErr string
	}{
		{"all-zero message with a matching id", make([]byte, 12), "response id"},
		{"query bit clear", header(uint16(dnsOpcodeUpdate) << 11), "QR=0"},
		{"wrong opcode", header(1 << 15), "opcode"},
		{"well-formed update response", header(1<<15 | uint16(dnsOpcodeUpdate)<<11), ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := checkUpdateResponse(tc.msg, 0x1234, "example.com")
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("checkUpdateResponse() = %v, want nil", err)
			case tc.wantErr == "":
			case err == nil:
				t.Fatalf("checkUpdateResponse() = nil, want an error containing %q", tc.wantErr)
			case !strings.Contains(err.Error(), tc.wantErr):
				t.Fatalf("checkUpdateResponse() = %v, want an error containing %q", err, tc.wantErr)
			}
		})
	}
}
