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
	"sync/atomic"
	"time"
)

const (
	downloadTimeout  = 10 * time.Minute
	maxDownloadBytes = int64(512 << 20)
)

type downloadTempFile interface {
	io.Writer
	Sync() error
	Close() error
}

var (
	createDownloadTemp = createDownloadTempInRoot
	lookupDownloadIPs  = net.DefaultResolver.LookupIPAddr
	dialDownloadIP     = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	localGetwd         = os.Getwd
	localAbs           = filepath.Abs
	localEvalSymlinks  = filepath.EvalSymlinks
	openDownloadRoot   = os.OpenRoot
	openDownloadParent = func(root *os.Root, name string) (*os.Root, error) { return root.OpenRoot(name) }
	downloadRootStat   = func(root *os.Root, name string) (os.FileInfo, error) { return root.Stat(name) }
	downloadRootLstat  = func(root *os.Root, name string) (os.FileInfo, error) { return root.Lstat(name) }
	downloadRootMkdir  = func(root *os.Root, name string, mode os.FileMode) error { return root.Mkdir(name, mode) }
	downloadRootLink   = func(root *os.Root, oldName, newName string) error { return root.Link(oldName, newName) }
	downloadRootRemove = func(root *os.Root, name string) error { return root.Remove(name) }
)

var downloadTempCounter atomic.Uint64

// DownloadOptions controls safe, atomic publication beneath BaseDir.
type DownloadOptions struct {
	BaseDir       string
	Output        string
	PreferredName string
	Headers       map[string]string
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
	return downloadWithClient(ctx, rawURL, opts, secureHTTPClient())
}

func downloadWithClient(ctx context.Context, rawURL string, opts DownloadOptions, client *http.Client) (DownloadResult, error) {
	return downloadWithClientLimit(ctx, rawURL, opts, client, maxDownloadBytes)
}

func downloadWithClientLimit(ctx context.Context, rawURL string, opts DownloadOptions, client *http.Client, maxBytes int64) (DownloadResult, error) {
	parsed, err := ValidateDownloadURL(rawURL)
	if err != nil {
		return DownloadResult{}, err
	}
	target, err := openDownloadTarget(opts.BaseDir, opts.Output, parsed.String(), opts.PreferredName)
	if err != nil {
		return DownloadResult{}, err
	}
	defer target.close()
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
	if resp.ContentLength > maxBytes {
		return DownloadResult{}, fmt.Errorf("LOCAL_DOWNLOAD_TOO_LARGE: 响应大小 %d 超过上限 %d 字节", resp.ContentLength, maxBytes)
	}
	if err := target.verifyParent(); err != nil {
		return DownloadResult{}, err
	}

	tmp, tmpName, err := createDownloadTemp(target.parentRoot)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("创建下载临时文件失败: %w", err)
	}
	cleanup := func() {
		_ = tmp.Close()
		_ = target.parentRoot.Remove(tmpName)
	}
	size, copyErr := io.Copy(tmp, io.LimitReader(resp.Body, maxBytes+1))
	if copyErr == nil && size > maxBytes {
		copyErr = fmt.Errorf("LOCAL_DOWNLOAD_TOO_LARGE: 下载内容超过上限 %d 字节", maxBytes)
	}
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
	if err := target.verifyParent(); err != nil {
		cleanup()
		return DownloadResult{}, err
	}
	if err := publishTempFile(target.parentRoot, tmpName, target.destinationName); err != nil {
		cleanup()
		return DownloadResult{}, err
	}
	return DownloadResult{AbsolutePath: target.absolutePath, RelativePath: filepath.ToSlash(target.relativePath), SizeBytes: size}, nil
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
type downloadTarget struct {
	baseRoot        *os.Root
	parentRoot      *os.Root
	parentInfo      os.FileInfo
	parentRelative  string
	destinationName string
	absolutePath    string
	relativePath    string
}

func (target *downloadTarget) close() {
	_ = target.parentRoot.Close()
	_ = target.baseRoot.Close()
}

func (target *downloadTarget) verifyParent() error {
	current, err := downloadRootStat(target.baseRoot, target.parentRelative)
	if err != nil || !os.SameFile(target.parentInfo, current) {
		return fmt.Errorf("LOCAL_PATH_CHANGED: 下载期间输出目录被替换")
	}
	return nil
}

func ResolveOutputPath(baseDir, output, rawURL, preferredName string) (string, string, error) {
	target, err := openDownloadTarget(baseDir, output, rawURL, preferredName)
	if err != nil {
		return "", "", err
	}
	defer target.close()
	return target.absolutePath, target.relativePath, nil
}

func openDownloadTarget(baseDir, output, rawURL, preferredName string) (*downloadTarget, error) {
	if err := ValidateOutput(output); err != nil {
		return nil, err
	}
	if strings.TrimSpace(baseDir) == "" {
		var err error
		baseDir, err = localGetwd()
		if err != nil {
			return nil, fmt.Errorf("读取工作目录失败: %w", err)
		}
	}
	absBase, err := localAbs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("解析工作目录失败: %w", err)
	}
	realBase, err := localEvalSymlinks(absBase)
	if err != nil {
		return nil, fmt.Errorf("解析工作目录失败: %w", err)
	}
	baseRoot, err := openDownloadRoot(realBase)
	if err != nil {
		return nil, fmt.Errorf("打开工作目录失败: %w", err)
	}
	fail := func(err error) (*downloadTarget, error) {
		_ = baseRoot.Close()
		return nil, err
	}

	rawOutput := strings.TrimSpace(output)
	directoryIntent := rawOutput == "." || strings.HasSuffix(rawOutput, "/") || strings.HasSuffix(rawOutput, string(os.PathSeparator))
	candidate := filepath.Clean(rawOutput)
	if info, statErr := downloadRootStat(baseRoot, candidate); statErr == nil && info.IsDir() {
		directoryIntent = true
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return fail(fmt.Errorf("LOCAL_PATH_UNSAFE: 检查输出路径失败: %w", statErr))
	}
	if directoryIntent {
		candidate = filepath.Join(candidate, SafeFilename(preferredName, rawURL))
	}
	parent := filepath.Dir(candidate)
	if err := ensureSafeParent(baseRoot, parent); err != nil {
		return fail(err)
	}
	parentRoot, err := openDownloadParent(baseRoot, parent)
	if err != nil {
		return fail(fmt.Errorf("固定输出目录失败: %w", err))
	}
	parentInfo, err := downloadRootStat(parentRoot, ".")
	if err != nil {
		_ = parentRoot.Close()
		return fail(fmt.Errorf("读取输出目录身份失败: %w", err))
	}
	currentParent, err := downloadRootStat(baseRoot, parent)
	if err != nil || !os.SameFile(parentInfo, currentParent) {
		_ = parentRoot.Close()
		return fail(fmt.Errorf("LOCAL_PATH_CHANGED: 输出目录在解析期间被替换"))
	}
	destinationName := filepath.Base(candidate)
	if info, statErr := downloadRootLstat(parentRoot, destinationName); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			_ = parentRoot.Close()
			return fail(fmt.Errorf("LOCAL_PATH_UNSAFE: --output 目标不能是符号链接"))
		}
		if info.IsDir() {
			_ = parentRoot.Close()
			return fail(fmt.Errorf("LOCAL_PATH_UNSAFE: --output 目标是目录"))
		}
		_ = parentRoot.Close()
		return fail(fmt.Errorf("LOCAL_FILE_EXISTS: 目标文件已存在；请选择新的输出路径"))
	} else if !errors.Is(statErr, os.ErrNotExist) {
		_ = parentRoot.Close()
		return fail(fmt.Errorf("检查输出文件失败: %w", statErr))
	}
	return &downloadTarget{
		baseRoot:        baseRoot,
		parentRoot:      parentRoot,
		parentInfo:      parentInfo,
		parentRelative:  parent,
		destinationName: destinationName,
		absolutePath:    filepath.Join(realBase, candidate),
		relativePath:    candidate,
	}, nil
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
		// Do not use environment proxies here. DialContext must resolve and dial
		// the validated download host itself; with a proxy it would receive the
		// proxy address and could not enforce the target host's public-IP policy.
		Proxy: nil,
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
		if _, err := ValidateDownloadURL(req.URL.String()); err != nil {
			return err
		}
		// net/http copies arbitrary request headers from the initial request to
		// every redirect. Never forward service-provided download credentials to
		// a different origin, even when both hosts are on the download allowlist.
		if len(via) > 0 && !sameDownloadOrigin(via[0].URL, req.URL) {
			req.Header = make(http.Header)
		}
		return nil
	}
	return client
}

func sameDownloadOrigin(left, right *url.URL) bool {
	return downloadOrigin(left) == downloadOrigin(right)
}

func downloadOrigin(parsed *url.URL) string {
	port := parsed.Port()
	if port == "" {
		port = "443"
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	return strings.ToLower(parsed.Scheme) + "://" + net.JoinHostPort(host, port)
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

func ensureSafeParent(root *os.Root, parent string) error {
	if parent == "." {
		return nil
	}
	current := "."
	for _, part := range strings.Split(parent, string(os.PathSeparator)) {
		current = filepath.Join(current, part)
		info, statErr := downloadRootLstat(root, current)
		if errors.Is(statErr, os.ErrNotExist) {
			if err := downloadRootMkdir(root, current, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
				return fmt.Errorf("创建输出目录失败: %w", err)
			}
			info, statErr = downloadRootLstat(root, current)
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

func createDownloadTempInRoot(root *os.Root) (downloadTempFile, string, error) {
	name := fmt.Sprintf(".dws-download-%d-%d", os.Getpid(), downloadTempCounter.Add(1))
	file, err := root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, "", err
	}
	return file, name, nil
}

func publishTempFile(root *os.Root, tempName, destinationName string) error {
	if err := downloadRootLink(root, tempName, destinationName); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("LOCAL_FILE_EXISTS: 目标文件已存在")
		}
		return fmt.Errorf("发布下载文件失败: %w", err)
	}
	if err := downloadRootRemove(root, tempName); err != nil {
		return fmt.Errorf("清理下载临时文件失败: %w", err)
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
