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
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/asynctask"
)

type asyncTaskMCPCall func(context.Context, string, string, map[string]any) (string, error)

func queryAsyncTask(ctx context.Context, taskType, id string) (asynctask.TaskResult, error) {
	return queryAsyncTaskWith(ctx, callAsyncTaskMCPToolReturnTextOnServer, taskType, id)
}

func callAsyncTaskMCPToolReturnTextOnServer(ctx context.Context, serverID, toolName string, args map[string]any) (string, error) {
	return callMCPToolReturnTextOnServerWithBusinessErrorClassifier(ctx, serverID, toolName, args, isBusinessErrorWithoutStatus)
}

func queryAsyncTaskWith(ctx context.Context, call asyncTaskMCPCall, taskType, id string) (asynctask.TaskResult, error) {
	var (
		tool string
		args map[string]any
	)
	switch taskType {
	case "export":
		tool, args = "query_export_job", map[string]any{"jobId": id}
	case "import":
		tool, args = "query_import_task", map[string]any{"taskId": id}
	default:
		return asynctask.TaskResult{}, fmt.Errorf("unsupported task type %q; expected export or import", taskType)
	}

	text, err := call(ctx, "doc", tool, args)
	if err != nil {
		return asynctask.TaskResult{}, err
	}
	return parseAsyncTaskResponse(text, taskType, id)
}

func parseAsyncTaskResponse(text, taskType, id string) (asynctask.TaskResult, error) {
	var response map[string]any
	if err := json.Unmarshal([]byte(text), &response); err != nil {
		return asynctask.TaskResult{}, fmt.Errorf("parse async task response: %w", err)
	}
	if err := asyncTaskBusinessError(response); err != nil {
		return asynctask.TaskResult{}, err
	}

	data := response
	if result, ok := response["result"].(map[string]any); ok {
		data = result
		if err := asyncTaskBusinessError(data); err != nil {
			return asynctask.TaskResult{}, err
		}
	}

	var resultURL string
	switch taskType {
	case "export":
		resultURL, _ = data["downloadUrl"].(string)
	case "import":
		resultURL, _ = data["documentUrl"].(string)
	default:
		return asynctask.TaskResult{}, fmt.Errorf("unsupported task type %q; expected export or import", taskType)
	}

	status, _ := data["status"].(string)
	message, _ := data["message"].(string)
	createTime, _ := data["createTime"].(string)
	return asynctask.TaskResult{
		ID:         id,
		Type:       taskType,
		Status:     asynctask.NormalizeStatus(status),
		ResultURL:  resultURL,
		Message:    message,
		CreateTime: createTime,
	}, nil
}

func asyncTaskBusinessError(response map[string]any) error {
	if !isBusinessErrorWithoutStatus(response) {
		return nil
	}
	message := businessErrorMessage(response)
	if message == "" {
		message = "async task query failed"
	}
	return errors.New(message)
}

func taskIDFromFlags(cmd *cobra.Command) (string, error) {
	if cmd.Flags().Changed("task-id") && cmd.Flags().Changed("job-id") {
		return "", errors.New("flags --task-id and --job-id cannot be used together")
	}
	return mustFlagOrFallback(cmd, "task-id", "job-id")
}
