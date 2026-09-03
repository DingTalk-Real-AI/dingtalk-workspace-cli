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
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

type mcpCoverageErrorRunner struct {
	err      error
	response map[string]any
}

func (r mcpCoverageErrorRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	return executor.Result{Invocation: invocation, Response: r.response}, r.err
}

type mcpCoverageStringer string

func (s mcpCoverageStringer) String() string { return string(s) }

func setMCPFlag(t *testing.T, cmd *cobra.Command, name, value string) {
	t.Helper()
	if err := cmd.Flags().Set(name, value); err != nil {
		t.Fatalf("set --%s: %v", name, err)
	}
}

func requireMCPError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func runMCPCommandBody(t *testing.T, cmd *cobra.Command) error {
	t.Helper()
	if cmd.RunE == nil {
		t.Fatalf("%s has no RunE", cmd.Name())
	}
	return cmd.RunE(cmd, nil)
}

func runMCPUncheckedBody(t *testing.T, cmd *cobra.Command) error {
	t.Helper()
	if runtime, ok := loadContractRuntime(cmd); ok {
		runtime.validate = nil
		runtime.confirm = false
	}
	return runMCPCommandBody(t, cmd)
}

func newHTTPUpsertCoverageCommand(t *testing.T, update bool) *cobra.Command {
	t.Helper()
	cmd := newDevMCPToolCreateCommand(&captureRunner{})
	if update {
		cmd = newDevMCPToolUpdateCommand(&captureRunner{})
		setMCPFlag(t, cmd, "tool-id", "G-ACT-1")
	}
	setMCPFlag(t, cmd, "mcp-id", "1")
	setMCPFlag(t, cmd, "name", "get_record")
	setMCPFlag(t, cmd, "http-info", `{"method":"GET","url":"https://example.test"}`)
	return cmd
}

func completeHTTPUpsertCoverageCommand(t *testing.T, update bool) *cobra.Command {
	t.Helper()
	cmd := newHTTPUpsertCoverageCommand(t, update)
	setMCPFlag(t, cmd, "api-outputs", `{"body":[{"key":"id","type":"string"}]}`)
	setMCPFlag(t, cmd, "tool-outputs", `[]`)
	setMCPFlag(t, cmd, "output-mappings", `[{"target":"$","type":"reference","source":"$.node_service_activator.Body"}]`)
	return cmd
}

func newHSFCreateCoverageCommand(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := newDevMCPToolCreateHsfCommand(&captureRunner{})
	setMCPFlag(t, cmd, "mcp-id", "1")
	setMCPFlag(t, cmd, "name", "get_record")
	setMCPFlag(t, cmd, "hsf-info", `{"interfaceName":"a.b.C","methodName":"get"}`)
	setMCPFlag(t, cmd, "tool-inputs", `[]`)
	setMCPFlag(t, cmd, "input-mappings", `[]`)
	setMCPFlag(t, cmd, "tool-outputs", `[]`)
	setMCPFlag(t, cmd, "output-mappings", `[]`)
	return cmd
}

func newHSFUpdateCoverageCommand(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := newDevMCPToolUpdateHsfCommand(&captureRunner{})
	setMCPFlag(t, cmd, "mcp-id", "1")
	setMCPFlag(t, cmd, "tool-id", "G-ACT-1")
	return cmd
}

func TestDevMCPCoverageGroupHelp(t *testing.T) {
	for _, args := range [][]string{
		{"dev", "mcp"},
		{"dev", "mcp", "service"},
		{"dev", "mcp", "tool"},
		{"dev", "mcp", "url"},
		{"dev", "mcp", "auth"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			root := newDevAppTestRoot(&captureRunner{})
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(args)
			if err := root.ExecuteContext(t.Context()); err != nil {
				t.Fatalf("execute help: %v", err)
			}
			if out.Len() == 0 {
				t.Fatal("expected help output")
			}
		})
	}
}

func TestDevMCPCoverageCommandRunErrors(t *testing.T) {
	failed := mcpCoverageErrorRunner{err: errors.New("runner failed")}

	tests := []struct {
		name    string
		cmd     func() *cobra.Command
		flags   map[string]string
		wantErr string
	}{
		{name: "url missing id", cmd: func() *cobra.Command { return newDevMCPURLGetCommand(&captureRunner{}) }, wantErr: "--mcp-id"},
		{name: "url missing source", cmd: func() *cobra.Command { return newDevMCPURLGetCommand(&captureRunner{}) }, flags: map[string]string{"mcp-id": "1", "source": " "}, wantErr: "--source"},
		{name: "service create missing name", cmd: func() *cobra.Command { return newDevMCPServiceCreateCommand(&captureRunner{}) }, wantErr: "--name"},
		{name: "service create missing description", cmd: func() *cobra.Command { return newDevMCPServiceCreateCommand(&captureRunner{}) }, flags: map[string]string{"name": "x"}, wantErr: "--description"},
		{name: "service create invalid server", cmd: func() *cobra.Command { return newDevMCPServiceCreateCommand(&captureRunner{}) }, flags: map[string]string{"name": "x", "description": "y", "server-name": "bad_name"}, wantErr: "kebab-case"},
		{name: "service update missing id", cmd: func() *cobra.Command { return newDevMCPServiceUpdateCommand(&captureRunner{}) }, wantErr: "--mcp-id"},
		{name: "service update invalid server", cmd: func() *cobra.Command { return newDevMCPServiceUpdateCommand(&captureRunner{}) }, flags: map[string]string{"mcp-id": "1", "server-name": "bad_name"}, wantErr: "kebab-case"},
		{name: "service update no changes", cmd: func() *cobra.Command { return newDevMCPServiceUpdateCommand(&captureRunner{}) }, flags: map[string]string{"mcp-id": "1"}, wantErr: "至少提供一项"},
		{name: "service delete missing id", cmd: func() *cobra.Command { return newDevMCPServiceDeleteCommand(&captureRunner{}) }, wantErr: "--mcp-id"},
		{name: "tool create missing id", cmd: func() *cobra.Command { return newDevMCPToolCreateCommand(&captureRunner{}) }, wantErr: "--mcp-id"},
		{name: "tool update missing id", cmd: func() *cobra.Command { return newDevMCPToolUpdateCommand(&captureRunner{}) }, wantErr: "--mcp-id"},
		{name: "tool debug missing locator", cmd: func() *cobra.Command { return newDevMCPToolDebugCommand(&captureRunner{}) }, wantErr: "--mcp-id"},
		{name: "tool debug missing value", cmd: func() *cobra.Command { return newDevMCPToolDebugCommand(&captureRunner{}) }, flags: map[string]string{"mcp-id": "1", "tool-id": "G-ACT-1"}, wantErr: "--value"},
		{name: "tool debug missing credential choice", cmd: func() *cobra.Command { return newDevMCPToolDebugCommand(&captureRunner{}) }, flags: map[string]string{"mcp-id": "1", "tool-id": "G-ACT-1", "value": `{}`}, wantErr: "调试需指定"},
		{name: "tool publish missing locator", cmd: func() *cobra.Command { return newDevMCPToolPublishCommand(&captureRunner{}) }, wantErr: "--mcp-id"},
		{name: "tool delete missing locator", cmd: func() *cobra.Command { return newDevMCPToolDeleteCommand(&captureRunner{}) }, wantErr: "--mcp-id"},
		{name: "auth save missing id", cmd: func() *cobra.Command { return newDevMCPAuthConfigSaveCommand(&captureRunner{}) }, wantErr: "--mcp-id"},
		{name: "auth save missing type", cmd: func() *cobra.Command { return newDevMCPAuthConfigSaveCommand(&captureRunner{}) }, flags: map[string]string{"mcp-id": "1"}, wantErr: "--auth-type"},
		{name: "auth save invalid type", cmd: func() *cobra.Command { return newDevMCPAuthConfigSaveCommand(&captureRunner{}) }, flags: map[string]string{"mcp-id": "1", "auth-type": "bad"}, wantErr: "只支持"},
		{name: "auth save invalid config", cmd: func() *cobra.Command { return newDevMCPAuthConfigSaveCommand(&captureRunner{}) }, flags: map[string]string{"mcp-id": "1", "auth-type": "TOKEN", "token-auth-config": `[]`}, wantErr: "JSON 对象"},
		{name: "credential save missing id", cmd: func() *cobra.Command { return newDevMCPCredentialSaveCommand(&captureRunner{}) }, wantErr: "--mcp-id"},
		{name: "credential save missing name", cmd: func() *cobra.Command { return newDevMCPCredentialSaveCommand(&captureRunner{}) }, flags: map[string]string{"mcp-id": "1"}, wantErr: "--name"},
		{name: "credential save missing content", cmd: func() *cobra.Command { return newDevMCPCredentialSaveCommand(&captureRunner{}) }, flags: map[string]string{"mcp-id": "1", "name": "x"}, wantErr: "--content"},
		{name: "member missing id", cmd: func() *cobra.Command {
			return newDevMCPMemberMutationCommand(&captureRunner{}, "add", "add", devMCPMemberAddTool)
		}, wantErr: "--mcp-id"},
		{name: "member missing users", cmd: func() *cobra.Command {
			return newDevMCPMemberMutationCommand(&captureRunner{}, "add", "add", devMCPMemberAddTool)
		}, flags: map[string]string{"mcp-id": "1"}, wantErr: "--user-ids"},
		{name: "hsf method missing interface", cmd: func() *cobra.Command { return newDevMCPHsfMethodListCommand(&captureRunner{}) }, wantErr: "--interface-name"},
		{name: "hsf create missing id", cmd: func() *cobra.Command { return newDevMCPToolCreateHsfCommand(&captureRunner{}) }, wantErr: "--mcp-id"},
		{name: "hsf update missing locator", cmd: func() *cobra.Command { return newDevMCPToolUpdateHsfCommand(&captureRunner{}) }, wantErr: "--mcp-id"},
		{name: "credential unbind missing id", cmd: func() *cobra.Command { return newDevMCPCredentialUnbindCommand(&captureRunner{}) }, wantErr: "--mcp-id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.cmd()
			for name, value := range tt.flags {
				setMCPFlag(t, cmd, name, value)
			}
			requireMCPError(t, runMCPUncheckedBody(t, cmd), tt.wantErr)
		})
	}

	for _, args := range [][]string{
		{"--yes", "dev", "mcp", "service", "create", "--name", "x", "--description", "y"},
		{"--dry-run", "dev", "mcp", "tool", "publish", "--mcp-id", "1", "--tool-id", "G-ACT-1"},
	} {
		root := newDevAppTestRoot(failed)
		root.SetArgs(args)
		requireMCPError(t, root.ExecuteContext(t.Context()), "runner failed")
	}
}

func TestDevMCPCoverageCredentialContentEdges(t *testing.T) {
	cmd := newDevMCPCredentialSaveCommand(&captureRunner{})
	setMCPFlag(t, cmd, "content", `{}`)
	setMCPFlag(t, cmd, "content-file", "also.json")
	_, err := devMCPCredentialContent(cmd)
	requireMCPError(t, err, "只能使用一个")

	cmd = newDevMCPCredentialSaveCommand(&captureRunner{})
	path := filepath.Join(t.TempDir(), "credential.json")
	if err := os.WriteFile(path, []byte(`{"token":"secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	setMCPFlag(t, cmd, "content-file", path)
	content, err := devMCPCredentialContent(cmd)
	if err != nil || content["token"] != "secret" {
		t.Fatalf("content = %#v, error = %v", content, err)
	}

	cmd = newDevMCPCredentialSaveCommand(&captureRunner{})
	setMCPFlag(t, cmd, "content-file", filepath.Join(t.TempDir(), "missing.json"))
	_, err = devMCPCredentialContent(cmd)
	requireMCPError(t, err, "读取 --content-file 失败")

	cmd = newDevMCPCredentialSaveCommand(&captureRunner{})
	_, err = devMCPCredentialContent(cmd)
	requireMCPError(t, err, "为必填")
}

func TestDevMCPCoverageToolUpsertJSONEdges(t *testing.T) {
	tests := []struct {
		name    string
		flag    string
		value   string
		wantErr string
	}{
		{name: "api inputs", flag: "api-inputs", value: `[]`, wantErr: "JSON 对象"},
		{name: "api outputs", flag: "api-outputs", value: `[]`, wantErr: "JSON 对象"},
		{name: "tool inputs", flag: "tool-inputs", value: `{}`, wantErr: "JSON 数组"},
		{name: "tool outputs", flag: "tool-outputs", value: `{}`, wantErr: "JSON 数组"},
		{name: "input mappings", flag: "input-mappings", value: `{}`, wantErr: "JSON 数组"},
		{name: "output mappings", flag: "output-mappings", value: `{}`, wantErr: "JSON 数组"},
		{name: "tool inputs null", flag: "tool-inputs", value: `null`, wantErr: "JSON 数组"},
		{name: "tool outputs null", flag: "tool-outputs", value: `null`, wantErr: "JSON 数组"},
		{name: "input mappings null", flag: "input-mappings", value: `null`, wantErr: "JSON 数组"},
		{name: "output mappings null", flag: "output-mappings", value: `null`, wantErr: "JSON 数组"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newHTTPUpsertCoverageCommand(t, false)
			setMCPFlag(t, cmd, tt.flag, tt.value)
			_, err := devMCPToolUpsertParams(cmd, false)
			requireMCPError(t, err, tt.wantErr)
		})
	}

	cmd := completeHTTPUpsertCoverageCommand(t, false)
	setMCPFlag(t, cmd, "timeout", "0")
	_, err := devMCPToolUpsertParams(cmd, false)
	requireMCPError(t, err, "--timeout")

	cmd = newDevMCPToolUpdateCommand(&captureRunner{})
	setMCPFlag(t, cmd, "mcp-id", "1")
	setMCPFlag(t, cmd, "name", "get_record")
	setMCPFlag(t, cmd, "http-info", `{}`)
	_, err = devMCPToolUpsertParams(cmd, true)
	requireMCPError(t, err, "--tool-id")

	cmd = newDevMCPToolCreateCommand(&captureRunner{})
	setMCPFlag(t, cmd, "mcp-id", "1")
	_, err = devMCPToolUpsertParams(cmd, false)
	requireMCPError(t, err, "--name")
}

func TestDevMCPCoverageValidationEdges(t *testing.T) {
	if err := devMCPValidateToolName(""); err != nil {
		t.Fatalf("blank optional name: %v", err)
	}
	requireMCPError(t, devMCPValidateToolName(strings.Repeat("a", 33)), "不超过 32")
	requireMCPError(t, devMCPValidateToolName("record_value"), "清晰动作词")
	if devMCPContainsKnownVerb("record_value") {
		t.Fatal("unexpected known verb")
	}
	if devMCPKnownVerb("record") {
		t.Fatal("unexpected known verb")
	}

	requireMCPError(t, devMCPValidateAPIFieldsFlag("api-inputs", map[string]any{"body": "bad"}), "必须是字段数组")
	requireMCPError(t, devMCPValidateAPIFieldsFlag("api-inputs", map[string]any{"body": []any{map[string]any{}}}), ".key 为必填")
	requireMCPError(t, devMCPValidateFieldsFlag("fields", []any{"bad"}, false), "必须是 JSON 对象")
	requireMCPError(t, devMCPValidateField("field", map[string]any{"key": "x"}, false), ".type 为必填")
	requireMCPError(t, devMCPValidateField("field", map[string]any{"key": "x", "type": "date"}, false), "只支持")
	requireMCPError(t, devMCPValidateField("field", map[string]any{
		"key": "items", "type": "array", "children": []any{map[string]any{"key": "wrong", "type": "string"}},
	}, false), "必须是 items")
	requireMCPError(t, devMCPValidateField("field", map[string]any{
		"key": "obj", "type": "object", "children": []any{"bad"},
	}, false), "必须是 JSON 对象")
	requireMCPError(t, devMCPValidateField("field", map[string]any{
		"key": "obj", "type": "object", "children": []any{map[string]any{}},
	}, false), ".key 为必填")
	if devMCPValidFieldType("date") {
		t.Fatal("date should not be a valid field type")
	}
	if got := stringValue(mcpCoverageStringer(" value ")); got != "value" {
		t.Fatalf("stringer value = %q", got)
	}
	if got := stringValue(42); got != "42" {
		t.Fatalf("default value = %q", got)
	}

	requireMCPError(t, devMCPValidateMappingsFlag("mappings", []any{"bad"}), "必须是 JSON 对象")
	requireMCPError(t, devMCPValidateMappingsFlag("mappings", []any{map[string]any{"target": "$"}}), ".type 为必填")
	requireMCPError(t, devMCPValidateMappingsFlag("mappings", []any{map[string]any{"target": "$", "type": "bad"}}), "只支持")
	requireMCPError(t, devMCPValidateMappingsFlag("mappings", []any{map[string]any{"target": "$", "type": "fixed"}}), ".source 为必填")

	requireMCPError(t, devMCPValidateToolUpsertParams(map[string]any{
		"name":        "get_record",
		"toolOutputs": []any{map[string]any{}},
	}), ".key 为必填")
	requireMCPError(t, devMCPValidateToolUpsertParams(map[string]any{
		"name":      "get_record",
		"apiInputs": map[string]any{"body": "bad"},
	}), "必须是字段数组")
	requireMCPError(t, devMCPValidateToolUpsertParams(map[string]any{
		"name":       "get_record",
		"apiOutputs": map[string]any{"body": "bad"},
	}), "必须是字段数组")
	requireMCPError(t, devMCPValidateToolUpsertParams(map[string]any{
		"name":           "get_record",
		"outputMappings": []any{map[string]any{}},
	}), ".target 为必填")

	cmd := &cobra.Command{}
	cmd.Flags().String("value", "", "")
	_, err := devMCPRequiredString(cmd, "value")
	requireMCPError(t, err, "--value")
	cmd.Flags().String("array", `{}`, "")
	params := map[string]any{}
	err = devMCPPutJSONArrayFlag(cmd, params, "array", "array")
	requireMCPError(t, err, "JSON 数组")
}

func TestDevMCPCoverageContractValidators(t *testing.T) {
	contract := devMCPContract(nil, "tool", "dev mcp tool", "description", false)
	if got := contract.Selection.Examples[0]; got != "dws dev mcp tool" {
		t.Fatalf("fallback example = %q", got)
	}

	serviceCreate := newDevMCPServiceCreateCommand(&captureRunner{})
	requireMCPError(t, validateDevMCPServiceCreate(serviceCreate, nil), "--name")
	setMCPFlag(t, serviceCreate, "name", "name")
	requireMCPError(t, validateDevMCPServiceCreate(serviceCreate, nil), "--description")

	serviceUpdate := newDevMCPServiceUpdateCommand(&captureRunner{})
	setMCPFlag(t, serviceUpdate, "mcp-id", "1")
	requireMCPError(t, validateDevMCPServiceUpdate(serviceUpdate, nil), "至少提供一项")
	setMCPFlag(t, serviceUpdate, "server-name", "bad_name")
	requireMCPError(t, validateDevMCPServiceUpdate(serviceUpdate, nil), "kebab-case")
	setMCPFlag(t, serviceUpdate, "server-name", "valid-name")
	if err := validateDevMCPServiceUpdate(serviceUpdate, nil); err != nil {
		t.Fatalf("service update with server name: %v", err)
	}

	debug := newDevMCPToolDebugCommand(&captureRunner{})
	setMCPFlag(t, debug, "mcp-id", "1")
	setMCPFlag(t, debug, "tool-id", "G-ACT-1")
	setMCPFlag(t, debug, "value", `{}`)
	setMCPFlag(t, debug, "credential-id", "2")
	setMCPFlag(t, debug, "no-credential", "true")
	requireMCPError(t, validateDevMCPToolDebug(debug, nil), "只能使用一个")

	auth := newDevMCPAuthConfigSaveCommand(&captureRunner{})
	setMCPFlag(t, auth, "mcp-id", "1")
	requireMCPError(t, validateDevMCPAuthSave(auth, nil), "--auth-type")
	setMCPFlag(t, auth, "auth-type", "TOKEN")
	setMCPFlag(t, auth, "token-auth-config", `[]`)
	requireMCPError(t, validateDevMCPAuthSave(auth, nil), "JSON 对象")

	credential := newDevMCPCredentialSaveCommand(&captureRunner{})
	setMCPFlag(t, credential, "mcp-id", "1")
	requireMCPError(t, validateDevMCPCredentialSave(credential, nil), "--name")
	setMCPFlag(t, credential, "name", "account")
	requireMCPError(t, validateDevMCPCredentialSave(credential, nil), "--content")
	setMCPFlag(t, credential, "content", `{}`)
	setMCPFlag(t, credential, "content-file", "file")
	requireMCPError(t, validateDevMCPCredentialSave(credential, nil), "只能使用一个")

	member := newDevMCPMemberMutationCommand(&captureRunner{}, "add", "add", devMCPMemberAddTool)
	setMCPFlag(t, member, "mcp-id", "1")
	requireMCPError(t, validateDevMCPMemberMutation(member, nil), "--user-ids")
}

func TestDevMCPCoverageHSFParamEdges(t *testing.T) {
	createTests := []struct {
		name    string
		mutate  func(*testing.T, *cobra.Command)
		wantErr string
	}{
		{name: "missing name", mutate: func(t *testing.T, cmd *cobra.Command) { setMCPFlag(t, cmd, "name", "") }, wantErr: "--name"},
		{name: "invalid name", mutate: func(t *testing.T, cmd *cobra.Command) { setMCPFlag(t, cmd, "name", "record_value") }, wantErr: "清晰动作词"},
		{name: "invalid hsf info", mutate: func(t *testing.T, cmd *cobra.Command) { setMCPFlag(t, cmd, "hsf-info", `[]`) }, wantErr: "JSON 对象"},
		{name: "invalid list json", mutate: func(t *testing.T, cmd *cobra.Command) { setMCPFlag(t, cmd, "tool-inputs", `{}`) }, wantErr: "JSON 数组"},
		{name: "invalid input field", mutate: func(t *testing.T, cmd *cobra.Command) { setMCPFlag(t, cmd, "tool-inputs", `[{}]`) }, wantErr: ".key 为必填"},
		{name: "invalid output field", mutate: func(t *testing.T, cmd *cobra.Command) { setMCPFlag(t, cmd, "tool-outputs", `[{}]`) }, wantErr: ".key 为必填"},
		{name: "invalid input mapping", mutate: func(t *testing.T, cmd *cobra.Command) { setMCPFlag(t, cmd, "input-mappings", `[{}]`) }, wantErr: ".target 为必填"},
		{name: "invalid output mapping", mutate: func(t *testing.T, cmd *cobra.Command) { setMCPFlag(t, cmd, "output-mappings", `[{}]`) }, wantErr: ".target 为必填"},
		{name: "invalid timeout", mutate: func(t *testing.T, cmd *cobra.Command) { setMCPFlag(t, cmd, "timeout", "0") }, wantErr: "--timeout"},
	}
	for _, tt := range createTests {
		t.Run("create "+tt.name, func(t *testing.T) {
			cmd := newHSFCreateCoverageCommand(t)
			tt.mutate(t, cmd)
			_, err := devMCPToolHsfCreateParams(cmd)
			requireMCPError(t, err, tt.wantErr)
		})
	}

	updateTests := []struct {
		name    string
		mutate  func(*testing.T, *cobra.Command)
		wantErr string
	}{
		{name: "invalid name", mutate: func(t *testing.T, cmd *cobra.Command) { setMCPFlag(t, cmd, "name", "record_value") }, wantErr: "清晰动作词"},
		{name: "invalid hsf info", mutate: func(t *testing.T, cmd *cobra.Command) { setMCPFlag(t, cmd, "hsf-info", `[]`) }, wantErr: "JSON 对象"},
		{name: "invalid list json", mutate: func(t *testing.T, cmd *cobra.Command) { setMCPFlag(t, cmd, "tool-inputs", `{}`) }, wantErr: "JSON 数组"},
		{name: "invalid input field", mutate: func(t *testing.T, cmd *cobra.Command) { setMCPFlag(t, cmd, "tool-inputs", `[{}]`) }, wantErr: ".key 为必填"},
		{name: "invalid output field", mutate: func(t *testing.T, cmd *cobra.Command) { setMCPFlag(t, cmd, "tool-outputs", `[{}]`) }, wantErr: ".key 为必填"},
		{name: "invalid input mapping", mutate: func(t *testing.T, cmd *cobra.Command) { setMCPFlag(t, cmd, "input-mappings", `[{}]`) }, wantErr: ".target 为必填"},
		{name: "invalid output mapping", mutate: func(t *testing.T, cmd *cobra.Command) { setMCPFlag(t, cmd, "output-mappings", `[{}]`) }, wantErr: ".target 为必填"},
		{name: "invalid timeout", mutate: func(t *testing.T, cmd *cobra.Command) { setMCPFlag(t, cmd, "timeout", "0") }, wantErr: "--timeout"},
	}
	for _, tt := range updateTests {
		t.Run("update "+tt.name, func(t *testing.T) {
			cmd := newHSFUpdateCoverageCommand(t)
			tt.mutate(t, cmd)
			_, err := devMCPToolHsfUpdateParams(cmd)
			requireMCPError(t, err, tt.wantErr)
		})
	}

	cmd := newHSFUpdateCoverageCommand(t)
	setMCPFlag(t, cmd, "name", "get_updated_record")
	params, err := devMCPToolHsfUpdateParams(cmd)
	if err != nil || params["name"] != "get_updated_record" {
		t.Fatalf("name update params = %#v, error = %v", params, err)
	}
}

func TestDevMCPCoverageMappingLintEdges(t *testing.T) {
	if err := devMCPLintInputMappings([]any{"skip"}, nil, nil); err != nil {
		t.Fatalf("skip invalid input mapping: %v", err)
	}
	if err := devMCPLintOutputMappings([]any{"skip"}, nil, nil, nil); err != nil {
		t.Fatalf("skip invalid output mapping: %v", err)
	}
	requireMCPError(t, devMCPLintOutputMappings([]any{map[string]any{
		"type": "reference", "source": "$.node_start.missing", "target": "$",
	}}, nil, nil, []any{map[string]any{"key": "present", "type": "string"}}), "缺少字段")

	if err := devMCPLintVariableSource("path", "$.system_node.corpId", nil, "--tool-inputs"); err != nil {
		t.Fatalf("system source: %v", err)
	}
	requireMCPError(t, devMCPLintVariableSource("path", "$.unknown.value", nil, "--tool-inputs"), "不是可解析")
	if err := devMCPLintVariableSource("path", "$.node_start", []any{map[string]any{"key": "x", "type": "string"}}, "--tool-inputs"); err != nil {
		t.Fatalf("node_start root: %v", err)
	}

	requireMCPError(t, devMCPLintActivatorSource("path", "$.node_service_activator", map[string]any{}), "至少要引用")
	requireMCPError(t, devMCPLintActivatorSource("path", "$.node_service_activator.Unknown", map[string]any{}), "不是合法出参位置")

	if err := devMCPLintInputTarget("path", "", nil); err != nil {
		t.Fatalf("blank input target: %v", err)
	}
	requireMCPError(t, devMCPLintInputTarget("path", "$.Body.id", nil), "未提供 --api-inputs")
	requireMCPError(t, devMCPLintInputTarget("path", "$.Head.id", map[string]any{}), "未声明")
	if err := devMCPLintInputTarget("path", "$.Body", map[string]any{"body": []any{}}); err != nil {
		t.Fatalf("group root input target: %v", err)
	}

	requireMCPError(t, devMCPLintOutputTarget("path", "plain", nil), "必须是 $")
	requireMCPError(t, devMCPLintOutputTarget("path", "$.missing", []any{map[string]any{"key": "present", "type": "string"}}), "缺少字段")
	if _, _, ok := devMCPSplitTargetPosition("plain"); ok {
		t.Fatal("plain target should not split")
	}
	if fields, message := devMCPInputGroupFields(map[string]any{}, "Head"); fields != nil || message == "" {
		t.Fatalf("missing input group = %#v, %q", fields, message)
	}
	if _, message := devMCPOutputGroupFields(map[string]any{}, "Head"); message == "" {
		t.Fatal("missing output head should report")
	}
	if _, message := devMCPOutputGroupFields(map[string]any{}, "Unknown"); message == "" {
		t.Fatal("unknown output group should report")
	}
	if devMCPResolveFieldPath([]any{map[string]any{"key": "x", "type": "string"}}, ".") {
		t.Fatal("empty path segment should not resolve")
	}
	if devMCPResolveFieldPath([]any{"skip"}, "x") {
		t.Fatal("non-object field should not resolve")
	}
}

func TestDevMCPCoveragePublishAndStoredMappingEdges(t *testing.T) {
	cmd := newDevMCPToolPublishCommand(&captureRunner{})
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	err := devMCPPublishPreflight(mcpCoverageErrorRunner{err: errors.New("read failed")}, cmd, map[string]any{"mcpId": 1, "toolId": "G-ACT-1"})
	requireMCPError(t, err, "读回工具详情失败")

	err = devMCPPublishPreflight(mcpCoverageErrorRunner{response: map[string]any{"success": true}}, cmd, map[string]any{"mcpId": 1, "toolId": "G-ACT-1"})
	if err != nil || !strings.Contains(stderr.String(), "跳过静态复验") {
		t.Fatalf("missing detail error = %v, stderr = %q", err, stderr.String())
	}

	urlCmd := newDevMCPURLGetCommand(&captureRunner{})
	err = runDevMCPURLGet(mcpCoverageErrorRunner{err: errors.New("url failed")}, urlCmd, map[string]any{"mcpId": 1})
	requireMCPError(t, err, "url failed")

	if got := devMCPExtractToolDetail(nil); got != nil {
		t.Fatalf("nil detail = %#v", got)
	}
	schemaPayload := map[string]any{"outputSchemaMappingJson": "{}"}
	if got := devMCPExtractToolDetail(schemaPayload); got == nil {
		t.Fatal("schema-only detail should be accepted")
	}
	if got := devMCPExtractToolDetail(map[string]any{"name": "none"}); got != nil {
		t.Fatalf("unrelated detail = %#v", got)
	}

	config := `{"rules":"[{\"source\":\"$.node_service_activator\",\"target\":\"$\",\"type\":\"reference\"},{\"source\":\"constant\",\"target\":\"$\",\"type\":\"fixed\"}]"}`
	if broken := devMCPLintStoredOutputMappings(`{}`, config); len(broken) != 1 {
		t.Fatalf("root mapping broken = %#v", broken)
	}
	for _, raw := range []string{`{`, `{"rules":""}`, `{"rules":"{"}`} {
		if rules := devMCPParseStoredRules(raw); len(rules) != 0 {
			t.Fatalf("rules for %q = %#v", raw, rules)
		}
	}

	head := map[string]any{"HEAD": map[string]any{"properties": map[string]any{"requestId": map[string]any{"type": "string"}}}}
	if !devMCPResolveSchemaGroupPath(head, "Head.requestId") {
		t.Fatal("head path should resolve")
	}
	if !devMCPResolveSchemaGroupPath(nil, "HSF.success") {
		t.Fatal("unknown HSF group should be ignored")
	}
	if devMCPResolveSchemaPath(map[string]any{}, ".") {
		t.Fatal("empty schema segment should fail")
	}
	if !devMCPResolveSchemaPath(map[string]any{"type": "array"}, "value") {
		t.Fatal("unknown array items should resolve leniently")
	}
	if devMCPResolveSchemaPath(map[string]any{"type": "object"}, "value") {
		t.Fatal("missing properties should fail")
	}
	if devMCPResolveSchemaPath(map[string]any{"properties": map[string]any{"value": "bad"}}, "value") {
		t.Fatal("non-object property should fail")
	}
	arraySchema := map[string]any{
		"type": "array",
		"items": map[string]any{
			"type":       "object",
			"properties": map[string]any{"value": map[string]any{"type": "string"}},
		},
	}
	if !devMCPResolveSchemaPath(arraySchema, "value") {
		t.Fatal("array items path should resolve")
	}
}

func TestDevMCPCoverageSmallHelpers(t *testing.T) {
	cmd := &cobra.Command{}
	if annotateDevMCPTool(cmd, "tool").Annotations["mcp-tool"] != "tool" {
		t.Fatal("annotation missing")
	}
	long := newDevMCPServiceCreateCommand(&captureRunner{})
	setMCPFlag(t, long, "server-name", strings.Repeat("a", 256))
	_, err := devMCPServerNameFlag(long)
	requireMCPError(t, err, "不超过 255")

	cmd = newDevMCPServiceListCommand(mcpCoverageErrorRunner{err: errors.New("run failed")})
	err = runMCPCommandBody(t, cmd)
	requireMCPError(t, err, "run failed")
}
