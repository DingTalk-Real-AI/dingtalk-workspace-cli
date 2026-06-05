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
	"fmt"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// ReportSenderSubmission is the transport-neutral input for submitting a
// report on behalf of an employee.
type ReportSenderSubmission struct {
	SenderUserID string
	TemplateID   string
	Contents     []map[string]any
	DDFrom       string
	ToChat       bool
	ToUserIDs    []string
}

// BuildReportCreateOAPIRequest converts the CLI/MCP-shaped submission into the
// legacy DingTalk OAPI request body accepted by /topapi/report/create.
func BuildReportCreateOAPIRequest(submission ReportSenderSubmission) (map[string]any, error) {
	return buildReportCreateOAPIRequest(submission)
}

func buildReportCreateOAPIRequest(submission ReportSenderSubmission) (map[string]any, error) {
	sender := strings.TrimSpace(submission.SenderUserID)
	if sender == "" {
		return nil, apperrors.NewValidation("--sender-user-id is required for delegated report submission")
	}
	templateID := strings.TrimSpace(submission.TemplateID)
	if templateID == "" {
		return nil, apperrors.NewValidation("--template-id is required")
	}
	if len(submission.Contents) == 0 {
		return nil, apperrors.NewValidation("--contents or --contents-file must contain at least one report field")
	}

	contents := make([]map[string]any, 0, len(submission.Contents))
	for i, item := range submission.Contents {
		if item == nil {
			return nil, apperrors.NewValidation(fmt.Sprintf("report content item %d must be an object", i))
		}
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
