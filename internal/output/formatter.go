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
	"sort"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type Format string

const (
	FormatJSON   Format = "json"
	FormatTable  Format = "table"
	FormatRaw    Format = "raw"
	FormatPretty Format = "pretty"
	// FormatNDJSON emits one JSON object per line — friendly for streaming /
	// piping list results into downstream tools. See ndjson.go.
	FormatNDJSON Format = "ndjson"
	// FormatCSV emits RFC-4180 comma-separated values for list-shaped results —
	// friendly for spreadsheets and non-technical consumers. See csv.go.
	FormatCSV Format = "csv"
)

// preferredListKeys is the shared allow-list of keys whose array values are
// treated as the "data list" by all tabular formatters (-f table / csv /
// ndjson). It is the single source of truth — findDataList in filter.go
// reuses it. When adding a new key, prefer real envelope keys observed in
// production responses over speculative future names.
var preferredListKeys = []string{
	// Generic well-known list keys.
	"value", "items", "results", "data", "list", "records",
	"tools", "servers", "products",
	// Envelope keys observed in real DingTalk responses.
	"result", "documents", "emailAccounts", "todoCards", "events", "messages",
}

func ResolveFormat(cmd *cobra.Command, fallback Format) Format {
	if cmd == nil {
		return fallback
	}
	for _, flags := range []*pflag.FlagSet{cmd.Flags(), cmd.InheritedFlags()} {
		if format, ok := formatFromFlagSet(flags, fallback); ok {
			return format
		}
	}
	if root := cmd.Root(); root != nil {
		if format, ok := formatFromFlagSet(root.PersistentFlags(), fallback); ok {
			return format
		}
	}
	return fallback
}

func WriteCommandPayload(cmd *cobra.Command, payload any, fallback Format) error {
	if cmd == nil {
		return Write(io.Discard, fallback, payload)
	}
	return WriteFiltered(
		cmd.OutOrStdout(),
		ResolveFormat(cmd, fallback),
		payload,
		ResolveFields(cmd),
		ResolveJQ(cmd),
	)
}

func Write(w io.Writer, format Format, payload any) error {
	payload = unwrapCompatRuntimePayload(payload)
	switch format {
	case FormatJSON:
		return WriteJSON(w, payload)
	case FormatRaw:
		return writeRaw(w, payload)
	case FormatTable:
		return writeTableish(w, payload)
	case FormatPretty:
		return writePretty(w, payload)
	case FormatNDJSON:
		return writeNDJSON(w, payload)
	case FormatCSV:
		return writeCSV(w, payload)
	default:
		return WriteJSON(w, payload)
	}
}

func unwrapCompatRuntimePayload(payload any) any {
	result, ok := payload.(executor.Result)
	if !ok {
		return payload
	}
	if !result.Invocation.Implemented {
		return payload
	}
	switch result.Invocation.Kind {
	case "compat_invocation", "helper_invocation":
		content, ok := result.Response["content"]
		if ok {
			return content
		}
	}
	return payload
}

func formatFromFlagSet(flags *pflag.FlagSet, fallback Format) (Format, bool) {
	if flags == nil {
		return fallback, false
	}
	flag := flags.Lookup("format")
	if flag == nil {
		return fallback, false
	}
	value, err := flags.GetString("format")
	if err != nil {
		return fallback, false
	}
	return normalizeFormat(value, fallback), true
}

func normalizeFormat(raw string, fallback Format) Format {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(fallback):
		return fallback
	case string(FormatJSON):
		return FormatJSON
	case string(FormatRaw):
		return FormatRaw
	case string(FormatTable):
		return FormatTable
	case string(FormatPretty):
		return FormatPretty
	case string(FormatNDJSON):
		return FormatNDJSON
	case string(FormatCSV):
		return FormatCSV
	default:
		return fallback
	}
}

// WriteFiltered applies field selection and/or jq filtering before
// writing the payload. If jq is non-empty, the jq result is written
// directly (bypassing format). If fields is non-empty, the payload
// is filtered to those fields before normal output.
func WriteFiltered(w io.Writer, format Format, payload any, fields, jq string) error {
	payload = unwrapCompatRuntimePayload(payload)

	if strings.TrimSpace(jq) != "" {
		return ApplyJQ(w, payload, strings.TrimSpace(jq))
	}

	if strings.TrimSpace(fields) != "" {
		fieldList := strings.Split(fields, ",")
		payload = SelectFields(payload, fieldList)
	}

	return Write(w, format, payload)
}

// ResolveFields extracts the --fields flag value from the command.
// It ensures that we do not mistakenly grab a business parameter also named "fields"
// by matching the flag's usage string against the global root definition.
func ResolveFields(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	rootFlags := rootPersistentFlags(cmd)
	if rootFlags == nil {
		return ""
	}
	globalFlag := rootFlags.Lookup("fields")
	if globalFlag == nil {
		return ""
	}

	for _, flags := range []*pflag.FlagSet{
		cmd.Flags(),
		cmd.InheritedFlags(),
		rootFlags,
	} {
		if flags == nil {
			continue
		}
		if f := flags.Lookup("fields"); f != nil && f.Changed {
			// To avoid collision with business flags (e.g. table create --fields),
			// verify this flag shares the same usage string as the global one.
			if f.Usage == globalFlag.Usage {
				if v, err := flags.GetString("fields"); err == nil {
					return v
				}
			}
		}
	}
	return ""
}

// ResolveJQ extracts the --jq flag value from the command. It ensures
// that we only grab the global output filter, not a similarly named business parameter.
func ResolveJQ(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	rootFlags := rootPersistentFlags(cmd)
	if rootFlags == nil {
		return ""
	}
	globalFlag := rootFlags.Lookup("jq")
	if globalFlag == nil {
		return ""
	}

	for _, flags := range []*pflag.FlagSet{
		cmd.Flags(),
		cmd.InheritedFlags(),
		rootFlags,
	} {
		if flags == nil {
			continue
		}
		if f := flags.Lookup("jq"); f != nil && f.Changed {
			if f.Usage == globalFlag.Usage {
				if v, err := flags.GetString("jq"); err == nil {
					return v
				}
			}
		}
	}
	return ""
}

func rootPersistentFlags(cmd *cobra.Command) *pflag.FlagSet {
	if root := cmd.Root(); root != nil {
		return root.PersistentFlags()
	}
	return nil
}

// WriteJSON marshals payload as indented JSON and writes it to w.
func WriteJSON(w io.Writer, payload any) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return apperrors.NewInternal("failed to encode command output as JSON")
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

func writeRaw(w io.Writer, payload any) error {
	if text, ok := payload.(string); ok {
		_, err := fmt.Fprintln(w, SanitizeForTerminal(text))
		return err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return apperrors.NewInternal("failed to encode raw command output")
	}
	_, err = fmt.Fprintln(w, SanitizeForTerminal(string(data)))
	return err
}

func writeTableish(w io.Writer, payload any) error {
	normalized, err := normalizePayload(payload)
	if err != nil {
		return err
	}

	switch typed := normalized.(type) {
	case map[string]any:
		// Try table extraction first so wrappers around list payloads
		// (e.g. {result: {todoCards: [...]}}) render as a table instead
		// of being peeled by unwrapPrimaryObject and degraded to key/
		// value rows. unwrapPrimaryObject remains the fallback for
		// single-object wrappers like {invocation: {kind, params, ...}}.
		if headers, rows, meta, ok := extractRowsFromMap(typed); ok {
			if err := writeTable(w, headers, rows); err != nil {
				return err
			}
			if len(meta) > 0 {
				if _, err := fmt.Fprintln(w); err != nil {
					return err
				}
				return writeKeyValues(w, meta)
			}
			return nil
		}
		if inner, ok := unwrapPrimaryObject(typed); ok {
			return writeKeyValues(w, inner)
		}
		return writeKeyValues(w, typed)
	case []any:
		if headers, rows, ok := rowsFromSlice(typed); ok {
			return writeTable(w, headers, rows)
		}
		return writeRaw(w, normalized)
	default:
		return writeRaw(w, normalized)
	}
}

func normalizePayload(payload any) (any, error) {
	if payload == nil {
		return nil, nil
	}
	if text, ok := payload.(string); ok {
		return text, nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, apperrors.NewInternal("failed to normalize command output")
	}
	var normalized any
	if err := json.Unmarshal(data, &normalized); err != nil {
		return nil, apperrors.NewInternal("failed to decode normalized command output")
	}
	return normalized, nil
}

func unwrapPrimaryObject(payload map[string]any) (map[string]any, bool) {
	if len(payload) != 1 {
		return nil, false
	}
	for _, key := range []string{"invocation", "response", "result", "data"} {
		value, ok := payload[key]
		if !ok {
			continue
		}
		object, ok := value.(map[string]any)
		if ok {
			return object, true
		}
	}
	return nil, false
}

// extractRowsFromMap finds the data list inside a wrapper map and returns it
// as (headers, rows, meta). It delegates the search to findDataList so the
// detection rules stay aligned with -f ndjson: top-level under a preferred
// key, or one level deep under {result|response|data}. Meta is built from
// every sibling of the list — at both the outer and inner level when the
// list sits one level deep — so callers like the table renderer's footer and
// the csv broadcastMeta path see the same key set.
func extractRowsFromMap(payload map[string]any) ([]string, [][]string, map[string]any, bool) {
	loc := findDataList(payload)
	if loc == nil {
		return nil, nil, nil, false
	}
	headers, rows, ok := rowsFromSlice(loc.list)
	if !ok {
		return nil, nil, nil, false
	}
	meta := make(map[string]any)
	if loc.outerKey == "" {
		for k, v := range payload {
			if k == loc.innerKey {
				continue
			}
			meta[k] = v
		}
	} else {
		for k, v := range payload {
			if k == loc.outerKey {
				continue
			}
			meta[k] = v
		}
		if inner, ok := payload[loc.outerKey].(map[string]any); ok {
			for k, v := range inner {
				if k == loc.innerKey {
					continue
				}
				if _, exists := meta[k]; exists {
					// Outer wins on key collision so users see the wrapper-level
					// sibling rather than a clobbered inner one.
					continue
				}
				meta[k] = v
			}
		}
	}
	return headers, rows, meta, true
}

func rowsFromSlice(items []any) ([]string, [][]string, bool) {
	if len(items) == 0 {
		return []string{"value"}, [][]string{}, true
	}

	allMaps := true
	keys := make(map[string]struct{})
	for _, item := range items {
		rowMap, ok := item.(map[string]any)
		if !ok {
			allMaps = false
			break
		}
		for key := range rowMap {
			keys[key] = struct{}{}
		}
	}
	if allMaps {
		headers := sortedKeys(keys)
		rows := make([][]string, 0, len(items))
		for _, item := range items {
			rowMap := item.(map[string]any)
			row := make([]string, 0, len(headers))
			for _, key := range headers {
				row = append(row, formatValue(rowMap[key]))
			}
			rows = append(rows, row)
		}
		return headers, rows, true
	}

	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{formatValue(item)})
	}
	return []string{"value"}, rows, true
}

func writeKeyValues(w io.Writer, payload map[string]any) error {
	keys := make([]string, 0, len(payload))
	maxWidth := 0
	for key := range payload {
		keys = append(keys, key)
		if width := runeWidth(key); width > maxWidth {
			maxWidth = width
		}
	}
	sort.Strings(keys)
	if maxWidth > 24 {
		maxWidth = 24
	}
	for _, key := range keys {
		if _, err := fmt.Fprintf(w, "%-*s  %s\n", maxWidth, key, formatValue(payload[key])); err != nil {
			return err
		}
	}
	return nil
}

func writeTable(w io.Writer, headers []string, rows [][]string) error {
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = runeWidth(header)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i >= len(widths) {
				continue
			}
			if width := runeWidth(cell); width > widths[i] {
				widths[i] = width
			}
		}
	}
	for i := range widths {
		if widths[i] > 60 {
			widths[i] = 60
		}
	}

	for i, header := range headers {
		if i > 0 {
			if _, err := io.WriteString(w, "  "); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "%-*s", widths[i], truncate(header, widths[i])); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	for i, width := range widths {
		if i > 0 {
			if _, err := io.WriteString(w, "  "); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(w, strings.Repeat("-", width)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	for _, row := range rows {
		for i, cell := range row {
			if i >= len(widths) {
				continue
			}
			if i > 0 {
				if _, err := io.WriteString(w, "  "); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintf(w, "%-*s", widths[i], truncate(cell, widths[i])); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	return nil
}

func sortedKeys(keys map[string]struct{}) []string {
	out := make([]string, 0, len(keys))
	for key := range keys {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func formatValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return SanitizeForTerminal(typed)
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return SanitizeForTerminal(fmt.Sprintf("%v", typed))
		}
		return SanitizeForTerminal(string(data))
	}
}

func truncate(s string, maxWidth int) string {
	if runeWidth(s) <= maxWidth {
		return s
	}
	runes := []rune(s)
	if len(runes) > maxWidth-1 {
		return string(runes[:maxWidth-1]) + "…"
	}
	return s
}

func runeWidth(s string) int {
	width := 0
	for _, r := range s {
		if r >= 0x2E80 && r <= 0x9FFF {
			width += 2
			continue
		}
		width++
	}
	return width
}
