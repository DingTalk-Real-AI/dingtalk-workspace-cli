// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultDownloadMaxBytes = int64(256 << 20)
	maxDownloadRedirects    = 3
)

// DownloadConfig holds download configuration.
type DownloadConfig struct {
	MaxRetries       int
	Timeout          time.Duration
	MaxBytes         int64
	HTTPClient       *http.Client
	ProgressCallback func(downloaded, total int64)
	ProgressInterval time.Duration
}

type downloadHTTPError struct {
	status     int
	url        string
	retryAfter time.Duration
}

func (e *downloadHTTPError) Error() string {
	return fmt.Sprintf("下载失败 (HTTP %d): %s", e.status, e.url)
}

var (
	downloadAfter      = time.After
	downloadMkdirAll   = os.MkdirAll
	downloadCreateTemp = os.CreateTemp
	downloadChmod      = func(file *os.File, mode os.FileMode) error { return file.Chmod(mode) }
	downloadSync       = func(file *os.File) error { return file.Sync() }
	downloadClose      = func(file *os.File) error { return file.Close() }
	downloadRename     = os.Rename
)

// DefaultDownloadConfig returns bounded defaults for release assets.
func DefaultDownloadConfig() *DownloadConfig {
	return &DownloadConfig{
		MaxRetries:       3,
		Timeout:          10 * time.Minute,
		MaxBytes:         defaultDownloadMaxBytes,
		ProgressInterval: 200 * time.Millisecond,
	}
}

// Download fetches url to destPath with default config.
func Download(url, destPath string) (int64, error) {
	return DownloadWithConfig(context.Background(), url, destPath, DefaultDownloadConfig())
}

// DownloadWithProgress downloads a file and reports progress.
func DownloadWithProgress(ctx context.Context, url, destPath string, showProgress func(percent float64, downloaded, total int64)) (int64, error) {
	cfg := DefaultDownloadConfig()
	cfg.ProgressCallback = func(downloaded, total int64) {
		if showProgress != nil && total > 0 {
			percent := float64(downloaded) / float64(total) * 100
			showProgress(percent, downloaded, total)
		}
	}
	return DownloadWithConfig(ctx, url, destPath, cfg)
}

// DownloadWithConfig fetches a file with bounded redirects, atomic publication,
// and retries for transient transport and HTTP failures.
func DownloadWithConfig(ctx context.Context, url, destPath string, cfg *DownloadConfig) (int64, error) {
	if cfg == nil {
		cfg = DefaultDownloadConfig()
	}
	if err := downloadMkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return 0, fmt.Errorf("创建目录失败: %w", err)
	}

	maxRetries := cfg.MaxRetries
	if maxRetries < 1 {
		maxRetries = 1
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := retryDelay(lastErr, attempt)
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-downloadAfter(backoff):
			}
		}

		n, err := doDownload(ctx, url, destPath, cfg)
		if err == nil {
			return n, nil
		}
		lastErr = err

		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		if !isRetriable(err) {
			return 0, err
		}
	}

	return 0, fmt.Errorf("下载失败 (重试 %d 次后): %w", maxRetries, lastErr)
}

func retryDelay(lastErr error, attempt int) time.Duration {
	var statusErr *downloadHTTPError
	if errors.As(lastErr, &statusErr) && statusErr.retryAfter > 0 {
		if statusErr.retryAfter > 30*time.Second {
			return 30 * time.Second
		}
		return statusErr.retryAfter
	}
	backoff := time.Second * time.Duration(1<<uint(attempt-1))
	if backoff > 30*time.Second {
		return 30 * time.Second
	}
	return backoff
}

func doDownload(ctx context.Context, url, destPath string, cfg *DownloadConfig) (n int64, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	originalScheme := strings.ToLower(req.URL.Scheme)
	client := &http.Client{}
	if cfg.HTTPClient != nil {
		clone := *cfg.HTTPClient
		client = &clone
	}
	client.Timeout = cfg.Timeout
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= maxDownloadRedirects {
			return fmt.Errorf("重定向次数超过 %d", maxDownloadRedirects)
		}
		if originalScheme == "https" && !strings.EqualFold(next.URL.Scheme, "https") {
			return fmt.Errorf("拒绝从 HTTPS 重定向到 %s", next.URL.Redacted())
		}
		return nil
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("下载失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, &downloadHTTPError{
			status:     resp.StatusCode,
			url:        req.URL.Redacted(),
			retryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()),
		}
	}

	maxBytes := cfg.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultDownloadMaxBytes
	}
	if resp.ContentLength > maxBytes {
		return 0, fmt.Errorf("下载内容过大: %d bytes（上限 %d）", resp.ContentLength, maxBytes)
	}

	tmp, err := downloadCreateTemp(filepath.Dir(destPath), "."+filepath.Base(destPath)+".*.part")
	if err != nil {
		return 0, fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpPath := tmp.Name()
	published := false
	defer func() {
		_ = downloadClose(tmp)
		if !published {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := downloadChmod(tmp, 0o600); err != nil {
		return 0, fmt.Errorf("设置临时文件权限失败: %w", err)
	}

	var writer io.Writer = tmp
	var progress *progressWriter
	if cfg.ProgressCallback != nil {
		progress = &progressWriter{
			writer:       tmp,
			total:        resp.ContentLength,
			callback:     cfg.ProgressCallback,
			interval:     cfg.ProgressInterval,
			lastCallback: time.Now(),
		}
		writer = progress
	}

	n, err = io.Copy(writer, io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return 0, fmt.Errorf("写入文件失败: %w", err)
	}
	if n > maxBytes {
		return 0, fmt.Errorf("下载内容超过上限 %d bytes", maxBytes)
	}
	if resp.ContentLength >= 0 && n != resp.ContentLength {
		return 0, fmt.Errorf("下载内容不完整: 期望 %d bytes，实际 %d", resp.ContentLength, n)
	}
	if n == 0 {
		return 0, fmt.Errorf("下载内容为空")
	}
	if err := downloadSync(tmp); err != nil {
		return 0, fmt.Errorf("同步下载文件失败: %w", err)
	}
	if err := downloadClose(tmp); err != nil {
		return 0, fmt.Errorf("关闭下载文件失败: %w", err)
	}
	if err := downloadRename(tmpPath, destPath); err != nil {
		return 0, fmt.Errorf("发布下载文件失败: %w", err)
	}
	published = true
	if progress != nil {
		progress.finalProgress()
	}
	return n, nil
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
		return 0
	}
	if when, err := http.ParseTime(value); err == nil && when.After(now) {
		return when.Sub(now)
	}
	return 0
}

// progressWriter wraps an io.Writer and reports download progress.
type progressWriter struct {
	writer       io.Writer
	total        int64
	downloaded   int64
	callback     func(downloaded, total int64)
	interval     time.Duration
	lastCallback time.Time
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.writer.Write(p)
	pw.downloaded += int64(n)

	now := time.Now()
	if pw.callback != nil && now.Sub(pw.lastCallback) >= pw.interval {
		pw.callback(pw.downloaded, pw.total)
		pw.lastCallback = now
	}
	return n, err
}

func (pw *progressWriter) finalProgress() {
	if pw.callback != nil {
		pw.callback(pw.downloaded, pw.total)
	}
}

func isRetriable(err error) bool {
	if err == nil {
		return false
	}
	var statusErr *downloadHTTPError
	if errors.As(err, &statusErr) {
		return statusErr.status == http.StatusRequestTimeout ||
			statusErr.status == http.StatusTooEarly ||
			statusErr.status == http.StatusTooManyRequests ||
			statusErr.status >= http.StatusInternalServerError
	}
	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	errStr := strings.ToLower(err.Error())
	for _, pattern := range []string{"connection reset", "connection refused", "timeout", "temporary failure", "unexpected eof", "下载内容不完整"} {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}
	return false
}
