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

package compat

import "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"

// crossEditionFlagAliases bridges flag-name differences between the
// open-source `dws` build and the closed-source DingTalk in-house build
// ("wukong"). The same underlying MCP parameter can be exposed as
// different CLI flag names across editions — when that happens, users
// who follow wukong-flavoured docs or copy-paste wukong commands get a
// confusing `unknown flag: --foo` error on open-source binaries.
//
// Each entry: canonical tool name (`<productID>.<rpcName>`) → MCP
// parameter name → list of *additional* hidden CLI flag aliases to
// accept. These do **not** override the primary CLI flag name advertised
// by the server-side overlay; they're registered as silent fallbacks so
// either spelling works.
//
// Adding entries:
//   - Use the canonical path printed by `dws schema "<group> <name>"`.
//   - Inspect the server overlay's `flag_overlay.<param>.alias` to find
//     the open-source primary name. Add only the *other* edition's name
//     as alias, not the primary.
//   - Keep entries minimal — alias registration is hidden in help, but
//     each one is a maintenance liability if the upstream renames again.
//
// Verified divergences (as of 2026-05):
//
//   sheet.find_cells | text → primary: --find (opensource), alias: --query (wukong)
//
// (Confirmed via `dws sheet find --help` showing `--find` on opensource
// v1.0.26 and `--query` on wukong v0.2.67, where wukong's help description
// explicitly states "别名: --find".)
var crossEditionFlagAliases = map[string]map[string][]string{
	"sheet.find_cells": {
		"text": {"query"},
	},
}

// applyCrossEditionAliases mutates `override.Flags` to add hidden CLI
// aliases declared in `crossEditionFlagAliases` for the given canonical
// tool path. No-op when the canonical path is unknown. Existing aliases
// (whether from the server overlay or already injected) are preserved
// and de-duplicated.
func applyCrossEditionAliases(override *market.CLIToolOverride, canonicalPath string) {
	if override == nil {
		return
	}
	extras, ok := crossEditionFlagAliases[canonicalPath]
	if !ok || len(extras) == 0 {
		return
	}
	if override.Flags == nil {
		override.Flags = make(map[string]market.CLIFlagOverride, len(extras))
	}
	for paramName, aliasesToAdd := range extras {
		flag := override.Flags[paramName]
		// Dedup against the primary alias + existing alias list.
		existing := map[string]bool{}
		if flag.Alias != "" {
			existing[flag.Alias] = true
		}
		for _, a := range flag.Aliases {
			existing[a] = true
		}
		added := false
		for _, a := range aliasesToAdd {
			if a == "" || existing[a] {
				continue
			}
			flag.Aliases = append(flag.Aliases, a)
			existing[a] = true
			added = true
		}
		if added {
			override.Flags[paramName] = flag
		}
	}
}
