package acme

// ACME Renewal Information (ARI, RFC 9773): a CA that offers it publishes a
// renewalInfo URL in its directory, and answers GET <renewalInfo>/<certID>
// with the window in which it wants that certificate renewed. gpm polls it
// during the regular renewal pass and renews at a time chosen inside the
// window when that is earlier than the lifetime rule (see renewalTime).
//
// Only polling is implemented. A new order does not carry the "replaces" field
// that tells the CA which certificate it supersedes: x/crypto/acme has no way
// to send it.

import (
	"context"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// ARIState is a certificate's renewal information as last seen, kept in its
// Meta. Every time is UTC.
type ARIState struct {
	// WindowStart and WindowEnd are the CA's suggested renewal window.
	WindowStart time.Time `json:"windowStart,omitzero"`
	WindowEnd   time.Time `json:"windowEnd,omitzero"`
	// RenewAt is the time gpm chose inside the window, once per window.
	RenewAt time.Time `json:"renewAt,omitzero"`
	// LastChecked is when the CA last answered a poll successfully.
	LastChecked time.Time `json:"lastChecked,omitzero"`
	// NextCheck is when the next poll is due.
	NextCheck time.Time `json:"nextCheck,omitzero"`
	// ExplanationURL is the CA's page explaining the window, when it gave a
	// usable one.
	ExplanationURL string `json:"explanationURL,omitempty"`
}

const (
	// ariDefaultPoll is the poll interval when the CA sends no usable
	// Retry-After; ariMinPoll and ariMaxPoll clamp the one it sends.
	ariDefaultPoll = 6 * time.Hour
	ariMinPoll     = time.Hour
	ariMaxPoll     = 7 * 24 * time.Hour
	// ariRetryAfterFailure is the next check after a failed poll.
	ariRetryAfterFailure = time.Hour
	// ariMinAge caps the age (min(ariMinAge, lifetime/3)) a certificate must
	// reach before an ARI time can renew it.
	ariMinAge = 72 * time.Hour
	// ariHTTPTimeout bounds each renewal-information request (directory fetch
	// and poll). It is shorter than the ACME exchanges' timeout because the
	// polls run under the issuance lock: a hanging endpoint would otherwise
	// hold every order, RenewNow included, for 30s per certificate.
	ariHTTPTimeout = 10 * time.Second
	// maxExplanationURL bounds the stored explanation URL.
	maxExplanationURL = 512
)

// ariDirectory is one ACME directory's renewal-information endpoint as found
// in the current pass.
type ariDirectory struct {
	client   *http.Client
	endpoint string // empty: the CA offers no usable ARI endpoint
	// err is set when the client could not be built, the directory fetch
	// failed, or a poll against this directory failed at the transport level;
	// the remaining polls for the directory are skipped in this pass.
	err error
}

// ariPass caches directory lookups for one EnsureAll pass, so each directory
// is fetched at most once per pass however many certificates use it. The key
// includes the CA bundle, which decides the client the directory is fetched
// with.
type ariPass struct {
	timeout time.Duration
	dirs    map[string]*ariDirectory
}

func (m *Manager) newARIPass() ariPass {
	return ariPass{timeout: m.ariTimeout, dirs: map[string]*ariDirectory{}}
}

// ariTransportError marks a poll that failed before any HTTP answer arrived
// (connection refused, timeout), as opposed to an answer gpm could not use.
type ariTransportError struct{ err error }

func (e ariTransportError) Error() string { return e.err.Error() }
func (e ariTransportError) Unwrap() error { return e.err }

// directory returns spec's directory entry, fetching it on first use in the
// pass. Its client is the certificate's ACME client (the bundle's trust, no
// redirects, the body cap) with the shorter ariHTTPTimeout.
func (p ariPass) directory(ctx context.Context, spec *model.ACMESpec) *ariDirectory {
	dirURL := effectiveDirectoryURL(spec.DirectoryURL)
	key := dirURL + "\x00" + spec.CABundle
	if d, ok := p.dirs[key]; ok {
		return d
	}
	d := &ariDirectory{}
	p.dirs[key] = d
	base, err := acmeHTTPClientFor(spec)
	if err != nil {
		d.err = err
		return d
	}
	c := *base
	c.Timeout = p.timeout
	d.client = &c
	endpoint, err := fetchRenewalInfoURL(ctx, d.client, dirURL)
	if err != nil {
		d.err = err
		return d
	}
	if endpoint == "" {
		return d // the CA does not offer ARI
	}
	if !sameOrigin(endpoint, dirURL) {
		// The poll is unauthenticated, but it would still send gpm to a host
		// the operator did not configure.
		log.Warn().Str("directory", dirURL).Str("renewalInfo", endpoint).
			Msg("acme: the directory's renewalInfo URL is on another scheme or host than the directory; renewal information is not used")
		return d
	}
	d.endpoint = endpoint
	return d
}

// fetchRenewalInfoURL reads the renewalInfo URL out of an ACME directory;
// empty means the CA does not offer ARI.
func fetchRenewalInfoURL(ctx context.Context, client *http.Client, dirURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dirURL, nil)
	if err != nil {
		return "", err
	}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("directory answered %s", res.Status)
	}
	var dir struct {
		RenewalInfo string `json:"renewalInfo"`
	}
	if err := json.NewDecoder(res.Body).Decode(&dir); err != nil {
		return "", fmt.Errorf("decode directory: %w", err)
	}
	return strings.TrimSpace(dir.RenewalInfo), nil
}

// sameOrigin reports whether raw has the scheme and host (with port) of base.
func sameOrigin(raw, base string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	b, err := url.Parse(base)
	if err != nil {
		return false
	}
	return u.Scheme == b.Scheme && u.Host != "" && strings.EqualFold(u.Host, b.Host)
}

// pollARI refreshes cert's renewal information when its next check is due and
// saves it into meta (in memory and on disk). Nothing here fails the pass or
// counts as a renewal failure: a CA without ARI is skipped, and a failed poll
// keeps the previous state and is retried an hour later.
func (m *Manager) pollARI(ctx context.Context, pass ariPass, cert model.Certificate, meta *Meta, now time.Time) {
	if meta.ARI != nil && now.Before(meta.ARI.NextCheck) {
		return
	}
	d := pass.directory(ctx, cert.ACME)
	if d.err != nil {
		log.Debug().Str("cert", cert.Name).Err(d.err).Msg("acme: renewal information not checked")
		return
	}
	if d.endpoint == "" {
		return
	}
	leaf, err := readStoredLeaf(m.certDir, cert.Name)
	if err != nil {
		log.Debug().Str("cert", cert.Name).Err(err).Msg("acme: renewal information not checked")
		return
	}
	id, err := ariCertID(leaf)
	if err != nil {
		log.Debug().Str("cert", cert.Name).Err(err).Msg("acme: renewal information not checked")
		return
	}
	var prev ARIState
	if meta.ARI != nil {
		prev = *meta.ARI
	}
	next, err := pollRenewalInfo(ctx, d.client, d.endpoint, id, prev, leaf, now)
	if err != nil {
		log.Debug().Str("cert", cert.Name).Err(err).Msg("acme: renewal information poll failed; keeping the previous state")
		if errors.As(err, new(ariTransportError)) {
			// The endpoint is unreachable or hanging: do not wait on it again
			// for every other certificate of this directory in this pass.
			d.err = err
		}
		next = prev
		next.NextCheck = now.Add(ariRetryAfterFailure).UTC()
	}
	meta.ARI = &next
	if err := saveMeta(m.certDir, cert.Name, *meta); err != nil {
		log.Warn().Str("cert", cert.Name).Err(err).Msg("acme: could not save renewal information")
	}
}

// pollRenewalInfo asks the CA for one certificate's renewal window and returns
// the new state. The window is clamped to the leaf's validity (a renewal
// before NotBefore or after NotAfter means nothing, and a time outside it
// could be one JSON cannot store); a window wholly in the past but inside the
// validity is kept, it means "renew now". The renewal time is drawn uniformly
// inside the window once: a poll that returns the window already seen keeps
// the time chosen for it, a changed window draws again.
func pollRenewalInfo(ctx context.Context, client *http.Client, endpoint, certID string, prev ARIState, leaf *x509.Certificate, now time.Time) (ARIState, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(endpoint, "/")+"/"+certID, nil)
	if err != nil {
		return ARIState{}, err
	}
	res, err := client.Do(req)
	if err != nil {
		return ARIState{}, ariTransportError{err}
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, res.Body)
		return ARIState{}, fmt.Errorf("renewal information answered %s", res.Status)
	}
	var body struct {
		SuggestedWindow struct {
			Start time.Time `json:"start"`
			End   time.Time `json:"end"`
		} `json:"suggestedWindow"`
		ExplanationURL string `json:"explanationURL"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return ARIState{}, fmt.Errorf("decode renewal information: %w", err)
	}
	start, end := body.SuggestedWindow.Start.UTC(), body.SuggestedWindow.End.UTC()
	if notBefore := leaf.NotBefore.UTC(); start.Before(notBefore) {
		start = notBefore
	}
	if notAfter := leaf.NotAfter.UTC(); end.After(notAfter) {
		end = notAfter
	}
	if !end.After(start) {
		return ARIState{}, errors.New("renewal information window does not end after it starts within the certificate's validity")
	}
	next := ARIState{
		WindowStart:    start,
		WindowEnd:      end,
		LastChecked:    now.UTC(),
		NextCheck:      now.Add(ariPollInterval(res.Header.Get("Retry-After"))).UTC(),
		ExplanationURL: explanationURL(body.ExplanationURL),
	}
	if !prev.RenewAt.IsZero() && prev.WindowStart.Equal(start) && prev.WindowEnd.Equal(end) {
		next.RenewAt = prev.RenewAt
	} else {
		next.RenewAt = start.Add(time.Duration(rand.Int64N(int64(end.Sub(start)))))
	}
	return next, nil
}

// ariPollInterval turns a Retry-After header (in seconds) into the time to the
// next poll: ariDefaultPoll when absent or unparsable, otherwise clamped to
// [ariMinPoll, ariMaxPoll].
func ariPollInterval(retryAfter string) time.Duration {
	secs, err := strconv.ParseInt(strings.TrimSpace(retryAfter), 10, 64)
	if err != nil || secs < 0 {
		return ariDefaultPoll
	}
	if secs > int64(ariMaxPoll/time.Second) {
		return ariMaxPoll
	}
	return max(time.Duration(secs)*time.Second, ariMinPoll)
}

// explanationURL keeps a CA's explanation link only when it is an http(s) URL
// of at most maxExplanationURL bytes; anything else is dropped.
func explanationURL(raw string) string {
	if raw == "" || len(raw) > maxExplanationURL {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return raw
}

// ariCertID is the RFC 9773 certificate identifier: the base64url (unpadded)
// authority key identifier, a dot, and the base64url (unpadded) DER content of
// the serial number.
func ariCertID(leaf *x509.Certificate) (string, error) {
	if len(leaf.AuthorityKeyId) == 0 {
		return "", errors.New("the certificate has no authority key identifier")
	}
	return certIDFrom(leaf.AuthorityKeyId, leaf.SerialNumber)
}

// certIDFrom builds the identifier from its two parts. The serial is encoded
// as the content octets of its DER INTEGER, so a serial whose high bit is set
// gains the leading 0x00 byte DER requires.
func certIDFrom(aki []byte, serial *big.Int) (string, error) {
	der, err := asn1.Marshal(serial)
	if err != nil {
		return "", err
	}
	var v asn1.RawValue
	if _, err := asn1.Unmarshal(der, &v); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(aki) + "." + base64.RawURLEncoding.EncodeToString(v.Bytes), nil
}

// readStoredLeaf parses the leaf of an issued certificate's stored chain.
func readStoredLeaf(certDir, name string) (*x509.Certificate, error) {
	b, err := os.ReadFile(IssuedCertPath(certDir, name)) //nolint:gosec // G304: path is certDir/acme/issued/<name>/fullchain.pem; name passed model.ValidateName
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, errors.New("stored certificate chain is not PEM")
	}
	return x509.ParseCertificate(block.Bytes)
}
