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
	"encoding/json"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

func init() {
	RegisterPublic(func() Handler {
		return aiappHandler{}
	})
}

type aiappHandler struct{}

func (aiappHandler) Name() string {
	return "aiapp"
}

func (aiappHandler) Command(runner executor.Runner) *cobra.Command {
	root := &cobra.Command{
		Use:               "aiapp",
		Short:             "AI 应用创建 / 查询 / 修改",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	root.AddCommand(
		newAIAppCreateCommand(runner),
		newAIAppQueryCommand(runner),
		newAIAppModifyCommand(runner),
	)
	return root
}

func newAIAppCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create",
		Short:             "创建 AI 应用",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			prompt, err := aiappRequiredFlag(cmd, "prompt")
			if err != nil {
				return err
			}
			params := map[string]any{"prompt": prompt}
			if err := addAIAppOptionalInputs(cmd, params); err != nil {
				return err
			}
			return runAIAppTool(cmd, runner, "create_ai_app", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("prompt", "", "创建 AI 应用的 prompt (必填)")
	cmd.Flags().String("attachments", "", "附件对象数组 JSON")
	cmd.Flags().String("skills", "", "技能 ID 列表，逗号分隔")
	return cmd
}

func newAIAppQueryCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "query",
		Short:             "查询 AI 应用任务",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID, err := aiappRequiredFlag(cmd, "task-id")
			if err != nil {
				return err
			}
			return runAIAppTool(cmd, runner, "query_ai_app", map[string]any{"taskId": taskID})
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("task-id", "", "AI 应用任务 ID (必填)")
	return cmd
}

func newAIAppModifyCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "modify",
		Short:             "修改 AI 应用",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			prompt, err := aiappRequiredFlag(cmd, "prompt")
			if err != nil {
				return err
			}
			threadID, err := aiappRequiredFlag(cmd, "thread-id")
			if err != nil {
				return err
			}
			params := map[string]any{
				"prompt":   prompt,
				"threadId": threadID,
			}
			if skills := aiappStringFlag(cmd, "skills"); skills != "" {
				params["officialSkillUids"] = aiappCSV(skills)
			}
			return runAIAppTool(cmd, runner, "modify_ai_app", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("prompt", "", "新的 prompt (必填)")
	cmd.Flags().String("thread-id", "", "threadId (必填)")
	cmd.Flags().String("skills", "", "技能 ID 列表，逗号分隔")
	return cmd
}

func addAIAppOptionalInputs(cmd *cobra.Command, params map[string]any) error {
	if attachments := aiappStringFlag(cmd, "attachments"); attachments != "" {
		var values []any
		if err := json.Unmarshal([]byte(attachments), &values); err != nil {
			return apperrors.NewValidation("--attachments must be a JSON array: " + err.Error())
		}
		if len(values) > 0 {
			params["attachments"] = values
		}
	}
	if skills := aiappStringFlag(cmd, "skills"); skills != "" {
		params["officialSkillUids"] = aiappCSV(skills)
	}
	return nil
}

func runAIAppTool(cmd *cobra.Command, runner executor.Runner, tool string, params map[string]any) error {
	invocation := executor.NewHelperInvocation(
		cobracmd.LegacyCommandPath(cmd),
		"aiapp",
		tool,
		params,
	)
	if commandDryRun(cmd) {
		return writeCommandPayload(cmd, invocation)
	}
	result, err := runner.Run(cmd.Context(), invocation)
	if err != nil {
		return err
	}
	return writeCommandPayload(cmd, result)
}

func aiappRequiredFlag(cmd *cobra.Command, name string) (string, error) {
	if value := aiappStringFlag(cmd, name); value != "" {
		return value, nil
	}
	return "", apperrors.NewValidation("--" + name + " is required")
}

func aiappStringFlag(cmd *cobra.Command, name string) string {
	if value, err := cmd.Flags().GetString(name); err == nil && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return ""
}

func aiappCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			values = append(values, value)
		}
	}
	return values
}
