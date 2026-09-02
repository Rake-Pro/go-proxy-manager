// Package k8s discovers annotated Kubernetes Ingress objects and reconciles them
// into gpm-managed proxy hosts, which then feed the existing DNS sync. It is the
// phase-2 half of the DNS story: phase 1 publishes records for hosts that opted
// in, phase 2 removes the manual step of hand-entering a cluster service as a
// host first.
//
// Two properties define the design (the same pair internal/dnssync is built on):
//
//   - Reconcile is FULL-STATE, not event-driven. The desired set is recomputed
//     from a complete list of annotated Ingresses on every poll and compared with
//     what the config actually holds, so drift is repaired in both directions and
//     a missed change is impossible by construction.
//   - Writes are OWNERSHIP-GATED. Only proxy hosts carrying the label
//     gpm.rake.pro/managed-by: ingress-discovery are ever created, updated or
//     deleted. A hand-written host with the same name is skipped with a warning,
//     never overwritten - the rule the DNS backends already apply to records.
//
// The client is deliberately plain net/http + encoding/json against the
// Kubernetes REST API. client-go and its transitive tree would dwarf this
// project's entire direct dependency set, which is the thing the project exists
// to avoid. gpm never writes to the cluster: the client only ever LISTs, and the
// shipped RBAC grants exactly that verb on ingresses and nothing else.
//
// See design/ingress-discovery.md for the decision record.
package k8s

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

const (
	// ingressesPath is the collection endpoint for cluster-wide listing.
	ingressesPath = "/apis/networking.k8s.io/v1/ingresses"
	// listPageSize bounds one page of a paginated LIST. It is deliberately small
	// relative to maxRespBody: a real Ingress carries managedFields and arbitrary
	// annotations and runs to tens of kilobytes, so a large page is one verbose
	// tenant manifest away from overflowing the body cap and freezing discovery.
	listPageSize = 100
	// listKind is the kind a LIST of ingresses must announce. A 200 whose body is
	// not an IngressList is an ERROR, never an empty list - see ListIngresses.
	listKind = "IngressList"
	// maxListPages / maxListItems bound a paginated LIST so a misbehaving or
	// hostile endpoint cannot stream unbounded data into memory. Exceeding either
	// is an ERROR, never a silent truncation: a truncated list read as complete is
	// exactly the input that would make the reconciler delete managed hosts it
	// should have kept.
	maxListPages = 100
	maxListItems = 10000
	// maxRespBody caps how much of one response is read.
	maxRespBody = 8 << 20
	// tokenTTL is how long a bearer token read from disk is reused before being
	// re-read. Projected ServiceAccount tokens rotate (typically hourly), so a
	// value cached for the process lifetime would start failing mid-run.
	tokenTTL = 5 * time.Minute
)

// Ingress is the subset of a networking.k8s.io/v1 Ingress this package consumes.
// Anything not listed here is deliberately not decoded: the derived proxy host
// takes everything security-relevant from the operator's template, so there is
// nothing else in an Ingress gpm is allowed to act on.
type Ingress struct {
	Metadata struct {
		Name        string            `json:"name"`
		Namespace   string            `json:"namespace"`
		Annotations map[string]string `json:"annotations"`
		// Labels is decoded so settings.ingressDiscovery.profileRules can match on
		// them (operator-side profile selection). It contributes to profile
		// SELECTION only, never to the derived host's own object metadata - the
		// same containment as everything else that comes off an untrusted
		// Ingress.
		Labels map[string]string `json:"labels"`
	} `json:"metadata"`
	Spec struct {
		Rules []struct {
			Host string `json:"host"`
		} `json:"rules"`
		// TLS is decoded because it is part of the object, but it is NOT
		// authoritative: it selects no certificate and contributes no domain (see
		// the design doc). Its only use is a debug log for a manifest that declares
		// TLS hosts its rules never mention.
		TLS []struct {
			Hosts      []string `json:"hosts"`
			SecretName string   `json:"secretName"`
		} `json:"tls"`
	} `json:"spec"`
}

// ingressList is one page of a LIST response.
//
// Kind and Items are shape assertions, not conveniences. Decoding a 200 into a
// plain struct accepts `null`, `{}`, a Status object and `{"items":null}` alike,
// all of which yield zero items and a nil error - which the reconciler would read
// as "the cluster has no annotated Ingresses" and act on by deleting every
// managed host. Items is a POINTER so an absent/null items field is
// distinguishable from an empty one.
type ingressList struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Continue string `json:"continue"`
	} `json:"metadata"`
	Items *[]Ingress `json:"items"`
}

// statusError carries the API server's own error reply for a non-2xx response.
type statusError struct {
	Code    int
	Message string
}

func (e *statusError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("kubernetes: unexpected status %d", e.Code)
	}
	return fmt.Sprintf("kubernetes: status %d: %s", e.Code, e.Message)
}

// ClientConfig is the connection configuration. An empty APIURL/TokenFile/CAFile
// falls back to the in-cluster ServiceAccount values, so the same struct covers
// both the in-cluster deployment and the off-cluster one (which is the real
// deployment here: gpm runs on the edge host, not as a pod).
type ClientConfig struct {
	APIURL        string
	TokenFile     string
	CAFile        string
	Namespace     string
	LabelSelector string
}

// Client is a minimal, read-only Kubernetes REST client.
//
// The HTTP client is hardened the same way internal/dnssync's is: redirects are
// never followed (an API server that 302s is a misconfiguration or an attack,
// never something to chase with a bearer token attached), link-local
// destinations are refused at connect time so a mistyped API URL cannot reach a
// cloud metadata service, TLS is verified against the supplied CA with no
// skip-verify escape hatch, and every read is bounded.
type Client struct {
	base          string
	tokenFile     string
	namespace     string
	labelSelector string
	http          *http.Client

	mu      sync.Mutex
	token   string
	tokenAt time.Time
}

// InClusterConfig fills empty fields from the in-cluster environment: the API
// endpoint from KUBERNETES_SERVICE_HOST/PORT and the projected ServiceAccount
// token/CA paths. It reports an error only when APIURL is empty AND the
// environment does not describe an in-cluster endpoint.
func InClusterConfig(c ClientConfig) (ClientConfig, error) {
	if c.APIURL == "" {
		host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
		if host == "" || port == "" {
			return c, fmt.Errorf("kubernetes: no apiURL configured and no in-cluster endpoint in the environment (KUBERNETES_SERVICE_HOST/KUBERNETES_SERVICE_PORT)")
		}
		c.APIURL = "https://" + net.JoinHostPort(host, port)
	}
	if c.TokenFile == "" {
		c.TokenFile = model.DefaultKubernetesTokenFile
	}
	if c.CAFile == "" {
		c.CAFile = model.DefaultKubernetesCAFile
	}
	return c, nil
}

// NewClient builds a client for cfg. The CA bundle is read here, so a missing or
// unparseable bundle fails loudly at construction rather than becoming a
// verification error on every poll.
func NewClient(cfg ClientConfig) (*Client, error) {
	full, err := InClusterConfig(cfg)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(full.APIURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, fmt.Errorf("kubernetes: apiURL must be an absolute https URL, got %q", full.APIURL)
	}
	if full.TokenFile == "" {
		return nil, fmt.Errorf("kubernetes: no tokenFile configured")
	}
	pool := x509.NewCertPool()
	pem, err := os.ReadFile(full.CAFile)
	if err != nil {
		return nil, fmt.Errorf("kubernetes: read caFile %q: %w", full.CAFile, err)
	}
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("kubernetes: caFile %q contains no usable PEM certificate", full.CAFile)
	}
	return &Client{
		base:          strings.TrimRight(full.APIURL, "/"),
		tokenFile:     full.TokenFile,
		namespace:     full.Namespace,
		labelSelector: full.LabelSelector,
		http: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   5 * time.Second,
					KeepAlive: 30 * time.Second,
					Control:   refuseLinkLocal,
				}).DialContext,
				TLSClientConfig: &tls.Config{
					RootCAs:    pool,
					MinVersion: tls.VersionTLS12,
				},
				MaxIdleConns:        2,
				IdleConnTimeout:     60 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
			},
		},
	}, nil
}

func refuseLinkLocal(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()) {
		return fmt.Errorf("kubernetes: link-local destination %s refused", ip)
	}
	return nil
}

// bearer returns the current token, re-reading it from disk when the cached copy
// is older than tokenTTL. Projected ServiceAccount tokens are rotated in place by
// the kubelet, so a token read once at startup starts returning 401 after its
// lifetime expires; re-reading on a TTL is what makes an unattended daemon keep
// working across a rotation.
func (c *Client) bearer() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Since(c.tokenAt) < tokenTTL {
		return c.token, nil
	}
	b, err := os.ReadFile(c.tokenFile)
	if err != nil {
		c.token = ""
		return "", fmt.Errorf("kubernetes: read tokenFile %q: %w", c.tokenFile, err)
	}
	tok := strings.TrimSpace(string(b))
	if tok == "" {
		c.token = ""
		return "", fmt.Errorf("kubernetes: tokenFile %q is empty", c.tokenFile)
	}
	// A token is carried in a header, so a stray newline or control character
	// would be a header-injection vector rather than a harmless typo.
	if strings.ContainsAny(tok, "\r\n\x00") {
		c.token = ""
		return "", fmt.Errorf("kubernetes: tokenFile %q contains control characters", c.tokenFile)
	}
	c.token, c.tokenAt = tok, time.Now()
	return tok, nil
}

// dropToken forces the next call to re-read the token file. It is called on a
// 401 so a rotation that happened inside the TTL window is picked up on the next
// poll instead of waiting the TTL out.
func (c *Client) dropToken() {
	c.mu.Lock()
	c.token = ""
	c.mu.Unlock()
}

// get performs one authenticated GET and decodes the JSON body into out.
func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	token, err := c.bearer()
	if err != nil {
		return err
	}
	full := c.base + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return fmt.Errorf("kubernetes: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("kubernetes: GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	// Read one byte past the cap so an oversized response is reported as such
	// rather than surfacing as an opaque JSON syntax error on a truncated body.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRespBody+1))
	if err != nil {
		return fmt.Errorf("kubernetes: GET %s: read body: %w", path, err)
	}
	if len(body) > maxRespBody {
		return fmt.Errorf("kubernetes: GET %s: response exceeded the %d-byte body cap (lower the page size, or trim oversized annotations on the listed objects)", path, maxRespBody)
	}
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized {
			c.dropToken()
		}
		return &statusError{Code: resp.StatusCode, Message: apiErrorMessage(body)}
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("kubernetes: GET %s: decode response: %w", path, err)
	}
	return nil
}

// apiErrorMessage pulls the message out of a Kubernetes Status reply, falling
// back to a bounded excerpt of whatever arrived.
func apiErrorMessage(body []byte) string {
	var st struct {
		Message string `json:"message"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal(body, &st); err == nil && st.Message != "" {
		return st.Message
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}

// ListIngresses returns every Ingress the configured scope exposes, following
// pagination to exhaustion.
//
// The contract that the reconciler's freeze behaviour depends on: a non-nil
// error NEVER comes with items, and a nil error ALWAYS means a complete list.
// If any page fails - transport, status, decode - the pages already accumulated
// are discarded and the error is returned, because a partial list read as
// complete is precisely what would make the reconciler delete managed hosts that
// should have been kept. An empty slice with a nil error is a legitimate "no
// Ingresses", which is a different thing entirely and is never produced by a
// failure path.
func (c *Client) ListIngresses(ctx context.Context) ([]Ingress, error) {
	path := ingressesPath
	if c.namespace != "" {
		path = "/apis/networking.k8s.io/v1/namespaces/" + url.PathEscape(c.namespace) + "/ingresses"
	}
	var out []Ingress
	cont := ""
	for page := 1; ; page++ {
		if page > maxListPages {
			return nil, fmt.Errorf("kubernetes: ingress list exceeded %d pages; refusing to treat a truncated list as complete", maxListPages)
		}
		q := url.Values{}
		q.Set("limit", fmt.Sprint(listPageSize))
		if c.labelSelector != "" {
			q.Set("labelSelector", c.labelSelector)
		}
		if cont != "" {
			q.Set("continue", cont)
		}
		var list ingressList
		if err := c.get(ctx, path, q, &list); err != nil {
			return nil, err
		}
		// Shape assertion. A LIST reply from a Kubernetes API server always carries
		// kind: IngressList; anything else on a 200 - a Status envelope, a mesh or
		// gateway response, an unrelated HTTPS service sharing the internal CA, or a
		// bare null - is a MISDIRECTED request, not an empty cluster. Erroring here
		// keeps it on the freeze path instead of letting it delete managed hosts.
		if list.Kind != listKind {
			return nil, fmt.Errorf("kubernetes: GET %s returned kind %q, want %q; refusing to treat a non-IngressList response as an empty list (check apiURL, namespace and labelSelector)", path, list.Kind, listKind)
		}
		if list.Items == nil {
			return nil, fmt.Errorf("kubernetes: GET %s returned an %s with no items field; refusing to treat it as an empty list", path, listKind)
		}
		out = append(out, *list.Items...)
		if len(out) > maxListItems {
			return nil, fmt.Errorf("kubernetes: ingress list exceeded %d items; refusing to treat a truncated list as complete", maxListItems)
		}
		cont = list.Metadata.Continue
		if cont == "" {
			return out, nil
		}
	}
}
