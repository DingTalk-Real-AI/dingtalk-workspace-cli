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
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/requestmeta"
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

var newHTTPRequest = http.NewRequestWithContext

var (
	multipartWriteField = func(writer *multipart.Writer, key, value string) error {
		return writer.WriteField(key, value)
	}
	multipartCreateFormFile = func(writer *multipart.Writer, fieldName, filename string) (io.Writer, error) {
		return writer.CreateFormFile(fieldName, filename)
	}
)

// RawAPIRequest describes a raw API request to DingTalk OpenAPI.
type RawAPIRequest struct {
	Method string         // GET, POST, PUT, PATCH, DELETE
	Path   string         // /v1.0/calendar/events or full URL
	Params map[string]any // query parameters
	Data   any            // request body (JSON), nil for GET
	File   *FileUpload    // optional single streaming multipart file
	// Sources are preview-only metadata for deferred stdin/@file inputs.
	ParamsSource string
	DataSource   string
}

// FileUpload describes one multipart file. Reader is set for stdin and tests;
// when it is nil, Path is opened lazily immediately before the request.
type FileUpload struct {
	FieldName string
	Path      string
	FileName  string
	Reader    io.Reader
}

// RawAPIResponse encapsulates the raw HTTP response.
type RawAPIResponse struct {
	StatusCode int
	Header     http.Header
	// Body remains available for tests and package callers that construct a
	// response in memory. Live requests use BodyReader and are consumed once.
	Body       []byte
	BodyReader io.ReadCloser
}

// APIClient wraps an HTTP client for DingTalk OpenAPI calls.
type APIClient struct {
	BaseURL     string
	HTTPClient  *http.Client
	Token       string
	DingTalkExt string
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
			Transport:     defaultTransport(),
			Timeout:       30 * time.Second,
			CheckRedirect: ValidateRedirect,
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
	var contentType string
	var openedFile io.Closer
	if req.File != nil {
		if method == http.MethodGet {
			return nil, fmt.Errorf("GET 请求不允许使用 multipart 文件")
		}
		fileReader := req.File.Reader
		if fileReader == nil {
			file, openErr := os.Open(req.File.Path)
			if openErr != nil {
				return nil, fmt.Errorf("打开上传文件失败: %w", openErr)
			}
			openedFile = file
			fileReader = file
		}
		defer func() {
			if openedFile != nil {
				_ = openedFile.Close()
			}
		}()
		bodyReader, contentType, err = newMultipartBody(req.File, fileReader, req.Data)
		if err != nil {
			return nil, err
		}
	} else if req.Data != nil && method != "GET" {
		data, marshalErr := json.Marshal(req.Data)
		if marshalErr != nil {
			return nil, fmt.Errorf("marshaling request body: %w", marshalErr)
		}
		bodyReader = bytes.NewReader(data)
		contentType = "application/json"
	}

	httpReq, err := newHTTPRequest(ctx, method, fullURL, bodyReader)
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
	if contentType != "" {
		httpReq.Header.Set("Content-Type", contentType)
	}
	httpReq.Header.Set("User-Agent", "dws-cli/raw-api")
	if c.DingTalkExt != "" {
		httpReq.Header.Set(requestmeta.DingTalkExtHeader, c.DingTalkExt)
	}

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("executing HTTP request: %w", err)
	}
	return &RawAPIResponse{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		BodyReader: resp.Body,
	}, nil
}

func newMultipartBody(upload *FileUpload, fileReader io.Reader, data any) (io.Reader, string, error) {
	if upload == nil || fileReader == nil {
		return nil, "", fmt.Errorf("multipart 文件不能为空")
	}
	fields := map[string]any{}
	if data != nil {
		var ok bool
		fields, ok = data.(map[string]any)
		if !ok {
			return nil, "", fmt.Errorf("使用 --file 时 --data 必须是 JSON object")
		}
	}
	fieldName := strings.TrimSpace(upload.FieldName)
	if fieldName == "" {
		fieldName = "file"
	}
	filename := strings.TrimSpace(upload.FileName)
	if filename == "" {
		filename = filepath.Base(upload.Path)
	}
	if filename == "" || filename == "." || filename == string(filepath.Separator) {
		filename = "stdin"
	}
	if strings.ContainsAny(fieldName, "\r\n") || strings.ContainsAny(filename, "\r\n") {
		return nil, "", fmt.Errorf("multipart field 或 filename 不能包含换行符")
	}
	for key := range fields {
		if strings.ContainsAny(key, "\r\n") {
			return nil, "", fmt.Errorf("multipart 字段名 %q 不能包含换行符", key)
		}
	}

	pipeReader, pipeWriter := io.Pipe()
	mw := multipart.NewWriter(pipeWriter)
	contentType := mw.FormDataContentType()
	go func() {
		var writeErr error
		defer func() {
			if closeErr := mw.Close(); writeErr == nil {
				writeErr = closeErr
			}
			_ = pipeWriter.CloseWithError(writeErr)
		}()

		keys := make([]string, 0, len(fields))
		for key := range fields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			value, valueErr := multipartFieldValue(fields[key])
			if valueErr != nil {
				writeErr = fmt.Errorf("编码 multipart 字段 %q 失败: %w", key, valueErr)
				return
			}
			if writeErr = multipartWriteField(mw, key, value); writeErr != nil {
				return
			}
		}
		part, partErr := multipartCreateFormFile(mw, fieldName, filepath.Base(filename))
		if partErr != nil {
			writeErr = partErr
			return
		}
		_, writeErr = io.Copy(part, fileReader)
	}()
	return pipeReader, contentType, nil
}

func multipartFieldValue(value any) (string, error) {
	if text, ok := value.(string); ok {
		return text, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// buildURL constructs the full request URL from path and query params.
func (c *APIClient) buildURL(path string, params map[string]any) (string, error) {
	if strings.Contains(path, "#") {
		return "", fmt.Errorf("request URL must not contain a fragment")
	}
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
	parsed, err := url.Parse(strings.TrimSpace(urlStr))
	return err == nil && strings.EqualFold(parsed.Hostname(), "oapi.dingtalk.com")
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
		// Honour HTTP_PROXY / HTTPS_PROXY / NO_PROXY env vars (#236).
		Proxy: http.ProxyFromEnvironment,
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
