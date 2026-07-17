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
	"strings"
	"testing"

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

func TestCrossPlatformCoverageMountRegistersFlagsAndUse(t *testing.T) {
	s := Shortcut{
		Service:     "contact",
		Command:     "+search-user",
		Description: "search",
		Flags: []Flag{
			{Name: "query", Type: FlagString, Required: true},
			{Name: "limit", Type: FlagInt, Default: "20"},
			{Name: "verbose", Type: FlagBool},
			{Name: "ids", Type: FlagStringSlice},
		},
	}
	cmd := mount(s)
	if cmd.Use != "+search-user" {
		t.Fatalf("Use = %q, want +search-user", cmd.Use)
	}
	for _, name := range []string{"query", "limit", "verbose", "ids"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("flag --%s not registered", name)
		}
	}
	if v, _ := cmd.Flags().GetInt("limit"); v != 20 {
		t.Errorf("limit default = %d, want 20", v)
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
	}
	cmd := mount(s)

	// Missing required flag → error.
	rt := &RuntimeContext{cmd: cmd, shortcut: s}
	if err := validateFlags(rt, s); err == nil {
		t.Fatal("expected error for missing required --query")
	}

	// Set required, leave enum unset → ok.
	_ = cmd.Flags().Set("query", "张三")
	if err := validateFlags(rt, s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Invalid enum → error.
	_ = cmd.Flags().Set("order", "sideways")
	if err := validateFlags(rt, s); err == nil {
		t.Fatal("expected error for invalid --order enum")
	}

	// Valid enum → ok.
	_ = cmd.Flags().Set("order", "asc")
	if err := validateFlags(rt, s); err != nil {
		t.Fatalf("unexpected error: %v", err)
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
	}
	cmd := mount(s)
	rt := &RuntimeContext{cmd: cmd, shortcut: s}

	_ = cmd.Flags().Set("id", "   ")
	if err := validateFlags(rt, s); err == nil {
		t.Fatal("expected empty required string to fail")
	}
	_ = cmd.Flags().Set("id", "task-1")
	_ = cmd.Flags().Set("artifacts", "summary,unknown")
	if err := validateFlags(rt, s); err == nil {
		t.Fatal("expected invalid string-slice enum value to fail")
	}
	cmd = mount(s)
	rt = &RuntimeContext{cmd: cmd, shortcut: s}
	_ = cmd.Flags().Set("id", "task-1")
	_ = cmd.Flags().Set("artifacts", "summary,todos")
	if err := validateFlags(rt, s); err != nil {
		t.Fatalf("valid string-slice enum failed: %v", err)
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
	}
	cmd := mount(s)
	rt := &RuntimeContext{cmd: cmd, shortcut: s}
	if err := validateConstraints(rt, s); err == nil {
		t.Fatal("expected missing exactly-one flags to fail")
	}
	_ = cmd.Flags().Set("group", " ")
	if err := validateConstraints(rt, s); err == nil {
		t.Fatal("empty flag must not satisfy exactly-one")
	}
	_ = cmd.Flags().Set("group", "cid-1")
	if err := validateConstraints(rt, s); err != nil {
		t.Fatalf("one non-empty flag should pass: %v", err)
	}
	_ = cmd.Flags().Set("user", "user-1")
	if err := validateConstraints(rt, s); err == nil {
		t.Fatal("two flags must violate exactly-one")
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

func noop(_ *RuntimeContext) error { return nil }

func TestCrossPlatformCoverageCallMCPWriteDataRejectsDryRun(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().Bool("dry-run", false, "")
	cmd := &cobra.Command{Use: "x"}
	root.AddCommand(cmd)
	if err := root.PersistentFlags().Set("dry-run", "true"); err != nil {
		t.Fatalf("set dry-run: %v", err)
	}

	rt := &RuntimeContext{cmd: cmd, shortcut: Shortcut{Service: "calendar"}}
	_, err := rt.CallMCPWriteData("calendar", "create_calendar_event", map[string]any{"summary": "x"})
	if err == nil {
		t.Fatal("expected dry-run write guard error")
	}
	if !strings.Contains(err.Error(), "calendar/create_calendar_event") {
		t.Fatalf("error = %q, want tool name", err.Error())
	}
}

func TestCrossPlatformCoverageCallMCPDataRejectsLikelyWriteUnderDryRun(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().Bool("dry-run", false, "")
	cmd := &cobra.Command{Use: "x"}
	root.AddCommand(cmd)
	if err := root.PersistentFlags().Set("dry-run", "true"); err != nil {
		t.Fatalf("set dry-run: %v", err)
	}

	rt := &RuntimeContext{cmd: cmd, shortcut: Shortcut{Service: "chat"}}
	_, err := rt.CallMCPData("chat", "send_personal_message", map[string]any{"receiverUid": "u"})
	if err == nil {
		t.Fatal("expected likely write guard error")
	}
	if !strings.Contains(err.Error(), "chat/send_personal_message") {
		t.Fatalf("error = %q, want tool name", err.Error())
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
