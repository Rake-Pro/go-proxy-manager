//go:build linux

package dataplane

import (
	"net"
	"syscall"
	"testing"
)

const ipprotoMPTCP = 262 // IPPROTO_MPTCP, Linux

// listenerProtocol reads SO_PROTOCOL from a listening socket: IPPROTO_TCP (6)
// for a plain listener, IPPROTO_MPTCP (262) for a Multipath TCP one.
func listenerProtocol(t *testing.T, ln net.Listener) int {
	t.Helper()
	sc, err := ln.(*net.TCPListener).SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var proto int
	var gerr error
	if err := sc.Control(func(fd uintptr) {
		proto, gerr = syscall.GetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_PROTOCOL)
	}); err != nil {
		t.Fatal(err)
	}
	if gerr != nil {
		t.Fatal(gerr)
	}
	return proto
}

// The stream forwarder must honour the data plane's ListenConfig: with MPTCP
// switched off the listener is a plain TCP socket, whatever the Go default is.
func TestStreamForwarderHonoursMPTCPSetting(t *testing.T) {
	routes := &streamRoutes{def: &streamTarget{name: "s", addr: "127.0.0.1:1"}}

	off := &net.ListenConfig{}
	off.SetMultipathTCP(false)
	f, err := startTCPForwarder(0, routes, nil, off)
	if err != nil {
		t.Fatal(err)
	}
	defer f.ln.Close()
	if p := listenerProtocol(t, f.ln); p != syscall.IPPROTO_TCP {
		t.Fatalf("listener protocol = %d, want plain TCP (%d) with MPTCP disabled", p, syscall.IPPROTO_TCP)
	}

	// Positive case only where the kernel can do MPTCP at all.
	probe := &net.ListenConfig{}
	probe.SetMultipathTCP(true)
	pl, err := probe.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("kernel refuses MPTCP listeners: %v", err)
	}
	kernelMPTCP := listenerProtocol(t, pl) == ipprotoMPTCP
	pl.Close()
	if !kernelMPTCP {
		t.Skip("kernel has MPTCP disabled; positive case not testable here")
	}
	on := &net.ListenConfig{}
	on.SetMultipathTCP(true)
	f2, err := startTCPForwarder(0, routes, nil, on)
	if err != nil {
		t.Fatal(err)
	}
	defer f2.ln.Close()
	if p := listenerProtocol(t, f2.ln); p != ipprotoMPTCP {
		t.Fatalf("listener protocol = %d, want MPTCP (%d) with MPTCP enabled", p, ipprotoMPTCP)
	}
}
