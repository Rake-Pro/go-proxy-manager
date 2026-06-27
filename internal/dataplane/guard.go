package dataplane

import (
	"net"
	"net/http"
	"strings"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// guard is a compiled GuardMiddleware: it denies a request that matches any
// trigger unless the client is in one of the allow networks.
type guard struct {
	triggers   []guardTrigger
	allowNets  []*net.IPNet
	denyStatus int
}

type guardTrigger struct {
	paths       map[string]struct{} // nil = any path
	methods     map[string]struct{} // nil = any method
	queryEquals map[string]string
}

func compileGuard(g model.GuardMiddleware) guard {
	c := guard{denyStatus: g.DenyStatus}
	if c.denyStatus == 0 {
		c.denyStatus = http.StatusForbidden
	}
	for _, cidr := range g.AllowFrom {
		if n := parseNet(cidr); n != nil {
			c.allowNets = append(c.allowNets, n)
		}
	}
	for _, t := range g.Triggers {
		gt := guardTrigger{queryEquals: t.QueryEquals}
		if len(t.Paths) > 0 {
			gt.paths = map[string]struct{}{}
			for _, p := range t.Paths {
				gt.paths[p] = struct{}{}
			}
		}
		if len(t.Methods) > 0 {
			gt.methods = map[string]struct{}{}
			for _, m := range t.Methods {
				gt.methods[strings.ToUpper(m)] = struct{}{}
			}
		}
		c.triggers = append(c.triggers, gt)
	}
	return c
}

func (t guardTrigger) matches(r *http.Request) bool {
	if t.paths != nil {
		if _, ok := t.paths[r.URL.Path]; !ok {
			return false
		}
	}
	if t.methods != nil {
		if _, ok := t.methods[r.Method]; !ok {
			return false
		}
	}
	for k, v := range t.queryEquals {
		if r.URL.Query().Get(k) != v {
			return false
		}
	}
	return true
}

// guardHandler denies a matching request with denyStatus unless ipOf(r) is in
// the allow networks. It fails closed: a matched request with an unresolvable
// client IP is denied (ipInNets(nil, ...) is false).
func guardHandler(c guard, ipOf func(*http.Request) net.IP, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		matched := false
		for _, t := range c.triggers {
			if t.matches(r) {
				matched = true
				break
			}
		}
		if matched && !ipInNets(ipOf(r), c.allowNets) {
			http.Error(w, http.StatusText(c.denyStatus), c.denyStatus)
			return
		}
		next.ServeHTTP(w, r)
	})
}
