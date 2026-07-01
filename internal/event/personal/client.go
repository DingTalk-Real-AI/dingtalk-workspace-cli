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

package personal

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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

const DefaultBasePath = "/v1/personal-events"

type Identity struct {
	AccessToken string `json:"-"`
	CorpID      string `json:"corp_id"`
	UserID      string `json:"user_id"`
	ClientID    string `json:"client_id"`
	SourceID    string `json:"source_id"`
}

func (i Identity) Key() string {
	return strings.Join([]string{i.CorpID, i.UserID, i.ClientID, i.SourceID}, "\x00")
}

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	Identity   Identity
}

type CreateSubscriptionRequest struct {
	EventKey       string         `json:"event_key"`
	RuleType       string         `json:"rule_type"`
	Name           string         `json:"name,omitempty"`
	RuleParam      map[string]any `json:"rule_param"`
	Filter         any            `json:"filter,omitempty"`
	Delivery       map[string]any `json:"delivery"`
	TTLSeconds     int64          `json:"ttl_seconds,omitempty"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
}

type Subscription struct {
	SubscribeID string     `json:"subscribe_id"`
	EventKey    string     `json:"event_key,omitempty"`
	RuleType    string     `json:"rule_type,omitempty"`
	Status      string     `json:"status,omitempty"`
	SourceID    string     `json:"source_id,omitempty"`
	CreatedAt   string     `json:"created_at,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

type ListOptions struct {
	Status      string
	EventKey    string
	SubscribeID string
}

type APIError struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Retryable bool           `json:"retryable,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code != "" && e.Message != "" {
		return e.Code + ": " + e.Message
	}
	if e.Code != "" {
		return e.Code
	}
	return e.Message
}

func NewClient(baseURL string, identity Identity) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(config.GetMCPBaseURL(), "/") + DefaultBasePath
	}
	return &Client{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Identity:   identity,
	}
}

func (c *Client) CreateSubscription(ctx context.Context, req CreateSubscriptionRequest) (*Subscription, error) {
	if req.EventKey == "" || req.RuleType == "" {
		return nil, errors.New("personal event: event_key and rule_type are required")
	}
	if req.Delivery == nil {
		req.Delivery = map[string]any{"mode": "stream"}
	}
	var sub Subscription
	if err := c.do(ctx, http.MethodPost, "/subscriptions", nil, req, &sub); err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) {
			if subID, ok := apiErr.Details["subscribe_id"].(string); ok && subID != "" {
				return &Subscription{
					SubscribeID: subID,
					EventKey:    req.EventKey,
					RuleType:    req.RuleType,
					Status:      "active",
					SourceID:    c.Identity.SourceID,
				}, nil
			}
		}
		return nil, err
	}
	return &sub, nil
}

func (c *Client) GetSubscription(ctx context.Context, subscribeID string) (*Subscription, error) {
	subscribeID = strings.TrimSpace(subscribeID)
	if subscribeID == "" {
		return nil, errors.New("personal event: subscribe_id is required")
	}
	var sub Subscription
	if err := c.do(ctx, http.MethodGet, "/subscriptions/"+url.PathEscape(subscribeID), nil, nil, &sub); err != nil {
		return nil, err
	}
	return &sub, nil
}

func (c *Client) ListSubscriptions(ctx context.Context, opts ListOptions) ([]Subscription, error) {
	q := make(url.Values)
	if opts.Status != "" {
		q.Set("status", opts.Status)
	}
	if opts.EventKey != "" {
		q.Set("event_key", opts.EventKey)
	}
	if opts.SubscribeID != "" {
		q.Set("subscribe_id", opts.SubscribeID)
	}
	var result struct {
		Items []Subscription `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, "/subscriptions", q, nil, &result); err != nil {
		return nil, err
	}
	return result.Items, nil
}

func (c *Client) DeleteSubscription(ctx context.Context, subscribeID string) error {
	subscribeID = strings.TrimSpace(subscribeID)
	if subscribeID == "" {
		return errors.New("personal event: subscribe_id is required")
	}
	err := c.do(ctx, http.MethodDelete, "/subscriptions/"+url.PathEscape(subscribeID), nil, nil, nil)
	if isNotFound(err) {
		return nil
	}
	return err
}

func (c *Client) do(ctx context.Context, method, path string, q url.Values, body any, out any) error {
	if c == nil {
		return errors.New("personal event: nil client")
	}
	if c.Identity.AccessToken == "" {
		return errors.New("personal event: access token is required")
	}
	u := strings.TrimRight(c.BaseURL, "/") + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("personal event: encode request: %w", err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, r)
	if err != nil {
		return fmt.Errorf("personal event: create request: %w", err)
	}
	c.decorate(req)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := c.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("personal event: send request: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, config.MaxResponseBodySize))
	if err != nil {
		return fmt.Errorf("personal event: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if apiErr := decodeAPIError(data); apiErr != nil {
			return apiErr
		}
		return fmt.Errorf("personal event: HTTP %d", resp.StatusCode)
	}
	if len(bytes.TrimSpace(data)) == 0 || out == nil {
		return nil
	}
	var env responseEnvelope
	if err := json.Unmarshal(data, &env); err == nil && (env.Success || env.Error != nil || env.Result != nil) {
		if !env.Success {
			if env.Error != nil {
				return env.Error
			}
			return errors.New("personal event: request failed")
		}
		if env.Result == nil {
			return nil
		}
		return decodeResult(env.Result, out)
	}
	return json.Unmarshal(data, out)
}

func (c *Client) decorate(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.Identity.AccessToken)
	req.Header.Set("x-user-access-token", c.Identity.AccessToken)
	req.Header.Set("X-DWS-Client-Id", c.Identity.ClientID)
	req.Header.Set("X-DWS-Source-Id", c.Identity.SourceID)
	if c.Identity.CorpID != "" {
		req.Header.Set("X-DWS-Corp-Id", c.Identity.CorpID)
	}
	req.Header.Set("Accept", "application/json")
}

type responseEnvelope struct {
	Success   bool            `json:"success"`
	RequestID string          `json:"request_id,omitempty"`
	Result    json.RawMessage `json:"result"`
	Error     *APIError       `json:"error"`
}

func decodeResult(raw json.RawMessage, out any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, out); err == nil {
		return nil
	}
	var ids []string
	if err := json.Unmarshal(raw, &ids); err == nil && len(ids) > 0 {
		if sub, ok := out.(*Subscription); ok {
			sub.SubscribeID = ids[0]
			return nil
		}
	}
	return json.Unmarshal(raw, out)
}

func decodeAPIError(data []byte) *APIError {
	var env responseEnvelope
	if err := json.Unmarshal(data, &env); err == nil && env.Error != nil {
		return env.Error
	}
	var apiErr APIError
	if err := json.Unmarshal(data, &apiErr); err == nil && (apiErr.Code != "" || apiErr.Message != "") {
		return &apiErr
	}
	return nil
}

func isNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && (apiErr.Code == "PERSONAL_EVENT_NOT_FOUND" || apiErr.Code == "NOT_FOUND")
}
