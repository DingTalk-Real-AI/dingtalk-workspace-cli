// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package helpers

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"syscall"
	"time"
)

const whiteboardExportMaxBytes int64 = 512 << 20

var whiteboardExportHTTPGet = downloadWhiteboardExportHTTP
var whiteboardDownloadClose = (*os.File).Close

// Validate every URL before a request. The dial control below additionally
// validates the resolved address used for the connection, preventing DNS rebinding.
func validateWhiteboardDownloadURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return fmt.Errorf("白板下载地址必须是无用户信息的 HTTPS URL，端口仅支持 443")
	}
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil && !whiteboardPublicIP(ip) {
		return fmt.Errorf("白板下载地址禁止访问非公网地址")
	}
	return nil
}

// Conservatively exclude all special-purpose allocations, including globally
// reachable exceptions and transition mechanisms, rather than treating global
// unicast as proof of public reachability. Nested entries are covered by parents.
// IANA registry snapshots: 2025-10-09 (reviewed 2026-09-09).
// https://www.iana.org/assignments/iana-ipv4-special-registry/
// https://www.iana.org/assignments/iana-ipv6-special-registry/
var whiteboardSpecialNetworks = [...]netip.Prefix{
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

func whiteboardPublicIP(ip netip.Addr) bool {
	if ip.Zone() != "" {
		return false
	}
	// Mapped IPv4 addresses must follow exactly the same policy as native IPv4.
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() {
		return false
	}
	for _, prefix := range whiteboardSpecialNetworks {
		if prefix.Contains(ip) {
			return false
		}
	}
	// Fail closed for IPv6 outside the currently allocated global-unicast space,
	// including deprecated site-local and IPv4-compatible addresses.
	return ip.Is4() || netip.MustParsePrefix("2000::/3").Contains(ip)
}

func whiteboardDownloadControl(_, address string, _ syscall.RawConn) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil || port != "443" {
		return fmt.Errorf("白板下载连接目标无效")
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !whiteboardPublicIP(ip) {
		return fmt.Errorf("白板下载连接禁止访问非公网地址")
	}
	return nil
}

func whiteboardDownloadRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return fmt.Errorf("白板下载重定向超过上限")
	}
	if err := validateWhiteboardDownloadURL(req.URL.String()); err != nil {
		return err
	}
	req.Header = make(http.Header)
	return nil
}

func downloadWhiteboardExportHTTP(ctx context.Context, rawURL string, _ map[string]string, destination string) error {
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 30 * time.Second, Control: whiteboardDownloadControl}).DialContext, TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: 30 * time.Second}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 10 * time.Minute, CheckRedirect: whiteboardDownloadRedirect}
	return downloadWhiteboardExportLimited(ctx, client, rawURL, destination, whiteboardExportMaxBytes)
}

func downloadWhiteboardExportLimited(ctx context.Context, client *http.Client, rawURL, destination string, limit int64) error {
	if err := validateWhiteboardDownloadURL(rawURL); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("白板下载请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("白板下载返回 HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > limit {
		return fmt.Errorf("白板导出文件超过大小上限 %d 字节", limit)
	}
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	size, copyErr := io.Copy(file, io.LimitReader(resp.Body, limit+1))
	closeErr := whiteboardDownloadClose(file)
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if size > limit {
		return fmt.Errorf("白板导出文件超过大小上限 %d 字节", limit)
	}
	return nil
}
