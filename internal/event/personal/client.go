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
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

const DefaultBasePath = "/dws"

const (
	controlLogPayloadLimit       = 8192
	subscriptionListPageSize     = 100
	subscriptionListMaxPageGuard = 10000
)

var subscriptionListMaxPages = subscriptionListMaxPageGuard

type Identity struct {
	AccessToken  string `json:"-"`
	LocalSubject string `json:"-"`
	CorpID       string `json:"corp_id"`
	UserID       string `json:"user_id"`
	ClientID     string `json:"client_id"`
	SourceID     string `json:"source_id"`
}

func (i Identity) Key() string {
	corpID := strings.TrimSpace(i.CorpID)
	userID := strings.TrimSpace(i.UserID)
	clientID := strings.TrimSpace(i.ClientID)
	sourceID := strings.TrimSpace(i.SourceID)
	if corpID != "" && userID != "" {
		return strings.Join([]string{"corp_user", corpID, userID, clientID, sourceID}, "\x00")
	}
	if localSubject := strings.TrimSpace(i.LocalSubject); localSubject != "" {
		return strings.Join([]string{"local_subject", localSubject, clientID, sourceID}, "\x00")
	}
	return strings.Join([]string{"unknown", corpID, userID, clientID, sourceID}, "\x00")
}

type Client struct {
	BaseURL             string
	HTTPClient          *http.Client
	Identity            Identity
	AccessTokenProvider func(context.Context) (string, error)
	ClientVersion       string
	UserAgent           string
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

type dwsCreateSubscriptionRequest struct {
	ClientID     string         `json:"clientId"`
	SourceID     string         `json:"sourceId,omitempty"`
	EventKey     string         `json:"eventKey"`
	FilterRule   string         `json:"filterRule,omitempty"`
	DeliveryPref string         `json:"deliveryPref,omitempty"`
	ExpiresAt    string         `json:"expiresAt,omitempty"`
	Ext          map[string]any `json:"ext,omitempty"`
}

type dwsSubListResult struct {
	Total    int               `json:"total,omitempty"`
	PageNo   int               `json:"pageNo,omitempty"`
	PageSize int               `json:"pageSize,omitempty"`
	Items    []dwsSubscription `json:"items"`
}

type dwsSubscription struct {
	SubID         string          `json:"subId"`
	SubscribeID   string          `json:"subscribe_id"`
	EventKey      string          `json:"eventKey"`
	EventKeySnake string          `json:"event_key"`
	RuleType      string          `json:"ruleType,omitempty"`
	RuleTypeSnake string          `json:"rule_type,omitempty"`
	ClientID      string          `json:"clientId,omitempty"`
	SourceID      string          `json:"sourceId"`
	SourceIDSnake string          `json:"source_id"`
	DeliveryPref  string          `json:"deliveryPref,omitempty"`
	Status        json.RawMessage `json:"status,omitempty"`
	GmtCreate     string          `json:"gmtCreate,omitempty"`
	CreatedAt     string          `json:"created_at,omitempty"`
}

func (s dwsSubscription) toSubscription() Subscription {
	return Subscription{
		SubscribeID: firstNonEmpty(s.SubID, s.SubscribeID),
		EventKey:    firstNonEmpty(s.EventKey, s.EventKeySnake),
		RuleType:    firstNonEmpty(s.RuleType, s.RuleTypeSnake),
		Status:      dwsStatusString(s.Status),
		SourceID:    firstNonEmpty(s.SourceID, s.SourceIDSnake),
		CreatedAt:   firstNonEmpty(s.GmtCreate, s.CreatedAt),
	}
}

type APIError struct {
	Code              string         `json:"code"`
	Message           string         `json:"message"`
	Retryable         *bool          `json:"retryable,omitempty"`
	RetryAfterSeconds *int64         `json:"retry_after_seconds,omitempty"`
	NextRetryAt       *time.Time     `json:"next_retry_at,omitempty"`
	TraceID           string         `json:"trace_id,omitempty"`
	HTTPStatus        int            `json:"http_status,omitempty"`
	Details           map[string]any `json:"details,omitempty"`
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
	var sub Subscription
	if err := c.do(ctx, http.MethodPost, "/subscription/user", nil, c.buildCreateRequest(req), &sub); err != nil {
		return nil, err
	}
	if sub.EventKey == "" {
		sub.EventKey = req.EventKey
	}
	if sub.RuleType == "" {
		sub.RuleType = req.RuleType
	}
	if sub.Status == "" {
		sub.Status = "active"
	}
	if sub.SourceID == "" {
		sub.SourceID = c.Identity.SourceID
	}
	return &sub, nil
}

func (c *Client) GetSubscription(ctx context.Context, subscribeID string) (*Subscription, error) {
	subscribeID = strings.TrimSpace(subscribeID)
	if subscribeID == "" {
		return nil, errors.New("personal event: subscribe_id is required")
	}
	subs, err := c.ListSubscriptions(ctx, ListOptions{SubscribeID: subscribeID})
	if err != nil {
		return nil, err
	}
	if len(subs) == 0 {
		return nil, &APIError{Code: "PERSONAL_EVENT_NOT_FOUND", Message: "subscription not found"}
	}
	return &subs[0], nil
}

func (c *Client) ListSubscriptions(ctx context.Context, opts ListOptions) ([]Subscription, error) {
	q := make(url.Values)
	if clientID := strings.TrimSpace(c.Identity.ClientID); clientID != "" {
		q.Set("clientId", clientID)
	}
	if sourceID := strings.TrimSpace(c.Identity.SourceID); sourceID != "" {
		q.Set("sourceId", sourceID)
	}
	q.Set("pageSize", fmt.Sprintf("%d", subscriptionListPageSize))
	all := make([]Subscription, 0, subscriptionListPageSize)
	seen := make(map[string]struct{}, subscriptionListPageSize)
	for pageNo := 1; pageNo <= subscriptionListMaxPages; pageNo++ {
		q.Set("pageNo", fmt.Sprintf("%d", pageNo))
		var result dwsSubListResult
		if err := c.do(ctx, http.MethodGet, "/event/sublist", q, nil, &result); err != nil {
			return nil, err
		}
		if len(result.Items) == 0 {
			break
		}
		effectivePageSize := subscriptionListPageSize
		if result.PageSize > 0 {
			effectivePageSize = result.PageSize
		}

		added := 0
		for _, item := range result.Items {
			sub := item.toSubscription()
			if sub.SubscribeID != "" {
				if _, ok := seen[sub.SubscribeID]; ok {
					continue
				}
				seen[sub.SubscribeID] = struct{}{}
			}
			all = append(all, sub)
			added++
		}
		if added == 0 && (result.Total > len(all) || len(result.Items) >= effectivePageSize) {
			return nil, fmt.Errorf("personal event: subscription pagination made no progress at page %d", pageNo)
		}
		if result.Total > 0 && len(all) >= result.Total {
			break
		}
		if len(result.Items) < effectivePageSize {
			break
		}
		if pageNo == subscriptionListMaxPages {
			return nil, fmt.Errorf("personal event: subscription pagination exceeded %d pages", subscriptionListMaxPages)
		}
	}

	items := make([]Subscription, 0, len(all))
	for _, sub := range all {
		if opts.Status != "" && opts.Status != "all" && sub.Status != opts.Status {
			continue
		}
		if opts.EventKey != "" && sub.EventKey != opts.EventKey {
			continue
		}
		if opts.SubscribeID != "" && sub.SubscribeID != opts.SubscribeID {
			continue
		}
		items = append(items, sub)
	}
	return items, nil
}

func (c *Client) DeleteSubscription(ctx context.Context, subscribeID string) error {
	subscribeID = strings.TrimSpace(subscribeID)
	if subscribeID == "" {
		return errors.New("personal event: subscribe_id is required")
	}
	err := c.do(ctx, http.MethodPost, "/subscription/cancel", nil, map[string]string{"subId": subscribeID}, nil)
	if isNotFound(err) {
		return nil
	}
	return err
}

func (c *Client) buildCreateRequest(req CreateSubscriptionRequest) dwsCreateSubscriptionRequest {
	filterRule := ""
	if req.RuleParam != nil {
		if b, err := json.Marshal(req.RuleParam); err == nil {
			filterRule = string(b)
		}
	}
	ext := map[string]any{
		"ruleType": req.RuleType,
	}
	if req.Name != "" {
		ext["name"] = req.Name
	}
	if req.Filter != nil {
		ext["filter"] = req.Filter
	}
	if req.IdempotencyKey != "" {
		ext["idempotencyKey"] = req.IdempotencyKey
	}
	out := dwsCreateSubscriptionRequest{
		ClientID:     c.Identity.ClientID,
		SourceID:     c.Identity.SourceID,
		EventKey:     req.EventKey,
		FilterRule:   filterRule,
		DeliveryPref: "realtime",
		Ext:          ext,
	}
	if req.TTLSeconds > 0 {
		out.ExpiresAt = time.Now().UTC().Add(time.Duration(req.TTLSeconds) * time.Second).Format(time.RFC3339)
	}
	return out
}

func (c *Client) do(ctx context.Context, method, path string, q url.Values, body any, out any) error {
	if c == nil {
		return errors.New("personal event: nil client")
	}
	accessToken, err := c.resolveAccessToken(ctx)
	if err != nil {
		return err
	}
	u := strings.TrimRight(c.BaseURL, "/") + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var r io.Reader
	requestLog := ""
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("personal event: encode request: %w", err)
		}
		requestLog = sanitizeLogPayload(b)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, r)
	if err != nil {
		return fmt.Errorf("personal event: create request: %w", err)
	}
	c.decorate(req, accessToken)
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
	responseLog := sanitizeLogPayload(data)
	responseID := firstNonEmpty(responseRequestID(data), responseHeaderRequestID(resp.Header))
	responseTrace := firstNonEmpty(responseBodyTraceID(data), responseTraceID(resp.Header))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if apiErr := decodeAPIError(data); apiErr != nil {
			apiErr = withHTTPResponseDetails(apiErr, method, path, resp, responseID, responseTrace)
			logControlRequest("personal event control request failed", method, path, q, resp.StatusCode, requestLog, responseLog, responseID, apiErr)
			return apiErr
		}
		apiErr := withHTTPResponseDetails(&APIError{
			Code:    fmt.Sprintf("HTTP_%d", resp.StatusCode),
			Message: firstNonEmpty(http.StatusText(resp.StatusCode), "HTTP request failed"),
		}, method, path, resp, responseID, responseTrace)
		logControlRequest("personal event control request failed", method, path, q, resp.StatusCode, requestLog, responseLog, responseID, apiErr)
		return apiErr
	}
	if len(bytes.TrimSpace(data)) == 0 {
		logControlRequest("personal event control request", method, path, q, resp.StatusCode, requestLog, responseLog, "", nil)
		return nil
	}
	var env responseEnvelope
	if err := json.Unmarshal(data, &env); err == nil && (env.Success != nil || env.Error != nil || env.Result != nil || env.ErrorCode != "" || env.ErrorMsg != "") {
		envID := firstNonEmpty(env.requestID(), responseID)
		envTrace := firstNonEmpty(env.traceID(), responseTrace)
		if env.Success == nil {
			if apiErr := env.apiError(); apiErr != nil {
				apiErr = withHTTPResponseDetails(apiErr, method, path, resp, envID, envTrace)
				logControlRequest("personal event control request failed", method, path, q, resp.StatusCode, requestLog, responseLog, envID, apiErr)
				return apiErr
			}
		}
		if env.Success != nil && !*env.Success {
			if apiErr := env.apiError(); apiErr != nil {
				apiErr = withHTTPResponseDetails(apiErr, method, path, resp, envID, envTrace)
				logControlRequest("personal event control request failed", method, path, q, resp.StatusCode, requestLog, responseLog, envID, apiErr)
				return apiErr
			}
			logControlRequest("personal event control request failed", method, path, q, resp.StatusCode, requestLog, responseLog, envID, nil)
			return errors.New("personal event: request failed")
		}
		logControlRequest("personal event control request", method, path, q, resp.StatusCode, requestLog, responseLog, envID, nil)
		if env.Result == nil {
			return nil
		}
		if out == nil {
			return nil
		}
		return decodeResult(env.Result, out)
	}
	if out == nil {
		logControlRequest("personal event control request", method, path, q, resp.StatusCode, requestLog, responseLog, responseID, nil)
		return nil
	}
	logControlRequest("personal event control request", method, path, q, resp.StatusCode, requestLog, responseLog, responseID, nil)
	return json.Unmarshal(data, out)
}

func (c *Client) resolveAccessToken(ctx context.Context) (string, error) {
	if c.AccessTokenProvider != nil {
		token, err := c.AccessTokenProvider(ctx)
		if err != nil {
			return "", fmt.Errorf("personal event: resolve access token: %w", err)
		}
		if token = strings.TrimSpace(token); token != "" {
			return token, nil
		}
		return "", errors.New("personal event: access token provider returned empty token")
	}
	if token := strings.TrimSpace(c.Identity.AccessToken); token != "" {
		return token, nil
	}
	return "", errors.New("personal event: access token is required")
}

func (c *Client) decorate(req *http.Request, accessToken string) {
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("x-user-access-token", accessToken)
	req.Header.Set("X-DWS-Client-Id", c.Identity.ClientID)
	req.Header.Set("X-DWS-Source-Id", c.Identity.SourceID)
	if c.Identity.CorpID != "" {
		req.Header.Set("X-DWS-Corp-Id", c.Identity.CorpID)
	}
	if version := strings.TrimSpace(c.ClientVersion); version != "" {
		req.Header.Set("X-Cli-Version", version)
	}
	if userAgent := strings.TrimSpace(c.UserAgent); userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	req.Header.Set("Accept", "application/json")
}

type responseEnvelope struct {
	Success                *bool             `json:"success"`
	RequestID              string            `json:"request_id,omitempty"`
	RequestID2             string            `json:"requestId,omitempty"`
	TraceID                string            `json:"trace_id,omitempty"`
	TraceID2               string            `json:"traceId,omitempty"`
	Arguments              []json.RawMessage `json:"arguments,omitempty"`
	Result                 json.RawMessage   `json:"result"`
	Error                  *APIError         `json:"error"`
	ErrorCode              string            `json:"errorCode,omitempty"`
	ErrorMsg               string            `json:"errorMsg,omitempty"`
	Retryable              *bool             `json:"retryable,omitempty"`
	RetryAfterSeconds      *int64            `json:"retry_after_seconds,omitempty"`
	RetryAfterSecondsCamel *int64            `json:"retryAfterSeconds,omitempty"`
	NextRetryAt            *time.Time        `json:"next_retry_at,omitempty"`
	NextRetryAtCamel       *time.Time        `json:"nextRetryAt,omitempty"`
}

func (e responseEnvelope) apiError() *APIError {
	if e.Error != nil {
		if e.Error.Retryable == nil {
			e.Error.Retryable = e.Retryable
		}
		if e.Error.RetryAfterSeconds == nil {
			e.Error.RetryAfterSeconds = firstInt64Pointer(e.RetryAfterSeconds, e.RetryAfterSecondsCamel)
		}
		if e.Error.NextRetryAt == nil {
			e.Error.NextRetryAt = firstTimePointer(e.NextRetryAt, e.NextRetryAtCamel)
		}
		if e.Error.TraceID == "" {
			e.Error.TraceID = e.traceID()
		}
		return e.Error
	}
	if e.ErrorCode != "" || e.ErrorMsg != "" {
		return &APIError{
			Code:              e.ErrorCode,
			Message:           e.ErrorMsg,
			Retryable:         e.Retryable,
			RetryAfterSeconds: firstInt64Pointer(e.RetryAfterSeconds, e.RetryAfterSecondsCamel),
			NextRetryAt:       firstTimePointer(e.NextRetryAt, e.NextRetryAtCamel),
			TraceID:           e.traceID(),
		}
	}
	return nil
}

func (e responseEnvelope) requestID() string {
	return firstNonEmpty(e.RequestID, e.RequestID2)
}

func (e responseEnvelope) traceID() string {
	return firstNonEmpty(e.TraceID, e.TraceID2, firstArgumentString(e.Arguments))
}

func decodeResult(raw json.RawMessage, out any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if sub, ok := out.(*Subscription); ok {
		if decoded, ok := decodeSubscriptionResult(raw); ok {
			*sub = decoded
			return nil
		}
	}
	return json.Unmarshal(raw, out)
}

func decodeSubscriptionResult(raw json.RawMessage) (Subscription, bool) {
	var ids []string
	if err := json.Unmarshal(raw, &ids); err == nil {
		if len(ids) == 0 {
			return Subscription{}, true
		}
		return Subscription{SubscribeID: ids[0]}, true
	}
	var item dwsSubscription
	if err := json.Unmarshal(raw, &item); err == nil &&
		(firstNonEmpty(item.SubID, item.SubscribeID) != "" ||
			firstNonEmpty(item.EventKey, item.EventKeySnake) != "") {
		return item.toSubscription(), true
	}
	return Subscription{}, false
}

func decodeAPIError(data []byte) *APIError {
	var env responseEnvelope
	if err := json.Unmarshal(data, &env); err == nil {
		if apiErr := env.apiError(); apiErr != nil {
			return apiErr
		}
	}
	var apiErr APIError
	if err := json.Unmarshal(data, &apiErr); err == nil && (apiErr.Code != "" || apiErr.Message != "") {
		return &apiErr
	}
	return nil
}

func responseRequestID(data []byte) string {
	var env responseEnvelope
	if err := json.Unmarshal(data, &env); err == nil {
		return env.requestID()
	}
	return ""
}

func responseBodyTraceID(data []byte) string {
	var env responseEnvelope
	if err := json.Unmarshal(data, &env); err == nil {
		if env.Error != nil && strings.TrimSpace(env.Error.TraceID) != "" {
			return strings.TrimSpace(env.Error.TraceID)
		}
		return env.traceID()
	}
	return ""
}

func withRequestDetails(apiErr *APIError, method, path string, status int, requestID string) *APIError {
	if apiErr == nil {
		return nil
	}
	apiErr.HTTPStatus = status
	if apiErr.Details == nil {
		apiErr.Details = make(map[string]any, 8)
	}
	apiErr.Details["method"] = method
	apiErr.Details["path"] = path
	apiErr.Details["http_status"] = status
	if requestID != "" {
		apiErr.Details["request_id"] = requestID
	}
	return apiErr
}

func withHTTPResponseDetails(apiErr *APIError, method, path string, resp *http.Response, requestID, traceID string) *APIError {
	if apiErr == nil {
		return nil
	}
	status := 0
	if resp != nil {
		status = resp.StatusCode
		traceID = firstNonEmpty(apiErr.TraceID, traceID, responseTraceID(resp.Header))
	}
	apiErr = withRequestDetails(apiErr, method, path, status, requestID)
	if apiErr.TraceID == "" {
		apiErr.TraceID = strings.TrimSpace(traceID)
	}
	if resp == nil {
		return apiErr
	}

	retryAfter := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if retryAfter == "" {
		return apiErr
	}
	apiErr.Details["retry_after"] = retryAfter
	if apiErr.RetryAfterSeconds == nil && apiErr.NextRetryAt == nil {
		if seconds, err := strconv.ParseInt(retryAfter, 10, 64); err == nil && seconds >= 0 {
			apiErr.RetryAfterSeconds = &seconds
		} else if next, err := http.ParseTime(retryAfter); err == nil {
			next = next.UTC()
			apiErr.NextRetryAt = &next
		}
	}
	if apiErr.RetryAfterSeconds != nil {
		apiErr.Details["retry_after_seconds"] = *apiErr.RetryAfterSeconds
	}
	if apiErr.NextRetryAt != nil {
		apiErr.Details["next_retry_at"] = apiErr.NextRetryAt.UTC().Format(time.RFC3339)
	}
	return apiErr
}

func responseTraceID(headers http.Header) string {
	for _, key := range []string{"X-Trace-Id", "X-Dingtalk-Trace-Id"} {
		if value := strings.TrimSpace(headers.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func responseHeaderRequestID(headers http.Header) string {
	return strings.TrimSpace(headers.Get("X-Request-Id"))
}

func firstArgumentString(arguments []json.RawMessage) string {
	if len(arguments) == 0 {
		return ""
	}
	var value string
	if err := json.Unmarshal(arguments[0], &value); err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func firstInt64Pointer(values ...*int64) *int64 {
	for _, value := range values {
		if value == nil {
			continue
		}
		copy := *value
		return &copy
	}
	return nil
}

func firstTimePointer(values ...*time.Time) *time.Time {
	for _, value := range values {
		if value == nil || value.IsZero() {
			continue
		}
		copy := value.UTC()
		return &copy
	}
	return nil
}

func logControlRequest(message, method, path string, q url.Values, status int, requestPayload, responsePayload, requestID string, apiErr *APIError) {
	attrs := []any{
		"method", method,
		"path", path,
		"http_status", status,
	}
	if query := redactedQueryString(q); query != "" {
		attrs = append(attrs, "query", query)
	}
	if requestPayload != "" {
		attrs = append(attrs, "request", requestPayload)
	}
	if responsePayload != "" {
		attrs = append(attrs, "response", responsePayload)
	}
	if requestID != "" {
		attrs = append(attrs, "request_id", requestID)
	}
	if apiErr != nil {
		if apiErr.TraceID != "" {
			attrs = append(attrs, "trace_id", apiErr.TraceID)
		}
		if apiErr.Code != "" {
			attrs = append(attrs, "error_code", apiErr.Code)
		}
		if apiErr.Message != "" {
			attrs = append(attrs, "error_msg", apiErr.Message)
		}
	}
	slog.Debug(message, attrs...)
}

func sanitizeLogPayload(data []byte) string {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return ""
	}
	var parsed any
	if err := json.Unmarshal(data, &parsed); err == nil {
		redacted := redactJSONValue(parsed)
		s, _ := marshalLogJSON(redacted) // JSON-decoded values are always encodable.
		return truncateLogPayload(s)
	}
	return truncateLogPayload(string(data))
}

func marshalLogJSON(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}

func redactJSONValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, value := range x {
			if sensitiveLogKey(k) {
				out[k] = "<redacted>"
				continue
			}
			out[k] = redactJSONValue(value)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, value := range x {
			out[i] = redactJSONValue(value)
		}
		return out
	default:
		return v
	}
}

func sensitiveLogKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(key, "token") ||
		strings.Contains(key, "secret") ||
		strings.Contains(key, "ticket") ||
		strings.Contains(key, "authorization")
}

func redactedQueryString(q url.Values) string {
	if len(q) == 0 {
		return ""
	}
	clone := make(url.Values, len(q))
	for key, values := range q {
		if sensitiveLogKey(key) {
			clone[key] = []string{"<redacted>"}
			continue
		}
		clone[key] = append([]string(nil), values...)
	}
	return clone.Encode()
}

func truncateLogPayload(s string) string {
	if len(s) <= controlLogPayloadLimit {
		return s
	}
	return s[:controlLogPayloadLimit] + "...<truncated>"
}

func isNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && (apiErr.Code == "PERSONAL_EVENT_NOT_FOUND" || apiErr.Code == "NOT_FOUND")
}

func dwsStatusString(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		switch n {
		case 1:
			return "active"
		case 2:
			return "paused"
		case 3:
			return "deleted"
		default:
			return fmt.Sprintf("%d", n)
		}
	}
	return string(raw)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
