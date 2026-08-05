// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

// Package localio owns safe local artifact publication shared by product
// shortcuts. Remote names and URLs are always treated as untrusted input.
package localio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"time"
)

const downloadTimeout = 10 * time.Minute

type downloadTempFile interface {
	io.Writer
	Name() string
	Sync() error
	Close() error
}

var (
	createDownloadTemp = func(dir, pattern string) (downloadTempFile, error) { return os.CreateTemp(dir, pattern) }
	lookupDownloadIPs  = net.DefaultResolver.LookupIPAddr
	dialDownloadIP     = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	localGetwd         = os.Getwd
	localAbs           = filepath.Abs
	localEvalSymlinks  = filepath.EvalSymlinks
	localRel           = filepath.Rel
	localStat          = os.Stat
	localLstat         = os.Lstat
	localMkdir         = os.Mkdir
)

// DownloadOptions controls safe, atomic publication beneath BaseDir.
type DownloadOptions struct {
	BaseDir       string
	Output        string
	PreferredName string
	Overwrite     bool
	Headers       map[string]string
	Client        *http.Client
}

// DownloadResult describes the published local artifact.
type DownloadResult struct {
	AbsolutePath string
	RelativePath string
	SizeBytes    int64
}

// Download validates a platform-owned HTTPS URL, resolves a workspace-relative
// output path without following symlink escapes, streams into a sibling temp
// file, fsyncs it, and atomically publishes the completed file.
func Download(ctx context.Context, rawURL string, opts DownloadOptions) (DownloadResult, error) {
	parsed, err := ValidateDownloadURL(rawURL)
	if err != nil {
		return DownloadResult{}, err
	}
	abs, rel, err := ResolveOutputPath(opts.BaseDir, opts.Output, parsed.String(), opts.PreferredName, opts.Overwrite)
	if err != nil {
		return DownloadResult{}, err
	}
	client := opts.Client
	if client == nil {
		client = secureHTTPClient()
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil) // URL was fully validated above
	for key, value := range opts.Headers {
		if strings.TrimSpace(key) != "" {
			req.Header.Set(key, value)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("下载资源失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return DownloadResult{}, fmt.Errorf("下载资源失败: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	tmp, err := createDownloadTemp(filepath.Dir(abs), ".dws-download-*")
	if err != nil {
		return DownloadResult{}, fmt.Errorf("创建下载临时文件失败: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	size, copyErr := io.Copy(tmp, resp.Body)
	if copyErr == nil {
		copyErr = tmp.Sync()
	}
	if closeErr := tmp.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		cleanup()
		return DownloadResult{}, fmt.Errorf("写入下载临时文件失败: %w", copyErr)
	}
	if err := publishTempFile(tmpName, abs, opts.Overwrite); err != nil {
		cleanup()
		return DownloadResult{}, err
	}
	return DownloadResult{AbsolutePath: abs, RelativePath: filepath.ToSlash(rel), SizeBytes: size}, nil
}

// ValidateOutput rejects absolute paths and portable `..` escapes.
func ValidateOutput(output string) error {
	output = strings.TrimSpace(output)
	if output == "" {
		return fmt.Errorf("LOCAL_PATH_UNSAFE: --output 不能为空")
	}
	portable := strings.ReplaceAll(output, "\\", "/")
	if filepath.IsAbs(output) || pathpkg.IsAbs(portable) ||
		(len(portable) >= 2 && portable[1] == ':' && ((portable[0] >= 'a' && portable[0] <= 'z') || (portable[0] >= 'A' && portable[0] <= 'Z'))) {
		return fmt.Errorf("LOCAL_PATH_UNSAFE: --output 只接受工作目录内的相对路径")
	}
	clean := pathpkg.Clean(portable)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("LOCAL_PATH_UNSAFE: --output 不允许使用 .. 逃逸工作目录")
	}
	return nil
}

// ResolveOutputPath returns a symlink-safe destination below baseDir.
func ResolveOutputPath(baseDir, output, rawURL, preferredName string, overwrite bool) (string, string, error) {
	if err := ValidateOutput(output); err != nil {
		return "", "", err
	}
	if strings.TrimSpace(baseDir) == "" {
		var err error
		baseDir, err = localGetwd()
		if err != nil {
			return "", "", fmt.Errorf("读取工作目录失败: %w", err)
		}
	}
	absBase, err := localAbs(baseDir)
	if err != nil {
		return "", "", fmt.Errorf("解析工作目录失败: %w", err)
	}
	realBase, err := localEvalSymlinks(absBase)
	if err != nil {
		return "", "", fmt.Errorf("解析工作目录失败: %w", err)
	}

	rawOutput := strings.TrimSpace(output)
	directoryIntent := rawOutput == "." || strings.HasSuffix(rawOutput, "/") || strings.HasSuffix(rawOutput, string(os.PathSeparator))
	candidate := filepath.Join(realBase, filepath.Clean(rawOutput))
	if info, statErr := localStat(candidate); statErr == nil && info.IsDir() {
		directoryIntent = true
	}
	if directoryIntent {
		candidate = filepath.Join(candidate, SafeFilename(preferredName, rawURL))
	}
	parent := filepath.Dir(candidate)
	if err := ensureSafeParent(realBase, parent); err != nil {
		return "", "", err
	}
	realParent, err := localEvalSymlinks(parent)
	if err != nil {
		return "", "", fmt.Errorf("解析输出目录失败: %w", err)
	}
	parentRel, err := localRel(realBase, realParent)
	if err != nil || escapes(parentRel) {
		return "", "", fmt.Errorf("LOCAL_PATH_UNSAFE: --output 解析后逃逸工作目录")
	}
	destination := filepath.Join(realParent, filepath.Base(candidate))
	if info, statErr := localLstat(destination); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", "", fmt.Errorf("LOCAL_PATH_UNSAFE: --output 目标不能是符号链接")
		}
		if info.IsDir() {
			return "", "", fmt.Errorf("LOCAL_PATH_UNSAFE: --output 目标是目录")
		}
		if !overwrite {
			return "", "", fmt.Errorf("LOCAL_FILE_EXISTS: 目标文件已存在；如确认覆盖请显式传 --overwrite")
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return "", "", fmt.Errorf("检查输出文件失败: %w", statErr)
	}
	rel, err := localRel(realBase, destination)
	if err != nil || escapes(rel) {
		return "", "", fmt.Errorf("LOCAL_PATH_UNSAFE: 无法解析安全输出路径")
	}
	return destination, rel, nil
}

// SafeFilename selects a portable basename from a preferred server name or URL.
func SafeFilename(preferredName, rawURL string) string {
	if name := sanitizeFilename(preferredName); name != "" {
		return name
	}
	if parsed, err := url.Parse(rawURL); err == nil {
		if decoded, decodeErr := url.PathUnescape(filepath.Base(parsed.Path)); decodeErr == nil {
			if name := sanitizeFilename(decoded); name != "" {
				return name
			}
		}
	}
	return "download"
}

// ValidateDownloadURL accepts only public DingTalk and Aliyun OSS HTTPS hosts.
func ValidateDownloadURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return nil, fmt.Errorf("下载地址必须是受信任域名上的 HTTPS URL")
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host == "" || net.ParseIP(host) != nil || !allowedDownloadHost(host) {
		return nil, fmt.Errorf("下载地址域名 %q 不属于受信任的钉钉或 OSS 域名", host)
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return nil, fmt.Errorf("下载地址只允许 HTTPS 默认端口")
	}
	return parsed, nil
}

func secureHTTPClient() *http.Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := lookupDownloadIPs(ctx, host)
			if err != nil {
				return nil, err
			}
			for _, resolved := range ips {
				if !publicIP(resolved.IP) {
					return nil, fmt.Errorf("下载域名解析到非公网地址 %s", resolved.IP)
				}
			}
			// Dial the already validated address, not the hostname, to avoid a
			// second DNS lookup opening a rebinding window.
			var lastErr error
			for _, resolved := range ips {
				conn, dialErr := dialDownloadIP(ctx, network, net.JoinHostPort(resolved.IP.String(), port))
				if dialErr == nil {
					return conn, nil
				}
				lastErr = dialErr
			}
			return nil, lastErr
		},
	}
	client := &http.Client{Transport: transport, Timeout: downloadTimeout}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("下载重定向次数超过上限")
		}
		_, err := ValidateDownloadURL(req.URL.String())
		return err
	}
	return client
}

func allowedDownloadHost(host string) bool {
	return host == "dingtalk.com" || strings.HasSuffix(host, ".dingtalk.com") ||
		(strings.HasSuffix(host, ".aliyuncs.com") && strings.Contains(host, "oss") && !strings.Contains(host, "internal"))
}

func publicIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsMulticast() || addr.IsUnspecified() {
		return false
	}
	for _, prefix := range nonPublicPrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

var nonPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),   // carrier-grade NAT
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments
	netip.MustParsePrefix("192.0.2.0/24"),    // TEST-NET-1
	netip.MustParsePrefix("198.18.0.0/15"),   // benchmark networks
	netip.MustParsePrefix("198.51.100.0/24"), // TEST-NET-2
	netip.MustParsePrefix("203.0.113.0/24"),  // TEST-NET-3
	netip.MustParsePrefix("240.0.0.0/4"),     // reserved
	netip.MustParsePrefix("2001:db8::/32"),   // IPv6 documentation
}

func ensureSafeParent(base, parent string) error {
	rel, err := localRel(base, parent)
	if err != nil || escapes(rel) {
		return fmt.Errorf("LOCAL_PATH_UNSAFE: --output 解析后逃逸工作目录")
	}
	current := base
	if rel == "." {
		return nil
	}
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		current = filepath.Join(current, part)
		info, statErr := localLstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			if err := localMkdir(current, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
				return fmt.Errorf("创建输出目录失败: %w", err)
			}
			info, statErr = localLstat(current)
		}
		if statErr != nil {
			return fmt.Errorf("检查输出目录失败: %w", statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("LOCAL_PATH_UNSAFE: --output 父路径必须是非符号链接目录")
		}
	}
	return nil
}

func publishTempFile(tempPath, destination string, overwrite bool) error {
	if !overwrite {
		if err := os.Link(tempPath, destination); err != nil {
			if errors.Is(err, os.ErrExist) {
				return fmt.Errorf("LOCAL_FILE_EXISTS: 目标文件已存在")
			}
			return fmt.Errorf("发布下载文件失败: %w", err)
		}
		return os.Remove(tempPath)
	}
	if info, err := os.Lstat(destination); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("LOCAL_PATH_UNSAFE: --output 目标不能是符号链接")
	}
	if err := replaceFileAtomically(tempPath, destination); err != nil {
		return fmt.Errorf("原子发布下载文件失败: %w", err)
	}
	return nil
}

func sanitizeFilename(raw string) string {
	normalized := strings.ReplaceAll(raw, "\\", "/")
	if strings.TrimSpace(normalized) != normalized {
		return ""
	}
	name := filepath.Base(normalized)
	if name == "" || name == "." || name == ".." || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return ""
	}
	for _, char := range name {
		if char < 0x20 || char == 0x7f || strings.ContainsRune(`<>:"/\|?*`, char) {
			return ""
		}
	}
	stem := strings.ToUpper(strings.TrimRight(strings.SplitN(name, ".", 2)[0], " ."))
	if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" ||
		(len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '1' && stem[3] <= '9') {
		return ""
	}
	return name
}

func escapes(rel string) bool {
	portable := pathpkg.Clean(strings.ReplaceAll(rel, "\\", "/"))
	return portable == ".." || strings.HasPrefix(portable, "../")
}
