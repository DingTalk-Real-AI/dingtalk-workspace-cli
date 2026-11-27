// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DownloadSource controls resource selection, not release or withdrawal authority.
type DownloadSource string

const (
	SourceAuto      DownloadSource = "auto"
	SourceGitHub    DownloadSource = "github"
	SourceNPMMirror DownloadSource = "npmmirror"
)

func ParseDownloadSource(value string) (DownloadSource, error) {
	switch DownloadSource(value) {
	case SourceAuto, SourceGitHub, SourceNPMMirror:
		return DownloadSource(value), nil
	default:
		return "", fmt.Errorf("--source 必须为 auto、github 或 npmmirror")
	}
}

func (s DownloadSource) normalized() DownloadSource {
	if s == "" {
		return SourceAuto
	}
	return s
}

// NewClientForSource retains the main package's availability/deprecation checks.
// Explicit modes override source environment variables without mutating them.
func NewClientForSource(source DownloadSource) *Client {
	source = source.normalized()
	if source == SourceAuto {
		return NewClient()
	}
	client := &Client{httpClient: &http.Client{Timeout: httpTimeout}}
	if _, err := ParseDownloadSource(string(source)); err != nil {
		client.configErr = err
	} else if !publicUpgradeEdition(upgradeEditionName()) {
		client.configErr = fmt.Errorf("--source %s 仅支持公共发行版；当前发行版请使用 auto 和专属升级源", source)
	}
	registry := defaultNPMRegistry
	if source == SourceNPMMirror {
		registry = publicNPMMirror
	}
	client.registryURL = registry + "/" + npmPackageName
	return client
}

// DownloadEvent is synchronous progress telemetry. Reasons never include URLs
// or raw transport errors, which may contain credentials or signed queries.
type DownloadEvent struct {
	Kind        string // source, switch, attempt
	Source      string
	Previous    string
	Resource    string
	Reason      string
	Attempt     int // includes the initial request
	MaxAttempts int
	ResumeBytes int64
	RetryDelay  time.Duration
}

type downloadObserverKey struct{}

// WithDownloadObserver attaches a request-scoped observer, including nested
// package downloads. It does not change source selection or retry policy.
func WithDownloadObserver(ctx context.Context, observe func(DownloadEvent)) context.Context {
	return context.WithValue(ctx, downloadObserverKey{}, observe)
}

func reportDownload(ctx context.Context, event DownloadEvent) {
	if observe, ok := ctx.Value(downloadObserverKey{}).(func(DownloadEvent)); ok && observe != nil {
		observe(event)
	}
}

func downloadSourceName(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "已配置来源"
	}
	host := strings.ToLower(u.Hostname())
	switch {
	case host == "github.com", host == "api.github.com", strings.HasSuffix(host, ".githubusercontent.com"):
		return string(SourceGitHub)
	case host == "npmmirror.com", strings.HasSuffix(host, ".npmmirror.com"):
		return string(SourceNPMMirror)
	case host == "registry.npmjs.org":
		return "npmjs"
	case host == "":
		return "已配置来源"
	default:
		return host
	}
}

func downloadFailureReason(err error) string {
	var status *downloadHTTPError
	switch {
	case errors.Is(err, errRestartDownload):
		return "资源身份或范围改变，重新下载"
	case errors.As(err, &status):
		return fmt.Sprintf("HTTP %d", status.status)
	case errors.Is(err, context.DeadlineExceeded):
		return "下载超时或停滞"
	default:
		return "网络连接失败或传输中断"
	}
}
