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

package helpers

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

type recordingReportSenderSubmitter struct {
	calls      int
	submission ReportSenderSubmission
	err        error
}

func (s *recordingReportSenderSubmitter) Submit(_ context.Context, _ *cobra.Command, submission ReportSenderSubmission) error {
	s.calls++
	s.submission = submission
	return s.err
}

func TestReportCreateDelegatedRouteReusesCurrentValidationAndSkipsMCP(t *testing.T) {
	caller := &reportTestCaller{dry: true, format: "json", response: `{"ok":true}`}
	submitter := &recordingReportSenderSubmitter{}
	previous := deps
	t.Cleanup(func() { deps = previous })
	InitDeps(caller, WithReportSenderSubmitter(submitter))

	cmd := &cobra.Command{Use: "submit"}
	addReportCreateFlags(cmd)
	setReportCreateTestFlags(t, cmd,
		"template-id", "template-1",
		"contents", `[{"key":" Done ","sort":0,"content":"work","contentType":"text","type":"text"}]`,
		"sender-user-id", " sender-1 ",
		"to-user-ids", " receiver-1, ,receiver-2 ",
	)
	if err := cmd.Flags().Set("to-chat", "true"); err != nil {
		t.Fatal(err)
	}

	if err := runReportCreate(cmd, nil); err != nil {
		t.Fatalf("runReportCreate() error = %v", err)
	}
	if submitter.calls != 1 {
		t.Fatalf("delegated submitter calls = %d, want 1", submitter.calls)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("delegated route called MCP tools: %#v", caller.calls)
	}
	got := submitter.submission
	if got.SenderUserID != "sender-1" || got.TemplateID != "template-1" || !got.ToChat || !got.DryRun {
		t.Fatalf("delegated submission identity = %#v", got)
	}
	if len(got.ToUserIDs) != 2 || got.ToUserIDs[0] != "receiver-1" || got.ToUserIDs[1] != "receiver-2" {
		t.Fatalf("delegated recipients = %#v", got.ToUserIDs)
	}
	if len(got.Contents) != 1 ||
		got.Contents[0]["key"] != "Done" ||
		got.Contents[0]["sort"] != "0" ||
		got.Contents[0]["contentType"] != "markdown" ||
		got.Contents[0]["type"] != "1" {
		t.Fatalf("delegated contents were not normalized by current report validation: %#v", got.Contents)
	}
}

func TestReportCreateExplicitEmptySenderNeverFallsBackToCurrentUser(t *testing.T) {
	caller := &reportTestCaller{dry: true, format: "json", response: `{"ok":true}`}
	submitter := &recordingReportSenderSubmitter{}
	previous := deps
	t.Cleanup(func() { deps = previous })
	InitDeps(caller, WithReportSenderSubmitter(submitter))

	cmd := &cobra.Command{Use: "submit"}
	addReportCreateFlags(cmd)
	setReportCreateTestFlags(t, cmd,
		"template-id", "template-1",
		"contents", `[{"key":"Done","sort":"0","content":"work","contentType":"markdown","type":"1"}]`,
		"sender-user-id", "   ",
	)
	err := runReportCreate(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "--sender-user-id") {
		t.Fatalf("runReportCreate() error = %v, want sender validation", err)
	}
	if submitter.calls != 0 || len(caller.calls) != 0 {
		t.Fatalf("empty sender routed remotely: submitter=%d MCP=%#v", submitter.calls, caller.calls)
	}
}

func TestReportCreateWithoutSenderKeepsMCPRoute(t *testing.T) {
	caller := &reportTestCaller{
		format:   "json",
		response: `{"reportId":"report-1","url":"dingtalk://report-1"}`,
	}
	installReportTestDeps(t, caller)
	previousArgs := os.Args
	os.Args = []string{"dws", "report", "entry", "submit"}
	t.Cleanup(func() { os.Args = previousArgs })

	cmd := &cobra.Command{Use: "submit"}
	addReportCreateFlags(cmd)
	setReportCreateTestFlags(t, cmd,
		"template-id", "template-1",
		"contents", `[{"key":"Done","sort":"0","content":"work","contentType":"markdown","type":"1"}]`,
		"to-user-ids", "receiver-1",
	)
	if err := runReportCreate(cmd, nil); err != nil {
		t.Fatalf("runReportCreate() error = %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0] != "create_report" {
		t.Fatalf("MCP calls = %#v, want create_report", caller.calls)
	}
	if _, exists := caller.lastArgs["senderUserId"]; exists {
		t.Fatalf("sender leaked into MCP args: %#v", caller.lastArgs)
	}
	if recipients, ok := caller.lastArgs["toUserIds"].([]string); !ok || len(recipients) != 1 || recipients[0] != "receiver-1" {
		t.Fatalf("MCP recipients = %#v", caller.lastArgs["toUserIds"])
	}
}

func TestReportCreateCanonicalAndDeprecatedAliasExposeSenderFlag(t *testing.T) {
	root := newReportCommand()
	canonical, _, err := root.Find([]string{"entry", "submit"})
	if err != nil || canonical == nil {
		t.Fatalf("find canonical report submit: %v", err)
	}
	alias, _, err := root.Find([]string{"create"})
	if err != nil || alias == nil {
		t.Fatalf("find deprecated report create: %v", err)
	}
	for _, cmd := range []*cobra.Command{canonical, alias} {
		if cmd.Flags().Lookup("sender-user-id") == nil {
			t.Fatalf("%s missing --sender-user-id", cmd.CommandPath())
		}
	}
}

func TestBuildReportCreateOAPIRequestMapsValidatedFields(t *testing.T) {
	request, err := BuildReportCreateOAPIRequest(ReportSenderSubmission{
		SenderUserID: "sender-1",
		TemplateID:   "template-1",
		DDFrom:       "automation",
		ToChat:       true,
		ToUserIDs:    []string{"receiver-1", "receiver-2"},
		Contents: []map[string]any{{
			"key":         "今日完成",
			"sort":        "0",
			"content":     "完成 CLI 开发",
			"contentType": "markdown",
			"type":        "1",
		}},
	})
	if err != nil {
		t.Fatalf("BuildReportCreateOAPIRequest() error = %v", err)
	}
	param, ok := request["create_report_param"].(map[string]any)
	if !ok {
		t.Fatalf("create_report_param = %T", request["create_report_param"])
	}
	if param["userid"] != "sender-1" || param["template_id"] != "template-1" ||
		param["dd_from"] != "automation" || param["to_chat"] != true {
		t.Fatalf("OAPI identity/body = %#v", param)
	}
	contents, ok := param["contents"].([]map[string]any)
	if !ok || len(contents) != 1 || contents[0]["content_type"] != "markdown" {
		t.Fatalf("OAPI contents = %#v", param["contents"])
	}
	if _, exists := contents[0]["contentType"]; exists {
		t.Fatalf("camelCase contentType leaked to OAPI: %#v", contents[0])
	}

	if _, err := BuildReportCreateOAPIRequest(ReportSenderSubmission{
		SenderUserID: "sender-1",
		TemplateID:   "template-1",
		Contents:     []map[string]any{{"key": "missing-current-required-fields"}},
	}); err == nil {
		t.Fatal("delegated OAPI builder accepted contents rejected by current report validation")
	}
}

func setReportCreateTestFlags(t *testing.T, cmd *cobra.Command, pairs ...string) {
	t.Helper()
	for index := 0; index < len(pairs); index += 2 {
		if err := cmd.Flags().Set(pairs[index], pairs[index+1]); err != nil {
			t.Fatalf("set --%s: %v", pairs[index], err)
		}
	}
}
