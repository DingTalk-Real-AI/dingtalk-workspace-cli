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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/jsonutil"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/tui"
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

var (
	marshalJSON       = json.Marshal
	unmarshalJSON     = json.Unmarshal
	marshalJSONOutput = jsonutil.Marshal
	marshalJSONIndent = jsonutil.MarshalIndent
)

func ResolveFormat(cmd *cobra.Command, fallback Format) Format {
	format, _ := resolveFormatWithWarning(cmd, fallback)
	return format
}

// resolveFormatWithWarning 解析命令的 --format flag（B36，AC-09）：已知值
// 归一化为对应 Format 常量；未知非空值**降级 fallback 并产出一条 warning
// 文本**（不崩不静默，契约规范 §5.2「未知值 → 降级 + stderr warning」）。
// warning 的写出由调用方负责（信封出口 WriteEnvelope 写 cmd.ErrOrStderr()），
// 本函数不做 I/O——ResolveFormat 的既有调用方（只读 format 值、不持有出口
// 语义）行为保持不变。flag 缺席或值为空返回 fallback 且不产生 warning
// （空值是「未指定」而非「未知」）。查找顺序与 ResolveFormat 一致：
// cmd.Flags() 优先于 InheritedFlags()，命中 format flag 即返回。
func resolveFormatWithWarning(cmd *cobra.Command, fallback Format) (Format, string) {
	if cmd == nil {
		return fallback, ""
	}
	for _, flags := range []*pflag.FlagSet{cmd.Flags(), cmd.InheritedFlags()} {
		value, ok := formatValueFromFlagSet(flags)
		if !ok {
			continue
		}
		format := normalizeFormat(value, fallback)
		return format, unknownFormatWarning(value, format)
	}
	return fallback, ""
}

// formatValueFromFlagSet 返回 flagSet 上 --format flag 的原始字符串值
// （B35/B36/B43 共用的查找辅助）。flag 缺席或类型非 string 时返回 ok=false——
// 与 formatFromFlagSet 的容错口径一致（不报错、交由 fallback 兜底）。
func formatValueFromFlagSet(flags *pflag.FlagSet) (string, bool) {
	if flags == nil || flags.Lookup("format") == nil {
		return "", false
	}
	value, err := flags.GetString("format")
	if err != nil {
		return "", false
	}
	return value, true
}

// unknownFormatWarning 在 raw 为非空未知值（归一化后不等于 resolved，即不是
// 任何已知 format 的大小写/空白变体）时返回 warning 文本，否则返回空串
// （B36，AC-09 不崩不静默）。文本只陈述事实（未知值 + 降级目标），
// 不做过度承诺。
func unknownFormatWarning(raw string, resolved Format) string {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" || trimmed == string(resolved) {
		return ""
	}
	return fmt.Sprintf("unknown --format %q, falling back to %q", raw, string(resolved))
}

// ResolveFormatWithJSONShorthand 解析命令的输出 format，实现 --json 简写语义
// （B43，契约规范 §5.2：「--json 自动注册为 --format json 简写（显式
// --format 优先）」）。优先级链：
//
//  1. 显式 --format（非空值）恒优先——含未知值（按 normalizeFormat 降级
//     fallback，--json 不 rescues 未知值）；
//  2. --format 缺席或为空时，--json 布尔 flag（Changed 且为 true）等价
//     --format json；
//  3. 两者皆无 → fallback。
//
// --json 判定只接受 bool 型 flag（GetBool 成功）：业务层存在同名 string
// flag（如 table create --json 载荷参数）的命令不会被误判为简写。本函数
// 纯判定不做 I/O，供不直接走 WriteEnvelope 的调用方（如主漏斗桥接，B44）
// 复用；与 resolveFormatWithWarning 共用 formatValueFromFlagSet /
// normalizeFormat，归一化规则单一事实源。
func ResolveFormatWithJSONShorthand(cmd *cobra.Command, fallback Format) Format {
	if cmd == nil {
		return fallback
	}
	for _, flags := range []*pflag.FlagSet{cmd.Flags(), cmd.InheritedFlags()} {
		value, ok := formatValueFromFlagSet(flags)
		if !ok {
			continue
		}
		if strings.TrimSpace(value) != "" {
			return normalizeFormat(value, fallback)
		}
	}
	if jsonShorthandActive(cmd) {
		return FormatJSON
	}
	return fallback
}

// jsonShorthandActive 报告 cmd 上是否有生效的 --json 简写（bool flag 被显式
// 设置为 true）。查找 Flags / InheritedFlags / PersistentFlags 三组，与
// internal/app 侧 commandRequestsJSONErrors 的判定面一致；GetBool 失败
// （非 bool 同名 flag）跳过该组继续。
func jsonShorthandActive(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	for _, flags := range []*pflag.FlagSet{cmd.Flags(), cmd.InheritedFlags(), cmd.PersistentFlags()} {
		if flags == nil {
			continue
		}
		flag := flags.Lookup("json")
		if flag == nil || !flag.Changed {
			continue
		}
		if value, err := flags.GetBool("json"); err == nil {
			return value
		}
	}
	return false
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
	value, ok := formatValueFromFlagSet(flags)
	if !ok {
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
	globalFlag := rootFlags.Lookup("fields")
	if globalFlag == nil {
		return ""
	}

	for _, flags := range []*pflag.FlagSet{
		cmd.Flags(),
		cmd.InheritedFlags(),
		rootFlags,
	} {
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
	globalFlag := rootFlags.Lookup("jq")
	if globalFlag == nil {
		return ""
	}

	for _, flags := range []*pflag.FlagSet{
		cmd.Flags(),
		cmd.InheritedFlags(),
		rootFlags,
	} {
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
	if cmd == nil {
		return nil
	}
	return cmd.Root().PersistentFlags()
}

// WriteJSON marshals payload as indented JSON and writes it to w.
func WriteJSON(w io.Writer, payload any) error {
	data, err := marshalJSONIndent(payload, "", "  ")
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
	data, err := marshalJSONOutput(payload)
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
		headers, rows := rowsFromSlice(typed)
		return writeTable(w, headers, rows)
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
	data, err := marshalJSON(payload)
	if err != nil {
		return nil, apperrors.NewInternal("failed to normalize command output")
	}
	var normalized any
	if err := unmarshalJSON(data, &normalized); err != nil {
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
	headers, rows := rowsFromSlice(loc.list)
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
		inner := payload[loc.outerKey].(map[string]any)
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
	return headers, rows, meta, true
}

func rowsFromSlice(items []any) ([]string, [][]string) {
	if len(items) == 0 {
		return []string{"value"}, [][]string{}
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
		return headers, rows
	}

	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{formatValue(item)})
	}
	return []string{"value"}, rows
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
		label := tui.PadRightANSI(tui.Key(key), maxWidth+1)
		if _, err := fmt.Fprintf(w, "%s %s\n", label, formatValue(payload[key])); err != nil {
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
		if widths[i] > tui.MaxTableColumnWidth {
			widths[i] = tui.MaxTableColumnWidth
		}
	}

	if len(widths) == 0 {
		return nil
	}

	writeRow := func(values []string, render func(string) string) error {
		if _, err := io.WriteString(w, tui.Gray("│ ")); err != nil {
			return err
		}
		for i := range widths {
			if i > 0 {
				if _, err := io.WriteString(w, tui.Gray(" │ ")); err != nil {
					return err
				}
			}
			cell := ""
			if i < len(values) {
				cell = values[i]
			}
			cell = truncate(cell, widths[i])
			if _, err := io.WriteString(w, tui.PadRightANSI(render(cell), widths[i])); err != nil {
				return err
			}
		}
		_, err := io.WriteString(w, tui.Gray(" │\n"))
		return err
	}
	writeDivider := func(left, mid, right string, render func(string) string) error {
		if _, err := io.WriteString(w, render(left)); err != nil {
			return err
		}
		for i, width := range widths {
			if i > 0 {
				if _, err := io.WriteString(w, render(mid)); err != nil {
					return err
				}
			}
			if _, err := io.WriteString(w, render(strings.Repeat("─", width+2))); err != nil {
				return err
			}
		}
		_, err := io.WriteString(w, render(right+"\n"))
		return err
	}

	if err := writeDivider("╭", "┬", "╮", tui.Blue); err != nil {
		return err
	}
	if err := writeRow(headers, tui.Brand); err != nil {
		return err
	}
	if err := writeDivider("├", "┼", "┤", tui.Gray); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writeRow(row, tui.White); err != nil {
			return err
		}
	}
	return writeDivider("╰", "┴", "╯", tui.Blue)
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
		data, err := marshalJSONOutput(typed)
		if err != nil {
			return SanitizeForTerminal(fmt.Sprintf("%v", typed))
		}
		return SanitizeForTerminal(string(data))
	}
}

func truncate(s string, maxWidth int) string {
	if maxWidth <= 0 || runeWidth(s) <= maxWidth {
		return s
	}
	if maxWidth == 1 {
		return "…"
	}

	var b strings.Builder
	used := 0
	limit := maxWidth - 1
	for _, r := range s {
		width := runeWidth(string(r))
		if used+width > limit {
			break
		}
		b.WriteRune(r)
		used += width
	}
	return b.String() + "…"
}

func runeWidth(s string) int {
	return tui.PlainRuneWidth(s)
}
