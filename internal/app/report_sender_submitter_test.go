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
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/apiclient"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/spf13/cobra"
)

type recordingReportAPICaller struct {
	calls int
	req   apiclient.RawAPIRequest
	resp  *apiclient.RawAPIResponse
	err   error
}

func (c *recordingReportAPICaller) Do(_ context.Context, req apiclient.RawAPIRequest) (*apiclient.RawAPIResponse, error) {
	c.calls++
	c.req = req
	return c.resp, c.err
}

func TestReportSenderOAPISubmitterPostsLegacyCreateRequest(t *testing.T) {
	t.Parallel()

	caller := &recordingReportAPICaller{resp: reportSenderTestResponse(`{"errcode":0,"result":"report-1"}`)}
	tokenCalls := 0
	submitter := &reportSenderOAPISubmitter{
		resolveToken: func(context.Context) (string, error) {
			tokenCalls++
			return "token-1", nil
		},
		newClient: func(token string) reportSenderAPICaller {
			if token != "token-1" {
				t.Fatalf("client token = %q, want token-1", token)
			}
			return caller
		},
	}
	cmd, out := newReportSenderSubmitterTestCommand(false)

	err := submitter.Submit(context.Background(), cmd, helpers.ReportSenderSubmission{
		SenderUserID: "sender-1",
		TemplateID:   "template-1",
		Contents: []map[string]any{{
			"key":         "x",
			"sort":        "0",
			"type":        "1",
			"contentType": "markdown",
			"content":     "done",
		}},
	})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if tokenCalls != 1 || caller.calls != 1 {
		t.Fatalf("token calls = %d, API calls = %d, want 1/1", tokenCalls, caller.calls)
	}
	if caller.req.Method != http.MethodPost || caller.req.Path != reportCreateOAPIPath {
		t.Fatalf("request = %s %s, want POST %s", caller.req.Method, caller.req.Path, reportCreateOAPIPath)
	}
	if !strings.Contains(out.String(), "report-1") {
		t.Fatalf("output missing report id: %q", out.String())
	}
}

func TestReportSenderOAPISubmitterDryRunSkipsTokenAndHTTP(t *testing.T) {
	t.Parallel()

	caller := &recordingReportAPICaller{}
	tokenCalls := 0
	submitter := &reportSenderOAPISubmitter{
		resolveToken: func(context.Context) (string, error) {
			tokenCalls++
			return "token-1", nil
		},
		newClient: func(string) reportSenderAPICaller { return caller },
	}
	cmd, out := newReportSenderSubmitterTestCommand(true)

	err := submitter.Submit(context.Background(), cmd, helpers.ReportSenderSubmission{
		SenderUserID: "sender-1",
		TemplateID:   "template-1",
		Contents:     []map[string]any{{"key": "x"}},
	})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if tokenCalls != 0 || caller.calls != 0 {
		t.Fatalf("dry-run token calls = %d, API calls = %d, want 0/0", tokenCalls, caller.calls)
	}
	if !strings.Contains(out.String(), `"sender-1"`) || !strings.Contains(out.String(), `"dry_run": true`) {
		t.Fatalf("dry-run output missing sender/request marker: %q", out.String())
	}
}

func TestReportSenderOAPISubmitterReturnsBusinessError(t *testing.T) {
	t.Parallel()

	caller := &recordingReportAPICaller{resp: reportSenderTestResponse(`{"errcode":60011,"errmsg":"no permission"}`)}
	submitter := &reportSenderOAPISubmitter{
		resolveToken: func(context.Context) (string, error) { return "token-1", nil },
		newClient:    func(string) reportSenderAPICaller { return caller },
	}
	cmd, _ := newReportSenderSubmitterTestCommand(false)

	err := submitter.Submit(context.Background(), cmd, helpers.ReportSenderSubmission{
		SenderUserID: "sender-1",
		TemplateID:   "template-1",
		Contents:     []map[string]any{{"key": "x"}},
	})
	if err == nil || !strings.Contains(err.Error(), "60011") {
		t.Fatalf("Submit() error = %v, want business error 60011", err)
	}
}

func newReportSenderSubmitterTestCommand(dryRun bool) (*cobra.Command, *bytes.Buffer) {
	out := &bytes.Buffer{}
	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().Bool("dry-run", dryRun, "")
	root.PersistentFlags().String("format", "json", "")
	root.PersistentFlags().String("fields", "", "")
	root.PersistentFlags().String("jq", "", "")
	cmd := &cobra.Command{Use: "submit"}
	root.AddCommand(cmd)
	cmd.SetOut(out)
	cmd.SetErr(&bytes.Buffer{})
	return cmd, out
}

func reportSenderTestResponse(body string) *apiclient.RawAPIResponse {
	header := make(http.Header)
	header.Set("Content-Type", "application/json")
	return &apiclient.RawAPIResponse{StatusCode: http.StatusOK, Header: header, Body: []byte(body)}
}
