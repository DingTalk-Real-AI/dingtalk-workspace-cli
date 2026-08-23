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

package builtin_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/builtin"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// TestCmdcoreMountPreservesEveryBuiltInShortcutSurface is the differential
// guard for the live mount migration. It derives the historical Cobra surface
// directly from each Shortcut declaration and checks the command-built tree.
func TestCrossPlatformCoverageCmdcoreMountPreservesEveryBuiltInShortcutSurface(t *testing.T) {
	mounted := map[string]*cobra.Command{}
	for _, service := range builtin.BaseCommands() {
		for _, command := range service.Commands() {
			mounted[service.Name()+"\x00"+command.Name()] = command
		}
	}

	var declarations int
	for _, spec := range shortcut.All() {
		if spec.UserDefined {
			continue
		}
		declarations++
		key := spec.Service + "\x00" + spec.Command
		command := mounted[key]
		if command == nil {
			t.Errorf("%s %s: command is not mounted", spec.Service, spec.Command)
			continue
		}
		if command.Short != spec.Description {
			t.Errorf("%s %s: Short = %q, want %q", spec.Service, spec.Command, command.Short, spec.Description)
		}
		if command.Long != expectedShortcutLong(spec) {
			t.Errorf("%s %s: Long mismatch\n got: %q\nwant: %q",
				spec.Service, spec.Command, command.Long, expectedShortcutLong(spec))
		}
		if command.Example != expectedShortcutExamples(spec.Tips) {
			t.Errorf("%s %s: Example = %q, want %q",
				spec.Service, spec.Command, command.Example, expectedShortcutExamples(spec.Tips))
		}
		if command.Hidden != spec.Hidden {
			t.Errorf("%s %s: Hidden = %v, want %v", spec.Service, spec.Command, command.Hidden, spec.Hidden)
		}

		declaredFlags := make(map[string]shortcut.Flag, len(spec.Flags))
		for _, flag := range spec.Flags {
			declaredFlags[flag.Name] = flag
			got := command.Flags().Lookup(flag.Name)
			if got == nil {
				t.Errorf("%s %s: flag --%s is not mounted", spec.Service, spec.Command, flag.Name)
				continue
			}
			if got.Value.Type() != expectedFlagType(flag.Type) {
				t.Errorf("%s %s --%s: type = %q, want %q",
					spec.Service, spec.Command, flag.Name, got.Value.Type(), expectedFlagType(flag.Type))
			}
			if got.Usage != expectedFlagHelp(flag) {
				t.Errorf("%s %s --%s: usage = %q, want %q",
					spec.Service, spec.Command, flag.Name, got.Usage, expectedFlagHelp(flag))
			}
			if got.Hidden != flag.Hidden {
				t.Errorf("%s %s --%s: hidden = %v, want %v",
					spec.Service, spec.Command, flag.Name, got.Hidden, flag.Hidden)
			}
			assertShortcutDefault(t, command, spec, flag)
			for _, alias := range flag.Aliases {
				declaredFlags[alias] = flag
				gotAlias := command.Flags().Lookup(alias)
				if gotAlias == nil {
					t.Errorf("%s %s: flag alias --%s is not mounted", spec.Service, spec.Command, alias)
					continue
				}
				wantHidden := !flag.AliasesVisible
				if gotAlias.Hidden != wantHidden {
					t.Errorf("%s %s: flag alias --%s hidden = %v, want %v", spec.Service, spec.Command, alias, gotAlias.Hidden, wantHidden)
				}
			}
		}
		command.Flags().VisitAll(func(flag *pflag.Flag) {
			if flag.Name == "help" {
				return
			}
			if _, ok := declaredFlags[flag.Name]; !ok {
				t.Errorf("%s %s: undeclared flag --%s was mounted", spec.Service, spec.Command, flag.Name)
			}
		})
	}
	if len(mounted) != declarations {
		t.Fatalf("mounted built-in shortcuts = %d, declarations = %d", len(mounted), declarations)
	}
}

func expectedShortcutLong(spec shortcut.Shortcut) string {
	long := strings.TrimSpace(spec.Intent)
	if long == "" {
		long = strings.TrimSpace(spec.Description)
	}
	if len(spec.Constraints) == 0 {
		return long
	}
	lines := make([]string, 0, len(spec.Constraints))
	for _, constraint := range spec.Constraints {
		text := strings.TrimSpace(constraint.Description)
		if text == "" {
			switch constraint.Kind {
			case shortcut.ConstraintAtLeastOne:
				text = expectedDashed(constraint.Flags) + " 至少指定一个"
			case shortcut.ConstraintExactlyOne:
				text = expectedDashed(constraint.Flags) + " 必须且只能指定一个"
			case shortcut.ConstraintMutuallyExclusive:
				text = expectedDashed(constraint.Flags) + " 互斥，最多指定一个"
			}
		}
		lines = append(lines, "  - "+text)
	}
	return long + "\n\n参数约束：\n" + strings.Join(lines, "\n")
}

func expectedShortcutExamples(tips []string) string {
	if len(tips) == 0 {
		return ""
	}
	return "  " + strings.Join(tips, "\n  ")
}

func expectedFlagType(kind shortcut.FlagType) string {
	switch kind {
	case shortcut.FlagBool:
		return "bool"
	case shortcut.FlagInt:
		return "int"
	case shortcut.FlagStringSlice:
		return "stringSlice"
	default:
		return "string"
	}
}

func expectedFlagHelp(flag shortcut.Flag) string {
	parts := make([]string, 0, 2)
	if flag.Required && !strings.Contains(flag.Desc, "必填") {
		parts = append(parts, "必填")
	}
	if len(flag.Enum) > 0 {
		parts = append(parts, "可选值: "+strings.Join(flag.Enum, ", "))
	}
	if len(parts) == 0 {
		return flag.Desc
	}
	if strings.TrimSpace(flag.Desc) == "" {
		return strings.Join(parts, "；")
	}
	return flag.Desc + "（" + strings.Join(parts, "；") + "）"
}

func assertShortcutDefault(t *testing.T, command *cobra.Command, spec shortcut.Shortcut, flag shortcut.Flag) {
	t.Helper()
	switch flag.Type {
	case shortcut.FlagBool:
		got, err := command.Flags().GetBool(flag.Name)
		if err != nil || got != (flag.Default == "true") {
			t.Errorf("%s %s --%s: bool default = %v/%v, want %v",
				spec.Service, spec.Command, flag.Name, got, err, flag.Default == "true")
		}
	case shortcut.FlagInt:
		want, _ := strconv.Atoi(flag.Default)
		got, err := command.Flags().GetInt(flag.Name)
		if err != nil || got != want {
			t.Errorf("%s %s --%s: int default = %d/%v, want %d",
				spec.Service, spec.Command, flag.Name, got, err, want)
		}
	case shortcut.FlagStringSlice:
		var want []string
		if value := strings.TrimSpace(flag.Default); value != "" {
			want = strings.Split(value, ",")
		}
		got, err := command.Flags().GetStringSlice(flag.Name)
		if err != nil || strings.Join(got, "\x00") != strings.Join(want, "\x00") {
			t.Errorf("%s %s --%s: string-slice default = %#v/%v, want %#v",
				spec.Service, spec.Command, flag.Name, got, err, want)
		}
	default:
		got, err := command.Flags().GetString(flag.Name)
		if err != nil || got != flag.Default {
			t.Errorf("%s %s --%s: string default = %q/%v, want %q",
				spec.Service, spec.Command, flag.Name, got, err, flag.Default)
		}
	}
}

func expectedDashed(flags []string) string {
	out := make([]string, len(flags))
	for i, flag := range flags {
		out[i] = "--" + flag
	}
	return strings.Join(out, "、")
}
