package docker

import (
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// A host restamped after a domain conflict carries the upstream derived from
// the container as it is now, not the one stored for it earlier: the container
// was recreated and its old address may belong to another container.
func TestDomainConflictRestampTakesTheCurrentUpstream(t *testing.T) {
	conf := testConf(nil)
	stale := managedHostFixture("dkr-app", func(h *model.ProxyHost) { h.AccessLists = nil })
	operator := model.ProxyHost{
		ObjectMeta: model.ObjectMeta{Name: "operator-www"},
		Domains:    []string{"www.example.com"},
		Upstream:   model.Upstream{Scheme: "http", Host: "192.0.2.41", Port: 80},
	}
	ct := container("app", enabled(map[string]string{"gpm.rake.pro/domains": "app.example.com,www.example.com"}),
		withNetworks(map[string]string{"bridge": "192.0.2.20"}))

	p := planReconcile(model.Config{ProxyHosts: []model.ProxyHost{stale, operator}}, conf, []Container{ct}, nil, nil)
	if len(p.Upserts) != 1 {
		t.Fatalf("plan %+v, want one restamp", p)
	}
	u := p.Upserts[0]
	if u.Upstream.Host != "192.0.2.20" || u.Upstream.Port != 8080 {
		t.Fatalf("restamped upstream %+v, want the container's current address 192.0.2.20:8080", u.Upstream)
	}
	if len(u.Domains) != 1 || u.Domains[0] != "app.example.com" || len(u.AccessLists) != 1 {
		t.Fatalf("upsert = %+v, want the profile chain on app.example.com only", u)
	}

	// The next reconcile, with the conflict still in place, leaves the restamped
	// host alone: nothing else would ever correct its upstream while it lasts.
	p = planReconcile(model.Config{ProxyHosts: []model.ProxyHost{u, operator}}, conf, []Container{ct}, nil, nil)
	if len(p.Upserts) != 0 || p.Skipped != 1 {
		t.Fatalf("second plan %+v, want a plain skip and no write", p)
	}

	// The container is recreated on a new address while the conflict lasts: the
	// chain still matches, and the stored upstream alone triggers the restamp.
	moved := container("app", enabled(map[string]string{"gpm.rake.pro/domains": "app.example.com,www.example.com"}),
		withNetworks(map[string]string{"bridge": "192.0.2.30"}))
	p = planReconcile(model.Config{ProxyHosts: []model.ProxyHost{u, operator}}, conf, []Container{moved}, nil, nil)
	if len(p.Upserts) != 1 || p.Upserts[0].Upstream.Host != "192.0.2.30" || p.Upserts[0].Disabled {
		t.Fatalf("third plan %+v, want one restamp onto 192.0.2.30", p)
	}
	if d := p.Upserts[0].Domains; len(d) != 1 || d[0] != "app.example.com" {
		t.Fatalf("restamp changed the domains to %v", d)
	}
}
