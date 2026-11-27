package dataplane

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
	"syscall"
)

// refuseLinkLocal is a net.Dialer Control hook for the data plane's own
// outbound clients (the bouncer): a destination that resolved to a link-local
// address is refused at connect time, so an operator-configured URL cannot be
// pointed at a cloud metadata service. Private ranges stay allowed, which is
// where a CrowdSec LAPI normally lives. The webhook, DNS-sync, Kubernetes and
// Docker clients carry the same hook.
func refuseLinkLocal(network, address string, _ syscall.RawConn) error {
	if strings.HasPrefix(network, "unix") {
		return nil // a socket path, not a network address
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	} else if host == "" {
		return nil // ":port": net dials the local system
	}
	// Control sees the resolved destination, an IP literal. A zone is dropped
	// before parsing, since netip refuses one on an IPv4 address and an
	// IPv4-mapped literal with a zone arrives as "169.254.169.254%eth0". What
	// still does not parse is refused: the guard never lets through an address
	// it cannot read.
	bare, _, _ := strings.Cut(host, "%")
	ip, err := netip.ParseAddr(bare)
	if err != nil {
		return fmt.Errorf("destination %q is not an IP address; refused", host)
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return fmt.Errorf("link-local destination %s refused", host)
	}
	return nil
}

// ipBucketKey is the per-client key the rate limiter uses (the basic-auth
// lockout groups IPv6 wider, see authGateKey). IPv6 clients are grouped by /64, as the admin login gate does:
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
