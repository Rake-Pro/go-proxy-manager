package dataplane

import (
	"fmt"
	"net"
	"sync/atomic"

	"github.com/Rake-Pro/go-proxy-manager/internal/accesssync"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
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
// So is an entry the fetcher's entry guard refuses: the ledger may hold entries
// stored before a guard rule existed, and a refused fetch keeps the stored set,
// so the guard is applied here as well as at fetch time.
func SetAccessListSources(l model.AccessListSourceLedger) {
	c := compileAccessListSources(l, func(e model.AccessListSourceEntry, err error) {
		log.Warn().Err(err).Str("accessList", e.List).Str("source", e.Source).
			Msg("dataplane: dropped a stored access-list source entry the entry guard refuses")
	})
	globalAccessListSources.Store(&c)
}

func currentAccessListSources() map[string][]*net.IPNet {
	if p := globalAccessListSources.Load(); p != nil {
		return *p
	}
	return nil
}

// compileAccessListSources resolves every ledger entry to the set the data
// plane serves. refused, when set, is told about each stored entry the entry
// guard drops.
func compileAccessListSources(l model.AccessListSourceLedger, refused func(model.AccessListSourceEntry, error)) map[string][]*net.IPNet {
	if len(l.Sources) == 0 {
		return nil
	}
	out := make(map[string][]*net.IPNet, len(l.Sources))
	for _, e := range l.Sources {
		// An expired entry (maxAge passed) resolves to the empty set, like an
		// unfetched one. The fetcher already stores it without entries; this
		// holds even for a ledger that did not.
		if e.ExpiredAt != "" {
			out[e.Key()] = nil
			continue
		}
		nets := make([]*net.IPNet, 0, len(e.Entries))
		for _, c := range e.Entries {
			n := accesssync.ParseEntry(c)
			if n == nil {
				continue
			}
			if err := accesssync.RefuseEntry(n); err != nil {
				if refused != nil {
					refused(e, err)
				}
				continue
			}
			nets = append(nets, n)
		}
		out[e.Key()] = nets
	}
	return out
}

// sourceNets resolves a source rule of al to its compiled set. A set over the
// source's coverage budget resolves to the empty set, like an unfetched one, so
// a set stored before the budget existed (or before it was lowered) is not
// served while refused fetches keep it in the ledger.
func sourceNets(sources map[string][]*net.IPNet, al model.AccessList, name string) []*net.IPNet {
	nets := sources[model.AccessListSourceKey(al.Name, name)]
	for _, src := range al.Sources {
		if src.Name == name && accesssync.CheckCoverage(nets, src) != nil {
			return nil
		}
	}
	return nets
}

// UnresolvedDenySourceWarnings returns one config warning per deny rule (in an
// enabled list, see model.DenySourceRefs) whose source the data plane would
// resolve to the empty set from l, so the rule denies no one (see
// model.IPRule.Source). It resolves through the same compile and sourceNets
// the data plane uses, so a warning and the served set cannot disagree; the
// ledger entry only picks the reason given.
func UnresolvedDenySourceWarnings(refs []model.AccessListSourceRef, l model.AccessListSourceLedger) []string {
	if len(refs) == 0 {
		return nil
	}
	sources := compileAccessListSources(l, nil)
	entries := l.AccessListSourceMap()
	var out []string
	for _, ref := range refs {
		if len(sourceNets(sources, ref.List, ref.Source)) > 0 {
			continue
		}
		key := model.AccessListSourceKey(ref.List.Name, ref.Source)
		e, ok := entries[key]
		var why string
		switch {
		case !ok:
			why = "has never been fetched successfully"
		case e.ExpiredAt != "":
			why = "expired at " + e.ExpiredAt + " (maxAge passed with no successful fetch)"
		case len(sources[key]) == 0:
			why = "has a stored set that resolves to no networks"
		default:
			why = "has a stored set over its maxCoverage budget"
		}
		out = append(out, fmt.Sprintf("access list %q: deny rule source %q %s, so it matches nothing and denies no one until a fetch succeeds; check settings.accessListSync and GET /api/access-list-sources/status", ref.List.Name, ref.Source, why))
	}
	return out
}
