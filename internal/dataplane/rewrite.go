package dataplane

import (
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// compiledRewrite is a rewrite middleware prepared once at router build time:
// the exact map as-is, the prefix rules ordered longest-first, and the regex
// rules already compiled. Nothing is parsed or compiled per request.
type compiledRewrite struct {
	exact  map[string]string
	prefix []model.RewriteRule
	regex  []compiledRegexRule
}

type compiledRegexRule struct {
	re *regexp.Regexp
	to string
}

// compileRewrite prepares a rewrite spec for the data plane. A regex that fails
// to compile is DROPPED with a loud log rather than taken as "matches nothing"
// or aborting the whole router: Middleware.Validate already rejects it at config
// load, so reaching here means the config bypassed validation, and one dead rule
// must not take the host down.
func compileRewrite(spec model.RewriteMiddleware) compiledRewrite {
	c := compiledRewrite{exact: spec.ReplacePath}
	if len(spec.PrefixRules) > 0 {
		c.prefix = append([]model.RewriteRule(nil), spec.PrefixRules...)
		// Longest prefix wins, so the most specific rule is found first by the
		// linear scan in rewritePath.
		sort.SliceStable(c.prefix, func(i, j int) bool { return len(c.prefix[i].From) > len(c.prefix[j].From) })
	}
	for i, r := range spec.RegexRules {
		re, err := regexp.Compile(model.AnchorRewritePattern(r.From))
		if err != nil {
			log.Error().Int("rule", i).Str("pattern", r.From).Err(err).
				Msg("dataplane: rewrite regex rule does not compile; skipping it")
			continue
		}
		c.regex = append(c.regex, compiledRegexRule{re: re, to: r.To})
	}
	return c
}

// rewritePath returns the rewritten path and whether any rule matched. Rule
// kinds are tried in a fixed order - exact, prefix (longest first), regex (in
// order) - and the FIRST match wins: a rule never sees another rule's output, so
// rules cannot chain into a path no operator wrote.
func (c compiledRewrite) rewritePath(p string) (string, bool) {
	if to, ok := c.exact[p]; ok {
		return to, true
	}
	for _, r := range c.prefix {
		if to, ok := applyPrefixRule(p, r.From, r.To); ok {
			return to, true
		}
	}
	for _, r := range c.regex {
		if loc := r.re.FindStringSubmatchIndex(p); loc != nil {
			out := r.re.ExpandString(nil, r.to, p, loc)
			return string(out) + p[loc[1]:], true
		}
	}
	return p, false
}

// safeRewritten re-cleans a rewritten path and reports whether it is safe to
// forward. Model validation already rejects a dot segment, a backslash and a
// ';' in every rewrite target, but a regex replacement composes at REQUEST time
// from client-supplied capture text, so the composed result is checked here the
// same way normalizeRequestPath checks the inbound path: clean it, and refuse
// the request outright if it carries a separator an upstream might re-interpret.
// A rewrite is applied inside every security tier, so a path that got past this
// point unchecked would be one no gate ever saw.
func safeRewritten(p string) (string, bool) {
	if strings.ContainsAny(p, `\;`) {
		return "", false
	}
	return cleanPath(p), true
}

// applyPrefixRule replaces the "from" prefix of p with "to", matching on a
// segment boundary exactly the way a location prefix does (the path equals
// "from", or continues with "/"), so "/reports" can never capture
// "/reports-evil". The remainder after the prefix is appended unchanged.
func applyPrefixRule(p, from, to string) (string, bool) {
	from = strings.TrimSuffix(from, "/")
	to = strings.TrimSuffix(to, "/")
	if from == "" { // a "/" rule matches every path
		if to == "" {
			return p, false // "/" -> "/" is a no-op
		}
		return to + p, true
	}
	switch {
	case p == from:
		if to == "" {
			return "/", true
		}
		return to, true
	case strings.HasPrefix(p, from+"/"):
		return to + p[len(from):], true
	}
	return "", false
}

// rewriteHandler rewrites the request path before proxying. It matches on the
// decoded r.URL.Path (already cleaned by normalizeRequestPath) and, on a hit,
// swaps in the target path. RawPath is cleared so Go re-derives the escaped form
// from the new Path; our configured paths are ASCII, so the plain and escaped
// forms coincide. RawQuery is untouched, so the query string is forwarded
// exactly as the client sent it.
func rewriteHandler(spec model.RewriteMiddleware, next http.Handler) http.Handler {
	c := compileRewrite(spec)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if to, ok := c.rewritePath(r.URL.Path); ok {
			clean, safe := safeRewritten(to)
			if !safe {
				log.Warn().Str("path", r.URL.Path).Msg("dataplane: rewritten path carries a path separator an upstream could re-interpret; refusing the request")
				http.Error(w, "Bad Request", http.StatusBadRequest)
				return
			}
			r.URL.Path = clean
			r.URL.RawPath = ""
		}
		next.ServeHTTP(w, r)
	})
}

// stripPrefixHandler removes a location's matched prefix from the request path
// before the request continues inward: with prefix "/app", "/app/foo" becomes
// "/foo" and "/app" itself becomes "/". prefix is the already-normalized
// location prefix (cleaned, no trailing slash - see normalizeLocationPrefix), and
// matching repeats the same boundary test route() used rather than cutting the
// path at a fixed offset, so a path that does not actually carry the prefix is
// left alone. RawQuery is untouched.
//
// A root location ("/") strips nothing: removing "/" would leave an empty path.
func stripPrefixHandler(prefix string, next http.Handler) http.Handler {
	if prefix == "" || prefix == "/" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch p := r.URL.Path; {
		case p == prefix:
			r.URL.Path = "/"
			r.URL.RawPath = ""
		case strings.HasPrefix(p, prefix+"/"):
			r.URL.Path = p[len(prefix):]
			r.URL.RawPath = ""
		}
		next.ServeHTTP(w, r)
	})
}
