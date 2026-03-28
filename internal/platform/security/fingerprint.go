// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package security

import (
	"fmt"
	"net"
	"slices"
	"strings"
)

// virtualMACPrefixes lists MAC address prefixes commonly used by
// hypervisors, Docker, and VPN adapters.
var virtualMACPrefixes = []string{
	"00:00:00",
	"02:42:",
	"02:00:",
	"fe:ff:",
	"00:50:56",
	"00:0c:29",
	"00:1c:42",
	"08:00:27",
	"52:54:00",
}

func isVirtualMAC(mac string) bool {
	for _, p := range virtualMACPrefixes {
		if strings.HasPrefix(mac, p) {
			return true
		}
	}
	return false
}

// GetMACAddress returns the first physical (non-loopback, non-virtual) MAC
// address, sorted lexicographically for determinism.
func GetMACAddress() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("enumerating network interfaces: %w", err)
	}
	var candidates []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		hw := iface.HardwareAddr
		if len(hw) == 0 {
			continue
		}
		s := strings.ToLower(hw.String())
		if isVirtualMAC(s) {
			continue
		}
		candidates = append(candidates, s)
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("no physical NIC MAC address found")
	}
	slices.Sort(candidates)
	return candidates[0], nil
}
