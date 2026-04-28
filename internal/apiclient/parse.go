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
	"strings"
)

// ParseJSONMap parses a --params flag value into a map[string]any.
// Supports:
//   - JSON string: '{"key":"value"}'
//   - "-" to read from stdin
//   - Empty string returns nil (no params)
func ParseJSONMap(raw, flagName string, stdin io.Reader) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	if raw == "-" {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("从 stdin 读取 %s 失败: %w", flagName, err)
		}
		raw = strings.TrimSpace(string(data))
		if raw == "" {
			return nil, nil
		}
	}

	// Strip wrapping single quotes (common shell escaping).
	raw = stripSingleQuotes(raw)

	var result map[string]any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("解析 %s JSON 失败: %w\n输入: %s", flagName, err, truncate(raw, 200))
	}
	return result, nil
}

// ParseOptionalBody parses a --data flag value into a request body.
// Returns nil for empty input. GET requests are not allowed to have a body.
func ParseOptionalBody(method, raw string, stdin io.Reader) (any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	if strings.ToUpper(method) == "GET" && raw != "" {
		return nil, fmt.Errorf("GET 请求不允许使用 --data 参数")
	}

	if raw == "-" {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("从 stdin 读取 --data 失败: %w", err)
		}
		raw = strings.TrimSpace(string(data))
		if raw == "" {
			return nil, nil
		}
	}

	// Strip wrapping single quotes.
	raw = stripSingleQuotes(raw)

	var result any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("解析 --data JSON 失败: %w\n输入: %s", err, truncate(raw, 200))
	}
	return result, nil
}

// stripSingleQuotes removes a leading and trailing single quote pair.
func stripSingleQuotes(s string) string {
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return s[1 : len(s)-1]
	}
	return s
}

// truncate returns at most n characters of s, appending "..." if truncated.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
