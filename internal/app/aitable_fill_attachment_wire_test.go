// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"context"
	"encoding/json"

	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// This caller replaces only endpoint discovery; real JSON-RPC encoding reaches the local HTTP server.
type aitableWireCaller struct {
	client   *transport.Client
	endpoint string
}

func (c *aitableWireCaller) CallTool(ctx context.Context, _, tool string, args map[string]any) (*edition.ToolResult, error) {
	result, err := c.client.CallTool(ctx, c.endpoint, tool, args)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(result.Content)
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: string(raw)}}}, err
}

func (*aitableWireCaller) Format() string { return "json" }

func (*aitableWireCaller) DryRun() bool { return false }

func (*aitableWireCaller) Fields() string { return "" }

func (*aitableWireCaller) JQ() string { return "" }

func executeAitableWire(t *testing.T, args []string, body string) (map[string]any, error) {
	t.Helper()
	requests := make(chan map[string]any, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var wire map[string]any
		if err := json.NewDecoder(r.Body).Decode(&wire); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests <- wire
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":3,"result":{"content":%s}}`, body)
	}))
	defer server.Close()
	argv := append([]string{"aitable"}, args...)
	argv = append(argv, "--yes", "--format=json")
	testseam.Swap(t, &os.Args, append([]string{"dws"}, argv...))
	root := NewRootCommand()
	helpers.InitDepsForTest(t, &aitableWireCaller{client: transport.NewClient(server.Client()), endpoint: server.URL})
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(argv)
	err := root.Execute()
	if len(requests) > 1 {
		t.Fatalf("write was replayed: %d requests", len(requests))
	}
	select {
	case request := <-requests:
		return request, err
	default:
		return nil, err
	}
}

// Known and other appearance strings retain the historical transport contract.
func TestCrossPlatformCoverageAitableAppAppearanceCompatibility(t *testing.T) {
	for _, appearance := range []string{"light", "dark", "auto", "legacy-mode", "LIGHT", "invalid"} {
		t.Run(appearance, func(t *testing.T) {
			wire, err := executeAitableWire(t, []string{"app", "update", "--base-id=b", "--appearance=" + appearance}, `{"status":"success","data":{}}`)
			if err != nil || wire == nil {
				t.Fatalf("valid appearance rejected: error=%v wire=%v", err, wire)
			}
			params := wire["params"].(map[string]any)
			if params["name"] != "update_app" || params["arguments"].(map[string]any)["appearance"] != appearance {
				t.Fatalf("appearance changed in transit: %#v", params)
			}
		})
	}
}

func TestCrossPlatformCoverageAitableFillWire(t *testing.T) {
	for _, leaf := range []string{"view update fill-color-rule", "+view-set-fill-color-rule"} {
		for _, value := range []string{"true", "false", `"True"`, `"False"`} {
			t.Run(leaf+"/"+value, func(t *testing.T) {
				rule := fmt.Sprintf(`[{"type":"row","formatFieldId":"paint","format":{"color":"red_light"},"filters":[{"fieldId":"condition","symbol":"EQ","value":%s}]}]`, value)
				args := append(strings.Fields(leaf), "--base-id=b", "--table-id=t", "--view-id=v", "--json="+rule)
				wire, err := executeAitableWire(t, args, `{"status":"success","data":{}}`)
				if err != nil {
					t.Fatal(err)
				}
				var want any
				if err := json.Unmarshal([]byte(rule), &want); err != nil {
					t.Fatal(err)
				}
				params := wire["params"].(map[string]any)
				if params["name"] != "set_view_fill_color_rule" || !reflect.DeepEqual(params["arguments"].(map[string]any)["conditionalFormats"], want) {
					t.Fatalf("wire lost JSON types: %#v", params)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageAitableFillValidationAndUpstreamFailure(t *testing.T) {
	for _, leaf := range []string{"view update fill-color-rule", "+view-set-fill-color-rule"} {
		for _, raw := range []string{"null", "{}", "[true]", "invalid", "[" + strings.TrimSuffix(strings.Repeat("{},", 41), ",") + "]"} {
			wire, err := executeAitableWire(t, append(strings.Fields(leaf), "--base-id=b", "--table-id=t", "--view-id=v", "--json="+raw), `{}`)
			if apperrors.ExitCode(err) != 3 || wire != nil {
				t.Fatalf("%s/%s: error=%v code=%d wire=%v", leaf, raw, err, apperrors.ExitCode(err), wire)
			}
		}
	}
	for _, tc := range []struct {
		args []string
		body string
		code int
	}{
		// Existing upstream MCP tool failures retain exit code 1; local argument failures above use 3.
		{[]string{"view", "update", "fill-color-rule", "--base-id=b", "--table-id=t", "--view-id=v", `--json=[{"format":{"color":"secondLine10"}}]`}, `{"status":"error","error":{"type":"INPUT_ERROR","code":"INVALID_CONDITIONAL_FORMAT","message":"Invalid format.color","retryable":false}}`, 1},
	} {
		wire, err := executeAitableWire(t, tc.args, tc.body)
		if apperrors.ExitCode(err) != tc.code || wire == nil {
			t.Fatalf("upstream failure masked: error=%v code=%d wire=%v", err, apperrors.ExitCode(err), wire)
		}

	}
}
