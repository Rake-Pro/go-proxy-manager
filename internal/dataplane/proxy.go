package dataplane

import (
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// newReverseProxy builds the terminal reverse-proxy handler for an upstream.
// WebSocket upgrades are carried transparently by httputil.ReverseProxy when the
// request advertises them (the per-host toggle gates whether Upgrade is offered).
func newReverseProxy(up model.Upstream, hostName string) *httputil.ReverseProxy {
	target := &url.URL{
		Scheme: up.Scheme,
		Host:   net.JoinHostPort(up.Host, strconv.Itoa(up.Port)),
	}
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.SetXForwarded()           // X-Forwarded-For / -Host / -Proto
			pr.Out.Host = pr.In.Host     // preserve the client's Host header
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Warn().Str("host", hostName).Str("path", r.URL.Path).Err(err).Msg("upstream error")
			w.WriteHeader(http.StatusBadGateway)
		},
	}
	return rp
}

// hostHandler is the compiled handler for one ProxyHost: its middleware chain
// wrapping the reverse proxy. forceSSL records whether plaintext requests should
// be redirected to HTTPS by the HTTP listener.
type hostHandler struct {
	host     string
	handler  http.Handler
	forceSSL bool
}
