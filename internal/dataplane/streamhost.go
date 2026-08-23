package dataplane

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

const (
	streamDialTimeout = 10 * time.Second
	udpSessionIdle    = 60 * time.Second
	udpBufSize        = 64 * 1024
	maxUDPSessions    = 4096 // per listener; bounds spoofed-source memory growth
)

// streamManager runs the raw TCP/UDP forwarders for StreamHosts. It lives on the
// data-plane Server (across reloads) and reconciles the running listeners with
// the desired config on each reload: ports added are opened, ports removed are
// closed, and a changed backend is swapped without dropping the listener.
type streamManager struct {
	mu  sync.Mutex
	tcp map[int]*tcpForwarder
	udp map[int]*udpForwarder

	// proxyProto supplies the live inbound PROXY protocol config, so a stream
	// listener behind an L4 balancer evaluates access lists against the real
	// client rather than against the balancer. Read once per accept; nil (the
	// default) leaves the listener untouched.
	proxyProto func() *proxyProtoConfig
}

func newStreamManager() *streamManager {
	return &streamManager{tcp: map[int]*tcpForwarder{}, udp: map[int]*udpForwarder{}}
}

// reload reconciles listeners with the desired stream hosts. A bind failure on a
// port is logged and skipped (never fatal), so one bad port can't take the plane
// down.
func (m *streamManager) reload(cfg model.Config, certs *certResolver) {
	wantTCP, wantUDP := buildStreamRoutes(cfg, certs)

	m.mu.Lock()
	defer m.mu.Unlock()

	for port, f := range m.tcp {
		if _, ok := wantTCP[port]; !ok {
			f.stop()
			delete(m.tcp, port)
		}
	}
	for port, f := range m.udp {
		if _, ok := wantUDP[port]; !ok {
			f.stop()
			delete(m.udp, port)
		}
	}
	for port, routes := range wantTCP {
		if f, ok := m.tcp[port]; ok {
			f.setRoutes(routes)
			continue
		}
		f, err := startTCPForwarder(port, routes, m.proxyProto)
		if err != nil {
			log.Error().Int("port", port).Err(err).Msg("stream: failed to start tcp listener")
			continue
		}
		m.tcp[port] = f
	}
	for port, target := range wantUDP {
		if f, ok := m.udp[port]; ok {
			f.setTarget(target)
			continue
		}
		f, err := startUDPForwarder(port, target)
		if err != nil {
			log.Error().Int("port", port).Err(err).Msg("stream: failed to start udp listener")
			continue
		}
		m.udp[port] = f
	}
}

// stopAll closes every stream listener (called on data-plane shutdown).
func (m *streamManager) stopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for port, f := range m.tcp {
		f.stop()
		delete(m.tcp, port)
	}
	for port, f := range m.udp {
		f.stop()
		delete(m.udp, port)
	}
}

// --- TCP ---------------------------------------------------------------------

// streamTarget is a forwarder's live backend plus the StreamHost NAME that
// configured it. The name is carried alongside the address so a connection can
// be attributed to a host in the metrics without a second lookup, and so a
// reload that re-points a port also re-labels it.
type streamTarget struct {
	name string
	addr string // "host:port"

	// acls gate the connection on the client IP BEFORE any backend is dialled,
	// so a denied peer costs one accept and nothing downstream. Only the IP and
	// geo dimensions of a list apply at L4: basic auth is an HTTP
	// challenge/response with nowhere to live in a raw stream, and config
	// validation rejects a stream host referencing such a list rather than
	// silently evaluating half of it.
	acls       []accessList
	geoCountry func(net.IP) (string, bool)
	geoLoaded  func() bool

	// tlsMode is "", model.StreamTLSPassthrough or model.StreamTLSTerminate;
	// tlsConf carries the server-side handshake config in terminate mode.
	tlsMode string
	tlsConf *tls.Config
}

// allow evaluates every attached access list against the client IP. All lists
// must allow, mirroring how the HTTP chain nests one handler per list.
func (t *streamTarget) allow(ip net.IP) bool {
	for _, al := range t.acls {
		if !al.allowIP(ip, t.geoCountry, t.geoLoaded) {
			return false
		}
	}
	return true
}

// allowIP evaluates only the IP/CIDR and geo dimensions of a compiled access
// list, for raw streams where there is no request to carry basic auth. A list
// with neither dimension has nothing to match on, so it honours an explicit
// "deny" default and otherwise imposes no restriction - exactly the rule
// accessListHandler applies on the HTTP side.
func (c accessList) allowIP(ip net.IP, geoLookup func(net.IP) (string, bool), geoLoaded func() bool) bool {
	if !c.hasIP && !c.hasGeo {
		return !c.explicitDeny
	}
	return c.ipAllowed(ip, geoLookup, geoLoaded)
}

// streamRoutes is the compiled routing table for ONE TCP listen port: either a
// single blind route (the historical behaviour) or, when the hosts on the port
// declare tls.sniMatch, an SNI-keyed table so several hosts can share it.
type streamRoutes struct {
	sni   bool
	def   *streamTarget // used when !sni
	exact map[string]*streamTarget
	wild  []wildcardRoute
}

type wildcardRoute struct {
	suffix string // ".example.com", from a "*.example.com" match
	target *streamTarget
}

// match resolves a server name to its route. An exact match wins over a
// wildcard, and a wildcard covers exactly one extra label. An unmatched name -
// or a client that sent no SNI at all - has no route, and the connection is
// dropped rather than handed to an arbitrary backend.
func (rs *streamRoutes) match(name string) *streamTarget {
	if !rs.sni {
		return rs.def
	}
	if name == "" {
		return nil
	}
	if t, ok := rs.exact[name]; ok {
		return t
	}
	for _, w := range rs.wild {
		if !strings.HasSuffix(name, w.suffix) {
			continue
		}
		if label := strings.TrimSuffix(name, w.suffix); label != "" && !strings.Contains(label, ".") {
			return w.target
		}
	}
	return nil
}

// buildStreamRoutes compiles the enabled stream hosts into one route table per
// TCP listen port and one target per UDP listen port. A host whose access list
// or certificate cannot be resolved is DROPPED (fail closed) rather than served
// without its gate; config validation rejects both at write time, so this is
// the safety net for a config that bypassed it.
func buildStreamRoutes(cfg model.Config, certs *certResolver) (map[int]*streamRoutes, map[int]streamTarget) {
	lists := map[string]model.AccessList{}
	for _, a := range cfg.AccessLists {
		lists[a.Name] = a
	}
	geoCountry := currentGeoDB().Country
	geoLoaded := currentGeoDB().Loaded

	tcpPorts := map[int]*streamRoutes{}
	udpPorts := map[int]streamTarget{}
	for _, h := range cfg.StreamHosts {
		if h.Disabled {
			continue
		}
		target := streamTarget{
			name:       h.Name,
			addr:       net.JoinHostPort(h.ForwardHost, strconv.Itoa(h.ForwardPort)),
			geoCountry: geoCountry,
			geoLoaded:  geoLoaded,
		}
		unresolved := false
		for _, name := range h.AccessLists {
			al, ok := lists[name]
			if !ok {
				log.Error().Str("streamHost", h.Name).Str("accessList", name).
					Msg("stream: host references an access list that does not exist; refusing to serve the port rather than dropping the gate")
				unresolved = true
				break
			}
			target.acls = append(target.acls, compileAccessList(al))
		}
		if unresolved {
			continue
		}
		if h.TLS != nil {
			target.tlsMode = h.TLS.Mode
			if h.TLS.Mode == model.StreamTLSTerminate {
				conf, err := streamTerminateConfig(h.TLS.CertificateRef, cfg.Certificates, certs)
				if err != nil {
					log.Error().Str("streamHost", h.Name).Err(err).
						Msg("stream: cannot terminate TLS for this host; skipping it")
					continue
				}
				target.tlsConf = conf
			}
		}

		if h.Protocol == "tcp" || h.Protocol == "both" {
			addStreamRoute(tcpPorts, h, target)
		}
		if h.Protocol == "udp" || h.Protocol == "both" {
			// UDP carries no ClientHello, so a UDP port is always one blind
			// route (config validation rejects two hosts sharing one).
			udpPorts[h.ListenPort] = target
		}
	}
	return tcpPorts, udpPorts
}

// addStreamRoute files a compiled target under its listen port, in SNI mode when
// the host declares server names and blind mode otherwise. Mixing the two on one
// port is unroutable, so the later host is skipped (validation rejects it first).
func addStreamRoute(ports map[int]*streamRoutes, h model.StreamHost, target streamTarget) {
	names := h.SNINames()
	rs := ports[h.ListenPort]
	if rs == nil {
		rs = &streamRoutes{exact: map[string]*streamTarget{}}
		ports[h.ListenPort] = rs
	}
	t := &target
	if len(names) == 0 {
		if rs.sni || rs.def != nil {
			log.Error().Str("streamHost", h.Name).Int("port", h.ListenPort).
				Msg("stream: hosts share a tcp port but not every one of them routes by SNI; skipping this host")
			return
		}
		rs.def = t
		return
	}
	if rs.def != nil {
		log.Error().Str("streamHost", h.Name).Int("port", h.ListenPort).
			Msg("stream: an SNI-routed host shares a tcp port with a non-SNI host; skipping this host")
		return
	}
	rs.sni = true
	for _, n := range names {
		if strings.HasPrefix(n, "*.") {
			rs.wild = append(rs.wild, wildcardRoute{suffix: n[1:], target: t})
			continue
		}
		if _, dup := rs.exact[n]; dup {
			log.Error().Str("streamHost", h.Name).Str("sni", n).Int("port", h.ListenPort).
				Msg("stream: duplicate sni claim on a shared tcp port; keeping the first host")
			continue
		}
		rs.exact[n] = t
	}
}

// streamTerminateConfig builds the server-side TLS config for a terminate-mode
// stream host from the named Certificate. It sets no ALPN list: what rides
// inside a stream is an arbitrary TCP protocol, not necessarily HTTP.
func streamTerminateConfig(certRef string, all []model.Certificate, certs *certResolver) (*tls.Config, error) {
	if certs == nil {
		return nil, fmt.Errorf("certificate %q: no certificate store", certRef)
	}
	var domains []string
	for _, c := range all {
		if c.Name != certRef {
			continue
		}
		if c.Disabled {
			return nil, fmt.Errorf("certificate %q is disabled", certRef)
		}
		domains = c.Domains
		break
	}
	if len(domains) == 0 {
		return nil, fmt.Errorf("certificate %q is unknown or has no domains", certRef)
	}
	crt, err := certs.GetCertificate(&tls.ClientHelloInfo{ServerName: domains[0]})
	if err != nil {
		return nil, fmt.Errorf("certificate %q: %w (an ACME certificate is unavailable until it is issued)", certRef, err)
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		CipherSuites: secureCipherSuites,
		Certificates: []tls.Certificate{*crt},
	}, nil
}

type tcpForwarder struct {
	port   int
	ln     net.Listener
	routes atomic.Pointer[streamRoutes] // swappable on reload
}

func startTCPForwarder(port int, routes *streamRoutes, proxyProto func() *proxyProtoConfig) (*tcpForwarder, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}
	f := &tcpForwarder{port: port, ln: wrapProxyProtocol(ln, proxyProto)}
	f.routes.Store(routes)
	go f.acceptLoop()
	log.Info().Int("port", port).Bool("sniRouted", routes.sni).Msg("stream: tcp listener started")
	return f, nil
}

func (f *tcpForwarder) setRoutes(r *streamRoutes) { f.routes.Store(r) }
func (f *tcpForwarder) stop()                     { _ = f.ln.Close() }

func (f *tcpForwarder) acceptLoop() {
	for {
		c, err := f.ln.Accept()
		if err != nil {
			return // listener closed
		}
		go f.handle(c)
	}
}

func (f *tcpForwarder) handle(client net.Conn) {
	defer client.Close()
	routes := f.routes.Load()
	if routes == nil {
		return
	}
	// RemoteAddr FIRST, always: on a PROXY-protocol listener this is what parses
	// and consumes the header, so the address every check below sees is the real
	// client and the header bytes never reach the backend.
	clientIP := addrIP(client.RemoteAddr())

	var target *streamTarget
	var prefix []byte
	if routes.sni {
		// Peek the ClientHello without consuming it: passthrough replays the
		// exact bytes to the backend, terminate replays them into crypto/tls.
		name, raw, err := peekClientHello(client, clientHelloTimeout)
		if err != nil {
			log.Debug().Int("port", f.port).Str("client", peerKey(clientIP)).Err(err).
				Msg("stream: could not read a TLS ClientHello on an SNI-routed port")
			return
		}
		prefix = raw
		if target = routes.match(name); target == nil {
			log.Debug().Int("port", f.port).Str("sni", name).Str("client", peerKey(clientIP)).
				Msg("stream: no stream host claims this server name")
			return
		}
	} else {
		target = routes.def
	}
	if target == nil {
		return
	}
	// The gate runs before the dial, so a denied client never causes an upstream
	// connection to exist at all.
	if !target.allow(clientIP) {
		log.Warn().Int("port", f.port).Str("streamHost", target.name).Str("client", peerKey(clientIP)).
			Msg("stream: access list denied the connection")
		return
	}

	backend, err := net.DialTimeout("tcp", target.addr, streamDialTimeout)
	if err != nil {
		log.Warn().Int("port", f.port).Str("target", target.addr).Err(err).Msg("stream: tcp dial failed")
		return
	}
	defer backend.Close()
	if mh := metricsHook(); mh != nil {
		mh.StreamOpened(target.name)
		defer mh.StreamClosed(target.name)
	}
	var front net.Conn = client
	if len(prefix) > 0 {
		front = &prefixConn{Conn: client, prefix: prefix}
	}
	if target.tlsMode == model.StreamTLSTerminate && target.tlsConf != nil {
		// Terminate at gpm: the backend receives plaintext. The handshake runs
		// lazily on the first copy below.
		front = tls.Server(front, target.tlsConf)
	}
	// Bidirectional copy. When either side ends, the deferred Close on both conns
	// unblocks the other io.Copy so the goroutine exits.
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(backend, front); done <- struct{}{} }()
	go func() { _, _ = io.Copy(front, backend); done <- struct{}{} }()
	<-done
}

// --- UDP ---------------------------------------------------------------------

type udpForwarder struct {
	port     int
	pc       net.PacketConn
	target   atomic.Pointer[streamTarget]
	closed   atomic.Bool
	mu       sync.Mutex
	sessions map[string]*udpSession // client addr -> backend session
}

type udpSession struct {
	backend net.Conn
	last    atomic.Int64 // unixnano of the last packet FROM the client
	// host is the StreamHost name this session was opened for, kept so the
	// active-connection gauge is decremented against the same label it was
	// incremented against even if a reload re-points the port meanwhile.
	host string
	// done makes teardown idempotent. A session can be torn down from three
	// places at once - the idle reaper, the backend read loop ending, and
	// stop() - and the pre-existing double Close was harmless only because
	// closing a closed conn is. Decrementing the active gauge twice is not, so
	// the first claimant wins.
	done atomic.Bool
}

func startUDPForwarder(port int, target streamTarget) (*udpForwarder, error) {
	pc, err := net.ListenPacket("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}
	f := &udpForwarder{port: port, pc: pc, sessions: map[string]*udpSession{}}
	f.target.Store(&target)
	go f.readLoop()
	go f.reapLoop()
	log.Info().Int("port", port).Str("host", target.name).Str("target", target.addr).Msg("stream: udp listener started")
	return f, nil
}

func (f *udpForwarder) setTarget(t streamTarget) { f.target.Store(&t) }

// closeSession closes a session's backend and releases its gauge slot. Every
// path that removes a session from the table goes through it, so the
// active-connection gauge cannot drift.
func (f *udpForwarder) closeSession(s *udpSession) {
	if !s.done.CompareAndSwap(false, true) {
		return
	}
	_ = s.backend.Close()
	if mh := metricsHook(); mh != nil {
		mh.StreamClosed(s.host)
	}
}

func (f *udpForwarder) stop() {
	f.closed.Store(true)
	_ = f.pc.Close()
	f.mu.Lock()
	for k, s := range f.sessions {
		f.closeSession(s)
		delete(f.sessions, k)
	}
	f.mu.Unlock()
}

func (f *udpForwarder) readLoop() {
	buf := make([]byte, udpBufSize)
	for {
		n, clientAddr, err := f.pc.ReadFrom(buf)
		if err != nil {
			return // listener closed
		}
		f.forward(clientAddr, buf[:n])
	}
}

func (f *udpForwarder) forward(clientAddr net.Addr, data []byte) {
	key := clientAddr.String()
	f.mu.Lock()
	s := f.sessions[key]
	if s == nil {
		target := *f.target.Load()
		// Access lists are evaluated once per session, before the backend is
		// dialled, so a denied source never causes an upstream socket to exist.
		if !target.allow(addrIP(clientAddr)) {
			f.mu.Unlock()
			if udpDeniedPeers.first(strconv.Itoa(f.port) + "/" + peerKey(addrIP(clientAddr))) {
				log.Warn().Int("port", f.port).Str("streamHost", target.name).Str("client", key).
					Msg("stream: access list denied the udp source")
			}
			return
		}
		if len(f.sessions) >= maxUDPSessions {
			f.mu.Unlock()
			return // session table full; drop (anti-amplification)
		}
		backend, err := net.DialTimeout("udp", target.addr, streamDialTimeout)
		if err != nil {
			f.mu.Unlock()
			log.Warn().Int("port", f.port).Err(err).Msg("stream: udp dial failed")
			return
		}
		s = &udpSession{backend: backend, host: target.name}
		f.sessions[key] = s
		if mh := metricsHook(); mh != nil {
			mh.StreamOpened(s.host)
		}
		go f.backendToClient(clientAddr, key, s)
	}
	f.mu.Unlock()
	s.last.Store(time.Now().UnixNano())
	_, _ = s.backend.Write(data)
}

func (f *udpForwarder) backendToClient(clientAddr net.Addr, key string, s *udpSession) {
	buf := make([]byte, udpBufSize)
	for {
		_ = s.backend.SetReadDeadline(time.Now().Add(udpSessionIdle))
		n, err := s.backend.Read(buf)
		if err != nil {
			break // backend idle past the deadline, or session closed
		}
		_, _ = f.pc.WriteTo(buf[:n], clientAddr)
	}
	f.mu.Lock()
	if f.sessions[key] == s {
		delete(f.sessions, key)
	}
	f.mu.Unlock()
	f.closeSession(s)
}

// udpDeniedPeers keeps a denied UDP source from writing one log line per packet.
var udpDeniedPeers = &peerWarnSet{}

// reapLoop closes sessions whose client has been silent past the idle window
// (the backend read-deadline handles the backend-silent case separately).
func (f *udpForwarder) reapLoop() {
	ticker := time.NewTicker(udpSessionIdle)
	defer ticker.Stop()
	for range ticker.C {
		if f.closed.Load() {
			return
		}
		cutoff := time.Now().Add(-udpSessionIdle).UnixNano()
		f.mu.Lock()
		for k, s := range f.sessions {
			if s.last.Load() < cutoff {
				f.closeSession(s)
				delete(f.sessions, k)
			}
		}
		f.mu.Unlock()
	}
}
