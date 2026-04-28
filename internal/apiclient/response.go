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
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
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

// HandleResponse routes response processing based on Content-Type and status code.
func HandleResponse(resp *RawAPIResponse, opts ResponseOptions) error {
	contentType := resp.Header.Get("Content-Type")
	isJSON := isJSONContentType(contentType)

	// HTTP error with non-JSON body: print as plain text error.
	if resp.StatusCode >= 400 && !isJSON {
		return fmt.Errorf("API 请求失败 (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(resp.Body)))
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
	if len(resp.Body) == 0 {
		return fmt.Errorf("API 返回空响应体 (HTTP %d)，如需下载文件请使用 --output 参数", resp.StatusCode)
	}

	var payload any
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		return fmt.Errorf("解析 JSON 响应失败: %w", err)
	}

	// Check for DingTalk business error: {"errcode": xxx, "errmsg": "xxx"}
	if apiErr := checkDingTalkError(payload, resp.StatusCode); apiErr != nil {
		return apiErr
	}

	return output.WriteFiltered(opts.Out, opts.Format, payload, opts.Fields, opts.JqExpr)
}

// checkDingTalkError inspects a parsed JSON response for DingTalk error codes.
// Returns nil if no error is detected.
func checkDingTalkError(payload any, statusCode int) error {
	obj, ok := payload.(map[string]any)
	if !ok {
		return nil
	}

	// Check for errcode != 0
	if errcode, hasCode := obj["errcode"]; hasCode {
		code := toFloat64(errcode)
		if code != 0 {
			errmsg, _ := obj["errmsg"].(string)
			if errmsg == "" {
				errmsg = "unknown error"
			}
			return fmt.Errorf("API 业务错误 (errcode: %.0f, HTTP %d): %s", code, statusCode, errmsg)
		}
	}

	// Also check HTTP error status even if no errcode field
	if statusCode >= 400 {
		errmsg, _ := obj["errmsg"].(string)
		if errmsg == "" {
			errmsg, _ = obj["message"].(string)
		}
		if errmsg == "" {
			errmsg, _ = obj["error"].(string)
		}
		if errmsg != "" {
			return fmt.Errorf("API 请求失败 (HTTP %d): %s", statusCode, errmsg)
		}
		return fmt.Errorf("API 请求失败 (HTTP %d)", statusCode)
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

	if err := os.WriteFile(outputPath, resp.Body, 0o644); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}

	fmt.Fprintf(opts.ErrOut, "已保存到: %s (%d 字节)\n", outputPath, len(resp.Body))
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
	return strings.TrimSpace(params["filename"])
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
