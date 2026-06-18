package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

func newDevAppTestRoot(runner executor.Runner) *cobra.Command {
	root := &cobra.Command{
		Use:               "dws",
		DisableAutoGenTag: true,
	}
	root.PersistentFlags().Bool("dry-run", false, "dry run")
	root.PersistentFlags().Bool("yes", false, "yes")
	root.AddCommand(devHandler{}.Command(runner))
	return root
}

type devAppResponseRunner struct {
	last     executor.Invocation
	response map[string]any
}

func (r *devAppResponseRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.last = invocation
	invocation.Implemented = true
	return executor.Result{Invocation: invocation, Response: r.response}, nil
}

func TestDevAppMemberCommandsBuildToolParams(t *testing.T) {
	cases := []struct {
		name       string
		cmd        string
		args       []string
		wantTool   string
		wantParams map[string]any
	}{
		{
			name:     "list",
			cmd:      "list",
			args:     []string{"--unified-app-id", "app-001"},
			wantTool: "list_dev_app_members",
			wantParams: map[string]any{
				"unifiedAppId": "app-001",
			},
		},
		{
			name:     "add multiple users",
			cmd:      "add",
			args:     []string{"--unified-app-id", "app-001", "--user-ids", "userId1,userId2,userId3,userId4", "--member-type", "DEVELOPER", "--yes"},
			wantTool: "add_dev_app_members",
			wantParams: map[string]any{
				"unifiedAppId": "app-001",
				"userIds":      []string{"userId1", "userId2", "userId3", "userId4"},
				"memberType":   "DEVELOPER",
			},
		},
		{
			name:     "remove trims users",
			cmd:      "remove",
			args:     []string{"--unified-app-id", "app-001", "--user-ids", " userId1 , userId2 ", "--member-type", "DEVELOPER", "--yes"},
			wantTool: "remove_dev_app_members",
			wantParams: map[string]any{
				"unifiedAppId": "app-001",
				"userIds":      []string{"userId1", "userId2"},
				"memberType":   "DEVELOPER",
			},
		},
		{
			name:     "add accepts legacy member user ids flag",
			cmd:      "add",
			args:     []string{"--unified-app-id", "app-001", "--member-user-ids", "userId1,userId2", "--member-type", "DEVELOPER", "--yes"},
			wantTool: "add_dev_app_members",
			wantParams: map[string]any{
				"unifiedAppId": "app-001",
				"userIds":      []string{"userId1", "userId2"},
				"memberType":   "DEVELOPER",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &captureRunner{}
			root := newDevAppTestRoot(runner)
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(append([]string{"dev", "app", "member", tc.cmd}, tc.args...))

			if err := root.Execute(); err != nil {
				t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
			}

			if got := runner.last.CanonicalProduct; got != "devapp" {
				t.Fatalf("CanonicalProduct = %q, want devapp", got)
			}
			if got := runner.last.Tool; got != tc.wantTool {
				t.Fatalf("Tool = %q, want %q", got, tc.wantTool)
			}
			if !reflect.DeepEqual(runner.last.Params, tc.wantParams) {
				t.Fatalf("Params = %#v, want %#v", runner.last.Params, tc.wantParams)
			}
		})
	}
}

func TestDevCommandTree(t *testing.T) {
	dev := devHandler{}.Command(&captureRunner{})
	if dev.Name() != "dev" {
		t.Fatalf("Name() = %q, want dev", dev.Name())
	}
	if len(dev.Aliases) != 0 {
		t.Fatalf("Aliases = %v, want none (clean switch from devapp)", dev.Aliases)
	}
	// 三支柱：app / connect / doc
	for _, name := range []string{"app", "connect", "doc"} {
		if _, _, err := dev.Find([]string{name}); err != nil {
			t.Fatalf("missing subtree %q: %v", name, err)
		}
	}
	app, _, err := dev.Find([]string{"app"})
	if err != nil {
		t.Fatalf("find app: %v", err)
	}
	for _, name := range []string{"list", "get", "create", "update", "delete", "disable", "enable", "credentials", "webapp", "permission", "member", "security", "robot", "version", "event"} {
		if _, _, err := app.Find([]string{name}); err != nil {
			t.Fatalf("missing app command %q: %v", name, err)
		}
	}
	// connect 已从 robot 子树提升为 dev connect，robot 下不应再有
	robot, _, err := app.Find([]string{"robot"})
	if err != nil {
		t.Fatalf("find robot: %v", err)
	}
	for _, sub := range robot.Commands() {
		if sub.Name() == "connect" {
			t.Fatalf("robot subtree still has connect; it moved to dev connect")
		}
	}
}

func TestDevAppRobotCommandsBuildToolParams(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantTool   string
		wantParams map[string]any
	}{
		{
			name:     "submit async fills media placeholders",
			args:     []string{"robot", "submit", "--name", "智能体", "--robot-name", "小助手", "--desc", "审批问答", "--task-id", "t-1", "--yes"},
			wantTool: "submit_robot_create_task",
			wantParams: map[string]any{
				"name":           "智能体",
				"robotName":      "小助手",
				"desc":           "审批问答",
				"iconMediaId":    "",
				"previewMediaId": "",
				"taskId":         "t-1",
			},
		},
		{
			name:       "result",
			args:       []string{"robot", "result", "--task-id", "t-1"},
			wantTool:   "query_robot_create_result",
			wantParams: map[string]any{"taskId": "t-1"},
		},
		{
			name:       "get config",
			args:       []string{"robot", "get", "--unified-app-id", "u-1"},
			wantTool:   "get_extension_robot_config",
			wantParams: map[string]any{"unifiedAppId": "u-1"},
		},
		{
			name:     "config create with skills and mode",
			args:     []string{"robot", "config", "--unified-app-id", "u-1", "--name", "小助手", "--brief", "审批助手", "--mode", "2", "--skills", "qa,approval", "--add-scope", "--yes"},
			wantTool: "set_extension_robot_config",
			wantParams: map[string]any{
				"unifiedAppId": "u-1",
				"name":         "小助手",
				"brief":        "审批助手",
				"mode":         2,
				"skills":       []string{"qa", "approval"},
				"addScope":     true,
			},
		},
		{
			name:       "robot disable",
			args:       []string{"robot", "disable", "--unified-app-id", "u-1", "--yes"},
			wantTool:   "disable_dev_app_robot",
			wantParams: map[string]any{"unifiedAppId": "u-1"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &captureRunner{}
			root := newDevAppTestRoot(runner)
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(append([]string{"dev", "app"}, tc.args...))

			if err := root.Execute(); err != nil {
				t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
			}
			if got := runner.last.CanonicalProduct; got != "devapp" {
				t.Fatalf("CanonicalProduct = %q, want devapp", got)
			}
			if got := runner.last.Tool; got != tc.wantTool {
				t.Fatalf("Tool = %q, want %q", got, tc.wantTool)
			}
			if !reflect.DeepEqual(runner.last.Params, tc.wantParams) {
				t.Fatalf("Params = %#v, want %#v", runner.last.Params, tc.wantParams)
			}
		})
	}
}

func TestDevAppVersionCommandsBuildToolParams(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantTool   string
		wantParams map[string]any
	}{
		{
			name:     "create lets server auto-increment version",
			args:     []string{"version", "create", "--unified-app-id", "u-1", "--desc", "新增机器人", "--yes"},
			wantTool: "create_dev_app_version",
			wantParams: map[string]any{
				"unifiedAppId": "u-1",
				"desc":         "新增机器人",
			},
		},
		{
			name:     "create with explicit version",
			args:     []string{"version", "create", "--unified-app-id", "u-1", "--version", "1.0.1", "--desc", "新增机器人", "--yes"},
			wantTool: "create_dev_app_version",
			wantParams: map[string]any{
				"unifiedAppId": "u-1",
				"version":      "1.0.1",
				"desc":         "新增机器人",
			},
		},
		{
			name:     "list",
			args:     []string{"version", "list", "--unified-app-id", "u-1", "--cursor", "tok-1", "--page-size", "5"},
			wantTool: "list_dev_app_versions",
			wantParams: map[string]any{
				"unifiedAppId": "u-1",
				"cursor":       "tok-1",
				"pageSize":     5,
			},
		},
		{
			name:       "get detail",
			args:       []string{"version", "get", "--unified-app-id", "u-1", "--version-id", "v-1"},
			wantTool:   "get_dev_app_version_detail",
			wantParams: map[string]any{"unifiedAppId": "u-1", "versionId": "v-1"},
		},
		{
			name:       "check-approval forces precheckOnly",
			args:       []string{"version", "check-approval", "--unified-app-id", "u-1", "--version-id", "v-1"},
			wantTool:   "publish_dev_app_version",
			wantParams: map[string]any{"unifiedAppId": "u-1", "versionId": "v-1", "precheckOnly": true},
		},
		{
			name:     "publish sets precheckOnly false and sensitive",
			args:     []string{"version", "publish", "--unified-app-id", "u-1", "--version-id", "v-1", "--confirmed-sensitive", "--approver-user-id", "user-1", "--yes"},
			wantTool: "publish_dev_app_version",
			wantParams: map[string]any{
				"unifiedAppId":       "u-1",
				"versionId":          "v-1",
				"precheckOnly":       false,
				"confirmedSensitive": true,
				"approverUserId":     "user-1",
			},
		},
		{
			name:       "status",
			args:       []string{"version", "status", "--unified-app-id", "u-1", "--version-id", "v-1"},
			wantTool:   "get_dev_app_version_status",
			wantParams: map[string]any{"unifiedAppId": "u-1", "versionId": "v-1"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &captureRunner{}
			root := newDevAppTestRoot(runner)
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(append([]string{"dev", "app"}, tc.args...))

			if err := root.Execute(); err != nil {
				t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
			}
			if got := runner.last.Tool; got != tc.wantTool {
				t.Fatalf("Tool = %q, want %q", got, tc.wantTool)
			}
			if !reflect.DeepEqual(runner.last.Params, tc.wantParams) {
				t.Fatalf("Params = %#v, want %#v", runner.last.Params, tc.wantParams)
			}
		})
	}
}

func TestDevAppRobotAndVersionWritesRequireGuard(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "robot config", args: []string{"dev", "app", "robot", "config", "--unified-app-id", "u-1", "--name", "小助手"}},
		{name: "robot disable", args: []string{"dev", "app", "robot", "disable", "--unified-app-id", "u-1"}},
		{name: "version create", args: []string{"dev", "app", "version", "create", "--unified-app-id", "u-1"}},
		{name: "version publish", args: []string{"dev", "app", "version", "publish", "--unified-app-id", "u-1", "--version-id", "v-1"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &captureRunner{}
			root := newDevAppTestRoot(runner)
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(tc.args)

			err := root.Execute()
			if err == nil {
				t.Fatal("Execute() error = nil, want write guard")
			}
			if !strings.Contains(err.Error(), "写操作") {
				t.Fatalf("error = %q, want write guard", err.Error())
			}
			if runner.last.Tool != "" {
				t.Fatalf("tool = %q, want no invocation", runner.last.Tool)
			}
		})
	}
}

func TestDevAppListBuildsListByConditionParams(t *testing.T) {
	runner := &captureRunner{}
	root := newDevAppCommand(runner)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"list", "--name", "Waker", "--cursor", "tok-2", "--page-size", "5", "--sort-type", "gmt_modified", "--sort-order", "desc"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	if got := runner.last.Tool; got != "list_dev_app" {
		t.Fatalf("Tool = %q, want list_dev_app", got)
	}
	want := map[string]any{
		"cursor":    "tok-2",
		"pageSize":  5,
		"name":      "Waker",
		"sortType":  "gmt_modified",
		"sortOrder": "desc",
	}
	if !reflect.DeepEqual(runner.last.Params, want) {
		t.Fatalf("Params = %#v, want %#v", runner.last.Params, want)
	}
}

func TestDevAppGetBuildsDetailParams(t *testing.T) {
	runner := &captureRunner{}
	root := newDevAppCommand(runner)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"get", "--unified-app-id", "u-1"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	if got := runner.last.Tool; got != "get_dev_app" {
		t.Fatalf("Tool = %q, want get_dev_app", got)
	}
	want := map[string]any{"unifiedAppId": "u-1"}
	if !reflect.DeepEqual(runner.last.Params, want) {
		t.Fatalf("Params = %#v, want %#v", runner.last.Params, want)
	}
}

func TestDevAppCreateUsesCurrentInnerToolAndWriteGuard(t *testing.T) {
	runner := &captureRunner{}
	root := newDevAppCommand(runner)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"create", "--name", "Demo"})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("error = %v, want write guard", err)
	}

	runner = &captureRunner{}
	root = newDevAppTestRoot(runner)
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"dev", "app", "create", "--name", "Demo", "--desc", "internal app", "--yes"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	if got := runner.last.Tool; got != "create_dev_app" {
		t.Fatalf("Tool = %q, want create_dev_app", got)
	}

	want := map[string]any{"name": "Demo", "desc": "internal app"}
	if !reflect.DeepEqual(runner.last.Params, want) {
		t.Fatalf("Params = %#v, want %#v", runner.last.Params, want)
	}
}

func TestDevAppUpdateUsesCurrentInnerTool(t *testing.T) {
	runner := &captureRunner{}
	root := newDevAppTestRoot(runner)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"dev", "app", "update", "--unified-app-id", "u-123", "--desc", "new desc", "--yes"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	if got := runner.last.Tool; got != "update_dev_app" {
		t.Fatalf("Tool = %q, want update_dev_app", got)
	}
	want := map[string]any{"unifiedAppId": "u-123", "desc": "new desc"}
	if !reflect.DeepEqual(runner.last.Params, want) {
		t.Fatalf("Params = %#v, want %#v", runner.last.Params, want)
	}
}

func TestDevAppLifecycleBuildsLocatorParams(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantTool   string
		wantParams map[string]any
	}{
		{
			name:       "inactive by unified app id",
			args:       []string{"disable", "--unified-app-id", "u-1", "--yes"},
			wantTool:   "disable_dev_app",
			wantParams: map[string]any{"unifiedAppId": "u-1"},
		},
		{
			name:       "active by unified id",
			args:       []string{"enable", "--unified-app-id", "u-2", "--yes"},
			wantTool:   "enable_dev_app",
			wantParams: map[string]any{"unifiedAppId": "u-2"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &captureRunner{}
			root := newDevAppTestRoot(runner)
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(append([]string{"dev", "app"}, tc.args...))

			if err := root.Execute(); err != nil {
				t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
			}
			if got := runner.last.Tool; got != tc.wantTool {
				t.Fatalf("Tool = %q, want %q", got, tc.wantTool)
			}
			if !reflect.DeepEqual(runner.last.Params, tc.wantParams) {
				t.Fatalf("Params = %#v, want %#v", runner.last.Params, tc.wantParams)
			}
		})
	}
}

// TestDevAppDeleteConfirmName covers the danger-tier delete: --confirm-name
// must match the located app's real name, and a name that can't be read is
// fail-closed (abort), never a silent proceed.
func TestDevAppDeleteConfirmName(t *testing.T) {
	t.Run("matching name proceeds", func(t *testing.T) {
		// get_dev_app 真实返回的应用名字段是 name（不是 appName）——
		// fixture 用 name 才能覆盖真实契约，避免 delete 取名回归。
		runner := &devAppResponseRunner{response: map[string]any{"name": "DemoApp"}}
		root := newDevAppTestRoot(runner)
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs([]string{"dev", "app", "delete", "--unified-app-id", "u-123", "--confirm-name", "DemoApp", "--yes"})
		if err := root.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if runner.last.Tool != "delete_dev_app" {
			t.Fatalf("Tool = %q, want delete_dev_app", runner.last.Tool)
		}
	})

	t.Run("mismatched name aborts", func(t *testing.T) {
		runner := &devAppResponseRunner{response: map[string]any{"name": "RealName"}}
		root := newDevAppTestRoot(runner)
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs([]string{"dev", "app", "delete", "--unified-app-id", "u-123", "--confirm-name", "WrongName", "--yes"})
		err := root.Execute()
		if err == nil || !strings.Contains(err.Error(), "名称不匹配") {
			t.Fatalf("error = %v, want 名称不匹配", err)
		}
		if runner.last.Tool == "delete_dev_app" {
			t.Fatal("delete must not run on name mismatch")
		}
	})

	t.Run("unreadable name is fail-closed", func(t *testing.T) {
		runner := &devAppResponseRunner{response: map[string]any{}} // no appName
		root := newDevAppTestRoot(runner)
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs([]string{"dev", "app", "delete", "--unified-app-id", "u-123", "--confirm-name", "DemoApp", "--yes"})
		err := root.Execute()
		if err == nil || !strings.Contains(err.Error(), "无法读取应用名") {
			t.Fatalf("error = %v, want 无法读取应用名 (fail-closed)", err)
		}
		if runner.last.Tool == "delete_dev_app" {
			t.Fatal("delete must not run when name is unreadable")
		}
	})
}

// TestDevAppEventSubscribeUsesEventCodes locks the param contract: the server's
// subscribe/unsubscribe tools (plural: subscribe_dev_app_events) read eventCodes
// as an ARRAY — real-device confirmed against the updated MCP schema.
func TestDevAppEventSubscribeUsesEventCodes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     []string
		wantTool string
	}{
		{"subscribe", []string{"dev", "app", "event", "subscribe", "--unified-app-id", "u-1", "--event-codes", "a,b", "--yes"}, "subscribe_dev_app_events"},
		{"unsubscribe", []string{"dev", "app", "event", "unsubscribe", "--unified-app-id", "u-1", "--event-codes", "a,b", "--yes"}, "unsubscribe_dev_app_events"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &captureRunner{}
			root := newDevAppTestRoot(runner)
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(tc.args)
			if err := root.Execute(); err != nil {
				t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
			}
			if runner.last.Tool != tc.wantTool {
				t.Fatalf("Tool = %q, want %q", runner.last.Tool, tc.wantTool)
			}
			got, ok := runner.last.Params["eventCodes"].([]string)
			if !ok || len(got) != 2 || got[0] != "a" || got[1] != "b" {
				t.Fatalf("eventCodes = %#v, want []string{a,b}", runner.last.Params["eventCodes"])
			}
			if _, bad := runner.last.Params["eventCode"]; bad {
				t.Fatal("must not send singular eventCode")
			}
		})
	}
}

func TestDevAppEventSubscribeRequiresEventCodes(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"subscribe", []string{"dev", "app", "event", "subscribe", "--unified-app-id", "u-1", "--yes"}},
		{"unsubscribe", []string{"dev", "app", "event", "unsubscribe", "--unified-app-id", "u-1", "--yes"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &captureRunner{}
			root := newDevAppTestRoot(runner)
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(tc.args)
			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), "--event-codes 为必填") {
				t.Fatalf("Execute() error = %v, want --event-codes 为必填", err)
			}
			if runner.last.Tool != "" {
				t.Fatalf("runner should not be called, got tool %q", runner.last.Tool)
			}
		})
	}
}

func TestDevAppWebappCommandsBuildParams(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantTool   string
		wantParams map[string]any
	}{
		{
			name:       "get",
			args:       []string{"webapp", "get", "--unified-app-id", "u-1"},
			wantTool:   "get_extension_webapp_config",
			wantParams: map[string]any{"unifiedAppId": "u-1"},
		},
		{
			name:     "config",
			args:     []string{"webapp", "config", "--unified-app-id", "u-1", "--homepage-url", "https://example.com", "--pc-homepage-url", "https://pc.example.com", "--yes"},
			wantTool: "set_extension_webapp_config",
			wantParams: map[string]any{
				"unifiedAppId":  "u-1",
				"homepageUrl":   "https://example.com",
				"pcHomepageUrl": "https://pc.example.com",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &captureRunner{}
			root := newDevAppTestRoot(runner)
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(append([]string{"dev", "app"}, tc.args...))

			if err := root.Execute(); err != nil {
				t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
			}
			if got := runner.last.Tool; got != tc.wantTool {
				t.Fatalf("Tool = %q, want %q", got, tc.wantTool)
			}
			if !reflect.DeepEqual(runner.last.Params, tc.wantParams) {
				t.Fatalf("Params = %#v, want %#v", runner.last.Params, tc.wantParams)
			}
		})
	}
}

func TestDevAppPermissionCommandsBuildParams(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantTool   string
		wantParams map[string]any
	}{
		{
			name:     "list",
			args:     []string{"permission", "list", "--unified-app-id", "u-123", "--keyword", "手机号", "--auth-status", "all", "--cursor", "tok-3", "--page-size", "5"},
			wantTool: "list_dev_app_permissions",
			wantParams: map[string]any{
				"unifiedAppId": "u-123",
				"keyword":      "手机号",
				"authStatus":   "ALL",
				"cursor":       "tok-3",
				"pageSize":     5,
			},
		},
		{
			name:     "add",
			args:     []string{"permission", "add", "--unified-app-id", "u-123", "--scope-values", "Contact.User.mobile,qyapi_robot_sendmsg", "--yes"},
			wantTool: "apply_dev_app_permissions",
			wantParams: map[string]any{
				"unifiedAppId": "u-123",
				"scopeValues":  []string{"Contact.User.mobile", "qyapi_robot_sendmsg"},
			},
		},
		{
			name:     "remove",
			args:     []string{"permission", "remove", "--unified-app-id", "u-123", "--scope-values", "Contact.User.mobile", "--yes"},
			wantTool: "remove_dev_app_permissions",
			wantParams: map[string]any{
				"unifiedAppId": "u-123",
				"scopeValues":  []string{"Contact.User.mobile"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &captureRunner{}
			root := newDevAppTestRoot(runner)
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(append([]string{"dev", "app"}, tc.args...))

			if err := root.Execute(); err != nil {
				t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
			}
			if got := runner.last.Tool; got != tc.wantTool {
				t.Fatalf("Tool = %q, want %q", got, tc.wantTool)
			}
			if !reflect.DeepEqual(runner.last.Params, tc.wantParams) {
				t.Fatalf("Params = %#v, want %#v", runner.last.Params, tc.wantParams)
			}
		})
	}
}

func TestDevAppCredentialsGetBuildsParams(t *testing.T) {
	runner := &captureRunner{}
	root := newDevAppTestRoot(runner)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"dev", "app", "credentials", "get", "--unified-app-id", "u-123"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	if got := runner.last.Tool; got != "get_dev_app_credentials" {
		t.Fatalf("Tool = %q, want get_dev_app_credentials", got)
	}
	want := map[string]any{"unifiedAppId": "u-123"}
	if !reflect.DeepEqual(runner.last.Params, want) {
		t.Fatalf("Params = %#v, want %#v", runner.last.Params, want)
	}
}

func TestDevAppCredentialsGetKeepsSecretFields(t *testing.T) {
	runner := &devAppResponseRunner{
		response: map[string]any{
			"content": map[string]any{
				"agentId":      123,
				"appKey":       "dingxxx",
				"appSecret":    "secret-app",
				"clientSecret": "secret-client",
			},
		},
	}
	root := newDevAppTestRoot(runner)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"dev", "app", "credentials", "get", "--unified-app-id", "u-123"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	rendered := out.String()
	for _, expected := range []string{"appSecret", "clientSecret", "secret-app", "secret-client"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("credentials output missing %q:\n%s", expected, rendered)
		}
	}
}

func TestDevAppMemberCommandsValidateRequiredFlags(t *testing.T) {
	cases := []struct {
		name    string
		cmd     string
		args    []string
		wantErr string
	}{
		{name: "list requires app", cmd: "list", args: nil, wantErr: "--unified-app-id 为必填"},
		{name: "add requires users", cmd: "add", args: []string{"--unified-app-id", "app-001", "--member-type", "DEVELOPER", "--dry-run"}, wantErr: "--user-ids 为必填"},
		{name: "add rejects empty users", cmd: "add", args: []string{"--unified-app-id", "app-001", "--user-ids", " , ", "--member-type", "DEVELOPER", "--dry-run"}, wantErr: "--user-ids 至少包含一个 userId"},
		{name: "remove requires member type", cmd: "remove", args: []string{"--unified-app-id", "app-001", "--user-ids", "userId1", "--dry-run"}, wantErr: "--member-type 为必填"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &captureRunner{}
			root := newDevAppTestRoot(runner)
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(append([]string{"dev", "app", "member", tc.cmd}, tc.args...))

			err := root.Execute()
			if err == nil {
				t.Fatalf("Execute() error = nil, want %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %q, want to contain %q", err.Error(), tc.wantErr)
			}
			if runner.last.Tool != "" {
				t.Fatalf("tool = %q, want no invocation", runner.last.Tool)
			}
		})
	}
}

func TestDevAppSecurityConfigBuildsOnlyProvidedLists(t *testing.T) {
	runner := &captureRunner{}
	root := newDevAppTestRoot(runner)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{
		"dev", "app", "security", "config",
		"--unified-app-id", "app-001",
		"--ip-whitelist", "192.0.2.10,192.0.2.11",
		"--redirect-urls", "https://callback.example.invalid/callback",
		"--sso-urls", "https://sso.example.invalid/sso",
		"--dry-run",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	if got := runner.last.CanonicalProduct; got != "devapp" {
		t.Fatalf("CanonicalProduct = %q, want devapp", got)
	}
	if got := runner.last.Tool; got != "update_dev_app_security_config" {
		t.Fatalf("Tool = %q, want update_dev_app_security_config", got)
	}
	want := map[string]any{
		"unifiedAppId": "app-001",
		"ipWhitelist":  []string{"192.0.2.10", "192.0.2.11"},
		"redirectUrls": []string{"https://callback.example.invalid/callback"},
		"ssoUrls":      []string{"https://sso.example.invalid/sso"},
	}
	if !reflect.DeepEqual(runner.last.Params, want) {
		t.Fatalf("Params = %#v, want %#v", runner.last.Params, want)
	}
}

func TestDevAppSecurityConfigOmitsAbsentOptionalLists(t *testing.T) {
	runner := &captureRunner{}
	root := newDevAppTestRoot(runner)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"dev", "app", "security", "config", "--unified-app-id", "app-001", "--redirect-urls", "https://callback.example.invalid/callback", "--dry-run"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	want := map[string]any{
		"unifiedAppId": "app-001",
		"redirectUrls": []string{"https://callback.example.invalid/callback"},
	}
	if !reflect.DeepEqual(runner.last.Params, want) {
		t.Fatalf("Params = %#v, want %#v", runner.last.Params, want)
	}
}

func TestDevAppSecurityConfigRequiresAtLeastOneConfig(t *testing.T) {
	runner := &captureRunner{}
	root := newDevAppTestRoot(runner)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"dev", "app", "security", "config", "--unified-app-id", "app-001", "--dry-run"})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), "至少提供一项安全配置：--ip-whitelist、--redirect-urls 或 --sso-urls") {
		t.Fatalf("error = %q", err.Error())
	}
	if runner.last.Tool != "" {
		t.Fatalf("tool = %q, want no invocation", runner.last.Tool)
	}
}

func TestDevAppMemberAndSecurityRequireWriteGuard(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{
			name: "member add",
			args: []string{"dev", "app", "member", "add", "--unified-app-id", "app-001", "--user-ids", "userId1", "--member-type", "DEVELOPER"},
		},
		{
			name: "member remove",
			args: []string{"dev", "app", "member", "remove", "--unified-app-id", "app-001", "--user-ids", "userId1", "--member-type", "DEVELOPER"},
		},
		{
			name: "security config",
			args: []string{"dev", "app", "security", "config", "--unified-app-id", "app-001", "--redirect-urls", "https://callback.example.invalid/callback"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &captureRunner{}
			root := newDevAppTestRoot(runner)
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(tc.args)

			err := root.Execute()
			if err == nil {
				t.Fatal("Execute() error = nil, want write guard")
			}
			if !strings.Contains(err.Error(), "写操作") {
				t.Fatalf("error = %q, want write guard", err.Error())
			}
			if runner.last.Tool != "" {
				t.Fatalf("tool = %q, want no invocation", runner.last.Tool)
			}
		})
	}
}

func TestDevAppUnwrapsSuccessfulServiceResult(t *testing.T) {
	runner := &devAppResponseRunner{
		response: map[string]any{
			"content": map[string]any{
				"success":   true,
				"errorCode": nil,
				"errorMsg":  nil,
				"result": map[string]any{
					"items": []any{
						map[string]any{"versionId": "v-1"},
					},
					"hasMore": false,
				},
			},
		},
	}
	root := newDevAppTestRoot(runner)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"dev", "app", "version", "list", "--unified-app-id", "u-1"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	var rendered map[string]any
	if err := json.Unmarshal(out.Bytes(), &rendered); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\noutput:\n%s", err, out.String())
	}
	if _, ok := rendered["success"]; ok {
		t.Fatalf("output kept ServiceResult wrapper: %#v", rendered)
	}
	items, ok := rendered["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("items = %#v, want one item", rendered["items"])
	}
}

func TestNormalizeDevAppRemovePermissionScopeValues(t *testing.T) {
	result := executor.Result{
		Response: map[string]any{
			"content": map[string]any{
				"removedScopeValues": []any{
					map[string]any{"scopeValue": "qyapi_robot_sendmsg", "scopeName": "机器人发消息"},
					map[string]any{"scopeValue": "Contact.User.mobile", "scopeName": "手机号"},
				},
			},
		},
	}

	normalized := normalizeDevAppToolResult(devAppPermissionRmTool, result)
	content := normalized.Response["content"].(map[string]any)
	got := content["removedScopeValues"]
	want := []string{"qyapi_robot_sendmsg", "Contact.User.mobile"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("removedScopeValues = %#v, want %#v", got, want)
	}
}

func TestNormalizeDevAppLifecycleResult(t *testing.T) {
	disable := normalizeDevAppToolResult(devAppDisableTool, executor.Result{
		Response: map[string]any{"content": map[string]any{"deleted": false}},
	})
	disableContent := disable.Response["content"].(map[string]any)
	if disabled, _ := disableContent["disabled"].(bool); !disabled {
		t.Fatalf("disabled = %#v, want true", disableContent["disabled"])
	}

	enable := normalizeDevAppToolResult(devAppEnableTool, executor.Result{
		Response: map[string]any{"content": map[string]any{"deleted": false}},
	})
	enableContent := enable.Response["content"].(map[string]any)
	if enabled, _ := enableContent["enabled"].(bool); !enabled {
		t.Fatalf("enabled = %#v, want true", enableContent["enabled"])
	}
}
