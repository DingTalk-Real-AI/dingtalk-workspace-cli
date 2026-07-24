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

package cli

import "sync"

// paramAliasIndex is the lazily built per-command view of the generated
// parameter-alias table, keyed by the same normalized CLI path the generator
// used. It is populated once; the generated slice never changes at runtime.
var (
	paramAliasIndexOnce sync.Once
	paramAliasIndex     map[string]ParamAliasEntry
)

func buildParamAliasIndex() {
	paramAliasIndex = make(map[string]ParamAliasEntry, len(generatedParamAliases))
	for _, e := range generatedParamAliases {
		paramAliasIndex[e.CLIPath] = e
	}
}

// LookupParamAlias resolves the reduced parameter-alias entry for a command.
//
// rawCommandPath is Cobra's CommandPath() (it still carries the "dws" prefix).
// It is normalized through normalizeSchemaCLIPath — the exact function the
// build-time generator used to key each entry — so the runtime lookup key is
// byte-identical to the generation key and there is zero mapping drift.
func LookupParamAlias(rawCommandPath string) (ParamAliasEntry, bool) {
	paramAliasIndexOnce.Do(buildParamAliasIndex)
	e, ok := paramAliasIndex[normalizeSchemaCLIPath(rawCommandPath)]
	return e, ok
}

// ResolveAlias returns the canonical real flag a morphed emitted name reduces
// to, if this command aliases it. The caller is expected to pass an
// already-morphed name (cmdutil.Morph), matching how the table is keyed.
func (e ParamAliasEntry) ResolveAlias(morphed string) (string, bool) {
	canon, ok := e.Aliases[morphed]
	return canon, ok
}

// IsBlocked reports whether a morphed emitted name is on this command's block
// list: it must never be auto-rewritten and instead routes to did-you-mean.
func (e ParamAliasEntry) IsBlocked(morphed string) bool {
	return containsParamAlias(e.Blocked, morphed)
}

// IsAmbiguous reports whether a morphed emitted name is on this command's
// reviewed co-occurrence whitelist: it is intentionally left unresolved so the
// runtime asks instead of guessing between two real flags.
func (e ParamAliasEntry) IsAmbiguous(morphed string) bool {
	return containsParamAlias(e.Ambiguous, morphed)
}

func containsParamAlias(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}
