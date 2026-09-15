// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"
	"unicode"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type personalIdentityOptions struct {
	ClientID   string
	TicketMode string
}

func personalIdentityOption(options []personalIdentityOptions) personalIdentityOptions {
	if len(options) > 0 {
		return options[0]
	}
	return personalIdentityOptions{}
}

// resolvePersonalClientID is event-local: do not mutate global OAuth routing or
// save the managed application ID beside a token whose app provenance is unknown.
func resolvePersonalClientID(ctx context.Context, configDir, clientID, mode string) (string, error) {
	if id := strings.TrimSpace(clientID); id != "" {
		return id, nil
	}
	mode = strings.TrimSpace(mode)
	if mode == "" || mode == "normal" {
		if id := personalClientIDMetadata(configDir); id != "" {
			return id, nil
		}
		if config.IsOpenEdition(edition.Get().Name) {
			return fetchPersonalEventClientID(ctx, personalEventMCPBaseURL(configDir))
		}
	} else {
		// Keep the existing paired credential path for custom applications.
		if id := strings.TrimSpace(personalClientID()); id != "" {
			return id, nil
		}
		if id, _, _, _, err := personalResolveAppCredentialsStrict(configDir); err == nil && strings.TrimSpace(id) != "" {
			return strings.TrimSpace(id), nil
		}
	}
	return "", apperrors.NewAuth("cannot resolve OAuth client_id for personal events", apperrors.WithReason("event_client_id_missing"), apperrors.WithRetryable(false), apperrors.WithHint("Check the selected profile or host application AppKey metadata. normal mode does not require an AppSecret."))
}

func personalClientIDFetchError(reason string, retryable bool) error {
	return apperrors.NewDiscovery("cannot obtain managed AppKey metadata for personal events", apperrors.WithReason(reason), apperrors.WithRetryable(retryable), apperrors.WithHint("Check the event MCP endpoint and network, then retry if indicated. normal mode does not require an AppSecret."))
}

func fetchPersonalEventClientID(ctx context.Context, baseURL string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+authpkg.ClientIDPath, nil)
	if err != nil || req.URL.User != nil || (req.URL.Scheme != "http" && req.URL.Scheme != "https") || req.URL.Host == "" {
		return "", personalClientIDFetchError("event_client_id_configuration", false)
	}
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		// Never include transport errors: URLs and proxy diagnostics can carry secrets.
		var networkError net.Error
		var dnsError *net.DNSError
		retryable := !errors.Is(ctx.Err(), context.Canceled) && (errors.Is(err, context.DeadlineExceeded) ||
			errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
			errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) ||
			(errors.As(err, &networkError) && networkError.Timeout()) ||
			(errors.As(err, &dnsError) && dnsError.IsTemporary))
		return "", personalClientIDFetchError("event_client_id_fetch_failed", retryable)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", personalClientIDFetchError("event_client_id_fetch_failed", resp.StatusCode == 429 || resp.StatusCode >= 500)
	}
	const maxMetadataBytes = 64 * 1024
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxMetadataBytes+1))
	if err != nil {
		return "", personalClientIDFetchError("event_client_id_fetch_failed", !errors.Is(ctx.Err(), context.Canceled))
	}
	var result authpkg.ClientIDResponse
	if len(body) > maxMetadataBytes || json.Unmarshal(body, &result) != nil {
		return "", personalClientIDFetchError("event_client_id_invalid_response", false)
	}
	if !result.Success {
		return "", personalClientIDFetchError("event_client_id_rejected", false)
	}
	id := strings.TrimSpace(result.Result)
	if id == "" || len(id) > 1024 || strings.ContainsAny(id, "<>") || strings.IndexFunc(id, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return "", personalClientIDFetchError("event_client_id_invalid_response", false)
	}
	return id, nil
}
