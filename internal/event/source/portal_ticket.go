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
	"net/http"
	"net/url"
	"strings"
	"time"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/gorilla/websocket"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/payload"
)

const (
	PortalTicketModeNormal = "normal"
	PortalTicketModeCustom = "custom"
)

// PortalTicketConfig describes the portal-managed user Stream ticket flow.
// normal mode uses portal-side managed credentials; custom mode asks portal to
// open the user connection with the caller-provided clientId/clientSecret.
type PortalTicketConfig struct {
	TicketURL    string
	AccessToken  string
	SourceID     string
	Mode         string
	ClientID     string
	ClientSecret string
	UserAgent    string
	HTTPClient   *http.Client
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
	if strings.TrimSpace(c.AccessToken) == "" {
		return errors.New("source: portal access token is required")
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

	ticket, err := requestPortalTicket(ctx, s.cfg.PortalTicket)
	if err != nil {
		s.machine.OnStopped()
		return err
	}
	wsURL, err := websocketURL(ticket)
	if err != nil {
		s.machine.OnStopped()
		return err
	}

	userAgent := strings.TrimSpace(s.cfg.PortalTicket.UserAgent)
	if userAgent == "" {
		userAgent = "dws-event-consume"
	}
	conn, resp, err := (&websocket.Dialer{HandshakeTimeout: 20 * time.Second}).DialContext(ctx, wsURL, http.Header{
		"User-Agent": []string{userAgent},
	})
	if err != nil {
		s.machine.OnStopped()
		if resp != nil {
			defer resp.Body.Close()
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			return fmt.Errorf("source: portal stream connect HTTP %d: %s: %w",
				resp.StatusCode, truncatePortalTicketLog(string(raw), 300), err)
		}
		return fmt.Errorf("source: portal stream connect: %w", err)
	}
	defer conn.Close()
	s.machine.OnConnected()

	closeOnContext(ctx, conn)
	handler := s.makeHandler(emit)
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			s.machine.OnStopped()
			if isContextDone(ctx) {
				return ctx.Err()
			}
			return fmt.Errorf("source: portal stream read: %w", err)
		}
		df, err := payload.DecodeDataFrame(message)
		if err != nil {
			continue
		}
		resp, _ := handler(ctx, df)
		ensurePortalAckHeaders(resp, df)
		if err := portalWriteMessage(conn, websocket.TextMessage, resp.Encode()); err != nil {
			s.machine.OnStopped()
			if isContextDone(ctx) {
				return ctx.Err()
			}
			return fmt.Errorf("source: portal stream ack: %w", err)
		}
	}
}

type portalStreamTicket struct {
	Endpoint string `json:"endpoint"`
	Ticket   string `json:"ticket"`
}

func requestPortalTicket(ctx context.Context, cfg *PortalTicketConfig) (portalStreamTicket, error) {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
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
		return portalStreamTicket{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if ua := strings.TrimSpace(cfg.UserAgent); ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	req.Header.Set("x-user-access-token", cfg.AccessToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return portalStreamTicket{}, fmt.Errorf("source: portal ticket request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return portalStreamTicket{}, fmt.Errorf("source: portal ticket HTTP %d: %s",
			resp.StatusCode, truncatePortalTicketLog(string(raw), 300))
	}

	var direct portalStreamTicket
	if err := json.Unmarshal(raw, &direct); err == nil && direct.Endpoint != "" && direct.Ticket != "" {
		return direct, nil
	}

	var envelope struct {
		Success   bool               `json:"success"`
		Result    portalStreamTicket `json:"result"`
		ErrorCode string             `json:"errorCode"`
		ErrorMsg  string             `json:"errorMsg"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return portalStreamTicket{}, fmt.Errorf("source: portal ticket parse: %w", err)
	}
	if !envelope.Success {
		return portalStreamTicket{}, fmt.Errorf("source: portal ticket failed: %s %s",
			envelope.ErrorCode, envelope.ErrorMsg)
	}
	if envelope.Result.Endpoint == "" || envelope.Result.Ticket == "" {
		return portalStreamTicket{}, errors.New("source: portal ticket result missing endpoint/ticket")
	}
	return envelope.Result, nil
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
