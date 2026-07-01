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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/gorilla/websocket"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/event"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/payload"
)

type PersonalConfig struct {
	AccessToken     string
	ClientID        string
	ClientSecret    string
	SourceID        string
	TicketURL       string
	TicketMode      string
	HTTPClient      *http.Client
	WebSocketDialer *websocket.Dialer
	Now             func() time.Time
}

type PersonalSource struct {
	cfg     PersonalConfig
	machine *Machine
	conn    *websocket.Conn
}

type ticketResponse struct {
	Endpoint string `json:"endpoint"`
	Ticket   string `json:"ticket"`
}

func NewPersonal(cfg PersonalConfig) (*PersonalSource, error) {
	if strings.TrimSpace(cfg.AccessToken) == "" {
		return nil, errors.New("personal source: AccessToken is required")
	}
	if strings.TrimSpace(cfg.ClientID) == "" {
		return nil, errors.New("personal source: ClientID is required")
	}
	if strings.TrimSpace(cfg.SourceID) == "" {
		return nil, errors.New("personal source: SourceID is required")
	}
	if strings.TrimSpace(cfg.TicketURL) == "" {
		return nil, errors.New("personal source: TicketURL is required")
	}
	if cfg.TicketMode == "" {
		cfg.TicketMode = "normal"
	}
	if cfg.TicketMode == "custom" && strings.TrimSpace(cfg.ClientSecret) == "" {
		return nil, errors.New("personal source: ClientSecret is required when stream ticket mode is custom")
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if cfg.WebSocketDialer == nil {
		cfg.WebSocketDialer = websocket.DefaultDialer
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	m := NewMachine()
	m.now = cfg.Now
	return &PersonalSource{cfg: cfg, machine: m}, nil
}

func (s *PersonalSource) State() Snapshot { return s.machine.Snapshot() }

func (s *PersonalSource) Start(ctx context.Context, emit dwsevent.EmitFn) error {
	if emit == nil {
		return errors.New("personal source: emit is required")
	}
	if s.conn != nil {
		return errors.New("personal source: Start called twice")
	}
	s.machine.OnConnecting()
	ticket, err := s.fetchTicket(ctx)
	if err != nil {
		s.machine.OnStopped()
		return err
	}
	wsURL, err := endpointWithTicket(ticket.Endpoint, ticket.Ticket)
	if err != nil {
		s.machine.OnStopped()
		return err
	}
	conn, _, err := s.cfg.WebSocketDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		s.machine.OnStopped()
		return fmt.Errorf("personal source: dial websocket: %w", err)
	}
	s.conn = conn
	defer func() {
		_ = conn.Close()
		s.machine.OnStopped()
	}()
	s.machine.OnConnected()

	closePersonalWebSocketOnContext(ctx, conn)
	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("personal source: read websocket: %w", err)
		}
		if messageType != websocket.TextMessage {
			continue
		}
		if err := s.handleFrame(conn, data, emit); err != nil {
			return err
		}
	}
}

func (s *PersonalSource) fetchTicket(ctx context.Context) (*ticketResponse, error) {
	body := map[string]any{
		"sourceId": s.cfg.SourceID,
		"mode":     s.cfg.TicketMode,
	}
	if s.cfg.TicketMode == "custom" {
		body["clientId"] = s.cfg.ClientID
		body["clientSecret"] = s.cfg.ClientSecret
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.TicketURL, bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("personal source: create ticket request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-user-access-token", s.cfg.AccessToken)
	req.Header.Set("Authorization", "Bearer "+s.cfg.AccessToken)
	req.Header.Set("X-DWS-Client-Id", s.cfg.ClientID)
	req.Header.Set("X-DWS-Source-Id", s.cfg.SourceID)

	resp, err := s.cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("personal source: fetch ticket: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, config.MaxResponseBodySize))
	if err != nil {
		return nil, fmt.Errorf("personal source: read ticket response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("personal source: ticket HTTP %d", resp.StatusCode)
	}
	ticket, err := decodeTicket(data)
	if err != nil {
		return nil, err
	}
	if ticket.Endpoint == "" || ticket.Ticket == "" {
		return nil, errors.New("personal source: ticket response missing endpoint or ticket")
	}
	return ticket, nil
}

func (s *PersonalSource) handleFrame(conn *websocket.Conn, data []byte, emit dwsevent.EmitFn) error {
	df, err := payload.DecodeDataFrame(data)
	if err != nil {
		return fmt.Errorf("personal source: decode dataframe: %w", err)
	}
	hdr := event.NewEventHeaderFromDataFrame(df)
	raw := &dwsevent.RawEvent{
		EventID:           firstNonEmpty(hdr.EventId, df.GetMessageId(), headerAny(df.Headers, "event_id", "eventId")),
		EventBornTime:     firstInt64(hdr.EventBornTime, df.GetTimestamp(), df.Time),
		EventCorpID:       hdr.EventCorpId,
		EventType:         firstNonEmpty(hdr.EventType, headerAny(df.Headers, "event_key", "eventKey", "event_type", "eventType")),
		EventUnifiedAppID: hdr.EventUnifiedAppId,
		EventScope:        firstNonEmpty(headerAny(df.Headers, "event_scope", "eventScope"), "personal"),
		SubscribeID:       firstNonEmpty(headerAny(df.Headers, "subscribe_id", "subscribeId", "sub_id", "subId"), dataString(df.Data, "subscribe_id", "subscribeId", "sub_id", "subId")),
		SourceID:          firstNonEmpty(headerAny(df.Headers, "source_id", "sourceId"), s.cfg.SourceID),
		RuleType:          firstNonEmpty(headerAny(df.Headers, "rule_type", "ruleType"), dataString(df.Data, "rule_type", "ruleType")),
		Data:              df.Data,
		Headers:           copyHeaders(df.Headers),
		ReceivedAt:        s.cfg.Now().UTC(),
	}
	emit(raw)
	s.machine.OnEvent()
	resp := payload.NewSuccessDataFrameResponse()
	resp.SetHeader(payload.DataFrameHeaderKMessageId, df.GetMessageId())
	resp.SetHeader(payload.DataFrameHeaderKContentType, payload.DataFrameContentTypeKJson)
	if err := resp.SetJson(event.NewEventProcessResultSuccess()); err != nil {
		return err
	}
	if err := conn.WriteJSON(resp); err != nil {
		return fmt.Errorf("personal source: write ack: %w", err)
	}
	return nil
}

func decodeTicket(data []byte) (*ticketResponse, error) {
	var env struct {
		Success bool            `json:"success"`
		Result  json.RawMessage `json:"result"`
		Data    json.RawMessage `json:"data"`
		Error   any             `json:"error"`
	}
	if err := json.Unmarshal(data, &env); err == nil && (env.Result != nil || env.Data != nil || env.Error != nil) {
		raw := env.Result
		if len(raw) == 0 || string(raw) == "null" {
			raw = env.Data
		}
		var tr ticketResponse
		if err := json.Unmarshal(raw, &tr); err != nil {
			return nil, fmt.Errorf("personal source: parse ticket result: %w", err)
		}
		return &tr, nil
	}
	var tr ticketResponse
	if err := json.Unmarshal(data, &tr); err != nil {
		return nil, fmt.Errorf("personal source: parse ticket response: %w", err)
	}
	return &tr, nil
}

func endpointWithTicket(endpoint, ticket string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return "", fmt.Errorf("personal source: parse endpoint: %w", err)
	}
	q := u.Query()
	if q.Get("ticket") == "" {
		q.Set("ticket", ticket)
		u.RawQuery = q.Encode()
	}
	return u.String(), nil
}

func closePersonalWebSocketOnContext(ctx context.Context, conn *websocket.Conn) {
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()
}

func headerAny(h payload.DataFrameHeader, names ...string) string {
	for _, name := range names {
		if v := strings.TrimSpace(h.Get(name)); v != "" {
			return v
		}
	}
	return ""
}

func dataString(raw string, names ...string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return ""
	}
	for _, name := range names {
		if v, ok := m[name].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func firstInt64(values ...int64) int64 {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}
