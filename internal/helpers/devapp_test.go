package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	stderrors "errors"
	"reflect"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
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
	root.PersistentFlags().String("format", "json", "format")
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
			args:     []string{"robot", "config", "--unified-app-id", "u-1", "--name", "小助手", "--brief", "审批助手", "--mode", "STREAM", "--skills", "qa,approval", "--add-scope", "--yes"},
			wantTool: "set_extension_robot_config",
			wantParams: map[string]any{
				"unifiedAppId": "u-1",
				"name":         "小助手",
				"brief":        "审批助手",
				"mode":         "STREAM",
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

func TestCrossPlatformCoverageDevAppGetBuildsDetailParams(t *testing.T) {
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

func TestCrossPlatformCoverageDevAppGetBuildsDetailParamsByAppKey(t *testing.T) {
	runner := &captureRunner{}
	root := newDevAppCommand(runner)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"get", "--app-key", "dingxxx"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	if got := runner.last.Tool; got != "get_dev_app" {
		t.Fatalf("Tool = %q, want get_dev_app", got)
	}
	want := map[string]any{"appKey": "dingxxx"}
	if !reflect.DeepEqual(runner.last.Params, want) {
		t.Fatalf("Params = %#v, want %#v", runner.last.Params, want)
	}
}

func TestCrossPlatformCoverageDevAppGetPrefersUnifiedAppIDWhenBothPresent(t *testing.T) {
	runner := &captureRunner{}
	root := newDevAppCommand(runner)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"get", "--unified-app-id", "u-1", "--app-key", "dingxxx"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	want := map[string]any{"unifiedAppId": "u-1", "appKey": "dingxxx"}
	if !reflect.DeepEqual(runner.last.Params, want) {
		t.Fatalf("Params = %#v, want %#v", runner.last.Params, want)
	}
}

func TestCrossPlatformCoverageDevAppGetRequiresLocator(t *testing.T) {
	runner := &captureRunner{}
	root := newDevAppCommand(runner)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"get"})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "--unified-app-id 或 --app-key") {
		t.Fatalf("error = %v, want locator validation", err)
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
func TestDevAppEventListForwardsKeyword(t *testing.T) {
	runner := &captureRunner{}
	root := newDevAppTestRoot(runner)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"dev", "app", "event", "list", "--unified-app-id", "u-1", "--keyword", "通讯录"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	if runner.last.Tool != devAppEventListTool {
		t.Fatalf("Tool = %q, want %q", runner.last.Tool, devAppEventListTool)
	}
	if got := runner.last.Params["keyword"]; got != "通讯录" {
		t.Fatalf("keyword = %#v, want 通讯录", got)
	}
}

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

func TestEveryDevAppWriteCommandRequiresGuard(t *testing.T) {
	paths := [][]string{
		{"event", "subscribe"},
		{"event", "unsubscribe"},
		{"create"},
		{"update"},
		{"delete"},
		{"disable"},
		{"enable"},
		{"webapp", "config"},
		{"permission", "add"},
		{"permission", "remove"},
		{"member", "add"},
		{"member", "remove"},
		{"security", "config"},
		{"robot", "submit"},
		{"robot", "config"},
		{"robot", "enable"},
		{"robot", "disable"},
		{"version", "create"},
		{"version", "publish"},
	}

	for _, path := range paths {
		path := path
		t.Run(strings.Join(path, "/"), func(t *testing.T) {
			runner := &captureRunner{}
			root := newDevAppTestRoot(runner)
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(append([]string{"dev", "app"}, path...))

			err := root.Execute()
			if err == nil {
				t.Fatal("Execute() error = nil, want write guard")
			}
			var appErr *apperrors.Error
			if !stderrors.As(err, &appErr) {
				t.Fatalf("error type = %T, want *errors.Error", err)
			}
			if appErr.Reason != "confirmation_required" {
				t.Fatalf("error reason = %q, want confirmation_required", appErr.Reason)
			}
			for _, marker := range []string{"写操作", "--dry-run", "--yes"} {
				if !strings.Contains(err.Error(), marker) {
					t.Fatalf("error = %q, want %q write-guard marker", err.Error(), marker)
				}
			}
			if runner.last.Tool != "" {
				t.Fatalf("tool = %q, want no invocation before confirmation", runner.last.Tool)
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

func TestDevAppVersionCheckApprovalPreservesApprovalCandidateNames(t *testing.T) {
	runner := &devAppResponseRunner{
		response: map[string]any{
			"content": map[string]any{
				"success":   true,
				"errorCode": nil,
				"errorMsg":  nil,
				"result": map[string]any{
					"requiresApproval": true,
					"publishable":      false,
					"approvalMode":     "SELECT_APPROVER",
					"approvalCandidates": []any{
						map[string]any{"userId": "034766", "name": "张三", "mainAdmin": true},
						map[string]any{"userId": "084896", "name": "李四", "mainAdmin": false},
					},
				},
			},
		},
	}
	root := newDevAppTestRoot(runner)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{
		"dev", "app", "version", "check-approval",
		"--unified-app-id", "u-1",
		"--version-id", "v-1",
		"--format", "json",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	if got := runner.last.Tool; got != devAppVersionPublishTool {
		t.Fatalf("Tool = %q, want %q", got, devAppVersionPublishTool)
	}
	if precheckOnly, _ := runner.last.Params["precheckOnly"].(bool); !precheckOnly {
		t.Fatalf("precheckOnly = %#v, want true", runner.last.Params["precheckOnly"])
	}
	var rendered map[string]any
	if err := json.Unmarshal(out.Bytes(), &rendered); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\noutput:\n%s", err, out.String())
	}
	candidates, ok := rendered["approvalCandidates"].([]any)
	if !ok || len(candidates) != 2 {
		t.Fatalf("approvalCandidates = %#v, want two candidates", rendered["approvalCandidates"])
	}
	first, ok := candidates[0].(map[string]any)
	if !ok {
		t.Fatalf("approvalCandidates[0] = %#v, want object", candidates[0])
	}
	if first["userId"] != "034766" || first["name"] != "张三" || first["mainAdmin"] != true {
		t.Fatalf("approvalCandidates[0] = %#v, want userId/name/mainAdmin preserved", first)
	}
	options := rendered["approvalOptions"].([]any)
	if len(options) != 2 {
		t.Fatalf("approvalOptions = %#v, want two options", rendered["approvalOptions"])
	}
	firstOption := devAppRenderedMap(t, options[0])
	if firstOption["label"] != "张三（userId: 034766）（主管理员）" || firstOption["name"] != "张三" || firstOption["userId"] != "034766" {
		t.Fatalf("approvalOptions[0] = %#v, want label with name/userId/mainAdmin", firstOption)
	}
	if rendered["completionState"] != "WAITING_FOR_APPROVER_SELECTION" ||
		rendered["actionRequired"] != "select_approver" ||
		rendered["mustAskUser"] != true ||
		rendered["requiresUserInput"] != true ||
		rendered["terminal"] != false {
		t.Fatalf("selection gate = %#v, want waiting for approver selection", rendered)
	}
	promptText, _ := rendered["approvalPromptText"].(string)
	if !strings.Contains(promptText, "张三（userId: 034766）") ||
		!strings.Contains(promptText, "李四（userId: 084896）") ||
		!strings.Contains(promptText, "A. ") || !strings.Contains(promptText, "B. ") {
		t.Fatalf("approvalPromptText = %q, want numbered readable names", promptText)
	}
	steps := devAppRenderedSteps(t, rendered)
	selectStep := devAppStepByID(t, steps, "select_approver")
	if selectStep["blocking"] != true || selectStep["requiresUserInput"] != true {
		t.Fatalf("select step = %#v, want blocking user input step", selectStep)
	}
	publishStep := devAppStepByID(t, steps, "publish_version")
	if command, _ := publishStep["command"].(string); !strings.Contains(command, "--approver-user-id <selectedUserId>") {
		t.Fatalf("publish command = %q, want selected approver placeholder", command)
	}
}

func TestDevAppRobotResultApprovalRequiredWithoutUnifiedAppIDBlocksForUserInput(t *testing.T) {
	rendered := runDevAppRobotResultOutput(t, map[string]any{
		"status":       "APPROVAL_REQUIRED",
		"taskId":       "t-approval",
		"clientId":     "dingrobot123",
		"clientSecret": "secret-client",
		"robotCode":    "dingrobot123",
	})

	if rendered["completionState"] != "BLOCKED_BY_MISSING_UNIFIED_APP_ID" ||
		rendered["mustContinue"] != true ||
		rendered["mustAskUser"] != true ||
		rendered["actionRequired"] != "provide_unified_app_id" ||
		rendered["terminal"] != false {
		t.Fatalf("top-level completion gate = %#v, want missing unifiedAppId gate", rendered)
	}
	if message, _ := rendered["message"].(string); !strings.Contains(message, "不能用 clientId/appKey 反查后写版本") {
		t.Fatalf("message = %q, want app-key lookup prohibition", message)
	}

	lifecycle := devAppRenderedMap(t, rendered["lifecycle"])
	if lifecycle["publicUseReady"] != false || lifecycle["requiresVersionPublish"] != true {
		t.Fatalf("lifecycle = %#v, want publicUseReady=false and requiresVersionPublish=true", lifecycle)
	}
	if lifecycle["overallComplete"] != false || lifecycle["completionGate"] != "provide_unified_app_id" {
		t.Fatalf("lifecycle = %#v, want overallComplete=false and completionGate=provide_unified_app_id", lifecycle)
	}
	if lifecycle["localOnlyReady"] != true {
		t.Fatalf("localOnlyReady = %#v, want true", lifecycle["localOnlyReady"])
	}
	if lifecycle["localConnectReady"] != true {
		t.Fatalf("localConnectReady = %#v, want true", lifecycle["localConnectReady"])
	}
	wantBlockingIDs := []string{"provide_unified_app_id"}
	if got := devAppStringSliceFromRendered(t, lifecycle["blockingStepIds"]); !reflect.DeepEqual(got, wantBlockingIDs) {
		t.Fatalf("blockingStepIds = %#v, want %#v", got, wantBlockingIDs)
	}

	steps := devAppRenderedSteps(t, rendered)
	first := devAppRenderedMap(t, steps[0])
	if first["id"] != "provide_unified_app_id" {
		t.Fatalf("first step id = %#v, want provide_unified_app_id", first["id"])
	}
	if first["blocking"] != true || first["requiresUserInput"] != true {
		t.Fatalf("provide step = %#v, want blocking user input", first)
	}
	if doneWhen, _ := first["doneWhen"].(string); !strings.Contains(doneWhen, "不能用 clientId/appKey") {
		t.Fatalf("provide doneWhen = %q, want app-key prohibition", doneWhen)
	}
	assertDevAppStepIDAbsent(t, steps, "resolve_unified_app")
	assertDevAppStepIDAbsent(t, steps, "create_version")
	assertDevAppStepIDAbsent(t, steps, "check_approval")
	assertDevAppStepIDAbsent(t, steps, "publish_version")
	assertDevAppStepIDAbsent(t, steps, "wait_release")
	assertDevAppStepCommandsDoNotContain(t, steps, "dws dev app list --app-key")
	assertDevAppStepCommandsDoNotContain(t, steps, "dws dev app version create")
	assertDevAppStepCommandsDoNotContain(t, steps, "dws dev app version publish")
	connect := devAppStepByID(t, steps, "connect_local")
	if connect["blocking"] != false || connect["optional"] != true || connect["scope"] != "local_debug_only" {
		t.Fatalf("connect step = %#v, want optional local debug non-blocking step", connect)
	}
	assertDevAppStepCommandsDoNotContain(t, steps, "secret-client")
}

func TestDevAppRobotResultApprovalRequiredWithUnifiedAppIDAddsPublishAndConnectSteps(t *testing.T) {
	rendered := runDevAppRobotResultOutput(t, map[string]any{
		"status":       "APPROVAL_REQUIRED",
		"taskId":       "t-approval",
		"unifiedAppId": "u-approval",
		"clientId":     "dingrobot123",
		"clientSecret": "secret-client",
		"robotCode":    "dingrobot123",
	})

	if rendered["completionState"] != "BLOCKED_BY_VERSION_PUBLISH" ||
		rendered["mustContinue"] != true ||
		rendered["actionRequired"] != "submit_version_publish" ||
		rendered["terminal"] != false {
		t.Fatalf("top-level completion gate = %#v, want blocked version publish state", rendered)
	}
	if message, _ := rendered["message"].(string); !strings.Contains(message, "必须继续执行 blocking nextSteps") {
		t.Fatalf("message = %q, want blocking nextSteps guidance", message)
	}

	lifecycle := devAppRenderedMap(t, rendered["lifecycle"])
	if lifecycle["overallComplete"] != false || lifecycle["completionGate"] != "version_publish" {
		t.Fatalf("lifecycle = %#v, want overallComplete=false and completionGate=version_publish", lifecycle)
	}
	wantBlockingIDs := []string{"create_version", "check_approval", "publish_version", "wait_release"}
	if got := devAppStringSliceFromRendered(t, lifecycle["blockingStepIds"]); !reflect.DeepEqual(got, wantBlockingIDs) {
		t.Fatalf("blockingStepIds = %#v, want %#v", got, wantBlockingIDs)
	}

	steps := devAppRenderedSteps(t, rendered)
	if first := devAppRenderedMap(t, steps[0]); first["id"] != "create_version" {
		t.Fatalf("first step = %#v, want create_version before connect_local", first)
	}
	create := devAppStepByID(t, steps, "create_version")
	if create["blocking"] != true {
		t.Fatalf("create blocking = %#v, want true", create["blocking"])
	}
	if command, _ := create["command"].(string); !strings.Contains(command, "--unified-app-id u-approval") {
		t.Fatalf("create command = %q, want concrete unifiedAppId", command)
	}
	if dryRun, _ := create["dryRunCommand"].(string); !strings.Contains(dryRun, "--dry-run") {
		t.Fatalf("create dryRunCommand = %q, want --dry-run", dryRun)
	}
	publish := devAppStepByID(t, steps, "publish_version")
	if publish["requiresUserInput"] != true {
		t.Fatalf("publish requiresUserInput = %#v, want true", publish["requiresUserInput"])
	}
	if publish["blocking"] != true {
		t.Fatalf("publish blocking = %#v, want true", publish["blocking"])
	}
	if doneWhen, _ := publish["doneWhen"].(string); !strings.Contains(doneWhen, "approvalSubmitted=true") || !strings.Contains(doneWhen, "UNDER_REVIEW") {
		t.Fatalf("publish doneWhen = %q, want published/submitted approval gate", doneWhen)
	}
	connect := devAppStepByID(t, steps, "connect_local")
	if connect["blocking"] != false || connect["optional"] != true || connect["scope"] != "local_debug_only" {
		t.Fatalf("connect step = %#v, want optional local debug non-blocking step", connect)
	}
	sensitive := connect["sensitiveFields"].([]any)
	if len(sensitive) != 1 || sensitive[0] != "clientSecret" {
		t.Fatalf("sensitiveFields = %#v, want clientSecret", sensitive)
	}
	assertDevAppStepCommandsDoNotContain(t, steps, "secret-client")
}

func TestDevAppRobotResultWaitingAddsPollStep(t *testing.T) {
	rendered := runDevAppRobotResultOutput(t, map[string]any{
		"status":          "WAITING",
		"taskId":          "t-wait",
		"intervalSeconds": 5,
	})

	lifecycle := devAppRenderedMap(t, rendered["lifecycle"])
	if lifecycle["phase"] != "creating" || lifecycle["robotTaskDone"] != false {
		t.Fatalf("lifecycle = %#v, want creating and not done", lifecycle)
	}
	steps := devAppRenderedSteps(t, rendered)
	poll := devAppStepByID(t, steps, "poll_robot_result")
	if command, _ := poll["command"].(string); command != "dws dev app robot result --task-id t-wait --format json" {
		t.Fatalf("poll command = %q, want task polling command", command)
	}
}

func TestDevAppRobotResultSuccessAddsPublishAndConnectSteps(t *testing.T) {
	rendered := runDevAppRobotResultOutput(t, map[string]any{
		"status":       "SUCCESS",
		"taskId":       "t-ok",
		"unifiedAppId": "u-1",
		"clientId":     "dingrobot123",
		"clientSecret": "secret-client",
	})

	lifecycle := devAppRenderedMap(t, rendered["lifecycle"])
	if lifecycle["localConnectReady"] != true || lifecycle["requiresVersionPublish"] != true || lifecycle["publicUseReady"] != false {
		t.Fatalf("lifecycle = %#v, want local ready, version required, public not ready", lifecycle)
	}
	if lifecycle["overallComplete"] != false || lifecycle["completionGate"] != "version_publish" {
		t.Fatalf("lifecycle = %#v, want robot result not complete until version publish", lifecycle)
	}
	if rendered["completionState"] != "BLOCKED_BY_VERSION_PUBLISH" ||
		rendered["mustContinue"] != true ||
		rendered["actionRequired"] != "submit_version_publish" ||
		rendered["terminal"] != false {
		t.Fatalf("top-level completion gate = %#v, want blocked version publish state", rendered)
	}
	if message, _ := rendered["message"].(string); !strings.Contains(message, "线上发布/审批未完成") {
		t.Fatalf("message = %q, want publish/approval incomplete guidance", message)
	}
	wantBlockingIDs := []string{"create_version", "check_approval", "publish_version", "wait_release"}
	if got := devAppStringSliceFromRendered(t, lifecycle["blockingStepIds"]); !reflect.DeepEqual(got, wantBlockingIDs) {
		t.Fatalf("blockingStepIds = %#v, want %#v", got, wantBlockingIDs)
	}
	steps := devAppRenderedSteps(t, rendered)
	if first := devAppRenderedMap(t, steps[0]); first["id"] == "resolve_unified_app" {
		t.Fatalf("first step = %#v, did not expect resolve step when unifiedAppId is present", first)
	}
	create := devAppStepByID(t, steps, "create_version")
	if create["blocking"] != true {
		t.Fatalf("create blocking = %#v, want true", create["blocking"])
	}
	if command, _ := create["command"].(string); !strings.Contains(command, "--unified-app-id u-1") {
		t.Fatalf("create command = %q, want concrete unifiedAppId", command)
	}
	connect := devAppStepByID(t, steps, "connect_local")
	if connect["blocking"] != false || connect["optional"] != true || connect["scope"] != "local_debug_only" {
		t.Fatalf("connect step = %#v, want optional local debug non-blocking step", connect)
	}
	if command, _ := connect["command"].(string); !strings.Contains(command, "--unified-app-id u-1") {
		t.Fatalf("connect command = %q, want --unified-app-id form (safe: clientSecret not on argv)", command)
	}
	if command, _ := connect["command"].(string); strings.Contains(command, "--robot-client-secret") {
		t.Fatalf("connect command = %q, must not put clientSecret on argv", command)
	}
	assertDevAppStepCommandsDoNotContain(t, steps, "secret-client")
}

func TestDevAppRobotResultSuccessWithoutUnifiedAppIDBlocksForUserInput(t *testing.T) {
	rendered := runDevAppRobotResultOutput(t, map[string]any{
		"status":       "SUCCESS",
		"taskId":       "t-ok",
		"clientId":     "dingrobot123",
		"clientSecret": "secret-client",
	})

	if rendered["completionState"] != "BLOCKED_BY_MISSING_UNIFIED_APP_ID" ||
		rendered["mustAskUser"] != true ||
		rendered["actionRequired"] != "provide_unified_app_id" ||
		rendered["terminal"] != false {
		t.Fatalf("top-level completion gate = %#v, want missing unifiedAppId gate", rendered)
	}
	lifecycle := devAppRenderedMap(t, rendered["lifecycle"])
	if lifecycle["overallComplete"] != false || lifecycle["completionGate"] != "provide_unified_app_id" {
		t.Fatalf("lifecycle = %#v, want robot result blocked on unifiedAppId", lifecycle)
	}
	wantBlockingIDs := []string{"provide_unified_app_id"}
	if got := devAppStringSliceFromRendered(t, lifecycle["blockingStepIds"]); !reflect.DeepEqual(got, wantBlockingIDs) {
		t.Fatalf("blockingStepIds = %#v, want %#v", got, wantBlockingIDs)
	}
	steps := devAppRenderedSteps(t, rendered)
	first := devAppRenderedMap(t, steps[0])
	if first["id"] != "provide_unified_app_id" || first["blocking"] != true {
		t.Fatalf("first step = %#v, want blocking provide_unified_app_id", first)
	}
	assertDevAppStepIDAbsent(t, steps, "resolve_unified_app")
	assertDevAppStepIDAbsent(t, steps, "create_version")
	assertDevAppStepIDAbsent(t, steps, "publish_version")
	assertDevAppStepCommandsDoNotContain(t, steps, "dws dev app list --app-key")
	assertDevAppStepCommandsDoNotContain(t, steps, "dws dev app version create")
	connect := devAppStepByID(t, steps, "connect_local")
	if connect["blocking"] != false || connect["optional"] != true || connect["scope"] != "local_debug_only" {
		t.Fatalf("connect step = %#v, want optional local debug non-blocking step", connect)
	}
	assertDevAppStepCommandsDoNotContain(t, steps, "secret-client")
}

func TestDevAppRobotResultFailAndExpiredAddRetrySteps(t *testing.T) {
	cases := []struct {
		name           string
		status         string
		wantTaskIDFlag bool
	}{
		{name: "fail reuses task id", status: "FAIL", wantTaskIDFlag: true},
		{name: "expired resubmits without task id", status: "EXPIRED", wantTaskIDFlag: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rendered := runDevAppRobotResultOutput(t, map[string]any{
				"status": tc.status,
				"taskId": "t-retry",
			})
			steps := devAppRenderedSteps(t, rendered)
			retry := devAppStepByID(t, steps, "retry_robot_submit")
			command, _ := retry["command"].(string)
			hasTaskID := strings.Contains(command, "--task-id t-retry")
			if hasTaskID != tc.wantTaskIDFlag {
				t.Fatalf("retry command = %q, has task id %v, want %v", command, hasTaskID, tc.wantTaskIDFlag)
			}
			if dryRun, _ := retry["dryRunCommand"].(string); !strings.Contains(dryRun, "--dry-run") {
				t.Fatalf("retry dryRunCommand = %q, want --dry-run", dryRun)
			}
		})
	}
}

func runDevAppRobotResultOutput(t *testing.T, result map[string]any) map[string]any {
	t.Helper()
	runner := &devAppResponseRunner{
		response: map[string]any{
			"content": map[string]any{
				"success":   true,
				"errorCode": nil,
				"errorMsg":  nil,
				"result":    result,
			},
		},
	}
	root := newDevAppTestRoot(runner)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"dev", "app", "robot", "result", "--task-id", "t-1", "--format", "json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	var rendered map[string]any
	if err := json.Unmarshal(out.Bytes(), &rendered); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\noutput:\n%s", err, out.String())
	}
	return rendered
}

func devAppRenderedSteps(t *testing.T, rendered map[string]any) []any {
	t.Helper()
	steps, ok := rendered["nextSteps"].([]any)
	if !ok || len(steps) == 0 {
		t.Fatalf("nextSteps = %#v, want non-empty array", rendered["nextSteps"])
	}
	return steps
}

func devAppStepByID(t *testing.T, steps []any, id string) map[string]any {
	t.Helper()
	for _, raw := range steps {
		step := devAppRenderedMap(t, raw)
		if step["id"] == id {
			return step
		}
	}
	t.Fatalf("step %q not found in %#v", id, steps)
	return nil
}

func assertDevAppStepIDAbsent(t *testing.T, steps []any, id string) {
	t.Helper()
	for _, raw := range steps {
		step := devAppRenderedMap(t, raw)
		if step["id"] == id {
			t.Fatalf("step %q unexpectedly present in %#v", id, steps)
		}
	}
}

func devAppRenderedMap(t *testing.T, raw any) map[string]any {
	t.Helper()
	m, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("value = %#v, want object", raw)
	}
	return m
}

func devAppStringSliceFromRendered(t *testing.T, raw any) []string {
	t.Helper()
	values, ok := raw.([]any)
	if !ok {
		t.Fatalf("value = %#v, want array", raw)
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			t.Fatalf("array value = %#v, want string", value)
		}
		out = append(out, text)
	}
	return out
}

func assertDevAppStepCommandsDoNotContain(t *testing.T, steps []any, secret string) {
	t.Helper()
	for _, raw := range steps {
		step := devAppRenderedMap(t, raw)
		for _, key := range []string{"command", "dryRunCommand"} {
			command, _ := step[key].(string)
			if strings.Contains(command, secret) {
				t.Fatalf("%s for step %s leaked secret in %q", key, step["id"], command)
			}
		}
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
