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

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
)

// The envelope is remote data; none of these malformed shapes may panic the
// command build — pflag panics on duplicate long names, duplicate shorthands,
// and multi-character shorthands, and a poisoned discovery cache used to take
// down every CLI invocation this way (pre-1.0.32 lockout class).
func TestBuildDynamicCommandsSurvivesMalformedFlagEnvelope(t *testing.T) {
	tests := []struct {
		name  string
		flags map[string]market.CLIFlagOverride
	}{
		{
			name: "duplicate shorthand across two flags",
			flags: map[string]market.CLIFlagOverride{
				"alpha": {Shorthand: "x"},
				"beta":  {Shorthand: "x"},
			},
		},
		{
			name: "multi-character shorthand",
			flags: map[string]market.CLIFlagOverride{
				"alpha": {Shorthand: "xy"},
			},
		},
		{
			name: "primary collides with reserved payload flag",
			flags: map[string]market.CLIFlagOverride{
				"params": {},
				"json":   {},
			},
		},
		{
			name: "cross-binding duplicate primary via alias",
			flags: map[string]market.CLIFlagOverride{
				"user_id":   {Alias: "target"},
				"member_id": {Alias: "target"},
			},
		},
		{
			name: "cross-binding alias collides with another primary",
			flags: map[string]market.CLIFlagOverride{
				"alpha": {},
				"beta":  {Aliases: []string{"alpha"}},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			servers := []market.ServerDescriptor{
				{
					Endpoint: "https://endpoint-guard",
					CLI: market.CLIOverlay{
						ID:      "guard",
						Command: "guard",
						ToolOverrides: map[string]market.CLIToolOverride{
							"guard_tool": {
								CLIName: "boom",
								Flags:   tc.flags,
							},
						},
					},
				},
			}

			// Must not panic; the command must build and stay executable.
			cmds := BuildDynamicCommands(servers, &captureRunner{}, nil, nil)
			if len(cmds) != 1 {
				t.Fatalf("BuildDynamicCommands() = %d commands, want 1", len(cmds))
			}
			cmds[0].SetArgs([]string{"boom", "--help"})
			cmds[0].SilenceErrors = true
			cmds[0].SilenceUsage = true
			if err := cmds[0].Execute(); err != nil {
				t.Fatalf("execute --help: %v", err)
			}
		})
	}
}

// TestBuildDynamicCommandsKeepsFirstShorthand pins the winner: when two
// flags claim the same shorthand, the first (sorted param order) keeps it
// and the second still registers its long flag.
func TestBuildDynamicCommandsKeepsFirstShorthand(t *testing.T) {
	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-guard",
			CLI: market.CLIOverlay{
				ID:      "guard",
				Command: "guard",
				ToolOverrides: map[string]market.CLIToolOverride{
					"guard_tool": {
						CLIName: "boom",
						Flags: map[string]market.CLIFlagOverride{
							"alpha": {Shorthand: "x"},
							"beta":  {Shorthand: "x"},
						},
					},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, &captureRunner{}, nil, nil)
	boom, _, err := cmds[0].Find([]string{"boom"})
	if err != nil {
		t.Fatalf("find boom: %v", err)
	}
	short := boom.Flags().ShorthandLookup("x")
	if short == nil || short.Name != "alpha" {
		t.Fatalf("shorthand -x bound to %v, want alpha", short)
	}
	if boom.Flags().Lookup("beta") == nil {
		t.Fatalf("long flag --beta missing; dropping the shorthand must not drop the flag")
	}
}
