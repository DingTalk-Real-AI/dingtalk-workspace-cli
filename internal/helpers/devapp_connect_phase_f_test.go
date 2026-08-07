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
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// phaseFConnectRoot 构造 stdout/stderr 分流的可观测命令树根，供 connect 族
// 流纪律断言使用（统一输出 dev 域试点，队列 B108/B109/B116）。
func phaseFConnectRoot(t *testing.T, runner *captureRunner) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	root := newDevAppTestRoot(runner)
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetIn(strings.NewReader(""))
	return root, &stdout, &stderr
}

type phaseFEnvelope struct {
	OK      bool           `json:"ok"`
	Outcome string         `json:"outcome"`
	DryRun  bool           `json:"dry_run"`
	Data    map[string]any `json:"data"`
}

func decodePhaseFEnvelope(t *testing.T, raw []byte) *phaseFEnvelope {
	t.Helper()
	var env phaseFEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("stdout is not a single valid JSON envelope: %v\n%s", err, raw)
	}
	if (env.Outcome == "success" || env.Outcome == "pending") && !env.OK {
		t.Fatalf("envelope invariant I1 violated: ok=false with outcome=%q\n%s", env.Outcome, raw)
	}
	return &env
}

// decodePhaseFConnectPreview parses the established legacy preview. The
// streaming root remains legacy while terminal children migrate independently.
func decodePhaseFConnectPreview(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("stdout is not a single valid JSON document: %v\n%s", err, raw)
	}
	preview, ok := document["invocation"].(map[string]any)
	if !ok {
		t.Fatalf("legacy connect preview missing invocation: %s", raw)
	}
	return preview
}

// TestDevConnectCustomChannelPreviewEnvelope 是队列 B108：--agent-cmd 自定义
// 渠道（custom）的 dry-run 预览信封断言。--agent-cmd 是 custom 渠道的语法糖：
// 未显式 --channel 时强制 custom，预览保持原有 invocation JSON shape。
func TestDevConnectCustomChannelPreviewEnvelope(t *testing.T) {
	clearChannelEnv(t)
	// RunE 内部用 os.Setenv 写 DWS_AGENT_CMD（不随 t.Setenv 恢复），显式清理防串测。
	t.Cleanup(func() { os.Unsetenv("DWS_AGENT_CMD") })

	root, stdout, stderr := phaseFConnectRoot(t, &captureRunner{})
	root.SetArgs([]string{
		"dev", "connect",
		"--agent-cmd", "lobster -p",
		"--robot-client-id", "cid-1",
		"--robot-client-secret", "sec-1",
		"--dry-run",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}

	data := decodePhaseFConnectPreview(t, stdout.Bytes())
	if data == nil {
		t.Fatalf("preview envelope data is nil: %s", stdout.String())
	}
	if data["channel"] != "custom" {
		t.Fatalf("channel = %#v, want custom (--agent-cmd 糖强制 custom): %s", data["channel"], stdout.String())
	}
	if data["kind"] != "connect_preview" || data["scope"] != "local_debug_only" {
		t.Fatalf("preview markers kind/scope = %#v/%#v: %s", data["kind"], data["scope"], stdout.String())
	}
	if data["clientId"] != "cid-1" {
		t.Fatalf("clientId = %#v, want cid-1: %s", data["clientId"], stdout.String())
	}
	if cred, _ := data["credentialSource"].(string); !strings.HasPrefix(cred, "flag:") {
		t.Fatalf("credentialSource = %#v, want flag:* 前缀: %s", data["credentialSource"], stdout.String())
	}
	if data["terminal"] != false || data["doesNotPublish"] != true {
		t.Fatalf("terminal/doesNotPublish = %#v/%#v, want false/true: %s",
			data["terminal"], data["doesNotPublish"], stdout.String())
	}
	if _, ok := data["connect"].(map[string]any); !ok {
		t.Fatalf("connect plan missing from preview data: %s", stdout.String())
	}

	// secret 落 argv 的人读警告只走 stderr；stdout 不得混入人读文案。
	if !strings.Contains(stderr.String(), "--robot-client-secret 出现在命令行") {
		t.Fatalf("argv secret warning missing from stderr: %q", stderr.String())
	}
	if strings.Contains(stdout.String(), "[connect]") {
		t.Fatalf("stdout must carry only the envelope, human-readable text leaked: %s", stdout.String())
	}
}

// TestDevConnectDryRunUnifiedAppIDPreviewEndToEnd 是队列 B116 的 dry-run
// 端到端分支：--unified-app-id 路径的 dry-run 必须跳过 credentials get
// （runner 零调用），预览信封如实标注凭证来源为「unified-app-id（skipped）」。
func TestDevConnectDryRunUnifiedAppIDPreviewEndToEnd(t *testing.T) {
	clearChannelEnv(t)
	runner := &captureRunner{}
	root, stdout, stderr := phaseFConnectRoot(t, runner)
	root.SetArgs([]string{
		"dev", "connect",
		"--channel", "hermes",
		"--unified-app-id", "u-9",
		"--dry-run",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}

	data := decodePhaseFConnectPreview(t, stdout.Bytes())
	if data == nil {
		t.Fatalf("preview envelope data is nil: %s", stdout.String())
	}
	if data["channel"] != "hermes" {
		t.Fatalf("channel = %#v, want hermes: %s", data["channel"], stdout.String())
	}
	if data["unifiedAppId"] != "u-9" {
		t.Fatalf("unifiedAppId = %#v, want u-9: %s", data["unifiedAppId"], stdout.String())
	}
	if data["credentialSource"] != "unified-app-id (credentials get, skipped in dry-run)" {
		t.Fatalf("credentialSource = %#v, want dry-run skipped marker: %s", data["credentialSource"], stdout.String())
	}
	// dry-run 禁止真实调用 credentials get。
	if runner.last.Tool != "" {
		t.Fatalf("dry-run must not invoke any tool, got %q", runner.last.Tool)
	}
}

// TestDevConnectForegroundStdoutDiscipline 是队列 B109：前台启动路径的
// channel/凭证来源提示行与本地调试声明保持 stderr，stdout 严格零字节
// （契约规范 §5.1：stdout 只承载数据；前台 connector 的后续输出由被 stub 的
// stream 层承载，本测试验证 dws 自身不向 stdout 写任何人读文案）。
func TestDevConnectForegroundStdoutDiscipline(t *testing.T) {
	clearChannelEnv(t)
	t.Setenv("DWS_CONNECT_CMD", "")
	t.Setenv("DWS_AGENT_CMD", "sh -c printf\\ ok")
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())

	origStream := devAppRunStreamConnector
	t.Cleanup(func() { devAppRunStreamConnector = origStream })
	devAppRunStreamConnector = func(context.Context, string, string, string, forwarder, *aiCardClient, *connectExtras) error {
		return nil
	}

	root, stdout, stderr := phaseFConnectRoot(t, &captureRunner{})
	root.SetArgs([]string{
		"dev", "connect",
		"--channel", "custom",
		"--robot-client-id", "cid-1",
		"--robot-client-secret", "sec-1",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}

	if stdout.Len() != 0 {
		t.Fatalf("foreground connect must keep stdout empty, got %q", stdout.String())
	}
	errText := stderr.String()
	for _, want := range []string{
		"[connect] channel=custom",
		"凭证来源=flag:--robot-client-id/--robot-client-secret",
		"不代表线上发布完成",
	} {
		if !strings.Contains(errText, want) {
			t.Fatalf("stderr missing %q:\n%s", want, errText)
		}
	}
}
