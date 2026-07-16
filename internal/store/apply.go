package store

import (
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// upsert replaces an object with the same name, or appends it.
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
	case model.DeadHost:
		c.DeadHosts = upsert(c.DeadHosts, o)
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
	case "DeadHost":
		c.DeadHosts = dropNamed(c.DeadHosts, name)
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

func stampMeta(m model.ObjectMeta, now time.Time) model.ObjectMeta {
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now
	return m
}

// stampTimes returns obj with its CreatedAt/UpdatedAt maintained for writing.
func stampTimes(obj model.Object, now time.Time) any {
	switch o := obj.(type) {
	case model.ProxyHost:
		o.ObjectMeta = stampMeta(o.ObjectMeta, now)
		return o
	case model.RedirectHost:
		o.ObjectMeta = stampMeta(o.ObjectMeta, now)
		return o
	case model.StreamHost:
		o.ObjectMeta = stampMeta(o.ObjectMeta, now)
		return o
	case model.DeadHost:
		o.ObjectMeta = stampMeta(o.ObjectMeta, now)
		return o
	case model.Certificate:
		o.ObjectMeta = stampMeta(o.ObjectMeta, now)
		return o
	case model.ClientCA:
		o.ObjectMeta = stampMeta(o.ObjectMeta, now)
		return o
	case model.DNSProvider:
		o.ObjectMeta = stampMeta(o.ObjectMeta, now)
		return o
	case model.IdentityProvider:
		o.ObjectMeta = stampMeta(o.ObjectMeta, now)
		return o
	case model.UpstreamGroup:
		o.ObjectMeta = stampMeta(o.ObjectMeta, now)
		return o
	case model.AccessList:
		o.ObjectMeta = stampMeta(o.ObjectMeta, now)
		return o
	case model.Middleware:
		o.ObjectMeta = stampMeta(o.ObjectMeta, now)
		return o
	}
	return obj
}
