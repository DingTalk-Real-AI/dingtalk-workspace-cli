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
	root.AddCommand(newDevAppCommand(runner))
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
			args:     []string{"--app-id", "app-001"},
			wantTool: "list_dev_app_members",
			wantParams: map[string]any{
				"unifiedAppId": "app-001",
			},
		},
		{
			name:     "add multiple users",
			cmd:      "add",
			args:     []string{"--app-id", "app-001", "--users", "userId1,userId2,userId3,userId4", "--member-type", "DEVELOPER", "--yes"},
			wantTool: "add_dev_app_members",
			wantParams: map[string]any{
				"unifiedAppId":  "app-001",
				"memberUserIds": []string{"userId1", "userId2", "userId3", "userId4"},
				"memberType":    "DEVELOPER",
			},
		},
		{
			name:     "remove trims users",
			cmd:      "remove",
			args:     []string{"--app-id", "app-001", "--users", " userId1 , userId2 ", "--member-type", "DEVELOPER", "--yes"},
			wantTool: "remove_dev_app_members",
			wantParams: map[string]any{
				"unifiedAppId":  "app-001",
				"memberUserIds": []string{"userId1", "userId2"},
				"memberType":    "DEVELOPER",
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
			root.SetArgs(append([]string{"devapp", "member", tc.cmd}, tc.args...))

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

func TestDevAppCommandHasAppAliasAndCoreCommands(t *testing.T) {
	root := newDevAppCommand(&captureRunner{})
	if root.Name() != "devapp" {
		t.Fatalf("Name() = %q, want devapp", root.Name())
	}
	hasAlias := false
	for _, alias := range root.Aliases {
		if alias == "app" {
			hasAlias = true
		}
	}
	if !hasAlias {
		t.Fatalf("Aliases = %v, want app", root.Aliases)
	}
	for _, name := range []string{"list", "get", "create", "update", "delete", "inactive", "active", "credentials", "webapp", "permission", "member", "security", "robot", "version", "event"} {
		if _, _, err := root.Find([]string{name}); err != nil {
			t.Fatalf("missing command %q: %v", name, err)
		}
	}
	robotCmd, _, err := root.Find([]string{"robot"})
	if err != nil {
		t.Fatalf("missing robot command: %v", err)
	}
	for _, cmd := range robotCmd.Commands() {
		if cmd.Name() == "update" {
			t.Fatal("robot update command exists, want removed public command")
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
			name:     "create sync",
			args:     []string{"robot", "create", "--app-name", "智能体", "--robot-name", "小助手", "--desc", "审批问答", "--yes"},
			wantTool: "create_dingtalk_robot",
			wantParams: map[string]any{
				"appName":   "智能体",
				"robotName": "小助手",
				"desc":      "审批问答",
			},
		},
		{
			name:     "submit async fills media placeholders",
			args:     []string{"robot", "submit", "--app-name", "智能体", "--robot-name", "小助手", "--desc", "审批问答", "--task-id", "t-1", "--yes"},
			wantTool: "submit_robot_create_task",
			wantParams: map[string]any{
				"appName":        "智能体",
				"robotName":      "小助手",
				"desc":           "审批问答",
				"robotMediaId":   "",
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
			wantTool:   "get_open_dev_app_robot_config",
			wantParams: map[string]any{"unifiedAppId": "u-1"},
		},
		{
			name:     "config create with skills and mode",
			args:     []string{"robot", "config", "--unified-app-id", "u-1", "--name", "小助手", "--brief", "审批助手", "--mode", "2", "--skills", "qa,approval", "--add-scope", "--yes"},
			wantTool: "set_open_dev_app_robot_config",
			wantParams: map[string]any{
				"unifiedAppId": "u-1",
				"name":         "小助手",
				"brief":        "审批助手",
				"mode":         2,
				"skillList":    []string{"qa", "approval"},
				"isAddScope":   true,
			},
		},
		{
			name:       "offline",
			args:       []string{"robot", "offline", "--unified-app-id", "u-1", "--yes"},
			wantTool:   "offline_open_dev_app_robot",
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
			root.SetArgs(append([]string{"devapp"}, tc.args...))

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
			name:     "create",
			args:     []string{"version", "create", "--unified-app-id", "u-1", "--version", "1.0.1", "--desc", "新增机器人", "--yes"},
			wantTool: "create_open_dev_app_version",
			wantParams: map[string]any{
				"unifiedAppId": "u-1",
				"version":      "1.0.1",
				"description":  "新增机器人",
			},
		},
		{
			name:     "list",
			args:     []string{"version", "list", "--unified-app-id", "u-1", "--cursor", "cursor-1", "--page-size", "5"},
			wantTool: "list_open_dev_app_versions",
			wantParams: map[string]any{
				"unifiedAppId": "u-1",
				"cursor":       "cursor-1",
				"pageSize":     5,
			},
		},
		{
			name:       "get detail",
			args:       []string{"version", "get", "--unified-app-id", "u-1", "--version-id", "v-1"},
			wantTool:   "get_open_dev_app_version_detail",
			wantParams: map[string]any{"unifiedAppId": "u-1", "versionId": "v-1"},
		},
		{
			name:       "check-approval prechecks only",
			args:       []string{"version", "check-approval", "--unified-app-id", "u-1", "--version-id", "v-1"},
			wantTool:   "publish_open_dev_app_version",
			wantParams: map[string]any{"unifiedAppId": "u-1", "versionId": "v-1", "precheckOnly": true},
		},
		{
			name:     "publish disables precheck and sets sensitive",
			args:     []string{"version", "publish", "--unified-app-id", "u-1", "--version-id", "v-1", "--confirm-sensitive", "--approver", "user-1", "--yes"},
			wantTool: "publish_open_dev_app_version",
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
			wantTool:   "get_open_dev_app_version_status",
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
			root.SetArgs(append([]string{"devapp"}, tc.args...))

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

func TestDevAppEventCommandsBuildToolParams(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantTool   string
		wantParams map[string]any
	}{
		{
			name:       "list",
			args:       []string{"event", "list", "--unified-app-id", "u-1"},
			wantTool:   "list_open_dev_app_events",
			wantParams: map[string]any{"unifiedAppId": "u-1"},
		},
		{
			name:       "subscribe",
			args:       []string{"event", "subscribe", "--unified-app-id", "u-1", "--event-code", "user_add_org", "--yes"},
			wantTool:   "subscribe_open_dev_app_event",
			wantParams: map[string]any{"unifiedAppId": "u-1", "eventCode": "user_add_org"},
		},
		{
			name:       "unsubscribe",
			args:       []string{"event", "unsubscribe", "--unified-app-id", "u-1", "--event-code", "user_add_org", "--yes"},
			wantTool:   "unsubscribe_open_dev_app_event",
			wantParams: map[string]any{"unifiedAppId": "u-1", "eventCode": "user_add_org"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &captureRunner{}
			root := newDevAppTestRoot(runner)
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(append([]string{"devapp"}, tc.args...))

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

func TestDevAppEventSubscribeRequiresEventCode(t *testing.T) {
	runner := &captureRunner{}
	root := newDevAppTestRoot(runner)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"devapp", "event", "subscribe", "--unified-app-id", "u-1", "--yes"})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want --event-code validation")
	}
	if !strings.Contains(err.Error(), "event-code") {
		t.Fatalf("error = %q, want event-code validation", err.Error())
	}
	if runner.last.Tool != "" {
		t.Fatalf("tool = %q, want no invocation", runner.last.Tool)
	}
}

func TestDevAppScopedCommandsRejectLegacyLocators(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "version create", args: []string{"devapp", "version", "create", "--app-id", "u-1", "--version", "1.0.1", "--yes"}},
		{name: "version list", args: []string{"devapp", "version", "list", "--app-id", "u-1"}},
		{name: "version get", args: []string{"devapp", "version", "get", "--app-id", "u-1", "--version-id", "v-1"}},
		{name: "version check approval", args: []string{"devapp", "version", "check-approval", "--app-id", "u-1", "--version-id", "v-1"}},
		{name: "version publish", args: []string{"devapp", "version", "publish", "--app-id", "u-1", "--version-id", "v-1", "--yes"}},
		{name: "version status", args: []string{"devapp", "version", "status", "--app-id", "u-1", "--version-id", "v-1"}},
		{name: "robot get", args: []string{"devapp", "robot", "get", "--app-id", "u-1"}},
		{name: "robot config", args: []string{"devapp", "robot", "config", "--app-id", "u-1", "--name", "小助手", "--yes"}},
		{name: "robot enable", args: []string{"devapp", "robot", "enable", "--app-id", "u-1", "--name", "小助手", "--yes"}},
		{name: "robot offline", args: []string{"devapp", "robot", "offline", "--app-id", "u-1", "--yes"}},
		{name: "event list", args: []string{"devapp", "event", "list", "--app-id", "u-1"}},
		{name: "event subscribe", args: []string{"devapp", "event", "subscribe", "--app-id", "u-1", "--event-code", "user_add_org", "--yes"}},
		{name: "event unsubscribe", args: []string{"devapp", "event", "unsubscribe", "--app-id", "u-1", "--event-code", "user_add_org", "--yes"}},
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
				t.Fatal("Execute() error = nil, want unknown --app-id flag")
			}
			if !strings.Contains(err.Error(), "unknown flag: --app-id") {
				t.Fatalf("error = %q, want unknown --app-id flag", err.Error())
			}
			if runner.last.Tool != "" {
				t.Fatalf("tool = %q, want no invocation", runner.last.Tool)
			}
		})
	}
}

func TestDevAppScopedCommandsRejectAgentAndCustomKeyLocators(t *testing.T) {
	for _, flag := range []string{"--agent-id", "--custom-key"} {
		for _, args := range [][]string{
			{"devapp", "version", "list", flag, "u-1"},
			{"devapp", "robot", "get", flag, "u-1"},
			{"devapp", "event", "list", flag, "u-1"},
		} {
			t.Run(strings.Join(args[1:3], " ")+" "+flag, func(t *testing.T) {
				runner := &captureRunner{}
				root := newDevAppTestRoot(runner)
				var out bytes.Buffer
				root.SetOut(&out)
				root.SetErr(&out)
				root.SetArgs(args)

				err := root.Execute()
				if err == nil {
					t.Fatalf("Execute() error = nil, want unknown %s flag", flag)
				}
				if !strings.Contains(err.Error(), "unknown flag: "+flag) {
					t.Fatalf("error = %q, want unknown %s flag", err.Error(), flag)
				}
				if runner.last.Tool != "" {
					t.Fatalf("tool = %q, want no invocation", runner.last.Tool)
				}
			})
		}
	}
}

func TestDevAppRobotAndVersionWritesRequireGuard(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "robot create", args: []string{"devapp", "robot", "create", "--app-name", "智能体", "--robot-name", "小助手", "--desc", "审批"}},
		{name: "robot config", args: []string{"devapp", "robot", "config", "--unified-app-id", "u-1", "--name", "小助手"}},
		{name: "robot offline", args: []string{"devapp", "robot", "offline", "--unified-app-id", "u-1"}},
		{name: "version create", args: []string{"devapp", "version", "create", "--unified-app-id", "u-1", "--version", "1.0.1"}},
		{name: "version publish", args: []string{"devapp", "version", "publish", "--unified-app-id", "u-1", "--version-id", "v-1"}},
		{name: "event subscribe", args: []string{"devapp", "event", "subscribe", "--unified-app-id", "u-1", "--event-code", "user_add_org"}},
		{name: "event unsubscribe", args: []string{"devapp", "event", "unsubscribe", "--unified-app-id", "u-1", "--event-code", "user_add_org"}},
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
			if !strings.Contains(err.Error(), "write operation") {
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
	root.SetArgs([]string{"list", "--name", "Waker", "--app-key", "dingxxx", "--page-size", "5", "--cursor", "next-1"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	if got := runner.last.Tool; got != "list_open_dev_app" {
		t.Fatalf("Tool = %q, want list_open_dev_app", got)
	}
	want := map[string]any{
		"pageSize": 5,
		"cursor":   "next-1",
		"name":     "Waker",
		"appKey":   "dingxxx",
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
	root.SetArgs([]string{"devapp", "create", "--name", "Demo", "--desc", "internal app", "--yes"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	if got := runner.last.Tool; got != "create_dev_app" {
		t.Fatalf("Tool = %q, want create_dev_app", got)
	}

	want := map[string]any{"appName": "Demo", "appDesc": "internal app"}
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
	root.SetArgs([]string{"devapp", "update", "--unified-app-id", "u-1", "--desc", "new desc", "--yes"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	if got := runner.last.Tool; got != "update_dev_app" {
		t.Fatalf("Tool = %q, want update_dev_app", got)
	}
	want := map[string]any{"unifiedAppId": "u-1", "appDesc": "new desc"}
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
			name:       "delete by name",
			args:       []string{"delete", "--name", "Demo", "--yes"},
			wantTool:   "delete_dev_app",
			wantParams: map[string]any{"name": "Demo"},
		},
		{
			name:       "inactive by unified app id",
			args:       []string{"inactive", "--unified-app-id", "u-1", "--yes"},
			wantTool:   "disable_dev_app",
			wantParams: map[string]any{"unifiedAppId": "u-1"},
		},
		{
			name:       "active by app key",
			args:       []string{"active", "--app-key", "dingxxx", "--yes"},
			wantTool:   "enable_dev_app",
			wantParams: map[string]any{"appKey": "dingxxx"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &captureRunner{}
			root := newDevAppTestRoot(runner)
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(append([]string{"devapp"}, tc.args...))

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
			wantTool:   "get_webapp_config",
			wantParams: map[string]any{"unifiedAppId": "u-1"},
		},
		{
			name:     "config",
			args:     []string{"webapp", "config", "--unified-app-id", "u-1", "--homepage-url", "https://example.com", "--pc-homepage-url", "https://pc.example.com", "--yes"},
			wantTool: "set_webapp_config",
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
			root.SetArgs(append([]string{"devapp"}, tc.args...))

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
			args:     []string{"permission", "list", "--unified-app-id", "u-1", "--keyword", "手机号", "--status", "all", "--page-size", "5", "--cursor", "next-1"},
			wantTool: "list_open_dev_app_permissions",
			wantParams: map[string]any{
				"unifiedAppId": "u-1",
				"keyword":      "手机号",
				"authStatus":   "ALL",
				"pageSize":     5,
				"cursor":       "next-1",
			},
		},
		{
			name:     "add",
			args:     []string{"permission", "add", "--unified-app-id", "u-1", "--permissions", "Contact.User.mobile,qyapi_robot_sendmsg", "--yes"},
			wantTool: "apply_open_dev_app_permissions",
			wantParams: map[string]any{
				"unifiedAppId": "u-1",
				"scopeValues":  []string{"Contact.User.mobile", "qyapi_robot_sendmsg"},
			},
		},
		{
			name:     "remove",
			args:     []string{"permission", "remove", "--unified-app-id", "u-1", "--permission", "Contact.User.mobile", "--yes"},
			wantTool: "remove_open_dev_app_permission",
			wantParams: map[string]any{
				"unifiedAppId": "u-1",
				"scopeValue":   "Contact.User.mobile",
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
			root.SetArgs(append([]string{"devapp"}, tc.args...))

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
	root.SetArgs([]string{"devapp", "credentials", "get", "--unified-app-id", "u-1"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	if got := runner.last.Tool; got != "get_open_dev_app_credentials" {
		t.Fatalf("Tool = %q, want get_open_dev_app_credentials", got)
	}
	want := map[string]any{"unifiedAppId": "u-1"}
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
	root.SetArgs([]string{"devapp", "credentials", "get", "--unified-app-id", "u-1"})

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
	root.SetArgs([]string{"devapp", "version", "list", "--unified-app-id", "u-1"})

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

func TestDevAppMemberCommandsValidateRequiredFlags(t *testing.T) {
	cases := []struct {
		name    string
		cmd     string
		args    []string
		wantErr string
	}{
		{name: "list requires app", cmd: "list", args: nil, wantErr: "--app-id is required"},
		{name: "add requires users", cmd: "add", args: []string{"--app-id", "app-001", "--member-type", "DEVELOPER", "--dry-run"}, wantErr: "--users is required"},
		{name: "add rejects empty users", cmd: "add", args: []string{"--app-id", "app-001", "--users", " , ", "--member-type", "DEVELOPER", "--dry-run"}, wantErr: "--users must contain at least one userId"},
		{name: "remove requires member type", cmd: "remove", args: []string{"--app-id", "app-001", "--users", "userId1", "--dry-run"}, wantErr: "--member-type is required"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &captureRunner{}
			root := newDevAppTestRoot(runner)
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(append([]string{"devapp", "member", tc.cmd}, tc.args...))

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
		"devapp", "security", "config",
		"--app-id", "app-001",
		"--ip-whitelist", "192.0.2.10,192.0.2.11",
		"--redirect-url", "https://callback.example.invalid/callback",
		"--sso-url", "https://sso.example.invalid/sso",
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
	root.SetArgs([]string{"devapp", "security", "config", "--app-id", "app-001", "--redirect-url", "https://callback.example.invalid/callback", "--dry-run"})

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
	root.SetArgs([]string{"devapp", "security", "config", "--app-id", "app-001", "--dry-run"})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), "one of --ip-whitelist, --redirect-url, or --sso-url is required") {
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
			args: []string{"devapp", "member", "add", "--app-id", "app-001", "--users", "userId1", "--member-type", "DEVELOPER"},
		},
		{
			name: "member remove",
			args: []string{"devapp", "member", "remove", "--app-id", "app-001", "--users", "userId1", "--member-type", "DEVELOPER"},
		},
		{
			name: "security config",
			args: []string{"devapp", "security", "config", "--app-id", "app-001", "--redirect-url", "https://callback.example.invalid/callback"},
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
			if !strings.Contains(err.Error(), "write operation") {
				t.Fatalf("error = %q, want write guard", err.Error())
			}
			if runner.last.Tool != "" {
				t.Fatalf("tool = %q, want no invocation", runner.last.Tool)
			}
		})
	}
}
