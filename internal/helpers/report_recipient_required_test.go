// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package helpers

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

// reportCreateArgsRecorder 记录 create_report 收到的原始参数，
// 用于验证 runReportCreate 对 --to-user-ids 的解析与传递。
type reportCreateArgsRecorder struct {
	response string
	format   string
	args     map[string]any
}

func (c *reportCreateArgsRecorder) CallTool(_ context.Context, _, tool string, args map[string]any) (*edition.ToolResult, error) {
	if tool == "create_report" {
		c.args = args
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: c.response}}}, nil
}

func (c *reportCreateArgsRecorder) Format() string { return c.format }
func (*reportCreateArgsRecorder) DryRun() bool     { return false }
func (*reportCreateArgsRecorder) Fields() string   { return "" }
func (*reportCreateArgsRecorder) JQ() string       { return "" }

func TestReportEntrySubmitRequiresRecipientFlag(t *testing.T) {
	t.Cleanup(func() { contract.ClearProductDeclForTest("report") })
	root := newReportCommand()
	// 主命令与废弃别名共用 addReportCreateFlags，两侧的 Cobra required
	// 标记必须同时存在（NativeRequired 一致性校验要求严格相等）。
	for _, path := range [][]string{{"entry", "submit"}, {"create"}} {
		leaf, _, err := root.Find(path)
		if err != nil || leaf == nil {
			t.Fatalf("%v command missing: %v", path, err)
		}
		flag := leaf.Flags().Lookup("to-user-ids")
		if flag == nil {
			t.Fatalf("%v missing --to-user-ids", path)
		}
		if _, required := flag.Annotations[cobra.BashCompOneRequiredFlag]; !required {
			t.Fatalf("%v --to-user-ids must stay Cobra required", path)
		}
	}

	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SilenceErrors = true
	root.SilenceUsage = true
	// Cobra 的 Execute 会跳回 Root 执行，必须携带完整命令路径；
	// required 校验在 RunE 之前拦截未传的 --to-user-ids。
	root.SetArgs([]string{
		"entry", "submit",
		"--template-id", "TPL",
		"--contents", `[{"key":"k","sort":"0","content":"c","contentType":"markdown","type":"1"}]`,
	})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "to-user-ids") {
		t.Fatalf("missing --to-user-ids error = %v", err)
	}
}

func TestReportCreateRejectsBlankRecipient(t *testing.T) {
	t.Cleanup(func() { contract.ClearProductDeclForTest("report") })
	root := newReportCommand()
	submit, _, _ := root.Find([]string{"entry", "submit"})
	_ = submit.Flags().Set("template-id", "TPL")
	_ = submit.Flags().Set("contents", `[{"key":"k","sort":"0","content":"c","contentType":"markdown","type":"1"}]`)
	// Cobra required 只拦未传；空值/纯分隔符仍会进入 RunE，必须 fail-closed。
	_ = submit.Flags().Set("to-user-ids", " , ")
	err := runReportCreate(submit, nil)
	cliErr, ok := err.(*CLIError)
	if !ok || cliErr.Code != CodeMissingParam {
		t.Fatalf("blank to-user-ids error = %#v", err)
	}
}

func TestReportCreatePassesRecipientsToCreateReport(t *testing.T) {
	t.Cleanup(func() { contract.ClearProductDeclForTest("report") })
	recorder := &reportCreateArgsRecorder{
		format:   "json",
		response: `{"reportId":"id","url":"dingtalk://direct"}`,
	}
	testseam.Protect(t, &deps)
	InitDeps(recorder)
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	deps.Out.w = out
	deps.Out.errW = errOut

	root := newReportCommand()
	submit, _, _ := root.Find([]string{"entry", "submit"})
	_ = submit.Flags().Set("template-id", "TPL")
	_ = submit.Flags().Set("contents", `[{"key":"k","sort":"0","content":"c","contentType":"markdown","type":"1"}]`)
	_ = submit.Flags().Set("to-user-ids", " userA , userB ")
	if err := runReportCreate(submit, nil); err != nil {
		t.Fatalf("runReportCreate: %v", err)
	}
	got, _ := recorder.args["toUserIds"].([]string)
	if len(got) != 2 || got[0] != "userA" || got[1] != "userB" {
		t.Fatalf("toUserIds = %#v", recorder.args["toUserIds"])
	}
}
