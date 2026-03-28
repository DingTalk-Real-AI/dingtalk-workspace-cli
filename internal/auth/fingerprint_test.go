package auth

import (
	"net"
	"testing"
)

func TestSelectMAC_PhysicalPreferred(t *testing.T) {
	t.Parallel()
	ifaces := []net.Interface{
		{Index: 1, Name: "lo", Flags: net.FlagLoopback, HardwareAddr: nil},
		{Index: 2, Name: "eth0", Flags: net.FlagUp, HardwareAddr: net.HardwareAddr{0x02, 0x42, 0xac, 0x11, 0x00, 0x02}},   // Docker virtual
		{Index: 3, Name: "enp0s3", Flags: net.FlagUp, HardwareAddr: net.HardwareAddr{0x14, 0x98, 0x77, 0xab, 0xcd, 0xef}}, // physical
	}
	mac, err := selectMAC(ifaces)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mac != "14:98:77:ab:cd:ef" {
		t.Fatalf("expected physical MAC, got %s", mac)
	}
}

func TestSelectMAC_VirtualFallback(t *testing.T) {
	t.Parallel()
	// Docker container scenario: only loopback + Docker veth
	ifaces := []net.Interface{
		{Index: 1, Name: "lo", Flags: net.FlagLoopback, HardwareAddr: nil},
		{Index: 2, Name: "eth0", Flags: net.FlagUp, HardwareAddr: net.HardwareAddr{0x02, 0x42, 0xac, 0x11, 0x00, 0x02}},
	}
	mac, err := selectMAC(ifaces)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mac != "02:42:ac:11:00:02" {
		t.Fatalf("expected Docker virtual MAC, got %s", mac)
	}
}

func TestSelectMAC_MultipleVirtualPicksFirst(t *testing.T) {
	t.Parallel()
	ifaces := []net.Interface{
		{Index: 1, Name: "lo", Flags: net.FlagLoopback},
		{Index: 2, Name: "eth0", Flags: net.FlagUp, HardwareAddr: net.HardwareAddr{0x02, 0x42, 0xbb, 0x00, 0x00, 0x01}},
		{Index: 3, Name: "eth1", Flags: net.FlagUp, HardwareAddr: net.HardwareAddr{0x02, 0x42, 0xaa, 0x00, 0x00, 0x01}},
	}
	mac, err := selectMAC(ifaces)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Lexicographically: 02:42:aa:... < 02:42:bb:...
	if mac != "02:42:aa:00:00:01" {
		t.Fatalf("expected lexicographically first virtual MAC, got %s", mac)
	}
}

func TestSelectMAC_NoInterfaces(t *testing.T) {
	t.Parallel()
	_, err := selectMAC(nil)
	if err == nil {
		t.Fatal("expected error for empty interface list")
	}
}

func TestSelectMAC_OnlyLoopback(t *testing.T) {
	t.Parallel()
	ifaces := []net.Interface{
		{Index: 1, Name: "lo", Flags: net.FlagLoopback, HardwareAddr: net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
	}
	_, err := selectMAC(ifaces)
	if err == nil {
		t.Fatal("expected error when only loopback exists")
	}
}

func TestSelectMAC_NoHardwareAddr(t *testing.T) {
	t.Parallel()
	ifaces := []net.Interface{
		{Index: 1, Name: "tun0", Flags: net.FlagUp, HardwareAddr: nil},
	}
	_, err := selectMAC(ifaces)
	if err == nil {
		t.Fatal("expected error for interfaces without hardware address")
	}
}

func TestGetMACAddress_ReturnsNonEmpty(t *testing.T) {
	t.Parallel()
	mac, err := GetMACAddress()
	if err != nil {
		t.Skipf("no NIC available: %v", err)
	}
	if mac == "" {
		t.Fatal("expected non-empty MAC address")
	}
	// MAC format: XX:XX:XX:XX:XX:XX
	if len(mac) != 17 {
		t.Fatalf("unexpected MAC format: %s", mac)
	}
}
