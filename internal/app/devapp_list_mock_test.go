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
	"io"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func executeDevAppListRoot(t *testing.T, args ...string) (string, string, error) {
	return executeDevAppRoot(t, "+list", args...)
}

func executeDevAppRoot(t *testing.T, command string, args ...string) (string, string, error) {
	t.Helper()
	helpers.InitDepsForTest(t, helpers.GetCaller())
	root := NewRootCommand()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(append([]string{"devapp", command}, args...))
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

func TestCrossPlatformCoverageDevAppPaginatedShortcutsMock(t *testing.T) {
	for _, test := range []struct {
		name       string
		command    string
		collection string
		args       []string
	}{
		{name: "apps", command: "+list", collection: "apps"},
		{name: "permissions", command: "+permission-list", collection: "permissions", args: []string{"--unified-app-id", "X"}},
		{name: "events", command: "+event-list", collection: "events", args: []string{"--unified-app-id", "X"}},
		{name: "versions", command: "+version-list", collection: "versions", args: []string{"--unified-app-id", "X"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := append(append([]string(nil), test.args...), "--mock", "--format", "json")
			stdout, stderr, err := executeDevAppRoot(t, test.command, args...)
			if err != nil {
				t.Fatalf("mock %s error = %v, stderr=%q", test.command, err, stderr)
			}
			if stderr != "" {
				t.Fatalf("mock %s stderr = %q", test.command, stderr)
			}

			var payload map[string]any
			if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
				t.Fatalf("decode mock %s output: %v, output=%q", test.command, err, stdout)
			}
			items, ok := payload[test.collection].([]any)
			if len(payload) != 4 || !ok || len(items) != 0 || payload["count"] != float64(0) ||
				payload["hasMore"] != false || payload["nextCursor"] != "" {
				t.Fatalf("mock %s payload = %#v", test.command, payload)
			}
		})
	}
}

func TestCrossPlatformCoverageDevAppListMockPaginationFormats(t *testing.T) {
	for _, format := range []string{"json", "raw", "ndjson", "pretty"} {
		t.Run(format, func(t *testing.T) {
			if format == "pretty" {
				t.Setenv("NO_COLOR", "1")
			}
			stdout, stderr, err := executeDevAppListRoot(t, "--mock", "--format", format)
			if err != nil {
				t.Fatalf("mock list error = %v, stderr=%q", err, stderr)
			}
			if stderr != "" {
				t.Fatalf("mock list stderr = %q", stderr)
			}
			if format == "pretty" {
				values := map[string]string{}
				lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
				if len(lines) != 4 {
					t.Fatalf("pretty mock lines = %d, want four: %q", len(lines), stdout)
				}
				for _, line := range lines {
					parts := strings.SplitN(line, ":", 2)
					if len(parts) != 2 {
						t.Fatalf("pretty mock line is not key/value: %q", line)
					}
					values[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
				}
				if len(values) != 4 || values["apps"] != "[]" || values["count"] != "0" ||
					values["hasMore"] != "false" || values["nextCursor"] != "" {
					t.Fatalf("pretty mock values = %#v, output=%q", values, stdout)
				}
				return
			}
			if format == "ndjson" && len(strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")) != 1 {
				t.Fatalf("ndjson mock output expanded envelope: %q", stdout)
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
				t.Fatalf("decode mock output: %v, output=%q", err, stdout)
			}
			apps, ok := payload["apps"].([]any)
			if len(payload) != 4 || !ok || len(apps) != 0 || payload["count"] != float64(0) ||
				payload["hasMore"] != false || payload["nextCursor"] != "" {
				t.Fatalf("mock pagination payload = %#v", payload)
			}
		})
	}
}

type devAppListFixtureRunner struct {
	response string
}

func (r devAppListFixtureRunner) Run(
	_ context.Context,
	invocation executor.Invocation,
) (executor.Result, error) {
	return executor.Result{
		Invocation: invocation,
		Response: map[string]any{
			"content": []any{map[string]any{"type": "text", "text": r.response}},
		},
	}, nil
}

func TestCrossPlatformCoverageDevAppListInvalidContractJSONError(t *testing.T) {
	helpers.InitDepsForTest(t, helpers.GetCaller())
	testseam.Swap(t, &rootNewCommandRunnerWithFlags, func(*GlobalFlags) executor.Runner {
		return devAppListFixtureRunner{response: `{
			"apps":[{"unifiedAppId":"would-be-partial"},"payload-do-not-leak"],
			"hasMore":false,
			"nextCursor":""
		}`}
	})

	root := NewRootCommand()
	var stdout bytes.Buffer
	var cobraStderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&cobraStderr)
	root.SetArgs([]string{"devapp", "+list", "--cursor", "cursor-do-not-leak", "--format", "json"})
	err := root.Execute()
	if err == nil {
		t.Fatal("invalid pagination contract unexpectedly succeeded")
	}
	if stdout.Len() != 0 || cobraStderr.Len() != 0 {
		t.Fatalf("invalid contract leaked command output: stdout=%q stderr=%q", stdout.String(), cobraStderr.String())
	}
	if code := apperrors.ExitCode(err); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}

	var machineStderr bytes.Buffer
	if printErr := printExecutionError(root, &stdout, &machineStderr, err); printErr != nil {
		t.Fatalf("print JSON error: %v", printErr)
	}
	if stdout.Len() != 0 {
		t.Fatalf("JSON error wrote stdout: %q", stdout.String())
	}
	decoder := json.NewDecoder(strings.NewReader(machineStderr.String()))
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		t.Fatalf("decode stderr JSON: %v, stderr=%q", err, machineStderr.String())
	}
	if err := decoder.Decode(&map[string]any{}); err != io.EOF {
		t.Fatalf("stderr contained more than one JSON value: %v, stderr=%q", err, machineStderr.String())
	}
	errorPayload, ok := payload["error"].(map[string]any)
	message, messageOK := errorPayload["message"].(string)
	if len(payload) != 1 || !ok || len(errorPayload) != 4 ||
		errorPayload["category"] != "api" || errorPayload["code"] != float64(1) ||
		errorPayload["reason"] != "devapp_pagination_contract_invalid" ||
		!messageOK || strings.TrimSpace(message) == "" {
		t.Fatalf("JSON error contract = %#v", payload)
	}
	if strings.Contains(machineStderr.String(), "cursor-do-not-leak") ||
		strings.Contains(machineStderr.String(), "payload-do-not-leak") ||
		strings.Contains(machineStderr.String(), "would-be-partial") {
		t.Fatalf("JSON error leaked request or response data: %q", machineStderr.String())
	}
}

func TestCrossPlatformCoverageDevAppListMockIsolation(t *testing.T) {
	helpers.InitDepsForTest(t, helpers.GetCaller())
	runner := newCommandRunnerWithFlags(&GlobalFlags{Mock: true})

	for _, test := range []struct {
		name    string
		product string
		tool    string
	}{
		{name: "other devapp tool", product: "devapp", tool: "get_dev_app"},
		{name: "other product", product: "contact", tool: "list_contacts"},
	} {
		t.Run(test.name, func(t *testing.T) {
			invocation := executor.NewHelperInvocation(
				"mock.isolation", test.product, test.tool, map[string]any{},
			)
			result, err := runner.Run(context.Background(), invocation)
			if err != nil {
				t.Fatalf("mock invocation error = %v", err)
			}
			content, ok := result.Response["content"].(map[string]any)
			if !ok {
				t.Fatalf("mock content = %#v", result.Response["content"])
			}
			mockResult, ok := content["result"].([]any)
			if !ok || len(mockResult) != 0 {
				t.Fatalf("non-target mock result changed = %#v", content["result"])
			}
		})
	}
}
