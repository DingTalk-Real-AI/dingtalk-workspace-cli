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

package app

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// TestParamAliasFixtureThroughEmbeddedDeliveryPath is the ⑥ regression gate.
// It reads the reviewed validation_fixture straight from the embedded concept
// dictionary and asserts every reviewed bad case through the REAL delivery
// path — not a generator unit test and not a reimplementation of the reduction
// logic:
//
//   - the runtime PreParse engine built by newPipelineEngine() (the exact
//     handler chain root.go installs, whose SemanticAliasHandler is wired to
//     cli.LookupParamAlias over the embedded generated table), and
//   - the embedded cli.LookupParamAlias query used to prove that a
//     did-you-mean case is an intentional block/ambiguous guard rather than a
//     name that merely happens to be absent from the table.
//
// Fixture expect semantics (see spec §⑥):
//   - expect=<realFlag>            : emitted must reduce to that canonical flag.
//   - expect=did-you-mean:blocked  : block guard hit; never auto-rewritten.
//   - expect=did-you-mean:ambiguous: co-occurrence guard hit; never rewritten.
func TestParamAliasFixtureThroughEmbeddedDeliveryPath(t *testing.T) {
	concepts, err := cli.LoadParamConcepts()
	if err != nil {
		t.Fatalf("LoadParamConcepts() error = %v", err)
	}
	if len(concepts.Fixture) == 0 {
		t.Fatal("validation_fixture declares no cases; ⑥ gate would be vacuous")
	}

	// The exact runtime handler chain (alias → semantic → sticky → paramname),
	// with the semantic table sourced from the embedded generated snapshot.
	engine := newPipelineEngine()
	// The live Cobra tree is the source of truth for whether a synonym is a
	// genuine (often hidden) real flag the command already accepts.
	root := NewRootCommand()

	const fixtureValue = "FIXTURE_VALUE"

	for _, c := range concepts.Fixture {
		t.Run(c.Command+"/"+c.Emitted, func(t *testing.T) {
			// Fixture command paths carry no "dws" prefix; LookupParamAlias
			// normalizes to the same key the generator used, so the runtime
			// lookup is byte-identical to the build-time key.
			entry, ok := cli.LookupParamAlias(c.Command)
			if !ok {
				t.Fatalf("no embedded alias entry for command %q (fixture references a command not covered by the generated table)", c.Command)
			}

			ctx := &pipeline.Context{
				Args:    []string{"--" + c.Emitted, fixtureValue},
				Command: c.Command,
			}
			if err := engine.RunPhase(pipeline.PreParse, ctx); err != nil {
				t.Fatalf("PreParse error = %v", err)
			}
			morphed := cmdutil.Morph(c.Emitted)

			switch c.Expect {
			case "did-you-mean:ambiguous":
				if !entry.IsAmbiguous(morphed) {
					t.Fatalf("%q on %q: expected co-occurrence guard (ambiguous) but embedded entry does not classify it; ambiguous=%v", c.Emitted, c.Command, entry.Ambiguous)
				}
				assertLeftUnchanged(t, ctx, c.Emitted, fixtureValue)
			case "did-you-mean:blocked":
				if !entry.IsBlocked(morphed) {
					t.Fatalf("%q on %q: expected block guard but embedded entry does not classify it; blocked=%v", c.Emitted, c.Command, entry.Blocked)
				}
				assertLeftUnchanged(t, ctx, c.Emitted, fixtureValue)
			default:
				// Real-flag expect: the reviewed canonical outcome is delivered
				// one of two equally valid ways, and the gate accepts either
				// (failing only on a genuine unknown-flag hallucination):
				//   1. semantic rewrite — the emitted synonym is not a real flag,
				//      so the embedded table rewrites it to the canonical flag; or
				//   2. native acceptance — the emitted synonym is still a genuine
				//      (usually hidden) real flag the command accepts directly and
				//      maps to the same entity via its fallback wiring. These are
				//      the un-migrated commands pending §5 hidden-flag cleanup.
				if len(ctx.Args) < 2 || ctx.Args[1] != fixtureValue {
					t.Fatalf("%q on %q lost its value: args=%v", c.Emitted, c.Command, ctx.Args)
				}
				switch got := ctx.Args[0]; got {
				case "--" + c.Expect:
					// (1) rewritten; the embedded table must agree.
					if canon, hit := entry.ResolveAlias(morphed); !hit || canon != c.Expect {
						t.Fatalf("embedded table ResolveAlias(%q) on %q = %q (hit=%v), want %q", morphed, c.Command, canon, hit, c.Expect)
					}
				case "--" + c.Emitted:
					// (2) not rewritten — only valid if the command natively
					// accepts the emitted synonym as a real flag.
					leaf := resolveParamLeaf(root, c.Command)
					if leaf == nil {
						t.Fatalf("%q on %q was not reduced and the command could not be resolved in the Cobra tree", c.Emitted, c.Command)
					}
					if !commandHasRealFlagByMorph(leaf, morphed) {
						t.Fatalf("%q on %q was neither reduced to --%s nor accepted as a real flag (unknown-flag hallucination)", c.Emitted, c.Command, c.Expect)
					}
				default:
					t.Fatalf("%q on %q reduced to unexpected %q, want --%s or native --%s (args=%v)", c.Emitted, c.Command, got, c.Expect, c.Emitted, ctx.Args)
				}
			}
		})
	}
}

// assertLeftUnchanged verifies a guarded (blocked/ambiguous) synonym is never
// silently rewritten: the flag token and its value survive verbatim so the
// unknown-flag did-you-mean path can surface the reviewed candidates.
func assertLeftUnchanged(t *testing.T, ctx *pipeline.Context, emitted, value string) {
	t.Helper()
	if got := ctx.Args[0]; got != "--"+emitted {
		t.Fatalf("guarded synonym --%s was rewritten to %q (must be left for did-you-mean): args=%v", emitted, got, ctx.Args)
	}
	if len(ctx.Args) < 2 || ctx.Args[1] != value {
		t.Fatalf("guarded synonym --%s lost its value: args=%v", emitted, ctx.Args)
	}
	for _, corr := range ctx.Corrections {
		if corr.Handler == "semantic-alias" && corr.Original == "--"+emitted {
			t.Fatalf("guarded synonym --%s was corrected by %s (must not be): %+v", emitted, corr.Handler, corr)
		}
	}
}

// resolveParamLeaf resolves a fixture command path (no "dws" prefix, e.g.
// "chat message search-advanced") to its live Cobra command, or nil.
func resolveParamLeaf(root *cobra.Command, path string) *cobra.Command {
	cmd, _, err := root.Find(strings.Fields(path))
	if err != nil || cmd == nil || cmd == root {
		return nil
	}
	return cmd
}

// commandHasRealFlagByMorph reports whether the command has any real flag
// (local or inherited, including hidden) whose Morph matches morphed — the same
// notion of "real flag" the build-time reducer uses to absorb legacy synonyms.
func commandHasRealFlagByMorph(cmd *cobra.Command, morphed string) bool {
	found := false
	check := func(f *pflag.Flag) {
		if f.Name != "help" && cmdutil.Morph(f.Name) == morphed {
			found = true
		}
	}
	cmd.Flags().VisitAll(check)
	cmd.InheritedFlags().VisitAll(check)
	return found
}
