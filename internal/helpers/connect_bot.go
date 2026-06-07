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
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

// Robot provisioning is a two-step async flow on the open-platform
// app-management MCP server: submit_robot_create_task returns a taskId, then
// query_robot_create_result is polled until the task reaches a terminal state.
// The async pair (replacing the old one-shot create_dingtalk_robot) lets the
// server dedupe by taskId so a retry never creates a second robot.
const (
	robotCreateSubmitTool = "submit_robot_create_task"
	robotCreateQueryTool  = "query_robot_create_result"

	// Poll cadence guards: honor the server-provided interval but keep it sane.
	robotCreatePollMinInterval = 1 * time.Second
	robotCreatePollMaxInterval = 30 * time.Second
	// Fallbacks when the submit response omits interval / expiresIn (seconds).
	robotCreateDefaultInterval = 3 * time.Second
	robotCreateDefaultDeadline = 5 * time.Minute
)

// runRobotCreateTool routes a robot-provisioning tool to the open-platform
// app-management MCP server via CanonicalProduct "opendev". That product's
// endpoint is hardcoded in internal/app/direct_runtime.go (NOT resolved from
// service discovery), so this command works without any discovery/overlay entry.
func runRobotCreateTool(runner executor.Runner, cmd *cobra.Command, tool string, params map[string]any, dryRun bool) (executor.Result, error) {
	invocation := executor.NewHelperInvocation(
		cobracmd.LegacyCommandPath(cmd),
		"opendev",
		tool,
		params,
	)
	invocation.DryRun = dryRun
	return runner.Run(cmd.Context(), invocation)
}

// robotCreateProvision submits an async robot-create task and polls for its
// result until SUCCESS / FAIL / EXPIRED (or the deadline). On success it writes
// the full query payload (agentId / robotCode / clientId / clientSecret). On
// FAIL / EXPIRED it returns an error carrying the taskId so the caller can retry
// with --task-id without creating a duplicate robot.
func robotCreateProvision(runner executor.Runner, cmd *cobra.Command, submitParams map[string]any) error {
	dryRun := commandDryRun(cmd)

	submitRes, err := runRobotCreateTool(runner, cmd, robotCreateSubmitTool, submitParams, dryRun)
	if err != nil {
		return err
	}
	// Dry-run only previews the submit routing; there is no real taskId to poll.
	if dryRun {
		return writeCommandPayload(cmd, submitRes)
	}

	submitPayload := robotCreatePayload(submitRes.Response)
	taskID := robotResultString(submitPayload, "taskId")
	if taskID == "" {
		// Server returned an inline (already-terminal) result without a taskId;
		// surface it verbatim rather than poll a task that does not exist.
		return writeCommandPayload(cmd, submitRes)
	}

	interval := robotResultDuration(submitPayload, "interval", robotCreateDefaultInterval)
	if interval < robotCreatePollMinInterval {
		interval = robotCreatePollMinInterval
	}
	if interval > robotCreatePollMaxInterval {
		interval = robotCreatePollMaxInterval
	}
	deadline := robotResultDuration(submitPayload, "expiresIn", robotCreateDefaultDeadline)

	ctx := cmd.Context()
	elapsed := time.Duration(0)
	queryParams := map[string]any{"taskId": taskID}
	for {
		if err := robotCreateSleepFn(ctx, interval); err != nil {
			return err
		}
		elapsed += interval

		queryRes, err := runRobotCreateTool(runner, cmd, robotCreateQueryTool, queryParams, false)
		if err != nil {
			return err
		}
		queryPayload := robotCreatePayload(queryRes.Response)
		switch strings.ToUpper(robotResultString(queryPayload, "status")) {
		case "SUCCESS":
			return writeCommandPayload(cmd, queryRes)
		case "FAIL", "EXPIRED":
			status := strings.ToUpper(robotResultString(queryPayload, "status"))
			return apperrors.NewInternal(fmt.Sprintf(
				"robot creation %s (taskId=%s); retry with: dws connect bot create ... --task-id %s",
				status, taskID, taskID))
		case "WAITING", "":
			// keep polling
		default:
			// Unknown terminal-ish status: surface the raw payload.
			return writeCommandPayload(cmd, queryRes)
		}

		if elapsed >= deadline {
			return apperrors.NewInternal(fmt.Sprintf(
				"robot creation still WAITING after %s (taskId=%s); check later or retry with: dws connect bot create ... --task-id %s",
				deadline, taskID, taskID))
		}
	}
}

// robotCreateSleepFn is the poll-wait function; overridable in tests so the
// polling loop can run without real delays.
var robotCreateSleepFn = robotCreateSleep

// robotCreateSleep waits for d or until the context is cancelled.
func robotCreateSleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// robotCreatePayload unwraps the executor/MCP envelope so callers can read
// taskId / status / agentId from the innermost object. The real shape is
// Response{"content":{"errorCode","errorMsg","success","result":{...}}}, so we
// descend through "content" and then "result", tolerating either wrapper being
// absent.
func robotCreatePayload(resp map[string]any) map[string]any {
	cur := resp
	if cur == nil {
		return nil
	}
	if inner, ok := cur["content"].(map[string]any); ok {
		cur = inner
	}
	if inner, ok := cur["result"].(map[string]any); ok {
		cur = inner
	}
	return cur
}

// robotResultString reads a string field from an MCP response map, tolerating
// nil maps and non-string scalars.
func robotResultString(resp map[string]any, key string) string {
	if resp == nil {
		return ""
	}
	switch v := resp[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		return ""
	}
}

// robotResultDuration reads a numeric field as a second-count duration, falling
// back to def when the field is missing or unparseable.
func robotResultDuration(resp map[string]any, key string, def time.Duration) time.Duration {
	if resp == nil {
		return def
	}
	switch v := resp[key].(type) {
	case float64:
		if v > 0 {
			return time.Duration(v) * time.Second
		}
	case int:
		if v > 0 {
			return time.Duration(v) * time.Second
		}
	case json.Number:
		if n, err := v.Float64(); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return def
}

// newConnectBotCreateCommand creates `dws connect bot create`, the robot-provisioning
// command. It submits an async robot-create task (submit_robot_create_task) and
// blocks while polling query_robot_create_result until the task reaches a
// terminal state, then returns agentId / robotCode / clientId / clientSecret
// (clientSecret is returned only once). corpId and userid are injected
// server-side from the current login. If creation FAILs / EXPIREs, re-run with
// --task-id <id> to retry without creating a duplicate robot.
func newConnectBotCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create",
		Short:             "创建钉钉智能体机器人",
		Long:              "创建企业自建 Agent 应用及承载机器人。服务端异步建号，本命令会阻塞轮询直到成功，返回 agentId / robotCode / clientId / clientSecret。⚠️ clientSecret 仅返回一次，请立即安全保存。corpId 和 userid 由 MCP 服务端按当前登录身份注入。建号失败时用 --task-id <上次返回的 taskId> 重试，可避免重复建号。",
		Example:           "  dws connect bot create --app-name \"销售助手\" --robot-name \"销售助手机器人\" --desc \"销售线索查询与客户跟进\"",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			appName, _ := cmd.Flags().GetString("app-name")
			robotName, _ := cmd.Flags().GetString("robot-name")
			desc, _ := cmd.Flags().GetString("desc")
			if strings.TrimSpace(appName) == "" {
				return apperrors.NewValidation("--app-name is required")
			}
			if strings.TrimSpace(robotName) == "" {
				return apperrors.NewValidation("--robot-name is required")
			}
			if strings.TrimSpace(desc) == "" {
				return apperrors.NewValidation("--desc is required")
			}
			params := map[string]any{
				"appName":   appName,
				"robotName": robotName,
				"desc":      desc,
			}
			if v, _ := cmd.Flags().GetString("robot-media-id"); strings.TrimSpace(v) != "" {
				params["robotMediaId"] = v
			}
			if v, _ := cmd.Flags().GetString("preview-media-id"); strings.TrimSpace(v) != "" {
				params["previewMediaId"] = v
			}
			if v, _ := cmd.Flags().GetString("task-id"); strings.TrimSpace(v) != "" {
				params["taskId"] = strings.TrimSpace(v)
			}
			return robotCreateProvision(runner, cmd, params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("app-name", "", "智能体应用名称，2~20 字，企业内唯一 (必填)")
	cmd.Flags().String("robot-name", "", "承载机器人名称，2~20 字 (必填)")
	cmd.Flags().String("desc", "", "机器人功能描述，≤200 字 (必填)")
	cmd.Flags().String("robot-media-id", "", "机器人图标 mediaId（可选，留空用服务端默认图标）")
	cmd.Flags().String("preview-media-id", "", "机器人预览图 mediaId（可选，留空复用 --robot-media-id）")
	cmd.Flags().String("task-id", "", "重试用：上次建号返回的 taskId，避免重复建号（可选）")
	return cmd
}
