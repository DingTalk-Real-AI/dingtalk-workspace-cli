// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package devapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type devAppCoverageCall struct {
	tool   string
	params map[string]any
}

type devAppCoverageCaller struct {
	responses map[string][]map[string]any
	calls     []devAppCoverageCall
}

func (c *devAppCoverageCaller) CallTool(_ context.Context, _, tool string, params map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, devAppCoverageCall{tool: tool, params: params})
	queue := c.responses[tool]
	if len(queue) == 0 {
		return nil, errors.New("unexpected devapp coverage call: " + tool)
	}
	payload := queue[0]
	c.responses[tool] = queue[1:]
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: string(encoded)}}}, nil
}

func (*devAppCoverageCaller) Format() string { return "json" }
func (*devAppCoverageCaller) DryRun() bool   { return false }
func (*devAppCoverageCaller) Fields() string { return "" }
func (*devAppCoverageCaller) JQ() string     { return "" }

func runDevAppCoverage(t *testing.T, declaration shortcut.Shortcut, caller *devAppCoverageCaller, args ...string) error {
	t.Helper()
	helpers.InitDeps(caller)
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	service := &cobra.Command{Use: "devapp"}
	service.AddCommand(corecmd.New(shortcut.FromShortcut(declaration)))
	root.AddCommand(service)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetIn(strings.NewReader(""))
	root.SetArgs(append([]string{"devapp", declaration.Command}, args...))
	return root.Execute()
}

func runDevAppUnifiedCoverage(t *testing.T, declaration shortcut.Shortcut, caller *devAppCoverageCaller, args ...string) ([]byte, error) {
	t.Helper()
	helpers.InitDeps(caller)
	ctx, _ := output.WithResultStore(context.Background())
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.SetContext(ctx)
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	service := &cobra.Command{Use: "devapp"}
	service.AddCommand(corecmd.New(shortcut.FromShortcut(declaration)))
	root.AddCommand(service)
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetIn(strings.NewReader(""))
	root.PersistentPostRunE = func(executed *cobra.Command, _ []string) error {
		_, _, err := output.EmitStoredResult(executed)
		return err
	}
	root.SetArgs(append([]string{"devapp", declaration.Command}, args...))
	if err := root.Execute(); err != nil {
		return nil, err
	}
	return stdout.Bytes(), nil
}

func devAppRegistered(command string) (shortcut.Shortcut, bool) {
	for _, item := range shortcut.All() {
		if item.Service == "devapp" && item.Command == command {
			return item, true
		}
	}
	return shortcut.Shortcut{}, false
}

func TestCrossPlatformCoverageDevAppStrictResponseValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		data map[string]any
	}{
		{name: "empty", data: nil},
		{name: "missing success", data: map[string]any{"list": []any{}}},
		{name: "wrong success type", data: map[string]any{"success": "true", "list": []any{}}},
		{name: "explicit failure", data: map[string]any{"success": false, "list": []any{}}},
		{name: "conflicting error", data: map[string]any{"success": true, "errorCode": "DENIED", "list": []any{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := listAppProject(tc.data); err == nil {
				t.Fatal("malformed response was accepted")
			}
		})
	}

	for _, tc := range []struct {
		name string
		data map[string]any
	}{
		{name: "missing collection", data: map[string]any{"success": true, "hasMore": false}},
		{name: "wrong collection type", data: map[string]any{"success": true, "list": map[string]any{}, "hasMore": false}},
		{name: "bad element", data: map[string]any{"success": true, "list": []any{"bad"}, "hasMore": false}},
		{name: "missing stable id", data: map[string]any{"success": true, "list": []any{map[string]any{"name": "app"}}, "hasMore": false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := listAppProject(tc.data); err == nil {
				t.Fatal("malformed collection was accepted")
			}
		})
	}

	valid := []map[string]any{{"unifiedAppId": "app"}}
	for _, tc := range []struct {
		name string
		data map[string]any
	}{
		{name: "missing pagination", data: map[string]any{"success": true}},
		{name: "wrong has more type", data: map[string]any{"success": true, "hasMore": "false"}},
		{name: "cursor without has more", data: map[string]any{"success": true, "nextCursor": "next"}},
		{name: "more without cursor", data: map[string]any{"success": true, "hasMore": true}},
		{name: "wrong cursor type", data: map[string]any{"success": true, "hasMore": true, "nextCursor": 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := devAppListProjection(tc.data, "apps", valid, "devapp/test"); err == nil {
				t.Fatal("malformed pagination was accepted")
			}
		})
	}
}

func TestCrossPlatformCoverageDevAppConfirmationAndValidationPrecedeCalls(t *testing.T) {
	writes := []struct {
		declaration shortcut.Shortcut
		args        []string
	}{
		{CreateApp, []string{"--name", "app"}},
		{UpdateApp, []string{"--unified-app-id", "app", "--name", "new"}},
		{DeleteApp, []string{"--unified-app-id", "app"}},
		{EnableApp, []string{"--unified-app-id", "app"}},
		{DisableApp, []string{"--unified-app-id", "app"}},
		{WebappConfig, []string{"--unified-app-id", "app", "--homepage-url", "https://example.invalid"}},
		{RobotConfig, []string{"--unified-app-id", "app", "--name", "bot"}},
		{RobotEnable, []string{"--unified-app-id", "app"}},
		{RobotDisable, []string{"--unified-app-id", "app"}},
		{EventSubscribe, []string{"--unified-app-id", "app", "--event-codes", "event.one"}},
		{VersionCreate, []string{"--unified-app-id", "app", "--desc", "version"}},
	}
	for _, tc := range writes {
		t.Run(tc.declaration.Command, func(t *testing.T) {
			caller := &devAppCoverageCaller{}
			err := runDevAppCoverage(t, tc.declaration, caller, tc.args...)
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Reason != "confirmation_required" {
				t.Fatalf("unconfirmed error = %#v, want confirmation_required", err)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("unconfirmed command called MCP: %#v", caller.calls)
			}
		})
	}

	for _, tc := range []struct {
		declaration shortcut.Shortcut
		args        []string
	}{
		{UpdateApp, []string{"--unified-app-id", "app", "--yes"}},
		{WebappConfig, []string{"--unified-app-id", "app", "--yes"}},
		{RobotConfig, []string{"--unified-app-id", "app", "--yes"}},
		{EventSubscribe, []string{"--unified-app-id", "app", "--event-codes", "event.one,event.one", "--yes"}},
	} {
		caller := &devAppCoverageCaller{}
		err := runDevAppCoverage(t, tc.declaration, caller, tc.args...)
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Reason == "confirmation_required" {
			t.Fatalf("invalid input error = %#v", err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("invalid input called MCP: %#v", caller.calls)
		}
	}
}

func TestCrossPlatformCoverageDevAppResidualAdapters(t *testing.T) {
	t.Run("credentials require stable identity client and secret", func(t *testing.T) {
		valid := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"get_dev_app_credentials": {{"success": true, "result": map[string]any{
				"unifiedAppId": "app", "appKey": "client", "appSecret": "secret",
			}}},
		}}
		if err := runDevAppCoverage(t, GetCredentials, valid, "--unified-app-id", "app"); err != nil {
			t.Fatal(err)
		}
		missingSecret := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"get_dev_app_credentials": {{"success": true, "result": map[string]any{
				"unifiedAppId": "app", "appKey": "client",
			}}},
		}}
		if err := runDevAppCoverage(t, GetCredentials, missingSecret, "--unified-app-id", "app"); err == nil {
			t.Fatal("credential response without a secret was accepted")
		}
	})

	t.Run("credentials unified data matches declared sensitive paths", func(t *testing.T) {
		declaration, ok := devAppRegistered("+credentials-get")
		if !ok || declaration.OutputRollout != output.RolloutUnifiedActive {
			t.Fatalf("registered credentials shortcut is not unified: found=%t rollout=%q", ok, declaration.OutputRollout)
		}
		caller := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"get_dev_app_credentials": {{"success": true, "result": map[string]any{
				"unifiedAppId": "app", "appKey": "client", "appSecret": "test-app-secret",
				"clientSecret": "test-client-secret", "secret": "test-secret",
			}}},
		}}
		encoded, err := runDevAppUnifiedCoverage(t, declaration, caller, "--unified-app-id", "app")
		if err != nil {
			t.Fatal(err)
		}
		var envelope map[string]any
		if err := json.Unmarshal(encoded, &envelope); err != nil {
			t.Fatalf("unified output is not JSON: %v", err)
		}
		data, ok := envelope["data"].(map[string]any)
		if !ok {
			t.Fatalf("unified output data type = %T, want object", envelope["data"])
		}
		if _, nested := data["result"]; nested {
			t.Fatal("credential business object is nested under data.result; declared sensitive paths would miss it")
		}
		for _, path := range declaration.Contract.Result.SensitivePaths {
			if value, present := data[path]; !present || strings.TrimSpace(fmt.Sprint(value)) == "" {
				t.Fatalf("declared sensitive path %q does not resolve to a non-empty data field", path)
			}
		}
	})

	t.Run("member projector supports known and guaranteed zero", func(t *testing.T) {
		members := []any{
			map[string]any{"userId": "user-1", "memberType": "OWNER"},
			map[string]any{"userId": "user-2", "memberType": "DEVELOPER"},
		}
		if got := projectDevAppMembers(members, "user-1"); len(got) != 1 || got[0]["userId"] != "user-1" {
			t.Fatalf("known member projection = %#v", got)
		}
		if got := projectDevAppMembers(members, "guaranteed-missing"); len(got) != 0 {
			t.Fatalf("zero member projection = %#v", got)
		}
		for _, bad := range []map[string]any{
			{"success": true, "members": []any{"bad"}},
			{"success": true, "members": []any{map[string]any{"name": "missing-id"}}},
		} {
			if _, _, err := requireDevAppCollection(bad, "devapp/list_dev_app_members", []string{"userId"}, "members"); err == nil {
				t.Fatalf("malformed member response accepted: %#v", bad)
			}
		}
	})

	for _, tc := range []struct {
		name        string
		declaration shortcut.Shortcut
		args        []string
		responses   map[string][]map[string]any
		wantTools   []string
	}{
		{
			name: "robot config", declaration: RobotConfig,
			args: []string{"--unified-app-id", "app", "--name", "bot", "--mode", "STREAM", "--yes"},
			responses: map[string][]map[string]any{
				"set_extension_robot_config": {{"success": true}},
				"get_extension_robot_config": {{"success": true, "result": map[string]any{"unifiedAppId": "app", "name": "bot", "mode": "STREAM"}}},
			},
			wantTools: []string{"set_extension_robot_config", "get_extension_robot_config"},
		},
		{
			name: "robot enable", declaration: RobotEnable,
			args: []string{"--unified-app-id", "app", "--yes"},
			responses: map[string][]map[string]any{
				"enable_dev_app_robot":       {{"success": true}},
				"get_extension_robot_config": {{"success": true, "result": map[string]any{"unifiedAppId": "app", "robotStatus": "ONLINE"}}},
			},
			wantTools: []string{"enable_dev_app_robot", "get_extension_robot_config"},
		},
		{
			name: "robot disable", declaration: RobotDisable,
			args: []string{"--unified-app-id", "app", "--yes"},
			responses: map[string][]map[string]any{
				"disable_dev_app_robot":      {{"success": true}},
				"get_extension_robot_config": {{"success": true, "result": map[string]any{"unifiedAppId": "app", "robotStatus": "UNCONFIGURED"}}},
			},
			wantTools: []string{"disable_dev_app_robot", "get_extension_robot_config"},
		},
		{
			name: "event subscribe", declaration: EventSubscribe,
			args: []string{"--unified-app-id", "app", "--event-codes", "event.one", "--yes"},
			responses: map[string][]map[string]any{
				"subscribe_dev_app_events": {{"success": true}},
				"list_dev_app_events":      {{"success": true, "list": []any{map[string]any{"eventCode": "event.one"}}, "hasMore": false}},
			},
			wantTools: []string{"subscribe_dev_app_events", "list_dev_app_events"},
		},
		{
			name: "version create", declaration: VersionCreate,
			args: []string{"--unified-app-id", "app", "--desc", "snapshot", "--yes"},
			responses: map[string][]map[string]any{
				"create_dev_app_version":     {{"success": true, "result": map[string]any{"versionId": "version"}}},
				"get_dev_app_version_detail": {{"success": true, "result": map[string]any{"unifiedAppId": "app", "versionId": "version", "desc": "snapshot"}}},
			},
			wantTools: []string{"create_dev_app_version", "get_dev_app_version_detail"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &devAppCoverageCaller{responses: tc.responses}
			if err := runDevAppCoverage(t, tc.declaration, caller, tc.args...); err != nil {
				t.Fatal(err)
			}
			gotTools := make([]string, 0, len(caller.calls))
			for _, call := range caller.calls {
				gotTools = append(gotTools, call.tool)
			}
			if !reflect.DeepEqual(gotTools, tc.wantTools) {
				t.Fatalf("tools = %#v, want %#v", gotTools, tc.wantTools)
			}
		})
	}

	t.Run("event false receipt is rejected by readback", func(t *testing.T) {
		caller := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"subscribe_dev_app_events": {{"success": true}},
			"list_dev_app_events":      {{"success": true, "list": []any{}, "hasMore": false}},
		}}
		if err := runDevAppCoverage(t, EventSubscribe, caller, "--unified-app-id", "app", "--event-codes", "event.one", "--yes"); err == nil {
			t.Fatal("event subscription without exact readback was accepted")
		}
	})
}

func TestCrossPlatformCoverageDevAppWritesUseExactReadback(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		caller := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"create_dev_app": {{"success": true, "result": map[string]any{"unifiedAppId": "app"}}},
			"get_dev_app":    {{"success": true, "result": map[string]any{"unifiedAppId": "app", "name": "created"}}},
		}}
		if err := runDevAppCoverage(t, CreateApp, caller, "--name", "created", "--yes"); err != nil {
			t.Fatal(err)
		}
		want := []devAppCoverageCall{
			{tool: "create_dev_app", params: map[string]any{"name": "created"}},
			{tool: "get_dev_app", params: map[string]any{"unifiedAppId": "app"}},
		}
		if !reflect.DeepEqual(caller.calls, want) {
			t.Fatalf("calls = %#v, want %#v", caller.calls, want)
		}
	})

	t.Run("update mismatch fails", func(t *testing.T) {
		caller := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"update_dev_app": {{"success": true}},
			"get_dev_app":    {{"success": true, "result": map[string]any{"unifiedAppId": "app", "name": "old"}}},
		}}
		if err := runDevAppCoverage(t, UpdateApp, caller, "--unified-app-id", "app", "--name", "new", "--yes"); err == nil {
			t.Fatal("write/readback mismatch was accepted")
		}
		if len(caller.calls) != 2 || caller.calls[0].tool != "update_dev_app" || caller.calls[1].tool != "get_dev_app" {
			t.Fatalf("calls = %#v", caller.calls)
		}
	})

	for _, tc := range []struct {
		name        string
		declaration shortcut.Shortcut
		writeTool   string
		status      string
	}{
		{name: "enable", declaration: EnableApp, writeTool: "enable_dev_app", status: "normal"},
		{name: "disable", declaration: DisableApp, writeTool: "disable_dev_app", status: "disabled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &devAppCoverageCaller{responses: map[string][]map[string]any{
				tc.writeTool: {{"success": true}},
				"get_dev_app": {{"success": true, "result": map[string]any{
					"unifiedAppId": "app", "appStatus": tc.status,
				}}},
			}}
			if err := runDevAppCoverage(t, tc.declaration, caller, "--unified-app-id", "app", "--yes"); err != nil {
				t.Fatal(err)
			}
			if len(caller.calls) != 2 || caller.calls[0].tool != tc.writeTool || caller.calls[1].tool != "get_dev_app" {
				t.Fatalf("calls = %#v", caller.calls)
			}
		})
	}

	t.Run("webapp config", func(t *testing.T) {
		caller := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"set_extension_webapp_config": {{"success": true}},
			"get_extension_webapp_config": {{"success": true, "result": map[string]any{
				"unifiedAppId": "app", "homepageUrl": "https://example.invalid",
			}}},
		}}
		if err := runDevAppCoverage(t, WebappConfig, caller, "--unified-app-id", "app", "--homepage-url", "https://example.invalid", "--yes"); err != nil {
			t.Fatal(err)
		}
		if len(caller.calls) != 2 || caller.calls[0].tool != "set_extension_webapp_config" || caller.calls[1].tool != "get_extension_webapp_config" {
			t.Fatalf("calls = %#v", caller.calls)
		}
	})

	t.Run("delete pre-read and absence", func(t *testing.T) {
		caller := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"get_dev_app":    {{"success": true, "result": map[string]any{"unifiedAppId": "app", "appKey": "key"}}},
			"delete_dev_app": {{"success": true}},
			"list_dev_app":   {{"success": true, "list": []any{}, "hasMore": false}},
		}}
		if err := runDevAppCoverage(t, DeleteApp, caller, "--unified-app-id", "app", "--yes"); err != nil {
			t.Fatal(err)
		}
		want := []devAppCoverageCall{
			{tool: "get_dev_app", params: map[string]any{"unifiedAppId": "app"}},
			{tool: "delete_dev_app", params: map[string]any{"unifiedAppId": "app"}},
			{tool: "list_dev_app", params: map[string]any{"appKey": "key", "pageSize": 20}},
		}
		if !reflect.DeepEqual(caller.calls, want) {
			t.Fatalf("calls = %#v, want %#v", caller.calls, want)
		}
	})
}

func TestCrossPlatformCoverageDevAppReviewedInventory(t *testing.T) {
	public := 0
	unavailable := 0
	total := 0
	for _, item := range shortcut.All() {
		if item.Service != "devapp" {
			continue
		}
		total++
		if item.Availability == shortcut.AvailabilityUnavailable {
			unavailable++
			if !item.Hidden || item.CompatibilityVisible || shortcut.InPublicCatalog(item.Service, item.Command) {
				t.Errorf("unavailable %s visibility/public = hidden=%v compatibility=%v public=%v",
					item.Command, item.Hidden, item.CompatibilityVisible, shortcut.InPublicCatalog(item.Service, item.Command))
			}
			if strings.TrimSpace(devAppUnavailableReasons[item.Command]) == "" {
				t.Errorf("unavailable %s lacks concrete reason", item.Command)
			}
			if !item.Contract.Empty() {
				t.Errorf("hidden unavailable %s must not publish an incomplete Contract", item.Command)
			}
			continue
		}
		if shortcut.InPublicCatalog(item.Service, item.Command) {
			public++
			if item.Hidden || item.Availability != shortcut.AvailabilityAvailable {
				t.Errorf("public %s visibility/availability = %v/%q", item.Command, item.Hidden, item.Availability)
			}
			if item.OutputRollout != output.RolloutUnifiedActive {
				t.Errorf("public %s rollout = %q", item.Command, item.OutputRollout)
			}
			if item.Contract.Result == nil || item.Contract.Interface == nil || item.Contract.Description == "" {
				t.Errorf("public %s has incomplete Contract/Result", item.Command)
			}
			if item.Safety.Effect == "" || item.Safety.Risk == "" || item.Safety.Confirmation == "" || item.Safety.Idempotency == "" {
				t.Errorf("public %s has incomplete Safety", item.Command)
			}
		}
	}
	if total != 30 || public != 25 || unavailable != 5 {
		t.Fatalf("devapp inventory total/public/unavailable = %d/%d/%d, want 30/25/5", total, public, unavailable)
	}
}

func TestCrossPlatformCoverageDevAppUnavailableCommandsMakeZeroCalls(t *testing.T) {
	for command, reason := range devAppUnavailableReasons {
		declaration, found := devAppRegistered(command)
		if !found {
			t.Fatalf("registered unavailable command missing: %s", command)
		}
		args := []string{}
		for _, flag := range declaration.Flags {
			if !flag.Required {
				continue
			}
			args = append(args, "--"+flag.Name)
			if flag.Type != shortcut.FlagBool {
				args = append(args, "fixture")
			}
		}
		caller := &devAppCoverageCaller{}
		err := runDevAppCoverage(t, declaration, caller, args...)
		if err == nil || !strings.Contains(err.Error(), reason) {
			t.Errorf("%s unavailable error = %v", command, err)
		}
		if len(caller.calls) != 0 {
			t.Errorf("%s unavailable command called MCP: %#v", command, caller.calls)
		}
	}
}
