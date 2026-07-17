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
	"fmt"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
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
// and aggregates a per-pair {applied|error} report through rt.Output — one
// failing pair does not abort the rest.
//
//	dws minutes +replace-batch --id <taskUuid> --pair "张三=>张三丰" --pair "Q2=>第二季度"
var ReplaceBatch = shortcut.Shortcut{
	Service:     "minutes",
	Command:     "+replace-batch",
	Product:     "minutes",
	Description: "对一条妙记（听记）批量执行多组文字替换（原文=>替换）",
	Intent: "当你要在同一条听记里一次性纠正多个词（如把多个错识别的人名/术语统一替换），而底层工具一次只能替换一组时使用；" +
		"内部按多个 --pair \"原文=>替换\" 逐组调用替换工具，先在本地校验去重（同一个「原文」不能出现两次，避免两条规则互相打架），" +
		"再逐组应用并聚合每组的成功/失败结果，某一组失败不会中断其余组。这是写操作，会实际修改听记文字内容，请确认 taskUuid 与替换规则无误。",
	Risk: shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "id", Type: shortcut.FlagString, Desc: "听记 taskUuid（必填）", Required: true},
		{Name: "pair", Type: shortcut.FlagStringSlice, Desc: `替换规则，格式 "原文=>替换"，可重复传多组（必填）`, Required: true},
	},
	Constraints: []shortcut.Constraint{
		{
			Kind:        shortcut.ConstraintCustom,
			Flags:       []string{"pair"},
			Description: `每个 --pair 必须使用 "原文=>替换" 格式，原文不能为空且不能重复`,
		},
	},
	Tips: []string{
		`dws minutes +replace-batch --id <taskUuid> --pair "张三=>张三丰"`,
		`dws minutes +replace-batch --id <taskUuid> --pair "Q2=>第二季度" --pair "PM=>产品经理"`,
	},
	Validate: func(rt *shortcut.RuntimeContext) error {
		_, err := parseReplacePairs(rt.StrSlice("pair"))
		return err
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		taskUUID := rt.Str("id")
		pairs, err := parseReplacePairs(rt.StrSlice("pair"))
		if err != nil {
			return err // already validated, but stay defensive
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
				"dryRun":       true,
				"taskUuid":     taskUUID,
				"total":        len(pairs),
				"replacements": replacements,
			})
		}

		results := make([]map[string]any, 0, len(pairs))
		applied := 0
		for _, p := range pairs {
			// Params mirror helpers replace_minutes_text call site: taskUuid /
			// originalText / replacedText.
			_, callErr := rt.CallMCPWriteData("minutes", "replace_minutes_text", map[string]any{
				"taskUuid":     taskUUID,
				"originalText": p.orig,
				"replacedText": p.repl,
			})
			entry := map[string]any{"originalText": p.orig, "replacedText": p.repl}
			if callErr != nil {
				entry["error"] = callErr.Error()
			} else {
				entry["applied"] = true
				applied++
			}
			results = append(results, entry)
		}

		return rt.Output(map[string]any{
			"taskUuid": taskUUID,
			"total":    len(pairs),
			"applied":  applied,
			"failed":   len(pairs) - applied,
			"results":  results,
		})
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

func init() {
	shortcut.Register(ReplaceBatch)
}
