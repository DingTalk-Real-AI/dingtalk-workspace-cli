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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

// ReportSenderSubmitter executes the delegated OAPI submission route.
type ReportSenderSubmitter interface {
	Submit(context.Context, *cobra.Command, ReportSenderSubmission) error
}

// AttachReportSenderSubmission adds the delegated sender route after dynamic
// and helper report commands have been merged.
func AttachReportSenderSubmission(commands []*cobra.Command, submitter ReportSenderSubmitter) {
	attachReportSenderSubmission(commands, submitter)
}

func attachReportSenderSubmission(commands []*cobra.Command, submitter ReportSenderSubmitter) {
	if submitter == nil {
		return
	}
	for _, top := range commands {
		if top == nil || top.Name() != "report" {
			continue
		}
		wrapReportSenderLeaf(findReportCommand(top, "entry", "submit"), submitter)
		wrapReportSenderLeaf(findReportCommand(top, "create"), submitter)
	}
}

func findReportCommand(root *cobra.Command, path ...string) *cobra.Command {
	current := root
	for _, name := range path {
		var next *cobra.Command
		for _, child := range current.Commands() {
			if child != nil && child.Name() == name {
				next = child
				break
			}
		}
		if next == nil {
			return nil
		}
		current = next
	}
	return current
}

func wrapReportSenderLeaf(leaf *cobra.Command, submitter ReportSenderSubmitter) {
	if leaf == nil || leaf.RunE == nil {
		return
	}
	if leaf.Flags().Lookup("sender-user-id") == nil {
		leaf.Flags().String("sender-user-id", "", "日志发送人 userId；设置后使用自有应用凭证通过钉钉 OAPI 代提交")
	}
	originalRunE := leaf.RunE
	leaf.RunE = func(cmd *cobra.Command, args []string) error {
		sender, _ := cmd.Flags().GetString("sender-user-id")
		if strings.TrimSpace(sender) == "" {
			return originalRunE(cmd, args)
		}
		submission, err := reportSenderSubmissionFromFlags(cmd, sender)
		if err != nil {
			return err
		}
		return submitter.Submit(cmd.Context(), cmd, submission)
	}
}

func reportSenderSubmissionFromFlags(cmd *cobra.Command, sender string) (ReportSenderSubmission, error) {
	templateID, err := requiredReportStringFlag(cmd, "template-id")
	if err != nil {
		return ReportSenderSubmission{}, err
	}
	contents, err := reportContentsFromFlags(cmd)
	if err != nil {
		return ReportSenderSubmission{}, err
	}
	ddFrom := optionalReportStringFlag(cmd, "dd-from")
	if ddFrom == "" {
		ddFrom = "dws"
	}
	toChat := false
	if cmd.Flags().Lookup("to-chat") != nil {
		toChat, _ = cmd.Flags().GetBool("to-chat")
	}
	toUserIDs := parseUserIDs(optionalReportStringFlag(cmd, "to-user-ids"))
	return ReportSenderSubmission{
		SenderUserID: strings.TrimSpace(sender),
		TemplateID:   templateID,
		Contents:     contents,
		DDFrom:       ddFrom,
		ToChat:       toChat,
		ToUserIDs:    toUserIDs,
	}, nil
}

func requiredReportStringFlag(cmd *cobra.Command, name string) (string, error) {
	value := optionalReportStringFlag(cmd, name)
	if value == "" {
		return "", apperrors.NewValidation("--" + name + " is required")
	}
	return value, nil
}

func optionalReportStringFlag(cmd *cobra.Command, name string) string {
	if cmd == nil || cmd.Flags().Lookup(name) == nil {
		return ""
	}
	value, _ := cmd.Flags().GetString(name)
	return strings.TrimSpace(value)
}

func reportContentsFromFlags(cmd *cobra.Command) ([]map[string]any, error) {
	raw := ""
	if filePath := optionalReportStringFlag(cmd, "contents-file"); filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, apperrors.NewValidation(fmt.Sprintf("read --contents-file: %v", err))
		}
		raw = string(data)
	} else {
		raw = optionalReportStringFlag(cmd, "contents")
		if raw == "-" {
			data, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return nil, apperrors.NewValidation(fmt.Sprintf("read --contents from stdin: %v", err))
			}
			raw = string(data)
		}
	}
	if strings.TrimSpace(raw) == "" {
		return nil, apperrors.NewValidation("--contents or --contents-file is required")
	}
	var contents []map[string]any
	if err := json.Unmarshal([]byte(raw), &contents); err != nil {
		return nil, apperrors.NewValidation(fmt.Sprintf("report contents JSON parse failed: %v", err))
	}
	if len(contents) == 0 {
		return nil, apperrors.NewValidation("report contents must contain at least one item")
	}
	return contents, nil
}
