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
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
)

// SemanticAliasHandler rewrites semantic parameter synonyms to a command's
// canonical real flag using the build-time reduced alias table. Where
// AliasHandler only folds a spelling that already matches a real flag
// (--userId → --user-id), this handler resolves a different word to the real
// flag the command actually accepts (--keyword → --query) based on the
// reviewed concept dictionary.
//
// It runs in PreParse after AliasHandler (morphology first) and before sticky
// and paramname. The alias table is injected as Lookup so this handler never
// imports the cli package; root.go wires cli.LookupParamAlias in.
type SemanticAliasHandler struct {
	// Lookup returns the aliases/blocked/ambiguous sets reduced for a raw
	// Cobra CommandPath, or ok=false when the command has no reduced entry.
	// Keys are already morphed (cmdutil.Morph), matching how the table is built.
	Lookup func(rawCommandPath string) (aliases map[string]string, blocked, ambiguous []string, ok bool)
}

func (SemanticAliasHandler) Name() string          { return "semantic-alias" }
func (SemanticAliasHandler) Phase() pipeline.Phase { return pipeline.PreParse }

func (h SemanticAliasHandler) Handle(ctx *pipeline.Context) error {
	if h.Lookup == nil || ctx.Command == "" || len(ctx.Args) == 0 {
		return nil
	}
	aliases, blocked, ambiguous, ok := h.Lookup(ctx.Command)
	if !ok || len(aliases) == 0 {
		return nil
	}

	for i, arg := range ctx.Args {
		bare, suffix, isFlag := splitFlagToken(arg)
		if !isFlag {
			continue
		}
		morphed := cmdutil.Morph(bare)

		// A blocked or intentionally ambiguous name must never be silently
		// rewritten: it is left untouched so the unknown-flag did-you-mean
		// path can surface the reviewed candidates instead of guessing.
		if contains(blocked, morphed) || contains(ambiguous, morphed) {
			continue
		}

		canon, hit := aliases[morphed]
		if !hit || canon == bare {
			continue
		}

		rewritten := "--" + canon + suffix
		ctx.Args[i] = rewritten
		ctx.AddCorrection("semantic-alias", pipeline.PreParse, canon, arg, rewritten, "semantic")
	}
	return nil
}

// splitFlagToken splits a raw argv token into its bare flag name and any
// "=value" suffix. isFlag is false for anything that is not a "--flag" token
// (positional args, "-x" short flags, the bare "--" separator, or "--=v").
func splitFlagToken(arg string) (bare, suffix string, isFlag bool) {
	if !strings.HasPrefix(arg, "--") {
		return "", "", false
	}
	body := arg[2:]
	if body == "" {
		return "", "", false
	}
	if idx := strings.IndexByte(body, '='); idx >= 0 {
		if idx == 0 {
			return "", "", false
		}
		return body[:idx], body[idx:], true
	}
	return body, "", true
}

func contains(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}
