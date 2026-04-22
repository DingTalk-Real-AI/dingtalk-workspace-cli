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

package edition

// DefaultOSSClawType is the wire value for request header claw-type in
// the open-source build. It is intentionally hard-wired — the open-source
// CLI does NOT derive claw-type from DINGTALK_AGENT or any other caller
// input, so third-party hosts get a predictable header regardless of
// their environment. See docs/pat/contract.md §7b.
const DefaultOSSClawType = "openClaw"

// defaultHooks returns the open-source edition defaults.
//
// MergeHeaders is the only hook that ships with behaviour: it pins the
// `claw-type` request header to DefaultOSSClawType so every open-source
// MCP request carries the same stable routing tag. All other fields are
// nil — the internal code interprets nil as "use standard open-source
// behaviour".
func defaultHooks() *Hooks {
	return &Hooks{
		Name: "open",
		MergeHeaders: func(base map[string]string) map[string]string {
			if base == nil {
				base = make(map[string]string)
			}
			base["claw-type"] = DefaultOSSClawType
			return base
		},
	}
}
