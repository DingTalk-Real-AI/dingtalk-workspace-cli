// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DownloadConfig holds download configuration.
type DownloadConfig struct {
	MaxRetries       int
	Timeout          time.Duration
	ProgressCallback func(downloaded, total int64)
	ProgressInterval time.Duration
}

var downloadAfter = time.After

// DefaultDownloadConfig returns sensible download defaults.
func DefaultDownloadConfig() *DownloadConfig {
	return &DownloadConfig{
		MaxRetries:       3,
		Timeout:          10 * time.Minute,
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

// DownloadWithConfig fetches a file with custom configuration and retries.
func DownloadWithConfig(ctx context.Context, url, destPath string, cfg *DownloadConfig) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return 0, fmt.Errorf("创建目录失败: %w", err)
	}

	maxRetries := cfg.MaxRetries
	if maxRetries < 1 {
		maxRetries = 1
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Second * time.Duration(1<<uint(attempt-1))
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
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
		os.Remove(destPath)
	}

	return 0, fmt.Errorf("下载失败 (重试 %d 次后): %w", maxRetries, lastErr)
}

func doDownload(ctx context.Context, url, destPath string, cfg *DownloadConfig) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	client := &http.Client{}
	if cfg.Timeout > 0 {
		client.Timeout = cfg.Timeout
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("下载失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("下载失败 (HTTP %d): %s", resp.StatusCode, url)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return 0, fmt.Errorf("创建文件失败: %w", err)
	}
	defer out.Close()

	var writer io.Writer = out
	if cfg.ProgressCallback != nil {
		pw := &progressWriter{
			writer:       out,
			total:        resp.ContentLength,
			callback:     cfg.ProgressCallback,
			interval:     cfg.ProgressInterval,
			lastCallback: time.Now(),
		}
		defer pw.finalProgress()
		writer = pw
	}

	n, err := io.Copy(writer, resp.Body)
	if err != nil {
		return 0, fmt.Errorf("写入文件失败: %w", err)
	}

	return n, nil
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
