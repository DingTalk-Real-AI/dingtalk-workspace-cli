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

import "testing"

func TestBuildReportCreateOAPIRequestMapsSenderAndContents(t *testing.T) {
	t.Parallel()

	request, err := buildReportCreateOAPIRequest(ReportSenderSubmission{
		SenderUserID: "sender-1",
		TemplateID:   "template-1",
		DDFrom:       "dws",
		ToChat:       true,
		ToUserIDs:    []string{"receiver-1", "receiver-2"},
		Contents: []map[string]any{{
			"key":         "今日完成工作",
			"sort":        "0",
			"type":        "1",
			"contentType": "markdown",
			"content":     "完成 CLI 开发",
		}},
	})
	if err != nil {
		t.Fatalf("buildReportCreateOAPIRequest() error = %v", err)
	}

	param, ok := request["create_report_param"].(map[string]any)
	if !ok {
		t.Fatalf("create_report_param type = %T, want map[string]any", request["create_report_param"])
	}
	if got := param["userid"]; got != "sender-1" {
		t.Fatalf("userid = %#v, want sender-1", got)
	}
	if got := param["template_id"]; got != "template-1" {
		t.Fatalf("template_id = %#v, want template-1", got)
	}
	if got := param["dd_from"]; got != "dws" {
		t.Fatalf("dd_from = %#v, want dws", got)
	}
	if got := param["to_chat"]; got != true {
		t.Fatalf("to_chat = %#v, want true", got)
	}

	contents, ok := param["contents"].([]map[string]any)
	if !ok || len(contents) != 1 {
		t.Fatalf("contents = %#v, want one mapped item", param["contents"])
	}
	if got := contents[0]["content_type"]; got != "markdown" {
		t.Fatalf("content_type = %#v, want markdown", got)
	}
	if _, exists := contents[0]["contentType"]; exists {
		t.Fatalf("camelCase contentType must not be sent to OAPI: %#v", contents[0])
	}
}

func TestBuildReportCreateOAPIRequestRejectsMissingSender(t *testing.T) {
	t.Parallel()

	_, err := buildReportCreateOAPIRequest(ReportSenderSubmission{
		TemplateID: "template-1",
		Contents:   []map[string]any{{"key": "x"}},
	})
	if err == nil {
		t.Fatal("expected missing sender validation error")
	}
}
