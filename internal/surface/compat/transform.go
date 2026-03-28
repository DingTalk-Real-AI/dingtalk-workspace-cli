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
	"strconv"
	"strings"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/platform/errors"
)

// ApplyTransform applies a named transform rule to a value.
// Supported transforms: iso8601_to_millis, csv_to_array, json_parse, enum_map.
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
	case "enum_map":
		return transformEnumMap(value, args)
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

func transformJSONParse(value any) (any, error) {
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
		return nil, apperrors.NewValidation(fmt.Sprintf("json_parse: invalid JSON: %v", err))
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
