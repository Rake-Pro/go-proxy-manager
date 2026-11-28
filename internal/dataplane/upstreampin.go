package dataplane

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// Upstream certificate pinning (model.UpstreamTLS.PinnedCertSHA256).
//
// With pins, the TLS connection to that upstream is accepted if and only if the
// SHA-256 of the leaf certificate the server presents equals one of the pins.
// Chain building, name matching and validity dates are NOT evaluated; the pin
// is the whole trust decision. Everything else about the connection is
// unchanged: protocol versions, ALPN / HTTP/2 negotiation, and SNI (taken from
// the upstream's host, never from hostHeader). Without pins the upstream rides
// the shared dataplaneTransport (or its timeout-override clone) exactly as
// before.
//
// Pool isolation: every distinct (timeouts, pin set) gets its own
// http.Transport, so its own connection pool. A connection verified under one
// pin set can therefore never serve an upstream with no pins or with another
// pin set, even at the same host:port, and the reverse. Go's transport keys
// pooled connections (HTTP/1 and HTTP/2 alike) by scheme and host:port inside
// one Transport and does not coalesce HTTP/2 connections across authorities,
// so within one pinned transport a connection also only serves the address it
// was dialled for.
//
// Lifetime: the transports live in the Server's reload-surviving chainStates,
// keyed by pinnedTransportKey and held weakly, like rate limiters. A reload
// whose spec is unchanged gets the same transport back (its pool survives); a
// transport the new router no longer uses has its idle connections closed at
// the router swap (closeReplacedIdle) and is collected with the old router.
// The set is bounded by the live config: one transport per distinct key the
// validated config names, never per request or per peer.

// pinnedTLSConfig is the client TLS config of a pinned upstream. It is the ONLY
// place in gpm that sets InsecureSkipVerify, and only together with the pin
// check in VerifyConnection, which also runs on a resumed session (unlike
// VerifyPeerCertificate). A guard test fails if InsecureSkipVerify appears
// anywhere else in non-test code.
func pinnedTLSConfig(pins [][sha256.Size]byte) *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true,
		VerifyConnection: func(cs tls.ConnectionState) error {
			return checkCertPin(cs, pins)
		},
	}
}

// pinMismatchError is the cause of a refused pinned handshake. presented is
// the leaf's SHA-256 in lower-case hex; it is public, so it is logged.
type pinMismatchError struct{ presented string }

func (e *pinMismatchError) Error() string {
	if e.presented == "" {
		return "upstream presented no certificate to check against the pinned fingerprints"
	}
	return "presented certificate sha256 " + e.presented + " does not match any pinned fingerprint"
}

// checkCertPin accepts the connection iff the SHA-256 of the presented leaf
// certificate (DER) equals one of pins. Full 32-byte digests are compared, never
// a prefix; constant time is not needed (pins and certificates are public). The
// failure is a *tls.CertificateVerificationError so it is classified exactly
// like a certificate verification failure today (502, connect-phase retry and
// passive health accounting in an upstream group).
func checkCertPin(cs tls.ConnectionState, pins [][sha256.Size]byte) error {
	if len(cs.PeerCertificates) == 0 {
		return &tls.CertificateVerificationError{Err: &pinMismatchError{}}
	}
	sum := sha256.Sum256(cs.PeerCertificates[0].Raw)
	if slices.Contains(pins, sum) {
		return nil
	}
	return &tls.CertificateVerificationError{
		UnverifiedCertificates: cs.PeerCertificates,
		Err:                    &pinMismatchError{presented: hex.EncodeToString(sum[:])},
	}
}

// pinMismatch returns the presented fingerprint when err is a refused pin.
func pinMismatch(err error) (string, bool) {
	var pm *pinMismatchError
	if errors.As(err, &pm) {
		return pm.presented, true
	}
	return "", false
}

// pinDigests decodes validated pins. An entry that does not decode (only an
// unvalidated config can carry one) is dropped, never replaced by "no pin": a
// pinned upstream with no usable digest refuses every handshake.
func pinDigests(t model.UpstreamTLS) [][sha256.Size]byte {
	out := make([][sha256.Size]byte, 0, len(t.PinnedCertSHA256))
	for _, p := range t.PinnedCertSHA256 {
		n, ok := model.NormalizeCertFingerprint(p)
		if !ok {
			continue
		}
		var d [sha256.Size]byte
		if _, err := hex.Decode(d[:], []byte(n)); err != nil {
			continue
		}
		out = append(out, d)
	}
	return out
}

// pinnedTransportKey identifies the transport of a pinned upstream: the host's
// effective timeout override plus the sorted pin set. Two upstreams with the
// same key make the same trust decision, so they may share a pool.
func pinnedTransportKey(to *model.HostTimeouts, t model.UpstreamTLS) string {
	c, r := 0, 0
	if to != nil {
		c, r = max(to.ConnectSeconds, 0), max(to.ReadSeconds, 0)
	}
	return "pin|c=" + strconv.Itoa(c) + "|r=" + strconv.Itoa(r) + "|" + strings.Join(pinSet(t), ",")
}

// pinSet is the effective pin set of an upstream as sorted lower-case hex: nil
// without pins, and non-nil (possibly empty, when no entry decodes) with them,
// so "no pins" and "only malformed pins" never compare equal.
func pinSet(t model.UpstreamTLS) []string {
	if t.IsZero() {
		return nil
	}
	pins := make([]string, 0, len(t.PinnedCertSHA256))
	for _, d := range pinDigests(t) {
		pins = append(pins, hex.EncodeToString(d[:]))
	}
	slices.Sort(pins)
	return pins
}

// newPinnedTransport is the transport transportFor would give the host, as its
// own clone, with the pin check as its TLS config.
func newPinnedTransport(to *model.HostTimeouts, t model.UpstreamTLS) *http.Transport {
	tr, ok := transportFor(to).(*http.Transport)
	if !ok || tr == dataplaneTransport {
		tr = dataplaneTransport.Clone()
	}
	tr.TLSClientConfig = pinnedTLSConfig(pinDigests(t))
	return tr
}

// pinnedTransport returns the reload-surviving transport for one pinned
// upstream and records it as a pool this build uses.
func (reg *registry) pinnedTransport(to *model.HostTimeouts, t model.UpstreamTLS) *http.Transport {
	mk := func() *http.Transport { return newPinnedTransport(to, t) }
	var tr *http.Transport
	if reg.states != nil {
		tr = reg.states.transport(pinnedTransportKey(to, t), mk)
	} else {
		tr = mk()
	}
	reg.own(tr)
	return tr
}

// upstreamTransport is the RoundTripper for a single upstream of a host with
// the given timeouts: the pinned transport when the upstream has pins,
// otherwise exactly what transportFor returns.
func (reg *registry) upstreamTransport(to *model.HostTimeouts, up model.Upstream) http.RoundTripper {
	if !up.TLS.IsZero() {
		return reg.pinnedTransport(to, up.TLS)
	}
	tr := transportFor(to)
	if c, ok := tr.(idleCloser); ok {
		reg.own(c)
	}
	return tr
}

// newPinnedProbeClient is a group member's health-probe client when the member
// has pins: the same non-pooling, redirect-refusing client as probeHTTPClient,
// but with the member's pin check, so an https probe trusts exactly what the
// proxied traffic trusts.
func newPinnedProbeClient(t model.UpstreamTLS) *http.Client {
	return &http.Client{
		CheckRedirect: probeHTTPClient.CheckRedirect,
		Transport: &http.Transport{
			DisableKeepAlives: true,
			Proxy:             nil, // never route probes through an egress proxy
			// A custom TLS config turns HTTP/2 off unless asked for; the
			// default probe transport (no TLS config) offers it.
			ForceAttemptHTTP2: true,
			TLSClientConfig:   pinnedTLSConfig(pinDigests(t)),
		},
	}
}
