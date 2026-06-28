package dataplane

import (
	"fmt"
	"io"
	"net"
	"strconv"
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
}

func newStreamManager() *streamManager {
	return &streamManager{tcp: map[int]*tcpForwarder{}, udp: map[int]*udpForwarder{}}
}

// reload reconciles listeners with the desired stream hosts. A bind failure on a
// port is logged and skipped (never fatal), so one bad port can't take the plane
// down.
func (m *streamManager) reload(hosts []model.StreamHost) {
	wantTCP, wantUDP := map[int]string{}, map[int]string{}
	for _, h := range hosts {
		if h.Disabled {
			continue
		}
		target := net.JoinHostPort(h.ForwardHost, strconv.Itoa(h.ForwardPort))
		switch h.Protocol {
		case "tcp":
			wantTCP[h.ListenPort] = target
		case "udp":
			wantUDP[h.ListenPort] = target
		case "both":
			wantTCP[h.ListenPort] = target
			wantUDP[h.ListenPort] = target
		}
	}

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
	for port, target := range wantTCP {
		if f, ok := m.tcp[port]; ok {
			f.setTarget(target)
			continue
		}
		f, err := startTCPForwarder(port, target)
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

type tcpForwarder struct {
	port   int
	ln     net.Listener
	target atomic.Pointer[string] // "host:port"; swappable on reload
}

func startTCPForwarder(port int, target string) (*tcpForwarder, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}
	f := &tcpForwarder{port: port, ln: ln}
	f.target.Store(&target)
	go f.acceptLoop()
	log.Info().Int("port", port).Str("target", target).Msg("stream: tcp listener started")
	return f, nil
}

func (f *tcpForwarder) setTarget(t string) { f.target.Store(&t) }
func (f *tcpForwarder) stop()              { _ = f.ln.Close() }

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
	target := *f.target.Load()
	backend, err := net.DialTimeout("tcp", target, streamDialTimeout)
	if err != nil {
		log.Warn().Int("port", f.port).Str("target", target).Err(err).Msg("stream: tcp dial failed")
		return
	}
	defer backend.Close()
	// Bidirectional copy. When either side ends, the deferred Close on both conns
	// unblocks the other io.Copy so the goroutine exits.
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(backend, client); done <- struct{}{} }()
	go func() { _, _ = io.Copy(client, backend); done <- struct{}{} }()
	<-done
}

// --- UDP ---------------------------------------------------------------------

type udpForwarder struct {
	port     int
	pc       net.PacketConn
	target   atomic.Pointer[string]
	closed   atomic.Bool
	mu       sync.Mutex
	sessions map[string]*udpSession // client addr -> backend session
}

type udpSession struct {
	backend net.Conn
	last    atomic.Int64 // unixnano of the last packet FROM the client
}

func startUDPForwarder(port int, target string) (*udpForwarder, error) {
	pc, err := net.ListenPacket("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}
	f := &udpForwarder{port: port, pc: pc, sessions: map[string]*udpSession{}}
	f.target.Store(&target)
	go f.readLoop()
	go f.reapLoop()
	log.Info().Int("port", port).Str("target", target).Msg("stream: udp listener started")
	return f, nil
}

func (f *udpForwarder) setTarget(t string) { f.target.Store(&t) }

func (f *udpForwarder) stop() {
	f.closed.Store(true)
	_ = f.pc.Close()
	f.mu.Lock()
	for k, s := range f.sessions {
		_ = s.backend.Close()
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
		if len(f.sessions) >= maxUDPSessions {
			f.mu.Unlock()
			return // session table full; drop (anti-amplification)
		}
		backend, err := net.DialTimeout("udp", *f.target.Load(), streamDialTimeout)
		if err != nil {
			f.mu.Unlock()
			log.Warn().Int("port", f.port).Err(err).Msg("stream: udp dial failed")
			return
		}
		s = &udpSession{backend: backend}
		f.sessions[key] = s
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
	_ = s.backend.Close()
}

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
				_ = s.backend.Close()
				delete(f.sessions, k)
			}
		}
		f.mu.Unlock()
	}
}
