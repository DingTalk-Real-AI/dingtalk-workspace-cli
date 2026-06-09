package helpers

import (
	"bytes"
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
			wantTool: "list_open_dev_app_members",
			wantParams: map[string]any{
				"unifiedAppId": "app-001",
			},
		},
		{
			name:     "add multiple users",
			cmd:      "add",
			args:     []string{"--app-id", "app-001", "--users", "userId1,userId2,userId3,userId4", "--member-type", "DEVELOPER", "--yes"},
			wantTool: "add_open_dev_app_members",
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
			wantTool: "remove_open_dev_app_members",
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
	for _, name := range []string{"list", "get", "create", "update", "delete", "inactive", "active", "webapp", "permission", "member", "security"} {
		if _, _, err := root.Find([]string{name}); err != nil {
			t.Fatalf("missing command %q: %v", name, err)
		}
	}
}

func TestDevAppListBuildsListByConditionParams(t *testing.T) {
	runner := &captureRunner{}
	root := newDevAppCommand(runner)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"list", "--name", "Waker", "--page", "2", "--page-size", "5", "--sort", "gmt_modified", "--order", "desc"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	if got := runner.last.Tool; got != "list_open_dev_apps_by_condition" {
		t.Fatalf("Tool = %q, want list_open_dev_apps_by_condition", got)
	}
	want := map[string]any{
		"currentPage": 2,
		"pageSize":    5,
		"appName":     "Waker",
		"sortType":    "gmt_modified",
		"sortOrder":   "desc",
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
	if got := runner.last.Tool; got != "get_open_dev_app_detail" {
		t.Fatalf("Tool = %q, want get_open_dev_app_detail", got)
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
	if got := runner.last.Tool; got != "create_inner_app" {
		t.Fatalf("Tool = %q, want create_inner_app", got)
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
	root.SetArgs([]string{"devapp", "update", "--agent-id", "123", "--desc", "new desc", "--yes"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	if got := runner.last.Tool; got != "update_inner_app" {
		t.Fatalf("Tool = %q, want update_inner_app", got)
	}
	want := map[string]any{"agentId": 123, "appDesc": "new desc"}
	if !reflect.DeepEqual(runner.last.Params, want) {
		t.Fatalf("Params = %#v, want %#v", runner.last.Params, want)
	}
}

func TestDevAppLifecycleBuildsLocatorParams(t *testing.T) {
	runner := &captureRunner{}
	root := newDevAppTestRoot(runner)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"devapp", "inactive", "--unified-app-id", "u-1", "--yes"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	if got := runner.last.Tool; got != "inactive_inner_app" {
		t.Fatalf("Tool = %q, want inactive_inner_app", got)
	}
	want := map[string]any{"unifiedAppId": "u-1"}
	if !reflect.DeepEqual(runner.last.Params, want) {
		t.Fatalf("Params = %#v, want %#v", runner.last.Params, want)
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
			args:     []string{"webapp", "config", "--unified-app-id", "u-1", "--homepage-link", "https://example.com", "--pc-homepage-link", "https://pc.example.com", "--yes"},
			wantTool: "set_webapp_config",
			wantParams: map[string]any{
				"unifiedAppId":   "u-1",
				"homepageLink":   "https://example.com",
				"pcHomepageLink": "https://pc.example.com",
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
			args:     []string{"permission", "list", "--agent-id", "123", "--keyword", "手机号", "--status", "all", "--limit", "5"},
			wantTool: "list_open_dev_app_permissions",
			wantParams: map[string]any{
				"agentId":    123,
				"keyword":    "手机号",
				"authStatus": "ALL",
				"limit":      5,
			},
		},
		{
			name:     "add",
			args:     []string{"permission", "add", "--agent-id", "123", "--permissions", "Contact.User.mobile,qyapi_robot_sendmsg", "--yes"},
			wantTool: "apply_open_dev_app_permissions",
			wantParams: map[string]any{
				"agentId":     123,
				"scopeValues": []string{"Contact.User.mobile", "qyapi_robot_sendmsg"},
			},
		},
		{
			name:     "remove",
			args:     []string{"permission", "remove", "--agent-id", "123", "--permission", "Contact.User.mobile", "--yes"},
			wantTool: "remove_open_dev_app_permission",
			wantParams: map[string]any{
				"agentId":    123,
				"scopeValue": "Contact.User.mobile",
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
		"--ip-whitelist", "103.211.230.150,103.211.230.151",
		"--redirect-url", "https://example.com/callback",
		"--sso-url", "https://example.com/sso",
		"--dry-run",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	if got := runner.last.CanonicalProduct; got != "devapp" {
		t.Fatalf("CanonicalProduct = %q, want devapp", got)
	}
	if got := runner.last.Tool; got != "update_app_security_config" {
		t.Fatalf("Tool = %q, want update_app_security_config", got)
	}
	want := map[string]any{
		"unifiedAppId":  "app-001",
		"ipWhiteList":   []string{"103.211.230.150", "103.211.230.151"},
		"redirectUrls":  []string{"https://example.com/callback"},
		"otherAuthUrls": []string{"https://example.com/sso"},
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
	root.SetArgs([]string{"devapp", "security", "config", "--app-id", "app-001", "--redirect-url", "https://example.com/callback", "--dry-run"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	want := map[string]any{
		"unifiedAppId": "app-001",
		"redirectUrls": []string{"https://example.com/callback"},
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
			args: []string{"devapp", "security", "config", "--app-id", "app-001", "--redirect-url", "https://example.com/callback"},
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
