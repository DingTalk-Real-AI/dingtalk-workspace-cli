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
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/spf13/cobra"
)

type mcpMarketSearchTestCaller struct {
	responses []string
	productID string
	toolName  string
	calls     []map[string]any
	err       error
	dryRun    bool
}

func (c *mcpMarketSearchTestCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.productID = productID
	c.toolName = toolName
	c.calls = append(c.calls, args)
	if c.err != nil {
		return nil, c.err
	}
	i := len(c.calls) - 1
	if i >= len(c.responses) {
		i = len(c.responses) - 1
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: c.responses[i]}}}, nil
}

func (*mcpMarketSearchTestCaller) Format() string { return "json" }
func (c *mcpMarketSearchTestCaller) DryRun() bool { return c.dryRun }
func (*mcpMarketSearchTestCaller) Fields() string { return "" }
func (*mcpMarketSearchTestCaller) JQ() string     { return "" }

// mcpMarketSearchPage builds one search_market_mcps response. totalCount is
// kept deliberately inconsistent with the items, as observed on the live
// endpoint.
func mcpMarketSearchPage(ids ...int) string {
	values := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		values = append(values, map[string]any{"mcpId": id, "name": fmt.Sprintf("MCP %d", id)})
	}
	raw, _ := json.Marshal(map[string]any{
		"success": true,
		"result":  map[string]any{"values": values, "totalCount": 1},
	})
	return string(raw)
}

func executeMCPMarketSearchCommand(t *testing.T, caller edition.ToolCaller, args ...string) (map[string]any, string, error) {
	t.Helper()
	ctx, _ := output.WithResultStore(t.Context())
	root := &cobra.Command{Use: "mcp", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().String("format", "json", "")
	root.PersistentPostRunE = func(executed *cobra.Command, _ []string) error {
		_, _, err := output.EmitStoredResult(executed)
		return err
	}
	root.AddCommand(newMCPMarketGroup(caller))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"market", "search"}, args...))
	err := corecmd.ExecuteContextForTest(root, ctx)
	var envelope map[string]any
	_ = json.Unmarshal(out.Bytes(), &envelope)
	return envelope, out.String(), err
}

func mcpMarketSearchPagination(t *testing.T, envelope map[string]any, raw string) map[string]any {
	t.Helper()
	meta, _ := envelope["meta"].(map[string]any)
	pagination, ok := meta["pagination"].(map[string]any)
	if !ok {
		t.Fatalf("meta.pagination missing:\n%s", raw)
	}
	return pagination
}

func mcpMarketSearchItems(t *testing.T, envelope map[string]any, raw string) []any {
	t.Helper()
	data, _ := envelope["data"].(map[string]any)
	items, ok := data["mcps"].([]any)
	if !ok {
		t.Fatalf("data.mcps missing:\n%s", raw)
	}
	return items
}

func TestCrossPlatformCoverageMCPMarketSearchCallsMetaServerWithPageArgs(t *testing.T) {
	caller := &mcpMarketSearchTestCaller{responses: []string{mcpMarketSearchPage(9595, 1024)}}

	envelope, raw, err := executeMCPMarketSearchCommand(t, caller, "--keyword", " 机器人 ", "--limit", "2")
	if err != nil {
		t.Fatalf("execute mcp market search: %v\n%s", err, raw)
	}
	if caller.productID != mcpMetaServerID || caller.toolName != mcpMetaMarketSearchTool {
		t.Fatalf("call routed to %s/%s, want %s/%s", caller.productID, caller.toolName, mcpMetaServerID, mcpMetaMarketSearchTool)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(caller.calls))
	}
	call := caller.calls[0]
	if call["search"] != "机器人" || call["page"] != 1 || call["pageSize"] != 2 {
		t.Fatalf("call args = %#v", call)
	}
	items := mcpMarketSearchItems(t, envelope, raw)
	if got := len(items); got != 2 {
		t.Fatalf("items = %d, want 2", got)
	}
	first, _ := items[0].(map[string]any)
	if value, present := first["serverName"]; !present || value != nil {
		t.Fatalf("serverName must be present as null when the endpoint omits it: %#v", first)
	}
	data, _ := envelope["data"].(map[string]any)
	if data["keyword"] != "机器人" {
		t.Fatalf("data.keyword = %#v", data["keyword"])
	}
	pagination := mcpMarketSearchPagination(t, envelope, raw)
	if pagination["next_token"] != "2" || pagination["endpoint_exhausted"] == true {
		t.Fatalf("pagination = %#v, want next_token 2 and not exhausted", pagination)
	}
	for _, key := range []string{"pages", "items"} {
		if _, present := pagination[key]; present {
			t.Fatalf("pagination.%s must not be published for a single-page command: %#v", key, pagination)
		}
	}
}

func TestCrossPlatformCoverageMCPMarketSearchProjectsReviewedFields(t *testing.T) {
	response := `{"success":true,"result":{"values":[{"mcpId":9595,"name":"机器人消息","description":"机器人消息","serverName":"bot","official":true,"corpName":"钉钉（中国）信息技术有限公司","charged":false,"installCount":283898,"highQuality":true,"sourceType":"DINGTALK","icon":"https://example.test/icon.png"}]}}`
	caller := &mcpMarketSearchTestCaller{responses: []string{response}}

	envelope, raw, err := executeMCPMarketSearchCommand(t, caller, "--keyword", "机器人")
	if err != nil {
		t.Fatalf("execute mcp market search: %v\n%s", err, raw)
	}
	items := mcpMarketSearchItems(t, envelope, raw)
	item, _ := items[0].(map[string]any)
	got := make([]string, 0, len(item))
	for key := range item {
		got = append(got, key)
	}
	sort.Strings(got)
	want := append([]string(nil), mcpMarketSearchFields...)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("item fields = %v, want %v", got, want)
	}
	if item["serverName"] != "bot" || item["official"] != true {
		t.Fatalf("projected values changed: %#v", item)
	}
}

func TestCrossPlatformCoverageMCPMarketSearchShortPageIsExhausted(t *testing.T) {
	caller := &mcpMarketSearchTestCaller{responses: []string{mcpMarketSearchPage(2013)}}

	envelope, raw, err := executeMCPMarketSearchCommand(t, caller, "--keyword", "Teambition", "--limit", "2")
	if err != nil {
		t.Fatalf("execute mcp market search: %v\n%s", err, raw)
	}
	pagination := mcpMarketSearchPagination(t, envelope, raw)
	if pagination["endpoint_exhausted"] != true {
		t.Fatalf("pagination = %#v, want exhausted", pagination)
	}
	if token, _ := pagination["next_token"].(string); token != "" {
		t.Fatalf("next_token = %q, want empty", token)
	}
}

func TestCrossPlatformCoverageMCPMarketSearchCursorSelectsPage(t *testing.T) {
	caller := &mcpMarketSearchTestCaller{responses: []string{mcpMarketSearchPage()}}

	_, raw, err := executeMCPMarketSearchCommand(t, caller, "--keyword", "机器人", "--cursor", "3")
	if err != nil {
		t.Fatalf("execute mcp market search: %v\n%s", err, raw)
	}
	if got := caller.calls[0]["page"]; got != 3 {
		t.Fatalf("page = %#v, want 3", got)
	}
	if got := caller.calls[0]["pageSize"]; got != mcpMarketSearchDefaultLimit {
		t.Fatalf("pageSize = %#v, want default %d", got, mcpMarketSearchDefaultLimit)
	}
}

func TestCrossPlatformCoverageMCPMarketSearchRejectsInvalidFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing keyword", args: []string{"--limit", "5"}, want: "keyword"},
		{name: "limit too small", args: []string{"--keyword", "x", "--limit", "0"}, want: "--limit"},
		{name: "limit too large", args: []string{"--keyword", "x", "--limit", "101"}, want: "--limit"},
		{name: "non-numeric cursor", args: []string{"--keyword", "x", "--cursor", "abc"}, want: "--cursor"},
		{name: "zero cursor", args: []string{"--keyword", "x", "--cursor", "0"}, want: "--cursor"},
		{name: "cursor cannot advance", args: []string{"--keyword", "x", "--cursor", strconv.Itoa(int(^uint(0) >> 1))}, want: "--cursor"},
		{name: "page-all is not supported", args: []string{"--keyword", "x", "--page-all"}, want: "page-all"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &mcpMarketSearchTestCaller{responses: []string{mcpMarketSearchPage()}}
			_, raw, err := executeMCPMarketSearchCommand(t, caller, tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want mention of %q\n%s", err, tt.want, raw)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("invalid flags must not call the endpoint, got %d calls", len(caller.calls))
			}
		})
	}
}

func TestCrossPlatformCoverageMCPMarketSearchClassifiesBusinessError(t *testing.T) {
	caller := &mcpMarketSearchTestCaller{responses: []string{`{"success":false,"errorMsg":"搜索失败"}`}}

	_, _, err := executeMCPMarketSearchCommand(t, caller, "--keyword", "x")
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "business_error" {
		t.Fatalf("error = %#v, want classified business error", err)
	}
}

func TestCrossPlatformCoverageMCPMarketSearchRejectsMalformedResults(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     string
	}{
		{name: "missing values", response: `{"success":true,"result":{}}`, want: "result.values"},
		{name: "item without mcpId", response: `{"success":true,"result":{"values":[{"name":"x"}]}}`, want: "mcpId"},
		{name: "item without name", response: `{"success":true,"result":{"values":[{"mcpId":1}]}}`, want: "name"},
		{name: "item with null name", response: `{"success":true,"result":{"values":[{"mcpId":1,"name":null}]}}`, want: "name"},
		{name: "item with non-string name", response: `{"success":true,"result":{"values":[{"mcpId":1,"name":42}]}}`, want: "name"},
		{name: "invalid json", response: "not-json", want: "无效 JSON"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &mcpMarketSearchTestCaller{responses: []string{tt.response}}
			_, _, err := executeMCPMarketSearchCommand(t, caller, "--keyword", "x")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestCrossPlatformCoverageMCPMarketSearchPropagatesCallError(t *testing.T) {
	caller := &mcpMarketSearchTestCaller{err: errors.New("permission denied")}

	_, _, err := executeMCPMarketSearchCommand(t, caller, "--keyword", "x")
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("error = %v, want call error", err)
	}
}

func TestCrossPlatformCoverageMCPMarketSearchDryRunDoesNotCallEndpoint(t *testing.T) {
	caller := &mcpMarketSearchTestCaller{dryRun: true}

	envelope, raw, err := executeMCPMarketSearchCommand(t, caller, "--keyword", "机器人", "--cursor", "2")
	if err != nil {
		t.Fatalf("execute mcp market search dry-run: %v\n%s", err, raw)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("dry-run called the endpoint %d times", len(caller.calls))
	}
	data, _ := envelope["data"].(map[string]any)
	if data["tool"] != mcpMetaMarketSearchTool {
		t.Fatalf("dry-run data = %#v", data)
	}
}

func TestCrossPlatformCoverageMCPMarketSearchRejectsMissingCaller(t *testing.T) {
	_, _, err := executeMCPMarketSearchCommand(t, nil, "--keyword", "x")
	if err == nil || !strings.Contains(err.Error(), "caller is not configured") {
		t.Fatalf("error = %v, want missing caller error", err)
	}
}

func TestCrossPlatformCoverageRootRegistersMCPMarketSearch(t *testing.T) {
	root := NewRootCommand(t.Context())
	cmd, _, err := root.Find([]string{"mcp", "market", "search"})
	if err != nil {
		t.Fatalf("find mcp market search: %v", err)
	}
	if got := cmd.CommandPath(); got != "dws mcp market search" {
		t.Fatalf("command path = %q, want %q", got, "dws mcp market search")
	}
}

func TestCrossPlatformCoverageMCPMarketGroupShowsHelp(t *testing.T) {
	root := &cobra.Command{Use: "mcp", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(newMCPMarketGroup(nil))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"market"})
	if err := corecmd.ExecuteContextForTest(root, t.Context()); err != nil {
		t.Fatalf("execute mcp market: %v", err)
	}
	if !strings.Contains(out.String(), "search") {
		t.Fatalf("help output does not list search command:\n%s", out.String())
	}
}

func TestCrossPlatformCoverageDecodeMCPMetaResult(t *testing.T) {
	text := func(body string) *edition.ToolResult {
		return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: body}}}
	}
	tests := []struct {
		name   string
		result *edition.ToolResult
		want   string
	}{
		{name: "nil result", result: nil, want: "返回空结果"},
		{name: "no usable text block", result: &edition.ToolResult{Content: []edition.ContentBlock{
			{Type: "image", Text: "ignored"},
			{Type: "text", Text: "   "},
		}}, want: "返回空结果"},
		{name: "invalid json", result: text("not-json"), want: "无效 JSON"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := decodeMCPMetaResult(tt.result); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}

	_, err := decodeMCPMetaResult(text(`{"success":false,"errorMsg":"搜索失败"}`))
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "business_error" {
		t.Fatalf("error = %#v, want classified business error", err)
	}

	payload, err := decodeMCPMetaResult(text(`{"result":{"name":"Example"}}`))
	if err != nil {
		t.Fatalf("decode valid payload: %v", err)
	}
	body, _ := payload.(map[string]any)
	if result, _ := body["result"].(map[string]any); result["name"] != "Example" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageMCPURLGetWritesDecodedPayload(t *testing.T) {
	const response = `{"result":{"mcpURL":"https://example.test/mcp?key=one","mcpJSON":"{}","name":"Example"}}`
	caller := &mcpURLTestCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: response}}}}

	out, err := executeMCPURLCommand(t, caller, "url", "get", "10043")
	if err != nil {
		t.Fatalf("execute mcp url get: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if result, _ := payload["result"].(map[string]any); result["name"] != "Example" {
		t.Fatalf("output payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageMCPURLGetReturnsDecodeError(t *testing.T) {
	caller := &mcpURLTestCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: "not-json"}}}}

	_, err := executeMCPURLCommand(t, caller, "url", "get", "10043")
	if err == nil || !strings.Contains(err.Error(), "无效 JSON") {
		t.Fatalf("error = %v, want invalid JSON error", err)
	}
}

func TestCrossPlatformCoverageMCPMarketSearchRejectsWrongFieldTypes(t *testing.T) {
	for field, invalid := range map[string][]any{
		"mcpId":       {"1050", nil, true, 1.5},
		"name":        {true, 42, nil},
		"description": {nil, true, 42},
		"serverName":  {true, 42},
		"official":    {nil, "true", 1},
		"corpName":    {nil, false, 42},
		"charged":     {nil, "false", 0},
	} {
		for _, value := range invalid {
			t.Run(fmt.Sprintf("%s_%v", field, value), func(t *testing.T) {
				item := map[string]any{"mcpId": 1050, "name": "日历"}
				item[field] = value
				body, err := json.Marshal(map[string]any{"result": map[string]any{"values": []any{item}}})
				if err != nil {
					t.Fatal(err)
				}
				caller := &mcpMarketSearchTestCaller{responses: []string{string(body)}}
				envelope, raw, err := executeMCPMarketSearchCommand(t, caller, "--keyword", "日历")
				if err == nil || !strings.Contains(err.Error(), field) {
					t.Fatalf("invalid %s accepted: err=%v output=%s", field, err, raw)
				}
				if envelope["ok"] == true {
					t.Fatalf("malformed response reported success: %s", raw)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageMCPMarketSearchFinalSchemaAcceptsActualResults(t *testing.T) {
	callers := []*mcpMarketSearchTestCaller{
		{dryRun: true},
		{responses: []string{`{"result":{"values":[{"mcpId":1050,"name":"日历","description":"日历管理","serverName":null,"official":false,"corpName":"钉钉","charged":true}]}}`}},
		{responses: []string{mcpMarketSearchPage()}},
	}
	for _, compact := range []bool{false, true} {
		args := []string{"--cli-path", "mcp market search"}
		if compact {
			args = append(args, "--compact")
		}
		leaf := executeShortcutSchemaQuery(t, args...)
		result := leaf["result"].(map[string]any)
		compiler := jsonschema.NewCompiler()
		compiler.DefaultDraft(jsonschema.Draft2020)
		const resource = "https://dws.test/mcp-market-search.json"
		if err := compiler.AddResource(resource, result["data_schema"]); err != nil {
			t.Fatal(err)
		}
		schema, err := compiler.Compile(resource)
		if err != nil {
			t.Fatal(err)
		}
		for _, caller := range callers {
			envelope, raw, err := executeMCPMarketSearchCommand(t, caller, "--keyword", "日历")
			if err != nil {
				t.Fatalf("command failed: %v\n%s", err, raw)
			}
			if err := schema.Validate(envelope["data"]); err != nil {
				t.Fatalf("compact=%t dry-run=%t actual data violates schema: %v", compact, caller.dryRun, err)
			}
			if caller.dryRun && (len(caller.calls) != 0 || envelope["dry_run"] != true) {
				t.Fatalf("dry-run executed remote work or lost marker: %s", raw)
			}
		}
		for _, invalid := range []any{
			map[string]any{"keyword": "日历"},
			map[string]any{"server": "mcp-meta", "tool": "wrong", "args": map[string]any{"search": "日历", "page": 1, "pageSize": 20}},
			map[string]any{"keyword": "日历", "mcps": []any{map[string]any{"mcpId": "1050", "name": "日历"}}},
		} {
			if err := schema.Validate(invalid); err == nil {
				t.Fatalf("invalid data accepted: %#v", invalid)
			}
		}
	}
}
