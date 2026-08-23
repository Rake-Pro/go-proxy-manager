package dataplane

import (
	"encoding/binary"
	"errors"
	"net"
	"strings"
	"time"
)

// TLS ClientHello SNI extraction, hand-written against RFC 8446 section 4.1.2
// (the ClientHello is wire-compatible back to TLS 1.0) and RFC 6066 section 3
// (server_name), using only the standard library.
//
// Stream hosts need the server name BEFORE deciding what to do with the
// connection: in passthrough mode gpm must route to a backend without ever
// holding the key, and in terminate mode it must know which certificate to
// present. crypto/tls can only surface the name after it has agreed to be one
// endpoint of the handshake, so the record is parsed directly instead. Every
// read is bounded (maxTLSRecord per record, maxClientHello overall) so a peer
// cannot make gpm buffer indefinitely before a route exists.

const (
	recordTypeHandshake  = 0x16
	handshakeClientHello = 0x01
	extensionServerName  = 0x0000

	// maxTLSRecord is the largest TLS record body the spec allows (2^14).
	maxTLSRecord = 16384
	// maxClientHello bounds the reassembled handshake message across records.
	maxClientHello = 16384
	// clientHelloTimeout bounds how long a stream client has to deliver its
	// ClientHello. A connection that never sends one holds a goroutine and a
	// file descriptor for nothing.
	clientHelloTimeout = 10 * time.Second
)

var (
	errNotTLS         = errors.New("connection does not begin with a TLS handshake record")
	errBadClientHello = errors.New("malformed TLS ClientHello")
	errHelloTooLarge  = errors.New("TLS ClientHello exceeds the maximum size")
)

// peekClientHello reads the ClientHello from c and returns the lower-cased SNI
// server name (empty when the client sent none) together with EVERY byte it
// consumed. The caller replays those bytes - to the backend for passthrough, or
// to crypto/tls for termination - so the handshake is never disturbed.
func peekClientHello(c net.Conn, timeout time.Duration) (string, []byte, error) {
	if timeout > 0 {
		_ = c.SetReadDeadline(time.Now().Add(timeout))
		defer func() { _ = c.SetReadDeadline(time.Time{}) }()
	}
	var raw, hs []byte
	for {
		var err error
		if raw, err = readUpTo(c, raw, len(raw)+5); err != nil {
			return "", raw, err
		}
		rec := raw[len(raw)-5:]
		if rec[0] != recordTypeHandshake {
			return "", raw, errNotTLS
		}
		n := int(binary.BigEndian.Uint16(rec[3:5]))
		if n == 0 || n > maxTLSRecord {
			return "", raw, errBadClientHello
		}
		if raw, err = readUpTo(c, raw, len(raw)+n); err != nil {
			return "", raw, err
		}
		hs = append(hs, raw[len(raw)-n:]...)
		if len(hs) > maxClientHello {
			return "", raw, errHelloTooLarge
		}
		// A ClientHello may be split across records; keep reading until the
		// handshake message is complete.
		name, done, err := parseClientHello(hs)
		if err != nil {
			return "", raw, err
		}
		if done {
			return name, raw, nil
		}
	}
}

// readUpTo appends socket bytes to buf until it holds exactly n, never reading
// past n so nothing beyond the ClientHello is consumed.
func readUpTo(c net.Conn, buf []byte, n int) ([]byte, error) {
	if n > maxClientHello+maxTLSRecord {
		return buf, errHelloTooLarge
	}
	for len(buf) < n {
		if cap(buf) < n {
			grown := make([]byte, len(buf), n)
			copy(grown, buf)
			buf = grown
		}
		m, err := c.Read(buf[len(buf):n])
		buf = buf[:len(buf)+m]
		if err != nil {
			return buf, err
		}
	}
	return buf, nil
}

// parseClientHello extracts the SNI server name from a (possibly incomplete)
// handshake message. done is false when more record data is needed; an error
// means the bytes are not a well-formed ClientHello and the caller must give up.
func parseClientHello(hs []byte) (name string, done bool, err error) {
	if len(hs) < 4 {
		return "", false, nil
	}
	if hs[0] != handshakeClientHello {
		return "", false, errBadClientHello
	}
	n := int(hs[1])<<16 | int(hs[2])<<8 | int(hs[3])
	if n > maxClientHello {
		return "", false, errHelloTooLarge
	}
	if len(hs) < 4+n {
		return "", false, nil
	}
	b := hs[4 : 4+n]

	// legacy_version (2) + random (32)
	if len(b) < 34 {
		return "", false, errBadClientHello
	}
	b = b[34:]
	// legacy_session_id
	if b, err = skipVector(b, 1); err != nil {
		return "", false, err
	}
	// cipher_suites
	if b, err = skipVector(b, 2); err != nil {
		return "", false, err
	}
	// legacy_compression_methods
	if b, err = skipVector(b, 1); err != nil {
		return "", false, err
	}
	// extensions: absent entirely is legal (and means no SNI)
	if len(b) < 2 {
		return "", true, nil
	}
	extLen := int(binary.BigEndian.Uint16(b))
	b = b[2:]
	if len(b) < extLen {
		return "", false, errBadClientHello
	}
	b = b[:extLen]
	for len(b) >= 4 {
		typ := binary.BigEndian.Uint16(b)
		l := int(binary.BigEndian.Uint16(b[2:4]))
		b = b[4:]
		if len(b) < l {
			return "", false, errBadClientHello
		}
		if typ == extensionServerName {
			return parseServerNameExtension(b[:l])
		}
		b = b[l:]
	}
	return "", true, nil
}

// parseServerNameExtension returns the first host_name entry of an RFC 6066
// server_name extension.
func parseServerNameExtension(b []byte) (string, bool, error) {
	if len(b) < 2 {
		return "", false, errBadClientHello
	}
	listLen := int(binary.BigEndian.Uint16(b))
	b = b[2:]
	if len(b) < listLen {
		return "", false, errBadClientHello
	}
	b = b[:listLen]
	for len(b) >= 3 {
		nameType := b[0]
		l := int(binary.BigEndian.Uint16(b[1:3]))
		b = b[3:]
		if len(b) < l {
			return "", false, errBadClientHello
		}
		if nameType == 0 { // host_name
			return strings.ToLower(strings.TrimSuffix(string(b[:l]), ".")), true, nil
		}
		b = b[l:]
	}
	return "", true, nil
}

// skipVector advances past a length-prefixed vector whose length occupies
// lenBytes (1 or 2) leading bytes.
func skipVector(b []byte, lenBytes int) ([]byte, error) {
	if len(b) < lenBytes {
		return nil, errBadClientHello
	}
	n := 0
	switch lenBytes {
	case 1:
		n = int(b[0])
	case 2:
		n = int(binary.BigEndian.Uint16(b))
	}
	b = b[lenBytes:]
	if len(b) < n {
		return nil, errBadClientHello
	}
	return b[n:], nil
}

// prefixConn replays already-read bytes before continuing on the socket, so a
// peeked ClientHello can be handed intact to crypto/tls or to a backend.
type prefixConn struct {
	net.Conn
	prefix []byte
}

func (c *prefixConn) Read(p []byte) (int, error) {
	if len(c.prefix) > 0 {
		n := copy(p, c.prefix)
		c.prefix = c.prefix[n:]
		return n, nil
	}
	return c.Conn.Read(p)
}
