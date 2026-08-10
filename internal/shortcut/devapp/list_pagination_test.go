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

package devapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type listPaginationCall struct {
	product string
	tool    string
	params  map[string]any
}

type listPaginationCaller struct {
	response string
	calls    []listPaginationCall
}

func (c *listPaginationCaller) CallTool(
	_ context.Context,
	product string,
	tool string,
	params map[string]any,
) (*edition.ToolResult, error) {
	cloned := make(map[string]any, len(params))
	for key, value := range params {
		cloned[key] = value
	}
	c.calls = append(c.calls, listPaginationCall{product: product, tool: tool, params: cloned})
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: c.response}}}, nil
}

func (*listPaginationCaller) Format() string { return "json" }
func (*listPaginationCaller) DryRun() bool   { return false }
func (*listPaginationCaller) Fields() string { return "" }
func (*listPaginationCaller) JQ() string     { return "" }

func executeListPagination(
	t *testing.T,
	response string,
	args ...string,
) (string, error, *listPaginationCaller) {
	t.Helper()

	caller := &listPaginationCaller{response: response}
	helpers.InitDepsForTest(t, caller)

	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().StringP("format", "f", "json", "")
	root.PersistentFlags().String("fields", "", "")
	root.PersistentFlags().String("jq", "", "")
	service := &cobra.Command{Use: "devapp"}
	service.AddCommand(corecmd.New(shortcut.FromShortcut(ListApp)))
	root.AddCommand(service)

	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs(append([]string{"devapp", "+list"}, args...))
	err := root.Execute()
	return output.String(), err, caller
}

func executeDevAppListShortcut(
	t *testing.T,
	command string,
	definition shortcut.Shortcut,
	response string,
	args ...string,
) (string, error, *listPaginationCaller) {
	t.Helper()

	caller := &listPaginationCaller{response: response}
	helpers.InitDepsForTest(t, caller)

	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().StringP("format", "f", "json", "")
	root.PersistentFlags().String("fields", "", "")
	root.PersistentFlags().String("jq", "", "")
	service := &cobra.Command{Use: "devapp"}
	service.AddCommand(corecmd.New(shortcut.FromShortcut(definition)))
	root.AddCommand(service)

	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs(append([]string{"devapp", command}, args...))
	err := root.Execute()
	return output.String(), err, caller
}

func decodeListPaginationEnvelope(t *testing.T, output string) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("decode output: %v\noutput=%q", err, output)
	}
	return payload
}

func decodePrettyListPagination(t *testing.T, output string) map[string]string {
	t.Helper()
	values := map[string]string{}
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("pretty output lines = %d, want four fields: %q", len(lines), output)
	}
	for _, line := range lines {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			t.Fatalf("pretty output line is not key/value: %q", line)
		}
		values[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return values
}

func TestCrossPlatformCoverageListPaginationNilRuntimeCursor(t *testing.T) {
	if got := listAppRequestCursor(nil); got != "" {
		t.Fatalf("nil runtime cursor = %q, want empty", got)
	}
}

func TestCrossPlatformCoverageAllDevAppPaginatedListsPreservePagination(t *testing.T) {
	const requestCursor = "opaque-current"
	const nextCursor = "opaque-next"
	tests := []struct {
		name       string
		command    string
		definition shortcut.Shortcut
		tool       string
		listKey    string
		response   string
		args       []string
	}{
		{
			name:       "permissions",
			command:    "+permission-list",
			definition: PermissionList,
			tool:       "list_dev_app_permissions",
			listKey:    "permissions",
			response:   `{"result":{"items":[{"scopeValue":"Contact.User.Read"}],"hasMore":true,"nextCursor":"opaque-next"}}`,
			args:       []string{"--unified-app-id", "app-1"},
		},
		{
			name:       "events",
			command:    "+event-list",
			definition: EventList,
			tool:       "list_dev_app_events",
			listKey:    "events",
			response:   `{"result":{"events":[{"eventCode":"chat_update"}],"hasMore":true,"nextCursor":"opaque-next"}}`,
			args:       []string{"--unified-app-id", "app-1"},
		},
		{
			name:       "versions",
			command:    "+version-list",
			definition: VersionList,
			tool:       "list_dev_app_versions",
			listKey:    "versions",
			response:   `{"result":{"items":[{"versionId":"version-1"}],"hasMore":true,"nextCursor":"opaque-next"}}`,
			args:       []string{"--unified-app-id", "app-1"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string{}, test.args...)
			args = append(args, "--cursor", requestCursor, "--page-size", "1", "--format", "json")
			output, err, caller := executeDevAppListShortcut(
				t, test.command, test.definition, test.response, args...,
			)
			if err != nil {
				t.Fatalf("execute %s: %v", test.command, err)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("calls = %#v", caller.calls)
			}
			call := caller.calls[0]
			if call.product != productDevApp || call.tool != test.tool ||
				call.params["cursor"] != requestCursor || call.params["pageSize"] != 1 {
				t.Fatalf("call = %#v", call)
			}

			payload := decodeListPaginationEnvelope(t, output)
			items, ok := payload[test.listKey].([]any)
			if len(payload) != 4 || !ok || len(items) != 1 || payload["count"] != float64(1) ||
				payload["hasMore"] != true || payload["nextCursor"] != nextCursor {
				t.Fatalf("pagination payload = %#v", payload)
			}
		})
	}
}

func TestCrossPlatformCoverageListPaginationTerminalFormats(t *testing.T) {
	response := `{
		"apps":[{
			"unified_app_id":"app-1",
			"appName":"Alpha",
			"clientId":"client-1",
			"agent_id":42,
			"appStatus":"ENABLED",
			"modifyTime":"2026-08-09T00:00:00Z",
			"ignored":"drop-me"
		}],
		"hasMore":false,
		"nextCursor":""
	}`

	for _, format := range []string{"json", "raw", "ndjson", "pretty"} {
		t.Run(format, func(t *testing.T) {
			if format == "pretty" {
				t.Setenv("NO_COLOR", "1")
			}
			output, err, caller := executeListPagination(t, response, "--format", format)
			if err != nil {
				t.Fatalf("execute terminal page: %v", err)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("calls = %#v, want one page", caller.calls)
			}

			if format == "pretty" {
				values := decodePrettyListPagination(t, output)
				if len(values) != 4 || values["count"] != "1" || values["hasMore"] != "false" ||
					values["nextCursor"] != "" || !strings.Contains(values["apps"], `"name":"Alpha"`) {
					t.Fatalf("pretty pagination values = %#v, output=%q", values, output)
				}
				return
			}

			if format == "ndjson" && len(strings.Split(strings.TrimSuffix(output, "\n"), "\n")) != 1 {
				t.Fatalf("ndjson expanded the page envelope: %q", output)
			}
			payload := decodeListPaginationEnvelope(t, output)
			if len(payload) != 4 || payload["count"] != float64(1) || payload["hasMore"] != false || payload["nextCursor"] != "" {
				t.Fatalf("terminal pagination payload = %#v", payload)
			}
			apps, ok := payload["apps"].([]any)
			if !ok || len(apps) != 1 {
				t.Fatalf("apps = %#v", payload["apps"])
			}
			app := apps[0].(map[string]any)
			want := map[string]any{
				"unifiedAppId": "app-1",
				"name":         "Alpha",
				"appKey":       "client-1",
				"agentId":      float64(42),
				"status":       "ENABLED",
				"gmtModified":  "2026-08-09T00:00:00Z",
			}
			if !reflect.DeepEqual(app, want) {
				t.Fatalf("app projection = %#v, want %#v", app, want)
			}
		})
	}
}

func TestCrossPlatformCoverageListPaginationValidItemProjectionIsOneToOne(t *testing.T) {
	response := `{
		"apps":[
			{"app_name":"Single","unknown":"drop-me"},
			{"unifiedAppId":"primary-id","unified_app_id":"secondary-id"},
			{"agent_id":null}
		],
		"hasMore":false,
		"nextCursor":""
	}`
	output, err, caller := executeListPagination(t, response, "--format", "json")
	if err != nil {
		t.Fatalf("valid item projection rejected: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v", caller.calls)
	}
	payload := decodeListPaginationEnvelope(t, output)
	apps, ok := payload["apps"].([]any)
	if len(payload) != 4 || !ok || len(apps) != 3 || payload["count"] != float64(3) {
		t.Fatalf("one-to-one payload = %#v", payload)
	}
	want := []any{
		map[string]any{"name": "Single"},
		map[string]any{"unifiedAppId": "primary-id"},
		map[string]any{"agentId": nil},
	}
	if !reflect.DeepEqual(apps, want) {
		t.Fatalf("projection = %#v, want %#v", apps, want)
	}
}

func TestCrossPlatformCoverageListPaginationSupportedOneLevelEnvelopes(t *testing.T) {
	for _, outerKey := range listAppCollectionKeys {
		for _, innerKey := range listAppCollectionKeys {
			name := outerKey + "/" + innerKey
			t.Run(name, func(t *testing.T) {
				response, err := json.Marshal(map[string]any{
					outerKey: map[string]any{
						innerKey:     []any{map[string]any{"unifiedAppId": "app-envelope"}},
						"hasMore":    false,
						"nextCursor": "",
					},
				})
				if err != nil {
					t.Fatal(err)
				}
				output, executeErr, caller := executeListPagination(t, string(response), "--format", "json")
				if executeErr != nil {
					t.Fatalf("supported envelope rejected: %v, response=%s", executeErr, response)
				}
				if len(caller.calls) != 1 {
					t.Fatalf("calls = %#v", caller.calls)
				}
				payload := decodeListPaginationEnvelope(t, output)
				if len(payload) != 4 || payload["count"] != float64(1) ||
					payload["hasMore"] != false || payload["nextCursor"] != "" {
					t.Fatalf("envelope payload = %#v", payload)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageListPaginationRejectsHiddenPageInSupportedEnvelopes(t *testing.T) {
	for _, outerKey := range listAppCollectionKeys {
		t.Run(outerKey, func(t *testing.T) {
			response, err := json.Marshal(map[string]any{
				outerKey: map[string]any{
					"apps":       []any{},
					"hasMore":    false,
					"nextCursor": "",
					"payload": map[string]any{
						"apps":       []any{},
						"hasMore":    false,
						"nextCursor": "",
					},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			output, executeErr, caller := executeListPagination(t, string(response), "--format", "json")
			if executeErr == nil {
				t.Fatalf("hidden page unexpectedly accepted: %s", response)
			}
			if output != "" || len(caller.calls) != 1 {
				t.Fatalf("hidden page output=%q calls=%#v", output, caller.calls)
			}
			var typed *apperrors.Error
			if !errors.As(executeErr, &typed) || typed.Reason != "devapp_pagination_contract_invalid" {
				t.Fatalf("hidden page error = %T %#v", executeErr, executeErr)
			}
		})
	}
}

func TestCrossPlatformCoverageListPaginationAllowsNonPageMetadataSiblings(t *testing.T) {
	nonPageMetadata := map[string]any{
		"data":   "trace-data",
		"result": "trace-result",
		"list":   "trace-list",
		"items":  "trace-items",
	}
	for _, test := range []struct {
		name     string
		response map[string]any
	}{
		{
			name: "top-level page",
			response: map[string]any{
				"apps":       []any{},
				"hasMore":    false,
				"nextCursor": "",
				"metadata":   nonPageMetadata,
			},
		},
		{
			name: "supported wrapper",
			response: map[string]any{
				"result": map[string]any{
					"apps":       []any{},
					"hasMore":    false,
					"nextCursor": "",
					"metadata":   nonPageMetadata,
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			response, err := json.Marshal(test.response)
			if err != nil {
				t.Fatal(err)
			}
			output, executeErr, caller := executeListPagination(t, string(response), "--format", "json")
			if executeErr != nil {
				t.Fatalf("non-page metadata rejected: %v, response=%s", executeErr, response)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("calls = %#v", caller.calls)
			}
			payload := decodeListPaginationEnvelope(t, output)
			if len(payload) != 4 || payload["count"] != float64(0) ||
				payload["hasMore"] != false || payload["nextCursor"] != "" {
				t.Fatalf("non-page metadata payload = %#v", payload)
			}
		})
	}
}

func TestCrossPlatformCoverageListPaginationOneLevelAndOpaqueCursor(t *testing.T) {
	requestCursor := " opaque/request/+==:? "
	nextCursor := "opaque/next/+==:?"
	response := fmt.Sprintf(`{
		"result":{
			"items":[{"unifiedAppId":"app-2","name":"Beta"}],
			"hasMore":true,
			"nextCursor":%q
		}
	}`, nextCursor)

	output, err, caller := executeListPagination(t, response,
		"--cursor", requestCursor,
		"--page-size", "7",
		"--name", "Beta",
		"--app-key", "client-2",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("execute non-terminal page: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v, want one page without auto-pagination", caller.calls)
	}
	call := caller.calls[0]
	if call.product != "devapp" || call.tool != "list_dev_app" {
		t.Fatalf("call target = %s/%s", call.product, call.tool)
	}
	wantParams := map[string]any{
		"cursor":   requestCursor,
		"pageSize": 7,
		"name":     "Beta",
		"appKey":   "client-2",
	}
	if !reflect.DeepEqual(call.params, wantParams) {
		t.Fatalf("params = %#v, want %#v", call.params, wantParams)
	}
	payload := decodeListPaginationEnvelope(t, output)
	if len(payload) != 4 || payload["hasMore"] != true || payload["nextCursor"] != nextCursor || payload["count"] != float64(1) {
		t.Fatalf("non-terminal payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageListPaginationNormalizesDeployedTerminalEnvelope(t *testing.T) {
	response := `{
		"arguments":[],
		"errorCode":null,
		"errorMsg":null,
		"result":{
			"items":[{"unifiedAppId":"app-live","name":"Live App"}],
			"hasMore":false,
			"nextCursor":"4010069592"
		},
		"success":true
	}`

	output, err, caller := executeListPagination(t, response, "--format", "json")
	if err != nil {
		t.Fatalf("deployed terminal envelope rejected: %v", err)
	}
	if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0].params, map[string]any{"pageSize": 20}) {
		t.Fatalf("calls = %#v", caller.calls)
	}
	payload := decodeListPaginationEnvelope(t, output)
	apps, ok := payload["apps"].([]any)
	if len(payload) != 4 || !ok || len(apps) != 1 || payload["count"] != float64(1) ||
		payload["hasMore"] != false || payload["nextCursor"] != "" {
		t.Fatalf("normalized deployed terminal payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageListPaginationValidEmptyPages(t *testing.T) {
	for _, test := range []struct {
		name        string
		response    string
		args        []string
		wantHasMore bool
		wantNext    string
		wantRequest map[string]any
	}{
		{
			name:        "terminal",
			response:    `{"apps":[],"hasMore":false,"nextCursor":""}`,
			wantHasMore: false,
			wantNext:    "",
			wantRequest: map[string]any{"pageSize": 20},
		},
		{
			name:        "terminal omitted provider cursor",
			response:    `{"apps":[],"hasMore":false}`,
			wantHasMore: false,
			wantNext:    "",
			wantRequest: map[string]any{"pageSize": 20},
		},
		{
			name:        "terminal null provider cursor",
			response:    `{"apps":[],"hasMore":false,"nextCursor":null}`,
			wantHasMore: false,
			wantNext:    "",
			wantRequest: map[string]any{"pageSize": 20},
		},
		{
			name:        "terminal last provider cursor",
			response:    `{"apps":[],"hasMore":false,"nextCursor":"4010069592"}`,
			wantHasMore: false,
			wantNext:    "",
			wantRequest: map[string]any{"pageSize": 20},
		},
		{
			name:        "non-terminal",
			response:    `{"apps":[],"hasMore":true,"nextCursor":"after-empty"}`,
			args:        []string{"--cursor", "before-empty"},
			wantHasMore: true,
			wantNext:    "after-empty",
			wantRequest: map[string]any{"cursor": "before-empty", "pageSize": 20},
		},
		{
			name:        "explicit empty request cursor",
			response:    `{"apps":[],"hasMore":false,"nextCursor":""}`,
			args:        []string{"--cursor", ""},
			wantHasMore: false,
			wantNext:    "",
			wantRequest: map[string]any{"pageSize": 20},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := append(append([]string(nil), test.args...), "--format", "json")
			output, err, caller := executeListPagination(t, test.response, args...)
			if err != nil {
				t.Fatalf("valid empty page rejected: %v", err)
			}
			if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0].params, test.wantRequest) {
				t.Fatalf("calls = %#v, want params %#v", caller.calls, test.wantRequest)
			}
			payload := decodeListPaginationEnvelope(t, output)
			apps, ok := payload["apps"].([]any)
			if len(payload) != 4 || !ok || len(apps) != 0 || payload["count"] != float64(0) ||
				payload["hasMore"] != test.wantHasMore || payload["nextCursor"] != test.wantNext {
				t.Fatalf("empty page payload = %#v", payload)
			}
		})
	}
}

func TestCrossPlatformCoverageListPaginationRejectsInvalidContracts(t *testing.T) {
	tests := []struct {
		name     string
		response string
		args     []string
	}{
		{name: "missing hasMore", response: `{"apps":[],"nextCursor":""}`},
		{name: "wrong hasMore type", response: `{"apps":[],"hasMore":"false","nextCursor":""}`},
		{name: "non-terminal missing continuation", response: `{"apps":[],"hasMore":true}`},
		{name: "empty continuation", response: `{"apps":[],"hasMore":true,"nextCursor":""}`},
		{name: "wrong continuation type", response: `{"apps":[],"hasMore":true,"nextCursor":7}`},
		{name: "terminal continuation wrong type", response: `{"apps":[],"hasMore":false,"nextCursor":7}`},
		{name: "stalled cursor", response: `{"apps":[],"hasMore":true,"nextCursor":"same"}`, args: []string{"--cursor", "same"}},
		{name: "null page", response: `null`},
		{name: "apps missing", response: `{"hasMore":false,"nextCursor":""}`},
		{name: "apps wrong type", response: `{"apps":"not-a-list","hasMore":false,"nextCursor":""}`},
		{name: "null app item", response: `{"apps":[null],"hasMore":false,"nextCursor":""}`},
		{name: "string app item", response: `{"apps":["bad"],"hasMore":false,"nextCursor":""}`},
		{name: "number app item", response: `{"apps":[7],"hasMore":false,"nextCursor":""}`},
		{name: "boolean app item", response: `{"apps":[true],"hasMore":false,"nextCursor":""}`},
		{name: "nested array app item", response: `{"apps":[[]],"hasMore":false,"nextCursor":""}`},
		{name: "empty app object", response: `{"apps":[{}],"hasMore":false,"nextCursor":""}`},
		{name: "unknown-only app object", response: `{"apps":[{"unknown":"value"}],"hasMore":false,"nextCursor":""}`},
		{name: "mixed valid and invalid app items", response: `{
			"apps":[{"unifiedAppId":"would-be-partial"},"bad"],
			"hasMore":false,"nextCursor":""
		}`},
		{name: "ambiguous list fields", response: `{"apps":[],"items":[],"hasMore":false,"nextCursor":""}`},
		{name: "ambiguous page envelopes", response: `{
			"apps":[],"hasMore":false,"nextCursor":"",
			"result":{"apps":[],"hasMore":false,"nextCursor":""}
		}`},
		{name: "metadata split across envelopes", response: `{
			"hasMore":false,"nextCursor":"",
			"result":{"apps":[]}
		}`},
		{name: "unknown payload wrapper conflicts with top page", response: `{
			"apps":[],"hasMore":false,"nextCursor":"",
			"payload":{"apps":[],"hasMore":false,"nextCursor":""}
		}`},
		{name: "unknown response wrapper conflicts with top page", response: `{
			"apps":[],"hasMore":false,"nextCursor":"",
			"response":{"items":[],"hasMore":false,"nextCursor":""}
		}`},
		{name: "unknown wrapper nextCursor evidence conflicts with top page", response: `{
			"apps":[],"hasMore":false,"nextCursor":"",
			"metadata":{"nextCursor":"hidden"}
		}`},
		{name: "unknown wrapper recursively nested list conflicts with top page", response: `{
			"apps":[],"hasMore":false,"nextCursor":"",
			"metadata":{"data":{"items":[]}}
		}`},
		{name: "unknown wrapper invalid apps evidence conflicts with top page", response: `{
			"apps":[],"hasMore":false,"nextCursor":"",
			"metadata":{"apps":"invalid"}
		}`},
		{name: "unsupported second-level envelope", response: `{
			"result":{"data":{"apps":[],"hasMore":false,"nextCursor":""}}
		}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := append(append([]string(nil), test.args...), "--format", "json")
			output, err, caller := executeListPagination(t, test.response, args...)
			if err == nil {
				t.Fatalf("invalid response unexpectedly succeeded: %s", test.response)
			}
			if output != "" {
				t.Fatalf("invalid response wrote success output: %q", output)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("calls = %#v, want one bounded read", caller.calls)
			}
			var typed *apperrors.Error
			if !errors.As(err, &typed) {
				t.Fatalf("error = %T %v, want structured API error", err, err)
			}
			if typed.Category != apperrors.CategoryAPI || typed.ExitCode() != 1 || typed.Reason != "devapp_pagination_contract_invalid" {
				t.Fatalf("error contract = %#v", typed)
			}
		})
	}
}
