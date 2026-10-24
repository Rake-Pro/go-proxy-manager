package dataplane

import (
	"fmt"
	"net"
	"net/netip"
	"syscall"
)

// refuseLinkLocal is a net.Dialer Control hook for the data plane's own
// outbound clients (the bouncer): a destination that resolved to a link-local
// address is refused at connect time, so an operator-configured URL cannot be
// pointed at a cloud metadata service. Private ranges stay allowed, which is
// where a CrowdSec LAPI normally lives. The webhook, DNS-sync, Kubernetes and
// Docker clients carry the same hook.
func refuseLinkLocal(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	// netip keeps a zone ("fe80::1%eth0"), which net.ParseIP would reject and
	// so let through.
	if ip, err := netip.ParseAddr(host); err == nil && (ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()) {
		return fmt.Errorf("link-local destination %s refused", ip)
	}
	return nil
}

// ipBucketKey is the per-client key the rate limiter and the basic-auth
// lockout use. IPv6 clients are grouped by /64, as the admin login gate does:
// one subscriber holds a whole /64, so keying each address on its own would
// hand them a fresh bucket per address (a bypass) and let one prefix fill a
// bounded map on its own (a lockout for everyone else). IPv4 keys stay the
// address.
func ipBucketKey(ip net.IP) string {
	if ip == nil {
		return ""
	}
	if ip.To4() == nil {
		return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
	}
	return ip.String()
}
