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
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/spf13/cobra"
)

func TestConnectStopAndRestartDoNotRequireConfirmation(t *testing.T) {
	for _, cmd := range []*cobra.Command{
		newDevAppRobotConnectStopCommand(),
		newDevAppRobotConnectRestartCommand(),
	} {
		final, ok := contractfinal.RuntimeContractFinal(cmd)
		if !ok || final.Safety == nil {
			t.Fatalf("%s missing final safety declaration", cmd.Name())
		}
		if got := final.Safety.Confirmation; got != "not_required" {
			t.Fatalf("%s confirmation = %q, want not_required", cmd.Name(), got)
		}
	}
}

// TestConnectDaemonFamilyMissingDaemonErrorPaths 是队列 B116 的「daemon 不存在
// 错误路径」分支：对不存在的守护进程，status/stop 不得把它当成失败（空态是合法
// 载荷，AC-06——status 保持已发布的顶层 JSON 并如实标注 not_running），restart 因无法安全
// 重建而报 validation 错误，缺定位标识则统一报 validation（错误路径继续走
// apperrors 通道，不进信封）。命令级端到端，stdout/stderr 分流断言。
func TestConnectDaemonFamilyMissingDaemonErrorPaths(t *testing.T) {
	connectDaemonDirOverride = t.TempDir()
	t.Cleanup(func() { connectDaemonDirOverride = "" })

	// status：daemon 不存在 → 兼容顶层 JSON，state=not_running（非错误）。
	status := prepareUnifiedTestCommand(newDevAppRobotConnectStatusCommand())
	var statusOut, statusErr bytes.Buffer
	status.SetOut(&statusOut)
	status.SetErr(&statusErr)
	status.SetArgs([]string{"--robot-client-id", "ghost", "--json"})
	if err := status.Execute(); err != nil {
		t.Fatalf("status on missing daemon must not error, got %v\nstderr:\n%s", err, statusErr.String())
	}
	var statusReport connectHealthReport
	if err := json.Unmarshal(statusOut.Bytes(), &statusReport); err != nil {
		t.Fatalf("status output is not the published top-level JSON: %v\n%s", err, statusOut.String())
	}
	if statusReport.State != healthNotRunning {
		t.Fatalf("status state = %q, want %q: %s", statusReport.State, healthNotRunning, statusOut.String())
	}
	if strings.Contains(statusOut.String(), `"outcome"`) || strings.Contains(statusOut.String(), `"data"`) {
		t.Fatalf("status compatibility JSON was enveloped: %s", statusOut.String())
	}

	// stop：daemon 不存在 → ok:true 信封，data.status=not_running；人读行走 stderr。
	stop := prepareUnifiedTestCommand(newDevAppRobotConnectStopCommand())
	var stopOut, stopErr bytes.Buffer
	stop.SetOut(&stopOut)
	stop.SetErr(&stopErr)
	stop.SetArgs([]string{"--unified-app-id", "ghost"})
	if err := stop.Execute(); err != nil {
		t.Fatalf("stop on missing daemon must not error, got %v\nstderr:\n%s", err, stopErr.String())
	}
	stopEnv := decodePhaseFEnvelope(t, stopOut.Bytes())
	if !stopEnv.OK || stopEnv.Outcome != "success" {
		t.Fatalf("stop envelope ok/outcome = %v/%q, want true/success: %s",
			stopEnv.OK, stopEnv.Outcome, stopOut.String())
	}
	if st, _ := stopEnv.Data["status"].(string); st != "not_running" {
		t.Fatalf("stop data.status = %q, want not_running: %s", st, stopOut.String())
	}
	if !strings.Contains(stopErr.String(), "not running") {
		t.Fatalf("stop human-readable line missing from stderr: %q", stopErr.String())
	}

	// restart：daemon 不存在 → validation 错误（无法安全重建），非信封。
	// SilenceUsage 对齐生产根命令（internal/app/root.go：SilenceUsage=true），
	// 否则单叶子 Execute() 报错时 Cobra 默认把 usage 打 stdout，污染断言。
	restart := prepareUnifiedTestCommand(newDevAppRobotConnectRestartCommand())
	restart.SilenceUsage = true
	var restartOut, restartErr bytes.Buffer
	restart.SetOut(&restartOut)
	restart.SetErr(&restartErr)
	restart.SetArgs([]string{"--robot-client-id", "ghost"})
	err := restart.Execute()
	if err == nil || !strings.Contains(err.Error(), "未找到连接器记录") {
		t.Fatalf("restart on missing daemon error = %v, want 未找到连接器记录", err)
	}
	if restartOut.Len() != 0 {
		t.Fatalf("restart error path must keep stdout empty, got %q", restartOut.String())
	}
}

// TestConnectDaemonFamilyRequiresLocatorIdentity 是队列 B116 的配套断言：
// status/stop/restart 无定位标识（--robot-client-id / --unified-app-id 均缺）
// 时统一报 validation 错误，错误路径不进信封、stdout 零字节。
func TestConnectDaemonFamilyRequiresLocatorIdentity(t *testing.T) {
	connectDaemonDirOverride = t.TempDir()
	t.Cleanup(func() { connectDaemonDirOverride = "" })

	for _, build := range []func() *cobra.Command{
		func() *cobra.Command { return newDevAppRobotConnectStatusCommand() },
		func() *cobra.Command { return newDevAppRobotConnectStopCommand() },
		func() *cobra.Command { return newDevAppRobotConnectRestartCommand() },
	} {
		cmd := prepareUnifiedTestCommand(build())
		cmd.SilenceUsage = true
		var out, errBuf bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errBuf)
		cmd.SetArgs(nil)
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "需要 --robot-client-id 或 --unified-app-id") {
			t.Fatalf("%s without locator error = %v, want 定位守护进程 validation", cmd.Name(), err)
		}
		if out.Len() != 0 {
			t.Fatalf("%s validation error must keep stdout empty, got %q", cmd.Name(), out.String())
		}
	}
}
