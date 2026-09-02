package store

import (
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// upsert replaces an object with the same name, or appends it. When it
// replaces, it mutates list[i] in place rather than copying - list and its
// caller's slice share the same backing array (withObject's "c := *cfg" is a
// shallow copy), so this also mutates whatever the caller's own slice sees at
// that index. Every withObject caller that still needs the pre-upsert value
// (CreatedAt preservation, an ownership guard) already captures it before
// calling withObject for exactly this reason - see the "Captured BEFORE
// withObject" comments in configstore.go's Save/SaveBatch/ApplyBatch.
func upsert[T model.Object](list []T, item T) []T {
	name := item.GetMeta().Name
	for i := range list {
		if list[i].GetMeta().Name == name {
			list[i] = item
			return list
		}
	}
	return append(list, item)
}

// withObject returns a copy of cfg with obj inserted or replaced in the
// appropriate slice, used to validate the whole graph before a write.
func withObject(cfg *model.Config, obj model.Object) model.Config {
	c := *cfg
	switch o := obj.(type) {
	case model.ProxyHost:
		c.ProxyHosts = upsert(c.ProxyHosts, o)
	case model.RedirectHost:
		c.RedirectHosts = upsert(c.RedirectHosts, o)
	case model.StreamHost:
		c.StreamHosts = upsert(c.StreamHosts, o)
	case model.ParkedHost:
		c.ParkedHosts = upsert(c.ParkedHosts, o)
	case model.Certificate:
		c.Certificates = upsert(c.Certificates, o)
	case model.ClientCA:
		c.ClientCAs = upsert(c.ClientCAs, o)
	case model.DNSProvider:
		c.DNSProviders = upsert(c.DNSProviders, o)
	case model.IdentityProvider:
		c.IdentityProviders = upsert(c.IdentityProviders, o)
	case model.UpstreamGroup:
		c.UpstreamGroups = upsert(c.UpstreamGroups, o)
	case model.AccessList:
		c.AccessLists = upsert(c.AccessLists, o)
	case model.Middleware:
		c.Middlewares = upsert(c.Middlewares, o)
	case model.APIToken:
		c.APITokens = upsert(c.APITokens, o)
	}
	return c
}

// withoutObject returns a copy of cfg with the named object of the given kind
// removed, used to validate that no dangling references remain before deleting.
func withoutObject(cfg *model.Config, kind, name string) model.Config {
	c := *cfg
	switch kind {
	case "ProxyHost":
		c.ProxyHosts = dropNamed(c.ProxyHosts, name)
	case "RedirectHost":
		c.RedirectHosts = dropNamed(c.RedirectHosts, name)
	case "StreamHost":
		c.StreamHosts = dropNamed(c.StreamHosts, name)
	case "ParkedHost":
		c.ParkedHosts = dropNamed(c.ParkedHosts, name)
	case "Certificate":
		c.Certificates = dropNamed(c.Certificates, name)
	case "ClientCA":
		c.ClientCAs = dropNamed(c.ClientCAs, name)
	case "DNSProvider":
		c.DNSProviders = dropNamed(c.DNSProviders, name)
	case "IdentityProvider":
		c.IdentityProviders = dropNamed(c.IdentityProviders, name)
	case "UpstreamGroup":
		c.UpstreamGroups = dropNamed(c.UpstreamGroups, name)
	case "AccessList":
		c.AccessLists = dropNamed(c.AccessLists, name)
	case "Middleware":
		c.Middlewares = dropNamed(c.Middlewares, name)
	case "APIToken":
		c.APITokens = dropNamed(c.APITokens, name)
	}
	return c
}

func dropNamed[T model.Object](list []T, name string) []T {
	out := make([]T, 0, len(list))
	for _, o := range list {
		if o.GetMeta().Name != name {
			out = append(out, o)
		}
	}
	return out
}

// stampMeta computes the CreatedAt/UpdatedAt to write for an object being
// saved. UpdatedAt is always stamped to now.
//
// CreatedAt is server-managed, not client-managed: if existing is non-nil (an
// object with this name already exists), its stored CreatedAt wins outright
// and any CreatedAt on the incoming object is ignored - a PUT can never
// rewrite an object's creation time, whether the client omitted the field (as
// the web UI does, which is what makes this matter) or supplied a different
// one. The one exception is an existing object whose own CreatedAt is zero
// (written before this field existed): that gets backfilled to now rather
// than preserved as zero. For a genuinely new object, the incoming CreatedAt
// is honoured if set (e.g. a batch import restoring timestamps), otherwise it
// is stamped to now.
func stampMeta(m model.ObjectMeta, existing *model.ObjectMeta, now time.Time) model.ObjectMeta {
	switch {
	case existing != nil && !existing.CreatedAt.IsZero():
		m.CreatedAt = existing.CreatedAt
	case existing != nil:
		m.CreatedAt = now
	case m.CreatedAt.IsZero():
		m.CreatedAt = now
	}
	m.UpdatedAt = now
	return m
}

// stampTimes returns obj with its CreatedAt/UpdatedAt maintained for writing.
// existing is the object currently on disk under the same kind and name, or
// nil if this is a new object; see stampMeta for how it governs CreatedAt.
func stampTimes(obj model.Object, existing model.Object, now time.Time) any {
	var existingMeta *model.ObjectMeta
	if existing != nil {
		m := existing.GetMeta()
		existingMeta = &m
	}
	switch o := obj.(type) {
	case model.ProxyHost:
		o.ObjectMeta = stampMeta(o.ObjectMeta, existingMeta, now)
		return o
	case model.RedirectHost:
		o.ObjectMeta = stampMeta(o.ObjectMeta, existingMeta, now)
		return o
	case model.StreamHost:
		o.ObjectMeta = stampMeta(o.ObjectMeta, existingMeta, now)
		return o
	case model.ParkedHost:
		o.ObjectMeta = stampMeta(o.ObjectMeta, existingMeta, now)
		return o
	case model.Certificate:
		o.ObjectMeta = stampMeta(o.ObjectMeta, existingMeta, now)
		return o
	case model.ClientCA:
		o.ObjectMeta = stampMeta(o.ObjectMeta, existingMeta, now)
		return o
	case model.DNSProvider:
		o.ObjectMeta = stampMeta(o.ObjectMeta, existingMeta, now)
		return o
	case model.IdentityProvider:
		o.ObjectMeta = stampMeta(o.ObjectMeta, existingMeta, now)
		return o
	case model.UpstreamGroup:
		o.ObjectMeta = stampMeta(o.ObjectMeta, existingMeta, now)
		return o
	case model.AccessList:
		o.ObjectMeta = stampMeta(o.ObjectMeta, existingMeta, now)
		return o
	case model.Middleware:
		o.ObjectMeta = stampMeta(o.ObjectMeta, existingMeta, now)
		return o
	case model.APIToken:
		o.ObjectMeta = stampMeta(o.ObjectMeta, existingMeta, now)
		return o
	}
	return obj
}
