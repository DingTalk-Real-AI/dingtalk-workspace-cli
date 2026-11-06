package helpers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 显式 opt-in 的真实 Agent 验收，正常 CI 不调用模型或读取模型凭据。
// 它证明模型调用和连续会话；不能替代钉钉真实消息上下行验收。
func TestEmployeeLiveAgentRoundTrip(t *testing.T) {
	channels := os.Getenv("DWS_EMPLOYEE_LIVE_CHANNELS")
	if channels == "" {
		t.Skip("NOT VERIFIED: set DWS_EMPLOYEE_LIVE_CHANNELS to run authorized real Agents")
	}
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	for _, channel := range strings.Split(channels, ",") {
		t.Run(channel, func(t *testing.T) {
			cfg := digitalEmployeeAdapterConfig{Binding: digitalEmployeeBinding{Channel: channel, DWSProfile: "live-test:isolated-employee"}, Options: connectAgentOptions{WorkDir: t.TempDir(), Memory: true, Timeout: 60 * time.Second}}
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
			defer cancel()
			fwd, err := digitalEmployeeNewForwarder(ctx, cfg)
			if err != nil {
				t.Fatalf("NOT VERIFIED: initialization failed (%T)", err)
			}
			if closer, ok := fwd.(forwarderCloser); ok {
				defer closer.close()
			}
			if err := prepareEmployeeForwarder(ctx, fwd); err != nil {
				t.Fatalf("NOT VERIFIED: Agent preflight failed (%T)", err)
			}
			answer, err := forwardEmployeeTurn(ctx, fwd, "isolated-conversation", "这是纯文本验收，请不要调用任何工具、不要访问文件或执行命令。记住校验数字731824，只回复731824。")
			if err != nil || !strings.Contains(answer, "731824") {
				t.Fatalf("NOT VERIFIED: first turn failed, errType=%T chars=%d", err, len(answer))
			}
			answer, err = forwardEmployeeTurn(ctx, fwd, "isolated-conversation", "不要调用工具。上轮要求记住的校验数字是什么？只回复数字。")
			if err != nil || !strings.Contains(answer, "731824") {
				t.Fatalf("NOT VERIFIED: session continuation failed, errType=%T chars=%d", err, len(answer))
			}
		})
	}
}

// 显式授权的真实 Codex 验收。保留线程 ID 供桌面权限同步的后续实测使用。
func TestEmployeeLivePermissionRoundTrip(t *testing.T) {
	report := os.Getenv("DWS_EMPLOYEE_PERMISSION_LIVE_REPORT")
	if report == "" {
		t.Skip("set DWS_EMPLOYEE_PERMISSION_LIVE_REPORT to opt in")
	}
	root := filepath.Dir(report)
	t.Setenv("DWS_CONFIG_DIR", filepath.Join(root, "permission-live-store"))
	work := filepath.Join(root, "permission-live-workdir")
	if err := os.MkdirAll(work, 0700); err != nil {
		t.Fatal(err)
	}
	cfg := digitalEmployeeAdapterConfig{Binding: digitalEmployeeBinding{Channel: "codex", DWSProfile: "live-test:permission-employee"}, Options: connectAgentOptions{WorkDir: work, Memory: true, Timeout: 90 * time.Second}}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	fwd, err := digitalEmployeeNewForwarder(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	const conversation = "permission-default-test"
	prompts := []string{"这是权限同步验收，请不要调用任何工具或读写文件，记住数字 719351，只回复719351。", "不要调用工具。上轮要求记住的数字是什么？只回复数字。"}
	if prompt := os.Getenv("DWS_EMPLOYEE_PERMISSION_LIVE_PROMPT"); prompt != "" {
		prompts = []string{prompt}
	}
	for _, prompt := range prompts {
		answer, err := fwd.forward(ctx, conversation, prompt)
		if err != nil || !strings.Contains(answer, "719351") {
			t.Fatalf("真实 Codex 会话验收失败: %T", err)
		}
	}
	thread := fwd.(*codexAppServerForwarder).sessions.threadID(conversation)
	data, _ := json.Marshal(map[string]any{"threadId": thread, "turns": len(prompts), "contextRetained": true})
	if err := os.WriteFile(report, data, 0600); err != nil {
		t.Fatal(err)
	}
}
