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

package helpers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers/docjsonml"
	jsonrepair "github.com/RealAlexandreAI/json-repair"
	"github.com/spf13/cobra"
)

var docDangerousUnicode = [...]rune{
	0x200B,
	0x200C,
	0x200D,
	0x200E,
	0x200F,
	0x202A,
	0x202B,
	0x202C,
	0x202D,
	0x202E,
	0x2028,
	0x2029,
	0x2066,
	0x2067,
	0x2068,
	0x2069,
	0xFEFF,
	0x00AD,
}

var docDangerousSet = func() map[rune]bool {
	m := make(map[rune]bool, len(docDangerousUnicode))
	for _, r := range docDangerousUnicode {
		m[r] = true
	}
	return m
}()

// stripDocInputUnsafe removes characters that the server-side RejectControlChars
// validator (mirrored by apiclient.rejectDangerousChars) would reject:
//
//  1. C0 control characters (except tab and newline) and DEL (0x7F)
//  2. Dangerous Unicode (zero-width, Bidi controls, line/paragraph separators, BOM)
//
// It is applied at the write boundary so document content passes server
// validation instead of being rejected. Tab and newline are preserved because
// they are legitimate in document text.
func stripDocInputUnsafe(s string) string {
	return strings.Map(func(r rune) rune {
		if r != '\t' && r != '\n' && (r < 0x20 || r == 0x7F) {
			return -1
		}
		if docDangerousSet[r] {
			return -1
		}
		return r
	}, s)
}

type docJSONMLFixMode int

const (
	docJSONMLFixDefault docJSONMLFixMode = iota
	docJSONMLFixFull
	docJSONMLFixNone
)

func docResolveFixMode(cmd *cobra.Command) docJSONMLFixMode {
	noFix, _ := cmd.Flags().GetBool("no-fix-jsonml")
	fix, _ := cmd.Flags().GetBool("fix-jsonml")
	if noFix && fix {
		fmt.Fprintln(cmd.ErrOrStderr(), "[WARN] --fix-jsonml 和 --no-fix-jsonml 同时传入，以 --no-fix-jsonml 为准（全部修复关闭）")
		return docJSONMLFixNone
	}
	if noFix {
		return docJSONMLFixNone
	}
	if fix {
		return docJSONMLFixFull
	}
	return docJSONMLFixDefault
}

func docCoerceJSONMLBodyShape(raw string) (string, []string, error) {
	if raw == "" {
		return raw, nil, nil
	}
	var probe any
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return raw, nil, nil
	}
	if _, ok := probe.([]any); ok {
		wrapped, err := json.Marshal(map[string]any{"jsonml": probe})
		if err != nil {
			return raw, nil, fmt.Errorf("wrap bare jsonml array: %w", err)
		}
		return string(wrapped), nil, nil
	}
	return raw, nil, nil
}

func docCoerceJSONMLNodeShape(raw string) (string, []string, error) {
	if raw == "" {
		return raw, nil, nil
	}
	var probe any
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return raw, nil, nil
	}
	if _, ok := probe.([]any); ok {
		return raw, nil, nil
	}
	wrapper, ok := probe.(map[string]any)
	if !ok {
		return raw, nil, nil
	}
	inner, hasKey := wrapper["jsonml"]
	if !hasKey {
		return raw, nil, nil
	}
	arr, ok := inner.([]any)
	if !ok {
		return raw, nil, nil
	}
	switch len(arr) {
	case 0:
		return "", nil, fmt.Errorf(`--content-format jsonml 输入 {"jsonml":[]}: wrapper 中 jsonml 数组为空`)
	case 1:
		out, err := json.Marshal(arr[0])
		if err != nil {
			return raw, nil, fmt.Errorf("unwrap single jsonml node: %w", err)
		}
		return string(out), []string{`输入为 {"jsonml":[node]} body 形态，已自动解包为单节点以符合 block 命令协议`}, nil
	default:
		return "", nil, fmt.Errorf(`block insert/update 一次只能处理一个 JSONML 节点，输入 {"jsonml":[...]} 包含 %d 个节点。请分多次调用，或使用 doc update --content-format jsonml 整篇覆盖`, len(arr))
	}
}

func prepareDocJSONMLBody(cmd *cobra.Command, raw string) (string, error) {
	mode := docResolveFixMode(cmd)

	coerced, coerceNotes, err := docCoerceJSONMLBodyShape(raw)
	if err != nil {
		return "", err
	}
	docEmitJSONMLFixNotes(cmd, coerceNotes)
	raw = coerced

	var wrapper map[string]any
	if err := json.Unmarshal([]byte(raw), &wrapper); err != nil {
		if mode == docJSONMLFixFull {
			repaired, repairErr := jsonrepair.RepairJSON(raw)
			if repairErr != nil {
				return "", fmt.Errorf("JSON 语法错误且自动修复失败: %w\n原始错误: %v", repairErr, err)
			}
			fmt.Fprintln(cmd.ErrOrStderr(), "[FIX] JSON 语法已自动修复（括号/逗号等结构性错误）")
			if err2 := json.Unmarshal([]byte(repaired), &wrapper); err2 != nil {
				return "", fmt.Errorf("JSON 修复后仍无法解析: %w", err2)
			}
		} else {
			return "", fmt.Errorf("JSON 语法错误: %w\n输入不是有效的 JSON（可能缺少括号或逗号）。如果输入来自 LLM 生成，可通过 --fix-jsonml 尝试自动修复", err)
		}
	}

	bodyAny, ok := wrapper["jsonml"]
	if !ok {
		return "", fmt.Errorf(`--content-format jsonml 输入 JSON 必须包含 "jsonml" 字段，格式: {"jsonml": [...]}`)
	}
	bodyArr, ok := bodyAny.([]any)
	if !ok {
		return "", fmt.Errorf(`--content-format jsonml 字段 "jsonml" 必须是数组`)
	}

	if mode != docJSONMLFixNone {
		fixed, notes := docjsonml.NormalizeJsonMLBody(bodyArr)
		bodyArr = fixed
		docEmitJSONMLFixNotes(cmd, notes)

		wrapped, wrapNotes := docjsonml.EnsureRootWrappedBody(bodyArr)
		bodyArr = wrapped
		docEmitJSONMLFixNotes(cmd, wrapNotes)
	}

	vr := docjsonml.ValidateJsonMLBodyV2(bodyArr)
	if vr.HasErrors() {
		return "", fmt.Errorf("JSONML 格式校验失败:\n%s\n请确认输入格式是否正确，或通过 --no-fix-jsonml 关闭自动修复以排查原始错误", vr.Summary())
	}
	if summary := vr.Summary(); summary != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "[WARN] "+summary)
	}

	out, err := json.Marshal(bodyArr)
	if err != nil {
		return "", fmt.Errorf("marshal normalized jsonml: %w", err)
	}
	return stripDocInputUnsafe(string(out)), nil
}

func prepareDocJSONMLNode(cmd *cobra.Command, rawElement string) (string, error) {
	if rawElement == "" {
		return "", fmt.Errorf("--content-format jsonml 要求通过 --element 提供 JSONML 数组")
	}
	mode := docResolveFixMode(cmd)

	coerced, coerceNotes, err := docCoerceJSONMLNodeShape(rawElement)
	if err != nil {
		return "", err
	}
	docEmitJSONMLFixNotes(cmd, coerceNotes)
	rawElement = coerced

	var node any
	if err := json.Unmarshal([]byte(rawElement), &node); err != nil {
		if mode == docJSONMLFixFull {
			repaired, repairErr := jsonrepair.RepairJSON(rawElement)
			if repairErr != nil {
				return "", fmt.Errorf("JSON 语法错误且自动修复失败: %w\n原始错误: %v", repairErr, err)
			}
			fmt.Fprintln(cmd.ErrOrStderr(), "[FIX] JSON 语法已自动修复（括号/逗号等结构性错误）")
			if err2 := json.Unmarshal([]byte(repaired), &node); err2 != nil {
				return "", fmt.Errorf("JSON 修复后仍无法解析: %w", err2)
			}
		} else {
			return "", fmt.Errorf("JSON 语法错误: %w\n输入不是有效的 JSON（可能缺少括号或逗号）。如果输入来自 LLM 生成，可通过 --fix-jsonml 尝试自动修复", err)
		}
	}
	if _, ok := node.([]any); !ok {
		return "", fmt.Errorf("--content-format jsonml 要求 --element 为 JSON 数组，实际类型: %T", node)
	}

	if mode != docJSONMLFixNone {
		fixed, notes := docjsonml.NormalizeJsonMLNode(node)
		node = fixed
		docEmitJSONMLFixNotes(cmd, notes)
	}

	vr := docjsonml.ValidateJsonMLNodeV2(node)
	if vr.HasErrors() {
		return "", fmt.Errorf("JSONML 格式校验失败:\n%s\n请确认输入格式是否正确，或通过 --no-fix-jsonml 关闭自动修复以排查原始错误", vr.Summary())
	}
	if summary := vr.Summary(); summary != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "[WARN] "+summary)
	}

	out, err := json.Marshal(node)
	if err != nil {
		return "", fmt.Errorf("marshal normalized jsonml: %w", err)
	}
	return stripDocInputUnsafe(string(out)), nil
}

func docEmitJSONMLFixNotes(cmd *cobra.Command, notes []string) {
	if len(notes) == 0 {
		return
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "[FIX] JSONML 自动修复（%d 项）:\n", len(notes))
	for i, n := range notes {
		fmt.Fprintf(cmd.ErrOrStderr(), "  %d. %s\n", i+1, n)
	}
}

func sniffJsonMLLike(content string) bool {
	const lookahead = 64
	s := strings.TrimLeft(content, " \t\r\n")
	if s == "" {
		return false
	}
	scan := s
	if len(scan) > lookahead {
		scan = scan[:lookahead]
	}
	switch s[0] {
	case '[':
		rest := strings.TrimLeft(scan[1:], " \t\r\n")
		return strings.HasPrefix(rest, `"`) || strings.HasPrefix(rest, `[`)
	case '{':
		rest := strings.TrimLeft(scan[1:], " \t\r\n")
		return strings.HasPrefix(rest, `"jsonml"`)
	}
	return false
}
