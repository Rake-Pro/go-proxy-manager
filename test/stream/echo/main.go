// Command echo is a minimal, dependency-free TCP and/or UDP echo server used to
// exercise go-proxy-manager StreamHosts end to end: it echoes back whatever it
// receives. Configure via env: ECHO_ADDR (default ":9000"), ECHO_PROTO
// (tcp|udp|both, default "both").
package main

import (
	"io"
	"log"
	"net"
	"os"
)

func main() {
	addr := env("ECHO_ADDR", ":9000")
	proto := env("ECHO_PROTO", "both")

	errc := make(chan error, 2)
	if proto == "tcp" || proto == "both" {
		go func() { errc <- serveTCP(addr) }()
	}
	if proto == "udp" || proto == "both" {
		go func() { errc <- serveUDP(addr) }()
	}
	log.Printf("echo: listening on %s (%s)", addr, proto)
	log.Fatal(<-errc)
}

func serveTCP(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		go func(c net.Conn) { defer c.Close(); _, _ = io.Copy(c, c) }(c)
	}
}

func serveUDP(addr string) error {
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		return err
	}
	buf := make([]byte, 65535)
	for {
		n, peer, err := pc.ReadFrom(buf)
		if err != nil {
			return err
		}
		_, _ = pc.WriteTo(buf[:n], peer)
	}
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
