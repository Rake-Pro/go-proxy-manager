package dataplane

import (
	"bufio"
	"crypto/tls"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// headerLimitListener serves a counting handler on a loopback listener built
// the way Start builds the data-plane listeners, with TLS (h2 + http/1.1) when
// tlsOn is set. It returns the address and the handler call counter.
func headerLimitListener(t *testing.T, c Config, tlsOn bool) (string, *atomic.Int64) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	calls := new(atomic.Int64)
	srv := New(c).listenerServer(ln.Addr().String(), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, "ok")
	}))
	if tlsOn {
		srv.TLSConfig = &tls.Config{NextProtos: []string{"h2", "http/1.1"}, Certificates: []tls.Certificate{testTLSCert(t)}}
		go func() { _ = srv.ServeTLS(ln, "", "") }()
	} else {
		go func() { _ = srv.Serve(ln) }()
	}
	t.Cleanup(func() { _ = srv.Close() })
	return ln.Addr().String(), calls
}

// TestListenerMaxHeaderBytesConfig proves Config.MaxHeaderBytes reaches the
// listeners: 0 selects the 64 KiB default and out-of-range values clamp.
func TestListenerMaxHeaderBytesConfig(t *testing.T) {
	for _, tc := range []struct {
		in, want int
	}{
		{0, DefaultMaxHeaderBytes},
		{8192, 8192},
		{1, MinMaxHeaderBytes},
		{MaxMaxHeaderBytes + 1, MaxMaxHeaderBytes},
	} {
		srv := New(Config{MaxHeaderBytes: tc.in}).listenerServer(":0", http.NotFoundHandler())
		if srv.MaxHeaderBytes != tc.want {
			t.Fatalf("MaxHeaderBytes %d: listener got %d, want %d", tc.in, srv.MaxHeaderBytes, tc.want)
		}
	}
	if got := newListenerServer(":0", http.NotFoundHandler()).MaxHeaderBytes; got != DefaultMaxHeaderBytes {
		t.Fatalf("newListenerServer MaxHeaderBytes = %d, want %d", got, DefaultMaxHeaderBytes)
	}
}

func TestValidateMaxHeaderBytes(t *testing.T) {
	for _, tc := range []struct {
		n  int
		ok bool
	}{
		{MinMaxHeaderBytes - 1, false},
		{MinMaxHeaderBytes, true},
		{DefaultMaxHeaderBytes, true},
		{MaxMaxHeaderBytes, true},
		{MaxMaxHeaderBytes + 1, false},
		{0, false},
		{-1, false},
	} {
		if err := ValidateMaxHeaderBytes(tc.n); (err == nil) != tc.ok {
			t.Fatalf("ValidateMaxHeaderBytes(%d) = %v, want ok=%v", tc.n, err, tc.ok)
		}
	}
}

// TestListenerHTTP1HeaderLimit proves an HTTP/1.1 request whose header block
// exceeds the limit (plus the stdlib's 4096 bytes of read slack) gets 431 and
// never reaches the handler, while one just under the limit passes.
func TestListenerHTTP1HeaderLimit(t *testing.T) {
	for _, tc := range []struct {
		name  string
		limit int // Config.MaxHeaderBytes; 0 = default
		size  int // total bytes of request line plus headers, final CRLF included
		want  int
	}{
		{"default just under", 0, DefaultMaxHeaderBytes, http.StatusOK},
		{"default over", 0, DefaultMaxHeaderBytes + 4096 + 1, http.StatusRequestHeaderFieldsTooLarge},
		{"default one MiB", 0, 1 << 20, http.StatusRequestHeaderFieldsTooLarge},
		{"configured just under", 8192, 8192, http.StatusOK},
		{"configured over", 8192, 8192 + 4096 + 1, http.StatusRequestHeaderFieldsTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			addr, calls := headerLimitListener(t, Config{MaxHeaderBytes: tc.limit}, false)
			head := "GET / HTTP/1.1\r\nHost: app.example.com\r\nX-Big: "
			tail := "\r\n\r\n"
			req := head + strings.Repeat("a", tc.size-len(head)-len(tail)) + tail
			c, err := net.Dial("tcp", addr)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = c.Close() }()
			_ = c.SetDeadline(time.Now().Add(5 * time.Second))
			go func() { _, _ = io.WriteString(c, req) }()
			resp, err := http.ReadResponse(bufio.NewReader(c), nil)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.want)
			}
			wantCalls := int64(0)
			if tc.want == http.StatusOK {
				wantCalls = 1
			}
			if got := calls.Load(); got != wantCalls {
				t.Fatalf("handler called %d times, want %d", got, wantCalls)
			}
		})
	}
}

// TestListenerHTTP2HeaderLimit proves the limit also holds on HTTP/2: the
// listener advertises it (plus the stdlib's 320 bytes of per-field overhead)
// as SETTINGS_MAX_HEADER_LIST_SIZE and answers a longer header list with 431
// without calling the handler, while a small h2 request passes.
func TestListenerHTTP2HeaderLimit(t *testing.T) {
	for _, limit := range []int{0, 8192} {
		t.Run("limit "+strconv.Itoa(limit), func(t *testing.T) {
			effective := limit
			if effective == 0 {
				effective = DefaultMaxHeaderBytes
			}
			addr, calls := headerLimitListener(t, Config{MaxHeaderBytes: limit}, true)

			resp, err := h2Client(t, 0).Get("https://" + addr + "/")
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.ProtoMajor != 2 || resp.StatusCode != http.StatusOK {
				t.Fatalf("small request: %s %d, want HTTP/2 200", resp.Proto, resp.StatusCode)
			}
			calls.Store(0)

			maxList, body, goaway := h2OversizedRequest(t, addr, effective, effective+320)
			if goaway >= 0 {
				t.Fatalf("server sent GOAWAY (code %d) instead of a 431", goaway)
			}
			if want := uint32(effective + 320); maxList != want {
				t.Fatalf("advertised MAX_HEADER_LIST_SIZE = %d, want %d", maxList, want)
			}
			if !strings.Contains(body, "HTTP Error 431") {
				t.Fatalf("oversized header list got body %q, want the 431 page", body)
			}
			if got := calls.Load(); got != 0 {
				t.Fatalf("handler called %d times for an oversized header list", got)
			}
		})
	}
}

// TestListenerHTTP2HeaderLimitContinuationClosesConnection pins what Go does
// with a header list that overflows the limit and then continues in further
// CONTINUATION frames (a 3x-limit block sent in 16 KiB frames): no
// 431, but a connection error (GOAWAY, PROTOCOL_ERROR), and the handler is
// never called.
func TestListenerHTTP2HeaderLimitContinuationClosesConnection(t *testing.T) {
	addr, calls := headerLimitListener(t, Config{}, true)
	_, body, goaway := h2OversizedRequest(t, addr, DefaultMaxHeaderBytes, 3*DefaultMaxHeaderBytes)
	if goaway != 0x1 {
		t.Fatalf("3x-limit header block: GOAWAY code %d, body %q; want GOAWAY PROTOCOL_ERROR (1)", goaway, body)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("handler called %d times for an oversized header list", got)
	}
}

// h2OversizedRequest speaks raw HTTP/2 to addr: it reads the server's
// MAX_HEADER_LIST_SIZE and sends one GET whose header list just exceeds total
// (HPACK field sizes), as fields that are each under limit (one field over it
// would be an HPACK error), split into 16 KiB HEADERS and CONTINUATION frames.
// With total = limit+320 the overflowing field is the last one, so no
// CONTINUATION follows the truncation. It returns the advertised size, the
// response body and the GOAWAY error code (-1 when the server sent none).
func h2OversizedRequest(t *testing.T, addr string, limit, total int) (uint32, string, int) {
	t.Helper()
	c, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2"}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if p := c.ConnectionState().NegotiatedProtocol; p != "h2" {
		t.Fatalf("negotiated %q, want h2", p)
	}
	writeFrame := func(typ, flags byte, stream uint32, payload []byte) {
		t.Helper()
		hdr := make([]byte, 9)
		hdr[0], hdr[1], hdr[2] = byte(len(payload)>>16), byte(len(payload)>>8), byte(len(payload))
		hdr[3], hdr[4] = typ, flags
		binary.BigEndian.PutUint32(hdr[5:], stream)
		if _, err := c.Write(append(hdr, payload...)); err != nil {
			t.Fatal(err)
		}
	}
	br := bufio.NewReader(c)
	readFrame := func() (typ, flags byte, stream uint32, payload []byte) {
		t.Helper()
		hdr := make([]byte, 9)
		if _, err := io.ReadFull(br, hdr); err != nil {
			t.Fatalf("reading frame: %v", err)
		}
		payload = make([]byte, int(hdr[0])<<16|int(hdr[1])<<8|int(hdr[2]))
		if _, err := io.ReadFull(br, payload); err != nil {
			t.Fatalf("reading frame payload: %v", err)
		}
		return hdr[3], hdr[4], binary.BigEndian.Uint32(hdr[5:]) & 0x7fffffff, payload
	}
	const (
		frameData         = 0x0
		frameHeaders      = 0x1
		frameRSTStream    = 0x3
		frameSettings     = 0x4
		frameGoAway       = 0x7
		frameContinuation = 0x9
		flagEndStream     = 0x1
		flagAck           = 0x1
		flagEndHeaders    = 0x4
	)

	if _, err := io.WriteString(c, "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	writeFrame(frameSettings, 0, 0, nil)
	var maxList uint32
	for {
		typ, flags, _, p := readFrame()
		if typ != frameSettings || flags&flagAck != 0 {
			continue
		}
		for i := 0; i+6 <= len(p); i += 6 {
			if binary.BigEndian.Uint16(p[i:]) == 0x6 { // SETTINGS_MAX_HEADER_LIST_SIZE
				maxList = binary.BigEndian.Uint32(p[i+2:])
			}
		}
		writeFrame(frameSettings, flagAck, 0, nil)
		break
	}

	// HPACK: indexed :method GET, :scheme https, :path /; :authority as a
	// literal with an indexed name; then literal fields with new names. No
	// Huffman coding and nothing added to the dynamic table.
	hpackInt := func(first byte, prefix uint, v int) []byte {
		mx := 1<<prefix - 1
		if v < mx {
			return []byte{first | byte(v)}
		}
		out := []byte{first | byte(mx)}
		for v -= mx; v >= 128; v >>= 7 {
			out = append(out, byte(v%128)|0x80)
		}
		return append(out, byte(v))
	}
	str := func(s string) []byte { return append(hpackInt(0, 7, len(s)), s...) }
	block := []byte{0x82, 0x87, 0x84}
	block = append(append(block, hpackInt(0x00, 4, 1)...), str("app.example.com")...)
	fieldLen := limit / 8
	for i, size := 0, 0; size <= total; i++ {
		name := "x-big-" + strconv.Itoa(i)
		block = append(block, 0x00)
		block = append(block, str(name)...)
		block = append(block, str(strings.Repeat("a", fieldLen))...)
		size += len(name) + fieldLen + 32 // HPACK field size
	}
	const maxFrame = 16384 // the default every peer accepts
	for first := true; ; first = false {
		n := min(len(block), maxFrame)
		typ, flags := byte(frameContinuation), byte(0)
		if first {
			typ, flags = frameHeaders, flagEndStream
		}
		if n == len(block) {
			flags |= flagEndHeaders
		}
		writeFrame(typ, flags, 1, block[:n])
		if block = block[n:]; len(block) == 0 {
			break
		}
	}

	var body strings.Builder
	for {
		typ, flags, stream, p := readFrame()
		switch {
		case typ == frameGoAway:
			return maxList, body.String(), int(binary.BigEndian.Uint32(p[4:]))
		case typ == frameRSTStream && stream == 1:
			t.Fatalf("server reset the stream (code %d) instead of answering 431", binary.BigEndian.Uint32(p))
		case typ == frameData && stream == 1:
			body.Write(p)
			if flags&flagEndStream != 0 {
				return maxList, body.String(), -1
			}
		case typ == frameHeaders && stream == 1 && flags&flagEndStream != 0:
			return maxList, body.String(), -1
		}
	}
}
