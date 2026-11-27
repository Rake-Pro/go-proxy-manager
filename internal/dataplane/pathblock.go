package dataplane

import (
	"net"
	"net/http"
	"path"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// globalPathBlock is settings.pathBlock, the fleet path block list buildRouter
// wraps around every proxy host that has not opted out. nil blocks nothing.
var globalPathBlock atomic.Pointer[model.PathBlockMiddleware]

// SetPathBlock installs settings.pathBlock. It takes effect at the next router
// build, like the other settings-level defaults a chain is compiled from.
func SetPathBlock(p *model.PathBlockMiddleware) {
	if p == nil {
		globalPathBlock.Store(nil)
		return
	}
	c := *p
	globalPathBlock.Store(&c)
}

func currentPathBlock() *model.PathBlockMiddleware { return globalPathBlock.Load() }

// pathBlock is a compiled PathBlockMiddleware. Every entry is stored
// lower-cased and a request path is lower-cased before it is compared, so the
// whole match is case-insensitive.
type pathBlock struct {
	exts  map[string]struct{}
	names map[string]struct{}
	globs []string
	// extLens holds the distinct byte lengths of exts, so a segment costs one
	// set lookup per length rather than one per dot it contains.
	extLens []int

	allowNets  []*net.IPNet
	allowLists []func(net.IP) bool

	denyStatus int
	plain      bool
}

// compilePathBlock compiles one spec. listExempt resolves an "@<list>"
// allowFrom entry to that list's first-match exemption (see
// registry.rateLimitExemption); a list it cannot resolve exempts nobody.
func compilePathBlock(spec model.PathBlockMiddleware, listExempt func(name string) func(net.IP) bool) *pathBlock {
	p := &pathBlock{
		exts:       map[string]struct{}{},
		names:      map[string]struct{}{},
		denyStatus: spec.DenyStatusOrDefault(),
		plain:      spec.DenyWith == model.BouncerDenyWithPlain,
	}
	lens := map[int]struct{}{}
	for _, e := range spec.Extensions {
		e = strings.ToLower(e)
		// Validation requires the dot; without one an entry would match as a
		// bare suffix ("php" inside "morphp"), so it is dropped here too.
		if len(e) < 2 || e[0] != '.' {
			continue
		}
		p.exts[e] = struct{}{}
		lens[len(e)] = struct{}{}
	}
	for n := range lens {
		p.extLens = append(p.extLens, n)
	}
	sort.Ints(p.extLens)
	for _, n := range spec.Names {
		if n != "" {
			p.names[strings.ToLower(n)] = struct{}{}
		}
	}
	for _, g := range spec.Globs {
		g = strings.ToLower(g)
		if _, err := path.Match(g, "/"); err == nil && strings.HasPrefix(g, "/") {
			p.globs = append(p.globs, g)
		}
	}
	for _, c := range spec.AllowFrom {
		if name, ok := model.AccessListRef(c); ok {
			if listExempt != nil {
				if f := listExempt(name); f != nil {
					p.allowLists = append(p.allowLists, f)
				}
			}
			continue
		}
		if n := parseNet(c); n != nil {
			p.allowNets = append(p.allowNets, n)
		}
	}
	return p
}

// blockSegment returns the form of one path segment the lists are compared
// with: up to its first NUL byte and without trailing dots and spaces. A
// backend that stops reading a name at NUL, or a filesystem that ignores
// trailing dots and spaces, serves "x.php%00.txt" and "x.php." as x.php.
func blockSegment(seg string) string {
	if i := strings.IndexByte(seg, 0); i >= 0 {
		seg = seg[:i]
	}
	return strings.TrimRight(seg, ". ")
}

// maxGlobPathLen bounds the path a list with globs is tried against, measured
// on the lower-cased path (which is what is matched; lower-casing can lengthen
// a path that is not valid UTF-8). A glob
// costs time in proportion to the path times the pattern, and the listeners
// accept request targets far longer than any path a file system can hold
// (4096 bytes), so a longer one counts as listed instead of being walked once
// per pattern. Together with model.MaxPathBlockGlobBytes this bounds the work
// one request can cause.
const maxGlobPathLen = 4096

// foldPath returns the second form of a lower-cased path the globs are
// compared with: cut at the first NUL byte, every segment as blockSegment
// leaves it, segments that fold to nothing dropped, and no trailing slash, so
// a glob sees "/server-status/" and "/backup/site.zip." the way a lenient
// upstream serves them. The common path needs no folding and is returned as
// is.
func foldPath(lower string) string {
	if i := strings.IndexByte(lower, 0); i >= 0 {
		// A backend that stops reading at NUL stops reading the whole path.
		lower = lower[:i]
	}
	fold := len(lower) > 1 && lower[len(lower)-1] == '/'
	for i := 0; !fold && i < len(lower); i++ {
		if c := lower[i]; c == '.' || c == ' ' {
			fold = i+1 == len(lower) || lower[i+1] == '/'
		}
	}
	if !fold {
		return lower
	}
	var b strings.Builder
	b.Grow(len(lower))
	rest := strings.TrimRight(lower, "/")
	for rest != "" {
		seg := rest
		if j := strings.IndexByte(rest, '/'); j >= 0 {
			seg, rest = rest[:j], rest[j+1:]
		} else {
			rest = ""
		}
		// A segment of only dots and spaces folds to nothing and goes with its
		// slash (the first segment, before the leading slash, is always empty).
		if seg = blockSegment(seg); seg != "" {
			b.WriteByte('/')
			b.WriteString(seg)
		}
	}
	if b.Len() == 0 {
		return "/"
	}
	return b.String()
}

// matches reports whether the request path is on the list. urlPath is the
// decoded path the router cleaned (see normalizeRequestPath): an encoded "/"
// or dot segment has already taken its final form, and the steps inside the
// list (strip prefix, rewrite, the upstream base path) start from this path
// without decoding it again.
func (p *pathBlock) matches(urlPath string) bool {
	lower := strings.ToLower(urlPath)
	if len(p.globs) > 0 {
		if len(lower) > maxGlobPathLen {
			return true
		}
		// A glob is tried on the path as it is and on its folded form, so the
		// fold only ever adds matches: "/secret/*" still covers "/secret/",
		// and "/*/secret" still covers a first segment that holds a NUL.
		folded := foldPath(lower)
		for _, g := range p.globs {
			if ok, _ := path.Match(g, lower); ok {
				return true
			}
			if folded != lower {
				if ok, _ := path.Match(g, folded); ok {
					return true
				}
			}
		}
	}
	if len(p.exts) == 0 && len(p.names) == 0 {
		return false
	}
	for len(lower) > 0 {
		seg := lower
		if i := strings.IndexByte(lower, '/'); i >= 0 {
			seg, lower = lower[:i], lower[i+1:]
		} else {
			lower = ""
		}
		if seg = blockSegment(seg); seg == "" {
			continue
		}
		if _, ok := p.names[seg]; ok {
			return true
		}
		for _, n := range p.extLens {
			if n > len(seg) {
				break
			}
			if _, ok := p.exts[seg[len(seg)-n:]]; ok {
				return true
			}
		}
	}
	return false
}

// exempt reports whether the client is in allowFrom. A client address that
// cannot be resolved is never exempt.
func (p *pathBlock) exempt(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ipInNets(ip, p.allowNets) {
		return true
	}
	for _, f := range p.allowLists {
		if f(ip) {
			return true
		}
	}
	return false
}

// pathBlockHandler refuses a request whose path is on the list, unless the
// client is exempt. The path is checked first, so a request that is not on the
// list costs no client-address work. host and ep resolve the custom error page
// for the denial (see serveErrorPage).
func pathBlockHandler(p *pathBlock, ipOf func(*http.Request) net.IP, host string, ep *compiledErrorPages, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !p.matches(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		var ip net.IP
		if ipOf != nil {
			ip = ipOf(r)
		}
		if p.exempt(ip) {
			next.ServeHTTP(w, r)
			return
		}
		countDenial(r, "path-block")
		plain := func() { http.Error(w, http.StatusText(p.denyStatus), p.denyStatus) }
		if p.plain {
			plain()
			return
		}
		serveErrorPage(w, p.denyStatus, ep, host, plain)
	})
}
