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

package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/itchyny/gojq"
)

// SelectFields filters a JSON-serialisable payload to include only
// the specified field names, following the gh CLI convention:
//
//  1. If the payload is an array of objects, filter each element.
//  2. If the payload is an object containing a "data list" (a nested
//     array under a well-known key like "value", "items", "data",
//     "records", etc.), filter each element of that list.
//  3. Otherwise, filter top-level keys.
//
// This approach lets callers write `--fields title,memberCount` and
// get the right result regardless of the response envelope structure.
// Field names are matched case-insensitively.
func SelectFields(payload any, fields []string) any {
	if len(fields) == 0 {
		return payload
	}

	normalised := toGeneric(payload)

	wanted := make(map[string]bool, len(fields))
	for _, f := range fields {
		wanted[strings.TrimSpace(strings.ToLower(f))] = true
	}

	switch typed := normalised.(type) {
	case []any:
		return filterSlice(typed, wanted)
	case map[string]any:
		// Try to find a nested data list and filter its elements.
		if loc := findDataList(typed); loc != nil {
			filtered := filterSlice(loc.list, wanted)
			result := shallowCopyMap(typed)
			if loc.outerKey == "" {
				// Top-level: {items: [...]}
				result[loc.innerKey] = filtered
			} else {
				// Nested: {result: {value: [...]}}
				inner := shallowCopyMap(typed[loc.outerKey].(map[string]any))
				inner[loc.innerKey] = filtered
				result[loc.outerKey] = inner
			}
			return result
		}
		return filterMap(typed, wanted)
	default:
		return normalised
	}
}

// dataListLocation describes where a data list was found in the
// object tree.
type dataListLocation struct {
	list     []any
	outerKey string // "" if top-level
	innerKey string // the key containing the array
}

// findDataList walks the object tree looking for the first array of
// objects under well-known keys. It searches both top-level and one
// level deep (e.g. result.value, response.items). The allow-list lives
// in preferredListKeys (formatter.go) and is shared with the table /
// csv renderers so all tabular formatters agree on what counts as the
// data list.
func findDataList(m map[string]any) *dataListLocation {
	// Top-level: {value: [...]}. Empty arrays under a preferred key still
	// match so an "empty list + metadata" payload renders as an empty table
	// (with the meta broadcast) rather than degrading to key/value rows.
	for _, key := range preferredListKeys {
		arr, ok := m[key].([]any)
		if !ok {
			continue
		}
		if len(arr) == 0 || isMapValue(arr[0]) {
			return &dataListLocation{list: arr, innerKey: key}
		}
	}

	// One level deep: {result: {value: [...]}}
	for _, outerKey := range []string{"result", "response", "data"} {
		inner, ok := m[outerKey].(map[string]any)
		if !ok {
			continue
		}
		for _, key := range preferredListKeys {
			arr, ok := inner[key].([]any)
			if !ok {
				continue
			}
			if len(arr) == 0 || isMapValue(arr[0]) {
				return &dataListLocation{list: arr, outerKey: outerKey, innerKey: key}
			}
		}
	}

	return nil
}

func isMapValue(v any) bool {
	_, ok := v.(map[string]any)
	return ok
}

// filterSlice applies field filtering to each object element in a
// slice. Non-object elements are passed through unchanged.
func filterSlice(items []any, wanted map[string]bool) []any {
	result := make([]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			result = append(result, filterMap(m, wanted))
		} else {
			result = append(result, item)
		}
	}
	return result
}

// ApplyJQ applies a jq expression to a JSON-serialisable payload and
// writes the results to w. Each result value is written as a separate
// line of JSON.
//
// The expression is compiled once and evaluated against the
// normalised payload. Multiple result values (e.g. from `.[]`) are
// each written as indented JSON followed by a newline.
func ApplyJQ(w io.Writer, payload any, expr string) error {
	query, err := gojq.Parse(expr)
	if err != nil {
		return apperrors.NewValidation(fmt.Sprintf("invalid --jq expression: %v", err))
	}

	normalised := toGeneric(payload)
	iter := query.Run(normalised)

	first := true
	for {
		value, ok := iter.Next()
		if !ok {
			break
		}
		if err, isErr := value.(error); isErr {
			return apperrors.NewValidation(fmt.Sprintf("--jq evaluation error: %v", err))
		}
		if !first {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		first = false

		data, marshalErr := json.MarshalIndent(value, "", "  ")
		if marshalErr != nil {
			return apperrors.NewInternal("failed to encode --jq result")
		}
		if _, err := fmt.Fprint(w, string(data)); err != nil {
			return err
		}
	}
	if !first {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	return nil
}

// shallowCopyMap returns a shallow copy of the map.
func shallowCopyMap(m map[string]any) map[string]any {
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

// filterMap returns a new map containing only keys that match the
// wanted set (case-insensitive).
func filterMap(m map[string]any, wanted map[string]bool) map[string]any {
	result := make(map[string]any, len(wanted))
	for key, value := range m {
		if wanted[strings.ToLower(key)] {
			result[key] = value
		}
	}
	return result
}

// toGeneric converts an arbitrary Go value into a generic JSON
// structure (map[string]any / []any / primitives) by round-tripping
// through JSON marshal/unmarshal. This ensures consistent types
// regardless of the original Go struct. The round-trip is always
// performed because even a map[string]any may contain typed values
// (e.g. []ir.CanonicalProduct) that gojq cannot handle.
func toGeneric(payload any) any {
	if payload == nil {
		return nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return payload
	}
	var generic any
	if err := json.Unmarshal(data, &generic); err != nil {
		return payload
	}
	return generic
}
