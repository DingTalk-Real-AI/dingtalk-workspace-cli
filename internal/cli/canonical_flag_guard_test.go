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

import (
	"testing"

	"github.com/spf13/cobra"
)

// newToolCommandFixture mirrors the flag environment of newToolCommand: the
// reserved payload flags are registered before the spec loop runs.
func newToolCommandFixture() *cobra.Command {
	cmd := &cobra.Command{Use: "probe"}
	cmd.Flags().String("json", "", "Base JSON object payload for this tool invocation")
	cmd.Flags().String("params", "", "Additional JSON object payload merged after --json")
	return cmd
}

// TestApplyFlagSpecsSkipsReservedNames locks in the fix for the 1.0.32-class
// lock-out: a tool schema property named after a reserved payload flag
// ("params", as cached during the chat_permission_grant incident, or "json")
// must be skipped instead of panicking pflag ("flag redefined") — that panic
// fires while the canonical tree is assembled, before Cobra dispatches
// anything, and used to kill every invocation including `dws cache refresh`
// and `dws upgrade`.
func TestApplyFlagSpecsSkipsReservedNames(t *testing.T) {
	t.Parallel()

	cmd := newToolCommandFixture()
	applyFlagSpecs(cmd, []FlagSpec{
		{PropertyName: "params", FlagName: "params", Kind: flagString, Description: "命令授权参数"},
		{PropertyName: "json", FlagName: "json", Kind: flagString},
		{PropertyName: "scope", FlagName: "scope", Kind: flagString},
	})

	if cmd.Flags().Lookup("scope") == nil {
		t.Errorf("non-colliding flag --scope was not registered")
	}
	// The reserved flags must keep their payload usage strings, proving the
	// schema-derived specs did not touch them.
	if got := cmd.Flags().Lookup("params").Usage; got != "Additional JSON object payload merged after --json" {
		t.Errorf("--params usage = %q, want the reserved payload usage", got)
	}
}

// TestApplyFlagSpecsSkipsDuplicates covers duplicate property names within a
// single tool schema (or a spec colliding with an already-applied one).
func TestApplyFlagSpecsSkipsDuplicates(t *testing.T) {
	t.Parallel()

	cmd := newToolCommandFixture()
	applyFlagSpecs(cmd, []FlagSpec{
		{PropertyName: "scope", FlagName: "scope", Kind: flagString, Description: "first"},
		{PropertyName: "scope", FlagName: "scope", Kind: flagBoolean, Description: "second"},
	})

	flag := cmd.Flags().Lookup("scope")
	if flag == nil {
		t.Fatalf("--scope was not registered at all")
	}
	if flag.Usage != "first" {
		t.Errorf("--scope usage = %q, want the first spec to win", flag.Usage)
	}
}

// TestApplyFlagSpecsSkipsCollidingAlias verifies an alias colliding with a
// reserved or existing flag is dropped while the primary still registers.
func TestApplyFlagSpecsSkipsCollidingAlias(t *testing.T) {
	t.Parallel()

	cmd := newToolCommandFixture()
	applyFlagSpecs(cmd, []FlagSpec{
		{PropertyName: "scope", FlagName: "scope", Alias: "params", Kind: flagString},
	})

	if cmd.Flags().Lookup("scope") == nil {
		t.Errorf("primary flag --scope was not registered when its alias collided")
	}
	if got := cmd.Flags().Lookup("params").Usage; got != "Additional JSON object payload merged after --json" {
		t.Errorf("--params usage = %q, alias overwrote the reserved payload flag", got)
	}
}

// TestApplyFlagSpecsSanitizesShorthand verifies multi-character and duplicate
// shorthands (both pflag panics) degrade to long-flag-only registration.
func TestApplyFlagSpecsSanitizesShorthand(t *testing.T) {
	t.Parallel()

	cmd := newToolCommandFixture()
	applyFlagSpecs(cmd, []FlagSpec{
		{PropertyName: "alpha", FlagName: "alpha", Shorthand: "ab", Kind: flagString},
		{PropertyName: "beta", FlagName: "beta", Shorthand: "s", Kind: flagString},
		{PropertyName: "gamma", FlagName: "gamma", Shorthand: "s", Kind: flagString},
	})

	for _, name := range []string{"alpha", "beta", "gamma"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("--%s was not registered", name)
		}
	}
	if flag := cmd.Flags().ShorthandLookup("s"); flag == nil || flag.Name != "beta" {
		t.Errorf("shorthand -s should stay bound to the first claimant --beta, got %v", flag)
	}
}
