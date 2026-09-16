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
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil && !IsPublicTransferIP(ip) {
		return fmt.Errorf("白板下载地址禁止访问非公网地址")
	}
	return nil
}

func whiteboardDownloadControl(_, address string, _ syscall.RawConn) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil || port != "443" {
		return fmt.Errorf("白板下载连接目标无效")
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !IsPublicTransferIP(ip) {
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
