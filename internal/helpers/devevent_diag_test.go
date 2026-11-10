package helpers

// 实机诊断 `dws dev event` 命令组（devevent.go）背后的 devapp op-app 网关。
// 仅在 DWS_EVENT_LIVE_SMOKE=1 时运行，否则整文件 skip。诊断性质：只发读请求
// 加一个「明显不存在的工具名」对照探测，不做任何业务写操作。
//
// 生产派发路径（app.runtimeRunner）在本包不可构造（会构成 import 环），所以
// 原始信封探测按 runner 的同款输入手工组装：真实 ~/.dws 配置与 keychain
// token、devapp 网关端点（mcp_url 主机映射到 *-gw 再拼 /server/op-app）、
// transport.Client 同款鉴权形态。走真实生产路径（NewRootCommand → runner →
// detectBusinessError）的全量 QC 在 internal/app/devevent_live_test.go。

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/publishedmcp"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

const deveventDiagUnifiedAppIDEnv = "DWS_LIVE_APP"

// deveventDiagServerPath 对齐 app 侧 devappServerPath（该常量未导出）。
const deveventDiagServerPath = "/server/op-app"

// deveventDiagLiveConfigDir 解除 internal/app 式的配置隔离，指向真实 ~/.dws，
// 让 OAuth/keychain 读到真实 token。绝不 t.Parallel。
func deveventDiagLiveConfigDir(t *testing.T) string {
	t.Helper()
	if os.Getenv("DWS_EVENT_LIVE_SMOKE") == "" {
		t.Skip("live diagnostics only: set DWS_EVENT_LIVE_SMOKE=1")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("resolve home: %v", err)
	}
	configDir := filepath.Join(home, ".dws")
	t.Setenv("DWS_CONFIG_DIR", configDir)
	t.Setenv(keychain.StorageDirEnv, "")
	os.Unsetenv(keychain.StorageDirEnv)
	return configDir
}

// deveventDiagDevappEndpoint 复刻 app 侧 devappMCPEndpoint 的推导：
// mcp_url 主机映射到网关主机（pre-mcp.* → pre-mcp-gw.*），再拼 op-app 路径。
func deveventDiagDevappEndpoint(t *testing.T) string {
	t.Helper()
	raw := strings.TrimSpace(authpkg.GetMCPBaseURL())
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		t.Fatalf("configured MCP base URL %q is unusable: %v", raw, err)
	}
	host := parsed.Hostname()
	switch {
	case host == "mcp.dingtalk.com":
		host = "mcp-gw.dingtalk.com"
	case strings.HasPrefix(host, "pre-mcp."):
		host = "pre-mcp-gw." + strings.TrimPrefix(host, "pre-mcp.")
	case strings.HasPrefix(host, "mcp."):
		host = "mcp-gw." + strings.TrimPrefix(host, "mcp.")
	}
	endpoint := parsed.Scheme + "://" + host + deveventDiagServerPath
	t.Logf("gateway endpoint = %s (mcp_url = %s)", endpoint, raw)
	return endpoint
}

// deveventDiagToken 按 app 侧 TokenManager 的解析顺序取 token：OAuth 快照优先，
// 失败回退 legacy manager。日志只记录长度，绝不打印 token 本体。
func deveventDiagToken(t *testing.T, configDir string) string {
	t.Helper()
	provider := authpkg.NewOAuthProvider(configDir, slog.New(slog.NewTextHandler(io.Discard, nil)))
	data, err := provider.GetTokenSnapshot(context.Background())
	if err == nil && data != nil && strings.TrimSpace(data.AccessToken) != "" {
		token := strings.TrimSpace(data.AccessToken)
		t.Logf("token: oauth snapshot len=%d expiresAt=%v loginRegion=%q", len(token), data.ExpiresAt, data.LoginRegion)
		return token
	}
	t.Logf("oauth snapshot unavailable (%v); falling back to legacy manager", err)
	token, source, legacyErr := authpkg.NewManager(configDir, nil).GetToken()
	if legacyErr != nil || strings.TrimSpace(token) == "" {
		t.Fatalf("no usable credentials: oauth=%v legacy=%v", err, legacyErr)
	}
	t.Logf("token: legacy manager source=%q len=%d", source, len(token))
	return strings.TrimSpace(token)
}

// deveventDiagClient 组装与 executeInvocation 同形态的鉴权客户端：身份头 +
// edition 固定 claw-type + bearer token。
func deveventDiagClient(t *testing.T, configDir, token string) *transport.Client {
	t.Helper()
	headers := map[string]string{}
	if id := authpkg.EnsureExists(configDir); id != nil {
		for key, value := range id.Headers() {
			headers[key] = value
		}
	}
	headers["claw-type"] = edition.DefaultOSSClawType
	return transport.NewClient(nil).WithAuth(token, headers)
}

// deveventDiagLogToolCall 发一次 tools/call 并打印脱敏摘要：错误类型、IsError、
// 内容块数量、文本摘要与服务端诊断代码；不输出完整业务响应。
func deveventDiagLogToolCall(t *testing.T, client *transport.Client, endpoint, tool string, params map[string]any) transport.ToolCallResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	start := time.Now()
	result, err := client.CallTool(ctx, endpoint, tool, params)
	t.Logf("tools/call %s finished in %s", tool, time.Since(start))
	t.Logf("transport error = %v", err)
	if err != nil {
		t.Logf("transport error (+%%v) = %+v", err)
	}
	t.Logf("result.IsError = %v", result.IsError)
	t.Logf("result content blocks = %d", len(result.Blocks))
	for i, block := range result.Blocks {
		text := strings.TrimSpace(block.Text)
		if len(text) > 120 {
			text = text[:120] + "..."
		}
		t.Logf("result.Blocks[%d] type=%q text=%q", i, block.Type, text)
	}
	diag := transport.ExtractServerDiagnosticsFromMap(result.Content)
	t.Logf("server diagnostics: error_code=%q error_type=%q reason=%q operation=%q", diag.ServerErrorCode, diag.ErrorType, diag.Reason, diag.Operation)
	return result
}

// deveventDiagLogLeafProjection 预演 helpers 侧对该载荷的失败投影（生产路径上
// runner 会先在 detectBusinessError 处拦截，载荷到不了这里；此预演只回答
// 「若载荷可达，CLI 会怎样归一」）。
func deveventDiagLogLeafProjection(t *testing.T, tool string, content map[string]any, params map[string]any) {
	t.Helper()
	result := executor.Result{
		Invocation: executor.Invocation{
			Implemented:      true,
			Kind:             "helper_invocation",
			CanonicalProduct: devEventProduct,
			Tool:             tool,
			Params:           params,
		},
		Response: map[string]any{"content": content},
	}
	result = normalizeDevAppServiceResult(result)
	data := devAppEnvelopeData(result)
	object, ok := data.(map[string]any)
	if !ok {
		t.Logf("leaf projection preview: payload is %T, not an object", data)
		return
	}
	if failure := devAppFailureResult(object); failure != nil {
		info := devEventErrorInfo(object)
		t.Logf("leaf projection preview: FAILURE subtype=%q upstreamCode=%v hint=%q detail_count=%d",
			info.Subtype, info.UpstreamCode, info.Hint, len(info.Details))
		return
	}
	t.Logf("leaf projection preview: payload projects as SUCCESS (no failure markers)")
}

// TestDevEventDiagLiveRawEnvelope 绕开叶子封装，直接对 devapp 网关调用
// list_dev_app_event_definitions，参数与 devevent.go 的 flag 绑定一致
// （--unified-app-id → unifiedAppId；page-size 默认 20 → pageSize）。
func TestDevEventDiagLiveRawEnvelope(t *testing.T) {
	configDir := deveventDiagLiveConfigDir(t)
	appID := strings.TrimSpace(os.Getenv(deveventDiagUnifiedAppIDEnv))
	if appID == "" {
		t.Skipf("live diagnostics require %s", deveventDiagUnifiedAppIDEnv)
	}
	endpoint := deveventDiagDevappEndpoint(t)
	token := deveventDiagToken(t, configDir)
	client := deveventDiagClient(t, configDir, token)

	params := map[string]any{
		"unifiedAppId": appID,
		"pageSize":     20,
	}
	raw, _ := json.Marshal(params)
	t.Logf("request params = %s", raw)

	probe := deveventDiagLogToolCall(t, client, endpoint, devEventListTool, params)
	content := probe.Content
	if content == nil {
		content = map[string]any{"_note": "nil content returned by gateway"}
	}
	deveventDiagLogLeafProjection(t, devEventListTool, content, params)

	// 决定性对照：服务端 tools/list 里真实存在的列表工具名。若它不再返回
	// TOOL_NOT_FOUND，即证明能力在服务端已注册，只是 devevent.go 的工具名
	// 常量与注册名不一致。
	t.Logf("--- contrast probe: server-registered tool %q with same params ---", "list_event_definitions")
	deveventDiagLogToolCall(t, client, endpoint, "list_event_definitions", params)
}

// TestDevEventDiagLiveToolRegistry 对 devapp 网关发 tools/list，判断 8 条远端
// dev event 工具是否真的注册在该服务上；本地 lint 不参与 tools/list；再用一个明显不存在的工具名对比
// 「工具未注册」与「业务 success=false」两种错误形态。
func TestDevEventDiagLiveToolRegistry(t *testing.T) {
	configDir := deveventDiagLiveConfigDir(t)
	appID := strings.TrimSpace(os.Getenv(deveventDiagUnifiedAppIDEnv))
	if appID == "" {
		t.Skipf("live diagnostics require %s", deveventDiagUnifiedAppIDEnv)
	}
	endpoint := deveventDiagDevappEndpoint(t)
	token := deveventDiagToken(t, configDir)

	tools, err := publishedmcp.New(transport.NewClient(nil), token, nil).Tools(context.Background(), endpoint)
	if err != nil {
		t.Fatalf("tools/list on %s failed: %+v", endpoint, err)
	}
	t.Logf("tools/list: total %d tools on devapp gateway", len(tools.Tools))

	registered := map[string]bool{}
	eventLike := make([]string, 0, 9)
	for _, tool := range tools.Tools {
		registered[tool.Name] = true
		if strings.Contains(strings.ToLower(tool.Name), "event") {
			eventLike = append(eventLike, tool.Name)
		}
	}
	t.Logf("tool names containing %q: %v", "event", eventLike)

	for _, tool := range []string{
		devEventListTool,
		devEventGetTool,
		devEventCreateTool,
		devEventUpdateTool,
		devEventValidateTool,
		devEventDebugTool,
		devEventPublishTool,
		devEventOfflineTool,
		// devapp 既有工具作阳性对照。
		devAppListTool,
	} {
		status := "ABSENT"
		if registered[tool] {
			status = "registered"
		}
		t.Logf("tool registry: %-42s %s", tool, status)
	}

	client := deveventDiagClient(t, configDir, token)
	t.Logf("--- control probe: nonexistent tool (error-shape comparison) ---")
	deveventDiagLogToolCall(t, client, endpoint, "dev_event_diag_no_such_tool__", map[string]any{
		"unifiedAppId": appID,
	})
}
