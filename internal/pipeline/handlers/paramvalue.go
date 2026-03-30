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

package handlers

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
)

// ParamValueHandler normalises parameter values after Cobra parsing.
// It operates on the structured Params map using the tool's input
// schema to determine value types and apply format-specific
// corrections.
//
// Supported normalisations:
//   - Boolean:  "yes"/"no"/"1"/"0"/"on"/"off"/"True"/"FALSE" → true/false
//   - Number:   "1,000" / "1_000" → 1000 (strip grouping separators)
//   - Date:     multiple input formats → RFC 3339 / ISO 8601
//   - Enum:     case-insensitive matching against allowed values
type ParamValueHandler struct{}

func (ParamValueHandler) Name() string          { return "paramvalue" }
func (ParamValueHandler) Phase() pipeline.Phase { return pipeline.PostParse }

func (ParamValueHandler) Handle(ctx *pipeline.Context) error {
	if len(ctx.Params) == 0 || len(ctx.Schema) == 0 {
		return nil
	}

	properties, ok := schemaProperties(ctx.Schema)
	if !ok {
		return nil
	}

	for key, value := range ctx.Params {
		propSchema, ok := properties[key].(map[string]any)
		if !ok {
			continue
		}

		corrected, changed := normaliseValue(value, propSchema)
		if changed {
			original := fmt.Sprintf("%v", value)
			ctx.Params[key] = corrected
			ctx.AddCorrection("paramvalue", pipeline.PostParse, key, original, fmt.Sprintf("%v", corrected), classifyCorrection(propSchema))
		}
	}
	return nil
}

// normaliseValue dispatches to the appropriate normaliser based on
// the schema type and format.
func normaliseValue(value any, schema map[string]any) (any, bool) {
	// Enum normalisation takes priority — it is type-independent.
	if enumValues, ok := schemaEnum(schema); ok {
		return normaliseEnum(value, enumValues)
	}

	schemaType, _ := schema["type"].(string)
	schemaFormat, _ := schema["format"].(string)

	switch schemaType {
	case "boolean":
		return normaliseBoolean(value)
	case "integer":
		return normaliseInteger(value)
	case "number":
		return normaliseNumber(value)
	case "string":
		if isDateFormat(schemaFormat) {
			return normaliseDate(value, schemaFormat)
		}
	}
	return value, false
}

// --- boolean normalisation ---

var booleanTrueValues = map[string]bool{
	"true": true, "yes": true, "on": true, "1": true,
}
var booleanFalseValues = map[string]bool{
	"false": true, "no": true, "off": true, "0": true,
}

func normaliseBoolean(value any) (any, bool) {
	// Already a bool — no change needed.
	if _, ok := value.(bool); ok {
		return value, false
	}

	str, ok := value.(string)
	if !ok {
		return value, false
	}
	lower := strings.ToLower(strings.TrimSpace(str))
	if booleanTrueValues[lower] {
		return true, true
	}
	if booleanFalseValues[lower] {
		return false, true
	}
	return value, false
}

// --- integer normalisation ---

func normaliseInteger(value any) (any, bool) {
	str, ok := value.(string)
	if !ok {
		return value, false
	}
	cleaned := stripNumberGrouping(str)
	if cleaned == str {
		return value, false
	}
	parsed, err := strconv.ParseInt(cleaned, 10, 64)
	if err != nil {
		return value, false
	}
	return parsed, true
}

// --- number normalisation ---

func normaliseNumber(value any) (any, bool) {
	str, ok := value.(string)
	if !ok {
		return value, false
	}
	cleaned := stripNumberGrouping(str)
	if cleaned == str {
		return value, false
	}
	parsed, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return value, false
	}
	return parsed, true
}

// stripNumberGrouping removes comma and underscore grouping
// separators from a numeric string.
func stripNumberGrouping(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "_", "")
	return s
}

// --- date normalisation ---

// Supported input date formats, tried in order.
var dateFormats = []string{
	time.RFC3339,          // 2006-01-02T15:04:05Z07:00
	"2006-01-02T15:04:05", // ISO without timezone
	"2006-01-02 15:04:05", // space-separated datetime
	"2006-01-02",          // date only
	"2006/01/02",          // slash-separated
	"02-01-2006",          // DD-MM-YYYY
	"01/02/2006",          // US format MM/DD/YYYY
	"Jan 2, 2006",         // English short month
	"January 2, 2006",     // English full month
	"2 Jan 2006",          // Day-first English
	"20060102",            // Compact YYYYMMDD
	"20060102T150405",     // Compact with time
	time.RFC1123,          // Mon, 02 Jan 2006 15:04:05 MST
	time.RFC1123Z,         // Mon, 02 Jan 2006 15:04:05 -0700
}

func isDateFormat(format string) bool {
	switch format {
	case "date", "date-time", "datetime":
		return true
	}
	return false
}

func normaliseDate(value any, schemaFormat string) (any, bool) {
	str, ok := value.(string)
	if !ok {
		return value, false
	}
	str = strings.TrimSpace(str)
	if str == "" {
		return value, false
	}

	// Try parsing as millisecond timestamp.
	if ms, err := strconv.ParseInt(str, 10, 64); err == nil && len(str) >= 10 {
		t := time.UnixMilli(ms).UTC()
		formatted := formatDate(t, schemaFormat)
		if formatted != str {
			return formatted, true
		}
		return value, false
	}

	// Try parsing as second timestamp.
	if sec, err := strconv.ParseInt(str, 10, 64); err == nil && len(str) >= 10 {
		t := time.Unix(sec, 0).UTC()
		formatted := formatDate(t, schemaFormat)
		if formatted != str {
			return formatted, true
		}
		return value, false
	}

	// Try known date formats.
	for _, layout := range dateFormats {
		t, err := time.Parse(layout, str)
		if err == nil {
			formatted := formatDate(t, schemaFormat)
			if formatted != str {
				return formatted, true
			}
			return value, false
		}
	}

	return value, false
}

// formatDate produces the output format based on the schema format
// field. "date" produces YYYY-MM-DD, everything else produces
// RFC 3339.
func formatDate(t time.Time, schemaFormat string) string {
	if schemaFormat == "date" {
		return t.Format("2006-01-02")
	}
	return t.Format(time.RFC3339)
}

// --- enum normalisation ---

func schemaEnum(schema map[string]any) ([]any, bool) {
	raw, ok := schema["enum"].([]any)
	if !ok || len(raw) == 0 {
		return nil, false
	}
	return raw, true
}

// normaliseEnum performs case-insensitive matching of a value against
// the allowed enum values. If exactly one enum entry matches
// (case-insensitive), the value is rewritten to match the canonical
// case from the schema.
func normaliseEnum(value any, allowed []any) (any, bool) {
	str, ok := value.(string)
	if !ok {
		return value, false
	}
	lower := strings.ToLower(strings.TrimSpace(str))

	var match any
	matches := 0
	for _, candidate := range allowed {
		candidateStr, ok := candidate.(string)
		if !ok {
			continue
		}
		if strings.ToLower(candidateStr) == lower {
			match = candidate
			matches++
		}
	}

	if matches == 1 && match != value {
		return match, true
	}
	return value, false
}

// --- helpers ---

func schemaProperties(schema map[string]any) (map[string]any, bool) {
	properties, ok := schema["properties"].(map[string]any)
	return properties, ok
}

func classifyCorrection(schema map[string]any) string {
	if _, ok := schemaEnum(schema); ok {
		return "enum"
	}
	schemaType, _ := schema["type"].(string)
	schemaFormat, _ := schema["format"].(string)
	if schemaType == "boolean" {
		return "boolean"
	}
	if schemaType == "integer" || schemaType == "number" {
		return "number"
	}
	if isDateFormat(schemaFormat) {
		return "date"
	}
	return "value"
}
