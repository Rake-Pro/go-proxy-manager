package dataplane

import (
	"net"
	"sync/atomic"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// globalAccessListSources holds the compiled fetched sets for AccessList
// sources, keyed by "<list>/<source>" (model.AccessListSourceKey), installed
// once per config reload via SetAccessListSources and read by compileAccessList
// when it resolves a source-backed rule.
//
// It is a package-level handle for exactly the reason globalSecurityHeaders and
// globalStripResponseHeaders are: the ledger is not part of model.Config (it is
// a separate committed singleton the fetcher owns), it only ever changes
// alongside a full reload, and threading it through buildRouter, buildRegistry,
// buildChain, buildLocations and the stream-host compile would touch far more of
// the data plane than the value is worth.
var globalAccessListSources atomic.Pointer[map[string][]*net.IPNet]

// SetAccessListSources compiles and installs the fetched source sets. It is
// called before the data-plane Reload, like every other settings-level
// installer, so the sets are in place before any request is served - and so a
// reload triggered by a completed fetch serves the new set on the very next
// request.
//
// A malformed entry is DROPPED rather than failing the install: the ledger has
// already passed model validation at load time, and an allow list that is one
// network short is a far better outcome than a data plane that will not compile.
func SetAccessListSources(l model.AccessListSourceLedger) {
	c := compileAccessListSources(l)
	globalAccessListSources.Store(&c)
}

func currentAccessListSources() map[string][]*net.IPNet {
	if p := globalAccessListSources.Load(); p != nil {
		return *p
	}
	return nil
}

func compileAccessListSources(l model.AccessListSourceLedger) map[string][]*net.IPNet {
	if len(l.Sources) == 0 {
		return nil
	}
	out := make(map[string][]*net.IPNet, len(l.Sources))
	for _, e := range l.Sources {
		nets := make([]*net.IPNet, 0, len(e.Entries))
		for _, c := range e.Entries {
			if n := parseNet(c); n != nil {
				nets = append(nets, n)
			}
		}
		out[e.Key()] = nets
	}
	return out
}
