package dataplane

import (
	"fmt"
	"math/rand/v2"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// installRawSources installs fetched source sets as given, past the fetcher's
// entry guard, so the tests can use documentation ranges.
func installRawSources(t *testing.T, sets map[string][]string) {
	t.Helper()
	prev := globalAccessListSources.Load()
	t.Cleanup(func() { globalAccessListSources.Store(prev) })
	m := map[string][]*net.IPNet{}
	for k, entries := range sets {
		for _, e := range entries {
			m[k] = append(m[k], parseNet(e))
		}
	}
	globalAccessListSources.Store(&m)
}

// rateLimitHandler is rateLimitHandlerWith with a fresh limiter.
func rateLimitHandler(rl model.RateLimitMiddleware, ipOf func(*http.Request) net.IP, host string, ep *compiledErrorPages, next http.Handler, listExempt func(name string) func(net.IP) bool) http.Handler {
	return rateLimitHandlerWith(newRateLimiter(rl), rl, ipOf, host, ep, next, listExempt)
}

// legacyAccessListAllowNets is what an "@<list>" exemption admitted before
// first-match evaluation: the networks of every allow rule without a path
// condition, in any order. The exemption must stay inside it.
func legacyAccessListAllowNets(al model.AccessList) []*net.IPNet {
	sources := currentAccessListSources()
	var nets []*net.IPNet
	for _, r := range al.Rules {
		if r.Action != model.ActionAllow || r.PathScoped() {
			continue
		}
		if r.Source != "" {
			nets = append(nets, sourceNets(sources, al, r.Source)...)
			continue
		}
		if n := parseNet(r.CIDR); n != nil {
			nets = append(nets, n)
		}
	}
	return nets
}

// exemptListOf returns the compiled "@<list>" exemption for al, built the way
// a router build resolves it.
func exemptListOf(t *testing.T, al model.AccessList) *rateLimitExemptList {
	t.Helper()
	reg := buildRegistry(model.Config{AccessLists: []model.AccessList{al}})
	reg.rateLimitExemption(al.Name)
	e := reg.rateLimitExempt[al.Name]
	if e == nil {
		t.Fatalf("no exemption for list %q", al.Name)
	}
	return e
}

// exemptionOf returns the production "@<list>" exemption predicate for al.
func exemptionOf(t *testing.T, al model.AccessList) func(net.IP) bool {
	t.Helper()
	f := buildRegistry(model.Config{AccessLists: []model.AccessList{al}}).rateLimitExemption(al.Name)
	if f == nil {
		t.Fatalf("no exemption for list %q", al.Name)
	}
	return f
}

// referenceFirstMatch is first-match evaluation written straight from the
// model, independent of compileAccessList: each rule parsed with parseNet (or
// its fetched source set), path-scoped rules skipped, the first rule
// containing ip deciding.
func referenceFirstMatch(al model.AccessList, ip net.IP) bool {
	sources := currentAccessListSources()
	for _, r := range al.Rules {
		if r.PathScoped() {
			continue
		}
		var nets []*net.IPNet
		if r.Source != "" {
			nets = sourceNets(sources, al, r.Source)
		} else if n := parseNet(r.CIDR); n != nil {
			nets = []*net.IPNet{n}
		}
		if ipInNets(ip, nets) {
			return r.Action == model.ActionAllow
		}
	}
	return false
}

// gateFirstMatch is the access-list gate's own verdict for ip on a raw stream
// (no request path) with the default action forced to deny and the geo rules
// removed: allow exactly when the first matching rule allows.
func gateFirstMatch(al model.AccessList, ip net.IP) bool {
	al.DefaultAction = model.ActionDeny
	al.Geo = nil
	return compileAccessList(al).ipAllowed(ip, nil, nil)
}

// checkExempt asserts the exemption for ip against want, against the gate's
// first-match verdict, and that it stays inside the exemption before the
// change (legacyAccessListAllowNets).
func checkExempt(t *testing.T, al model.AccessList, exempt func(net.IP) bool, legacy []*net.IPNet, s string, want bool) {
	t.Helper()
	ip := net.ParseIP(s)
	got := exempt(ip)
	if got != want {
		t.Fatalf("%s: exempt = %v, want %v", s, got, want)
	}
	if got != gateFirstMatch(al, ip) {
		t.Fatalf("%s: exempt = %v disagrees with the gate's first match", s, got)
	}
	if got != referenceFirstMatch(al, ip) {
		t.Fatalf("%s: exempt = %v disagrees with the reference first match", s, got)
	}
	if got && !ipInNets(ip, legacy) {
		t.Fatalf("%s: exempt now but was not before the change", s)
	}
}

func TestRateLimitExemptionFirstMatch(t *testing.T) {
	installRawSources(t, map[string][]string{
		model.AccessListSourceKey("feeds", "blocked"): {"203.0.113.0/25", "2001:db8:bad::/48"},
		model.AccessListSourceKey("feeds", "probes"):  {"198.51.100.0/24"},
	})
	allow := func(cidr string) model.IPRule { return model.IPRule{Action: model.ActionAllow, CIDR: cidr} }
	deny := func(cidr string) model.IPRule { return model.IPRule{Action: model.ActionDeny, CIDR: cidr} }
	list := func(rules ...model.IPRule) model.AccessList {
		return model.AccessList{ObjectMeta: model.ObjectMeta{Name: "feeds"}, DefaultAction: model.ActionDeny, Rules: rules,
			Sources: []model.AccessListSource{
				{Name: "blocked", URL: "https://example.com/i.txt"},
				{Name: "probes", URL: "https://example.com/i.txt"},
				{Name: "never", URL: "https://example.com/i.txt"},
			}}
	}
	for _, tc := range []struct {
		name   string
		al     model.AccessList
		exempt []string
		not    []string
	}{
		{"deny before allow keeps its networks limited", list(deny("10.9.0.0/16"), allow("10.0.0.0/8")),
			[]string{"10.1.2.3", "10.8.255.255", "10.10.0.0", "10.255.255.255"}, []string{"10.9.0.0", "10.9.200.1", "192.0.2.1"}},
		{"deny after allow changes nothing", list(allow("10.0.0.0/8"), deny("10.9.0.0/16")),
			[]string{"10.1.2.3", "10.9.200.1"}, []string{"192.0.2.1"}},
		{"deny covering the allow", list(deny("10.0.0.0/8"), allow("10.1.0.0/16")),
			nil, []string{"10.1.2.3"}},
		{"bare IP deny", list(deny("10.1.2.3"), allow("10.0.0.0/8")),
			[]string{"10.1.2.2", "10.1.2.4"}, []string{"10.1.2.3"}},
		{"IPv4-mapped deny matches IPv4 clients", list(deny("::ffff:10.9.0.0/112"), allow("10.0.0.0/8")),
			[]string{"10.1.2.3", "::ffff:10.1.2.3"}, []string{"10.9.1.1", "::ffff:10.9.1.1"}},
		{"other family deny", list(deny("::/0"), allow("10.0.0.0/8"), allow("2001:db8::/32")),
			[]string{"10.1.2.3"}, []string{"2001:db8::1"}},
		{"IPv6 deny before allow", list(deny("2001:db8:1::/48"), allow("2001:db8::/32")),
			[]string{"2001:db8:2::5", "2001:db8::1"}, []string{"2001:db8:1::5", "2001:db8:1:ffff::1"}},
		{"deny source before allow", list(model.IPRule{Action: model.ActionDeny, Source: "blocked"}, allow("203.0.113.0/24"), allow("2001:db8::/32")),
			[]string{"203.0.113.128", "203.0.113.255", "2001:db8:1::1"}, []string{"203.0.113.5", "2001:db8:bad::1"}},
		{"allow source after a deny", list(deny("198.51.100.0/26"), model.IPRule{Action: model.ActionAllow, Source: "probes"}),
			[]string{"198.51.100.64", "198.51.100.200"}, []string{"198.51.100.1"}},
		{"never-fetched deny source matches nothing", list(model.IPRule{Action: model.ActionDeny, Source: "never"}, allow("10.0.0.0/8")),
			[]string{"10.1.2.3"}, nil},
		{"path-scoped allow never exempts", list(model.IPRule{Action: model.ActionAllow, CIDR: "0.0.0.0/0", Paths: []string{"/public"}}),
			nil, []string{"192.0.2.1"}},
		{"path-scoped allow is skipped, a later deny decides", list(model.IPRule{Action: model.ActionAllow, CIDR: "10.9.0.0/16", Paths: []string{"/public"}}, deny("10.9.0.0/16"), allow("10.0.0.0/8")),
			[]string{"10.1.2.3"}, []string{"10.9.1.1"}},
		{"default allow exempts nobody", model.AccessList{ObjectMeta: model.ObjectMeta{Name: "open"}, DefaultAction: model.ActionAllow, Rules: []model.IPRule{deny("192.0.2.0/24")}},
			nil, []string{"198.51.100.1", "192.0.2.1"}},
		{"geo rules neither exempt nor limit", model.AccessList{ObjectMeta: model.ObjectMeta{Name: "geo"}, DefaultAction: model.ActionDeny,
			Rules: []model.IPRule{allow("10.0.0.0/8")}, Geo: &model.AccessListGeo{CountryAllow: []string{"US"}}},
			[]string{"10.1.2.3"}, []string{"192.0.2.1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exempt := exemptionOf(t, tc.al)
			legacy := legacyAccessListAllowNets(tc.al)
			for _, s := range tc.exempt {
				checkExempt(t, tc.al, exempt, legacy, s, true)
			}
			for _, s := range tc.not {
				checkExempt(t, tc.al, exempt, legacy, s, false)
			}
		})
	}
}

// Randomized differential: random lists of nested and overlapping IPv4, IPv6
// and IPv4-mapped prefixes, bare IPs, path-scoped allows and source rules. For
// every sampled client the pre-checked exemption equals the plain first-match
// walk and the gate's own verdict, and never exempts anyone the exemption
// before the change did not.
func TestRateLimitExemptionRandomDifferential(t *testing.T) {
	rng := rand.New(rand.NewPCG(172, 5))
	bases := []netip.Prefix{
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("198.51.100.0/28"),
		netip.MustParsePrefix("2001:db8::/120"),
	}
	// lowV6 is sampled as clients only: ::/32 is where a bare IPv4-mapped
	// literal misread as an IPv6 /32 would land.
	lowV6 := netip.MustParsePrefix("::/120")
	ancestors := []string{"0.0.0.0/0", "192.0.0.0/8", "::/0", "2001:db8::/32", "::ffff:0.0.0.0/96", "::ffff:192.0.2.0/120"}
	randAddr := func(base netip.Prefix) netip.Addr {
		b := base.Addr().As16()
		start := 0
		if base.Addr().Is4() {
			start = 96
		}
		for bit := start + base.Bits(); bit < 128; bit++ {
			if rng.IntN(2) == 1 {
				b[bit/8] |= 0x80 >> (bit % 8)
			}
		}
		a := netip.AddrFrom16(b)
		if base.Addr().Is4() {
			a = a.Unmap()
		}
		return a
	}
	// Prefixes are drawn inside small bases, plus a few fixed ancestors, so
	// random rules nest and overlap often.
	randPrefix := func() string {
		if rng.IntN(8) == 0 {
			return ancestors[rng.IntN(len(ancestors))]
		}
		base := bases[rng.IntN(len(bases))]
		max := base.Addr().BitLen()
		bits := base.Bits() + rng.IntN(max-base.Bits()+1)
		p := netip.PrefixFrom(randAddr(base), bits).Masked()
		switch {
		case bits == max && p.Addr().Is4() && rng.IntN(3) == 0:
			return "::ffff:" + p.Addr().String() // bare IPv4-mapped IP
		case bits == max:
			return p.Addr().String() // bare IP
		case p.Addr().Is4() && rng.IntN(4) == 0:
			return netip.PrefixFrom(netip.AddrFrom16(p.Addr().As16()), 96+bits).String() // IPv4-mapped
		}
		return p.String()
	}
	for round := 0; round < 300; round++ {
		name := fmt.Sprintf("r%d", round)
		sets := map[string][]string{}
		al := model.AccessList{ObjectMeta: model.ObjectMeta{Name: name}, DefaultAction: model.ActionDeny}
		if rng.IntN(2) == 0 {
			al.DefaultAction = model.ActionAllow
		}
		for i, n := 0, 1+rng.IntN(8); i < n; i++ {
			r := model.IPRule{Action: model.ActionDeny}
			if rng.IntN(2) == 0 {
				r.Action = model.ActionAllow
			}
			if rng.IntN(4) == 0 {
				src := fmt.Sprintf("s%d", i)
				al.Sources = append(al.Sources, model.AccessListSource{Name: src, URL: "https://example.com/i.txt"})
				for j, m := 0, rng.IntN(5); j < m; j++ {
					sets[model.AccessListSourceKey(name, src)] = append(sets[model.AccessListSourceKey(name, src)], randPrefix())
				}
				r.Source = src
			} else {
				r.CIDR = randPrefix()
			}
			if r.Action == model.ActionAllow && rng.IntN(5) == 0 {
				r.Paths = []string{"/p"}
			}
			al.Rules = append(al.Rules, r)
		}
		installRawSources(t, sets)
		exempt := exemptionOf(t, al)
		compiled := exemptListOf(t, al)
		legacy := legacyAccessListAllowNets(al)
		for k := 0; k < 64; k++ {
			a := randAddr(append(bases, lowV6)[rng.IntN(len(bases)+1)])
			ip := net.IP(a.AsSlice())
			if a.Is4() && rng.IntN(3) == 0 {
				ip = net.IP(netip.AddrFrom16(a.As16()).AsSlice()) // IPv4-mapped client form
			}
			got := exempt(ip)
			if walked, _ := compiled.walk(ip); got != walked {
				t.Fatalf("round %d %v %s: pre-checked exempt = %v, plain walk = %v", round, al.Rules, ip, got, walked)
			}
			if got != gateFirstMatch(al, ip) {
				t.Fatalf("round %d %v %s: exempt = %v disagrees with the gate's first match", round, al.Rules, ip, got)
			}
			if got != referenceFirstMatch(al, ip) {
				t.Fatalf("round %d %v %s: exempt = %v disagrees with the reference first match", round, al.Rules, ip, got)
			}
			if got && !ipInNets(ip, legacy) {
				t.Fatalf("round %d %v %s: exempt now but was not before the change", round, al.Rules, ip)
			}
		}
	}
}

// Scattered 10k-entry deny feeds above allow rules compile to the feeds' own
// networks plus the allow rules, split by family without copying a
// single-family feed, once per build for every route and reference. A client
// walks only its own family: outside the allow networks it costs one
// comparison per allow network of its family, inside them no more than its
// family's networks, and a feed of the other family costs it nothing.
func TestRateLimitExemptionLargeDenyFeedIsProportional(t *testing.T) {
	const n = 10000
	var v4, v6 []string
	for i := 0; i < n; i++ {
		v4 = append(v4, fmt.Sprintf("10.%d.%d.%d", i%7*31, i%251, i%241))
		v6 = append(v6, fmt.Sprintf("2001:db8:%x:%x::%x", i%509, i%499, i))
	}
	installRawSources(t, map[string][]string{
		model.AccessListSourceKey("big", "v4"): v4,
		model.AccessListSourceKey("big", "v6"): v6,
	})
	feedList := func(rules ...model.IPRule) model.AccessList {
		return model.AccessList{ObjectMeta: model.ObjectMeta{Name: "big"}, DefaultAction: model.ActionDeny,
			Sources: []model.AccessListSource{{Name: "v4", URL: "https://example.com/i.txt"}, {Name: "v6", URL: "https://example.com/i.txt"}},
			Rules:   rules}
	}
	deny4 := model.IPRule{Action: model.ActionDeny, Source: "v4"}
	deny6 := model.IPRule{Action: model.ActionDeny, Source: "v6"}
	allow := func(cidr string) model.IPRule { return model.IPRule{Action: model.ActionAllow, CIDR: cidr} }
	both := feedList(deny4, deny6, allow("10.0.0.0/8"), allow("2001:db8::/32"))
	broad := feedList(deny4, deny6, allow("0.0.0.0/0"), allow("::/0"))
	only6 := feedList(deny6, allow("10.0.0.0/8"))
	for _, tc := range []struct {
		name     string
		al       model.AccessList
		client   string
		exempt   bool
		compared int
	}{
		// Outside the allow networks: one comparison, the feeds untouched.
		{"IPv4 client outside", both, "192.0.2.1", false, 1},
		{"IPv6 client outside", both, "3fff::1", false, 1},
		// Inside: the pre-check hit, then its own family's feed and allow rule.
		{"IPv4 client inside", both, "10.200.0.1", true, 1 + n + 1},
		{"IPv4-mapped client inside", both, "::ffff:10.200.0.1", true, 1 + n + 1},
		{"IPv6 client inside", both, "2001:db8:ffff::1", true, 1 + n + 1},
		// v4[1]: the pre-check hit, then the first two feed entries.
		{"IPv4 client in the feed", both, "10.31.1.1", false, 1 + 2},
		{"broad allow", broad, "192.0.2.1", true, 1 + n + 1},
		// An IPv6-only feed above an IPv4 allow costs an IPv4 client nothing.
		{"IPv6 feed, IPv4 client inside", only6, "10.200.0.1", true, 1 + 1},
		{"IPv6 feed, IPv4-mapped client inside", only6, "::ffff:10.200.0.1", true, 1 + 1},
		{"IPv6 feed, IPv6 client in the feed", only6, "2001:db8:1:1::1", false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := exemptListOf(t, tc.al)
			ip := net.ParseIP(tc.client)
			got, compared := e.check(ip)
			if got != tc.exempt || compared != tc.compared {
				t.Fatalf("%s: exempt %v after %d comparisons, want %v after %d", tc.client, got, compared, tc.exempt, tc.compared)
			}
			family := 0
			for _, r := range e.rules(ip) {
				family += len(r.nets)
			}
			if compared > 2*family {
				t.Fatalf("%s: %d comparisons, more than twice its family's %d networks", tc.client, compared, family)
			}
		})
	}

	// The single-family feeds are shared with the installed source sets, not
	// copied, and land in their own family's view only.
	e := exemptListOf(t, both)
	sources := currentAccessListSources()
	if len(e.v4) != 2 || len(e.v6) != 2 || len(e.v4[0].nets) != n || len(e.v6[0].nets) != n {
		t.Fatalf("views: %d IPv4 rules, %d IPv6 rules", len(e.v4), len(e.v6))
	}
	if &e.v4[0].nets[0] != &sources[model.AccessListSourceKey("big", "v4")][0] || &e.v6[0].nets[0] != &sources[model.AccessListSourceKey("big", "v6")][0] {
		t.Fatal("a single-family feed must be shared, not copied")
	}

	// Through router chains: two hosts, one with the middleware twice, each
	// reference naming the list twice, all served by the one compiled list.
	reg := buildRegistry(model.Config{
		AccessLists: []model.AccessList{broad},
		Middlewares: []model.Middleware{{ObjectMeta: model.ObjectMeta{Name: "rl"}, Type: model.MWTypeRateLimit,
			RateLimit: &model.RateLimitMiddleware{RequestsPerSecond: 0.001, Burst: 1, AllowFrom: []string{"@big", "@big"}}}},
	})
	for _, host := range []model.ProxyHost{
		{ObjectMeta: model.ObjectMeta{Name: "a"}, Middlewares: []string{"rl"}},
		{ObjectMeta: model.ObjectMeta{Name: "b"}, Middlewares: []string{"rl", "rl"}},
	} {
		h := buildChain(okHandler(), host, reg, nil)
		for _, c := range []struct {
			remote string
			exempt bool
		}{
			{"10.31.1.1:1", false}, // v4[1]
			{"192.0.2.1:1", true},
			{"[2001:db8:1:1::1]:1", false}, // v6[1]
			{"[2001:db8:ffff::1]:1", true},
		} {
			serveRL(h, c.remote, "http://a/")
			code := serveRL(h, c.remote, "http://a/").Code
			if (code == http.StatusOK) != c.exempt {
				t.Fatalf("%s: second request got %d, exempt want %v", c.remote, code, c.exempt)
			}
		}
	}
	if len(reg.rateLimitExempt) != 1 || reg.rateLimitExempt["big"] == nil {
		t.Fatalf("the build must hold one compiled list, got %d", len(reg.rateLimitExempt))
	}
}

// Through the handler: a client the list denies before its allow rule is
// rate limited, a client the allow rule matches first is not, and a literal
// allowFrom entry still exempts.
func TestRateLimitHandlerAccessListExemptionFirstMatch(t *testing.T) {
	al := model.AccessList{ObjectMeta: model.ObjectMeta{Name: "lan"}, DefaultAction: model.ActionDeny, Rules: []model.IPRule{
		{Action: model.ActionDeny, CIDR: "10.9.0.0/16"},
		{Action: model.ActionAllow, CIDR: "10.0.0.0/8"},
	}}
	reg := buildRegistry(model.Config{AccessLists: []model.AccessList{al}})
	rl := model.RateLimitMiddleware{RequestsPerSecond: 0.001, Burst: 1, AllowFrom: []string{"@lan", "@missing", "10.9.9.9"}}
	h := rateLimitHandlerWith(newRateLimiter(rl), rl, peerIP, "", nil, okHandler(), reg.rateLimitExemption)
	for _, ip := range []string{"10.1.2.3", "10.9.9.9"} {
		for i := 0; i < 3; i++ {
			if rec := serveRL(h, ip+":1", "http://c/"); rec.Code != http.StatusOK {
				t.Fatalf("%s request %d should be exempt, got %d", ip, i+1, rec.Code)
			}
		}
	}
	serveRL(h, "10.9.1.1:1", "http://c/")
	if rec := serveRL(h, "10.9.1.1:1", "http://c/"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("10.9.1.1 is denied by the list before its allow rule and must be limited, got %d", rec.Code)
	}
}

// ipv6Prefix sets how IPv6 clients are grouped into buckets; unset keeps the
// /64, and IPv4 clients are keyed per address whatever the value.
func TestRateLimitIPv6Prefix(t *testing.T) {
	for _, tc := range []struct {
		prefix      int
		first, same string // same shares first's bucket
		other       string // in a different bucket
	}{
		{0, "[2001:db8:1:2::10]:1", "[2001:db8:1:2:ffff::1]:1", "[2001:db8:1:3::10]:1"},
		{64, "[2001:db8:1:2::10]:1", "[2001:db8:1:2:ffff::1]:1", "[2001:db8:1:3::10]:1"},
		{56, "[2001:db8:1:2::10]:1", "[2001:db8:1:ff::1]:1", "[2001:db8:1:100::10]:1"},
		{48, "[2001:db8:1:2::10]:1", "[2001:db8:1:ffff::1]:1", "[2001:db8:2::10]:1"},
		{48, "192.0.2.10:1", "192.0.2.10:2", "192.0.2.11:1"},
	} {
		t.Run(fmt.Sprintf("%d %s", tc.prefix, tc.first), func(t *testing.T) {
			h := rateLimitHandler(model.RateLimitMiddleware{RequestsPerSecond: 0.001, Burst: 1, IPv6Prefix: tc.prefix}, peerIP, "", nil, okHandler(), nil)
			if rec := serveRL(h, tc.first, "http://c/"); rec.Code != http.StatusOK {
				t.Fatalf("first request: %d", rec.Code)
			}
			if rec := serveRL(h, tc.same, "http://c/"); rec.Code != http.StatusTooManyRequests {
				t.Fatalf("%s must share the bucket of %s, got %d", tc.same, tc.first, rec.Code)
			}
			if rec := serveRL(h, tc.other, "http://c/"); rec.Code != http.StatusOK {
				t.Fatalf("%s must have its own bucket, got %d", tc.other, rec.Code)
			}
		})
	}
}

// Requests read the build's compiled exemption lists while another build fills
// its own; run under -race this shows the request path needs no lock.
func TestRateLimitExemptionConcurrentReads(t *testing.T) {
	cfg := model.Config{
		AccessLists: []model.AccessList{{ObjectMeta: model.ObjectMeta{Name: "lan"}, DefaultAction: model.ActionDeny,
			Rules: []model.IPRule{{Action: model.ActionDeny, CIDR: "10.9.0.0/16"}, {Action: model.ActionAllow, CIDR: "10.0.0.0/8"}}}},
		Middlewares: []model.Middleware{{ObjectMeta: model.ObjectMeta{Name: "rl"}, Type: model.MWTypeRateLimit,
			RateLimit: &model.RateLimitMiddleware{RequestsPerSecond: 1000, Burst: 1000, AllowFrom: []string{"@lan"}}}},
	}
	h := buildChain(okHandler(), model.ProxyHost{ObjectMeta: model.ObjectMeta{Name: "a"}, Middlewares: []string{"rl"}}, buildRegistry(cfg), nil)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				serveRL(h, "10.1.2.3:1", "http://a/")
				serveRL(h, "10.9.1.1:1", "http://a/")
			}
		}()
	}
	for i := 0; i < 20; i++ {
		buildRegistry(cfg)
	}
	wg.Wait()
}
