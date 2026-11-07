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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

// ResponseOptions controls how an API response is processed.
type ResponseOptions struct {
	OutputPath string        // --output file path for binary responses
	Format     output.Format // output format (json|table|raw)
	JqExpr     string        // --jq expression
	Fields     string        // --fields comma-separated field names
	Out        io.Writer     // stdout
	ErrOut     io.Writer     // stderr
}

// ResponseError marks failures caused by the remote HTTP/business response.
// Local rendering, jq validation, and download filesystem failures deliberately
// remain unwrapped so the CLI can preserve their validation/internal category.
type ResponseError struct {
	Err error
}

var (
	responseCreateTemp = os.CreateTemp
	responseFileSync   = func(file *os.File) error { return file.Sync() }
	responseFileClose  = func(file *os.File) error { return file.Close() }
	responseChmod      = os.Chmod
	responseReplace    = atomicReplace
)

func (e *ResponseError) Error() string {
	if e == nil || e.Err == nil {
		return "API response error"
	}
	return e.Err.Error()
}

func (e *ResponseError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newResponseError(err error) error {
	if err == nil {
		return nil
	}
	return &ResponseError{Err: err}
}

// HandleResponse routes response processing based on Content-Type and status code.
func HandleResponse(resp *RawAPIResponse, opts ResponseOptions) error {
	if resp == nil {
		return newResponseError(fmt.Errorf("API 返回空响应"))
	}
	defer func() {
		if resp.BodyReader != nil {
			_ = resp.BodyReader.Close()
			resp.BodyReader = nil
		}
	}()
	contentType := resp.Header.Get("Content-Type")
	isJSON := isJSONContentType(contentType)

	// HTTP error with non-JSON body: print as plain text error.
	if resp.StatusCode >= 400 && !isJSON {
		body, err := readBoundedResponse(resp)
		if err != nil {
			return newResponseError(fmt.Errorf("API 请求失败 (HTTP %d): %w", resp.StatusCode, err))
		}
		return newResponseError(fmt.Errorf("API 请求失败 (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body))))
	}

	// JSON response
	if isJSON {
		return handleJSONResponse(resp, opts)
	}

	// Binary response
	return handleBinaryResponse(resp, opts)
}

// handleJSONResponse parses the JSON body, checks for DingTalk business errors,
// and writes the output using the configured format and filters.
func handleJSONResponse(resp *RawAPIResponse, opts ResponseOptions) error {
	body, err := readBoundedResponse(resp)
	if err != nil {
		return newResponseError(err)
	}
	if len(body) == 0 {
		return newResponseError(fmt.Errorf("API 返回空响应体 (HTTP %d)，如需下载文件请使用 --output 参数", resp.StatusCode))
	}

	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return newResponseError(fmt.Errorf("解析 JSON 响应失败: %w", err))
	}

	// Check for DingTalk business error: {"errcode": xxx, "errmsg": "xxx"}
	requestID := firstHeader(resp.Header, "x-acs-request-id", "x-acs-dingtalk-request-id", "x-request-id")
	if apiErr := checkDingTalkErrorWithRequestID(payload, resp.StatusCode, requestID); apiErr != nil {
		return newResponseError(apiErr)
	}

	return output.WriteFiltered(opts.Out, opts.Format, payload, opts.Fields, opts.JqExpr)
}

// checkDingTalkError inspects a parsed JSON response for DingTalk error codes.
// Returns nil if no error is detected.
func checkDingTalkError(payload any, statusCode int) error {
	return checkDingTalkErrorWithRequestID(payload, statusCode, "")
}

func checkDingTalkErrorWithRequestID(payload any, statusCode int, headerRequestID string) error {
	obj, ok := payload.(map[string]any)
	if !ok {
		return nil
	}

	requestID := firstString(obj, "requestId", "request_id", "requestid")
	if requestID == "" {
		requestID = strings.TrimSpace(headerRequestID)
	}
	requestSuffix := ""
	if requestID != "" {
		requestSuffix = ", requestId: " + requestID
	}

	// Check for errcode != 0. Keep the historical prefix for compatibility.
	if errcode, hasCode := obj["errcode"]; hasCode {
		code, nonzero := errorCode(errcode)
		if nonzero {
			errmsg := firstString(obj, "errmsg", "message", "error")
			if errmsg == "" {
				errmsg = "unknown error"
			}
			return fmt.Errorf("API 业务错误 (errcode: %s, HTTP %d%s): %s", code, statusCode, requestSuffix, errmsg)
		}
	}
	// New OpenAPI error envelopes use top-level code on non-2xx responses.
	// Successful payloads may also have an unrelated business field named code,
	// so preserve the historical 2xx behavior and only interpret it as an error
	// when the HTTP status already indicates failure.
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		if codeValue, hasCode := obj["code"]; hasCode {
			if code, nonzero := errorCode(codeValue); nonzero {
				message := firstString(obj, "message", "errmsg", "error")
				if message == "" {
					message = "unknown error"
				}
				return fmt.Errorf("API 业务错误 (code: %s, HTTP %d%s): %s", code, statusCode, requestSuffix, message)
			}
		}
	}

	// Preserve the historical generic-error boundary for responses without a
	// DingTalk error code. A 1xx/3xx JSON body without an error envelope remains
	// raw output; top-level code envelopes above are still errors on any non-2xx.
	if statusCode >= http.StatusBadRequest {
		errmsg := firstString(obj, "errmsg", "message", "error")
		if errmsg != "" {
			return fmt.Errorf("API 请求失败 (HTTP %d%s): %s", statusCode, requestSuffix, errmsg)
		}
		return fmt.Errorf("API 请求失败 (HTTP %d%s)", statusCode, requestSuffix)
	}

	return nil
}

// handleBinaryResponse saves the response body to a file.
func handleBinaryResponse(resp *RawAPIResponse, opts ResponseOptions) error {
	outputPath := strings.TrimSpace(opts.OutputPath)

	if outputPath == "" {
		// Try to infer filename from Content-Disposition header.
		outputPath = inferFilename(resp.Header)
		if outputPath == "" {
			return fmt.Errorf("响应为非 JSON 格式 (Content-Type: %s)，请使用 --output 指定保存路径",
				resp.Header.Get("Content-Type"))
		}
	}

	dir := filepath.Dir(outputPath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("创建输出目录失败: %w", err)
		}
	}

	reader, closeFn := responseBodyReader(resp)
	if closeFn != nil {
		defer closeFn()
	}
	tmp, err := responseCreateTemp(dir, "."+filepath.Base(outputPath)+"-*.part")
	if err != nil {
		return fmt.Errorf("创建临时下载文件失败: %w", err)
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()
	written, err := io.Copy(tmp, reader)
	if err != nil {
		return fmt.Errorf("写入下载文件失败: %w", err)
	}
	if err := responseFileSync(tmp); err != nil {
		return fmt.Errorf("同步下载文件失败: %w", err)
	}
	if err := responseFileClose(tmp); err != nil {
		return fmt.Errorf("关闭下载文件失败: %w", err)
	}
	if err := responseChmod(tmpPath, 0o644); err != nil {
		return fmt.Errorf("设置下载文件权限失败: %w", err)
	}
	if err := responseReplace(tmpPath, outputPath); err != nil {
		return fmt.Errorf("原子替换下载文件失败: %w", err)
	}
	committed = true

	fmt.Fprintf(opts.ErrOut, "已保存到: %s (%d 字节)\n", outputPath, written)
	return nil
}

// inferFilename tries to extract a filename from the Content-Disposition header.
func inferFilename(header http.Header) string {
	cd := header.Get("Content-Disposition")
	if cd == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(cd)
	if err != nil {
		return ""
	}
	filename := strings.TrimSpace(params["filename"])
	if filename == "" || strings.ContainsRune(filename, '\x00') {
		return ""
	}
	filename = filepath.Base(strings.ReplaceAll(filename, "\\", "/"))
	if filename == "." || filename == ".." || filename == string(filepath.Separator) {
		return ""
	}
	return filename
}

func readBoundedResponse(resp *RawAPIResponse) ([]byte, error) {
	reader, closeFn := responseBodyReader(resp)
	if closeFn != nil {
		defer closeFn()
	}
	data, err := io.ReadAll(io.LimitReader(reader, config.MaxResponseBodySize+1))
	if err != nil {
		return nil, fmt.Errorf("读取 API 响应失败: %w", err)
	}
	if len(data) > config.MaxResponseBodySize {
		return nil, fmt.Errorf("API 响应超过安全上限 %d 字节", config.MaxResponseBodySize)
	}
	resp.Body = data
	resp.BodyReader = nil
	return data, nil
}

func responseBodyReader(resp *RawAPIResponse) (io.Reader, func() error) {
	if resp.BodyReader != nil {
		return resp.BodyReader, resp.BodyReader.Close
	}
	return bytes.NewReader(resp.Body), nil
}

func firstString(obj map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := obj[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstHeader(header http.Header, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(header.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func errorCode(value any) (string, bool) {
	switch typed := value.(type) {
	case float64:
		return fmt.Sprintf("%.0f", typed), typed != 0 && typed != http.StatusOK
	case json.Number:
		return typed.String(), typed.String() != "0" && typed.String() != "200"
	case string:
		trimmed := strings.TrimSpace(typed)
		return trimmed, trimmed != "" && trimmed != "0" && trimmed != "200" &&
			!strings.EqualFold(trimmed, "ok") && !strings.EqualFold(trimmed, "success")
	case int:
		return fmt.Sprint(typed), typed != 0 && typed != http.StatusOK
	case int64:
		return fmt.Sprint(typed), typed != 0 && typed != http.StatusOK
	default:
		return "", false
	}
}

// isJSONContentType returns true if the Content-Type indicates JSON.
func isJSONContentType(ct string) bool {
	ct = strings.TrimSpace(strings.ToLower(ct))
	return strings.HasPrefix(ct, "application/json") ||
		strings.HasPrefix(ct, "text/json") ||
		strings.Contains(ct, "+json")
}

// toFloat64 attempts to convert a JSON number to float64.
func toFloat64(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return 0
}
