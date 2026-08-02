// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package handlers

import (
	"sort"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
)

// BoolValueHandler gives every real Cobra boolean flag one consistent input
// grammar before pflag parsing. pflag interprets `--yes false` as a bare
// `--yes` (true) plus a positional `false`; rewriting the pair to
// `--yes=false` preserves the value the caller actually supplied. The same
// rule applies to default-true, required, local, and inherited boolean flags:
// the handler preserves an explicit value and never substitutes a default.
//
// This handler runs after all flag-name handlers so camelCase, semantic
// aliases, and conservative fuzzy corrections have already reached their
// canonical names. It consumes only recognised boolean literals, stops at
// `--`, and rejects contradictory values for the same canonical flag so the
// result cannot depend on argv order.
type BoolValueHandler struct{}

func (BoolValueHandler) Name() string          { return "boolvalue" }
func (BoolValueHandler) Phase() pipeline.Phase { return pipeline.PreParse }

func (BoolValueHandler) Handle(ctx *pipeline.Context) error {
	if len(ctx.Args) == 0 || len(ctx.FlagSpecs) == 0 {
		return nil
	}

	longNames := make(map[string]pipeline.FlagInfo)
	shortNames := make(map[string]pipeline.FlagInfo)
	for _, spec := range ctx.FlagSpecs {
		if spec.Name == "" || (spec.Type != "bool" && spec.Type != "boolean") {
			continue
		}
		longNames[spec.Name] = spec
		if spec.Shorthand != "" {
			shortNames[spec.Shorthand] = spec
		}
	}

	result := make([]string, 0, len(ctx.Args))
	valuesByFlag := make(map[string]map[string]bool)
	for index := 0; index < len(ctx.Args); index++ {
		argument := ctx.Args[index]
		if argument == "--" {
			result = append(result, ctx.Args[index:]...)
			break
		}

		spec, inlineValue, hasInlineValue, matched := matchBooleanFlagToken(argument, longNames, shortNames)
		if !matched || ctx.IsFlagProtected(cmdutil.Morph(spec.Name)) {
			result = append(result, argument)
			continue
		}

		normalized := "true"
		corrected := argument
		original := argument
		changed := false
		if hasInlineValue {
			var ok bool
			normalized, ok = cmdutil.NormalizeBoolLiteral(inlineValue)
			if !ok {
				result = append(result, argument)
				continue
			}
			corrected = "--" + spec.Name + "=" + normalized
			changed = corrected != argument
		} else if index+1 < len(ctx.Args) {
			if value, ok := cmdutil.NormalizeBoolLiteral(ctx.Args[index+1]); ok {
				normalized = value
				corrected = "--" + spec.Name + "=" + normalized
				original = strings.Join(ctx.Args[index:index+2], " ")
				changed = true
				index++
			}
		}

		if valuesByFlag[spec.Name] == nil {
			valuesByFlag[spec.Name] = make(map[string]bool)
		}
		valuesByFlag[spec.Name][normalized] = true
		if changed {
			ctx.AddCorrection("boolvalue", pipeline.PreParse, "--"+spec.Name, original, corrected, "explicit-bool")
		}
		result = append(result, corrected)
	}

	ctx.Args = result
	names := make([]string, 0, len(valuesByFlag))
	for name := range valuesByFlag {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		values := valuesByFlag[name]
		if len(values) < 2 {
			continue
		}
		list := make([]string, 0, len(values))
		for value := range values {
			list = append(list, value)
		}
		sort.Strings(list)
		return &pipeline.BoolValueConflictError{
			Command: ctx.Command,
			Flag:    name,
			Values:  list,
		}
	}
	return nil
}

// matchBooleanFlagToken recognises canonical long flags, exact shorthands,
// and their explicit =value forms. Shorthand clusters remain native pflag
// syntax and are intentionally not reinterpreted here.
func matchBooleanFlagToken(argument string, longNames, shortNames map[string]pipeline.FlagInfo) (pipeline.FlagInfo, string, bool, bool) {
	if strings.HasPrefix(argument, "--") {
		bare, suffix, isFlag := splitFlagToken(argument)
		if !isFlag {
			return pipeline.FlagInfo{}, "", false, false
		}
		spec, ok := longNames[bare]
		if !ok {
			return pipeline.FlagInfo{}, "", false, false
		}
		if suffix == "" {
			return spec, "", false, true
		}
		return spec, strings.TrimPrefix(suffix, "="), true, true
	}
	if !strings.HasPrefix(argument, "-") || strings.HasPrefix(argument, "--") {
		return pipeline.FlagInfo{}, "", false, false
	}
	body := strings.TrimPrefix(argument, "-")
	name, value, hasValue := body, "", false
	if index := strings.IndexByte(body, '='); index >= 0 {
		name, value, hasValue = body[:index], body[index+1:], true
	}
	spec, ok := shortNames[name]
	if !ok {
		return pipeline.FlagInfo{}, "", false, false
	}
	return spec, value, hasValue, true
}
