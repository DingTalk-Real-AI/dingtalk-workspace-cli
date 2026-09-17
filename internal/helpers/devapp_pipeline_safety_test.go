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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

func newPipelineLeafCmd(format string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	cmd := &cobra.Command{Use: "leaf"}
	cmd.Flags().String("format", format, "")
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	return cmd, &out, &errOut
}

// runDevAppFamilyProdAligned 是 B118/B119/B120 共用的生产对齐 runner：
// 在 newDevAppTestRoot 基础上补齐生产根命令缺的全局 --jq/--fields flag 与
// SilenceUsage=true（对齐 internal/app/root.go bindPersistentFlags + 根命令
// SilenceUsage），并注入--json 简写布尔。这样 --jq/--fields 可用、错误路径
// 不把 usage 打到 stdout（管道安全断言的前提）。
func runDevAppFamilyProdAligned(t *testing.T, runner executor.Runner, args ...string) (*bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()
	// 复用与生产一致的 ResultStore/PostRun emitter，再补齐其余全局 flag。
	root := newDevAppTestRoot(runner)
	root.SilenceUsage = true
	root.SilenceErrors = true
	root.PersistentFlags().String("fields", "", "筛选输出字段 (逗号分隔, 如: name,id,status)")
	root.PersistentFlags().String("jq", "", "jq 表达式过滤输出 (如: '.items[] | .name')")
	root.PersistentFlags().Bool("json", false, "以 JSON 输出")
	return runRootBuffered(t, root, args...)
}

// runRootBuffered 在 stdout/stderr 分流下执行任意 root，返回两路缓冲与错误。
func runRootBuffered(t *testing.T, root *cobra.Command, args ...string) (*bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetIn(strings.NewReader(""))
	root.SetArgs(args)
	err := root.Execute()
	return &out, &errBuf, err
}

// TestDevPipelineSafetyStdoutJSONParseable 是队列 B118 的管道安全断言（成功
// 路径）：stdout 必须恰为一个可 JSON 解析的完整信封，且默认 json 输出与
// --jq 提取同一字段等价（jq 等价断言）。这保证 `dws ... | jq .` 管道不会因
// 日志/人读杂音混入 stdout 而解析失败。
func TestDevPipelineSafetyStdoutJSONParseable(t *testing.T) {
	content := map[string]any{
		"unifiedAppId": "u-1",
		"name":         "DemoApp",
		"appStatus":    "ENABLED",
	}

	// 成功叶：stdout 恰为一个 JSON 信封（无日志字节污染）。
	out, errBuf, err := runDevAppFamily(t, devAppFamilyContentRunner(content),
		"dev", "app", "get", "--unified-app-id", "u-1")
	if err != nil {
		t.Fatalf("Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, out.String(), errBuf.String())
	}
	var env map[string]any
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not parseable as a single JSON envelope: %v\n%s", err, out.String())
	}
	if env["ok"] != true || env["outcome"] != "success" {
		t.Fatalf("envelope = %#v", env)
	}

	// jq 等价：--jq .data.name 必须与信封 data.name 一致。
	jqOut, jqErr, err := runDevAppFamilyProdAligned(t, devAppFamilyContentRunner(content),
		"dev", "app", "get", "--unified-app-id", "u-1", "--jq", ".data.name")
	if err != nil {
		t.Fatalf("Execute(--jq) error = %v\nstdout:\n%s\nstderr:\n%s", err, jqOut.String(), jqErr.String())
	}
	if strings.TrimSpace(jqOut.String()) != `"DemoApp"` {
		t.Fatalf("--jq .data.name = %q, want \"DemoApp\"", jqOut.String())
	}
}

// TestDevPipelineSafetyFailureStdoutZeroBytes 是队列 B118 的失败路径管道安全
// 断言：参数校验失败错误走 apperrors 通道（不进信封），stdout 必须严格零
// 字节——`dws ... | jq .` 在失败时不应把错误细节混进 stdout 数据流。
// （失败细节由生产 Execute 的 printExecutionError 落 stderr——本测试聚焦
// stdout 数据通道纯净性，与轮11-B0 族错误路径口径一致。）
func TestDevPipelineSafetyFailureStdoutZeroBytes(t *testing.T) {
	out, _, err := runDevAppFamilyProdAligned(t, &captureRunner{},
		"dev", "app", "get") // 缺 --unified-app-id / --app-key
	if err == nil || !strings.Contains(err.Error(), "请传入 --unified-app-id 或 --app-key") {
		t.Fatalf("Execute() error = %v, want locator validation", err)
	}
	if out.Len() != 0 {
		t.Fatalf("validation failure must keep stdout zero bytes (pipeline safety): got %q", out.String())
	}
}

// TestDevPipelineSafetyFailureEnvelopeJSONOnStderr 是队列 B118 的失败信封
// 管道安全断言：显式信封装失败（如 Confirmation 门禁）经错误通道落 stderr 且
// 为完整 JSON 信封，stdout 零字节。错误信封不得被 jq/format 过滤为空。
func TestDevPipelineSafetyFailureEnvelopeJSONOnStderr(t *testing.T) {
	// 写叶子真实执行（无 --dry-run、无 --yes）触发 confirmation_required 门禁。
	// 信封经 writeEnvelope → Emitter 落 stderr（stdout 严格零字节）。
	out, _, err := runDevAppFamilyProdAligned(t,
		devAppFamilyContentRunner(map[string]any{"unifiedAppId": "u-1", "name": "x"}),
		"dev", "app", "update", "--unified-app-id", "u-1", "--name", "x")
	if err == nil {
		t.Fatalf("update without --yes must fail, got nil")
	}
	if out.Len() != 0 {
		t.Fatalf("confirmation failure must keep stdout zero bytes, got %q", out.String())
	}
	// 错误路径完整 JSON 信封落在 stderr（结构化错误通道）。面板级观察：
	// 此处直接复用 writeEnvelope 的 Emitter 语义做单测视角——先验证
	// writeEnvelope 对失败信封的 stderr 输出为 JSON，再端到端断言 stdout 零字节。
	cmd, cmdOut, cmdErr := newPipelineLeafCmd("json")
	cmd.SilenceUsage = true
	cmd.SetOut(cmdOut)
	cmd.SetErr(cmdErr)
	failEnv := output.NewFailureEnvelope(&output.ErrorInfo{Type: "permission", Subtype: "confirmation_required", Message: "need confirmation"})
	if werr := writeEnvelope(cmd, failEnv); werr != nil {
		t.Fatalf("writeEnvelope failure envelope error = %v", werr)
	}
	if cmdOut.Len() != 0 {
		t.Fatalf("writeEnvelope failure must keep stdout zero bytes, got %q", cmdOut.String())
	}
	var env struct {
		OK      bool   `json:"ok"`
		Outcome string `json:"outcome"`
	}
	if jerr := json.Unmarshal(cmdErr.Bytes(), &env); jerr != nil {
		t.Fatalf("stderr is not a JSON envelope: %v\n%s", jerr, cmdErr.String())
	}
	if env.OK || env.Outcome != "failure" {
		t.Fatalf("stderr envelope ok/outcome = %v/%q, want false/failure", env.OK, env.Outcome)
	}
}

var _ = output.FormatJSON
