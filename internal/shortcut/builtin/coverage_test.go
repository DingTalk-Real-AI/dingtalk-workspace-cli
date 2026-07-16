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
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/builtin"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

// fakeCaller implements edition.ToolCaller. It records the (product, tool, args)
// of the last CallTool so the coverage test can assert what MCP call a shortcut
// assembled — WITHOUT any network I/O or side effects. This lets us exercise
// every shortcut, including write/delete commands, safely.
type fakeCaller struct {
	called  bool
	product string
	tool    string
	args    map[string]any
	dryRun  bool
	payload int
}

func (f *fakeCaller) reset() { f.called, f.product, f.tool, f.args = false, "", "", nil }

func (f *fakeCaller) CallTool(_ context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	f.called, f.product, f.tool, f.args = true, product, tool, args
	payload, err := json.Marshal(fakePayload(f.payload))
	if err != nil {
		return nil, err
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: string(payload)}}}, nil
}
func (f *fakeCaller) Format() string { return "json" }
func (f *fakeCaller) DryRun() bool   { return f.dryRun }
func (f *fakeCaller) Fields() string { return "" }
func (f *fakeCaller) JQ() string     { return "" }

// realToolSet scans the helper sources (the ground truth) and returns the set of
// snake_case identifiers found there. Every tool a shortcut invokes must appear
// in this set; anything else would be a hallucinated tool name.
func realToolSet(t *testing.T) map[string]bool {
	t.Helper()
	files, err := filepath.Glob("../../../internal/helpers/*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("cannot locate helper sources: %v (found %d)", err, len(files))
	}
	// Two oracles unioned:
	//   - every ASCII word token in the helper sources (guarantees any
	//     snake_case / camelCase tool name present anywhere is captured);
	//   - quoted literals containing CJK, to cover Chinese const tool names
	//     (e.g. minutes' "执行听记指令-发起AI听记录音").
	wordRe := regexp.MustCompile(`[A-Za-z][A-Za-z0-9_]{2,}`)
	cjkRe := regexp.MustCompile(`"([^"]*\p{Han}[^"]*)"`)
	set := map[string]bool{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		s := string(data)
		for _, m := range wordRe.FindAllString(s, -1) {
			set[m] = true
		}
		for _, m := range cjkRe.FindAllStringSubmatch(s, -1) {
			set[m[1]] = true
		}
	}
	return set
}

// synthArgs builds a plausible argument vector for a shortcut: a value for every
// declared flag (enum flags get their first allowed value), plus --yes to skip
// confirmation on write/high-risk commands.
func synthArgs(s shortcut.Shortcut) []string {
	args := []string{s.Service, s.Command}
	skipped := map[string]bool{}
	for _, constraint := range s.Constraints {
		if constraint.Kind != shortcut.ConstraintExactlyOne && constraint.Kind != shortcut.ConstraintMutuallyExclusive {
			continue
		}
		for _, name := range constraint.Flags[1:] {
			skipped[name] = true
		}
	}
	for _, f := range s.Flags {
		if skipped[f.Name] {
			continue
		}
		switch {
		case len(f.Enum) > 0:
			args = append(args, "--"+f.Name, f.Enum[0])
		case f.Type == shortcut.FlagBool:
			args = append(args, "--"+f.Name)
		default:
			args = append(args, "--"+f.Name, synthFlagValue(s, f))
		}
	}
	return args
}

func synthFlagValue(s shortcut.Shortcut, f shortcut.Flag) string {
	name := strings.ToLower(f.Name)
	desc := strings.ToLower(f.Desc)
	if f.Type == shortcut.FlagInt {
		switch name {
		case "from":
			return "9"
		case "to":
			return "18"
		default:
			return "1"
		}
	}
	if name == "pair" {
		return "原文=>替换"
	}
	if name == "category-ids" {
		return "1"
	}
	if name == "role-types" {
		return "creator"
	}
	if name == "num" {
		return "1"
	}
	if name == "type" && s.Service == "attendance" && s.Command == "+get-approve-template" {
		return "leave"
	}
	if s.Service == "attendance" {
		switch {
		case s.Command == "+list-approve" && name == "types":
			return "overtime"
		case s.Command == "+import-schedule" && name == "schedules":
			return `[{"userId":"user-1","workDate":"2026-04-01 09:00:00","classId":1,"isRest":"N"}]`
		case s.Command == "+create-class" && name == "class-vo":
			return `{"sections":[{"times":[{"checkType":"OnDuty","checkTime":"09:00","across":0},{"checkType":"OffDuty","checkTime":"18:00","across":0}]}]}`
		case s.Command == "+create-group" && name == "group-vo":
			return `{"defaultClassId":1,"workDayClassList":[0,1,1,1,1,1,0]}`
		case s.Command == "+update-group" && name == "class-ids":
			return `[1]`
		case s.Command == "+query-report-data" && name == "columns":
			return "1"
		case s.Command == "+update-leave-type" && name == "visibility-rules":
			return `[{"type":"dept","visible":["-1"]}]`
		}
	}
	if strings.Contains(desc, "json") || strings.Contains(name, "config") || strings.HasSuffix(name, "-vo") {
		switch name {
		case "records":
			return `[{"recordId":"rec-1","cells":{"name":"test"}}]`
		case "schedules":
			return `[{}]`
		case "filters", "sub-roles", "visibility-rules":
			return `[]`
		default:
			return `{}`
		}
	}
	if name == "start" || name == "end" || name == "date" {
		if s.Service == "attendance" {
			if strings.Contains(desc, "hh:mm:ss") || strings.Contains(desc, "时分秒") {
				if name == "end" {
					return "2026-04-01 18:00:00"
				}
				return "2026-04-01 09:00:00"
			}
			if name == "end" {
				return "2026-04-02"
			}
			return "2026-04-01"
		}
		if name == "end" {
			return "2026-04-01T18:00:00+08:00"
		}
		return "2026-04-01T09:00:00+08:00"
	}
	if name == "modified-start" {
		return "2026-04-01T09:00:00+08:00"
	}
	if name == "modified-end" {
		return "2026-04-01T18:00:00+08:00"
	}
	if strings.Contains(name, "email") {
		return "user@example.com"
	}
	if strings.Contains(name, "url") {
		return "https://example.com/file"
	}
	return "x"
}

func fakePayload(mode int) map[string]any {
	item := map[string]any{
		"id": "id-1", "uuid": "uuid-1", "taskUuid": "task-1", "taskId": "task-1",
		"todoTaskId": "todo-1", "recordId": "rec-1", "baseId": "base-1", "tableId": "table-1",
		"fieldId": "field-1", "viewId": "view-1", "userId": "user-1", "deptId": "dept-1",
		"spaceId": "space-1", "nodeId": "node-1", "dentryUuid": "file-1", "eventId": "event-1",
		"roomId": "room-1", "calendarId": "cal-1", "messageId": "msg-1", "reportId": "report-1",
		"processInstanceId": "process-1", "openConversationId": "cid-1", "conversationId": "cid-1",
		"name": "测试名称", "title": "测试标题", "subject": "测试主题", "summary": "测试摘要",
		"description": "测试描述", "text": "测试内容", "content": "测试内容", "status": "NORMAL",
		"type": "text", "email": "user@example.com", "mobile": "13800000000", "priority": 1,
		"startTime": int64(1775005200000), "endTime": int64(1775037600000), "createTime": int64(1775005200000),
		"planFinishDate": int64(1775037600000), "fileSize": int64(128), "cells": map[string]any{"name": "test"},
		"sender":  map[string]any{"userId": "user-1", "name": "测试用户"},
		"creator": map[string]any{"userId": "user-1", "name": "测试用户"},
		"owner":   map[string]any{"userId": "user-1", "name": "测试用户"},
	}
	list := []any{item}
	container := map[string]any{
		"ok": true, "hasMore": false, "nextCursor": "", "nextToken": "", "total": 1,
	}
	for _, key := range []string{
		"list", "items", "records", "users", "departments", "spaces", "tables", "fields", "views",
		"messages", "events", "rooms", "calendars", "participants", "attendees", "approvals", "tasks",
		"todos", "mails", "reports", "minutes", "documents", "files", "nodes", "groups", "members",
	} {
		container[key] = list
	}
	for key, value := range item {
		container[key] = value
	}
	switch mode {
	case 1:
		container["result"] = list
		container["data"] = list
		return container
	case 2:
		return map[string]any{"ok": true, "result": container, "data": container}
	case 3:
		return map[string]any{"ok": true, "result": list, "data": container, "items": list, "list": list}
	default:
		return map[string]any{"ok": true}
	}
}

func setOutputRecursive(cmd *cobra.Command, writer io.Writer) {
	cmd.SetOut(writer)
	cmd.SetErr(writer)
	for _, child := range cmd.Commands() {
		setOutputRecursive(child, writer)
	}
}

func newCoverageRoot() *cobra.Command {
	root := &cobra.Command{Use: "dws", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.AddCommand(builtin.Commands()...)
	setOutputRecursive(root, io.Discard)
	return root
}

func silenceProcessOutput(t *testing.T) {
	t.Helper()
	file, err := os.Create(filepath.Join(t.TempDir(), "shortcut-output.log"))
	if err != nil {
		t.Fatal(err)
	}
	oldStdout, oldStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = file, file
	t.Cleanup(func() {
		os.Stdout, os.Stderr = oldStdout, oldStderr
		_ = file.Close()
	})
}

func TestAllShortcutConstraintsAreDiscoverableAndBound(t *testing.T) {
	for _, s := range shortcut.All() {
		name := s.Service + " " + s.Command
		flags := make(map[string]bool, len(s.Flags))
		for _, flag := range s.Flags {
			flags[flag.Name] = true
		}
		hasCustom := false
		for _, constraint := range s.Constraints {
			if len(constraint.Flags) == 0 {
				t.Errorf("%s: %s constraint has no flags", name, constraint.Kind)
			}
			for _, flag := range constraint.Flags {
				if !flags[flag] {
					t.Errorf("%s: %s constraint references unknown --%s", name, constraint.Kind, flag)
				}
			}
			if constraint.Kind == shortcut.ConstraintCustom {
				hasCustom = true
				if strings.TrimSpace(constraint.Description) == "" {
					t.Errorf("%s: custom constraint has no description", name)
				}
			}
		}
		if s.Validate != nil && !hasCustom {
			t.Errorf("%s: Validate is not published as a custom constraint", name)
		}
		if hasCustom && s.Validate == nil {
			t.Errorf("%s: custom constraint has no Validate implementation", name)
		}
	}
}

// TestAllShortcutsAssemble drives every registered shortcut end-to-end through
// the cobra tree with synthesized inputs and asserts each one is healthy:
//   - it either assembles an MCP call with a real (non-hallucinated) tool name,
//   - or it is rejected by its own validation (proving the plumbing ran),
//   - and it never panics or silently no-ops.
func TestAllShortcutsAssemble(t *testing.T) {
	silenceProcessOutput(t)
	fake := &fakeCaller{}
	helpers.InitDeps(fake)

	root := newCoverageRoot()

	real := realToolSet(t)
	all := shortcut.All()
	if len(all) == 0 {
		t.Fatal("no shortcuts registered")
	}

	var assembled, validated, failed int
	var validatedNames []string
	for _, s := range all {
		name := s.Service + " " + s.Command
		args := append(synthArgs(s), "--yes")

		fake.reset()
		err := func() (e error) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("PANIC in %s: %v", name, r)
					e = nil
				}
			}()
			root.SetArgs(args)
			return root.Execute()
		}()

		switch {
		case fake.called:
			assembled++
			if fake.tool == "" {
				t.Errorf("%s: assembled call with EMPTY tool name", name)
			} else if !real[fake.tool] {
				t.Errorf("%s: tool %q not found in any helper (hallucinated?)", name, fake.tool)
			}
			if fake.product == "" {
				t.Errorf("%s: assembled call with EMPTY product", name)
			}
		case err != nil:
			// Rejected by required/enum/Validate before dispatch — plumbing OK.
			validated++
			validatedNames = append(validatedNames, s.Service+s.Command+"="+err.Error())
		default:
			failed++
			t.Errorf("%s: returned nil error but never assembled an MCP call (dead command)", name)
		}
	}

	t.Logf("shortcuts=%d  assembled(真实MCP)=%d  validated(自校验拦截)=%d  failed=%d",
		len(all), assembled, validated, failed)
	t.Logf("validated(自校验拦截) 明细: %s", strings.Join(validatedNames, " "))
	if assembled == 0 {
		t.Fatal("no shortcut assembled an MCP call — harness likely broken")
	}
}

func TestAllShortcutsHandleRepresentativeResponseShapes(t *testing.T) {
	silenceProcessOutput(t)
	all := shortcut.All()
	for _, mode := range []int{1, 2, 3} {
		fake := &fakeCaller{payload: mode}
		helpers.InitDeps(fake)
		root := newCoverageRoot()
		called := 0
		for _, s := range all {
			fake.reset()
			root.SetArgs(append(synthArgs(s), "--yes"))
			func() {
				defer func() {
					if recovered := recover(); recovered != nil {
						t.Errorf("payload mode %d panicked in %s %s: %v", mode, s.Service, s.Command, recovered)
					}
				}()
				_ = root.Execute()
			}()
			if fake.called {
				called++
			}
		}
		if called < 350 {
			t.Errorf("payload mode %d reached only %d/%d shortcuts", mode, called, len(all))
		}
	}
}

func TestCrossPlatformCoverageReplaceBatchDryRunDoesNotCallTool(t *testing.T) {
	fake := &fakeCaller{dryRun: true}
	helpers.InitDeps(fake)

	root := newCoverageRoot()
	root.SetArgs([]string{
		"minutes", "+replace-batch", "--id", "task-1",
		"--pair", "Q2=>第二季度", "--dry-run", "--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if fake.called {
		t.Fatalf("dry-run called real tool %s/%s with %#v", fake.product, fake.tool, fake.args)
	}
}

// TestAllToolLiteralsAreReal statically scans every shortcut package source for
// CallMCP("tool", ...) literals and asserts each tool exists in the helper
// ground truth. This covers the ~55 shortcuts whose own validation blocks the
// runtime assemble path, so no shortcut's tool name goes unverified.
func TestCrossPlatformCoverageAllToolLiteralsAreReal(t *testing.T) {
	real := realToolSet(t)
	srcs, err := filepath.Glob("../*/*.go")
	if err != nil || len(srcs) == 0 {
		t.Fatalf("cannot locate shortcut sources: %v", err)
	}
	// CallMCP("tool", ...) — 1:1 wrappers; the tool is the 1st arg.
	callRe := regexp.MustCompile(`CallMCP\("([^"]+)"`)
	// CallMCPData("product", "tool", ...) — multi-step (smart) shortcuts; the
	// tool is the 2nd arg.
	dataRe := regexp.MustCompile(`CallMCPData\("[^"]+",\s*"([^"]+)"`)
	var checked, bad int
	check := func(f, tool string) {
		checked++
		if !real[tool] {
			bad++
			t.Errorf("%s: tool %q not found in any helper (hallucinated?)", filepath.Base(f), tool)
		}
	}
	for _, f := range srcs {
		if strings.HasSuffix(f, "_test.go") || strings.Contains(f, "/builtin/") {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		src := string(data)
		for _, m := range callRe.FindAllStringSubmatch(src, -1) {
			check(f, m[1])
		}
		for _, m := range dataRe.FindAllStringSubmatch(src, -1) {
			check(f, m[1])
		}
	}
	t.Logf("tool literals checked=%d hallucinated=%d", checked, bad)
}

// TestAllHaveIntent enforces that every shortcut carries a natural-language
// Intent (a fuller "what/when to use" description for discovery and AI-agent
// matching), not just the terse one-line Description.
func TestCrossPlatformCoverageAllHaveIntent(t *testing.T) {
	var missing []string
	for _, s := range shortcut.All() {
		if strings.TrimSpace(s.Intent) == "" {
			missing = append(missing, s.Service+" "+s.Command)
		}
	}
	if len(missing) > 0 {
		t.Errorf("%d shortcut(s) missing Intent (natural-language description):\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
}

// TestNoDuplicateCommands guards against two shortcuts colliding on the same
// service+command path.
func TestCrossPlatformCoverageNoDuplicateCommands(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range shortcut.All() {
		key := s.Service + " " + s.Command
		if seen[key] {
			t.Errorf("duplicate shortcut: %s", key)
		}
		seen[key] = true
		if !strings.HasPrefix(s.Command, "+") {
			t.Errorf("%s: command must start with '+'", key)
		}
	}
}
