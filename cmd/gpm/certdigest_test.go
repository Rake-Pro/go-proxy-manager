package main

import (
	"strings"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/acme"
)

// The daily digest lists a certificate as expiring only when its expiry is
// inside the threshold and its renewal is already due (or unknown): a healthy
// short-lived certificate always expires inside the threshold.
func TestCertDigestLists(t *testing.T) {
	day := 24 * time.Hour
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(14 * day)
	obs := []acme.CertObservation{
		{Name: "healthy-90d", NotAfter: now.Add(60 * day), RenewAt: now.Add(30 * day)},
		{Name: "stuck-90d", NotAfter: now.Add(10 * day), RenewAt: now.Add(-20 * day)},
		{Name: "healthy-6d", NotAfter: now.Add(4 * day), RenewAt: now.Add(2 * day)},
		{Name: "stuck-6d", NotAfter: now.Add(1 * day), RenewAt: now.Add(-1 * day)},
		{Name: "due-now", NotAfter: now.Add(2 * day), RenewAt: now},
		{Name: "unknown-renewal", NotAfter: now.Add(3 * day)},
		{Name: "expired", NotAfter: now.Add(-1 * day), RenewAt: now.Add(-31 * day)},
		{Name: "never-issued"},
	}
	expiring, expired := certDigestLists(obs, now, cutoff)
	names := func(list []string) string {
		var out []string
		for _, l := range list {
			out = append(out, strings.Fields(l)[0])
		}
		return strings.Join(out, ",")
	}
	if got := names(expiring); got != "due-now,stuck-6d,stuck-90d,unknown-renewal" {
		t.Errorf("expiring = %s", got)
	}
	if got := names(expired); got != "expired" {
		t.Errorf("expired = %s", got)
	}
}
