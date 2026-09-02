package acme

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// ACMEDNSConfig configures an ACMEDNSSolver. The four credential fields are the
// exact values an acme-dns /register call returns.
type ACMEDNSConfig struct {
	BaseURL   string // e.g. https://acme-dns.example.com
	Username  string // "username" from /register (a UUID)
	Password  string // "password" from /register
	Subdomain string // "subdomain" from /register (a UUID)
	// AllowInsecureLocal permits a BaseURL whose host is a loopback literal, for
	// an acme-dns running beside gpm. Without it such a URL is refused, along
	// with link-local and other non-destinations: this client carries the
	// account credentials in custom headers, so where it points matters.
	AllowInsecureLocal bool
}

// cnameLookuper resolves CNAMEs; *net.Resolver satisfies it. The seam keeps the
// pre-flight check hermetic in tests.
type cnameLookuper interface {
	LookupCNAME(ctx context.Context, host string) (string, error)
}

// ACMEDNSSolver implements DNSSolver against an acme-dns server
// (github.com/joohoi/acme-dns). acme-dns is a tiny DNS server that holds exactly
// one TXT record per registered account, so it is the escape hatch for a provider
// with no API at all: the operator delegates
//
//	_acme-challenge.example.com.  CNAME  <subdomain>.acme-dns.example.com.
//
// once, by hand, and from then on gpm only ever writes to acme-dns. The
// credentials grant no authority over the real zone, which is the point.
type ACMEDNSSolver struct {
	api       *restAPI
	subdomain string
	resolver  cnameLookuper
}

// ACMEDNSOption configures an ACMEDNSSolver.
type ACMEDNSOption func(*ACMEDNSSolver)

// WithACMEDNSHTTPClient overrides the HTTP client used for API calls.
func WithACMEDNSHTTPClient(c *http.Client) ACMEDNSOption {
	return func(s *ACMEDNSSolver) { s.api.client = c }
}

// WithACMEDNSResolver overrides the resolver used for the CNAME pre-flight check.
func WithACMEDNSResolver(r cnameLookuper) ACMEDNSOption {
	return func(s *ACMEDNSSolver) { s.resolver = r }
}

// NewACMEDNSSolver validates cfg and builds a solver. It performs no network I/O.
func NewACMEDNSSolver(cfg ACMEDNSConfig, opts ...ACMEDNSOption) (*ACMEDNSSolver, error) {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		return nil, errors.New("acme-dns: baseURL is required")
	}
	if err := model.ValidateOutboundBaseURL("acme-dns: baseURL", base, cfg.AllowInsecureLocal); err != nil {
		return nil, err
	}
	user := strings.TrimSpace(cfg.Username)
	if user == "" {
		return nil, errors.New("acme-dns: username is required")
	}
	pass := strings.TrimSpace(cfg.Password)
	if pass == "" {
		return nil, errors.New("acme-dns: password is required")
	}
	sub := strings.TrimSpace(cfg.Subdomain)
	if sub == "" {
		return nil, errors.New("acme-dns: subdomain is required")
	}
	s := &ACMEDNSSolver{
		api: newRESTAPI("acme-dns", base, func(r *http.Request) {
			r.Header.Set("X-Api-User", user)
			r.Header.Set("X-Api-Key", pass)
		}),
		subdomain: sub,
		resolver:  &net.Resolver{},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// acmeDNSUpdate is the POST /update body acme-dns expects.
type acmeDNSUpdate struct {
	Subdomain string `json:"subdomain"`
	TXT       string `json:"txt"`
}

// Present writes the challenge value into the acme-dns account. acme-dns keeps
// the last two values per account, so an apex + wildcard order that shares one
// challenge name works without any read-modify-write here.
func (s *ACMEDNSSolver) Present(ctx context.Context, name, value string) error {
	s.checkDelegation(ctx, name)
	if err := s.api.do(ctx, http.MethodPost, "/update", acmeDNSUpdate{Subdomain: s.subdomain, TXT: value}, nil); err != nil {
		if statusIs(err, http.StatusUnauthorized) {
			return fmt.Errorf("acme-dns: credentials rejected for subdomain %s (check username/password, and that the account is not restricted by allowfrom): %w", s.subdomain, err)
		}
		return err
	}
	return nil
}

// CleanUp is a no-op: an acme-dns account holds a fixed-size ring of TXT values
// with no delete endpoint, and the next issuance overwrites them.
func (s *ACMEDNSSolver) CleanUp(ctx context.Context, name, value string) error {
	return nil
}

// checkDelegation is a best-effort pre-flight: it warns when
// _acme-challenge.<domain> is not CNAMEd into this acme-dns account, which is the
// one manual step an operator has to do and the one that is easy to forget. It
// never fails issuance - a resolver that cannot see the record yet is not proof
// the delegation is missing.
func (s *ACMEDNSSolver) checkDelegation(ctx context.Context, name string) {
	if s.resolver == nil {
		return
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cname, err := s.resolver.LookupCNAME(lookupCtx, name)
	if err != nil {
		log.Warn().Str("record", name).Err(err).
			Msg("acme-dns: could not check the challenge CNAME; make sure it points at this acme-dns account")
		return
	}
	target := strings.TrimSuffix(strings.TrimSpace(cname), ".")
	label, _, _ := strings.Cut(target, ".")
	if strings.EqualFold(label, s.subdomain) {
		return
	}
	log.Warn().Str("record", name).Str("cname", target).Str("subdomain", s.subdomain).
		Msg("acme-dns: challenge name is not delegated to this acme-dns account; add a CNAME from it to <subdomain>.<acme-dns zone>")
}
