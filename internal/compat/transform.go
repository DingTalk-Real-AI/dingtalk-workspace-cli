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

package compat

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// ApplyTransform applies a named transform rule to a value.
// Supported transforms: iso8601_to_millis, csv_to_array, json_parse,
// json_parse_strict, enum_map, file_read.
func ApplyTransform(value any, transform string, args map[string]any) (any, error) {
	switch strings.TrimSpace(transform) {
	case "":
		return value, nil
	case "iso8601_to_millis":
		return transformISO8601ToMillis(value)
	case "csv_to_array":
		return transformCSVToArray(value)
	case "json_parse":
		return transformJSONParse(value)
	case "json_parse_strict":
		return transformJSONParseStrict(value)
	case "enum_map":
		return transformEnumMap(value, args)
	case "file_read":
		return transformFileRead(value)
	default:
		return value, nil
	}
}

func transformISO8601ToMillis(value any) (any, error) {
	s, ok := toString(value)
	if !ok {
		return value, nil
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return value, nil
	}
	// Try direct millisecond integer first.
	if millis, err := strconv.ParseInt(s, 10, 64); err == nil && millis > 1_000_000_000_000 {
		return millis, nil
	}

	layouts := []struct {
		layout   string
		location *time.Location
	}{
		{layout: time.RFC3339},
		{layout: "2006-01-02T15:04:05"},
		{layout: "2006-01-02 15:04:05"},
		{layout: "2006-01-02", location: time.UTC},
	}
	for _, candidate := range layouts {
		var (
			parsed time.Time
			err    error
		)
		if candidate.location != nil {
			parsed, err = time.ParseInLocation(candidate.layout, s, candidate.location)
		} else {
			parsed, err = time.Parse(candidate.layout, s)
		}
		if err == nil {
			return parsed.UnixMilli(), nil
		}
	}
	return nil, apperrors.NewValidation(fmt.Sprintf("iso8601_to_millis: cannot parse %q as ISO-8601", s))
}

func transformCSVToArray(value any) (any, error) {
	s, ok := toString(value)
	if !ok {
		// If it's already a slice, pass through.
		return value, nil
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return []any{}, nil
	}
	// If already looks like a JSON array, try parsing it.
	if strings.HasPrefix(s, "[") {
		var arr []any
		if err := json.Unmarshal([]byte(s), &arr); err == nil {
			return arr, nil
		}
	}
	parts := strings.Split(s, ",")
	result := make([]any, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result, nil
}

// transformJSONParse parses a CLI string into a structured value so callers can
// pass complex payloads (JSON arrays/objects) through a single flag.
//
// Two input dialects are accepted, in order:
//  1. Strict JSON  — `[{"fieldName":"x","type":"text"}]`
//  2. YAML (flow)  — `[{fieldName: x, type: text}]`
//
// YAML is a superset of JSON that permits unquoted keys and strings, which
// dramatically reduces the need for shell-level escaping. Users can therefore
// write `--fields '[{fieldName: 标题, type: text}]'` instead of piling quotes
// around every token. The output shape is the same either way; downstream
// consumers see the parsed Go value, not the original dialect.
func transformJSONParse(value any) (any, error) {
	s, ok := toString(value)
	if !ok {
		return value, nil
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return value, nil
	}
	// Strict JSON first — fast path and unambiguous type promotion (numbers
	// stay numbers, etc.).
	var parsed any
	if err := json.Unmarshal([]byte(s), &parsed); err == nil {
		return parsed, nil
	}
	// YAML (flow) fallback — accepts `{key: value}` without surrounding
	// quotes, which is the natural form when typing at a shell prompt.
	if err := yaml.Unmarshal([]byte(s), &parsed); err == nil {
		return parsed, nil
	}
	return nil, apperrors.NewValidation(
		"json_parse: input is not valid JSON or YAML; " +
			"quote the whole value and use `[{key: value, ...}]` for ad-hoc input, " +
			"or pass `@path/to/file.json` to read from a file",
	)
}

// transformJSONParseStrict is the strict variant of json_parse: only accepts
// well-formed JSON, rejecting input that the YAML fallback would otherwise
// silently coerce to a scalar string. Use when the upstream tool requires a
// structured array/object value and "garbage in → empty out" is unacceptable.
func transformJSONParseStrict(value any) (any, error) {
	s, ok := toString(value)
	if !ok {
		return value, nil
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return value, nil
	}
	var parsed any
	if err := json.Unmarshal([]byte(s), &parsed); err != nil {
		return nil, apperrors.NewValidation(
			"json_parse_strict: input is not valid JSON; " +
				"this transform rejects YAML-style ad-hoc input — quote the whole value " +
				"as strict JSON (e.g. '[{\"key\":\"value\"}]') or use `json_parse` for YAML-tolerant parsing",
		)
	}
	return parsed, nil
}

func transformEnumMap(value any, args map[string]any) (any, error) {
	s, ok := toString(value)
	if !ok {
		s = fmt.Sprint(value)
	}
	s = strings.TrimSpace(s)

	if mapped, exists := args[s]; exists {
		return mapped, nil
	}
	if defaultVal, exists := args["_default"]; exists {
		return defaultVal, nil
	}
	return value, nil
}

// transformFileRead reads the file at the given path and returns its contents
// as a UTF-8 string. The special path "-" reads from stdin. Use with MapsTo to
// route a path-typed CLI flag (e.g. --content-file ./a.md) into a content-typed
// MCP parameter (e.g. markdown).
//
// Errors are surfaced as validation errors so the dispatcher returns exit code 2
// (user input) rather than the generic exit code 1 (transient failure).
func transformFileRead(value any) (any, error) {
	s, ok := toString(value)
	if !ok {
		return nil, apperrors.NewValidation("file_read: expected string path, got non-string value")
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, apperrors.NewValidation("file_read: empty path")
	}
	var buf []byte
	var err error
	if s == "-" {
		buf, err = io.ReadAll(os.Stdin)
		if err != nil {
			return nil, apperrors.NewValidation(fmt.Sprintf("file_read: read stdin: %v", err))
		}
	} else {
		buf, err = os.ReadFile(s)
		if err != nil {
			return nil, apperrors.NewValidation(fmt.Sprintf("file_read: read %q: %v", s, err))
		}
	}
	if !utf8.Valid(buf) {
		return nil, apperrors.NewValidation(fmt.Sprintf("file_read: %q is not valid UTF-8", s))
	}
	return string(buf), nil
}

func toString(v any) (string, bool) {
	switch val := v.(type) {
	case string:
		return val, true
	case fmt.Stringer:
		return val.String(), true
	default:
		return "", false
	}
}
