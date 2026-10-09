package helpers

import (
	"context"
	"os"
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
