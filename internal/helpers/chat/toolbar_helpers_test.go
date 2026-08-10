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

package chat

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type toolbarCall struct {
	product string
	tool    string
	args    map[string]any
}

type toolbarTestCaller struct {
	product string
	tool    string
	args    map[string]any
	err     error
	calls   []toolbarCall
}

func (c *toolbarTestCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, toolbarCall{product: productID, tool: toolName, args: args})
	c.product = productID
	c.tool = toolName
	c.args = args
	if c.err != nil {
		return nil, c.err
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: "{}"}}}, nil
}

func (c *toolbarTestCaller) Format() string { return "json" }
func (c *toolbarTestCaller) DryRun() bool   { return false }
func (c *toolbarTestCaller) Fields() string { return "" }
func (c *toolbarTestCaller) JQ() string     { return "" }

func executeToolbarCommand(t *testing.T, cmd *cobra.Command, caller *toolbarTestCaller, args ...string) error {
	t.Helper()
	initToolbarDepsForTest(t, caller)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	return cmd.Execute()
}

func initToolbarDepsForTest(t *testing.T, caller *toolbarTestCaller) {
	t.Helper()
	previous := deps
	SetDeps(Deps{
		GroupRunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
		CallMCPToolOnServer: func(productID, toolName string, args map[string]any) error {
			_, err := caller.CallTool(context.Background(), productID, toolName, args)
			return err
		},
		DeclareLeafMetadata: func(cmd *cobra.Command, spec LeafSpec) *cobra.Command {
			return cmd
		},
		ValidateRequiredFlag: validateRequiredFlagsLocal,
	})
	t.Cleanup(func() {
		deps = previous
	})
}

func requireTypedConfirmationError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected confirmation error, got nil")
	}
	if !strings.Contains(err.Error(), "获得用户确认后加 --yes 执行") {
		t.Fatalf("expected confirmation error, got %v", err)
	}
}

func requireToolbarCall(
	t *testing.T,
	caller *toolbarTestCaller,
	wantProduct, wantTool string,
	wantArgs map[string]any,
) {
	t.Helper()
	if caller.product != wantProduct || caller.tool != wantTool {
		t.Fatalf("tool call = %s/%s, want %s/%s", caller.product, caller.tool, wantProduct, wantTool)
	}
	if !reflect.DeepEqual(caller.args, wantArgs) {
		t.Fatalf("tool args = %#v, want %#v", caller.args, wantArgs)
	}
}

func toolbarCustomArgs(extra ...string) []string {
	args := []string{"--conversation-id", "cid", "--title", "Title", "--url", "https://example.com/mobile", "--icon-url", "https://example.com/icon.png", "--pc-url", "https://example.com/pc"}
	return append(args, extra...)
}

func TestHasIntersection(t *testing.T) {
	tests := []struct {
		name string
		a, b []int64
		want bool
	}{
		{"both empty", nil, nil, false},
		{"a empty", nil, []int64{1}, false},
		{"b empty", []int64{1}, nil, false},
		{"no overlap", []int64{1, 2, 3}, []int64{4, 5, 6}, false},
		{"has overlap", []int64{1, 2, 3}, []int64{3, 4, 5}, true},
		{"single match", []int64{1}, []int64{1}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasIntersection(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("hasIntersection(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestIsSystemBusy(t *testing.T) {
	if isSystemBusy(nil) {
		t.Error("isSystemBusy(nil) should be false")
	}
	if !isSystemBusy(errors.New("error code: SYSTEM_BUSY")) {
		t.Error("isSystemBusy should detect SYSTEM_BUSY in error message")
	}
	if isSystemBusy(errors.New("some other error")) {
		t.Error("isSystemBusy should return false for non-SYSTEM_BUSY errors")
	}
}

func TestParseExtension(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().StringArray("extension", nil, "")

	ext, err := parseExtension(cmd)
	if err != nil {
		t.Fatalf("empty extension should not error: %v", err)
	}
	if ext != nil {
		t.Fatalf("empty extension should return nil, got %v", ext)
	}

	_ = cmd.Flags().Set("extension", "key1=val1")
	_ = cmd.Flags().Set("extension", "key2=val2")
	ext, err = parseExtension(cmd)
	if err != nil {
		t.Fatalf("valid extension should not error: %v", err)
	}
	if ext["key1"] != "val1" || ext["key2"] != "val2" {
		t.Fatalf("unexpected extension map: %v", ext)
	}

	cmd2 := &cobra.Command{Use: "test2"}
	cmd2.Flags().StringArray("extension", nil, "")
	_ = cmd2.Flags().Set("extension", "badformat")
	_, err = parseExtension(cmd2)
	if err == nil {
		t.Fatal("invalid extension format should error")
	}

	cmd3 := &cobra.Command{Use: "test3"}
	cmd3.Flags().StringArray("extension", nil, "")
	_ = cmd3.Flags().Set("extension", "color=blue")
	_ = cmd3.Flags().Set("extension", "color=red")
	_, err = parseExtension(cmd3)
	if err == nil {
		t.Fatal("duplicate extension keys should error")
	}

	cmd4 := &cobra.Command{Use: "test4"}
	cmd4.Flags().StringArray("extension", nil, "")
	_ = cmd4.Flags().Set("extension", " =value")
	_, err = parseExtension(cmd4)
	if err == nil {
		t.Fatal("blank extension key should error")
	}
}

func TestToolbarConversationID(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("conversation-id", "", "")

	_, err := toolbarConversationID(cmd)
	if err == nil {
		t.Fatal("missing conversation-id should error")
	}

	_ = cmd.Flags().Set("conversation-id", "cid123")
	cid, err := toolbarConversationID(cmd)
	if err != nil {
		t.Fatalf("valid conversation-id should not error: %v", err)
	}
	if cid != "cid123" {
		t.Fatalf("expected cid123, got %s", cid)
	}
}

func TestToolbarDepsDefaultWrappersAndValidationEdges(t *testing.T) {
	cmd := &cobra.Command{Use: "toolbar"}
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := groupRunE(cmd, nil); err != nil {
		t.Fatalf("default groupRunE returned error: %v", err)
	}
	if err := callMCPToolOnServer("im", "tool", nil); err == nil ||
		!strings.Contains(err.Error(), "CallMCPToolOnServer is not initialized") {
		t.Fatalf("default callMCPToolOnServer error = %v", err)
	}
	if got := DeclareLeafMetadata(cmd, LeafSpec{}); got != cmd {
		t.Fatal("default DeclareLeafMetadata should return the original command")
	}

	toolbar := NewChatToolbarCommand()
	if toolbar.Name() != "toolbar" || len(toolbar.Commands()) != 7 {
		t.Fatalf("toolbar command = %q with %d children, want toolbar with 7 children", toolbar.Name(), len(toolbar.Commands()))
	}

	parent := &cobra.Command{Use: "parent"}
	parent.PersistentFlags().String("conversation-id", "", "")
	child := &cobra.Command{Use: "child"}
	parent.AddCommand(child)
	_ = parent.PersistentFlags().Set("conversation-id", " inherited ")
	if got := mustGetFlag(child, "conversation-id"); got != "inherited" {
		t.Fatalf("mustGetFlag inherited value = %q, want inherited", got)
	}

	if _, err := parseCSVInt64(" , "); err == nil || !strings.Contains(err.Error(), "at least one ID") {
		t.Fatalf("empty csv error = %v", err)
	}
	if _, err := parseCSVInt64("1,bad"); err == nil || !strings.Contains(err.Error(), `invalid integer "bad"`) {
		t.Fatalf("invalid csv error = %v", err)
	}
	required := &cobra.Command{Use: "required"}
	required.Flags().String("title", "", "")
	if err := validateRequiredFlagsLocal(required, "title"); err == nil ||
		!strings.Contains(err.Error(), "flag --title is required") {
		t.Fatalf("validateRequiredFlagsLocal error = %v", err)
	}
	if commandBoolFlag(nil, "yes") {
		t.Fatal("commandBoolFlag(nil) should be false")
	}
	boolParent := &cobra.Command{Use: "parent"}
	boolParent.PersistentFlags().Bool("yes", false, "")
	boolChild := &cobra.Command{Use: "child"}
	boolParent.AddCommand(boolChild)
	_ = boolParent.PersistentFlags().Set("yes", "true")
	if !commandBoolFlag(boolChild, "yes") {
		t.Fatal("commandBoolFlag should read inherited true value")
	}
}

func TestToolbarCommandsValidationEdges(t *testing.T) {
	tests := []struct {
		name      string
		cmd       func() *cobra.Command
		args      []string
		wantError string
	}{
		{
			name:      "add blank conversation id",
			cmd:       newToolbarAddCommand,
			args:      []string{"--conversation-id", " ", "--shortcut-ids", "1"},
			wantError: "flag --conversation-id is required",
		},
		{
			name:      "add blank shortcut ids",
			cmd:       newToolbarAddCommand,
			args:      []string{"--conversation-id", "cid", "--shortcut-ids", " "},
			wantError: "flag --shortcut-ids is required",
		},
		{
			name:      "add invalid shortcut ids",
			cmd:       newToolbarAddCommand,
			args:      []string{"--conversation-id", "cid", "--shortcut-ids", "1,bad"},
			wantError: "--shortcut-ids",
		},
		{
			name:      "hide blank conversation id",
			cmd:       newToolbarHideCommand,
			args:      []string{"--conversation-id", " ", "--shortcut-ids", "1"},
			wantError: "flag --conversation-id is required",
		},
		{
			name:      "hide blank shortcut ids",
			cmd:       newToolbarHideCommand,
			args:      []string{"--conversation-id", "cid", "--shortcut-ids", " "},
			wantError: "flag --shortcut-ids is required",
		},
		{
			name:      "hide invalid shortcut ids",
			cmd:       newToolbarHideCommand,
			args:      []string{"--conversation-id", "cid", "--shortcut-ids", "1,bad"},
			wantError: "--shortcut-ids",
		},
		{
			name:      "list blank conversation id",
			cmd:       newToolbarListCommand,
			args:      []string{"--conversation-id", " "},
			wantError: "flag --conversation-id is required",
		},
		{
			name:      "sort blank conversation id",
			cmd:       newToolbarSortCommand,
			args:      []string{"--conversation-id", " ", "--sorted-ids", "1"},
			wantError: "flag --conversation-id is required",
		},
		{
			name:      "sort blank sorted ids",
			cmd:       newToolbarSortCommand,
			args:      []string{"--conversation-id", "cid", "--sorted-ids", " "},
			wantError: "flag --sorted-ids is required",
		},
		{
			name:      "sort invalid sorted ids",
			cmd:       newToolbarSortCommand,
			args:      []string{"--conversation-id", "cid", "--sorted-ids", "1,bad"},
			wantError: "--sorted-ids",
		},
		{
			name:      "sort invalid unsorted ids",
			cmd:       newToolbarSortCommand,
			args:      []string{"--conversation-id", "cid", "--sorted-ids", "1", "--unsorted-ids", "bad"},
			wantError: "--unsorted-ids",
		},
		{
			name:      "create blank conversation id",
			cmd:       newToolbarCreateCustomCommand,
			args:      toolbarCustomArgs("--conversation-id", " "),
			wantError: "flag --conversation-id is required",
		},
		{
			name:      "create blank title",
			cmd:       newToolbarCreateCustomCommand,
			args:      toolbarCustomArgs("--title", " "),
			wantError: "flag --title is required",
		},
		{
			name:      "create invalid extension",
			cmd:       newToolbarCreateCustomCommand,
			args:      toolbarCustomArgs("--extension", "bad"),
			wantError: "--extension",
		},
		{
			name:      "remove blank conversation id",
			cmd:       newToolbarRemoveCustomCommand,
			args:      []string{"--conversation-id", " ", "--shortcut-id", "42", "--yes"},
			wantError: "flag --conversation-id is required",
		},
		{
			name:      "update blank conversation id",
			cmd:       newToolbarUpdateCustomCommand,
			args:      toolbarCustomArgs("--shortcut-id", "42", "--conversation-id", " "),
			wantError: "flag --conversation-id is required",
		},
		{
			name:      "update blank title",
			cmd:       newToolbarUpdateCustomCommand,
			args:      toolbarCustomArgs("--shortcut-id", "42", "--title", " "),
			wantError: "flag --title is required",
		},
		{
			name:      "update invalid extension",
			cmd:       newToolbarUpdateCustomCommand,
			args:      toolbarCustomArgs("--shortcut-id", "42", "--extension", "bad"),
			wantError: "--extension",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := executeToolbarCommand(t, tt.cmd(), &toolbarTestCaller{}, tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantError)
			}
		})
	}
}

func TestToolbarCommandsConvertSystemBusy(t *testing.T) {
	tests := []struct {
		name string
		cmd  func() *cobra.Command
		args []string
	}{
		{
			name: "add",
			cmd:  newToolbarAddCommand,
			args: []string{"--conversation-id", "cid", "--shortcut-ids", "1,2"},
		},
		{
			name: "hide",
			cmd:  newToolbarHideCommand,
			args: []string{"--conversation-id", "cid", "--shortcut-ids", "1,2"},
		},
		{
			name: "sort",
			cmd:  newToolbarSortCommand,
			args: []string{"--conversation-id", "cid", "--sorted-ids", "1,2"},
		},
		{
			name: "remove-custom",
			cmd:  newToolbarRemoveCustomCommand,
			args: []string{"--conversation-id", "cid", "--shortcut-id", "1", "--yes"},
		},
		{
			name: "create-custom",
			cmd:  newToolbarCreateCustomCommand,
			args: toolbarCustomArgs(),
		},
		{
			name: "update-custom",
			cmd:  newToolbarUpdateCustomCommand,
			args: toolbarCustomArgs("--shortcut-id", "99"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := executeToolbarCommand(t, tt.cmd(), &toolbarTestCaller{
				err: errors.New("remote SYSTEM_BUSY"),
			}, tt.args...)
			if err == nil || !strings.Contains(err.Error(), "SYSTEM_BUSY") {
				t.Fatalf("expected system_busy validation error, got %v", err)
			}
		})
	}
}

func TestToolbarCreateAndUpdateCustomIncludeOptionalFields(t *testing.T) {
	createCaller := &toolbarTestCaller{}
	err := executeToolbarCommand(t, newToolbarCreateCustomCommand(), createCaller, toolbarCustomArgs(
		"--desc", "Description",
		"--tag", "Tag",
		"--sort-index", "7",
		"--extension", "color=blue",
	)...)
	if err != nil {
		t.Fatalf("create-custom returned error: %v", err)
	}
	requireToolbarCall(t, createCaller, "im", "create_custom_shortcut", map[string]any{
		"openConversationId": "cid",
		"name":               `{"zh_CN":"Title"}`,
		"url":                "https://example.com/mobile",
		"icons":              `{"iconMobile":"https://example.com/icon.png"}`,
		"pcUrl":              "https://example.com/pc",
		"desc":               `{"zh_CN":"Description"}`,
		"tag":                "Tag",
		"sortIndex":          7,
		"extension":          map[string]string{"color": "blue"},
	})

	updateCaller := &toolbarTestCaller{}
	err = executeToolbarCommand(t, newToolbarUpdateCustomCommand(), updateCaller, toolbarCustomArgs(
		"--shortcut-id", "99",
		"--desc", "Description",
		"--tag", "Tag",
		"--sort-index", "7",
		"--extension", "color=blue",
	)...)
	if err != nil {
		t.Fatalf("update-custom returned error: %v", err)
	}
	requireToolbarCall(t, updateCaller, "im", "update_custom_shortcut", map[string]any{
		"openConversationId": "cid",
		"shortcutId":         int64(99),
		"name":               `{"zh_CN":"Title"}`,
		"url":                "https://example.com/mobile",
		"icons":              `{"iconMobile":"https://example.com/icon.png"}`,
		"pcUrl":              "https://example.com/pc",
		"desc":               `{"zh_CN":"Description"}`,
		"tag":                "Tag",
		"sortIndex":          7,
		"extension":          map[string]string{"color": "blue"},
	})
}

func TestToolbarCreateCustomDefaultsDescToTitle(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing desc", args: toolbarCustomArgs()},
		{name: "blank desc", args: toolbarCustomArgs("--desc", "   ")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &toolbarTestCaller{}
			if err := executeToolbarCommand(t, newToolbarCreateCustomCommand(), caller, tt.args...); err != nil {
				t.Fatalf("create-custom returned error: %v", err)
			}
			requireToolbarCall(t, caller, "im", "create_custom_shortcut", map[string]any{
				"openConversationId": "cid",
				"name":               `{"zh_CN":"Title"}`,
				"url":                "https://example.com/mobile",
				"icons":              `{"iconMobile":"https://example.com/icon.png"}`,
				"pcUrl":              "https://example.com/pc",
				"desc":               `{"zh_CN":"Title"}`,
			})
		})
	}
}

func TestToolbarUpdateCustomDefaultsDescToTitle(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing desc", args: toolbarCustomArgs("--shortcut-id", "99")},
		{name: "blank desc", args: toolbarCustomArgs("--shortcut-id", "99", "--desc", "   ")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &toolbarTestCaller{}
			if err := executeToolbarCommand(t, newToolbarUpdateCustomCommand(), caller, tt.args...); err != nil {
				t.Fatalf("update-custom returned error: %v", err)
			}
			requireToolbarCall(t, caller, "im", "update_custom_shortcut", map[string]any{
				"openConversationId": "cid",
				"shortcutId":         int64(99),
				"name":               `{"zh_CN":"Title"}`,
				"url":                "https://example.com/mobile",
				"icons":              `{"iconMobile":"https://example.com/icon.png"}`,
				"pcUrl":              "https://example.com/pc",
				"desc":               `{"zh_CN":"Title"}`,
			})
		})
	}
}

func TestToolbarSortValidatesOptionalUnsortedIDs(t *testing.T) {
	intersectionErr := executeToolbarCommand(t, newToolbarSortCommand(), &toolbarTestCaller{},
		"--conversation-id", "cid",
		"--sorted-ids", "1,2",
		"--unsorted-ids", "2,3",
	)
	if intersectionErr == nil || !strings.Contains(intersectionErr.Error(), "不能有交集") {
		t.Fatalf("expected id_intersection error, got %v", intersectionErr)
	}

	caller := &toolbarTestCaller{}
	err := executeToolbarCommand(t, newToolbarSortCommand(), caller,
		"--conversation-id", "cid",
		"--sorted-ids", "1,2",
		"--unsorted-ids", "3,4",
	)
	if err != nil {
		t.Fatalf("sort returned error: %v", err)
	}
	requireToolbarCall(t, caller, "im", "sort_shortcut_bar", map[string]any{
		"openConversationId":  "cid",
		"sortedShortcutIds":   []int64{1, 2},
		"unsortedShortcutIds": []int64{3, 4},
	})
}

func TestCrossPlatformCoverageToolbarAddHideAndListCallMCPWithExactArgs(t *testing.T) {
	tests := []struct {
		name     string
		cmd      func() *cobra.Command
		args     []string
		wantTool string
		wantArgs map[string]any
	}{
		{
			name:     "add",
			cmd:      newToolbarAddCommand,
			args:     []string{"--conversation-id", "cidX", "--shortcut-ids", "101,102"},
			wantTool: "add_shortcut_to_bar",
			wantArgs: map[string]any{
				"openConversationId": "cidX",
				"shortcutIds":        []int64{101, 102},
			},
		},
		{
			name:     "hide",
			cmd:      newToolbarHideCommand,
			args:     []string{"--conversation-id", "cidX", "--shortcut-ids", "201,202"},
			wantTool: "hide_shortcut_from_bar",
			wantArgs: map[string]any{
				"openConversationId": "cidX",
				"shortcutIds":        []int64{201, 202},
			},
		},
		{
			name:     "list",
			cmd:      newToolbarListCommand,
			args:     []string{"--conversation-id", "cidX"},
			wantTool: "list_chat_toolbar_shortcuts",
			wantArgs: map[string]any{
				"openConversationId": "cidX",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &toolbarTestCaller{}
			if err := executeToolbarCommand(t, tt.cmd(), caller, tt.args...); err != nil {
				t.Fatalf("%s returned error: %v", tt.name, err)
			}
			requireToolbarCall(t, caller, "im", tt.wantTool, tt.wantArgs)
		})
	}
}

func TestCrossPlatformCoverageToolbarRemoveCustomRejectsWithoutYes(t *testing.T) {
	caller := &toolbarTestCaller{}
	err := executeToolbarCommand(t, newToolbarRemoveCustomCommand(), caller,
		"--conversation-id", "cidX", "--shortcut-id", "42")
	requireTypedConfirmationError(t, err)
	if len(caller.calls) != 0 {
		t.Fatalf("expected 0 MCP calls before --yes, got %d: %+v", len(caller.calls), caller.calls)
	}
	if caller.product != "" || caller.tool != "" || caller.args != nil {
		t.Fatalf("expected no MCP call recorded, got product=%q tool=%q args=%#v", caller.product, caller.tool, caller.args)
	}
}

func TestCrossPlatformCoverageToolbarRemoveCustomCallsMCPWithExactArgsWhenYes(t *testing.T) {
	caller := &toolbarTestCaller{}
	err := executeToolbarCommand(t, newToolbarRemoveCustomCommand(), caller,
		"--conversation-id", "cidX", "--shortcut-id", "42", "--yes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("expected exactly 1 MCP call, got %d: %+v", len(caller.calls), caller.calls)
	}
	call := caller.calls[0]
	if call.product != "im" {
		t.Fatalf("product = %q, want im", call.product)
	}
	if call.tool != "remove_custom_shortcut" {
		t.Fatalf("tool = %q, want remove_custom_shortcut", call.tool)
	}
	wantArgs := map[string]any{"openConversationId": "cidX", "shortcutId": int64(42)}
	if !reflect.DeepEqual(call.args, wantArgs) {
		t.Fatalf("tool args = %#v, want %#v", call.args, wantArgs)
	}
}

type removeCustomSeamCall struct {
	openConversationId string
	shortcutId         int64
}

type removeCustomSeamStub struct {
	calls []removeCustomSeamCall
	err   error
}

// invoke records the seam call and forwards to deps.Caller.CallTool so the
// command path still records the same remote invocation shape as production.
func (s *removeCustomSeamStub) invoke(openConversationId string, shortcutId int64) error {
	s.calls = append(s.calls, removeCustomSeamCall{openConversationId: openConversationId, shortcutId: shortcutId})
	if err := callMCPToolOnServer("im", "remove_custom_shortcut", map[string]any{
		"openConversationId": openConversationId,
		"shortcutId":         shortcutId,
	}); err != nil {
		return err
	}
	return s.err
}

func TestCrossPlatformCoverageToolbarRemoveCustomSeamRejectsWithoutYes(t *testing.T) {
	stub := &removeCustomSeamStub{}
	testseam.Swap(t, &removeChatToolbarCustomShortcutFn, stub.invoke)

	err := executeToolbarCommand(t, newToolbarRemoveCustomCommand(), &toolbarTestCaller{},
		"--conversation-id", "cidX", "--shortcut-id", "42")
	requireTypedConfirmationError(t, err)
	if len(stub.calls) != 0 {
		t.Fatalf("expected 0 seam calls before --yes, got %d: %+v", len(stub.calls), stub.calls)
	}
}

func TestCrossPlatformCoverageToolbarRemoveCustomSeamCallsWithExactArgsWhenYes(t *testing.T) {
	stub := &removeCustomSeamStub{}
	testseam.Swap(t, &removeChatToolbarCustomShortcutFn, stub.invoke)

	err := executeToolbarCommand(t, newToolbarRemoveCustomCommand(), &toolbarTestCaller{},
		"--conversation-id", "cidX", "--shortcut-id", "42", "--yes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stub.calls) != 1 {
		t.Fatalf("expected exactly 1 seam call, got %d: %+v", len(stub.calls), stub.calls)
	}
	call := stub.calls[0]
	if call.openConversationId != "cidX" {
		t.Fatalf("seam openConversationId = %q, want cidX", call.openConversationId)
	}
	if call.shortcutId != int64(42) {
		t.Fatalf("seam shortcutId = %d, want 42", call.shortcutId)
	}
}

func TestToolbarRemoveCustomDeclaresDestructiveHighRiskConfirmation(t *testing.T) {
	var got LeafSpec
	next := deps
	next.DeclareLeafMetadata = func(cmd *cobra.Command, spec LeafSpec) *cobra.Command {
		got = spec
		return cmd
	}
	testseam.Swap(t, &deps, next)

	_ = newToolbarRemoveCustomCommand()
	if got.Safety.Effect != "destructive" || got.Safety.Risk != "high" ||
		got.Safety.Confirmation != "user_required" {
		t.Fatalf("remove-custom safety = %#v, want destructive/high/user_required", got.Safety)
	}
}
