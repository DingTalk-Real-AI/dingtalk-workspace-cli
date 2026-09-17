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
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type docImportTargetCall struct {
	server string
	tool   string
	args   map[string]any
}

type docImportTargetCaller struct {
	responses map[string][]scriptedToolStep
	calls     []docImportTargetCall
	dryRun    bool
}

func (c *docImportTargetCaller) CallTool(_ context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, docImportTargetCall{server: server, tool: tool, args: args})
	steps := c.responses[tool]
	if len(steps) == 0 {
		return nil, errors.New("unexpected tool call " + server + "/" + tool)
	}
	step := steps[0]
	c.responses[tool] = steps[1:]
	if step.err != nil {
		return nil, step.err
	}
	return textToolResult(step.text), nil
}

func (*docImportTargetCaller) Format() string { return "json" }
func (c *docImportTargetCaller) DryRun() bool { return c.dryRun }
func (*docImportTargetCaller) Fields() string { return "" }
func (*docImportTargetCaller) JQ() string     { return "" }

func successfulDocImportResponses() map[string][]scriptedToolStep {
	return map[string][]scriptedToolStep{
		"create_import_session": {{text: `{"sessionId":"session-1","uploadUrl":"https://upload.example.test/object"}`}},
		"confirm_import":        {{text: `{"taskId":"task-1"}`}},
		"query_import_task":     {{text: `{"status":"completed","documentUrl":"https://alidocs.dingtalk.com/i/nodes/node-1","documentName":"Sales","documentType":"0"}`}},
	}
}

func runDocImportTargetFlow(t *testing.T, caller *docImportTargetCaller, fileExt, folder, workspace string) (map[string]any, error) {
	t.Helper()
	previousDeps := deps
	previousArgs := os.Args
	t.Cleanup(func() {
		deps = previousDeps
		os.Args = previousArgs
		SetHTTPPutFile(nil)
	})

	InitDeps(caller)
	var output bytes.Buffer
	deps.Out.w = &output
	deps.Out.errW = io.Discard
	os.Args = []string{"dws", "doc"}
	SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error { return nil })

	cmd := htmlFallbackCommand(t, writeImportFixture(t, fileExt))
	if folder != "" {
		if err := cmd.Flags().Set("folder", folder); err != nil {
			t.Fatal(err)
		}
	}
	if workspace != "" {
		if err := cmd.Flags().Set("workspace", workspace); err != nil {
			t.Fatal(err)
		}
	}
	cfg := docImportFlowConfig()
	cfg.poll.maxPolls = 1
	cfg.poll.interval = func(int) time.Duration { return 0 }
	cfg.poll.wait = func(context.Context, time.Duration) error { return nil }
	if !caller.dryRun {
		if _, exists := caller.responses["get_document_info"]; !exists {
			verifiedFolder := folder
			if verifiedFolder == "" && workspace == "" {
				verifiedFolder = "root-folder-1"
			}
			info := map[string]any{"nodeId": "node-1"}
			if verifiedFolder != "" {
				info["folderId"] = verifiedFolder
			}
			if workspace != "" {
				info["workspaceId"] = workspace
			}
			encoded, err := json.Marshal(info)
			if err != nil {
				t.Fatal(err)
			}
			caller.responses["get_document_info"] = []scriptedToolStep{{text: string(encoded)}}
		}
	}
	err := runImportCommand(cmd, nil, cfg)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatalf("doc import output is not JSON: %v\n%s", err, output.String())
	}
	return payload, nil
}

func assertImportPreflightError(t *testing.T, err error) {
	t.Helper()
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation {
		t.Fatalf("error = %T %v, want validation error", err, err)
	}
	if typed.ExecutionStarted == nil || *typed.ExecutionStarted {
		t.Fatalf("execution_started = %#v, want false", typed.ExecutionStarted)
	}
}

func TestCrossPlatformCoverageDocImportResolvesUniqueOrgRootBeforeWrite(t *testing.T) {
	responses := successfulDocImportResponses()
	responses["list_spaces"] = []scriptedToolStep{{text: `{"success":true,"result":{"items":[{"spaceId":"29218217248","rootFolderId":"root-folder-1","spaceType":"orgSpace"}]}}`}}
	caller := &docImportTargetCaller{responses: responses}

	payload, err := runDocImportTargetFlow(t, caller, "docx", "", "")
	if err != nil {
		t.Fatalf("doc import returned error: %v", err)
	}
	if len(caller.calls) != 5 {
		t.Fatalf("calls = %#v, want resolver, three import calls, and placement readback", caller.calls)
	}
	if got := caller.calls[0]; got.server != "drive" || got.tool != "list_spaces" || !reflect.DeepEqual(got.args, map[string]any{"spaceType": "orgSpace"}) {
		t.Fatalf("resolver call = %#v", got)
	}
	if got := caller.calls[1]; got.server != "doc" || got.tool != "create_import_session" || got.args["targetFolderId"] != "root-folder-1" {
		t.Fatalf("create session call = %#v", got)
	}
	if _, exists := caller.calls[1].args["workspaceId"]; exists {
		t.Fatalf("default root must use targetFolderId, got %#v", caller.calls[1].args)
	}
	if payload["success"] != true || payload["targetSource"] != "default_org_root" || payload["targetFolderId"] != "root-folder-1" {
		t.Fatalf("target receipt missing: %#v", payload)
	}
}

func TestCrossPlatformCoverageDocImportExplicitTargetSkipsResolver(t *testing.T) {
	tests := []struct {
		name       string
		folder     string
		workspace  string
		wantKey    string
		wantValue  string
		wantSource string
	}{
		{name: "folder", folder: "folder-1", wantKey: "targetFolderId", wantValue: "folder-1", wantSource: "explicit_folder"},
		{name: "workspace", workspace: "workspace-1", wantKey: "workspaceId", wantValue: "workspace-1", wantSource: "explicit_workspace"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &docImportTargetCaller{responses: successfulDocImportResponses()}
			payload, err := runDocImportTargetFlow(t, caller, "docx", test.folder, test.workspace)
			if err != nil {
				t.Fatalf("doc import returned error: %v", err)
			}
			if len(caller.calls) != 4 || caller.calls[0].tool != "create_import_session" || caller.calls[3].tool != "get_document_info" {
				t.Fatalf("explicit target unexpectedly resolved defaults: %#v", caller.calls)
			}
			if caller.calls[0].args[test.wantKey] != test.wantValue {
				t.Fatalf("session args = %#v", caller.calls[0].args)
			}
			if payload[test.wantKey] != test.wantValue || payload["targetSource"] != test.wantSource {
				t.Fatalf("target receipt = %#v", payload)
			}
		})
	}
}

func TestCrossPlatformCoverageDocImportRejectsAmbiguousOrMissingDefaultBeforeWrite(t *testing.T) {
	t.Run("folder and workspace conflict", func(t *testing.T) {
		caller := &docImportTargetCaller{responses: map[string][]scriptedToolStep{}}
		_, err := runDocImportTargetFlow(t, caller, "docx", "folder-1", "workspace-1")
		assertImportPreflightError(t, err)
		if len(caller.calls) != 0 {
			t.Fatalf("conflict made remote calls: %#v", caller.calls)
		}
	})

	for _, test := range []struct {
		name string
		body string
		want string
	}{
		{name: "invalid json", body: `{`, want: "无法解析"},
		{name: "missing items", body: `{"result":{}}`, want: "缺少 items"},
		{name: "zero spaces", body: `{"result":{"items":[]}}`, want: "0 个 orgSpace"},
		{name: "multiple spaces", body: `{"result":{"items":[{"rootFolderId":"a"},{"rootFolderId":"b"}]}}`, want: "2 个 orgSpace"},
		{name: "invalid space item", body: `{"result":{"items":[1]}}`, want: "返回结构无效"},
		{name: "missing root folder", body: `{"result":{"items":[{"spaceType":"orgSpace"}]}}`, want: "未返回 rootFolderId"},
		{name: "wrong type", body: `{"result":{"items":[{"spaceType":"mySpace","rootFolderId":"root"}]}}`, want: "不是 orgSpace"},
		{name: "first page is not unique", body: `{"result":{"items":[{"spaceType":"orgSpace","rootFolderId":"root"}],"nextToken":"page-2"}}`, want: "仍有下一页"},
		{name: "has more without a usable cursor", body: `{"result":{"items":[{"spaceType":"orgSpace","rootFolderId":"root"}],"hasMore":true}}`, want: "仍有下一页"},
		{name: "outer next token", body: `{"result":{"items":[{"spaceType":"orgSpace","rootFolderId":"root"}]},"nextToken":"page-2"}`, want: "仍有下一页"},
		{name: "outer has more", body: `{"result":{"items":[{"spaceType":"orgSpace","rootFolderId":"root"}]},"hasMore":true}`, want: "仍有下一页"},
		{name: "invalid next token fails closed", body: `{"result":{"items":[{"spaceType":"orgSpace","rootFolderId":"root"}],"nextToken":1}}`, want: "仍有下一页"},
		{name: "invalid has more fails closed", body: `{"result":{"items":[{"spaceType":"orgSpace","rootFolderId":"root"}],"hasMore":"true"}}`, want: "仍有下一页"},
	} {
		t.Run(test.name, func(t *testing.T) {
			caller := &docImportTargetCaller{responses: map[string][]scriptedToolStep{"list_spaces": {{text: test.body}}}}
			_, err := runDocImportTargetFlow(t, caller, "docx", "", "")
			assertImportPreflightError(t, err)
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
			if len(caller.calls) != 1 || caller.calls[0].tool != "list_spaces" {
				t.Fatalf("failure must stop before create_import_session: %#v", caller.calls)
			}
		})
	}

	t.Run("resolver API failure", func(t *testing.T) {
		caller := &docImportTargetCaller{responses: map[string][]scriptedToolStep{"list_spaces": {{err: errors.New("permission denied")}}}}
		_, err := runDocImportTargetFlow(t, caller, "docx", "", "")
		assertImportPreflightError(t, err)
		if !strings.Contains(err.Error(), "显式提供") || len(caller.calls) != 1 {
			t.Fatalf("error/calls = %v / %#v", err, caller.calls)
		}
	})

	t.Run("resolver returns an empty target", func(t *testing.T) {
		previousDeps := deps
		previousArgs := os.Args
		t.Cleanup(func() {
			deps = previousDeps
			os.Args = previousArgs
		})
		caller := &docImportTargetCaller{responses: map[string][]scriptedToolStep{}}
		InitDeps(caller)
		deps.Out.w = io.Discard
		deps.Out.errW = io.Discard
		os.Args = []string{"dws", "doc"}
		cmd := htmlFallbackCommand(t, writeImportFixture(t, "docx"))
		cfg := docImportFlowConfig()
		cfg.resolveDefaultTarget = func(context.Context) (importTarget, error) {
			return importTarget{}, nil
		}
		err := runImportCommand(cmd, nil, cfg)
		assertImportPreflightError(t, err)
		if !strings.Contains(err.Error(), "解析结果为空") || len(caller.calls) != 0 {
			t.Fatalf("error/calls = %v / %#v", err, caller.calls)
		}
	})
}

func TestCrossPlatformCoverageDocImportDryRunDefersDefaultTargetRead(t *testing.T) {
	for _, extension := range []string{"docx", "html"} {
		t.Run(extension, func(t *testing.T) {
			caller := &docImportTargetCaller{responses: map[string][]scriptedToolStep{}, dryRun: true}
			payload, err := runDocImportTargetFlow(t, caller, extension, "", "")
			if err != nil {
				t.Fatalf("dry-run returned error: %v", err)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("dry-run made remote calls: %#v", caller.calls)
			}
			if payload["targetSource"] != "default_org_root" || payload["targetResolution"] != "deferred" || payload["executed"] != false {
				t.Fatalf("dry-run target plan = %#v", payload)
			}
			if extension == "html" && payload["fallback"] != "upload" {
				t.Fatalf("fallback dry-run markers = %#v", payload)
			}
		})
	}
}

func TestCrossPlatformCoverageDocImportFallbackResolvesDefaultBeforeUpload(t *testing.T) {
	t.Run("unique org root is passed to upload and returned in receipt", func(t *testing.T) {
		caller := &docImportTargetCaller{responses: map[string][]scriptedToolStep{
			"list_spaces":          {{text: `{"result":{"items":[{"spaceType":"orgSpace","rootFolderId":"root-folder-1"}]}}`}},
			"get_file_upload_info": {{text: `{"resourceUrl":"https://upload.example.test/object","uploadKey":"key-1"}`}},
			"commit_uploaded_file": {{text: `{"dentryUuid":"node-1","name":"page.html"}`}},
		}}
		payload, err := runDocImportTargetFlow(t, caller, "html", "", "")
		if err != nil {
			t.Fatalf("fallback import returned error: %v", err)
		}
		if len(caller.calls) != 4 || caller.calls[0].tool != "list_spaces" || caller.calls[2].tool != "commit_uploaded_file" || caller.calls[3].tool != "get_document_info" {
			t.Fatalf("fallback calls = %#v", caller.calls)
		}
		if got := caller.calls[2].args["folderId"]; got != "root-folder-1" {
			t.Fatalf("commit folderId = %v, want root-folder-1", got)
		}
		if payload["targetFolderId"] != "root-folder-1" || payload["targetSource"] != "default_org_root" || payload["fallback"] != "upload" {
			t.Fatalf("fallback target receipt = %#v", payload)
		}
	})

	t.Run("ambiguous org roots stop before upload", func(t *testing.T) {
		caller := &docImportTargetCaller{responses: map[string][]scriptedToolStep{
			"list_spaces": {{text: `{"result":{"items":[{"rootFolderId":"a"},{"rootFolderId":"b"}]}}`}},
		}}
		_, err := runDocImportTargetFlow(t, caller, "html", "", "")
		assertImportPreflightError(t, err)
		if len(caller.calls) != 1 || caller.calls[0].tool != "list_spaces" {
			t.Fatalf("ambiguous fallback must stop before upload: %#v", caller.calls)
		}
	})
}
