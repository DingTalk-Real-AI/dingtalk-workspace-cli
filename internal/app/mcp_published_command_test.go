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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/publishedmcp"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type mcpPublishedTestTransport struct {
	listEndpoint string
	callEndpoint string
	callTool     string
	callArgs     map[string]any
	listResult   transport.ToolsListResult
	callResult   transport.ToolCallResult
	listErr      error
	callErr      error
	listContext  context.Context
	callContext  context.Context
	factoryCtx   context.Context
}

func (c *mcpPublishedTestTransport) Tools(ctx context.Context, endpoint string) (transport.ToolsListResult, error) {
	c.listContext = ctx
	c.listEndpoint = endpoint
	return c.listResult, c.listErr
}

func (c *mcpPublishedTestTransport) InvokeValidated(ctx context.Context, endpoint, tool string, args map[string]any) (publishedmcp.ValidatedInvocationResult, error) {
	c.callContext = ctx
	c.callEndpoint = endpoint
	c.callTool = tool
	c.callArgs = args
	if c.listErr != nil {
		return publishedmcp.ValidatedInvocationResult{}, fmt.Errorf("发现已发布 MCP 工具: %w", c.listErr)
	}
	if c.callErr != nil {
		return publishedmcp.ValidatedInvocationResult{}, fmt.Errorf("调用已发布 MCP 工具: %w", c.callErr)
	}
	return publishedmcp.ValidatedInvocationResult{
		InputSchemaValidation: "fresh_core_subset_snapshot",
		InputSchemaDigest:     strings.Repeat("a", 64),
		Result:                c.callResult,
	}, nil
}

func executeMCPPublishedCommand(
	t *testing.T,
	caller edition.ToolCaller,
	client *mcpPublishedTestTransport,
	args ...string,
) (string, error) {
	t.Helper()
	root := &cobra.Command{Use: "mcp", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.PersistentFlags().Int("timeout", 30, "")
	root.AddCommand(newMCPPublishedGroup(caller, func(ctx context.Context) (mcpPublishedTransport, error) {
		client.factoryCtx = ctx
		return client, nil
	}))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(""))
	root.SetArgs(args)
	ctx, _ := output.WithResultStore(t.Context())
	executed, err := corecmd.ExecuteContextCForTest(root, ctx)
	if err == nil && executed != nil {
		_, _, err = output.EmitStoredResult(executed)
	}
	return out.String(), err
}

func publishedURLCaller() *mcpURLTestCaller {
	return &mcpURLTestCaller{
		result: &edition.ToolResult{Content: []edition.ContentBlock{{
			Type: "text",
			Text: `{"result":{"mcpURL":"https://example.test/path-secret?key=secret&token=private"}}`,
		}}},
	}
}

func publishedSearchTool() transport.ToolDescriptor {
	return transport.ToolDescriptor{
		Name: "search",
		InputSchema: map[string]any{
			"type":     "object",
			"required": []any{"query"},
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
			},
		},
	}
}

func TestCrossPlatformCoverageMCPPublishedToolsResolvesIdentityEndpointAndRedactsOutput(t *testing.T) {
	client := &mcpPublishedTestTransport{
		listResult: transport.ToolsListResult{Tools: []transport.ToolDescriptor{{
			Name: "search", Description: "Search records",
		}}},
	}
	caller := publishedURLCaller()
	out, err := executeMCPPublishedCommand(t, caller, client, "published", "tools", "2480")
	if err != nil {
		t.Fatalf("execute published tools: %v", err)
	}
	if caller.args["mcpId"] != "2480" {
		t.Fatalf("meta mcpId = %#v", caller.args["mcpId"])
	}
	if client.listEndpoint != "https://example.test/path-secret?key=secret&token=private" {
		t.Fatalf("list endpoint = %q", client.listEndpoint)
	}
	if strings.Contains(out, "secret") || strings.Contains(out, "private") {
		t.Fatalf("output leaked endpoint credentials: %s", out)
	}
	if !strings.Contains(out, `"toolCount": 1`) {
		t.Fatalf("output missing tool count: %s", out)
	}
}

func TestCrossPlatformCoverageMCPPublishedInvokeDryRunDoesNotResolveOrCall(t *testing.T) {
	client := &mcpPublishedTestTransport{}
	out, err := executeMCPPublishedCommand(
		t,
		nil,
		client,
		"--dry-run", "published", "invoke", "2480", "search", "--params", `{"query":"example"}`,
	)
	if err != nil {
		t.Fatalf("execute published invoke dry-run: %v", err)
	}
	if client.callEndpoint != "" {
		t.Fatalf("dry-run called endpoint %q", client.callEndpoint)
	}
	if client.listEndpoint != "" {
		t.Fatalf("dry-run discovered endpoint %q", client.listEndpoint)
	}
	if client.factoryCtx != nil {
		t.Fatal("dry-run constructed an authenticated published client")
	}
	if !strings.Contains(out, `"dry_run": true`) || !strings.Contains(out, `"executed": false`) {
		t.Fatalf("dry-run output missing evidence: %s", out)
	}
}

func TestCrossPlatformCoverageMCPPublishedInvokeRequiresConfirmationBeforeResolution(t *testing.T) {
	client := &mcpPublishedTestTransport{}
	_, err := executeMCPPublishedCommand(
		t,
		nil,
		client,
		"published", "invoke", "2480", "search", "--params", `{"query":"example"}`,
	)
	if err == nil || !strings.Contains(err.Error(), "需要用户确认") {
		t.Fatalf("error = %v, want confirmation_required", err)
	}
	if client.callEndpoint != "" {
		t.Fatalf("unconfirmed invocation called endpoint %q", client.callEndpoint)
	}
	if client.listEndpoint != "" {
		t.Fatalf("unconfirmed invocation discovered endpoint %q", client.listEndpoint)
	}
	if client.factoryCtx != nil {
		t.Fatal("unconfirmed invocation constructed an authenticated published client")
	}
}

func TestCrossPlatformCoverageMCPPublishedInvokeConfirmedCallsSelectedTool(t *testing.T) {
	client := &mcpPublishedTestTransport{
		listResult: transport.ToolsListResult{Tools: []transport.ToolDescriptor{publishedSearchTool()}},
		callResult: transport.ToolCallResult{
			StructuredContent: map[string]any{"items": []any{"one"}},
		},
	}
	out, err := executeMCPPublishedCommand(
		t,
		publishedURLCaller(),
		client,
		"--yes", "published", "invoke", "2480", "search", "--params", `{"query":"example"}`,
	)
	if err != nil {
		t.Fatalf("execute confirmed published invoke: %v", err)
	}
	if client.callTool != "search" || client.callArgs["query"] != "example" {
		t.Fatalf("call = tool %q args %#v", client.callTool, client.callArgs)
	}
	if client.callEndpoint != "https://example.test/path-secret?key=secret&token=private" {
		t.Fatalf("operation endpoint = %q", client.callEndpoint)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if payload["tool"] != "search" {
		t.Fatalf("output tool = %#v", payload["tool"])
	}
	if payload["inputSchemaValidation"] != "fresh_core_subset_snapshot" {
		t.Fatalf("schema validation evidence = %#v", payload["inputSchemaValidation"])
	}
	if payload["inputSchemaDigest"] != strings.Repeat("a", 64) {
		t.Fatalf("schema digest evidence = %#v", payload["inputSchemaDigest"])
	}
	if _, exposed := payload["endpoint"]; exposed {
		t.Fatal("published invoke output exposed endpoint")
	}
	if len(payload) != 5 {
		t.Fatalf("published invoke security-migration keys = %#v", payload)
	}
	remote, _ := payload["result"].(map[string]any)
	structured, _ := remote["structuredContent"].(map[string]any)
	if items, ok := structured["items"].([]any); !ok || len(items) != 1 || items[0] != "one" {
		t.Fatalf("arbitrary remote result = %#v", remote)
	}
}

type mcpPublishedDeadlineCaller struct {
	*mcpURLTestCaller
	ctx   context.Context
	calls int
}

func (c *mcpPublishedDeadlineCaller) CallTool(ctx context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.ctx = ctx
	c.calls++
	return c.mcpURLTestCaller.CallTool(ctx, productID, toolName, args)
}

func TestCrossPlatformCoverageMCPPublishedOperationsPropagateOneParsedTimeoutContext(t *testing.T) {
	for _, tt := range []struct {
		name           string
		timeoutSeconds int
		args           []string
	}{
		{name: "tools", timeoutSeconds: 7, args: []string{"--timeout", "7", "published", "tools", "2480"}},
		{name: "confirmed invoke", timeoutSeconds: 11, args: []string{"--timeout", "11", "--yes", "published", "invoke", "2480", "search", "--params", `{"query":"example"}`}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			caller := &mcpPublishedDeadlineCaller{mcpURLTestCaller: publishedURLCaller()}
			client := &mcpPublishedTestTransport{
				listResult: transport.ToolsListResult{Tools: []transport.ToolDescriptor{publishedSearchTool()}},
			}
			before := time.Now()
			if _, err := executeMCPPublishedCommand(t, caller, client, tt.args...); err != nil {
				t.Fatalf("execute: %v", err)
			}
			after := time.Now()
			resolverDeadline, resolverOK := caller.ctx.Deadline()
			factoryDeadline, factoryOK := client.factoryCtx.Deadline()
			operationCtx := client.listContext
			if tt.name == "confirmed invoke" {
				operationCtx = client.callContext
			}
			operationDeadline, operationOK := operationCtx.Deadline()
			if !resolverOK || !factoryOK || !operationOK {
				t.Fatalf("deadlines resolver=%t factory=%t operation=%t", resolverOK, factoryOK, operationOK)
			}
			if !resolverDeadline.Equal(factoryDeadline) || !resolverDeadline.Equal(operationDeadline) {
				t.Fatalf("deadlines resolver=%s factory=%s operation=%s", resolverDeadline, factoryDeadline, operationDeadline)
			}
			parsedTimeout := time.Duration(tt.timeoutSeconds) * time.Second
			if resolverDeadline.Before(before.Add(parsedTimeout)) || resolverDeadline.After(after.Add(parsedTimeout)) {
				t.Fatalf("deadline %s does not reflect parsed timeout %s within [%s,%s]", resolverDeadline, parsedTimeout, before, after)
			}
			if caller.calls != 1 {
				t.Fatalf("endpoint resolutions = %d, want exactly one", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageMCPPublishedInvokeUsesPublishedClientForLaterPageValidation(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     int            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		methods = append(methods, request.Method)
		var result any
		switch request.Method {
		case "tools/list":
			if request.Params["cursor"] == "page-2" {
				result = map[string]any{"tools": []map[string]any{{
					"name": "search",
					"inputSchema": map[string]any{
						"type": "object", "required": []any{"query"},
						"properties": map[string]any{"query": map[string]any{"type": "string"}},
					},
				}}}
			} else {
				result = map[string]any{
					"tools":      []map[string]any{{"name": "other", "inputSchema": map[string]any{"type": "object"}}},
					"nextCursor": "page-2",
				}
			}
		case "tools/call":
			result = map[string]any{"content": map[string]any{"found": true}}
		default:
			t.Fatalf("unexpected method %q", request.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	defer server.Close()
	caller := &mcpURLTestCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{
		Type: "text", Text: fmt.Sprintf(`{"result":{"mcpURL":%q}}`, server.URL),
	}}}}
	base := transport.NewClient(server.Client())
	base.TrustedDomains = []string{"127.0.0.1"}
	root := &cobra.Command{Use: "mcp", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.PersistentFlags().Int("timeout", 30, "")
	root.AddCommand(newMCPPublishedGroup(caller, func(context.Context) (mcpPublishedTransport, error) {
		return publishedmcp.New(base, "", nil), nil
	}))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--yes", "published", "invoke", "2480", "search", "--params", `{"query":"example"}`})
	ctx, _ := output.WithResultStore(t.Context())
	executed, err := corecmd.ExecuteContextCForTest(root, ctx)
	if err == nil {
		_, _, err = output.EmitStoredResult(executed)
	}
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := strings.Join(methods, ","); got != "tools/list,tools/list,tools/call" {
		t.Fatalf("methods = %q", got)
	}
	if !strings.Contains(out.String(), `"inputSchemaDigest"`) {
		t.Fatalf("output missing digest: %s", out.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	remote, _ := payload["result"].(map[string]any)
	content, _ := remote["content"].(map[string]any)
	if content["found"] != true {
		t.Fatalf("object-form call result was not preserved: %#v", remote)
	}
}

func TestCrossPlatformCoverageMCPPublishedInvokeResultContractMatchesPayload(t *testing.T) {
	cmd := newMCPPublishedInvokeCommand(nil, func(context.Context) (mcpPublishedTransport, error) {
		return &mcpPublishedTestTransport{}, nil
	})
	final, ok := contractfinal.RuntimeContractFinal(cmd)
	if !ok || final.Result == nil {
		t.Fatal("published invoke is missing Result contract")
	}
	var schema map[string]any
	if err := json.Unmarshal(final.Result.DataSchema, &schema); err != nil {
		t.Fatalf("decode Result schema: %v", err)
	}
	properties, _ := schema["properties"].(map[string]any)
	want := []string{"mcpId", "tool", "kind", "dry_run", "executed", "product", "arguments", "inputSchemaValidation", "inputSchemaDigest", "result"}
	for _, name := range want {
		property, ok := properties[name].(map[string]any)
		if !ok || strings.TrimSpace(fmt.Sprint(property["description"])) == "" {
			t.Errorf("Result property %q missing description: %#v", name, properties[name])
		}
	}

	deliveredTool := fullSchemaSnapshotForTest(t).Tools["mcp.published_invoke"]
	if _, exposed := deliveredTool["result"]; exposed {
		t.Fatalf("dual_validate command exposed Result before unified activation: %#v", deliveredTool["result"])
	}
	if state := output.CommandRollout(cmd); state != output.RolloutDualValidate {
		t.Fatalf("output rollout = %q, want dual_validate", state)
	}
}

func TestCrossPlatformCoverageMCPPublishedInvokeRejectsTrailingParamsJSON(t *testing.T) {
	_, err := executeMCPPublishedCommand(
		t,
		publishedURLCaller(),
		&mcpPublishedTestTransport{},
		"published", "invoke", "2480", "search", "--params", `{} {}`,
	)
	if err == nil || !strings.Contains(err.Error(), "--params 必须是 JSON 对象") {
		t.Fatalf("error = %v, want trailing params validation", err)
	}
}

func TestCrossPlatformCoverageMCPPublishedInvokeRejectsDuplicateParamsJSON(t *testing.T) {
	_, err := executeMCPPublishedCommand(
		t,
		publishedURLCaller(),
		&mcpPublishedTestTransport{},
		"published", "invoke", "2480", "search", "--params", `{"query":"first","query":"second"}`,
	)
	if err == nil || !strings.Contains(err.Error(), "--params 必须是 JSON 对象") {
		t.Fatalf("error = %v, want duplicate params validation", err)
	}
}

func TestCrossPlatformCoverageMCPPublishedInvokeRejectsOversizedParams(t *testing.T) {
	_, err := executeMCPPublishedCommand(
		t,
		publishedURLCaller(),
		&mcpPublishedTestTransport{},
		"published", "invoke", "2480", "search", "--params", `{"value":"`+strings.Repeat("x", maxMCPPublishedParamsBytes)+`"}`,
	)
	if err == nil || !strings.Contains(err.Error(), "--params 不能超过") {
		t.Fatalf("error = %v, want params size validation", err)
	}
}

func TestCrossPlatformCoverageMCPPublishedInvokeRejectsNonObjectParamsBeforeConfirmation(t *testing.T) {
	_, err := executeMCPPublishedCommand(
		t,
		publishedURLCaller(),
		&mcpPublishedTestTransport{},
		"published", "invoke", "2480", "search", "--params", `["bad"]`,
	)
	if err == nil || !strings.Contains(err.Error(), "--params 必须是 JSON 对象") {
		t.Fatalf("error = %v, want params validation", err)
	}
}

func TestCrossPlatformCoverageMCPPublishedClientConfigInheritsRuntimeTokenAndTransport(t *testing.T) {
	base := transport.NewClient(nil)
	runner := &runtimeRunner{transport: base}
	flags := &GlobalFlags{Token: " explicit-token "}

	gotBase, token, _, err := resolveMCPPublishedClientConfig(t.Context(), runner, flags)
	if err != nil {
		t.Fatalf("resolve published client config: %v", err)
	}
	if gotBase != base {
		t.Fatal("published client did not inherit the runtime transport")
	}
	if token != "explicit-token" {
		t.Fatalf("published client token = %q, want explicit-token", token)
	}
}

func TestCrossPlatformCoverageMCPPublishedClientConfigClonesHTTPTimeoutForCustomValues(t *testing.T) {
	for _, timeoutSeconds := range []int{2, 90} {
		t.Run(fmt.Sprintf("%ds", timeoutSeconds), func(t *testing.T) {
			base := transport.NewClient(&http.Client{Timeout: 30 * time.Second})
			runner := &runtimeRunner{transport: base}
			got, _, _, err := resolveMCPPublishedClientConfig(t.Context(), runner, &GlobalFlags{
				Token: "explicit-token", Timeout: timeoutSeconds,
			})
			if err != nil {
				t.Fatalf("resolve published client config: %v", err)
			}
			if got == base || got.HTTPClient == base.HTTPClient {
				t.Fatal("custom timeout did not clone the transport and HTTP client")
			}
			if got.HTTPClient.Timeout != time.Duration(timeoutSeconds)*time.Second {
				t.Fatalf("HTTP timeout = %s, want %ds", got.HTTPClient.Timeout, timeoutSeconds)
			}
			if base.HTTPClient.Timeout != 30*time.Second {
				t.Fatalf("base HTTP timeout mutated to %s", base.HTTPClient.Timeout)
			}
		})
	}
}

func TestCrossPlatformCoverageMCPPublishedEndpointCallerUsesOperationDeadline(t *testing.T) {
	base := transport.NewClient(&http.Client{Timeout: 30 * time.Second})
	runner := &runtimeRunner{transport: base}
	caller := newRecordingToolCaller(newToolCallerAdapter(runner, &GlobalFlags{}))
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	recording, ok := mcpPublishedCallerWithDeadline(caller, ctx).(recordingToolCaller)
	adapter, adapterOK := recording.inner.(*toolCallerAdapter)
	if !ok || !adapterOK {
		t.Fatalf("deadline caller wrappers were not preserved: %#v", recording)
	}
	got, runnerOK := adapter.runner.(*runtimeRunner)
	if !runnerOK || got == runner || got.transport == base || got.transport.HTTPClient == base.HTTPClient {
		t.Fatalf("deadline caller was not independently cloned: %#v", recording)
	}
	if got.transport.HTTPClient.Timeout <= 89*time.Second || base.HTTPClient.Timeout != 30*time.Second {
		t.Fatalf("endpoint timeouts cloned=%s base=%s", got.transport.HTTPClient.Timeout, base.HTTPClient.Timeout)
	}
}

func TestCrossPlatformCoverageMCPPublishedRejectsNonPositiveTimeout(t *testing.T) {
	for _, timeout := range []string{"0", "-1"} {
		_, err := executeMCPPublishedCommand(t, publishedURLCaller(), &mcpPublishedTestTransport{}, "--timeout", timeout, "published", "tools", "2480")
		if err == nil || !strings.Contains(err.Error(), "--timeout 必须是正整数秒") {
			t.Fatalf("timeout %s error = %v", timeout, err)
		}
	}
}

func TestCrossPlatformCoverageMCPPublishedGroupHelpAndDefaultFactory(t *testing.T) {
	group := newMCPPublishedGroup(nil, nil)
	root := &cobra.Command{Use: "mcp", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(group)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"published"})
	if err := corecmd.ExecuteContextForTest(root, t.Context()); err != nil {
		t.Fatalf("execute group help: %v", err)
	}
	if !strings.Contains(out.String(), "tools") || !strings.Contains(out.String(), "invoke") {
		t.Fatalf("group help missing commands:\n%s", out.String())
	}
}

func TestCrossPlatformCoverageMCPPublishedPositionalsAreValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "tools missing mcp id", args: []string{"published", "tools"}},
		{name: "tools extra argument", args: []string{"published", "tools", "2480", "extra"}},
		{name: "invoke missing tool", args: []string{"published", "invoke", "2480"}},
		{name: "invoke extra argument", args: []string{"published", "invoke", "2480", "search", "extra"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := executeMCPPublishedCommand(t, nil, &mcpPublishedTestTransport{}, tt.args...)
			if err == nil {
				t.Fatal("expected positional validation error")
			}
			var typed *apperrors.Error
			if !errors.As(err, &typed) {
				t.Fatalf("error type = %T, want *errors.Error: %v", err, err)
			}
			if typed.Category != apperrors.CategoryValidation || typed.ExitCode() != apperrors.ExitCodeValidation || typed.Reason != "invalid_positionals" {
				t.Fatalf("classification = category %q, code %d, reason %q", typed.Category, typed.ExitCode(), typed.Reason)
			}
		})
	}
}

func TestCrossPlatformCoverageMCPPublishedToolsErrorPaths(t *testing.T) {
	tests := []struct {
		name    string
		caller  edition.ToolCaller
		factory mcpPublishedTransportFactory
		wantErr string
	}{
		{
			name:    "endpoint resolution",
			wantErr: "caller is not configured",
		},
		{
			name:   "factory",
			caller: publishedURLCaller(),
			factory: func(context.Context) (mcpPublishedTransport, error) {
				return nil, errors.New("factory failed")
			},
			wantErr: "factory failed",
		},
		{
			name:   "transport",
			caller: publishedURLCaller(),
			factory: func(context.Context) (mcpPublishedTransport, error) {
				return &mcpPublishedTestTransport{listErr: errors.New("list failed")}, nil
			},
			wantErr: "列出已发布 MCP 工具: list failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := &cobra.Command{Use: "mcp", SilenceErrors: true, SilenceUsage: true}
			root.PersistentFlags().String("format", "json", "")
			factory := tt.factory
			if factory == nil {
				factory = func(context.Context) (mcpPublishedTransport, error) {
					return &mcpPublishedTestTransport{}, nil
				}
			}
			root.AddCommand(newMCPPublishedGroup(tt.caller, factory))
			root.SetArgs([]string{"published", "tools", "2480"})
			err := corecmd.ExecuteContextForTest(root, t.Context())
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestCrossPlatformCoverageMCPPublishedInvokeErrorPaths(t *testing.T) {
	tests := []struct {
		name       string
		caller     edition.ToolCaller
		factory    mcpPublishedTransportFactory
		callResult transport.ToolCallResult
		listErr    error
		callErr    error
		wantErr    string
	}{
		{
			name:    "endpoint resolution",
			wantErr: "caller is not configured",
		},
		{
			name:   "factory",
			caller: publishedURLCaller(),
			factory: func(context.Context) (mcpPublishedTransport, error) {
				return nil, errors.New("factory failed")
			},
			wantErr: "factory failed",
		},
		{
			name:    "discovery",
			caller:  publishedURLCaller(),
			listErr: errors.New("list failed"),
			wantErr: "发现已发布 MCP 工具: list failed",
		},
		{
			name:    "transport",
			caller:  publishedURLCaller(),
			callErr: errors.New("call failed"),
			wantErr: "调用已发布 MCP 工具: call failed",
		},
		{
			name:   "tool result",
			caller: publishedURLCaller(),
			callResult: transport.ToolCallResult{
				IsError: true,
				Blocks:  []transport.ContentBlock{{Type: "text", Text: "remote rejected"}},
			},
			wantErr: "remote rejected",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory := tt.factory
			if factory == nil {
				factory = func(context.Context) (mcpPublishedTransport, error) {
					return &mcpPublishedTestTransport{
						listResult: transport.ToolsListResult{Tools: []transport.ToolDescriptor{{
							Name: "search", InputSchema: map[string]any{"type": "object"},
						}}},
						listErr:    tt.listErr,
						callResult: tt.callResult,
						callErr:    tt.callErr,
					}, nil
				}
			}
			root := &cobra.Command{Use: "mcp", SilenceErrors: true, SilenceUsage: true}
			root.PersistentFlags().Bool("dry-run", false, "")
			root.PersistentFlags().Bool("yes", false, "")
			root.PersistentFlags().String("format", "json", "")
			root.AddCommand(newMCPPublishedGroup(tt.caller, factory))
			root.SetArgs([]string{"--yes", "published", "invoke", "2480", "search"})
			err := corecmd.ExecuteContextForTest(root, t.Context())
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestCrossPlatformCoverageParseMCPPublishedInvokeValidationEdges(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("params", "{}", "")
	for _, tt := range []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "arity", args: []string{"only-one"}, wantErr: "需要提供 mcpId 和工具名"},
		{name: "blank mcp id", args: []string{" ", "search"}, wantErr: "mcpId 不能为空"},
		{name: "blank tool", args: []string{"2480", " "}, wantErr: "工具名不能为空"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := parseMCPPublishedInvoke(cmd, tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
		})
	}

	_, _, _, err := parseMCPPublishedInvoke(&cobra.Command{}, []string{"2480", "search"})
	if err == nil {
		t.Fatal("missing params flag should fail")
	}

	run := runMCPPublishedInvoke(nil, nil)
	if err := run(cmd, []string{"only-one"}); err == nil {
		t.Fatal("invoke body should propagate parse errors")
	}
}

func TestCrossPlatformCoverageResolvePublishedMCPEndpointEdges(t *testing.T) {
	if _, err := resolvePublishedMCPEndpoint(t.Context(), nil, "2480"); err == nil {
		t.Fatal("nil caller should fail")
	}
	if _, err := resolvePublishedMCPEndpoint(t.Context(), &mcpURLTestCaller{}, " "); err == nil {
		t.Fatal("blank mcpId should fail")
	}

	tests := []struct {
		name    string
		caller  *mcpURLTestCaller
		want    string
		wantErr string
	}{
		{name: "call error", caller: &mcpURLTestCaller{err: errors.New("denied")}, wantErr: "获取 MCP 服务地址: denied"},
		{name: "nil result", caller: &mcpURLTestCaller{}, wantErr: "返回空结果"},
		{
			name: "skip unusable blocks",
			caller: &mcpURLTestCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{
				{Type: "image", Text: "ignored"},
				{Type: "text", Text: " "},
			}}},
			wantErr: "返回空结果",
		},
		{
			name: "business error",
			caller: &mcpURLTestCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{
				Type: "text", Text: `{"success":false,"errorMsg":"denied"}`,
			}}}},
			wantErr: "denied",
		},
		{
			name: "invalid json",
			caller: &mcpURLTestCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{
				Type: "text", Text: `{`,
			}}}},
			wantErr: "无效 JSON",
		},
		{
			name: "duplicate top-level url",
			caller: &mcpURLTestCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{
				Type: "text", Text: `{"mcpURL":"https://first.example/mcp","mcpURL":"https://second.example/mcp"}`,
			}}}},
			wantErr: "无效 JSON",
		},
		{
			name: "duplicate nested url",
			caller: &mcpURLTestCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{
				Type: "text", Text: `{"result":{"mcpURL":"https://first.example/mcp","mcpURL":"https://second.example/mcp"}}`,
			}}}},
			wantErr: "无效 JSON",
		},
		{
			name: "missing url",
			caller: &mcpURLTestCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{
				Type: "text", Text: `{"result":{"name":"missing"}}`,
			}}}},
			wantErr: "缺少 mcpURL",
		},
		{
			name: "flat url",
			caller: &mcpURLTestCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{
				Type: "text", Text: `{"mcpURL":" https://flat.example/mcp "}`,
			}}}},
			want: "https://flat.example/mcp",
		},
		{
			name: "nested blank falls back to flat",
			caller: &mcpURLTestCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{
				Type: "text", Text: `{"mcpURL":"https://flat.example/mcp","result":{"mcpURL":" "}}`,
			}}}},
			want: "https://flat.example/mcp",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolvePublishedMCPEndpoint(t.Context(), tt.caller, "2480")
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("endpoint = %q, error = %v, want %q", got, err, tt.want)
			}
		})
	}
}

func TestCrossPlatformCoverageMCPPublishedAuthenticatedFactory(t *testing.T) {
	factory := newAuthenticatedMCPPublishedTransportFactory(
		&runtimeRunner{transport: transport.NewClient(nil)},
		&GlobalFlags{Token: "factory-token"},
	)
	client, err := factory(t.Context())
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if client == nil {
		t.Fatal("factory returned nil client")
	}

	if got := firstNonEmpty(" ", "\t"); got != "" {
		t.Fatalf("firstNonEmpty blanks = %q", got)
	}
}

func TestCrossPlatformCoverageMCPPublishedAuthenticatedFactoryPropagatesTokenError(t *testing.T) {
	testseam.Swap(t, &runtimeTokenManager, NewTokenManager())
	testseam.Swap(t, &newAccessTokenProvider, func(string) accessTokenGetter {
		return fakeAccessTokenGetter{err: errors.New("oauth load failed")}
	})
	testseam.Swap(t, &newLegacyTokenManager, func(string) legacyTokenGetter {
		return fakeLegacyTokenGetter{err: errors.New("legacy load failed")}
	})
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())

	factory := newAuthenticatedMCPPublishedTransportFactory(nil, nil)
	if _, err := factory(t.Context()); err == nil {
		t.Fatal("factory should propagate token resolution errors")
	}
}

func TestCrossPlatformCoverageMCPPublishedShadowValidationFailures(t *testing.T) {
	wantErr := errors.New("shadow validation failed")

	t.Run("dry run stays local", func(t *testing.T) {
		testseam.Swap(t, &validateMCPPublishedResult, func(output.CommandResult) error { return wantErr })
		client := &mcpPublishedTestTransport{}
		_, err := executeMCPPublishedCommand(t, nil, client, "--dry-run", "published", "invoke", "2480", "search")
		if !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want injected validation failure", err)
		}
		if client.factoryCtx != nil || client.callContext != nil || client.listContext != nil {
			t.Fatal("dry-run validation failure performed network setup")
		}
	})

	t.Run("confirmed result is validated after one call", func(t *testing.T) {
		testseam.Swap(t, &validateMCPPublishedResult, func(output.CommandResult) error { return wantErr })
		client := &mcpPublishedTestTransport{}
		_, err := executeMCPPublishedCommand(t, publishedURLCaller(), client, "--yes", "published", "invoke", "2480", "search")
		if !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want injected validation failure", err)
		}
		if client.callTool != "search" {
			t.Fatalf("validated invocation tool = %q, want one search call", client.callTool)
		}
	})
}

func TestCrossPlatformCoverageMCPPublishedTimeoutAndCallerEdges(t *testing.T) {
	t.Run("invoke rejects timeout before resolution", func(t *testing.T) {
		client := &mcpPublishedTestTransport{}
		_, err := executeMCPPublishedCommand(t, nil, client, "--timeout", "0", "--yes", "published", "invoke", "2480", "search")
		if err == nil || !strings.Contains(err.Error(), "--timeout 必须是正整数秒") {
			t.Fatalf("error = %v, want timeout validation", err)
		}
		if client.factoryCtx != nil || client.callContext != nil || client.listContext != nil {
			t.Fatal("invalid timeout performed network setup")
		}
	})

	t.Run("wrong timeout flag type", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().String("timeout", "bad", "")
		_, cancel, err := mcpPublishedOperationContext(cmd)
		cancel()
		if err == nil {
			t.Fatal("string timeout flag should fail integer lookup")
		}
	})

	t.Run("expired deadline leaves caller unchanged", func(t *testing.T) {
		caller := publishedURLCaller()
		ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
		defer cancel()
		if got := mcpPublishedCallerWithDeadline(caller, ctx); got != caller {
			t.Fatal("expired deadline unexpectedly wrapped caller")
		}
	})

	t.Run("non-runtime adapter leaves caller unchanged", func(t *testing.T) {
		caller := &toolCallerAdapter{}
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		if got := mcpPublishedCallerWithDeadline(caller, ctx); got != caller {
			t.Fatal("adapter without runtime transport was replaced")
		}
	})
}

func TestCrossPlatformCoverageMCPPublishedClientConfigCreatesDefaultTransport(t *testing.T) {
	base, token, _, err := resolveMCPPublishedClientConfig(t.Context(), nil, &GlobalFlags{Token: " explicit-token "})
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}
	if base == nil || base.HTTPClient == nil {
		t.Fatal("nil runner did not receive a default transport")
	}
	if token != "explicit-token" {
		t.Fatalf("token = %q, want explicit-token", token)
	}
}
