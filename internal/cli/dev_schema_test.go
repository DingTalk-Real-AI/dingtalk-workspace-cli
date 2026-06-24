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

package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
)

// buildHelperTestTree mirrors the shape of the real `dws dev` subtree closely
// enough to exercise the live-schema renderer: a group and leaves carrying the
// `mcp-tool` annotation that names the op-app tool to fetch.
func buildHelperTestTree() *cobra.Command {
	root := &cobra.Command{Use: "dws"}

	create := &cobra.Command{
		Use:         "create",
		Short:       "创建应用",
		Annotations: map[string]string{"mcp-tool": "create_dev_app"},
		Run:         func(*cobra.Command, []string) {},
	}

	config := &cobra.Command{
		Use:         "config",
		Short:       "配置机器人",
		Annotations: map[string]string{"mcp-tool": "set_extension_robot_config"},
		Run:         func(*cobra.Command, []string) {},
	}

	// A leaf without an mcp-tool annotation (e.g. dev connect / dev doc search).
	noTool := &cobra.Command{Use: "connect", Short: "无 MCP 工具", Run: func(*cobra.Command, []string) {}}

	robot := &cobra.Command{Use: "robot", Short: "机器人能力"}
	robot.AddCommand(config)

	app := &cobra.Command{Use: "app", Short: "应用"}
	app.AddCommand(create, robot)

	dev := &cobra.Command{Use: "dev", Short: "开放平台开发者命令"}
	dev.AddCommand(app, noTool)

	root.AddCommand(dev)
	return root
}

// fakeFetcher returns a canned op-app tools/list so the renderer is exercised
// without network. It mirrors the MCP shape: properties keyed by camelCase param
// name, required[] listing the camelCase names.
func fakeFetcher(tools map[string]HelperToolSchema) HelperToolFetcher {
	return func(context.Context, string) (map[string]HelperToolSchema, error) {
		return tools, nil
	}
}

func robotConfigToolSchema() HelperToolSchema {
	return HelperToolSchema{
		Name:        "set_extension_robot_config",
		Description: "创建或更新现有应用的机器人配置",
		Properties: map[string]any{
			"unifiedAppId":     map[string]any{"type": "string", "description": "统一应用 ID"},
			"eventCallbackUrl": map[string]any{"type": "string", "description": "事件回调地址"},
			"skills":           map[string]any{"type": "array", "description": "技能列表"},
			"mode":             map[string]any{"type": "string", "description": "机器人模式", "enum": []any{"HTTPS", "STREAM", "AISKILL"}},
		},
		Required: []string{"unifiedAppId"},
	}
}

func TestRenderHelperSchema_LeafGwsFlat(t *testing.T) {
	root := buildHelperTestTree()
	fetch := fakeFetcher(map[string]HelperToolSchema{
		"set_extension_robot_config": robotConfigToolSchema(),
	})

	payload, ok, err := renderHelperSchema(context.Background(), root, "dev app robot config", fetch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected helper renderer to claim the path")
	}

	// Flat top-level: description / path / source / parameters; no wrapper.
	if payload["description"] != "创建或更新现有应用的机器人配置" {
		t.Fatalf("description = %v", payload["description"])
	}
	if payload["path"] != "dev app robot config" {
		t.Fatalf("path = %v", payload["path"])
	}
	if payload["source"] != "mcp:op-app" {
		t.Fatalf("source = %v", payload["source"])
	}
	for _, leaked := range []string{"kind", "tool", "product", "helper"} {
		if _, present := payload[leaked]; present {
			t.Fatalf("gws-flat output must not carry %q wrapper key", leaked)
		}
	}

	params, _ := payload["parameters"].(map[string]any)
	if params == nil {
		t.Fatalf("no parameters: %#v", payload)
	}

	// Keys are kebab-case of the MCP param name == the CLI flag.
	uid, _ := params["unified-app-id"].(map[string]any)
	if uid == nil {
		t.Fatalf("missing unified-app-id param: %#v", params)
	}
	if uid["type"] != "string" || uid["required"] != true {
		t.Fatalf("unified-app-id = %#v, want string+required", uid)
	}
	if _, hasDefault := uid["default"]; hasDefault {
		t.Fatal("unified-app-id must not carry a default (MCP provides none)")
	}

	cb, _ := params["event-callback-url"].(map[string]any)
	if cb == nil || cb["required"] != false {
		t.Fatalf("event-callback-url = %#v, want required=false", cb)
	}

	skills, _ := params["skills"].(map[string]any)
	if skills == nil || skills["type"] != "array" {
		t.Fatalf("skills = %#v, want array", skills)
	}

	mode, _ := params["mode"].(map[string]any)
	if mode == nil || mode["type"] != "string" {
		t.Fatalf("mode = %#v, want string", mode)
	}
	if _, hasDefault := mode["default"]; hasDefault {
		t.Fatalf("mode default = %v, want none", mode["default"])
	}
	if mode["required"] != false {
		t.Fatalf("mode required = %v, want false", mode["required"])
	}
}

func TestRenderHelperSchema_Group(t *testing.T) {
	root := buildHelperTestTree()
	payload, ok, err := renderHelperSchema(context.Background(), root, "dev app", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected claim")
	}
	if payload["path"] != "dev app" {
		t.Fatalf("path = %v", payload["path"])
	}
	cmds, _ := payload["commands"].([]map[string]any)
	if len(cmds) != 2 { // create + robot
		t.Fatalf("commands count = %d, want 2", len(cmds))
	}
}

func TestRenderHelperSchema_NoAnnotation(t *testing.T) {
	root := buildHelperTestTree()
	payload, ok, err := renderHelperSchema(context.Background(), root, "dev connect", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected claim")
	}
	if payload["error"] == nil {
		t.Fatalf("expected a clear no-MCP-tool error, got %#v", payload)
	}
	if _, present := payload["parameters"]; present {
		t.Fatal("no-tool command must not emit parameters")
	}
}

func TestRenderHelperSchema_UnknownSubcommand(t *testing.T) {
	root := buildHelperTestTree()
	payload, ok, err := renderHelperSchema(context.Background(), root, "dev app nope", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected claim")
	}
	if payload["error"] == nil {
		t.Fatalf("expected error for unknown subcommand, got %#v", payload)
	}
	if avail, _ := payload["available"].([]map[string]any); len(avail) == 0 {
		t.Fatal("expected available subcommands listed")
	}
}

func TestRenderHelperSchema_ToolMissingInList(t *testing.T) {
	root := buildHelperTestTree()
	// Fetcher returns an empty list — the annotated tool isn't present.
	payload, ok, err := renderHelperSchema(context.Background(), root, "dev app create", fakeFetcher(map[string]HelperToolSchema{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected claim")
	}
	if payload["error"] == nil {
		t.Fatalf("expected not-found error, got %#v", payload)
	}
}

func TestRenderHelperSchema_FetchError(t *testing.T) {
	root := buildHelperTestTree()
	failing := func(context.Context, string) (map[string]HelperToolSchema, error) {
		return nil, errors.New("network down")
	}
	_, ok, err := renderHelperSchema(context.Background(), root, "dev app create", failing)
	if !ok {
		t.Fatal("expected claim even on fetch error")
	}
	if err == nil {
		t.Fatal("expected the fetch error to surface")
	}
}

func TestRenderHelperSchema_NonHelperPathDeclined(t *testing.T) {
	root := buildHelperTestTree()
	if _, ok, _ := renderHelperSchema(context.Background(), root, "ding.message.send", nil); ok {
		t.Fatal("non-helper path must not be claimed by the helper renderer")
	}
}

func TestKebabCase(t *testing.T) {
	cases := map[string]string{
		"eventCallbackUrl": "event-callback-url",
		"unifiedAppId":     "unified-app-id",
		"disableSSLVerify": "disable-ssl-verify",
		"mode":             "mode",
		"skills":           "skills",
		"i18nName":         "i18n-name",
	}
	for in, want := range cases {
		if got := kebabCase(in); got != want {
			t.Errorf("kebabCase(%q) = %q, want %q", in, got, want)
		}
	}
}
