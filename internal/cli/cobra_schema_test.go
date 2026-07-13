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
	"testing"

	"github.com/spf13/cobra"
)

// buildEventTestTree mirrors the shape of the real `dws event` subtree closely
// enough to exercise the cobra-flag schema renderer: a group with a runnable
// leaf that carries a positional arg, typed flags (string/int/duration/bool),
// a required flag, a hidden internal flag, and a defaulted flag.
func buildEventTestTree() *cobra.Command {
	root := &cobra.Command{Use: "dws"}
	// A global persistent flag inherited by every command — must NOT appear in a
	// command's synthesized parameters (it describes the CLI, not the command).
	root.PersistentFlags().String("profile", "", "组织 profile")

	consume := &cobra.Command{
		Use:   "consume [event_key]",
		Short: "订阅事件流并输出到 stdout",
		Args:  cobra.MaximumNArgs(1),
		Run:   func(*cobra.Command, []string) {},
	}
	f := consume.Flags()
	f.StringP("format", "f", "ndjson", "输出格式")
	f.String("user", "", "单聊对端 userId")
	f.String("group", "", "群 openConversationId")
	f.Int("max-events", 0, "收到 N 条后退出")
	f.Duration("duration", 0, "运行时长上限")
	f.Bool("ephemeral", false, "退出时强制退订")
	f.String("subscribe-id", "", "复用已有订阅")
	f.String("client-id", "", "内部：覆盖凭证解析")
	_ = f.MarkHidden("client-id")
	// A flag marked required via cobra — must read required:true.
	f.String("token", "", "必填令牌")
	_ = consume.MarkFlagRequired("token")

	stop := &cobra.Command{
		Use:   "stop [subscribe_id]",
		Short: "取消订阅",
		Run:   func(*cobra.Command, []string) {},
	}
	stop.Flags().Bool("all", false, "取消全部")

	event := &cobra.Command{Use: "event", Short: "个人消息事件"}
	// A hidden internal subcommand — must not appear in the browse listing.
	bus := &cobra.Command{Use: "_bus", Short: "内部 bus", Hidden: true, Run: func(*cobra.Command, []string) {}}
	event.AddCommand(consume, stop, bus)

	root.AddCommand(event)
	return root
}

func TestRenderCobraSchema_LeafFlatShape(t *testing.T) {
	root := buildEventTestTree()

	payload, ok, err := renderCobraSchema(root, "event consume")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected cobra renderer to claim the event path")
	}

	if payload["description"] != "订阅事件流并输出到 stdout" {
		t.Fatalf("description = %v", payload["description"])
	}
	if payload["path"] != "event consume" {
		t.Fatalf("path = %v", payload["path"])
	}
	if payload["source"] != "cobra" {
		t.Fatalf("source = %v, want cobra", payload["source"])
	}

	params, _ := payload["parameters"].(map[string]any)
	if params == nil {
		t.Fatalf("no parameters: %#v", payload)
	}

	// Inherited global flag must be excluded.
	if _, present := params["profile"]; present {
		t.Fatal("inherited --profile must not appear in a command's parameters")
	}
	// Hidden internal flag must be excluded.
	if _, present := params["client-id"]; present {
		t.Fatal("hidden --client-id must not appear")
	}

	// Type mapping.
	if got := paramField(t, params, "user", "type"); got != "string" {
		t.Errorf("user type = %v, want string", got)
	}
	if got := paramField(t, params, "max-events", "type"); got != "integer" {
		t.Errorf("max-events type = %v, want integer", got)
	}
	if got := paramField(t, params, "duration", "type"); got != "string" {
		t.Errorf("duration type = %v, want string (CLI string like 10m)", got)
	}
	if got := paramField(t, params, "ephemeral", "type"); got != "boolean" {
		t.Errorf("ephemeral type = %v, want boolean", got)
	}

	// Meaningful default is surfaced; zero defaults are omitted.
	if got := paramField(t, params, "format", "default"); got != "ndjson" {
		t.Errorf("format default = %v, want ndjson", got)
	}
	if _, hasDefault := params["max-events"].(map[string]any)["default"]; hasDefault {
		t.Error("max-events has a zero default (0) — must be omitted")
	}
	if _, hasDefault := params["ephemeral"].(map[string]any)["default"]; hasDefault {
		t.Error("ephemeral has a zero default (false) — must be omitted")
	}
	if _, hasDefault := params["duration"].(map[string]any)["default"]; hasDefault {
		t.Error("duration has a zero default (0s) — must be omitted")
	}

	// Required annotation is honored; unmarked flags read required:false.
	if got := paramField(t, params, "token", "required"); got != true {
		t.Errorf("token required = %v, want true", got)
	}
	if got := paramField(t, params, "user", "required"); got != false {
		t.Errorf("user required = %v, want false", got)
	}
}

func TestRenderCobraSchema_PositionalArguments(t *testing.T) {
	root := buildEventTestTree()

	payload, _, err := renderCobraSchema(root, "event.consume")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	args, _ := payload["arguments"].([]map[string]any)
	if len(args) != 1 {
		t.Fatalf("arguments = %#v, want 1 positional", payload["arguments"])
	}
	if args[0]["name"] != "event_key" {
		t.Errorf("arg name = %v, want event_key", args[0]["name"])
	}
	// [event_key] is optional syntax → required:false.
	if args[0]["required"] != false {
		t.Errorf("arg required = %v, want false", args[0]["required"])
	}
}

func TestRenderCobraSchema_DotAndSpacePathEquivalent(t *testing.T) {
	root := buildEventTestTree()
	dotted, _, _ := renderCobraSchema(root, "event.consume")
	spaced, _, _ := renderCobraSchema(root, "event consume")
	if dotted["path"] != spaced["path"] || dotted["path"] != "event consume" {
		t.Fatalf("dot/space forms diverged: %v vs %v", dotted["path"], spaced["path"])
	}
}

func TestRenderCobraSchema_GroupBrowse(t *testing.T) {
	root := buildEventTestTree()

	payload, ok, err := renderCobraSchema(root, "event")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected claim")
	}
	cmds, _ := payload["commands"].([]map[string]any)
	// consume + stop; hidden _bus excluded.
	if len(cmds) != 2 {
		t.Fatalf("commands = %#v, want 2 (hidden _bus excluded)", cmds)
	}
	for _, c := range cmds {
		if c["cli_path"] == "event _bus" {
			t.Fatal("hidden _bus must not be listed")
		}
	}
}

func TestRenderCobraSchema_UnknownSubcommand(t *testing.T) {
	root := buildEventTestTree()
	payload, ok, err := renderCobraSchema(root, "event nope")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected claim")
	}
	if payload["error"] == nil {
		t.Fatalf("expected unknown-subcommand error, got %#v", payload)
	}
	if avail, _ := payload["available"].([]map[string]any); len(avail) == 0 {
		t.Fatal("expected available subcommands listed")
	}
}

func TestRenderCobraSchema_NonRegisteredPathDeclined(t *testing.T) {
	root := buildEventTestTree()
	if _, ok, _ := renderCobraSchema(root, "dev app create"); ok {
		t.Fatal("non-registered path must not be claimed by the cobra renderer")
	}
	if _, ok, _ := renderCobraSchema(root, "ding.message.send"); ok {
		t.Fatal("non-registered path must not be claimed")
	}
}

// paramField fetches params[<name>][<field>], failing the test if the param is
// absent.
func paramField(t *testing.T, params map[string]any, name, field string) any {
	t.Helper()
	p, _ := params[name].(map[string]any)
	if p == nil {
		t.Fatalf("missing param %q in %#v", name, params)
	}
	return p[field]
}
