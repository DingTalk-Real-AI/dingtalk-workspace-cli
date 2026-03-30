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
	FormatJSON  Format = "json"
	FormatTable Format = "table"
	FormatRaw   Format = "raw"
)

var preferredListKeys = []string{"items", "results", "data", "list", "records", "tools", "servers", "products"}

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
func ResolveFields(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	for _, flags := range []*pflag.FlagSet{
		cmd.Flags(),
		cmd.InheritedFlags(),
		rootPersistentFlags(cmd),
	} {
		if flags == nil {
			continue
		}
		if f := flags.Lookup("fields"); f != nil && f.Changed {
			if v, err := flags.GetString("fields"); err == nil {
				return v
			}
		}
	}
	return ""
}

// ResolveJQ extracts the --jq flag value from the command. It checks
// local flags, inherited flags, and root persistent flags because
// --jq is registered as a root PersistentFlag.
func ResolveJQ(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	for _, flags := range []*pflag.FlagSet{
		cmd.Flags(),
		cmd.InheritedFlags(),
		rootPersistentFlags(cmd),
	} {
		if flags == nil {
			continue
		}
		if f := flags.Lookup("jq"); f != nil && f.Changed {
			if v, err := flags.GetString("jq"); err == nil {
				return v
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
		if inner, ok := unwrapPrimaryObject(typed); ok {
			return writeKeyValues(w, inner)
		}
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

func extractRowsFromMap(payload map[string]any) ([]string, [][]string, map[string]any, bool) {
	for _, key := range preferredListKeys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		list, ok := value.([]any)
		if !ok {
			continue
		}
		headers, rows, ok := rowsFromSlice(list)
		if !ok {
			continue
		}
		meta := make(map[string]any, len(payload)-1)
		for metaKey, metaValue := range payload {
			if metaKey == key {
				continue
			}
			meta[metaKey] = metaValue
		}
		return headers, rows, meta, true
	}
	return nil, nil, nil, false
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
