package dataplane

import (
	"bufio"
	"net"
	"net/http"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// headersHandler applies declarative request/response header mutations.
func headersHandler(spec model.HeadersMiddleware, next http.Handler) http.Handler {
	mutatesResp := len(spec.SetResponse) > 0 || len(spec.RemoveResponse) > 0
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, k := range spec.RemoveRequest {
			r.Header.Del(k)
		}
		for k, v := range spec.SetRequest {
			r.Header.Set(k, v)
		}
		if !mutatesResp {
			next.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(&headerRW{ResponseWriter: w, spec: spec}, r)
	})
}

// headerRW applies response header mutations just before the status is written.
// It forwards Hijacker/Flusher so WebSocket upgrades and streaming still work.
type headerRW struct {
	http.ResponseWriter
	spec    model.HeadersMiddleware
	written bool
}

func (h *headerRW) WriteHeader(status int) {
	if !h.written {
		for _, k := range h.spec.RemoveResponse {
			h.ResponseWriter.Header().Del(k)
		}
		for k, v := range h.spec.SetResponse {
			h.ResponseWriter.Header().Set(k, v)
		}
		h.written = true
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
