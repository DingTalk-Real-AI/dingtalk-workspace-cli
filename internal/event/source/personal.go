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

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/gorilla/websocket"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/event"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/payload"
)

const personalRawDebugPayloadLimit = 8192

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
	raw := s.rawEventFromDataFrame(df)
	logPersonalDataFrame(raw, df.Data)
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

func (s *PersonalSource) rawEventFromDataFrame(df *payload.DataFrame) *dwsevent.RawEvent {
	hdr := event.NewEventHeaderFromDataFrame(df)
	dataFields := parsePersonalDataFields(df.Data)
	now := time.Now
	if s != nil && s.cfg.Now != nil {
		now = s.cfg.Now
	}
	sourceID := ""
	if s != nil {
		sourceID = s.cfg.SourceID
	}
	raw := &dwsevent.RawEvent{
		EventID:           firstNonEmpty(hdr.EventId, df.GetMessageId(), headerAny(df.Headers, "MESSAGE_ID", "messageId", "event_id", "eventId"), dataFields.string("eventId", "event_id")),
		EventBornTime:     firstInt64(hdr.EventBornTime, df.GetTimestamp(), df.Time),
		EventCorpID:       hdr.EventCorpId,
		EventType:         firstNonEmpty(hdr.EventType, eventTypeFromHeaders(df.Headers), dataFields.string("eventKey", "event_key"), dataFields.nestedString([]string{"source", "tag"})),
		EventUnifiedAppID: hdr.EventUnifiedAppId,
		EventScope:        firstNonEmpty(headerAny(df.Headers, "event_scope", "eventScope"), "personal"),
		SubscribeID:       firstNonEmpty(headerAny(df.Headers, "SUB_ID", "subscribe_id", "subscribeId", "sub_id", "subId"), dataFields.string("subId", "sub_id", "subscribeId", "subscribe_id")),
		SourceID:          firstNonEmpty(headerAny(df.Headers, "SOURCE_ID", "source_id", "sourceId"), sourceID),
		RuleType:          firstNonEmpty(headerAny(df.Headers, "RULE_TYPE", "rule_type", "ruleType"), dataFields.string("ruleType", "rule_type"), dataFields.nestedString([]string{"ext", "ruleType"}, []string{"ext", "rule_type"})),
		Data:              df.Data,
		Headers:           copyHeaders(df.Headers),
		ReceivedAt:        now().UTC(),
	}
	return raw
}

func logPersonalDataFrame(raw *dwsevent.RawEvent, data string) {
	if raw == nil {
		return
	}
	slog.Debug("personal source received dataframe",
		"event_type", raw.EventType,
		"event_key", raw.EventType,
		"event_id", raw.EventID,
		"subscribe_id", raw.SubscribeID,
		"source_id", raw.SourceID,
		"rule_type", raw.RuleType,
		"headers", redactPersonalRawStringMap(raw.Headers),
		"data", sanitizePersonalRawPayload([]byte(data)),
	)
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
		if v := strings.TrimSpace(headerValue(h, name)); v != "" {
			return v
		}
	}
	return ""
}

func headerValue(h payload.DataFrameHeader, name string) string {
	if len(h) == 0 {
		return ""
	}
	if v := h.Get(name); strings.TrimSpace(v) != "" {
		return v
	}
	for k, v := range h {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return ""
}

func eventTypeFromHeaders(h payload.DataFrameHeader) string {
	if v := headerAny(h, "EVENT_TYPE", "event_type", "eventType", "EVENT_KEY", "event_key", "eventKey"); v != "" {
		return v
	}
	if v := headerAny(h, "TOPIC", "topic"); v != "" && v != "*" {
		return v
	}
	return ""
}

type personalDataFields map[string]any

func parsePersonalDataFields(raw string) personalDataFields {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil
	}
	if s, ok := value.(string); ok {
		var nested any
		if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &nested); err == nil {
			value = nested
		}
	}
	m, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	return personalDataFields(m)
}

func (f personalDataFields) string(names ...string) string {
	if len(f) == 0 {
		return ""
	}
	for _, name := range names {
		if v, ok := f[name].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		for k, raw := range f {
			if strings.EqualFold(k, name) {
				if v, ok := raw.(string); ok && strings.TrimSpace(v) != "" {
					return strings.TrimSpace(v)
				}
			}
		}
	}
	return ""
}

func (f personalDataFields) nestedString(paths ...[]string) string {
	if len(f) == 0 {
		return ""
	}
	for _, path := range paths {
		if v := nestedStringCI(map[string]any(f), path...); v != "" {
			return v
		}
	}
	return ""
}

func nestedStringCI(m map[string]any, path ...string) string {
	var cur any = m
	for _, segment := range path {
		obj, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		next, ok := obj[segment]
		if !ok {
			for k, v := range obj {
				if strings.EqualFold(k, segment) {
					next = v
					ok = true
					break
				}
			}
		}
		if !ok {
			return ""
		}
		cur = next
	}
	if v, ok := cur.(string); ok {
		return strings.TrimSpace(v)
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

func redactPersonalRawStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		if personalRawSensitiveKey(k) {
			out[k] = "<redacted>"
			continue
		}
		out[k] = v
	}
	return out
}

func sanitizePersonalRawPayload(data []byte) string {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return ""
	}
	var parsed any
	if err := json.Unmarshal(data, &parsed); err == nil {
		redacted := redactPersonalRawJSONValue(parsed)
		if s, err := marshalPersonalRawJSON(redacted); err == nil {
			return truncatePersonalRawLog(s)
		}
	}
	return truncatePersonalRawLog(string(data))
}

func redactPersonalRawJSONValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, value := range x {
			if personalRawSensitiveKey(k) {
				out[k] = "<redacted>"
				continue
			}
			out[k] = redactPersonalRawJSONValue(value)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, value := range x {
			out[i] = redactPersonalRawJSONValue(value)
		}
		return out
	default:
		return v
	}
}

func marshalPersonalRawJSON(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}

func personalRawSensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(key, "token") ||
		strings.Contains(key, "secret") ||
		strings.Contains(key, "ticket") ||
		strings.Contains(key, "authorization")
}

func truncatePersonalRawLog(s string) string {
	if len(s) <= personalRawDebugPayloadLimit {
		return s
	}
	return s[:personalRawDebugPayloadLimit] + "...<truncated>"
}
