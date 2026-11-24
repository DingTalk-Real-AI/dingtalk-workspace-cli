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
	"sync/atomic"
	"time"
)

// DownloadConfig holds download configuration.
type DownloadConfig struct {
	MaxRetries       int
	Timeout          time.Duration
	ProgressCallback func(downloaded, total int64)
	ProgressInterval time.Duration
	// StallTimeout bounds a body read without progress; Timeout bounds an attempt.
	StallTimeout  time.Duration
	MaxBytes      int64
	CheckRedirect func(*http.Request, []*http.Request) error
}

var (
	downloadAfter      = time.After
	downloadCreateTemp = os.CreateTemp
	downloadOpenFile   = os.OpenFile
)

// DefaultDownloadConfig returns sensible download defaults.
func DefaultDownloadConfig() *DownloadConfig {
	return &DownloadConfig{
		MaxRetries:       3,
		Timeout:          10 * time.Minute,
		ProgressInterval: 200 * time.Millisecond,
		StallTimeout:     15 * time.Second,
		MaxBytes:         1 << 30,
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

// DownloadWithConfig fetches a file with custom configuration and retries.
func DownloadWithConfig(ctx context.Context, url, destPath string, cfg *DownloadConfig) (int64, error) {
	if cfg == nil {
		cfg = DefaultDownloadConfig()
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return 0, &downloadFileError{fmt.Errorf("创建目录失败: %w", err)}
	}
	partial, err := downloadCreateTemp(filepath.Dir(destPath), ".dws-download-*")
	if err != nil {
		return 0, &downloadFileError{err}
	}
	partialPath := partial.Name()
	if err := partial.Close(); err != nil {
		os.Remove(partialPath)
		return 0, &downloadFileError{err}
	}
	defer os.Remove(partialPath)
	state := &downloadState{}

	maxRetries := cfg.MaxRetries
	if maxRetries < 1 {
		maxRetries = 1
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		event := DownloadEvent{Kind: "attempt", Source: downloadSourceName(url), Resource: filepath.Base(destPath), Attempt: attempt + 1, MaxAttempts: maxRetries}
		if state.offset > 0 && state.etag != "" && state.total > state.offset {
			event.ResumeBytes = state.offset
		}
		if attempt > 0 {
			backoff := time.Second * time.Duration(1<<uint(min(attempt-1, 5)))
			backoff = min(backoff, 30*time.Second)
			var status *downloadHTTPError
			if errors.As(lastErr, &status) {
				backoff = max(backoff, status.retryAfter)
			}
			event.Reason, event.RetryDelay = downloadFailureReason(lastErr), backoff
			reportDownload(ctx, event)
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-downloadAfter(backoff):
			}
		} else {
			reportDownload(ctx, event)
		}

		n, err := downloadAttempt(ctx, url, partialPath, cfg, state)
		if err == nil {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			if err := os.Rename(partialPath, destPath); err != nil {
				return 0, &downloadFileError{fmt.Errorf("发布下载文件失败: %w", err)}
			}
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

	return 0, fmt.Errorf("下载失败 (尝试 %d 次，重试 %d 次): %w", maxRetries, maxRetries-1, lastErr)
}

func doDownload(ctx context.Context, url, destPath string, cfg *DownloadConfig) (int64, error) {
	return downloadAttempt(ctx, url, destPath, cfg, &downloadState{})
}

type downloadState struct {
	offset int64
	total  int64
	etag   string
}

type downloadHTTPError struct {
	status     int
	retryAfter time.Duration
}

func (e *downloadHTTPError) Error() string { return fmt.Sprintf("下载失败 (HTTP %d)", e.status) }

type downloadFileError struct{ err error }

func (e *downloadFileError) Error() string { return e.err.Error() }
func (e *downloadFileError) Unwrap() error { return e.err }

var errRestartDownload = errors.New("资源身份或范围已改变，需要重新下载")

func downloadAttempt(ctx context.Context, url, destPath string, cfg *DownloadConfig, state *downloadState) (int64, error) {
	requestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, "GET", url, nil)
	if err != nil {
		return 0, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept-Encoding", "identity")
	resume := state.offset > 0 && state.etag != "" && state.total > state.offset
	if resume {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", state.offset))
		// Some GitHub CDN paths ignore If-Range. If-Match plus response
		// identity validation prevents appending a different representation.
		req.Header.Set("If-Match", state.etag)
	}

	transport := http.DefaultTransport
	if base, ok := transport.(*http.Transport); ok {
		tuned := base.Clone()
		tuned.DialContext = (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext
		tuned.TLSHandshakeTimeout = 5 * time.Second
		tuned.ResponseHeaderTimeout = 8 * time.Second
		defer tuned.CloseIdleConnections()
		transport = tuned
	}
	client := &http.Client{Transport: transport, CheckRedirect: func(next *http.Request, via []*http.Request) error {
		if len(via) >= 10 || next.URL.User != nil || req.URL.Scheme == "https" && next.URL.Scheme != "https" {
			return fmt.Errorf("拒绝不安全的下载重定向")
		}
		return nil
	}}
	if cfg.CheckRedirect != nil {
		client.CheckRedirect = cfg.CheckRedirect
	}
	if cfg.Timeout > 0 {
		client.Timeout = cfg.Timeout
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("下载失败: %w", err)
	}
	defer resp.Body.Close()

	if resume && (resp.StatusCode == http.StatusPreconditionFailed || resp.StatusCode == http.StatusRequestedRangeNotSatisfiable) {
		*state = downloadState{}
		return 0, errRestartDownload
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return 0, &downloadHTTPError{status: resp.StatusCode, retryAfter: retryAfterDuration(resp.Header.Get("Retry-After"))}
	}
	if encoding := resp.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		return 0, fmt.Errorf("下载响应不支持的 Content-Encoding: %s", encoding)
	}
	appendFile := false
	if resp.StatusCode == http.StatusPartialContent {
		start, end, total, valid := parseDownloadRange(resp.Header.Get("Content-Range"))
		if !resume || !valid || start != state.offset || total != state.total || end != total-1 ||
			resp.Header.Get("ETag") != state.etag || (resp.ContentLength >= 0 && resp.ContentLength != end-start+1) {
			*state = downloadState{}
			return 0, errRestartDownload
		}
		appendFile = true
	} else {
		*state = downloadState{total: resp.ContentLength, etag: strongDownloadETag(resp.Header.Get("ETag"))}
	}
	if cfg.MaxBytes > 0 && state.total > cfg.MaxBytes {
		return 0, fmt.Errorf("下载文件超过大小上限")
	}
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if appendFile {
		flags = os.O_WRONLY | os.O_APPEND
	}
	out, err := downloadOpenFile(destPath, flags, 0600)
	if err != nil {
		return 0, &downloadFileError{fmt.Errorf("创建文件失败: %w", err)}
	}
	defer out.Close()
	if appendFile {
		info, statErr := out.Stat()
		if statErr != nil {
			return 0, &downloadFileError{statErr}
		}
		if info.Size() != state.offset {
			return 0, &downloadFileError{errors.New("下载临时文件长度发生变化")}
		}
	}

	fileWriter := &downloadWriter{file: out}
	var writer io.Writer = fileWriter
	if cfg.ProgressCallback != nil {
		pw := &progressWriter{
			writer:       fileWriter,
			total:        state.total,
			downloaded:   state.offset,
			callback:     cfg.ProgressCallback,
			interval:     cfg.ProgressInterval,
			lastCallback: time.Now(),
		}
		defer pw.finalProgress()
		writer = pw
	}

	var stalled atomic.Bool
	var reader io.Reader = resp.Body
	if cfg.StallTimeout > 0 {
		timer := time.AfterFunc(cfg.StallTimeout, func() { stalled.Store(true); cancel() })
		defer timer.Stop()
		reader = &downloadIdleReader{reader: reader, timer: timer, timeout: cfg.StallTimeout}
	}
	if cfg.MaxBytes > 0 {
		reader = io.LimitReader(reader, cfg.MaxBytes-state.offset+1)
	}
	n, err := io.Copy(writer, reader)
	state.offset += n
	if fileWriter.err != nil {
		return 0, &downloadFileError{fileWriter.err}
	}
	if cfg.MaxBytes > 0 && state.offset > cfg.MaxBytes {
		return 0, fmt.Errorf("下载文件超过大小上限")
	}
	if stalled.Load() {
		return 0, fmt.Errorf("下载停滞 timeout: %w", context.DeadlineExceeded)
	}
	if err != nil {
		return 0, fmt.Errorf("读取下载数据失败: %w", err)
	}
	if state.total >= 0 && state.offset != state.total {
		return 0, io.ErrUnexpectedEOF
	}
	if err := out.Close(); err != nil {
		return 0, &downloadFileError{err}
	}
	return state.offset, nil
}

type downloadWriter struct {
	file *os.File
	err  error
}

func (w *downloadWriter) Write(p []byte) (int, error) {
	n, err := w.file.Write(p)
	if err != nil {
		w.err = err
	}
	return n, err
}

type downloadIdleReader struct {
	reader  io.Reader
	timer   *time.Timer
	timeout time.Duration
}

func (r *downloadIdleReader) Read(p []byte) (int, error) {
	r.timer.Reset(r.timeout)
	n, err := r.reader.Read(p)
	// A slow filesystem or progress callback is not a network body stall.
	r.timer.Stop()
	return n, err
}

func strongDownloadETag(value string) string {
	if len(value) >= 2 && strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
		return value
	}
	return ""
}

func parseDownloadRange(value string) (int64, int64, int64, bool) {
	if !strings.HasPrefix(value, "bytes ") {
		return 0, 0, 0, false
	}
	parts := strings.Split(strings.TrimPrefix(value, "bytes "), "/")
	if len(parts) != 2 {
		return 0, 0, 0, false
	}
	bounds := strings.Split(parts[0], "-")
	if len(bounds) != 2 {
		return 0, 0, 0, false
	}
	start, e1 := strconv.ParseInt(bounds[0], 10, 64)
	end, e2 := strconv.ParseInt(bounds[1], 10, 64)
	total, e3 := strconv.ParseInt(parts[1], 10, 64)
	return start, end, total, e1 == nil && e2 == nil && e3 == nil && start >= 0 && end >= start && total > end
}

func retryAfterDuration(value string) time.Duration {
	if seconds, err := strconv.ParseInt(value, 10, 32); err == nil {
		return time.Duration(max(0, min(seconds, 30))) * time.Second
	}
	if deadline, err := http.ParseTime(value); err == nil {
		return max(0, min(time.Until(deadline), 30*time.Second))
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
	var fileErr *downloadFileError
	if errors.As(err, &fileErr) || errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, errRestartDownload) {
		return true
	}
	var status *downloadHTTPError
	if errors.As(err, &status) {
		switch status.status {
		case 408, 429, 500, 502, 503, 504:
			return true
		default:
			return false
		}
	}
	if os.IsTimeout(err) {
		return true
	}
	errStr := err.Error()
	for _, pattern := range []string{"connection reset", "connection refused", "timeout", "temporary failure", "eof"} {
		if strings.Contains(strings.ToLower(errStr), pattern) {
			return true
		}
	}
	return false
}
