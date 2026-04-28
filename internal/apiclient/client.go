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

// Package apiclient provides a lightweight HTTP client for calling DingTalk
// OpenAPI (https://api.dingtalk.com) directly, bypassing the MCP JSON-RPC
// transport. It is used exclusively by the `dws api` command.
package apiclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the DingTalk new-style OpenAPI base URL.
	DefaultBaseURL = "https://api.dingtalk.com"

	// LegacyBaseURL is the DingTalk legacy (oapi) API base URL.
	LegacyBaseURL = "https://oapi.dingtalk.com"

	// AuthHeader is the new-style OpenAPI authentication header.
	AuthHeader = "x-acs-dingtalk-access-token"

	// LegacyAuthParam is the query parameter used for legacy API authentication.
	LegacyAuthParam = "access_token"
)

// AllowedMethods is the set of HTTP methods permitted for raw API calls.
var AllowedMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true,
}

// RawAPIRequest describes a raw API request to DingTalk OpenAPI.
type RawAPIRequest struct {
	Method string         // GET, POST, PUT, PATCH, DELETE
	Path   string         // /v1.0/calendar/events or full URL
	Params map[string]any // query parameters
	Data   any            // request body (JSON), nil for GET
}

// RawAPIResponse encapsulates the raw HTTP response.
type RawAPIResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// APIClient wraps an HTTP client for DingTalk OpenAPI calls.
type APIClient struct {
	BaseURL    string
	HTTPClient *http.Client
	Token      string
}

// NewClient creates an APIClient with sensible defaults.
func NewClient(token, baseURL string) *APIClient {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultBaseURL
	}
	return &APIClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTPClient: &http.Client{
			Transport: defaultTransport(),
			Timeout:   30 * time.Second,
		},
	}
}

// Do sends a raw API request and returns the response.
func (c *APIClient) Do(ctx context.Context, req RawAPIRequest) (*RawAPIResponse, error) {
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if !AllowedMethods[method] {
		return nil, fmt.Errorf("unsupported HTTP method: %s (allowed: GET, POST, PUT, PATCH, DELETE)", req.Method)
	}

	fullURL, err := c.buildURL(req.Path, req.Params)
	if err != nil {
		return nil, fmt.Errorf("building request URL: %w", err)
	}

	// Security: verify target host before sending token.
	if err := ValidateTargetHost(fullURL); err != nil {
		return nil, err
	}

	var bodyReader io.Reader
	if req.Data != nil && method != "GET" {
		data, marshalErr := json.Marshal(req.Data)
		if marshalErr != nil {
			return nil, fmt.Errorf("marshaling request body: %w", marshalErr)
		}
		bodyReader = bytes.NewReader(data)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating HTTP request: %w", err)
	}

	// Set headers and auth based on API style.
	if IsLegacyAPI(fullURL) {
		// Legacy API: token goes in query parameter.
		parsed, _ := url.Parse(fullURL)
		q := parsed.Query()
		q.Set(LegacyAuthParam, c.Token)
		parsed.RawQuery = q.Encode()
		httpReq.URL = parsed
	} else {
		// New API: token goes in header.
		httpReq.Header.Set(AuthHeader, c.Token)
	}
	if bodyReader != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	httpReq.Header.Set("User-Agent", "dws-cli/raw-api")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("executing HTTP request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	return &RawAPIResponse{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       body,
	}, nil
}

// buildURL constructs the full request URL from path and query params.
func (c *APIClient) buildURL(path string, params map[string]any) (string, error) {
	normalised := NormalisePath(path, c.BaseURL)
	parsed, err := url.Parse(normalised)
	if err != nil {
		return "", fmt.Errorf("parsing URL %q: %w", normalised, err)
	}

	if len(params) > 0 {
		q := parsed.Query()
		for k, v := range params {
			q.Set(k, fmt.Sprintf("%v", v))
		}
		parsed.RawQuery = q.Encode()
	}

	return parsed.String(), nil
}

// IsLegacyAPI returns true if the URL targets the legacy oapi.dingtalk.com endpoint.
// Legacy APIs use query-parameter authentication instead of header-based auth.
func IsLegacyAPI(urlStr string) bool {
	lower := strings.ToLower(urlStr)
	return strings.Contains(lower, "oapi.dingtalk.com") ||
		strings.HasPrefix(lower, LegacyBaseURL)
}

// NormalisePath normalises an API path:
//   - Full URLs are accepted as-is (after stripping query/fragment)
//   - Relative paths are prefixed with the base URL
//   - Query strings and fragments are stripped (must use --params)
func NormalisePath(path, baseURL string) string {
	path = strings.TrimSpace(path)

	// Strip query and fragment to force --params usage.
	if idx := strings.IndexAny(path, "?#"); idx >= 0 {
		path = path[:idx]
	}

	// Full URL: extract the path portion relative to the base.
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}

	// Ensure leading slash.
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultBaseURL
	}
	return strings.TrimRight(baseURL, "/") + path
}

// defaultTransport returns a tuned http.Transport matching the project conventions.
func defaultTransport() *http.Transport {
	return &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   3 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     true,
	}
}
