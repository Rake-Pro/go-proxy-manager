package api

import (
	"testing"
	"time"
)

// The "expiring" state follows the renewal rule: 30 days for a certificate
// that lives 90 days or longer, a third of the lifetime for a shorter one.
func TestCertStateIsLifetimeRelative(t *testing.T) {
	day := 24 * time.Hour
	issued := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		lifetime time.Duration
		left     time.Duration
		want     string
	}{
		{"90-day, 31 days left", 90 * day, 31 * day, CertStateValid},
		{"90-day, 30 days left", 90 * day, 30 * day, CertStateExpiring},
		{"one second short of 90 days, 30 days left", 90*day - time.Second, 30 * day, CertStateExpiring},
		{"one-year, 31 days left", 365 * day, 31 * day, CertStateValid},
		{"one-year, 29 days left", 365 * day, 29 * day, CertStateExpiring},
		{"6-day, fresh", 6 * day, 6 * day, CertStateValid},
		{"6-day, 3 days left", 6 * day, 3 * day, CertStateValid},
		{"6-day, 2 days left", 6 * day, 2 * day, CertStateExpiring},
		{"6-day, expired", 6 * day, 0, CertStateExpired},
		{"24-hour, 9 hours left", day, 9 * time.Hour, CertStateValid},
		{"24-hour, 8 hours left", day, 8 * time.Hour, CertStateExpiring},
	}
	for _, c := range cases {
		notAfter := issued.Add(c.lifetime)
		if got := certState(notAfter.Add(-c.left), issued, notAfter); got != c.want {
			t.Errorf("%s: state = %q, want %q", c.name, got, c.want)
		}
	}
	// A zero NotBefore (a custom certificate, which nothing renews) and a leaf
	// without a usable lifetime keep the fixed 30-day window.
	if got := certState(issued, time.Time{}, issued.Add(3*day)); got != CertStateExpiring {
		t.Errorf("zero NotBefore: state = %q, want expiring inside 30 days", got)
	}
	if got := certState(issued, time.Time{}, issued.Add(31*day)); got != CertStateValid {
		t.Errorf("zero NotBefore, 31 days left: state = %q, want valid", got)
	}
	if got := certState(issued, issued.Add(60*day), issued.Add(29*day)); got != CertStateExpiring {
		t.Errorf("no usable lifetime: state = %q, want expiring inside 30 days", got)
	}
}
