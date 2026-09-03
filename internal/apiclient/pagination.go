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

package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

const (
	// DefaultPageLimit is the maximum number of pages fetched with --page-all
	// when --page-limit is not explicitly set.
	DefaultPageLimit = 10

	// MaxPageLimit is the hard safety cap to prevent infinite loops when an
	// API endpoint has a bug that causes has_more to never become false.
	// Use --page-limit 0 to hit this cap; any explicit positive value is
	// honoured up to this ceiling.
	MaxPageLimit = 500

	// DefaultPageDelay is the delay between paginated requests in milliseconds.
	DefaultPageDelay = 200
)

// PaginationOptions controls automatic pagination behaviour.
type PaginationOptions struct {
	PageLimit int       // Maximum pages (0 = unlimited, capped at MaxPageLimit)
	PageDelay int       // Delay between pages in milliseconds
	LogWriter io.Writer // Optional: progress log output (typically stderr)
}

// PaginateAll fetches all pages of a paginated API and merges the results.
// DingTalk APIs use two pagination patterns:
//   - cursor/next_cursor/has_more (in response body)
//   - next_token (in response body)
//
// The function auto-detects which pattern the API uses.
func (c *APIClient) PaginateAll(ctx context.Context, req RawAPIRequest, opts PaginationOptions) ([]any, error) {
	limit := resolvePageLimit(opts.PageLimit)
	if opts.PageDelay <= 0 {
		opts.PageDelay = DefaultPageDelay
	}

	var allResults []any
	pageCount := 0

	for {
		pageCount++

		// Safety cap — only break if a carry is active (pageCount > 1).
		if limit > 0 && pageCount > limit {
			logf(opts.LogWriter, "[pagination] ⚠ 已达安全上限 %d 页，停止翻页。数据可能不完整，请检查 API 是否异常。\n", limit)
			break
		}

		logf(opts.LogWriter, "[pagination] 第 %d 页 请求中...\n", pageCount)

		resp, err := c.Do(ctx, req)
		if err != nil {
			if pageCount == 1 {
				return nil, err
			}
			// Non-first page error: return what we have so far.
			return allResults, fmt.Errorf("分页第 %d 页请求失败 (已获取 %d 页结果): %w", pageCount, pageCount-1, err)
		}

		result, hasMore, continuation, parseErr := parsePaginatedResponseDetails(resp)
		if parseErr != nil {
			if pageCount == 1 {
				return nil, parseErr
			}
			// Never report a partial page set as a successful complete result.
			return allResults, fmt.Errorf("分页第 %d 页解析失败 (已获取 %d 页结果): %w", pageCount, pageCount-1, parseErr)
		}

		allResults = append(allResults, result)

		if !hasMore {
			logf(opts.LogWriter, "[pagination] 数据获取完成 (共 %d 页)\n", pageCount)
			break
		}
		// Inject the next page token into the request.
		req, parseErr = injectPageTokenWithKey(req, continuation)
		if parseErr != nil {
			return allResults, parseErr
		}

		// Delay between pages to prevent API throttling.
		select {
		case <-ctx.Done():
			return allResults, ctx.Err()
		case <-time.After(time.Duration(opts.PageDelay) * time.Millisecond):
		}
	}

	return allResults, nil
}

// resolvePageLimit translates the user-facing value into an internal limit:
//
//	0           → MaxPageLimit (user wants unlimited; safety cap applies)
//	positive N  → min(N, MaxPageLimit) (explicit page limit, still capped)
//	negative    → DefaultPageLimit (invalid input treated as default)
func resolvePageLimit(raw int) int {
	if raw == 0 {
		return MaxPageLimit
	}
	if raw < 0 {
		return DefaultPageLimit
	}
	if raw > MaxPageLimit {
		return MaxPageLimit
	}
	return raw
}

func logf(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, format, args...)
}

// parsePaginatedResponse extracts the response payload and pagination info.
// It auto-detects DingTalk's two pagination patterns.
func parsePaginatedResponse(resp *RawAPIResponse) (result any, hasMore bool, nextToken string, err error) {
	result, hasMore, continuation, err := parsePaginatedResponseDetails(resp)
	return result, hasMore, continuation.Value, err
}

type paginationContinuation struct {
	Value      string
	RequestKey string
}

func parsePaginatedResponseDetails(resp *RawAPIResponse) (result any, hasMore bool, continuation paginationContinuation, err error) {
	contentType := resp.Header.Get("Content-Type")
	if !isJSONContentType(contentType) {
		if resp.BodyReader != nil {
			_ = resp.BodyReader.Close()
		}
		return nil, false, continuation, fmt.Errorf("分页响应非 JSON 格式 (Content-Type: %s)", contentType)
	}

	body, readErr := readBoundedResponse(resp)
	if readErr != nil {
		return nil, false, continuation, readErr
	}
	if len(body) == 0 {
		return nil, false, continuation, fmt.Errorf("分页响应体为空 (HTTP %d)", resp.StatusCode)
	}

	var payload map[string]any
	if unmarshalErr := jsonUnmarshal(body, &payload); unmarshalErr != nil {
		return nil, false, continuation, fmt.Errorf("解析分页 JSON 响应失败: %w", unmarshalErr)
	}

	// Check for DingTalk errors first.
	requestID := firstHeader(resp.Header, "x-acs-request-id", "x-acs-dingtalk-request-id", "x-request-id")
	if apiErr := checkDingTalkErrorWithRequestID(payload, resp.StatusCode, requestID); apiErr != nil {
		return nil, false, continuation, apiErr
	}

	containers := []map[string]any{payload}
	if nested, ok := payload["result"].(map[string]any); ok {
		containers = append([]map[string]any{nested}, containers...)
	}
	var hasMoreSet bool
	for _, container := range containers {
		for _, key := range []string{"has_more", "hasMore"} {
			if value, ok := container[key].(bool); ok {
				if hasMoreSet && hasMore != value {
					return nil, false, continuation, fmt.Errorf("分页响应包含冲突的 hasMore 字段")
				}
				hasMore, hasMoreSet = value, true
			}
		}
		for _, candidate := range []struct {
			responseKey string
			requestKey  string
		}{
			{"next_cursor", "cursor"},
			{"nextCursor", "cursor"},
			{"next_token", "next_token"},
			{"nextToken", "nextToken"},
		} {
			value := paginationValue(container[candidate.responseKey])
			if value == "" {
				continue
			}
			if continuation.Value != "" && (continuation.Value != value || continuation.RequestKey != candidate.requestKey) {
				return nil, false, paginationContinuation{}, fmt.Errorf("分页响应包含冲突的 continuation 字段")
			}
			continuation = paginationContinuation{Value: value, RequestKey: candidate.requestKey}
		}
	}
	if continuation.Value != "" && !hasMoreSet {
		hasMore = true
	}
	if hasMore && continuation.Value == "" {
		return nil, false, paginationContinuation{}, fmt.Errorf("分页响应返回 hasMore=true，但未提供 next cursor/token")
	}
	return payload, hasMore, continuation, nil
}

// injectPageToken injects the pagination token into the next request.
// For GET requests, it's added as a query param; for POST, it's in the body.
func injectPageToken(req RawAPIRequest, token string) RawAPIRequest {
	key := "next_token"
	if req.Method == "GET" && req.Params != nil {
		for _, candidate := range []string{"cursor", "next_token", "nextToken"} {
			if _, ok := req.Params[candidate]; ok {
				key = candidate
				break
			}
		}
	} else if body, ok := req.Data.(map[string]any); ok {
		for _, candidate := range []string{"cursor", "next_token", "nextToken"} {
			if _, exists := body[candidate]; exists {
				key = candidate
				break
			}
		}
	}
	updated, _ := injectPageTokenWithKey(req, paginationContinuation{Value: token, RequestKey: key})
	return updated
}

func injectPageTokenWithKey(req RawAPIRequest, continuation paginationContinuation) (RawAPIRequest, error) {
	method := req.Method
	if method == "GET" {
		if req.Params == nil {
			req.Params = make(map[string]any)
		}
		req.Params[continuation.RequestKey] = continuation.Value
	} else {
		// For POST/PUT requests, inject into the body
		if bodyMap, ok := req.Data.(map[string]any); ok {
			bodyMap[continuation.RequestKey] = continuation.Value
			req.Data = bodyMap
		} else {
			return req, fmt.Errorf("分页 continuation 无法注入非 object 请求体")
		}
	}
	return req, nil
}

func paginationValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		if typed > 0 {
			return fmt.Sprintf("%.0f", typed)
		}
	case json.Number:
		return typed.String()
	}
	return ""
}

// jsonUnmarshal is a helper for JSON unmarshaling.
func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
