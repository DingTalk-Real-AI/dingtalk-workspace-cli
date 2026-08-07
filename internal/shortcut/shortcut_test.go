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

package shortcut

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageProductDefaultsToService(t *testing.T) {
	if got := (Shortcut{Service: "contact"}).product(); got != "contact" {
		t.Fatalf("product() = %q, want contact", got)
	}
	if got := (Shortcut{Service: "contact", Product: "org"}).product(); got != "org" {
		t.Fatalf("product() = %q, want org", got)
	}
}

func TestCrossPlatformCoverageRiskDefaultsToRead(t *testing.T) {
	if got := (Shortcut{}).risk(); got != RiskRead {
		t.Fatalf("risk() = %q, want read", got)
	}
	if got := (Shortcut{Risk: RiskHighWrite}).risk(); got != RiskHighWrite {
		t.Fatalf("risk() = %q, want high-risk-write", got)
	}
}

func TestCrossPlatformCoverageLiveMountAcceptsPipedWriteConfirmation(t *testing.T) {
	var stderr bytes.Buffer
	called := false
	s := Shortcut{
		Service: "chat",
		Command: "+messages-send",
		Risk:    RiskWrite,
		Execute: func(*RuntimeContext) error {
			called = true
			return nil
		},
	}

	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.AddCommand(mount(s))
	root.SetArgs([]string{s.Command})
	root.SetIn(strings.NewReader("yes\n"))
	root.SetErr(&stderr)
	if err := root.Execute(); err != nil {
		t.Fatalf("static write risk was not confirmed: %v", err)
	}
	if !called {
		t.Fatal("confirmed write did not execute")
	}
	// A pipe is not a terminal: the prompt must be suppressed (the structured
	// error carries the semantics for non-interactive callers), while the
	// piped answer is still honored.
	if got := stderr.String(); got != "" {
		t.Fatalf("non-terminal stdin must not print a prompt, got %q", got)
	}
}

func TestCrossPlatformCoverageMountRegistersFlagsAndUse(t *testing.T) {
	s := Shortcut{
		Service:     "contact",
		Command:     "+search-user",
		Description: "search",
		Hidden:      true,
		Tips:        []string{"dws contact +search-user --query name"},
		Flags: []Flag{
			{Name: "query", Type: FlagString, Required: true, Enum: []string{"name", "mobile"}},
			{Name: "limit", Type: FlagInt, Default: "20"},
			{Name: "verbose", Type: FlagBool},
			{Name: "ids", Type: FlagStringSlice, Default: "self,team"},
			{Name: "mode", Type: FlagString},
			{Name: "start", Type: FlagString},
			{Name: "end", Type: FlagString},
			{Name: "legacy", Type: FlagString, Hidden: true},
		},
		Constraints: []Constraint{
			{
				Kind:  ConstraintExactlyOne,
				Flags: []string{"query", "ids"},
			},
			{
				Kind:  ConstraintAtLeastOne,
				Flags: []string{"mode", "start"},
			},
			{
				Kind:  ConstraintMutuallyExclusive,
				Flags: []string{"start", "end"},
			},
		},
	}
	cmd := mount(s)
	if cmd.Use != "+search-user" {
		t.Fatalf("Use = %q, want +search-user", cmd.Use)
	}
	if !cmd.Hidden || cmd.Example != "  dws contact +search-user --query name" {
		t.Fatalf("hidden/example = %v/%q", cmd.Hidden, cmd.Example)
	}
	for _, name := range []string{"query", "limit", "verbose", "ids"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("flag --%s not registered", name)
		}
	}
	if v, _ := cmd.Flags().GetInt("limit"); v != 20 {
		t.Errorf("limit default = %d, want 20", v)
	}
	if v, _ := cmd.Flags().GetStringSlice("ids"); strings.Join(v, ",") != "self,team" {
		t.Errorf("ids default = %#v, want [self team]", v)
	}
	if hidden := cmd.Flags().Lookup("legacy"); hidden == nil || !hidden.Hidden {
		t.Fatalf("hidden flag = %#v", hidden)
	}
	query := cmd.Flags().Lookup("query")
	if got := query.Annotations["dws.schema.required"]; len(got) != 1 || got[0] != "true" {
		t.Fatalf("required annotation = %#v", got)
	}
	if got := query.Annotations["x-cli-enum"]; strings.Join(got, ",") != "name,mobile" {
		t.Fatalf("enum annotation = %#v", got)
	}
	if raw := cmd.Annotations["dws.schema.constraints"]; raw != `{"mutually_exclusive":[["query","ids"],["start","end"]],"require_one_of":[["query","ids"],["mode","start"]]}` {
		t.Fatalf("constraints annotation = %q", raw)
	}
}

func TestCrossPlatformCoverageSchemaConstraintKeepsHiddenSiblingGroups(t *testing.T) {
	cmd := mount(Shortcut{
		Service: "chat",
		Command: "+search",
		Flags: []Flag{
			{Name: "query", Type: FlagString},
			{Name: "keyword", Type: FlagString, Hidden: true},
			{Name: "id", Type: FlagString},
			{Name: "legacy-id", Type: FlagString, Hidden: true},
		},
		Constraints: []Constraint{
			{
				Kind:  ConstraintAtLeastOne,
				Flags: []string{"query", "keyword"},
			},
			{
				Kind:  ConstraintExactlyOne,
				Flags: []string{"id", "legacy-id"},
			},
		},
	})
	// declare≡execute: hidden siblings still satisfy ValidateConstraints, so Schema
	// must project the full group instead of collapsing the visible flag to required.
	query := cmd.Flags().Lookup("query")
	if got := query.Annotations["dws.schema.required"]; len(got) > 0 {
		t.Fatalf("visible flag must not collapse to required with hidden sibling: %#v", got)
	}
	id := cmd.Flags().Lookup("id")
	if got := id.Annotations["dws.schema.required"]; len(got) > 0 {
		t.Fatalf("visible exact-one flag must not collapse to required with hidden sibling: %#v", got)
	}
	want := `{"mutually_exclusive":[["id","legacy-id"]],"require_one_of":[["query","keyword"],["id","legacy-id"]]}`
	if raw := cmd.Annotations["dws.schema.constraints"]; raw != want {
		t.Fatalf("constraints annotation = %q, want %q", raw, want)
	}
}

func TestCrossPlatformCoverageAliasAndAIMessageTagHelpers(t *testing.T) {
	aiTag := AIMessageTagFlag()
	if aiTag.Name != "ai-tag" || aiTag.Type != FlagBool || aiTag.Default != "true" {
		t.Fatalf("AIMessageTagFlag() = %#v", aiTag)
	}

	s := Shortcut{
		Service: "chat",
		Command: "+compat",
		Flags: []Flag{
			{Name: "query", Type: FlagString},
			{Name: "keyword", Type: FlagString},
			{Name: "limit", Type: FlagInt, Default: "20"},
			{Name: "size", Type: FlagInt},
			aiTag,
		},
	}
	cmd := mount(s)
	rt := &RuntimeContext{cmd: cmd, shortcut: s}

	if err := cmd.Flags().Set("keyword", "树莓派"); err != nil {
		t.Fatal(err)
	}
	if got := rt.StrFirst("query", "keyword"); got != "树莓派" {
		t.Fatalf("StrFirst() = %q, want 树莓派", got)
	}
	if got := rt.IntFirst("limit", "size"); got != 20 {
		t.Fatalf("IntFirst() default = %d, want 20", got)
	}
	if err := cmd.Flags().Set("size", "7"); err != nil {
		t.Fatal(err)
	}
	if got := rt.IntFirst("limit", "size"); got != 7 {
		t.Fatalf("IntFirst() alias = %d, want 7", got)
	}

	params := rt.AddAIMessageTag(nil)
	if got := params["clawType"]; got != edition.ClawType() {
		t.Fatalf("clawType = %#v, want %q", got, edition.ClawType())
	}
	if err := cmd.Flags().Set("ai-tag", "false"); err != nil {
		t.Fatal(err)
	}
	params = rt.AddAIMessageTag(map[string]any{"content": "hello"})
	if _, ok := params["clawType"]; ok {
		t.Fatalf("clawType unexpectedly present with --ai-tag=false: %#v", params)
	}
}

func TestCrossPlatformCoverageValidateFlagsRequiredAndEnum(t *testing.T) {
	s := Shortcut{
		Service: "contact",
		Command: "+x",
		Flags: []Flag{
			{Name: "query", Required: true},
			{Name: "order", Enum: []string{"asc", "desc"}},
		},
		Execute: noop,
	}
	run := func(args ...string) error {
		cmd := mount(s)
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		cmd.SetArgs(args)
		return cmd.Execute()
	}

	if err := run(); err == nil || err.Error() != "缺少必填参数 --query：" {
		t.Fatalf("missing required error = %v", err)
	}
	if err := run("--query", "张三"); err != nil {
		t.Fatalf("required-only execution failed: %v", err)
	}
	if err := run("--query", "张三", "--order", "sideways"); err == nil ||
		!strings.Contains(err.Error(), `参数 --order 取值 "sideways" 不合法`) {
		t.Fatalf("invalid enum error = %v", err)
	}
	if err := run("--query", "张三", "--order", "asc"); err != nil {
		t.Fatalf("valid enum failed: %v", err)
	}
}

func TestLiveMountPreservesRequiredEnumDeclarationOrder(t *testing.T) {
	s := Shortcut{
		Service: "contact",
		Command: "+validation-order",
		Flags: []Flag{
			{Name: "order", Enum: []string{"asc", "desc"}},
			{Name: "query", Required: true},
		},
		Execute: noop,
	}
	cmd := mount(s)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"--order", "sideways"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), `参数 --order 取值 "sideways" 不合法`) {
		t.Fatalf("validation order changed: %v", err)
	}
}

func TestCrossPlatformCoverageValidateFlagsRejectsEmptyRequiredValuesAndInvalidSliceEnums(t *testing.T) {
	s := Shortcut{
		Service: "minutes",
		Command: "+detail",
		Flags: []Flag{
			{Name: "id", Type: FlagString, Required: true},
			{Name: "artifacts", Type: FlagStringSlice, Enum: []string{"summary", "todos"}},
		},
		Execute: noop,
	}
	run := func(args ...string) error {
		cmd := mount(s)
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		cmd.SetArgs(args)
		return cmd.Execute()
	}

	if err := run("--id", "   "); err == nil ||
		err.Error() != "必填参数 --id 不能为空" {
		t.Fatalf("empty required string error = %v", err)
	}
	if err := run("--id", "task-1", "--artifacts", "summary,unknown"); err == nil ||
		!strings.Contains(err.Error(), `参数 --artifacts 取值 "unknown" 不合法`) {
		t.Fatalf("invalid string-slice enum error = %v", err)
	}
	if err := run("--id", "task-1", "--artifacts", "summary,todos"); err != nil {
		t.Fatalf("valid string-slice enum failed: %v", err)
	}
}

func TestLiveMountRequiredDefaultStillRequiresChanged(t *testing.T) {
	s := Shortcut{
		Service: "contact",
		Command: "+required-default",
		Flags: []Flag{
			{Name: "query", Type: FlagString, Default: "fallback", Desc: "查询词", Required: true},
		},
		Execute: noop,
	}
	run := func(args ...string) error {
		cmd := mount(s)
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		cmd.SetArgs(args)
		return cmd.Execute()
	}
	if err := run(); err == nil ||
		err.Error() != "缺少必填参数 --query：查询词" {
		t.Fatalf("registration default satisfied explicit requirement: %v", err)
	}
	if err := run("--query", "fallback"); err != nil {
		t.Fatalf("explicit required default failed: %v", err)
	}
}

func TestCrossPlatformCoverageDeclarativeConstraintsRejectEmptyAndConflictingValues(t *testing.T) {
	s := Shortcut{
		Service: "chat",
		Command: "+messages",
		Flags: []Flag{
			{Name: "group", Type: FlagString},
			{Name: "user", Type: FlagString},
		},
		Constraints: []Constraint{
			{Kind: ConstraintExactlyOne, Flags: []string{"group", "user"}},
		},
		Execute: noop,
	}
	run := func(args ...string) error {
		cmd := mount(s)
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		cmd.SetArgs(args)
		return cmd.Execute()
	}

	if err := run(); err == nil || err.Error() != "请指定 --group、--user 之一" {
		t.Fatalf("missing exactly-one error = %v", err)
	}
	if err := run("--group", " "); err == nil ||
		err.Error() != "请指定 --group、--user 之一" {
		t.Fatalf("empty exactly-one error = %v", err)
	}
	if err := run("--group", "cid-1"); err != nil {
		t.Fatalf("one non-empty flag should pass: %v", err)
	}
	if err := run("--group", "cid-1", "--user", "user-1"); err == nil ||
		!strings.Contains(err.Error(), "只能指定其一") {
		t.Fatalf("two flags error = %v", err)
	}
}

func TestLiveMountCustomConstraintRunsShortcutValidate(t *testing.T) {
	validated := false
	executed := false
	s := Shortcut{
		Service: "aitable",
		Command: "+upload",
		Flags: []Flag{
			{Name: "file-size", Type: FlagInt, Required: true},
		},
		Constraints: []Constraint{{
			Kind:        ConstraintCustom,
			Flags:       []string{"file-size"},
			Description: "--file-size 必须大于 0",
		}},
		Validate: func(rt *RuntimeContext) error {
			validated = true
			if rt.Int("file-size") <= 0 {
				return apperrors.NewValidation("--file-size 必须大于 0")
			}
			return nil
		},
		Execute: func(*RuntimeContext) error {
			executed = true
			return nil
		},
	}

	cmd := mount(s)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"--file-size", "0"})
	err := cmd.Execute()
	if err == nil || err.Error() != "--file-size 必须大于 0" {
		t.Fatalf("custom validation error = %v", err)
	}
	if !validated || executed {
		t.Fatalf("validated/executed = %v/%v", validated, executed)
	}
	if !strings.Contains(cmd.Long, "--file-size 必须大于 0") {
		t.Fatalf("custom constraint missing from help: %q", cmd.Long)
	}
}

func TestCrossPlatformCoverageMountHelpPublishesRequiredEnumsAndConstraints(t *testing.T) {
	s := Shortcut{
		Service:     "chat",
		Command:     "+messages",
		Description: "messages",
		Intent:      "读取群聊或单聊消息。",
		Flags: []Flag{
			{Name: "group", Type: FlagString, Required: true, Desc: "群 ID"},
			{Name: "user", Type: FlagString, Desc: "用户 ID"},
			{Name: "direction", Type: FlagString, Enum: []string{"newer", "older"}, Desc: "方向"},
		},
		Constraints: []Constraint{
			{Kind: ConstraintExactlyOne, Flags: []string{"group", "user"}},
		},
	}
	cmd := mount(s)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Help(); err != nil {
		t.Fatal(err)
	}
	help := out.String()
	for _, want := range []string{
		"参数约束", "--group、--user 必须且只能指定一个", "群 ID（必填）", "可选值: newer, older",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("help missing %q:\n%s", want, help)
		}
	}
}

func TestCrossPlatformCoverageBuildGroupsByService(t *testing.T) {
	shortcuts := []Shortcut{
		{Service: "contact", Command: "+a", Execute: noop},
		{Service: "contact", Command: "+b", Execute: noop},
		{Service: "calendar", Command: "+c", Execute: noop},
	}
	cmds := build(shortcuts)
	if len(cmds) != 2 {
		t.Fatalf("got %d service commands, want 2", len(cmds))
	}
	byName := map[string]*cobra.Command{}
	for _, c := range cmds {
		byName[c.Use] = c
	}
	if got := len(byName["contact"].Commands()); got != 2 {
		t.Errorf("contact has %d leaves, want 2", got)
	}
	if got := len(byName["calendar"].Commands()); got != 1 {
		t.Errorf("calendar has %d leaves, want 1", got)
	}
}

func TestBuiltInCommandsExcludeUserDefinedShortcuts(t *testing.T) {
	testseam.Swap(t, &allShortcuts, []Shortcut(nil))
	Register(
		Shortcut{Service: "calendar", Command: "+builtin", Execute: noop},
		Shortcut{Service: "calendar", Command: "+user", UserDefined: true, Execute: noop},
	)

	all := Commands()
	if len(all) != 1 || len(all[0].Commands()) != 2 {
		t.Fatalf("all shortcut commands = %#v", all)
	}
	builtins := BuiltInCommands()
	if len(builtins) != 1 || len(builtins[0].Commands()) != 1 ||
		builtins[0].Commands()[0].Name() != "+builtin" {
		t.Fatalf("built-in shortcut commands = %#v", builtins)
	}
}

func noop(_ *RuntimeContext) error { return nil }

func TestLiveMountEOFRequiresConfirmation(t *testing.T) {
	called := false
	s := Shortcut{
		Service: "chat", Command: "+send", Risk: RiskWrite,
		Execute: func(*RuntimeContext) error {
			called = true
			return nil
		},
	}
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	cmd := mount(s)
	root.AddCommand(cmd)
	root.SetArgs([]string{s.Command})
	root.SetIn(strings.NewReader(""))
	root.SetErr(&bytes.Buffer{})

	err := root.Execute()
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || appErr.Reason != "confirmation_required" {
		t.Fatalf("EOF err = %#v, want confirmation_required", err)
	}
	if called {
		t.Fatal("EOF must not execute")
	}
}

func TestLiveMountExplicitSafetyDrivesRuntimeAndContractFinal(t *testing.T) {
	called := false
	explicit := contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	}
	s := Shortcut{
		Service: "chat", Command: "+send", Risk: RiskRead,
		Safety: explicit,
		Contract: corecmd.ContractDecl{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "create_thing",
				CanonicalPath:  "dev.create_thing",
				CLIPath:        "dev create",
				PrimaryCLIPath: "dev create",
			},

			Description: "发送消息",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "chat", RPCName: "send_message"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "发送消息",
				UseWhen:      []string{"需要发送消息时"},
				AvoidWhen:    []string{"只需读取消息时"},
				Examples:     []string{"dws chat +send"},
			},
		},
		Execute: func(*RuntimeContext) error {
			called = true
			return nil
		},
	}

	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	cmd := mount(s)
	root.AddCommand(cmd)

	final, ok := contractfinal.RuntimeContractFinal(cmd)
	if !ok || final.Safety == nil {
		t.Fatal("mounted Shortcut must publish ContractFinal Safety")
	}
	if got := *final.Safety; got.Effect != explicit.Effect || got.Risk != explicit.Risk ||
		got.Confirmation != explicit.Confirmation || got.Idempotency != explicit.Idempotency {
		t.Fatalf("ContractFinal Safety = %#v, want explicit %#v", got, explicit)
	}

	root.SetArgs([]string{s.Command})
	root.SetIn(strings.NewReader(""))
	root.SetErr(&bytes.Buffer{})
	err := root.Execute()
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || appErr.Reason != "confirmation_required" {
		t.Fatalf("explicit Safety must drive runtime confirmation; err = %#v", err)
	}
	if called {
		t.Fatal("explicit Safety confirmation gate must run before Execute")
	}
}

func TestLiveMountInteractiveDeclineReturnsCancelError(t *testing.T) {
	called := false
	s := Shortcut{
		Service: "chat", Command: "+send", Risk: RiskWrite,
		Execute: func(*RuntimeContext) error {
			called = true
			return nil
		},
	}
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	cmd := mount(s)
	root.AddCommand(cmd)
	root.SetArgs([]string{s.Command})
	root.SetIn(strings.NewReader("no\n"))
	root.SetErr(&bytes.Buffer{})

	err := root.Execute()
	if err == nil || err.Error() != "用户取消了操作" {
		t.Fatalf("interactive decline error = %v", err)
	}
	if called {
		t.Fatal("interactive decline must not execute")
	}
}

func TestLiveMountYesBypassesPrompt(t *testing.T) {
	called := false
	s := Shortcut{
		Service: "chat", Command: "+send", Risk: RiskHighWrite,
		Execute: func(*RuntimeContext) error {
			called = true
			return nil
		},
	}
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	cmd := mount(s)
	root.AddCommand(cmd)
	root.SetArgs([]string{s.Command, "--yes"})
	root.SetIn(strings.NewReader("")) // would be unavailable without --yes
	if err := root.Execute(); err != nil {
		t.Fatalf("--yes must proceed: %v", err)
	}
	if !called {
		t.Fatal("--yes did not execute")
	}
}

func TestLiveMountDryRunBypassesPrompt(t *testing.T) {
	called := false
	s := Shortcut{
		Service: "chat", Command: "+send", Risk: RiskWrite,
		Execute: func(rt *RuntimeContext) error {
			called = rt.DryRun()
			return nil
		},
	}
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.AddCommand(mount(s))
	root.SetArgs([]string{s.Command, "--dry-run"})
	root.SetIn(strings.NewReader(""))
	if err := root.Execute(); err != nil {
		t.Fatalf("--dry-run must proceed without confirmation: %v", err)
	}
	if !called {
		t.Fatal("--dry-run did not reach Execute with dry-run state")
	}
}

func TestCrossPlatformCoverageCallMCPWriteDataRejectsDryRun(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().Bool("dry-run", false, "")
	cmd := &cobra.Command{Use: "x"}
	root.AddCommand(cmd)
	if err := root.PersistentFlags().Set("dry-run", "true"); err != nil {
		t.Fatalf("set dry-run: %v", err)
	}

	rt := &RuntimeContext{cmd: cmd, shortcut: Shortcut{Service: "calendar"}}
	for name, call := range map[string]func(string, string, map[string]any) (map[string]any, error){
		"legacy": rt.CallMCPWriteData,
		"strict": rt.CallMCPWriteDataStrict,
	} {
		_, err := call("calendar", "create_calendar_event", map[string]any{"summary": "x"})
		if err == nil {
			t.Fatalf("%s: expected dry-run write guard error", name)
		}
		if !strings.Contains(err.Error(), "calendar/create_calendar_event") {
			t.Fatalf("%s: error = %q, want tool name", name, err.Error())
		}
	}
}

func TestCrossPlatformCoverageCallMCPDataDryRunReadNameFailsClosed(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().Bool("dry-run", false, "")
	cmd := &cobra.Command{Use: "x"}
	root.AddCommand(cmd)
	if err := root.PersistentFlags().Set("dry-run", "true"); err != nil {
		t.Fatalf("set dry-run: %v", err)
	}

	rt := &RuntimeContext{cmd: cmd, shortcut: Shortcut{Service: "chat"}}
	for _, tool := range []string{"send_personal_message", "custom_operation"} {
		_, err := rt.CallMCPData("chat", tool, map[string]any{"receiverUid": "u"})
		if err == nil {
			t.Fatalf("%s: expected dry-run read allowlist error", tool)
		}
		if !strings.Contains(err.Error(), "chat/"+tool) {
			t.Fatalf("%s: error = %q, want tool name", tool, err.Error())
		}
	}

	for _, tool := range []string{
		"get_conversation_info",
		"list_topic_replies",
		"query_busy_status",
		"search_groups",
		"unread_message_conversation_list",
	} {
		if !looksReadTool(tool) {
			t.Errorf("%s: expected read-only classification", tool)
		}
	}
}

func TestCrossPlatformCoverageValidationHelpers(t *testing.T) {
	s := Shortcut{
		Service: "x", Command: "+y",
		Flags: []Flag{
			{Name: "a"}, {Name: "b"}, {Name: "c"},
			{Name: "n", Type: FlagInt},
		},
	}
	cmd := mount(s)
	rt := &RuntimeContext{cmd: cmd, shortcut: s}

	// nothing set
	if err := rt.MutuallyExclusive("a", "b"); err != nil {
		t.Errorf("MutuallyExclusive with none set should pass: %v", err)
	}
	if err := rt.AtLeastOne("a", "b"); err == nil {
		t.Error("AtLeastOne with none set should fail")
	}
	if err := rt.ExactlyOne("a", "b"); err == nil {
		t.Error("ExactlyOne with none set should fail")
	}

	_ = cmd.Flags().Set("a", "1")
	if err := rt.MutuallyExclusive("a", "b"); err != nil {
		t.Errorf("MutuallyExclusive with one set should pass: %v", err)
	}
	if err := rt.AtLeastOne("a", "b"); err != nil {
		t.Errorf("AtLeastOne with one set should pass: %v", err)
	}
	if err := rt.ExactlyOne("a", "b"); err != nil {
		t.Errorf("ExactlyOne with one set should pass: %v", err)
	}

	_ = cmd.Flags().Set("b", "1")
	if err := rt.MutuallyExclusive("a", "b"); err == nil {
		t.Error("MutuallyExclusive with two set should fail")
	}
	if err := rt.ExactlyOne("a", "b"); err == nil {
		t.Error("ExactlyOne with two set should fail")
	}

	// RangeInt
	_ = cmd.Flags().Set("n", "50")
	if err := rt.RangeInt("n", 1, 30); err == nil {
		t.Error("RangeInt 50 not in [1,30] should fail")
	}
	_ = cmd.Flags().Set("n", "20")
	if err := rt.RangeInt("n", 1, 30); err != nil {
		t.Errorf("RangeInt 20 in [1,30] should pass: %v", err)
	}

	// RequireAll
	if err := rt.RequireAll("a", "c"); err == nil {
		t.Error("RequireAll with c unset should fail")
	}
	_ = cmd.Flags().Set("c", "1")
	if err := rt.RequireAll("a", "c"); err != nil {
		t.Errorf("RequireAll with both set should pass: %v", err)
	}
}
