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

package smart

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/minutesdata"
)

// ReplaceBatch: apply MANY word replacements to one minute (听记) in a single
// command.
//
// dws's atomic tool replace_minutes_text handles exactly one
// originalText→replacedText pair (see internal/helpers/minutes.go, the
// +word-replace 1:1 shortcut). Fixing several terms at once means calling it
// repeatedly and eyeballing each result. This shortcut takes multiple --pair
// "原文=>替换" entries, validates them (rejecting duplicate source words so two
// rules don't fight over the same term), applies each via replace_minutes_text
// and aggregates a per-pair {applied|error} report through rt.Output. The
// default policy stops at the first error; explicit continue still returns
// non-zero when any write fails.
//
//	dws minutes +replace-batch --id <taskUuid> --pair "张三=>张三丰" --pair "Q2=>第二季度"
var ReplaceBatch = shortcut.Shortcut{
	Service:     "minutes",
	Command:     "+replace-batch",
	Product:     "minutes",
	Description: "预检并批量执行多组听记文字替换，逐项验证且失败必定非零",
	Intent: "当你要在同一条听记里一次纠正多个错识别人名或术语时使用；支持重复 --pair，或通过 --json 接受字面量、@相对文件和 stdin。" +
		"命令会先读取完整逐字稿并校验所有原文、空值和重复规则，再逐项写入并重新读取完整逐字稿验证效果。" +
		"默认首错停止；只有显式 failure-policy=continue 才继续，但任一失败仍返回非零及 applied/failed/unattempted ledger。底层不是事务且没有自动回滚，并会同时影响逐字稿和摘要。",
	Risk: shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "minutes",
			Name:           "shortcut_replace_batch",
			CanonicalPath:  "minutes.shortcut_replace_batch",
			CLIPath:        "minutes +replace-batch",
			PrimaryCLIPath: "minutes +replace-batch",
		},
		Description: "预检并批量执行多组听记文字替换，逐项验证且失败必定非零",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "对一条妙记（听记）批量执行多组文字替换（原文=>替换）",
			UseWhen:      []string{"当你要在同一条听记里一次性纠正多个词（如把多个错识别的人名/术语统一替换），而底层工具一次只能替换一组时使用；内部按多个 --pair \"原文=>替换\" 逐组调用替换工具，先在本地校验去重（同一个「原文」不能出现两次，避免两条规则互相打架），再逐组应用并聚合每组的成功/失败结果，某一组失败不会中断其余组。这是写操作，会实际修改听记文字内容，请确认 taskUuid 与替换规则无误。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples: []string{
				"dws minutes +replace-batch --id <taskUuid> --pair \"张三=>张三丰\"",
				"dws minutes +replace-batch --id <taskUuid> --pair \"Q2=>第二季度\" --pair \"PM=>产品经理\"",
			},
		},
		DryRun: &contract.DryRunSpec{
			PreviewKind: contract.DryRunPreviewPlan,
			RemoteReads: false,
		},
		Parameters: []contract.ParamDecl{
			{Name: "pair", Description: `替换规则，格式 "原文=>替换"，可重复传多组（必填）；每组原文不能为空且不能重复`},
			{Name: "json", Description: "JSON 字面量、@工作目录相对文件或 - 表示 stdin"},
			{Name: "failure-policy", Description: "stop=首错停止；continue=显式继续，但只要有失败仍返回非零"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "id", Type: shortcut.FlagString, Desc: "听记 taskUuid（必填）", Required: true},
		{Name: "pair", Type: shortcut.FlagStringSlice, Desc: `替换规则，格式 "原文=>替换"，可重复传多组`},
		{Name: "json", Type: shortcut.FlagString, Desc: "替换规则 JSON 字面量、@相对文件或 - 表示 stdin"},
		{Name: "failure-policy", Type: shortcut.FlagString, Default: "stop", Desc: "失败策略", Enum: []string{"stop", "continue"}},
		{Name: "page-limit", Type: shortcut.FlagInt, Default: "100", Desc: "逐字稿写前/写后验证的翻页上限"},
	},
	Constraints: []shortcut.Constraint{
		{
			Kind:        shortcut.ConstraintCustom,
			Flags:       []string{"pair", "json"},
			Description: `--pair 与 --json 至少提供一种；规则必须使用 "原文=>替换" 或 JSON 数组；原文不能为空且不能重复`,
		},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"page-limit"}, Description: "--page-limit 必须大于 0"},
	},
	Tips: []string{
		`dws minutes +replace-batch --id <taskUuid> --pair "张三=>张三丰"`,
		`dws minutes +replace-batch --id <taskUuid> --pair "Q2=>第二季度" --pair "PM=>产品经理"`,
	},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if rt.Int("page-limit") <= 0 {
			return apperrors.NewValidation("--page-limit 必须大于 0")
		}
		if len(rt.StrSlice("pair")) > 0 {
			_, err := parseReplacePairs(rt.StrSlice("pair"))
			return err
		}
		return nil
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		taskUUID := rt.Str("id")
		pairs, err := loadReplacePairs(rt)
		if err != nil {
			return err
		}
		if rt.DryRun() {
			replacements := make([]map[string]any, 0, len(pairs))
			for _, p := range pairs {
				replacements = append(replacements, map[string]any{
					"originalText": p.orig,
					"replacedText": p.repl,
				})
			}
			return rt.Output(map[string]any{
				"operation":     "minutes.replace_batch",
				"dry_run":       true,
				"dryRun":        true,
				"preview_kind":  contract.DryRunPreviewPlan,
				"executed":      false,
				"failurePolicy": rt.Str("failure-policy"),
				"taskUuid":      taskUUID,
				"total":         len(pairs),
				"replacements":  replacements,
			})
		}

		currentText, err := minutesReplaceTranscriptText(rt, taskUUID, rt.Int("page-limit"))
		if err != nil {
			return err
		}
		for _, pair := range pairs {
			if strings.Count(currentText, pair.orig) == 0 {
				return apperrors.NewValidation(fmt.Sprintf("完整逐字稿中不存在待替换原文 %q；未执行任何写入", pair.orig))
			}
		}

		results := make([]map[string]any, 0, len(pairs))
		applied := 0
		failed := 0
		unattempted := 0
		for index, p := range pairs {
			// Params mirror helpers replace_minutes_text call site: taskUuid /
			// originalText / replacedText.
			writeData, callErr := rt.CallMCPWriteDataStrict("minutes", "replace_minutes_text", map[string]any{
				"taskUuid":     taskUUID,
				"originalText": p.orig,
				"replacedText": p.repl,
			})
			if callErr == nil {
				callErr = minutesdata.RequireWriteAcknowledgement("replace text", writeData)
			}
			entry := map[string]any{"originalText": p.orig, "replacedText": p.repl}
			acknowledged := callErr == nil
			if acknowledged {
				entry["acknowledged"] = true
				beforeSource := strings.Count(currentText, p.orig)
				beforeTarget := strings.Count(currentText, p.repl)
				afterText, readErr := minutesReplaceTranscriptText(rt, taskUUID, rt.Int("page-limit"))
				if readErr != nil {
					callErr = fmt.Errorf("写入已确认但逐字稿读回失败: %w", readErr)
				} else if !replaceReadbackVerified(currentText, afterText, p, beforeSource, beforeTarget) {
					callErr = fmt.Errorf("写入已确认但逐字稿读回未证明替换生效")
				} else {
					currentText = afterText
					entry["verified"] = true
				}
			}
			if callErr != nil {
				entry["error"] = callErr.Error()
				entry["remoteEffectUnknown"] = acknowledged
				failed++
				results = append(results, entry)
				if rt.Str("failure-policy") == "stop" {
					for _, pending := range pairs[index+1:] {
						results = append(results, map[string]any{
							"originalText": pending.orig,
							"replacedText": pending.repl,
							"unattempted":  true,
						})
						unattempted++
					}
					break
				}
			} else {
				entry["applied"] = true
				applied++
				results = append(results, entry)
			}
		}

		payload := map[string]any{
			"ok":            failed == 0,
			"partial":       applied > 0 && (failed > 0 || unattempted > 0),
			"taskUuid":      taskUUID,
			"failurePolicy": rt.Str("failure-policy"),
			"total":         len(pairs),
			"applied":       applied,
			"failed":        failed,
			"unattempted":   unattempted,
			"results":       results,
		}
		if err := rt.Output(payload); err != nil {
			return err
		}
		if failed > 0 {
			return apperrors.NewAPI(
				fmt.Sprintf("批量替换未完成：成功 %d、失败 %d、未执行 %d", applied, failed, unattempted),
				apperrors.WithOperation("minutes/replace_minutes_text"),
				apperrors.WithOrigin("shortcut"),
				apperrors.WithFailureStage("write"),
				apperrors.WithReason("minutes_replace_partial_failure"),
				apperrors.WithExecutionStarted(true),
				apperrors.WithRetryable(false),
				apperrors.WithDetails(payload),
			)
		}
		return nil
	},
}

// replacePair is one parsed "原文=>替换" rule.
type replacePair struct {
	orig string
	repl string
}

// replacePairSep separates original from replacement text in a --pair value.
// "=>" is chosen because a bare "=" commonly appears inside the source text.
const replacePairSep = "=>"

// parseReplacePairs parses "原文=>替换" entries and rejects malformed input,
// empty source text, and duplicate source words (which would make two rules
// contend for the same term).
func parseReplacePairs(raw []string) ([]replacePair, error) {
	pairs := make([]replacePair, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, r := range raw {
		idx := strings.Index(r, replacePairSep)
		if idx < 0 {
			return nil, apperrors.NewValidation(fmt.Sprintf("替换规则 %q 缺少分隔符 %q，应为 \"原文%s替换\"", r, replacePairSep, replacePairSep))
		}
		orig := strings.TrimSpace(r[:idx])
		repl := strings.TrimSpace(r[idx+len(replacePairSep):])
		if orig == "" {
			return nil, apperrors.NewValidation(fmt.Sprintf("替换规则 %q 的「原文」不能为空", r))
		}
		if _, dup := seen[orig]; dup {
			return nil, apperrors.NewValidation(fmt.Sprintf("重复的「原文」%q：同一个原文只能有一条替换规则", orig))
		}
		seen[orig] = struct{}{}
		pairs = append(pairs, replacePair{orig: orig, repl: repl})
	}
	return pairs, nil
}

func loadReplacePairs(rt *shortcut.RuntimeContext) ([]replacePair, error) {
	pairs, err := parseReplacePairs(rt.StrSlice("pair"))
	if err != nil {
		return nil, err
	}
	if rt.Changed("json") {
		text, err := localio.ReadTextInput(rt.Str("json"), rt.Command().InOrStdin(), 8<<20)
		if err != nil {
			return nil, apperrors.NewValidation(err.Error())
		}
		jsonPairs, err := parseReplaceJSON(text)
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, jsonPairs...)
	}
	return validateReplacePairs(pairs)
}

func parseReplaceJSON(raw string) ([]replacePair, error) {
	type wirePair struct {
		Source       string `json:"source"`
		Replacement  string `json:"replacement"`
		OriginalText string `json:"originalText"`
		ReplacedText string `json:"replacedText"`
	}
	var direct []wirePair
	if err := json.Unmarshal([]byte(raw), &direct); err != nil {
		var envelope struct {
			Replacements []wirePair `json:"replacements"`
		}
		if envelopeErr := json.Unmarshal([]byte(raw), &envelope); envelopeErr != nil || envelope.Replacements == nil {
			return nil, apperrors.NewValidation(fmt.Sprintf("--json 必须是替换规则数组或 {\"replacements\":[...]}: %v", err))
		}
		direct = envelope.Replacements
	}
	if len(direct) == 0 {
		return nil, apperrors.NewValidation("--json 至少需要一条替换规则")
	}
	pairs := make([]replacePair, 0, len(direct))
	for index, item := range direct {
		source := item.Source
		if source == "" {
			source = item.OriginalText
		}
		replacement := item.Replacement
		if replacement == "" {
			replacement = item.ReplacedText
		}
		if strings.TrimSpace(source) == "" {
			return nil, apperrors.NewValidation(fmt.Sprintf("--json 第 %d 条规则缺少 source/originalText", index))
		}
		pairs = append(pairs, replacePair{orig: strings.TrimSpace(source), repl: strings.TrimSpace(replacement)})
	}
	return pairs, nil
}

func validateReplacePairs(pairs []replacePair) ([]replacePair, error) {
	if len(pairs) == 0 {
		return nil, apperrors.NewValidation("至少需要一条替换规则")
	}
	seen := map[string]bool{}
	for _, pair := range pairs {
		if strings.TrimSpace(pair.orig) == "" {
			return nil, apperrors.NewValidation("替换规则的原文不能为空")
		}
		if seen[pair.orig] {
			return nil, apperrors.NewValidation(fmt.Sprintf("重复的「原文」%q：同一个原文只能有一条替换规则", pair.orig))
		}
		seen[pair.orig] = true
	}
	return pairs, nil
}

func minutesReplaceTranscriptText(rt *shortcut.RuntimeContext, taskUUID string, pageLimit int) (string, error) {
	result, err := collectMinutesTranscript(rt, taskUUID, "0", "", false, pageLimit)
	if err != nil {
		return "", err
	}
	if len(result.Paragraphs) == 0 {
		return "", fmt.Errorf("逐字稿为空，无法证明文字替换目标或写入结果")
	}
	parts := make([]string, 0, len(result.Paragraphs))
	var collect func(any)
	collect = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			for key, child := range typed {
				switch key {
				case "paragraph", "sentence", "word", "text", "content":
					if text, ok := child.(string); ok && text != "" {
						parts = append(parts, text)
					}
				default:
					collect(child)
				}
			}
		case []any:
			for _, child := range typed {
				collect(child)
			}
		}
	}
	for _, paragraph := range result.Paragraphs {
		collect(paragraph)
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("逐字稿没有可验证的文本字段")
	}
	return strings.Join(parts, "\n"), nil
}

func replaceReadbackVerified(before, after string, pair replacePair, beforeSource, beforeTarget int) bool {
	if before == after {
		return false
	}
	afterSource := strings.Count(after, pair.orig)
	if pair.repl == "" {
		return afterSource < beforeSource
	}
	afterTarget := strings.Count(after, pair.repl)
	if strings.Contains(pair.repl, pair.orig) {
		return afterTarget > beforeTarget
	}
	return afterSource < beforeSource && afterTarget > beforeTarget
}

func init() {
	shortcut.Register(finalizeMinutesSmartShortcut(ReplaceBatch))
}
