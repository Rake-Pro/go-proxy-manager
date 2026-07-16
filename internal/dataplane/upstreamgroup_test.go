package dataplane

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// upstreamOf converts an httptest server URL into a model.Upstream.
func upstreamOf(t *testing.T, srv *httptest.Server) model.Upstream {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(u.Port())
	return model.Upstream{Scheme: "http", Host: u.Hostname(), Port: port}
}

// deadUpstream reserves a port with a listener, closes it, and returns an
// upstream pointing at the now-refusing port.
func deadUpstream(t *testing.T) model.Upstream {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: port}
}

// groupRouter compiles a one-host config whose host is backed by group and
// returns the router plus the health manager for inspection. probe=false skips
// the stage commit, so no probe goroutines run and health changes only through
// passive (live-traffic) detection - deterministic for failover tests that need
// the initial optimistic state to survive until the first request.
func groupRouter(t *testing.T, group model.UpstreamGroup, probe bool) (*router, *healthManager) {
	t.Helper()
	cfg := model.Config{
		UpstreamGroups: []model.UpstreamGroup{group},
		ProxyHosts: []model.ProxyHost{{
			ObjectMeta:       model.ObjectMeta{Name: "app"},
			Domains:          []string{"app.example.com"},
			UpstreamGroupRef: group.Name,
		}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("fixture config invalid: %v", err)
	}
	m := newHealthManager()
	st := m.stage(cfg.UpstreamGroups)
	rt, err := buildRouter(cfg, "", st)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}
	if probe {
		st.commit()
		t.Cleanup(m.stopAll)
	}
	return rt, m
}

// gups wraps plain upstreams as unweighted group upstreams.
func gups(ups ...model.Upstream) []model.GroupUpstream {
	out := make([]model.GroupUpstream, len(ups))
	for i, u := range ups {
		out[i] = model.GroupUpstream{Upstream: u}
	}
	return out
}

func serveGroup(rt *router, method, path string, body io.Reader) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://app.example.com"+path, body)
	w := httptest.NewRecorder()
	rt.serveHTTP(w, req)
	return w
}

func TestGroupFailoverOnDialError(t *testing.T) {
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		fmt.Fprintf(w, "backup:%s", b)
	}))
	defer backup.Close()

	group := model.UpstreamGroup{
		ObjectMeta:  model.ObjectMeta{Name: "g"},
		Upstreams:   gups(deadUpstream(t), upstreamOf(t, backup)),
		HealthCheck: model.HealthCheck{IntervalSeconds: 3600, Fall: 1},
	}
	rt, m := groupRouter(t, group, true)

	// A POST with a body must arrive intact at the backup: the dial to the dead
	// primary never transmitted anything, so the retry is safe and lossless.
	w := serveGroup(rt, http.MethodPost, "/submit", strings.NewReader("hello"))
	if w.Code != http.StatusOK || w.Body.String() != "backup:hello" {
		t.Fatalf("failover POST = %d %q, want 200 backup:hello", w.Code, w.Body.String())
	}

	// The dial failure fed the fall counter (fall=1): primary now unhealthy.
	snap := m.snapshot()["g"]
	if len(snap) != 2 || snap[0].Healthy || !snap[1].Healthy {
		t.Fatalf("snapshot after failover = %+v, want primary down / backup up", snap)
	}

	// Next request goes straight to the healthy backup (no dial attempt on the
	// dead primary; still 200 either way, but exercises candidates() ordering).
	w = serveGroup(rt, http.MethodGet, "/", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("second request = %d, want 200", w.Code)
	}
}

func TestGroupLargeBodySingleAttempt(t *testing.T) {
	var backupHits int
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backupHits++
		n, _ := io.Copy(io.Discard, r.Body)
		fmt.Fprintf(w, "%d", n)
	}))
	defer backup.Close()

	group := model.UpstreamGroup{
		ObjectMeta:  model.ObjectMeta{Name: "g"},
		Upstreams:   gups(deadUpstream(t), upstreamOf(t, backup)),
		HealthCheck: model.HealthCheck{IntervalSeconds: 3600, Fall: 1},
	}
	// probe=false: with probers running, the immediate first probe round would
	// mark the dead primary down and healthy-first ordering would dodge the
	// single-attempt path this test exists to exercise.
	rt, _ := groupRouter(t, group, false)

	big := strings.Repeat("x", groupRetryBufBytes+1024)

	// Over the replay buffer: exactly one attempt, at the (still healthy-marked)
	// dead primary - a 502, and the backup must NOT see a half-replayed body.
	w := serveGroup(rt, http.MethodPost, "/upload", strings.NewReader(big))
	if w.Code != http.StatusBadGateway || backupHits != 0 {
		t.Fatalf("large body first attempt = %d (backup hits %d), want 502 with no backup attempt", w.Code, backupHits)
	}

	// The dial failure marked the primary down (fall=1): the next large upload
	// goes straight to the backup, intact.
	w = serveGroup(rt, http.MethodPost, "/upload", strings.NewReader(big))
	if w.Code != http.StatusOK || w.Body.String() != fmt.Sprint(len(big)) {
		t.Fatalf("large body after fall = %d %q, want 200 with %d bytes received", w.Code, w.Body.String(), len(big))
	}
}

func TestGroupNoFailoverOnAppError(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer primary.Close()
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "backup")
	}))
	defer backup.Close()

	group := model.UpstreamGroup{
		ObjectMeta:  model.ObjectMeta{Name: "g"},
		Upstreams:   gups(upstreamOf(t, primary), upstreamOf(t, backup)),
		HealthCheck: model.HealthCheck{IntervalSeconds: 3600, Fall: 1},
	}
	rt, m := groupRouter(t, group, true)

	// An application-level 5xx is a served response, NOT an upstream-connect
	// failure: it must pass through unchanged (the same app would 500 via every
	// entry point), and it must not mark the upstream down.
	w := serveGroup(rt, http.MethodGet, "/", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("app error = %d, want 500 passed through", w.Code)
	}
	snap := m.snapshot()["g"]
	if !snap[0].Healthy {
		t.Fatalf("primary marked unhealthy by app 5xx; passive detection must be connect-only")
	}
}

func TestGroupAllDownFailsOpen(t *testing.T) {
	group := model.UpstreamGroup{
		ObjectMeta:  model.ObjectMeta{Name: "g"},
		Upstreams:   gups(deadUpstream(t), deadUpstream(t)),
		HealthCheck: model.HealthCheck{IntervalSeconds: 3600, Fall: 1},
	}
	rt, _ := groupRouter(t, group, true)

	// First pass marks both down via passive failures...
	if w := serveGroup(rt, http.MethodGet, "/", nil); w.Code != http.StatusBadGateway {
		t.Fatalf("all-down = %d, want 502", w.Code)
	}
	// ...and with every upstream unhealthy the group still ATTEMPTS them
	// (fail-open) rather than short-circuiting: the result is a 502 from a real
	// dial attempt, not a refusal to try.
	if w := serveGroup(rt, http.MethodGet, "/", nil); w.Code != http.StatusBadGateway {
		t.Fatalf("all-down retry = %d, want 502", w.Code)
	}
}

func TestRiseFallTransitions(t *testing.T) {
	g := newGroupHealth(model.UpstreamGroup{
		ObjectMeta:  model.ObjectMeta{Name: "g"},
		Upstreams:   gups(model.Upstream{Scheme: "http", Host: "192.0.2.11", Port: 80}),
		HealthCheck: model.HealthCheck{Rise: 2, Fall: 2},
	})
	u := g.ups[0]

	if !u.healthy.Load() {
		t.Fatal("upstream must start healthy")
	}
	u.reportFailure()
	if !u.healthy.Load() {
		t.Fatal("one failure below fall threshold must not mark down")
	}
	u.reportFailure()
	if u.healthy.Load() {
		t.Fatal("fall threshold reached, must be down")
	}
	u.reportSuccess()
	if u.healthy.Load() {
		t.Fatal("one success below rise threshold must not recover")
	}
	u.reportSuccess()
	if !u.healthy.Load() {
		t.Fatal("rise threshold reached, must recover")
	}
	// An intervening failure resets the rise streak.
	u.reportFailure()
	u.reportFailure()
	u.reportSuccess()
	u.reportFailure()
	u.reportSuccess()
	if u.healthy.Load() {
		t.Fatal("rise streak broken by failure must not recover")
	}
}

func TestCandidatesOrdering(t *testing.T) {
	g := newGroupHealth(model.UpstreamGroup{
		ObjectMeta: model.ObjectMeta{Name: "g"},
		Upstreams: gups(
			model.Upstream{Scheme: "http", Host: "192.0.2.11", Port: 80},
			model.Upstream{Scheme: "http", Host: "192.0.2.12", Port: 80},
			model.Upstream{Scheme: "http", Host: "192.0.2.13", Port: 80},
		),
	})
	g.ups[0].healthy.Store(false)

	got := g.candidates("")
	want := []string{g.ups[1].label, g.ups[2].label, g.ups[0].label}
	for i, u := range got {
		if u.label != want[i] {
			t.Fatalf("candidates[%d] = %s, want %s (healthy first in config order, then down)", i, u.label, want[i])
		}
	}
}

func policyGroup(policy string, ups ...model.GroupUpstream) *groupHealth {
	return newGroupHealth(model.UpstreamGroup{
		ObjectMeta: model.ObjectMeta{Name: "g"},
		Policy:     policy,
		Upstreams:  ups,
	})
}

func TestRoundRobinWeightedDistribution(t *testing.T) {
	g := policyGroup(model.PolicyRoundRobin,
		model.GroupUpstream{Upstream: model.Upstream{Scheme: "http", Host: "192.0.2.11", Port: 80}, Weight: 3},
		model.GroupUpstream{Upstream: model.Upstream{Scheme: "http", Host: "192.0.2.12", Port: 80}},
		model.GroupUpstream{Upstream: model.Upstream{Scheme: "http", Host: "192.0.2.13", Port: 80}},
	)
	picks := map[string]int{}
	for i := 0; i < 50; i++ {
		picks[g.candidates("")[0].label]++
	}
	// Smooth WRR with weights 3/1/1 over 50 picks: exactly 30/10/10.
	if picks[g.ups[0].label] != 30 || picks[g.ups[1].label] != 10 || picks[g.ups[2].label] != 10 {
		t.Fatalf("weighted round-robin distribution = %v, want 30/10/10", picks)
	}

	// An unhealthy upstream drops out of rotation but stays as the retry tail.
	g.ups[0].healthy.Store(false)
	picks = map[string]int{}
	for i := 0; i < 10; i++ {
		c := g.candidates("")
		picks[c[0].label]++
		if c[len(c)-1] != g.ups[0] {
			t.Fatal("unhealthy upstream must be demoted to the tail")
		}
	}
	if picks[g.ups[0].label] != 0 || picks[g.ups[1].label] != 5 || picks[g.ups[2].label] != 5 {
		t.Fatalf("post-down distribution = %v, want 0/5/5", picks)
	}
}

func TestLeastConnectionsOrdering(t *testing.T) {
	g := policyGroup(model.PolicyLeastConnections,
		model.GroupUpstream{Upstream: model.Upstream{Scheme: "http", Host: "192.0.2.11", Port: 80}},
		model.GroupUpstream{Upstream: model.Upstream{Scheme: "http", Host: "192.0.2.12", Port: 80}},
		model.GroupUpstream{Upstream: model.Upstream{Scheme: "http", Host: "192.0.2.13", Port: 80}, Weight: 4},
	)
	g.ups[0].active.Store(5)
	g.ups[1].active.Store(1)
	g.ups[2].active.Store(8) // 8 in-flight but weight 4 -> effective load 2

	got := g.candidates("")
	want := []*upstreamHealth{g.ups[1], g.ups[2], g.ups[0]} // loads 1, 2, 5
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("least-connections order[%d] = %s, want %s", i, got[i].label, want[i].label)
		}
	}
}

func TestIPHashStickiness(t *testing.T) {
	g := policyGroup(model.PolicyIPHash,
		model.GroupUpstream{Upstream: model.Upstream{Scheme: "http", Host: "192.0.2.11", Port: 80}},
		model.GroupUpstream{Upstream: model.Upstream{Scheme: "http", Host: "192.0.2.12", Port: 80}},
		model.GroupUpstream{Upstream: model.Upstream{Scheme: "http", Host: "192.0.2.13", Port: 80}},
	)
	// Same client key always maps to the same upstream.
	first := g.candidates("203.0.113.7")[0]
	for i := 0; i < 5; i++ {
		if g.candidates("203.0.113.7")[0] != first {
			t.Fatal("ip-hash pick must be stable for a fixed client key")
		}
	}
	// Rendezvous property: taking down an upstream a client is NOT pinned to
	// must not move that client.
	var other *upstreamHealth
	for _, u := range g.ups {
		if u != first {
			other = u
			break
		}
	}
	other.healthy.Store(false)
	if g.candidates("203.0.113.7")[0] != first {
		t.Fatal("client moved although its pinned upstream stayed healthy")
	}
	other.healthy.Store(true)
	// When the pinned upstream dies, the client falls to its next-ranked choice
	// deterministically.
	first.healthy.Store(false)
	fallback := g.candidates("203.0.113.7")[0]
	if fallback == first || g.candidates("203.0.113.7")[0] != fallback {
		t.Fatal("fallback pick must be a different upstream and stable")
	}
}

func TestActiveCountTracksBodyLifecycle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	g := policyGroup(model.PolicyLeastConnections, gups(upstreamOf(t, srv))...)
	tr := &groupTransport{gh: g, base: http.DefaultTransport}

	req, _ := http.NewRequest(http.MethodGet, "http://placeholder/", nil)
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if got := g.ups[0].active.Load(); got != 1 {
		t.Fatalf("in-flight count with open body = %d, want 1", got)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	_ = resp.Body.Close() // idempotent: a double close must not underflow
	if got := g.ups[0].active.Load(); got != 0 {
		t.Fatalf("in-flight count after body close = %d, want 0", got)
	}

	// A connect failure releases the count immediately.
	dead := policyGroup("", gups(deadUpstream(t))...)
	dtr := &groupTransport{gh: dead, base: http.DefaultTransport}
	if _, err := dtr.RoundTrip(req.Clone(req.Context())); err == nil {
		t.Fatal("expected connect error")
	}
	if got := dead.ups[0].active.Load(); got != 0 {
		t.Fatalf("in-flight count after connect failure = %d, want 0", got)
	}
}

func TestStickyCookieAffinity(t *testing.T) {
	mkBackend := func(tag string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, tag)
		}))
	}
	a, b := mkBackend("A"), mkBackend("B")
	defer a.Close()
	defer b.Close()

	g := newGroupHealth(model.UpstreamGroup{
		ObjectMeta: model.ObjectMeta{Name: "g"},
		Policy:     model.PolicyRoundRobin, // would alternate without the pin
		Stickiness: &model.Stickiness{TTL: "1h"},
		Upstreams:  gups(upstreamOf(t, a), upstreamOf(t, b)),
	})
	tr := &groupTransport{gh: g, base: http.DefaultTransport}

	do := func(cookies ...*http.Cookie) (string, *http.Response) {
		req, _ := http.NewRequest(http.MethodGet, "http://placeholder/", nil)
		for _, c := range cookies {
			req.AddCookie(c)
		}
		resp, err := tr.RoundTrip(req)
		if err != nil {
			t.Fatalf("RoundTrip: %v", err)
		}
		bodyBytes, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return string(bodyBytes), resp
	}

	// First request: assigned by policy, signed affinity cookie issued.
	body1, resp1 := do()
	setCookies := resp1.Cookies()
	if len(setCookies) != 1 || setCookies[0].Name != "gpm-sticky-g" {
		t.Fatalf("first response cookies = %+v, want one gpm-sticky-g", setCookies)
	}
	pin := setCookies[0]
	if !pin.HttpOnly || pin.Path != "/" || pin.MaxAge != 3600 || pin.Secure {
		t.Fatalf("cookie attrs = %+v, want HttpOnly, Path=/, MaxAge=3600, not Secure (plain HTTP)", pin)
	}

	// Replaying the cookie pins the client: round-robin would alternate, the
	// pin must not - and an honored pin gets no fresh Set-Cookie.
	for i := 0; i < 3; i++ {
		body, resp := do(pin)
		if body != body1 {
			t.Fatalf("pinned request %d hit %q, want %q", i, body, body1)
		}
		if len(resp.Cookies()) != 0 {
			t.Fatal("honored pin must not re-issue the cookie")
		}
	}
	// Without a cookie round-robin really does move (sanity that the pin, not
	// chance, kept the requests together).
	if body, _ := do(); body == body1 {
		if body2, _ := do(); body2 == body1 {
			t.Fatal("round-robin never alternated; pin assertions prove nothing")
		}
	}

	// A tampered cookie is ignored: request is reassigned and re-cookied.
	bad := *pin
	bad.Value = bad.Value[:len(bad.Value)-2] + "xx"
	if _, resp := do(&bad); len(resp.Cookies()) != 1 {
		t.Fatal("tampered cookie must trigger a fresh assignment cookie")
	}

	// An expired-but-validly-signed cookie is rejected server-side even though
	// a misbehaving client replays it past Max-Age.
	expiredPayload := strings.Join([]string{stickyPayloadPrefix, "g", g.ups[0].label,
		strconv.FormatInt(time.Now().Add(-time.Minute).Unix(), 10)}, "|")
	expired := &http.Cookie{Name: "gpm-sticky-g", Value: signToken([]byte(expiredPayload))}
	if _, resp := do(expired); len(resp.Cookies()) != 1 {
		t.Fatal("expired cookie must trigger a fresh assignment cookie")
	}

	// A cookie for another group's name/payload is ignored.
	foreignPayload := strings.Join([]string{stickyPayloadPrefix, "other", g.ups[0].label,
		strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)}, "|")
	foreign := &http.Cookie{Name: "gpm-sticky-g", Value: signToken([]byte(foreignPayload))}
	if _, resp := do(foreign); len(resp.Cookies()) != 1 {
		t.Fatal("foreign-group cookie must trigger a fresh assignment cookie")
	}

	// When the pinned upstream dies, the client is re-homed and re-cookied.
	req, _ := http.NewRequest(http.MethodGet, "http://placeholder/", nil)
	req.AddCookie(pin)
	pinned := g.stickyUpstream(req)
	if pinned == nil {
		t.Fatal("pin did not resolve")
	}
	pinned.healthy.Store(false)
	body, resp := do(pin)
	if body == body1 {
		t.Fatalf("client stayed on a dead upstream (%q)", body)
	}
	if len(resp.Cookies()) != 1 {
		t.Fatal("re-homed client must get a fresh cookie")
	}

	// Secure attribute follows the client-facing scheme.
	reqTLS, _ := http.NewRequest(http.MethodGet, "http://placeholder/", nil)
	reqTLS.Header.Set("X-Forwarded-Proto", "https")
	if c := g.stickyCookieFor(g.ups[1], reqTLS); !c.Secure {
		t.Fatal("cookie must be Secure when the client came over https")
	}
}

func TestLocationGroupRefServing(t *testing.T) {
	hostBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "host-backend")
	}))
	defer hostBackend.Close()
	groupBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "group-backend")
	}))
	defer groupBackend.Close()

	cfg := model.Config{
		UpstreamGroups: []model.UpstreamGroup{{
			ObjectMeta:  model.ObjectMeta{Name: "g"},
			Upstreams:   gups(upstreamOf(t, groupBackend)),
			HealthCheck: model.HealthCheck{IntervalSeconds: 3600},
		}},
		ProxyHosts: []model.ProxyHost{{
			ObjectMeta: model.ObjectMeta{Name: "app"},
			Domains:    []string{"app.example.com"},
			Upstream:   upstreamOf(t, hostBackend),
			Locations:  []model.Location{{Path: "/svc", UpstreamGroupRef: "g"}},
		}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("fixture config invalid: %v", err)
	}
	m := newHealthManager()
	defer m.stopAll()
	st := m.stage(cfg.UpstreamGroups)
	rt, err := buildRouter(cfg, "", st)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}
	st.commit()

	// A single-upstream host can use a group for one location only.
	if w := serveGroup(rt, http.MethodGet, "/", nil); w.Body.String() != "host-backend" {
		t.Fatalf("host path served %q, want host-backend", w.Body.String())
	}
	if w := serveGroup(rt, http.MethodGet, "/svc/x", nil); w.Body.String() != "group-backend" {
		t.Fatalf("location path served %q, want group-backend", w.Body.String())
	}
}

func TestProbeUpstream(t *testing.T) {
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(http.StatusOK)
		case "/missing":
			w.WriteHeader(http.StatusNotFound)
		case "/redirect":
			http.Redirect(w, r, "/elsewhere", http.StatusMovedPermanently)
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer okSrv.Close()
	up := upstreamOf(t, okSrv)
	uh := &upstreamHealth{up: up, addr: net.JoinHostPort(up.Host, strconv.Itoa(up.Port))}

	timeout := 2 * time.Second
	if !probeUpstream(uh, "", timeout) {
		t.Fatal("TCP probe against live listener must succeed")
	}
	if !probeUpstream(uh, "/ok", timeout) {
		t.Fatal("HTTP probe 200 must succeed")
	}
	if !probeUpstream(uh, "/missing", timeout) {
		t.Fatal("HTTP probe 404 must succeed (entry point is alive)")
	}
	if !probeUpstream(uh, "/redirect", timeout) {
		t.Fatal("HTTP probe 301 must succeed without following the redirect")
	}
	if probeUpstream(uh, "/fail", timeout) {
		t.Fatal("HTTP probe 503 must fail")
	}

	dead := deadUpstream(t)
	dh := &upstreamHealth{up: dead, addr: net.JoinHostPort(dead.Host, strconv.Itoa(dead.Port))}
	if probeUpstream(dh, "", timeout) {
		t.Fatal("TCP probe against closed port must fail")
	}
	if probeUpstream(dh, "/ok", timeout) {
		t.Fatal("HTTP probe against closed port must fail")
	}
}

func TestHealthStagePreservesUnchangedState(t *testing.T) {
	group := model.UpstreamGroup{
		ObjectMeta:  model.ObjectMeta{Name: "g"},
		Upstreams:   gups(model.Upstream{Scheme: "http", Host: "192.0.2.11", Port: 80}),
		HealthCheck: model.HealthCheck{IntervalSeconds: 3600},
	}
	m := newHealthManager()
	defer m.stopAll()

	st := m.stage([]model.UpstreamGroup{group})
	first := st.lookup("g")
	st.commit()
	first.ups[0].healthy.Store(false) // accumulate state

	// Same spec (meta-only change): staged state is the SAME object, down-marker
	// intact after commit.
	changedMeta := group
	changedMeta.DisplayName = "Edge nodes"
	st = m.stage([]model.UpstreamGroup{changedMeta})
	if st.lookup("g") != first {
		t.Fatal("unchanged spec must reuse the live group state")
	}
	st.commit()
	if m.snapshot()["g"][0].Healthy {
		t.Fatal("health state lost across a meta-only reload")
	}

	// Changed spec: fresh state, healthy again.
	changed := group
	changed.Upstreams = append([]model.GroupUpstream{}, group.Upstreams...)
	changed.Upstreams[0].Port = 81
	st = m.stage([]model.UpstreamGroup{changed})
	if st.lookup("g") == first {
		t.Fatal("changed spec must build fresh group state")
	}
	st.commit()
	if !m.snapshot()["g"][0].Healthy {
		t.Fatal("fresh group state must start healthy")
	}

	// Group removed: state dropped.
	m.stage(nil).commit()
	if len(m.snapshot()) != 0 {
		t.Fatal("removed group must leave no health state")
	}

	// A discarded stage (failed build) must not disturb the running state.
	st = m.stage(nil)
	_ = st // never committed
	st2 := m.stage([]model.UpstreamGroup{group})
	st2.commit()
	if len(m.snapshot()["g"]) != 1 {
		t.Fatal("running state disturbed by a discarded stage")
	}
}

func TestConcurrentReloadsDoNotCorruptHealthState(t *testing.T) {
	// Reload is invoked from independent goroutines in production (admin API
	// writes and the ACME renewal loop). Interleaved stage/commit sequences on
	// the shared health state used to be able to reinstate a stopped prober and
	// later panic closing its already-closed stop channel; Reload is now
	// serialized end to end.
	mkCfg := func(port int) model.Config {
		return model.Config{
			UpstreamGroups: []model.UpstreamGroup{{
				ObjectMeta:  model.ObjectMeta{Name: "g"},
				Upstreams:   gups(model.Upstream{Scheme: "http", Host: "192.0.2.11", Port: port}),
				HealthCheck: model.HealthCheck{IntervalSeconds: 3600},
			}},
			ProxyHosts: []model.ProxyHost{{
				ObjectMeta:       model.ObjectMeta{Name: "app"},
				Domains:          []string{"app.example.com"},
				UpstreamGroupRef: "g",
			}},
		}
	}
	s := New(Config{})
	defer s.health.stopAll()

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				// Alternate specs so groups are constantly replaced (stop+start).
				if err := s.Reload(mkCfg(80 + (n+j)%2)); err != nil {
					t.Errorf("Reload: %v", err)
					return
				}
			}
		}(i)
	}
	wg.Wait()

	if err := s.Reload(mkCfg(80)); err != nil {
		t.Fatalf("final Reload: %v", err)
	}
	snap := s.UpstreamHealth()
	if len(snap["g"]) != 1 {
		t.Fatalf("health state corrupted: snapshot = %+v", snap)
	}
}

func TestGroupLocationOverride(t *testing.T) {
	hostBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "group-backend")
	}))
	defer hostBackend.Close()
	locBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "loc-backend")
	}))
	defer locBackend.Close()

	locUp := upstreamOf(t, locBackend)
	cfg := model.Config{
		UpstreamGroups: []model.UpstreamGroup{{
			ObjectMeta:  model.ObjectMeta{Name: "g"},
			Upstreams:   gups(upstreamOf(t, hostBackend)),
			HealthCheck: model.HealthCheck{IntervalSeconds: 3600},
		}},
		ProxyHosts: []model.ProxyHost{{
			ObjectMeta:       model.ObjectMeta{Name: "app"},
			Domains:          []string{"app.example.com"},
			UpstreamGroupRef: "g",
			Locations:        []model.Location{{Path: "/svc", Upstream: &locUp}},
		}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("fixture config invalid: %v", err)
	}
	m := newHealthManager()
	defer m.stopAll()
	st := m.stage(cfg.UpstreamGroups)
	rt, err := buildRouter(cfg, "", st)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}
	st.commit()

	if w := serveGroup(rt, http.MethodGet, "/", nil); w.Body.String() != "group-backend" {
		t.Fatalf("host path served %q, want group-backend", w.Body.String())
	}
	// A location with its own single upstream overrides the host's group.
	if w := serveGroup(rt, http.MethodGet, "/svc/x", nil); w.Body.String() != "loc-backend" {
		t.Fatalf("location path served %q, want loc-backend", w.Body.String())
	}
}

func TestBuildRouterRejectsMissingGroupState(t *testing.T) {
	cfg := model.Config{
		ProxyHosts: []model.ProxyHost{{
			ObjectMeta:       model.ObjectMeta{Name: "app"},
			Domains:          []string{"app.example.com"},
			UpstreamGroupRef: "ghost",
		}},
	}
	// nil resolver (and equally a stage without the group) must fail the build,
	// not compile a host with no backend.
	if _, err := buildRouter(cfg, "", nil); err == nil {
		t.Fatal("buildRouter with unresolvable group must error")
	}
}
