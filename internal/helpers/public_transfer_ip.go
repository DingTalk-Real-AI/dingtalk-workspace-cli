// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import "net/netip"

// Conservatively exclude all special-purpose allocations, including globally
// reachable exceptions and transition mechanisms, rather than treating global
// unicast as proof of public reachability. Nested entries are covered by parents.
// IANA registry snapshots: 2025-10-09 (reviewed 2026-09-09).
// https://www.iana.org/assignments/iana-ipv4-special-registry/
// https://www.iana.org/assignments/iana-ipv6-special-registry/
var transferSpecialNetworks = [...]netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.31.196.0/24"),
	netip.MustParsePrefix("192.52.193.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("192.175.48.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("100:0:0:1::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("2620:4f:8000::/48"),
	netip.MustParsePrefix("3fff::/20"),
	netip.MustParsePrefix("5f00::/16"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
}

// IsPublicTransferIP reports whether an address is an ordinary public transfer
// destination. It excludes special-purpose and unallocated IPv6 space even when
// Go classifies it as global unicast. Callers must also validate URL hosts and
// pin the checked address for the actual connection.
func IsPublicTransferIP(ip netip.Addr) bool {
	if ip.Zone() != "" {
		return false
	}
	// Mapped IPv4 addresses must follow exactly the same policy as native IPv4.
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() {
		return false
	}
	for _, prefix := range transferSpecialNetworks {
		if prefix.Contains(ip) {
			return false
		}
	}
	// Fail closed for IPv6 outside the currently allocated global-unicast space,
	// including deprecated site-local and IPv4-compatible addresses.
	return ip.Is4() || netip.MustParsePrefix("2000::/3").Contains(ip)
}
