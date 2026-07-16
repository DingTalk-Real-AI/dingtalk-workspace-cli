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
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

// connectCreds carries credentials resolved by the interactive onboarding flow.
type connectCreds struct {
	clientID     string
	clientSecret string
	source       string // how they were resolved, for the [connect] log line
}

var (
	connectStdinStat       = func() (os.FileInfo, error) { return os.Stdin.Stat() }
	connectOnboardingSleep = sleepCtx
)

// connectStdinInteractive reports whether stdin is a real terminal (char
// device), so the onboarding prompt only runs interactively and never blocks a
// script, a daemon, or a piped invocation — those keep the original
// "need a flag" error.
func connectStdinInteractive() bool {
	fi, err := connectStdinStat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// runConnectOnboarding interactively resolves robot credentials when `connect`
// was run without any (no --robot-client-id/secret, no --unified-app-id). It
// asks whether to provision a NEW robot app or reuse an EXISTING one, then
// collects the minimum input per path. in/out are injected for testability.
//
// The "new app" path has a REAL side effect: it provisions a real robot app via
// the dev-app create/poll tools. The "existing app" path only reads identifiers
// and reuses the same credentials path as --unified-app-id.
func runConnectOnboarding(runner executor.Runner, cmd *cobra.Command, in io.Reader, out io.Writer) (connectCreds, error) {
	r := bufio.NewReader(in)
	fmt.Fprintln(out, "未检测到机器人凭证，进入建联引导。")
	choice, err := connectPromptLine(r, out, "新建应用还是已有应用？输入 1=新建应用并建号，2=使用已有应用 [2]: ")
	if err != nil {
		return connectCreds{}, err
	}
	switch choice {
	case "1":
		return onboardNewApp(runner, cmd, r, out)
	case "2", "":
		return onboardExistingApp(runner, cmd, r, out)
	default:
		return connectCreds{}, apperrors.NewValidation(fmt.Sprintf("无效选项 %q：只能输入 1 或 2", choice))
	}
}

// connectPromptLine writes prompt to out and reads one trimmed line from r.
func connectPromptLine(r *bufio.Reader, out io.Writer, prompt string) (string, error) {
	fmt.Fprint(out, prompt)
	line, err := r.ReadString('\n')
	if err != nil && line == "" {
		return "", apperrors.NewValidation("读取输入失败或已取消")
	}
	return strings.TrimSpace(line), nil
}

// onboardExistingApp resolves credentials for an existing robot app: it asks for
// a unified app id (reusing the --unified-app-id credentials path) or, failing
// that, an explicit clientId/clientSecret pair.
func onboardExistingApp(runner executor.Runner, cmd *cobra.Command, r *bufio.Reader, out io.Writer) (connectCreds, error) {
	appID, err := connectPromptLine(r, out, "输入统一应用 ID（unified-app-id，留空改为直接填凭证）: ")
	if err != nil {
		return connectCreds{}, err
	}
	if appID != "" {
		id, secret, ferr := devAppFetchCredentials(runner, cmd, appID)
		if ferr != nil {
			return connectCreds{}, ferr
		}
		if id == "" || secret == "" {
			return connectCreds{}, apperrors.NewInternal("credentials get 未返回 clientId/clientSecret；clientSecret 可能仅建号时返回一次，请改用现成机器人凭证")
		}
		return connectCreds{clientID: id, clientSecret: secret, source: "onboarding:unified-app-id"}, nil
	}
	id, _ := connectPromptLine(r, out, "输入机器人 clientId（AppKey）: ")
	secret, _ := connectPromptLine(r, out, "输入机器人 clientSecret（AppSecret）: ")
	if id == "" || secret == "" {
		return connectCreds{}, apperrors.NewValidation("clientId/clientSecret 不能为空")
	}
	return connectCreds{clientID: id, clientSecret: secret, source: "onboarding:robot-client"}, nil
}

// onboardNewApp provisions a NEW robot app: it collects the app name, robot
// display name and description, submits the async create task, then polls for
// the result to obtain credentials. This creates a real robot app.
func onboardNewApp(runner executor.Runner, cmd *cobra.Command, r *bufio.Reader, out io.Writer) (connectCreds, error) {
	name, _ := connectPromptLine(r, out, "应用名称（2-20 字，企业内唯一）: ")
	robotName, _ := connectPromptLine(r, out, "机器人名称（客户端展示用）: ")
	desc, _ := connectPromptLine(r, out, "机器人功能描述: ")
	if name == "" || robotName == "" || desc == "" {
		return connectCreds{}, apperrors.NewValidation("应用名称、机器人名称、功能描述均为必填")
	}
	fmt.Fprintln(out, "正在提交建号任务…（这会创建一个真实的机器人应用）")
	taskID, err := submitRobotCreateTask(runner, cmd, name, robotName, desc)
	if err != nil {
		return connectCreds{}, err
	}
	if taskID == "" {
		return connectCreds{}, apperrors.NewInternal("建号任务未返回 taskId")
	}
	fmt.Fprintf(out, "建号任务已提交 taskId=%s，等待结果…\n", taskID)
	return pollRobotCreateResult(runner, cmd, taskID, out)
}

// submitRobotCreateTask calls the async robot-create tool and returns its taskId.
func submitRobotCreateTask(runner executor.Runner, cmd *cobra.Command, name, robotName, desc string) (string, error) {
	params := map[string]any{
		"name":           name,
		"robotName":      robotName,
		"desc":           desc,
		"iconMediaId":    "",
		"previewMediaId": "",
	}
	inv := executor.NewHelperInvocation(cobracmd.LegacyCommandPath(cmd), devAppProduct, devAppRobotSubmitTool, params)
	res, err := runner.Run(cmd.Context(), inv)
	if err != nil {
		return "", err
	}
	payload := devAppConnectUnwrap(res.Response)
	return devAppConnectFirst(payload, "taskId", "taskID", "task_id"), nil
}

// pollRobotCreateResult polls the async create-result tool until credentials are
// available or the attempt budget is exhausted. Provisioning may take a few
// seconds (and possibly admin approval), so it polls with a bounded budget and
// degrades to an actionable hint on timeout.
func pollRobotCreateResult(runner executor.Runner, cmd *cobra.Command, taskID string, out io.Writer) (connectCreds, error) {
	const maxAttempts = 10
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		inv := executor.NewHelperInvocation(cobracmd.LegacyCommandPath(cmd), devAppProduct, devAppRobotResultTool, map[string]any{"taskId": taskID})
		res, err := runner.Run(cmd.Context(), inv)
		if err != nil {
			return connectCreds{}, err
		}
		payload := devAppConnectUnwrap(res.Response)
		id := devAppConnectFirst(payload, "clientId", "appKey", "clientID")
		secret := devAppConnectFirst(payload, "clientSecret", "appSecret")
		if id != "" && secret != "" {
			return connectCreds{clientID: id, clientSecret: secret, source: "onboarding:provisioned"}, nil
		}
		status := devAppConnectFirst(payload, "status", "taskStatus", "state")
		if isRobotCreateFailed(status) {
			return connectCreds{}, apperrors.NewInternal(fmt.Sprintf("建号失败，taskId=%s 状态=%s", taskID, status))
		}
		if attempt < maxAttempts {
			fmt.Fprintf(out, "  建号进行中（%d/%d，状态=%s），稍候…\n", attempt, maxAttempts, status)
			_ = connectOnboardingSleep(cmd.Context(), 3*time.Second)
		}
	}
	return connectCreds{}, apperrors.NewInternal(fmt.Sprintf("建号结果轮询超时，taskId=%s；可稍后用 `dws dev app robot result --task-id %s` 继续查询", taskID, taskID))
}

func isRobotCreateFailed(status string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	return s == "fail" || s == "failed" || s == "error"
}
