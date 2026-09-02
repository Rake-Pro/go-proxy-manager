// Package docker discovers labelled Docker containers and reconciles them into
// gpm-managed proxy hosts, which then feed the existing DNS sync. It is the
// container-native twin of internal/k8s: same opt-in-by-label contract, same
// operator-written templates and profiles, same ownership-gated writes, and the
// same shared full-state planner (internal/discovery). Only the source differs.
//
// The client is deliberately plain net/http + encoding/json against the Docker
// Engine API. The official SDK pulls a large transitive tree for what is, for
// gpm's purposes, two GETs; minimising the dependency and advisory surface is
// the point of this project.
//
// gpm never writes to the Engine: the only requests this file can issue are GET
// /version, GET /containers/json and GET /events. There is no code path here
// that can create, start, stop or exec anything, which is what makes a
// read-only socket proxy (see docs/how-to/docker-discovery.md) an exact fit
// rather than a
// downgrade.
//
// See design/docker-discovery.md for the decision record.
package docker

import (
	"bufio"
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
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	// preferredAPIVersion is the LOWEST Engine API version that carries
	// everything discovery reads: label filters on /containers/json, the
	// per-network IPAddress map, and container-scoped event filters. Asking for
	// the oldest sufficient version is what keeps gpm working against an older
	// daemon (and an older socket proxy) instead of demanding whatever is newest.
	preferredAPIVersion = "1.41"
	// maxRespBody caps how much of one response is read. A container list carries
	// every label of every container, so it is generous but still bounded.
	maxRespBody = 8 << 20
	// maxContainers bounds one list. Exceeding it is an ERROR, never a silent
	// truncation: a truncated list read as complete is exactly the input that
	// would make the reconciler delete managed hosts it should have kept.
	maxContainers = 5000
	// maxEventLine bounds one line of the event stream, so a hostile or
	// misbehaving endpoint cannot stream one unbounded "line" into memory.
	maxEventLine = 1 << 20
	// versionTTL is how long a negotiated API version is reused. Re-negotiating
	// occasionally is what picks up a daemon upgrade without a gpm restart.
	versionTTL = 30 * time.Minute
)

// Container is the subset of a /containers/json entry this package consumes.
// Anything not listed here is deliberately not decoded: the derived proxy host
// takes everything security-relevant from the operator's template, so there is
// nothing else about a container gpm is allowed to act on.
type Container struct {
	ID    string   `json:"Id"`
	Names []string `json:"Names"`
	// State is "running", "exited", ... Only used to explain a skip.
	State  string            `json:"State"`
	Labels map[string]string `json:"Labels"`
	Ports  []Port            `json:"Ports"`

	NetworkSettings struct {
		Networks map[string]struct {
			IPAddress string `json:"IPAddress"`
		} `json:"Networks"`
	} `json:"NetworkSettings"`
}

// Port is one entry of a container's port list. PublicPort is 0 when the port
// is exposed but not published to the host.
type Port struct {
	IP          string `json:"IP"`
	PrivatePort int    `json:"PrivatePort"`
	PublicPort  int    `json:"PublicPort"`
	Type        string `json:"Type"`
}

// Name returns the container's primary name without the Engine's leading "/",
// or its short id when it somehow has no name.
func (c Container) Name() string {
	for _, n := range c.Names {
		n = strings.TrimPrefix(n, "/")
		if n != "" {
			return n
		}
	}
	if len(c.ID) > 12 {
		return c.ID[:12]
	}
	return c.ID
}

// statusError carries the Engine's own error reply for a non-2xx response.
type statusError struct {
	Code    int
	Message string
}

func (e *statusError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("docker: unexpected status %d", e.Code)
	}
	return fmt.Sprintf("docker: status %d: %s", e.Code, e.Message)
}

// ClientConfig is the connection configuration. Socket and Host are mutually
// exclusive; an empty pair means the default unix socket.
type ClientConfig struct {
	// Socket is a unix socket path (default /var/run/docker.sock).
	Socket string
	// Host is a tcp:// or https:// endpoint, used instead of Socket.
	Host string
	// TLSCert/TLSKey/TLSCA are file paths for a TLS-protected Host.
	TLSCert, TLSKey, TLSCA string
}

// Client is a minimal, read-only Docker Engine API client.
//
// It is hardened the same way internal/k8s's is: redirects are never followed,
// link-local destinations are refused at connect time so a mistyped host cannot
// reach a cloud metadata service, TLS (when used) is verified with no
// skip-verify escape hatch, and every read is bounded.
type Client struct {
	base string // scheme://host with no trailing slash
	http *http.Client
	// stream has NO client timeout: the event watch is a long-lived response
	// whose body is read for hours, and http.Client.Timeout covers the body read
	// as well as the request.
	stream *http.Client

	mu        sync.Mutex
	version   string
	versionAt time.Time
}

// NewClient builds a client for cfg. Certificates are read here, so a missing
// or unparseable file fails loudly at construction rather than becoming a
// handshake error on every poll.
func NewClient(cfg ClientConfig) (*Client, error) {
	transport := &http.Transport{
		MaxIdleConns:        2,
		IdleConnTimeout:     60 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	base := ""

	switch {
	case cfg.Host != "":
		u, err := url.Parse(cfg.Host)
		if err != nil || u.Host == "" {
			return nil, fmt.Errorf("docker: host must be an absolute tcp:// or https:// URL, got %q", cfg.Host)
		}
		scheme := u.Scheme
		switch scheme {
		case "tcp", "http":
			scheme = "http"
		case "https":
			tlsCfg, err := clientTLS(cfg)
			if err != nil {
				return nil, err
			}
			transport.TLSClientConfig = tlsCfg
		default:
			return nil, fmt.Errorf("docker: host scheme %q is not supported (use tcp:// or https://)", u.Scheme)
		}
		base = scheme + "://" + u.Host
		transport.DialContext = (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
			Control:   refuseLinkLocal,
		}).DialContext
	default:
		path := cfg.Socket
		if path == "" {
			path = "/var/run/docker.sock"
		}
		if !strings.HasPrefix(path, "/") {
			return nil, fmt.Errorf("docker: socket must be an absolute path, got %q", path)
		}
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("docker: socket %q is not reachable: %w", path, err)
		}
		dialer := &net.Dialer{Timeout: 5 * time.Second}
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", path)
		}
		// The Host header is meaningless over a unix socket, but net/http needs a
		// syntactically valid URL to build a request at all.
		base = "http://docker"
	}

	noRedirect := func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{
		base:   base,
		http:   &http.Client{Timeout: 30 * time.Second, CheckRedirect: noRedirect, Transport: transport},
		stream: &http.Client{CheckRedirect: noRedirect, Transport: transport},
	}, nil
}

// clientTLS builds the TLS configuration for an https endpoint. There is no
// skip-verify option: an endpoint that hands out the Docker API is exactly the
// one where an unverified peer is unacceptable.
func clientTLS(cfg ClientConfig) (*tls.Config, error) {
	out := &tls.Config{MinVersion: tls.VersionTLS12}
	if cfg.TLSCA != "" {
		pem, err := os.ReadFile(cfg.TLSCA)
		if err != nil {
			return nil, fmt.Errorf("docker: read tlsCA %q: %w", cfg.TLSCA, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("docker: tlsCA %q contains no usable PEM certificate", cfg.TLSCA)
		}
		out.RootCAs = pool
	}
	if (cfg.TLSCert == "") != (cfg.TLSKey == "") {
		return nil, fmt.Errorf("docker: tlsCert and tlsKey must be set together")
	}
	if cfg.TLSCert != "" {
		pair, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
		if err != nil {
			return nil, fmt.Errorf("docker: load tlsCert/tlsKey: %w", err)
		}
		out.Certificates = []tls.Certificate{pair}
	}
	return out, nil
}

func refuseLinkLocal(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()) {
		return fmt.Errorf("docker: link-local destination %s refused", ip)
	}
	return nil
}

// versionInfo is the subset of GET /version discovery negotiates on.
type versionInfo struct {
	APIVersion    string `json:"ApiVersion"`
	MinAPIVersion string `json:"MinAPIVersion"`
}

// apiPrefix returns the "/vX.Y" path prefix to use, negotiating it once per
// versionTTL. gpm asks for preferredAPIVersion, drops to the daemon's own
// version when that is older, and raises to the daemon's MINIMUM when the
// daemon has dropped support for something as old as gpm's preference. A
// /version call that fails is an error, not a guess: talking to an endpoint of
// unknown vintage with an assumed path prefix is how a 404 turns into "no
// containers", which is a delete-everything input.
func (c *Client) apiPrefix(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.version != "" && time.Since(c.versionAt) < versionTTL {
		v := c.version
		c.mu.Unlock()
		return "/v" + v, nil
	}
	c.mu.Unlock()

	var info versionInfo
	if err := c.get(ctx, "/version", nil, &info); err != nil {
		return "", err
	}
	if info.APIVersion == "" {
		return "", fmt.Errorf("docker: GET /version returned no ApiVersion; is %s a Docker Engine endpoint?", c.base)
	}
	use := preferredAPIVersion
	if compareAPIVersion(info.APIVersion, use) < 0 {
		use = info.APIVersion
	}
	if info.MinAPIVersion != "" && compareAPIVersion(use, info.MinAPIVersion) < 0 {
		use = info.MinAPIVersion
	}
	c.mu.Lock()
	c.version, c.versionAt = use, time.Now()
	c.mu.Unlock()
	return "/v" + use, nil
}

// compareAPIVersion compares two "major.minor" Engine API versions.
func compareAPIVersion(a, b string) int {
	amaj, amin := splitVersion(a)
	bmaj, bmin := splitVersion(b)
	switch {
	case amaj != bmaj:
		if amaj < bmaj {
			return -1
		}
		return 1
	case amin != bmin:
		if amin < bmin {
			return -1
		}
		return 1
	}
	return 0
}

func splitVersion(v string) (int, int) {
	maj, min, _ := strings.Cut(strings.TrimPrefix(v, "v"), ".")
	m, _ := strconv.Atoi(maj)
	n, _ := strconv.Atoi(min)
	return m, n
}

// Ping reports whether the endpoint answers as a Docker Engine. It is the
// "socket reachable" probe the status endpoint surfaces.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.apiPrefix(ctx)
	return err
}

// get performs one GET and decodes the JSON body into out.
func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	body, err := c.getRaw(ctx, path, query)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("docker: GET %s: decode response: %w", path, err)
	}
	return nil
}

func (c *Client) getRaw(ctx context.Context, path string, query url.Values) ([]byte, error) {
	full := c.base + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return nil, fmt.Errorf("docker: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("docker: GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	// Read one byte past the cap so an oversized response is reported as such
	// rather than surfacing as an opaque JSON syntax error on a truncated body.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRespBody+1))
	if err != nil {
		return nil, fmt.Errorf("docker: GET %s: read body: %w", path, err)
	}
	if len(body) > maxRespBody {
		return nil, fmt.Errorf("docker: GET %s: response exceeded the %d-byte body cap", path, maxRespBody)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &statusError{Code: resp.StatusCode, Message: apiErrorMessage(body)}
	}
	return body, nil
}

// apiErrorMessage pulls the message out of an Engine error reply, falling back
// to a bounded excerpt of whatever arrived.
func apiErrorMessage(body []byte) string {
	var st struct {
		Message string `json:"message"`
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

// ListContainers returns every container carrying enabledLabel="true",
// filtered server-side. all includes non-running containers.
//
// The contract the reconciler's freeze behaviour depends on: a non-nil error
// NEVER comes with items, and a nil error ALWAYS means a complete list. An
// empty slice with a nil error is a legitimate "nothing is labelled", which is
// a different thing entirely and is never produced by a failure path - so a
// response that is not a JSON array (a Status object, an HTML error page, a
// bare null from something that is not the Engine) is an ERROR, not an empty
// list, because an empty list is a delete-everything input.
func (c *Client) ListContainers(ctx context.Context, enabledLabel string, all bool) ([]Container, error) {
	prefix, err := c.apiPrefix(ctx)
	if err != nil {
		return nil, err
	}
	filters, err := json.Marshal(map[string][]string{"label": {enabledLabel + "=true"}})
	if err != nil {
		return nil, fmt.Errorf("docker: build filters: %w", err)
	}
	q := url.Values{}
	q.Set("filters", string(filters))
	if all {
		q.Set("all", "1")
	}
	body, err := c.getRaw(ctx, prefix+"/containers/json", q)
	if err != nil {
		return nil, err
	}
	if trimmed := strings.TrimSpace(string(body)); !strings.HasPrefix(trimmed, "[") {
		return nil, fmt.Errorf("docker: GET %s/containers/json did not return a JSON array; refusing to treat a non-list response as an empty container list (check socket/host)", prefix)
	}
	var out []Container
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("docker: GET %s/containers/json: decode response: %w", prefix, err)
	}
	if len(out) > maxContainers {
		return nil, fmt.Errorf("docker: container list exceeded %d items; refusing to treat it as complete", maxContainers)
	}
	return out, nil
}

// eventActions are the container lifecycle events that can change what
// discovery would derive. "create" is deliberately absent: a created container
// has no address yet, and its "start" follows immediately.
var eventActions = []string{"start", "stop", "die", "update", "destroy", "rename"}

// WatchEvents streams container lifecycle events until ctx is cancelled or the
// stream fails, calling onEvent once per event. It never returns nil while ctx
// is alive: a stream that ends is a condition the caller has to back off and
// retry, not a completion.
//
// Events are a LATENCY optimisation only. Discovery's correctness comes from
// the full-state poll; an event just says "look sooner". That is why this
// method decodes nothing but the fact that a line arrived, and why a broken
// stream degrades to the poll interval rather than to a stale config.
func (c *Client) WatchEvents(ctx context.Context, onEvent func()) error {
	prefix, err := c.apiPrefix(ctx)
	if err != nil {
		return err
	}
	filters, err := json.Marshal(map[string][]string{
		"type":  {"container"},
		"event": eventActions,
	})
	if err != nil {
		return fmt.Errorf("docker: build filters: %w", err)
	}
	q := url.Values{}
	q.Set("filters", string(filters))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+prefix+"/events?"+q.Encode(), nil)
	if err != nil {
		return fmt.Errorf("docker: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.stream.Do(req)
	if err != nil {
		return fmt.Errorf("docker: GET %s/events: %w", prefix, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return &statusError{Code: resp.StatusCode, Message: apiErrorMessage(body)}
	}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64<<10), maxEventLine)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		if onEvent != nil {
			onEvent()
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("docker: event stream: %w", err)
	}
	return fmt.Errorf("docker: event stream closed by the endpoint")
}
