package dataplane

import (
	"bufio"
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// An upgraded connection (WebSocket or any other protocol switch) is admitted
// by the host's chain once and then carries data for as long as both peers
// keep it open. The registry below closes it when the decision that admitted
// it no longer holds:
//
//   - a reload removes or disables its host, or changes the host's chain key
//     (the digest of everything that decides who may use the host);
//   - the SSO session that admitted it expires, or the revocation watermark
//     moves past the session's issue time;
//   - the client certificate it was opened with is on the current revocation
//     list of its CA (a CRL reload, or a reload that re-reads the CRL).
//
// A reload that leaves the host's chain key unchanged leaves the tunnel open.

// maxTunnels bounds the registry. An upgrade that finds it full is refused
// before the connection is taken over (the reverse proxy answers 502). A var
// only so tests can shrink it.
var maxTunnels = 1 << 16

var errTooManyTunnels = errors.New("too many open upgraded connections")

// tunnel is one open upgraded connection and what admitted it.
type tunnel struct {
	conn     net.Conn
	live     func() *router // the owning Server's current router
	name     string         // hostKey of the request
	chainKey string
	chains   [][]*x509.Certificate // the connection's verified client chains, if any
	sso      bool                  // admitted by an SSO session
	ssoIat   int64                 // that session's issue time
	expiry   *time.Timer           // closes the tunnel when the SSO session expires
	once     sync.Once
}

func (t *tunnel) close() { t.once.Do(func() { _ = t.conn.Close() }) }

// stale reports whether the tunnel would no longer be admitted.
func (t *tunnel) stale() bool {
	rt := t.live()
	if rt == nil {
		return true
	}
	hh, ok := rt.hosts[t.name]
	if !ok || hh.chainKey != t.chainKey {
		return true
	}
	if t.sso && t.ssoIat < ssoRevokedAt() {
		return true
	}
	if len(t.chains) > 0 {
		if req := rt.clientAuth[t.name]; req != nil && req.anchor != nil && req.anchor.chainRevoked(t.chains) {
			return true
		}
		if revokedNow(t.chains) {
			return true
		}
	}
	return false
}

var tunnels struct {
	mu sync.Mutex
	m  map[*tunnel]struct{}
}

func tunnelsFull() bool {
	tunnels.mu.Lock()
	defer tunnels.mu.Unlock()
	return len(tunnels.m) >= maxTunnels
}

// registerTunnel adds t and then checks it, so a change published between the
// request's admission and the protocol switch is not missed: such a change is
// either visible to this check or its recheck sees t in the registry.
func registerTunnel(t *tunnel) {
	tunnels.mu.Lock()
	if tunnels.m == nil {
		tunnels.m = map[*tunnel]struct{}{}
	}
	tunnels.m[t] = struct{}{}
	tunnels.mu.Unlock()
	if t.stale() {
		t.close()
	}
}

func unregisterTunnel(t *tunnel) {
	tunnels.mu.Lock()
	delete(tunnels.m, t)
	tunnels.mu.Unlock()
	if t.expiry != nil {
		t.expiry.Stop()
	}
	t.close()
}

// recheckTunnels closes every open tunnel that would no longer be admitted.
// It runs after a reload is published, after a CRL reload, and after the SSO
// revocation watermark moves.
func recheckTunnels() {
	tunnels.mu.Lock()
	open := make([]*tunnel, 0, len(tunnels.m))
	for t := range tunnels.m {
		open = append(open, t)
	}
	tunnels.mu.Unlock()
	for _, t := range open {
		if t.stale() {
			t.close()
		}
	}
}

// tunnelNote is filled in by the gates while an upgrade request passes the
// chain, so the tunnel it opens knows what admitted it.
type tunnelNote struct {
	sso    bool
	ssoExp int64
	ssoIat int64
}

type tunnelNoteKey struct{}

// noteTunnelSession records the SSO session that admitted an upgrade request.
// With several SSO gates on the route the earliest expiry and issue time win.
func noteTunnelSession(r *http.Request, exp, iat int64) {
	n, _ := r.Context().Value(tunnelNoteKey{}).(*tunnelNote)
	if n == nil {
		return
	}
	if !n.sso || exp < n.ssoExp {
		n.ssoExp = exp
	}
	if !n.sso || iat < n.ssoIat {
		n.ssoIat = iat
	}
	n.sso = true
}

// serveTracked serves an upgrade request so that the connection, if the
// upstream switches protocols, is registered for the life of the tunnel.
func serveTracked(h http.Handler, w http.ResponseWriter, r *http.Request, live func() *router, name, chainKey string) {
	note := &tunnelNote{}
	tw := &tunnelWriter{ResponseWriter: w, live: live, name: name, chainKey: chainKey, note: note}
	if r.TLS != nil {
		tw.chains = r.TLS.VerifiedChains
	}
	defer func() {
		if tw.t != nil {
			unregisterTunnel(tw.t)
		}
	}()
	h.ServeHTTP(tw, r.WithContext(context.WithValue(r.Context(), tunnelNoteKey{}, note)))
}

// tunnelWriter registers the connection when the handler takes it over.
type tunnelWriter struct {
	http.ResponseWriter
	live     func() *router
	name     string
	chainKey string
	chains   [][]*x509.Certificate
	note     *tunnelNote
	t        *tunnel
}

func (w *tunnelWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if tunnelsFull() {
		return nil, nil, errTooManyTunnels
	}
	c, brw, err := http.NewResponseController(w.ResponseWriter).Hijack()
	if err != nil {
		return c, brw, err
	}
	t := &tunnel{conn: c, live: w.live, name: w.name, chainKey: w.chainKey, chains: w.chains, sso: w.note.sso, ssoIat: w.note.ssoIat}
	if t.sso {
		t.expiry = time.AfterFunc(time.Until(time.Unix(w.note.ssoExp, 0)), t.close)
	}
	w.t = t
	registerTunnel(t)
	return c, brw, nil
}

func (w *tunnelWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *tunnelWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// accessMiddlewareType reports whether a middleware of type t decides who may
// use a host. Other types (headers, rewrite) only shape the request or the
// response and stay out of the chain key.
func accessMiddlewareType(t string) bool {
	switch t {
	case model.MWTypeAuth, model.MWTypeGuard, model.MWTypeRateLimit, model.MWTypeBouncer, model.MWTypePathBlock:
		return true
	}
	return false
}

// hostChainKey digests what decides who may use a proxy host: its access
// fields and those of its locations, the access middlewares they reference,
// every access list, identity provider and client CA named in any of that, and
// the effective trusted-proxy set. Object metadata other than the name and the
// disabled flag is left out, and so is a client CA's inline CRL, which the
// revocation check covers on its own.
func hostChainKey(h model.ProxyHost, cfg model.Config, trusted []*net.IPNet) string {
	type locSpec struct {
		Path            string
		Middlewares     []string
		AccessLists     []string
		AccessListsMode string
		Auth            *model.AuthMiddleware
		RateLimit       *model.RateLimitMiddleware
	}
	meta := func(m model.ObjectMeta) model.ObjectMeta {
		return model.ObjectMeta{Name: m.Name, Disabled: m.Disabled}
	}
	// Names of middlewares that exist and do not decide access are dropped
	// from the reference lists; an unknown name stays, so it cannot hide a
	// change.
	shaping := map[string]bool{}
	for _, m := range cfg.Middlewares {
		if !accessMiddlewareType(m.Type) {
			shaping[m.Name] = true
		}
	}
	access := func(names []string) []string {
		var out []string
		for _, n := range names {
			if !shaping[n] {
				out = append(out, n)
			}
		}
		return out
	}
	spec := struct {
		ClientAuth      *model.ClientAuth
		MinTLSVersion   string
		Middlewares     []string
		AccessLists     []string
		AccessListsMode string
		Auth            *model.AuthMiddleware
		RateLimit       *model.RateLimitMiddleware
		Locations       []locSpec
		Trusted         []string
		PathBlock       *model.PathBlockMiddleware
		RefMiddlewares  []model.Middleware
		RefAccessLists  []model.AccessList
		RefIdPs         []model.IdentityProvider
		RefClientCAs    []model.ClientCA
	}{
		ClientAuth: h.TLS.ClientAuth, MinTLSVersion: h.TLS.MinTLSVersion,
		Middlewares: access(h.Middlewares), AccessLists: h.AccessLists,
		AccessListsMode: h.AccessListsMode, Auth: h.Auth, RateLimit: h.RateLimit,
	}
	used := map[string]bool{}
	for _, m := range spec.Middlewares {
		used[m] = true
	}
	for _, l := range h.Locations {
		mws := access(l.Middlewares)
		spec.Locations = append(spec.Locations, locSpec{l.Path, mws, l.AccessLists, l.AccessListsMode, l.Auth, l.RateLimit})
		for _, m := range mws {
			used[m] = true
		}
	}
	for _, n := range trusted {
		spec.Trusted = append(spec.Trusted, n.String())
	}
	// The fleet path block list, for a host it covers (see buildChainInline).
	if h.PathBlock != model.PathBlockOff && !h.AdminEdge {
		spec.PathBlock = currentPathBlock()
	}
	for _, m := range cfg.Middlewares {
		if used[m.Name] && accessMiddlewareType(m.Type) {
			m.ObjectMeta = meta(m.ObjectMeta)
			spec.RefMiddlewares = append(spec.RefMiddlewares, m)
		}
	}
	// An object counts as referenced when its name appears as a whole JSON
	// string, or as an "@name" access-list reference. A string that only
	// happens to equal a name adds the object to the key, which can close a
	// tunnel needlessly but never keeps one open.
	refs, _ := json.Marshal(spec)
	named := func(name string) bool {
		return strings.Contains(string(refs), `"`+name+`"`) || strings.Contains(string(refs), `"@`+name+`"`)
	}
	for _, al := range cfg.AccessLists {
		if named(al.Name) {
			al.ObjectMeta = meta(al.ObjectMeta)
			spec.RefAccessLists = append(spec.RefAccessLists, al)
		}
	}
	refs, _ = json.Marshal(spec)
	for _, idp := range cfg.IdentityProviders {
		if named(idp.Name) {
			idp.ObjectMeta = meta(idp.ObjectMeta)
			spec.RefIdPs = append(spec.RefIdPs, idp)
		}
	}
	for _, ca := range cfg.ClientCAs {
		if named(ca.Name) {
			ca.ObjectMeta = meta(ca.ObjectMeta)
			ca.CRLPEM = ""
			spec.RefClientCAs = append(spec.RefClientCAs, ca)
		}
	}
	key, ok := specKey(spec)
	if !ok {
		// Unhashable: a key no other build shares, so a reload closes the
		// host's tunnels rather than keeping them on an unchecked chain.
		return "unhashed-" + strconv.FormatUint(unhashedChainKeys.Add(1), 10)
	}
	return key
}

var unhashedChainKeys atomic.Uint64
