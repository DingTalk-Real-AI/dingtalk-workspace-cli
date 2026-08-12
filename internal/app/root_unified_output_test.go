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
	"errors"
	"os"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/spf13/cobra"
)

func TestExecuteEmitsStoredUnifiedResultAtSingleRootExit(t *testing.T) {
	oldNormalize := rootNormalizeProcessProfileArgs
	oldExecute := rootExecuteCommand
	oldNewRoot := rootNewRootCommandWithEngine
	oldPreParse := rootRunPreParse
	oldStop := rootStopAllStdioClients
	oldArgs := os.Args
	t.Cleanup(func() {
		rootNormalizeProcessProfileArgs = oldNormalize
		rootExecuteCommand = oldExecute
		rootNewRootCommandWithEngine = oldNewRoot
		rootRunPreParse = oldPreParse
		rootStopAllStdioClients = oldStop
		os.Args = oldArgs
	})
	os.Args = []string{"dws"}
	rootNormalizeProcessProfileArgs = func() func() { return func() {} }
	rootRunPreParse = func(*cobra.Command, *pipeline.Engine) error { return nil }
	rootStopAllStdioClients = func() {}
	rootNewRootCommandWithEngine = func(ctx context.Context, _ *pipeline.Engine) *cobra.Command {
		cmd := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
		cmd.SetContext(ctx)
		return cmd
	}

	var stdout, stderr bytes.Buffer
	executed := &cobra.Command{Use: "leaf"}
	output.SetCommandRollout(executed, output.RolloutUnifiedActive)
	executed.SetOut(&stdout)
	executed.SetErr(&stderr)
	rootExecuteCommand = func(root *cobra.Command) (*cobra.Command, error) {
		executed.SetContext(root.Context())
		if err := output.StoreResult(executed.Context(), output.Success(map[string]any{"id": "a"})); err != nil {
			return executed, err
		}
		if _, _, err := output.EmitStoredResult(executed); err != nil {
			return executed, err
		}
		return executed, nil
	}
	if code := Execute(); code != 0 {
		t.Fatalf("Execute code=%d, want 0", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr=%q, want diagnostics only/empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"outcome": "success"`) || strings.Contains(stdout.String(), `"contract_version"`) {
		t.Fatalf("stdout does not match the unified envelope: %s", stdout.String())
	}
}

// TestRootExecutionErrorToStderrOnly 是 B184 的回归断言：失败信封（JSON 错误
// 输出）恒走 stderr，stdout 严格为空（契约 §5.1：失败时 stdout 必须为空）。
// printExecutionError 把 PrintJSON/PrintHuman 都写 stderr writer，stdout
// writer 不得收到任何字节。
func TestRootExecutionErrorToStderrOnly(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().String("format", "json", "")
	_ = root.PersistentFlags().Set("format", "json")

	var stdout, stderr bytes.Buffer
	if err := printExecutionError(root, &stdout, &stderr, apperrors.NewAuth("token expired")); err != nil {
		t.Fatalf("printExecutionError() error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("failure must keep stdout empty, got %q", stdout.String())
	}
	want := "{\n  \"error\": {\n    \"category\": \"auth\",\n    \"code\": 2,\n    \"message\": \"token expired\"\n  }\n}\n"
	if got := stderr.String(); got != want {
		t.Fatalf("legacy root error wire changed\n got: %q\nwant: %q", got, want)
	}
}

// TestRootHumanErrorToStderrOnly 是 B184 的人类可读分支断言：非 JSON 模式下，
// 失败走 stderr（PrintHuman），stdout 为空。
func TestRootHumanErrorToStderrOnly(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().String("format", "table", "")
	_ = root.PersistentFlags().Set("format", "table")

	var stdout, stderr bytes.Buffer
	if err := printExecutionError(root, &stdout, &stderr, apperrors.NewInternal("boom")); err != nil {
		t.Fatalf("printExecutionError() error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("failure must keep stdout empty, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Error:") {
		t.Fatalf("expected human error on stderr, got %q", stderr.String())
	}
}

// TestRootExecuteOutcomeToExitCode 是 B185 的 Execute 出口断言：Execute 把
// 命令返回的 error 类别映射为进程退出码（apperrors.ExitCode）。ok→0、
// confirmation/validation→3、panic 与 unrepresentable partial error→5。
func TestRootExecuteOutcomeToExitCode(t *testing.T) {
	oldNormalize := rootNormalizeProcessProfileArgs
	oldExecute := rootExecuteCommand
	oldNewRoot := rootNewRootCommandWithEngine
	oldPreParse := rootRunPreParse
	oldStop := rootStopAllStdioClients
	oldArgs := os.Args
	t.Cleanup(func() {
		rootNormalizeProcessProfileArgs = oldNormalize
		rootExecuteCommand = oldExecute
		rootNewRootCommandWithEngine = oldNewRoot
		rootRunPreParse = oldPreParse
		rootStopAllStdioClients = oldStop
		os.Args = oldArgs
	})
	os.Args = []string{"dws"}
	rootNormalizeProcessProfileArgs = func() func() { return func() {} }
	rootRunPreParse = func(*cobra.Command, *pipeline.Engine) error { return nil }
	rootStopAllStdioClients = func() {}
	rootNewRootCommandWithEngine = func(context.Context, *pipeline.Engine) *cobra.Command {
		return &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	}

	// ok / pending（信封 success/pending 语义）→ 0
	rootExecuteCommand = func(*cobra.Command) (*cobra.Command, error) { return nil, nil }
	if code := Execute(); code != 0 {
		t.Fatalf("success Execute code = %d, want 0", code)
	}

	// An error cannot carry partial succeeded/failed data and fails closed.
	rootExecuteCommand = func(*cobra.Command) (*cobra.Command, error) {
		return nil, &apperrors.Error{Category: apperrors.CategoryPartial, Message: "partial"}
	}
	if code := Execute(); code != 5 {
		t.Fatalf("partial error Execute code = %d, want 5", code)
	}

	// confirmation_required（validation 子类）→ 3
	rootExecuteCommand = func(*cobra.Command) (*cobra.Command, error) {
		return nil, apperrors.NewValidation("blocked", apperrors.WithReason("confirmation_required"))
	}
	if code := Execute(); code != 3 {
		t.Fatalf("confirmation Execute code = %d, want 3", code)
	}

	// plain internal → 5
	rootExecuteCommand = func(*cobra.Command) (*cobra.Command, error) {
		return nil, errors.New("plain")
	}
	if code := Execute(); code != 5 {
		t.Fatalf("plain Execute code = %d, want 5", code)
	}
}

// TestRootSilenceErrorsAndDeferTeardown 是 B186 的断言：根命令 SilenceErrors/
// SilenceUsage 打开（Cobra 不自行打印），且 Execute 出口 defer 收尾路径
// （StopAllStdioClients）在错误路径也被调用。
func TestRootSilenceErrorsAndDeferTeardown(t *testing.T) {
	oldNormalize := rootNormalizeProcessProfileArgs
	oldExecute := rootExecuteCommand
	oldNewRoot := rootNewRootCommandWithEngine
	oldPreParse := rootRunPreParse
	oldStop := rootStopAllStdioClients
	oldArgs := os.Args
	t.Cleanup(func() {
		rootNormalizeProcessProfileArgs = oldNormalize
		rootExecuteCommand = oldExecute
		rootNewRootCommandWithEngine = oldNewRoot
		rootRunPreParse = oldPreParse
		rootStopAllStdioClients = oldStop
		os.Args = oldArgs
	})
	os.Args = []string{"dws"}
	rootNormalizeProcessProfileArgs = func() func() { return func() {} }
	rootRunPreParse = func(*cobra.Command, *pipeline.Engine) error { return nil }
	stopped := false
	rootStopAllStdioClients = func() { stopped = true }
	rootNewRootCommandWithEngine = func(context.Context, *pipeline.Engine) *cobra.Command {
		return &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	}

	// 错误路径：Execute 返回非零，且 defer 收尾（StopAllStdioClients）被调用。
	rootExecuteCommand = func(*cobra.Command) (*cobra.Command, error) {
		return nil, apperrors.NewInternal("fail")
	}
	_ = Execute()
	if !stopped {
		t.Fatal("defer teardown (StopAllStdioClients) not called on error path")
	}
}

// TestRootSilenceErrorsFlag 断言根命令的 SilenceErrors/SilenceUsage 为真，
// 保证 Cobra 不自行在错误时打印 usage/错误（错误渲染统一走 printExecutionError）。
func TestRootSilenceErrorsFlag(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	if !root.SilenceErrors || !root.SilenceUsage {
		t.Fatalf("root must set SilenceErrors=%v SilenceUsage=%v", root.SilenceErrors, root.SilenceUsage)
	}
}
