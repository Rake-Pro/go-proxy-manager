package dataplane

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"net/textproto"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// headersHandler applies declarative request/response header mutations.
func headersHandler(spec model.HeadersMiddleware, next http.Handler) http.Handler {
	mutatesResp := len(spec.SetResponse) > 0 || len(spec.RemoveResponse) > 0
	var pinned []string
	for k := range spec.SetRequest {
		pinned = append(pinned, textproto.CanonicalMIMEHeaderKey(k))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, k := range spec.RemoveRequest {
			r.Header.Del(k)
		}
		for k, v := range spec.SetRequest {
			r.Header.Set(k, v)
		}
		if len(pinned) > 0 {
			r = withSetRequestHeaders(r, pinned)
		}
		if !mutatesResp {
			next.ServeHTTP(w, r)
			return
		}
		r = withResponseRules(r, w.Header(), spec)
		next.ServeHTTP(&headerRW{ResponseWriter: w, spec: spec}, r)
	})
}

// responseRulesKey carries the response rules of every headers middleware on
// the chain, so the reverse proxy can apply them to a 101 Switching Protocols
// (see applyUpgradeResponseRules): httputil.ReverseProxy writes a 101 to the
// hijacked connection and never calls WriteHeader, so headerRW does not see it.
type responseRulesKey struct{}

// responseRules is the context value under responseRulesKey. header is the
// client response's header map (every writer on the chain shares it); specs are
// the headers middlewares outermost first.
type responseRules struct {
	header http.Header
	specs  []model.HeadersMiddleware
}

func withResponseRules(r *http.Request, header http.Header, spec model.HeadersMiddleware) *http.Request {
	rules := &responseRules{header: header, specs: []model.HeadersMiddleware{spec}}
	if prev, ok := r.Context().Value(responseRulesKey{}).(*responseRules); ok {
		rules.specs = append(append(rules.specs[:0:0], prev.specs...), spec)
	}
	return r.WithContext(context.WithValue(r.Context(), responseRulesKey{}, rules))
}

// upgradeHandshakeHeaders are never set or removed by a headers middleware on
// a 101: they carry the protocol switch itself, and a rule written for ordinary
// responses must not break the handshake.
var upgradeHandshakeHeaders = map[string]struct{}{
	"Connection": {}, "Upgrade": {}, "Sec-Websocket-Accept": {},
	"Sec-Websocket-Protocol": {}, "Sec-Websocket-Extensions": {},
}

// applyUpgradeResponseRules applies the headers middlewares' response rules to
// a 101 from the upstream, called from ModifyResponse. The stdlib then merges
// resp.Header onto the client header map (handleUpgradeResponse), so each rule
// acts on both maps: the result is the same as headerRW running the rules on
// the merged map, innermost middleware first, which is the order an ordinary
// response gets.
func applyUpgradeResponseRules(resp *http.Response) {
	if resp.StatusCode != http.StatusSwitchingProtocols || resp.Request == nil {
		return
	}
	rules, ok := resp.Request.Context().Value(responseRulesKey{}).(*responseRules)
	if !ok {
		return
	}
	for i := len(rules.specs) - 1; i >= 0; i-- {
		spec := rules.specs[i]
		for _, k := range spec.RemoveResponse {
			if _, hs := upgradeHandshakeHeaders[textproto.CanonicalMIMEHeaderKey(k)]; hs {
				continue
			}
			rules.header.Del(k)
			resp.Header.Del(k)
		}
		for k, v := range spec.SetResponse {
			if _, hs := upgradeHandshakeHeaders[textproto.CanonicalMIMEHeaderKey(k)]; hs {
				continue
			}
			rules.header.Del(k)
			resp.Header.Set(k, v)
		}
	}
}

// setRequestKey carries the names a headers middleware set on the request, so
// the reverse proxy can restore them after its hop-by-hop purge (see
// reapplySetRequest).
type setRequestKey struct{}

func withSetRequestHeaders(r *http.Request, names []string) *http.Request {
	if prev, ok := r.Context().Value(setRequestKey{}).([]string); ok {
		names = append(append([]string(nil), prev...), names...)
	}
	return r.WithContext(context.WithValue(r.Context(), setRequestKey{}, names))
}

// proxyOwnedRequestHeaders are removed from every outbound request by the
// reverse proxy itself (the fixed hop-by-hop set and X-Forwarded-For/-Host/
// -Proto, which the Rewrite hook writes); a setRequest value under one of them
// is not restored, so what the upstream sees for these names is unchanged.
var proxyOwnedRequestHeaders = map[string]struct{}{
	"Connection": {}, "Proxy-Connection": {}, "Keep-Alive": {}, "Proxy-Authenticate": {},
	"Proxy-Authorization": {}, "Te": {}, "Trailer": {}, "Transfer-Encoding": {}, "Upgrade": {},
	"X-Forwarded-For": {}, "X-Forwarded-Host": {}, "X-Forwarded-Proto": {},
}

// reapplySetRequest copies the headers a headers middleware set (setRequest)
// from the inbound request to the outbound one. httputil.ReverseProxy deletes
// every header the client names in Connection before Rewrite runs, which would
// otherwise let a client remove an operator-set request header. The inbound
// value is the one in effect after the whole chain, so a header a later
// middleware removed stays removed.
func reapplySetRequest(pr *httputil.ProxyRequest) {
	names, _ := pr.In.Context().Value(setRequestKey{}).([]string)
	for _, name := range names {
		if _, owned := proxyOwnedRequestHeaders[name]; owned {
			continue
		}
		if v, ok := pr.In.Header[name]; ok && len(v) > 0 {
			pr.Out.Header[name] = append([]string(nil), v...)
		}
	}
}

// headerRW applies response header mutations just before the status is written.
// It forwards Hijacker/Flusher so WebSocket upgrades and streaming still work.
type headerRW struct {
	http.ResponseWriter
	spec    model.HeadersMiddleware
	written bool
}

func (h *headerRW) WriteHeader(status int) {
	// The rules run on every header write until a final status: an interim
	// 1xx (other than 101, which ends the response) does not latch, because
	// the reverse proxy clears the header map after writing it, so the rules
	// must run again for the final response. A 101 written by the reverse proxy
	// never reaches here (see applyUpgradeResponseRules).
	if !h.written {
		for _, k := range h.spec.RemoveResponse {
			h.ResponseWriter.Header().Del(k)
		}
		for k, v := range h.spec.SetResponse {
			h.ResponseWriter.Header().Set(k, v)
		}
		h.written = status >= 200 || status == http.StatusSwitchingProtocols
	}
	h.ResponseWriter.WriteHeader(status)
}

func (h *headerRW) Write(b []byte) (int, error) {
	if !h.written {
		h.WriteHeader(http.StatusOK)
	}
	return h.ResponseWriter.Write(b)
}

func (h *headerRW) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := h.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

func (h *headerRW) Flush() {
	if f, ok := h.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the underlying writer's deadlines.
func (h *headerRW) Unwrap() http.ResponseWriter { return h.ResponseWriter }
