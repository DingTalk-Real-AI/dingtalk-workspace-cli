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
	"os"
	"path/filepath"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

// ParseJSONMap parses a --params flag value into a map[string]any.
// Supports:
//   - JSON string: '{"key":"value"}'
//   - "-" to read from stdin
//   - "@file" to read from a JSON file
//   - Empty string returns nil (no params)
func ParseJSONMap(raw, flagName string, stdin io.Reader) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	if raw == "-" || strings.HasPrefix(raw, "@") {
		data, err := readJSONInput(raw, flagName, stdin)
		if err != nil {
			return nil, err
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

	if raw == "-" || strings.HasPrefix(raw, "@") {
		data, err := readJSONInput(raw, "--data", stdin)
		if err != nil {
			return nil, err
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

func readJSONInput(raw, flagName string, stdin io.Reader) ([]byte, error) {
	var reader io.Reader
	var closeFn func() error
	description := "stdin"
	if raw == "-" {
		if stdin == nil {
			return nil, fmt.Errorf("%s 指定从 stdin 读取，但 stdin 不可用", flagName)
		}
		reader = stdin
	} else {
		path := strings.TrimSpace(strings.TrimPrefix(raw, "@"))
		if path == "" {
			return nil, fmt.Errorf("%s 的 @file 路径不能为空", flagName)
		}
		if err := ValidateUserInput(path, flagName+" file"); err != nil {
			return nil, err
		}
		if strings.ContainsAny(path, "\r\n\t") {
			return nil, fmt.Errorf("%s file 路径不能包含换行或制表符", flagName)
		}
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("打开 %s 文件 %q 失败: %w", flagName, path, err)
		}
		reader = file
		closeFn = file.Close
		description = path
	}
	if closeFn != nil {
		defer closeFn()
	}
	data, err := io.ReadAll(io.LimitReader(reader, config.MaxResponseBodySize+1))
	if err != nil {
		return nil, fmt.Errorf("从 %s 读取 %s 失败: %w", description, flagName, err)
	}
	if len(data) > config.MaxResponseBodySize {
		return nil, fmt.Errorf("%s 输入超过安全上限 %d 字节", flagName, config.MaxResponseBodySize)
	}
	return data, nil
}

// IsDeferredInput reports whether a value would read bytes from stdin or a
// file. Dry-run uses it to avoid touching either input source.
func IsDeferredInput(raw string) bool {
	raw = strings.TrimSpace(raw)
	return raw == "-" || strings.HasPrefix(raw, "@")
}

// ParseFileSpec parses --file [field=]path without opening the file.
func ParseFileSpec(raw string) (*FileUpload, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	field, path := "file", raw
	if before, after, found := strings.Cut(raw, "="); found && isExplicitFileField(before) {
		field, path = strings.TrimSpace(before), strings.TrimSpace(after)
	}
	if field == "" || path == "" {
		return nil, fmt.Errorf("--file 格式必须为 [field=]path 或 [field=]-")
	}
	if err := ValidateUserInput(field, "--file field"); err != nil {
		return nil, err
	}
	if err := ValidateUserInput(path, "--file path"); err != nil {
		return nil, err
	}
	if strings.ContainsAny(field, "\r\n\t") || strings.ContainsAny(path, "\r\n\t") {
		return nil, fmt.Errorf("--file field 和 path 不能包含换行或制表符")
	}
	filename := "stdin"
	if path != "-" {
		filename = filepath.Base(path)
	}
	return &FileUpload{FieldName: field, Path: path, FileName: filename}, nil
}

func isExplicitFileField(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	return !strings.ContainsAny(value, `/\\`)
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
