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
	"strings"

	"github.com/spf13/cobra"
)

// ReportSenderSubmitter executes the delegated report submission route. The
// application owns authentication and HTTP transport so helpers does not
// depend on internal/auth (which already imports helpers).
type ReportSenderSubmitter interface {
	Submit(context.Context, *cobra.Command, ReportSenderSubmission) error
}

// ReportSenderSubmission is the validated, transport-neutral input for
// submitting a report on behalf of an employee.
type ReportSenderSubmission struct {
	SenderUserID string
	TemplateID   string
	Contents     []map[string]any
	DDFrom       string
	ToChat       bool
	ToUserIDs    []string
	DryRun       bool
}

// BuildReportCreateOAPIRequest converts the shared report input into the
// legacy DingTalk OAPI body accepted by POST /topapi/report/create.
func BuildReportCreateOAPIRequest(submission ReportSenderSubmission) (map[string]any, error) {
	sender := strings.TrimSpace(submission.SenderUserID)
	if sender == "" {
		return nil, &CLIError{
			Code:       CodeMissingParam,
			Message:    "flag --sender-user-id requires a non-empty value",
			Suggestion: "传入要作为日志发送人的员工 userId；如需以当前登录用户提交，请完全移除 --sender-user-id",
			Operation:  "report.create.delegated",
		}
	}
	templateID := strings.TrimSpace(submission.TemplateID)
	if templateID == "" {
		return nil, &CLIError{
			Code:      CodeMissingParam,
			Message:   "flag --template-id is required",
			Operation: "report.create.delegated",
		}
	}
	normalizedContents := make([]map[string]any, 0, len(submission.Contents))
	for _, item := range submission.Contents {
		if item == nil {
			normalizedContents = append(normalizedContents, nil)
			continue
		}
		copied := make(map[string]any, len(item))
		for key, value := range item {
			copied[key] = value
		}
		normalizedContents = append(normalizedContents, copied)
	}
	if err := validateAndNormalizeReportContents(normalizedContents); err != nil {
		return nil, err
	}

	contents := make([]map[string]any, 0, len(normalizedContents))
	for _, item := range normalizedContents {
		mapped := make(map[string]any, len(item))
		for key, value := range item {
			switch key {
			case "contentType":
				mapped["content_type"] = value
			default:
				mapped[key] = value
			}
		}
		contents = append(contents, mapped)
	}

	ddFrom := strings.TrimSpace(submission.DDFrom)
	if ddFrom == "" {
		ddFrom = "dws"
	}
	param := map[string]any{
		"userid":      sender,
		"template_id": templateID,
		"contents":    contents,
		"dd_from":     ddFrom,
		"to_chat":     submission.ToChat,
	}
	if len(submission.ToUserIDs) > 0 {
		param["to_userids"] = append([]string(nil), submission.ToUserIDs...)
	}
	return map[string]any{"create_report_param": param}, nil
}
