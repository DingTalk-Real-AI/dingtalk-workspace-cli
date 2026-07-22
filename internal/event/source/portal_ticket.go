// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package source

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/gorilla/websocket"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/payload"
)

const (
	PortalTicketModeNormal = "normal"
	PortalTicketModeCustom = "custom"
	portalReconnectMin     = time.Second
	portalReconnectMax     = 30 * time.Second
)

// PortalTicketConfig describes the portal-managed user Stream ticket flow.
// normal mode uses portal-side managed credentials; custom mode asks portal to
// open the user connection with the caller-provided clientId/clientSecret.
type PortalTicketConfig struct {
	TicketURL           string
	AccessToken         string
	AccessTokenProvider AccessTokenProvider
	ForceRefreshToken   ForceRefreshTokenFn
	SourceID            string
	Mode                string
	ClientID            string
	ClientSecret        string
	UserAgent           string
	HTTPClient          *http.Client
	WebSocketDialer     *websocket.Dialer
	ReconnectMin        time.Duration
	ReconnectMax        time.Duration
	DisableReconnect    bool
}

// portalStageError tags a portal stream failure with the stage it happened
// in and whether the reconnect loop may retry it. Error() stays free of
// untrusted response content so it is safe to log on every reconnect.
type portalStageError struct {
	stage     string
	status    int
	retryable bool
	cause     error
}

func (e *portalStageError) Error() string {
	if e == nil {
		return "source: portal stream failed"
	}
	message := "source: portal " + strings.ReplaceAll(strings.TrimSpace(e.stage), "_", " ") + " failed"
	if e.status != 0 {
		message += fmt.Sprintf(" (HTTP %d)", e.status)
	}
	return message
}

func (e *portalStageError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

var portalWriteMessage = func(conn *websocket.Conn, messageType int, data []byte) error {
	return conn.WriteMessage(messageType, data)
}

func (c *PortalTicketConfig) Valid() error {
	if c == nil {
		return errors.New("source: PortalTicketConfig is nil")
	}
	if strings.TrimSpace(c.TicketURL) == "" {
		return errors.New("source: portal ticket URL is required")
	}
	if c.AccessTokenProvider == nil && strings.TrimSpace(c.AccessToken) == "" {
		return errors.New("source: portal access token or provider is required")
	}
	if strings.TrimSpace(c.SourceID) == "" {
		return errors.New("source: portal sourceId is required")
	}
	mode := normalizePortalTicketMode(c.Mode)
	if mode == "" {
		return fmt.Errorf("source: unsupported portal ticket mode %q", c.Mode)
	}
	if mode == PortalTicketModeCustom &&
		(strings.TrimSpace(c.ClientID) == "" || strings.TrimSpace(c.ClientSecret) == "") {
		return errors.New("source: custom portal ticket mode requires clientId/clientSecret")
	}
	return nil
}

func (c *PortalTicketConfig) normalizedMode() string {
	return normalizePortalTicketMode(c.Mode)
}

func normalizePortalTicketMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return PortalTicketModeNormal
	}
	switch mode {
	case PortalTicketModeNormal, PortalTicketModeCustom:
		return mode
	default:
		return ""
	}
}

func (s *DingtalkSource) startPortalTicket(ctx context.Context, emit dwsevent.EmitFn) error {
	s.machine.OnConnecting()
	defer s.machine.OnStopped()

	minBackoff := s.cfg.PortalTicket.ReconnectMin
	if minBackoff <= 0 {
		minBackoff = portalReconnectMin
	}
	maxBackoff := s.cfg.PortalTicket.ReconnectMax
	if maxBackoff <= 0 {
		maxBackoff = portalReconnectMax
	}
	if maxBackoff < minBackoff {
		maxBackoff = minBackoff
	}
	backoff := minBackoff
	for {
		acked, err := s.runPortalTicketAttempt(ctx, emit)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var stageErr *portalStageError
		if !errors.As(err, &stageErr) || stageErr == nil || !stageErr.retryable || s.cfg.PortalTicket.DisableReconnect {
			return err
		}
		if acked {
			backoff = minBackoff
		}
		s.machine.OnReconnect()
		slog.Warn("portal source reconnecting",
			"stage", stageErr.stage,
			"http_status", stageErr.status,
			"error_type", fmt.Sprintf("%T", stageErr.cause),
			"retry_in", backoff,
			"reconnect_count", s.machine.Snapshot().ReconnectCount,
		)
		if err := waitPersonalReconnect(ctx, backoff); err != nil {
			return err
		}
		backoff = nextPersonalBackoff(backoff, maxBackoff)
	}
}

func (s *DingtalkSource) runPortalTicketAttempt(ctx context.Context, emit dwsevent.EmitFn) (bool, error) {
	ticket, err := requestPortalTicket(ctx, s.cfg.PortalTicket)
	if err != nil {
		return false, err
	}
	wsURL, err := websocketURL(ticket)
	if err != nil {
		return false, err
	}

	userAgent := strings.TrimSpace(s.cfg.PortalTicket.UserAgent)
	if userAgent == "" {
		userAgent = "dws-event-consume"
	}
	dialer := s.cfg.PortalTicket.WebSocketDialer
	if dialer == nil {
		dialer = &websocket.Dialer{HandshakeTimeout: 20 * time.Second}
	}
	conn, resp, err := dialer.DialContext(ctx, wsURL, http.Header{
		"User-Agent": []string{userAgent},
	})
	if err != nil {
		status := 0
		cause := fmt.Errorf("source: portal stream connect: %w", err)
		if resp != nil {
			defer resp.Body.Close()
			status = resp.StatusCode
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			cause = fmt.Errorf("source: portal stream connect HTTP %d: %s: %w",
				resp.StatusCode, truncatePortalTicketLog(string(raw), 300), err)
		}
		return false, &portalStageError{
			stage:     "stream_connect",
			status:    status,
			retryable: status == 0 || retryableTicketStatus(status),
			cause:     cause,
		}
	}
	attemptCtx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel()
		_ = conn.Close()
	}()
	closeOnContext(attemptCtx, conn)
	s.machine.OnConnected()

	handler := s.makeHandler(emit)
	acked := false
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if isContextDone(ctx) {
				return acked, ctx.Err()
			}
			return acked, &portalStageError{stage: "stream_read", retryable: true, cause: fmt.Errorf("source: portal stream read: %w", err)}
		}
		df, err := payload.DecodeDataFrame(message)
		if err != nil {
			continue
		}
		resp, _ := handler(ctx, df)
		ensurePortalAckHeaders(resp, df)
		if err := portalWriteMessage(conn, websocket.TextMessage, resp.Encode()); err != nil {
			if isContextDone(ctx) {
				return acked, ctx.Err()
			}
			return acked, &portalStageError{stage: "stream_ack", retryable: true, cause: fmt.Errorf("source: portal stream ack: %w", err)}
		}
		acked = true
	}
}

type portalStreamTicket struct {
	Endpoint string `json:"endpoint"`
	Ticket   string `json:"ticket"`
}

func requestPortalTicket(ctx context.Context, cfg *PortalTicketConfig) (portalStreamTicket, error) {
	accessToken, err := resolveSourceAccessToken(ctx, cfg.AccessTokenProvider, cfg.AccessToken, "source: portal ticket")
	if err != nil {
		// Transient provider failures (network, 429, 5xx) must not kill a
		// long-running source; the reconnect loop retries after backoff.
		if authpkg.ClassifyRefreshFailure(err) == authpkg.RefreshFailureTransient {
			return portalStreamTicket{}, &portalStageError{stage: "ticket_auth", status: refreshHTTPStatus(err), retryable: true, cause: err}
		}
		return portalStreamTicket{}, err
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	ticket, status, err := requestPortalTicketAttempt(ctx, cfg, httpClient, accessToken)
	if status == http.StatusUnauthorized && cfg.ForceRefreshToken != nil {
		refreshed, refreshErr := refreshRejectedSourceToken(ctx, cfg.ForceRefreshToken, accessToken, "source: portal ticket", err)
		if refreshErr != nil {
			if authpkg.ClassifyRefreshFailure(refreshErr) == authpkg.RefreshFailureTransient {
				return portalStreamTicket{}, &portalStageError{stage: "ticket_auth_refresh", status: refreshHTTPStatus(refreshErr), retryable: true, cause: refreshErr}
			}
			return portalStreamTicket{}, refreshErr
		}
		// Retry once with the freshly rotated token; a second 401 stays fatal.
		ticket, _, err = requestPortalTicketAttempt(ctx, cfg, httpClient, refreshed)
	}
	return ticket, err
}

func requestPortalTicketAttempt(ctx context.Context, cfg *PortalTicketConfig, httpClient *http.Client, accessToken string) (portalStreamTicket, int, error) {
	body := map[string]string{
		"sourceId":    strings.TrimSpace(cfg.SourceID),
		"channelType": strings.TrimSpace(cfg.SourceID),
		"mode":        cfg.normalizedMode(),
	}
	if cfg.normalizedMode() == PortalTicketModeCustom {
		body["clientId"] = strings.TrimSpace(cfg.ClientID)
		body["clientSecret"] = strings.TrimSpace(cfg.ClientSecret)
	}
	rawBody, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSpace(cfg.TicketURL), bytes.NewReader(rawBody))
	if err != nil {
		return portalStreamTicket{}, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if ua := strings.TrimSpace(cfg.UserAgent); ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	req.Header.Set("x-user-access-token", accessToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return portalStreamTicket{}, 0, &portalStageError{stage: "ticket_request", retryable: true, cause: fmt.Errorf("source: portal ticket request: %w", err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Preserve HTTP status semantics before touching the body. A truncated
		// 401 body must not become a retryable read error that bypasses the
		// single token-refresh guard; the body is only best-effort diagnostics.
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		httpErr := fmt.Errorf("source: portal ticket HTTP %d: %s",
			resp.StatusCode, truncatePortalTicketLog(string(raw), 300))
		if retryableTicketStatus(resp.StatusCode) {
			return portalStreamTicket{}, resp.StatusCode, &portalStageError{stage: "ticket_request", status: resp.StatusCode, retryable: true, cause: httpErr}
		}
		return portalStreamTicket{}, resp.StatusCode, httpErr
	}
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return portalStreamTicket{}, resp.StatusCode, &portalStageError{
			stage:     "ticket_request",
			status:    resp.StatusCode,
			retryable: true,
			cause:     fmt.Errorf("source: portal ticket read: %w", readErr),
		}
	}

	var direct portalStreamTicket
	if err := json.Unmarshal(raw, &direct); err == nil && direct.Endpoint != "" && direct.Ticket != "" {
		return direct, resp.StatusCode, nil
	}

	var envelope struct {
		Success   bool               `json:"success"`
		Result    portalStreamTicket `json:"result"`
		ErrorCode string             `json:"errorCode"`
		ErrorMsg  string             `json:"errorMsg"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return portalStreamTicket{}, resp.StatusCode, fmt.Errorf("source: portal ticket parse: %w", err)
	}
	if !envelope.Success {
		return portalStreamTicket{}, resp.StatusCode, fmt.Errorf("source: portal ticket failed: %s %s",
			envelope.ErrorCode, envelope.ErrorMsg)
	}
	if envelope.Result.Endpoint == "" || envelope.Result.Ticket == "" {
		return portalStreamTicket{}, resp.StatusCode, errors.New("source: portal ticket result missing endpoint/ticket")
	}
	return envelope.Result, resp.StatusCode, nil
}

func websocketURL(ticket portalStreamTicket) (string, error) {
	u, err := url.Parse(ticket.Endpoint)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("ticket", ticket.Ticket)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func ensurePortalAckHeaders(resp *payload.DataFrameResponse, df *payload.DataFrame) {
	if resp.GetHeader(payload.DataFrameHeaderKMessageId) == "" {
		resp.SetHeader(payload.DataFrameHeaderKMessageId, df.GetMessageId())
	}
	if resp.GetHeader(payload.DataFrameHeaderKContentType) == "" {
		resp.SetHeader(payload.DataFrameHeaderKContentType, payload.DataFrameContentTypeKJson)
	}
}

func closeOnContext(ctx context.Context, conn *websocket.Conn) {
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()
}

func isContextDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

func truncatePortalTicketLog(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
