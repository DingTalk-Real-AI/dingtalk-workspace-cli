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
	stderrors "errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
)

// runDevDomainErrorCase 在生产口径的静默根下执行错误路径用例（队列
// B128~B137）。生产根命令设 SilenceUsage/SilenceErrors=true
// （internal/app/root.go），错误时 Cobra 不向 stdout 打 usage/error；测试根
// 对齐同一口径，使「错误路径 stdout 零字节」断言与真实调用一致。
func runDevDomainErrorCase(t *testing.T, runner executor.Runner, args ...string) (*bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()
	root := newDevAppTestRoot(runner)
	root.SilenceUsage = true
	root.SilenceErrors = true
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetIn(strings.NewReader(""))
	root.SetArgs(args)
	err := root.Execute()
	return &out, &errBuf, err
}

// requireDevValidationError 断言错误是结构化 apperrors validation（AC-03）：
// Category=validation + rc=3 + 消息可定位。
func requireDevValidationError(t *testing.T, err error, wantMsg string) *apperrors.Error {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want structured validation error containing %q", wantMsg)
	}
	var appErr *apperrors.Error
	if !stderrors.As(err, &appErr) {
		t.Fatalf("error type = %T (%v), want structured *errors.Error", err, err)
	}
	if appErr.Category != apperrors.CategoryValidation {
		t.Fatalf("category = %q, want validation", appErr.Category)
	}
	if got := apperrors.ExitCode(err); got != 3 {
		t.Fatalf("exit code = %d, want 3 (validation)", got)
	}
	if !strings.Contains(err.Error(), wantMsg) {
		t.Fatalf("message = %q, want contains %q", err.Error(), wantMsg)
	}
	return appErr
}

// TestDevAppGetMissingLocatorStructuredValidation 是队列 B128：dev app get
// 无参数二选一报结构化 validation（rc=3），stdout 零字节。
func TestDevAppGetMissingLocatorStructuredValidation(t *testing.T) {
	out, _, err := runDevDomainErrorCase(t, &captureRunner{}, "dev", "app", "get")
	requireDevValidationError(t, err, "请传入 --unified-app-id 或 --app-key")
	if out.Len() != 0 {
		t.Fatalf("error path must keep stdout empty, got %q", out.String())
	}
}

// TestDevConnectUnknownChannelStructuredValidation 是队列 B129：dev connect
// 未知渠道报结构化 validation（NewValidation，rc=3），stdout 零字节。
func TestDevConnectUnknownChannelStructuredValidation(t *testing.T) {
	clearChannelEnv(t)
	out, _, err := runDevDomainErrorCase(t, &captureRunner{},
		"dev", "connect", "--channel", "bogus")
	requireDevValidationError(t, err, `未知渠道 "bogus"`)
	if out.Len() != 0 {
		t.Fatalf("error path must keep stdout empty, got %q", out.String())
	}
}

// TestDevConnectMissingCredentialsNonInteractiveStructuredValidation 是队列
// B130：非交互环境缺凭证不进引导流程，直接报结构化 validation（rc=3）。
// stdin 交互判定经 seam 显式置 false，保证脚本/CI 环境断言确定性。
func TestDevConnectMissingCredentialsNonInteractiveStructuredValidation(t *testing.T) {
	clearChannelEnv(t)
	origInteractive := devAppConnectStdinInteractive
	t.Cleanup(func() { devAppConnectStdinInteractive = origInteractive })
	devAppConnectStdinInteractive = func() bool { return false }

	out, _, err := runDevDomainErrorCase(t, &captureRunner{},
		"dev", "connect", "--channel", "hermes")
	requireDevValidationError(t, err, "需要 --robot-client-id/--robot-client-secret")
	if out.Len() != 0 {
		t.Fatalf("error path must keep stdout empty, got %q", out.String())
	}
}

// TestDevConnectRestartMissingRecordStructuredValidation 是队列 B131 的结构化
// 增量：restart 无连接器记录报结构化 validation（rc=3）。错误消息与 stdout 零
// 字节断言已由轮 11-B0 B116（TestConnectDaemonFamilyMissingDaemonErrorPaths）
// 覆盖，本测试补 AC-03 的 Category/rc 映射面。
func TestDevConnectRestartMissingRecordStructuredValidation(t *testing.T) {
	connectDaemonDirOverride = t.TempDir()
	t.Cleanup(func() { connectDaemonDirOverride = "" })

	out, _, err := runDevDomainErrorCase(t, &captureRunner{},
		"dev", "connect", "restart", "--robot-client-id", "ghost")
	requireDevValidationError(t, err, "未找到连接器记录")
	if out.Len() != 0 {
		t.Fatalf("error path must keep stdout empty, got %q", out.String())
	}
}

// TestDevConnectStatusDaemonDirResolutionFailure 是队列 B133：status 的
// daemon 目录解析失败（override 指向普通文件使 MkdirAll 失败）报结构化
// internal 错误（rc=5），stdout 零字节。
func TestDevConnectStatusDaemonDirResolutionFailure(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatalf("setup blocked file: %v", err)
	}
	connectDaemonDirOverride = blocked
	t.Cleanup(func() { connectDaemonDirOverride = "" })

	out, _, err := runDevDomainErrorCase(t, &captureRunner{},
		"dev", "connect", "status", "--robot-client-id", "ghost", "--json")
	if err == nil {
		t.Fatalf("error = nil, want daemon dir resolution failure")
	}
	var appErr *apperrors.Error
	if !stderrors.As(err, &appErr) {
		t.Fatalf("error type = %T (%v), want structured *errors.Error", err, err)
	}
	if appErr.Category != apperrors.CategoryInternal {
		t.Fatalf("category = %q, want internal", appErr.Category)
	}
	if got := apperrors.ExitCode(err); got != 5 {
		t.Fatalf("exit code = %d, want 5 (internal)", got)
	}
	if !strings.Contains(err.Error(), "resolve daemon dir") {
		t.Fatalf("message = %q, want contains resolve daemon dir", err.Error())
	}
	if out.Len() != 0 {
		t.Fatalf("error path must keep stdout empty, got %q", out.String())
	}
}

// TestDevAppCreateMissingNameErrorPath 是队列 B134：create 缺 --name 报错、
// stdout 零字节。LeafSpec RequiredHint 由 corecmd 统一转成 typed validation，
// 与手写参数校验共享 rc=3。
func TestDevAppCreateMissingNameErrorPath(t *testing.T) {
	out, _, err := runDevDomainErrorCase(t, &captureRunner{},
		"dev", "app", "create", "--yes")
	if err == nil || !strings.Contains(err.Error(), "--name 为必填") {
		t.Fatalf("error = %v, want --name 为必填", err)
	}
	var appErr *apperrors.Error
	if !stderrors.As(err, &appErr) || appErr.Category != apperrors.CategoryValidation {
		t.Fatalf("error = %T (%v), want typed validation", err, err)
	}
	if got := apperrors.ExitCode(err); got != 3 {
		t.Fatalf("exit code = %d, want 3 (validation)", got)
	}
	if out.Len() != 0 {
		t.Fatalf("error path must keep stdout empty, got %q", out.String())
	}
}

// TestDevAppVersionPublishMissingParamsErrorPath 是队列 B135：version publish
// 参数缺失报错、stdout 零字节。ValidateRequired 按声明顺序报第一个缺失的
// 必填参数（先 unified-app-id 后 version-id）；两者均为 typed validation rc=3。
func TestDevAppVersionPublishMissingParamsErrorPath(t *testing.T) {
	out, _, err := runDevDomainErrorCase(t, &captureRunner{},
		"dev", "app", "version", "publish", "--yes")
	if err == nil || !strings.Contains(err.Error(), "--unified-app-id 为必填") {
		t.Fatalf("error = %v, want --unified-app-id 为必填", err)
	}
	if got := apperrors.ExitCode(err); got != 3 {
		t.Fatalf("exit code = %d, want 3 (validation)", got)
	}
	if out.Len() != 0 {
		t.Fatalf("error path must keep stdout empty, got %q", out.String())
	}

	out2, _, err := runDevDomainErrorCase(t, &captureRunner{},
		"dev", "app", "version", "publish", "--yes", "--unified-app-id", "u-1")
	if err == nil || !strings.Contains(err.Error(), "--version-id 为必填") {
		t.Fatalf("error = %v, want --version-id 为必填", err)
	}
	if got := apperrors.ExitCode(err); got != 3 {
		t.Fatalf("exit code = %d, want 3 (validation)", got)
	}
	if out2.Len() != 0 {
		t.Fatalf("error path must keep stdout empty, got %q", out2.String())
	}
}

// TestDevAppDeleteDisableEnableMissingUnifiedAppID 是队列 B136：
// delete/disable/enable 缺统一应用标识报错、stdout 零字节，均由统一 required
// preflight 返回 typed validation rc=3。
func TestDevAppDeleteDisableEnableMissingUnifiedAppID(t *testing.T) {
	for _, leaf := range []string{"delete", "disable", "enable"} {
		t.Run(leaf, func(t *testing.T) {
			out, _, err := runDevDomainErrorCase(t, &captureRunner{},
				"dev", "app", leaf, "--yes")
			if err == nil || !strings.Contains(err.Error(), "--unified-app-id 为必填") {
				t.Fatalf("error = %v, want --unified-app-id 为必填", err)
			}
			var appErr *apperrors.Error
			if !stderrors.As(err, &appErr) || appErr.Category != apperrors.CategoryValidation {
				t.Fatalf("error = %T (%v), want typed validation", err, err)
			}
			if got := apperrors.ExitCode(err); got != 3 {
				t.Fatalf("exit code = %d, want 3 (validation)", got)
			}
			if out.Len() != 0 {
				t.Fatalf("error path must keep stdout empty, got %q", out.String())
			}
		})
	}
}

// TestDevDomainErrorPathsKeepStdoutEmpty 是队列 B137 的总断言（AC-11）：
// dev 域全部错误路径 stdout 严格零字节。dev 域失败走 apperrors 通道（生产
// root 错误处理器写 stderr），Phase F 不引入 failure 信封（轮 8 裁决⑪），
// 故 AC-11 在测试层的投影即「stdout 零字节」。各用例的结构化/消息断言见
// 上述分测试，本表只锁流纪律。
func TestDevDomainErrorPathsKeepStdoutEmpty(t *testing.T) {
	daemonDirSetup := func(t *testing.T) {
		t.Helper()
		connectDaemonDirOverride = t.TempDir()
		t.Cleanup(func() { connectDaemonDirOverride = "" })
	}
	cases := []struct {
		name  string
		args  []string
		setup func(t *testing.T)
	}{
		{"app get missing locator", []string{"dev", "app", "get"}, nil},
		{"app update no field", []string{"dev", "app", "update", "--unified-app-id", "u-1", "--dry-run"}, nil},
		{"app create missing name", []string{"dev", "app", "create", "--yes"}, nil},
		{"app delete missing id", []string{"dev", "app", "delete", "--yes"}, nil},
		{"app disable missing id", []string{"dev", "app", "disable", "--yes"}, nil},
		{"app enable missing id", []string{"dev", "app", "enable", "--yes"}, nil},
		{"version publish missing params", []string{"dev", "app", "version", "publish", "--yes"}, nil},
		{"connect unknown channel", []string{"dev", "connect", "--channel", "bogus"}, nil},
		{"connect status missing locator", []string{"dev", "connect", "status"}, nil},
		{"connect restart missing record", []string{"dev", "connect", "restart", "--robot-client-id", "ghost"}, daemonDirSetup},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setup != nil {
				tc.setup(t)
			}
			out, _, err := runDevDomainErrorCase(t, &captureRunner{}, tc.args...)
			if err == nil {
				t.Fatalf("want error, got nil\nstdout:\n%s", out.String())
			}
			if out.Len() != 0 {
				t.Fatalf("error path must keep stdout empty (AC-11), got %q", out.String())
			}
		})
	}
}
