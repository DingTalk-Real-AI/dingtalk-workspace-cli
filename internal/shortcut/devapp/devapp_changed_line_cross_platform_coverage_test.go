// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package devapp

import (
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/spf13/cobra"
)

func devAppSuccessResult(values map[string]any) map[string]any {
	return map[string]any{"success": true, "result": values}
}

func devAppSuccessList(key string, items []any, more bool, cursor string) map[string]any {
	result := map[string]any{"success": true, key: items, "hasMore": more}
	if cursor != "" {
		result["nextCursor"] = cursor
	}
	return result
}

func TestCrossPlatformCoverageDevAppChangedStrictHelpers(t *testing.T) {
	if devAppContainers(nil) != nil {
		t.Fatal("nil response unexpectedly produced containers")
	}
	for _, tc := range []struct {
		value any
		want  bool
	}{
		{nil, false}, {false, false}, {true, true},
		{float64(0), false}, {float64(200), false}, {float64(500), true},
		{"", false}, {"ok", false}, {"SUCCESS", false}, {"denied", true},
		{map[string]any{}, false}, {map[string]any{"code": 1}, true},
		{[]any{}, false}, {[]any{"error"}, true}, {struct{}{}, true},
	} {
		if got := devAppErrorValue(tc.value); got != tc.want {
			t.Fatalf("devAppErrorValue(%#v)=%v, want %v", tc.value, got, tc.want)
		}
	}

	if _, err := requireDevAppSuccess(map[string]any{
		"success": true, "result": map[string]any{"versionStatus": "FAILED"},
	}, "devapp/test"); err != nil {
		t.Fatalf("successful query with failed business state was rejected: %v", err)
	}
	if !devAppOnlyEnvelopeFields(map[string]any{"success": true, "result": map[string]any{}}) {
		t.Fatal("known envelope was not recognized")
	}
	if devAppOnlyEnvelopeFields(map[string]any{"success": true, "business": true}) {
		t.Fatal("business field was treated as envelope-only")
	}
	if _, err := requireDevAppObject(map[string]any{"success": true, "result": map[string]any{}}, "devapp/test"); err == nil {
		t.Fatal("empty business object was accepted")
	}

	object := map[string]any{"id": "actual", "name": "actual"}
	if err := requireDevAppIdentity(object, "devapp/test", map[string]string{"ignored": ""}); err != nil {
		t.Fatal(err)
	}
	if err := requireDevAppIdentity(object, "devapp/test", map[string]string{"missing": "wanted"}); err == nil {
		t.Fatal("missing identity was accepted")
	}
	if err := requireDevAppIdentity(object, "devapp/test", map[string]string{"id": "wanted"}); err == nil {
		t.Fatal("mismatched identity was accepted")
	}
	if err := requireDevAppFields(object, "devapp/test", map[string]any{"missing": "wanted"}); err == nil {
		t.Fatal("missing readback field was accepted")
	}
	if err := requireDevAppFields(object, "devapp/test", map[string]any{"name": "wanted"}); err == nil {
		t.Fatal("mismatched readback field was accepted")
	}
	if err := requireDevAppFields(object, "devapp/test", map[string]any{"name": "actual"}); err != nil {
		t.Fatal(err)
	}
	if err := requireDevAppStringField(object, "devapp/test", "wanted", "missing"); err == nil {
		t.Fatal("missing string field was accepted")
	}

	deep := map[string]any{"success": true, "content": map[string]any{
		"result": map[string]any{"hasMore": true, "nextCursor": " next "},
	}}
	projection, err := devAppListProjection(deep, "items", nil, "devapp/test")
	if err != nil || projection["nextCursor"] != "next" {
		t.Fatalf("deep pagination projection=%#v err=%v", projection, err)
	}
	if _, err := devAppListProjection(map[string]any{"success": false}, "items", nil, "devapp/test"); err == nil {
		t.Fatal("failed list envelope was accepted")
	}
}

func TestCrossPlatformCoverageDevAppChangedReadAndListExecutors(t *testing.T) {
	t.Run("list all filters and projection", func(t *testing.T) {
		item := map[string]any{
			"unified_app_id": "app", "appName": "name", "clientId": "client",
			"agent_id": "agent", "app_status": "normal", "modified_time": "now",
		}
		caller := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"list_dev_app": {devAppSuccessList("apps", []any{item}, true, "next")},
		}}
		err := runDevAppCoverage(t, ListApp, caller,
			"--name", "name", "--app-key", "client", "--app-group-id", "7",
			"--creator", "owner", "--robot-name", "robot", "--develop-type", "2",
			"--filter-cool-app", "1", "--sort-type", "gmt_modified", "--sort-order", "desc",
			"--cursor", "cursor", "--page-size", "9")
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]any{
			"name": "name", "appKey": "client", "appGroupId": 7, "creator": "owner",
			"robotName": "robot", "developType": 2, "filterCoolApp": 1,
			"sortType": "gmt_modified", "sortOrder": "desc", "cursor": "cursor", "pageSize": 9,
		}
		if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0].params, want) {
			t.Fatalf("list params=%#v, want %#v", caller.calls, want)
		}
		badPage := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"list_dev_app": {{"success": true, "list": []any{map[string]any{"unifiedAppId": "app"}}}},
		}}
		if err := runDevAppCoverage(t, ListApp, badPage); err == nil {
			t.Fatal("list without terminal pagination evidence was accepted")
		}
	})

	for _, tc := range []struct {
		name        string
		declaration shortcut.Shortcut
		tool        string
		args        []string
		result      map[string]any
	}{
		{"get", GetApp, "get_dev_app", []string{"--unified-app-id", "app"}, map[string]any{"unifiedAppId": "app", "name": "app"}},
		{"webapp", WebappGet, "get_extension_webapp_config", []string{"--unified-app-id", "app"}, map[string]any{"unifiedAppId": "app", "homepageUrl": "https://example.invalid"}},
		{"robot", RobotGet, "get_extension_robot_config", []string{"--unified-app-id", "app"}, map[string]any{"unifiedAppId": "app", "robotStatus": "ONLINE"}},
		{"version", VersionGet, "get_dev_app_version_detail", []string{"--unified-app-id", "app", "--version-id", "version"}, map[string]any{"unifiedAppId": "app", "versionId": "version"}},
		{"precheck", VersionCheckApproval, "publish_dev_app_version", []string{"--unified-app-id", "app", "--version-id", "version"}, map[string]any{"unifiedAppId": "app", "versionId": "version", "approvalRequired": false}},
		{"status", VersionStatus, "get_dev_app_version_status", []string{"--unified-app-id", "app", "--version-id", "version"}, map[string]any{"unifiedAppId": "app", "versionId": "version", "status": "PUBLISHED"}},
		{"status failed", VersionStatus, "get_dev_app_version_status", []string{"--unified-app-id", "app", "--version-id", "version"}, map[string]any{"unifiedAppId": "app", "versionId": "version", "versionStatus": "FAILED"}},
		{"status expired", VersionStatus, "get_dev_app_version_status", []string{"--unified-app-id", "app", "--version-id", "version"}, map[string]any{"unifiedAppId": "app", "versionId": "version", "versionStatus": "EXPIRED"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &devAppCoverageCaller{responses: map[string][]map[string]any{tc.tool: {devAppSuccessResult(tc.result)}}}
			if err := runDevAppCoverage(t, tc.declaration, caller, tc.args...); err != nil {
				t.Fatal(err)
			}
		})
	}

	t.Run("read transport and identity failure", func(t *testing.T) {
		if err := runDevAppCoverage(t, GetApp, &devAppCoverageCaller{}, "--unified-app-id", "app"); err == nil {
			t.Fatal("transport failure was accepted")
		}
		caller := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"get_dev_app": {devAppSuccessResult(map[string]any{"unifiedAppId": "other"})},
		}}
		if err := runDevAppCoverage(t, GetApp, caller, "--unified-app-id", "app"); err == nil {
			t.Fatal("wrong object identity was accepted")
		}
	})

	t.Run("permission list", func(t *testing.T) {
		item := map[string]any{
			"permissionCode": "scope", "permissionName": "name", "interfaceName": "api",
			"auth_status": "AUTHED", "scope_type": "APP",
		}
		caller := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"list_dev_app_permissions": {devAppSuccessList("permissions", []any{item}, false, "")},
		}}
		err := runDevAppCoverage(t, PermissionList, caller,
			"--unified-app-id", "app", "--keyword", "word", "--scope-value", "scope",
			"--auth-status", "authed", "--scope-type", "app", "--api-status", "OPEN")
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := permissionListFirst(item, "missing"); ok {
			t.Fatal("missing permission field unexpectedly resolved")
		}
	})

	t.Run("member list success and failures", func(t *testing.T) {
		caller := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"list_dev_app_members": {devAppSuccessList("members", []any{map[string]any{"userId": "user"}}, false, "")},
		}}
		if err := runDevAppCoverage(t, MemberList, caller, "--unified-app-id", "app", "--user-id", " user "); err != nil {
			t.Fatal(err)
		}
		if err := runDevAppCoverage(t, MemberList, &devAppCoverageCaller{}, "--unified-app-id", "app"); err == nil {
			t.Fatal("member transport failure was accepted")
		}
		bad := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"list_dev_app_members": {{"success": true, "members": "bad"}},
		}}
		if err := runDevAppCoverage(t, MemberList, bad, "--unified-app-id", "app"); err == nil {
			t.Fatal("malformed member list was accepted")
		}
	})

	for _, tc := range []struct {
		name        string
		declaration shortcut.Shortcut
		tool        string
		args        []string
		key         string
		item        map[string]any
	}{
		{"events", EventList, "list_dev_app_events", []string{"--unified-app-id", "app", "--keyword", "event"}, "events", map[string]any{"event_code": "event", "event_name": "name", "subscribe_status": "ON", "modified_time": "now"}},
		{"versions", VersionList, "list_dev_app_versions", []string{"--unified-app-id", "app"}, "versions", map[string]any{"version_id": "version", "version_name": "1", "versionStatus": "READY", "remark": "desc", "create_time": "now"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &devAppCoverageCaller{responses: map[string][]map[string]any{
				tc.tool: {devAppSuccessList(tc.key, []any{tc.item}, false, "")},
			}}
			if err := runDevAppCoverage(t, tc.declaration, caller, tc.args...); err != nil {
				t.Fatal(err)
			}
		})
	}
	if _, ok := eventListFirst(map[string]any{}, "missing"); ok {
		t.Fatal("missing event field unexpectedly resolved")
	}
	if _, ok := versionListFirst(map[string]any{}, "missing"); ok {
		t.Fatal("missing version field unexpectedly resolved")
	}
}

func TestCrossPlatformCoverageDevAppChangedWriteExecutors(t *testing.T) {
	t.Run("create all fields and failure branches", func(t *testing.T) {
		caller := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"create_dev_app": {devAppSuccessResult(map[string]any{"unifiedAppId": "app"})},
			"get_dev_app": {devAppSuccessResult(map[string]any{
				"unifiedAppId": "app", "name": "created", "desc": "desc", "iconMediaId": "icon",
			})},
		}}
		if err := runDevAppCoverage(t, CreateApp, caller,
			"--name", "created", "--desc", "desc", "--icon-media-id", "icon", "--yes"); err != nil {
			t.Fatal(err)
		}
		if err := runDevAppCoverage(t, CreateApp, &devAppCoverageCaller{}, "--name", "created", "--dry-run"); err != nil {
			t.Fatal(err)
		}
		if err := runDevAppCoverage(t, CreateApp, &devAppCoverageCaller{}, "--name", "created", "--yes"); err == nil {
			t.Fatal("create transport failure was accepted")
		}
		for name, responses := range map[string]map[string][]map[string]any{
			"receipt object missing": {"create_dev_app": {{"success": true, "result": map[string]any{}}}},
			"resource id missing":    {"create_dev_app": {devAppSuccessResult(map[string]any{"name": "created"})}},
			"readback failure": {
				"create_dev_app": {devAppSuccessResult(map[string]any{"unifiedAppId": "app"})},
			},
			"name mismatch": {
				"create_dev_app": {devAppSuccessResult(map[string]any{"unifiedAppId": "app"})},
				"get_dev_app":    {devAppSuccessResult(map[string]any{"unifiedAppId": "app", "name": "other"})},
			},
			"description mismatch": {
				"create_dev_app": {devAppSuccessResult(map[string]any{"unifiedAppId": "app"})},
				"get_dev_app":    {devAppSuccessResult(map[string]any{"unifiedAppId": "app", "name": "created", "desc": "other"})},
			},
			"icon mismatch": {
				"create_dev_app": {devAppSuccessResult(map[string]any{"unifiedAppId": "app"})},
				"get_dev_app":    {devAppSuccessResult(map[string]any{"unifiedAppId": "app", "name": "created", "iconMediaId": "other"})},
			},
		} {
			t.Run(name, func(t *testing.T) {
				args := []string{"--name", "created", "--yes"}
				if strings.Contains(name, "description") {
					args = append(args, "--desc", "desc")
				}
				if strings.Contains(name, "icon") {
					args = append(args, "--icon-media-id", "icon")
				}
				if err := runDevAppCoverage(t, CreateApp, &devAppCoverageCaller{responses: responses}, args...); err == nil {
					t.Fatal("invalid create sequence was accepted")
				}
			})
		}
	})

	t.Run("update all fields and dry run", func(t *testing.T) {
		caller := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"update_dev_app": {map[string]any{"success": true}},
			"get_dev_app": {devAppSuccessResult(map[string]any{
				"unifiedAppId": "app", "name": "new", "desc": "desc", "iconMediaId": "icon",
			})},
		}}
		if err := runDevAppCoverage(t, UpdateApp, caller,
			"--unified-app-id", "app", "--name", "new", "--desc", "desc", "--icon-media-id", "icon", "--yes"); err != nil {
			t.Fatal(err)
		}
		if err := runDevAppCoverage(t, UpdateApp, &devAppCoverageCaller{},
			"--unified-app-id", "app", "--name", "new", "--dry-run"); err != nil {
			t.Fatal(err)
		}
		bad := &devAppCoverageCaller{responses: map[string][]map[string]any{"update_dev_app": {map[string]any{"success": true}}}}
		if err := runDevAppCoverage(t, UpdateApp, bad, "--unified-app-id", "app", "--name", "new", "--yes"); err == nil {
			t.Fatal("update readback failure was accepted")
		}
	})

	t.Run("delete guards and bounded readback", func(t *testing.T) {
		if err := runDevAppCoverage(t, DeleteApp, &devAppCoverageCaller{}, "--unified-app-id", "app", "--dry-run"); err != nil {
			t.Fatal(err)
		}
		if err := verifyDeletedDevApp(shortcut.RuntimeContextForTest(&cobra.Command{Use: "+delete"}, DeleteApp), "app", ""); err == nil {
			t.Fatal("delete verification without selector was accepted")
		}
		missingSelector := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"get_dev_app": {devAppSuccessResult(map[string]any{"unifiedAppId": "app"})},
		}}
		if err := runDevAppCoverage(t, DeleteApp, missingSelector, "--unified-app-id", "app", "--yes"); err == nil {
			t.Fatal("delete without appKey was accepted")
		}
		writeFailure := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"get_dev_app": {devAppSuccessResult(map[string]any{"unifiedAppId": "app", "appKey": "key"})},
		}}
		if err := runDevAppCoverage(t, DeleteApp, writeFailure, "--unified-app-id", "app", "--yes"); err == nil {
			t.Fatal("delete transport failure was accepted")
		}
		stillPresent := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"get_dev_app":    {devAppSuccessResult(map[string]any{"unifiedAppId": "app", "appKey": "key"})},
			"delete_dev_app": {map[string]any{"success": true}},
			"list_dev_app":   {devAppSuccessList("list", []any{map[string]any{"unifiedAppId": "app"}}, false, "")},
		}}
		if err := runDevAppCoverage(t, DeleteApp, stillPresent, "--unified-app-id", "app", "--yes"); err == nil {
			t.Fatal("delete with present readback was accepted")
		}
		stalled := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"get_dev_app":    {devAppSuccessResult(map[string]any{"unifiedAppId": "app", "appKey": "key"})},
			"delete_dev_app": {map[string]any{"success": true}},
			"list_dev_app": {
				devAppSuccessList("list", []any{map[string]any{"unifiedAppId": "other"}}, true, "same"),
				devAppSuccessList("list", []any{map[string]any{"unifiedAppId": "other"}}, true, "same"),
			},
		}}
		if err := runDevAppCoverage(t, DeleteApp, stalled, "--unified-app-id", "app", "--yes"); err == nil {
			t.Fatal("delete stalled pagination was accepted")
		}
		malformed := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"get_dev_app":    {devAppSuccessResult(map[string]any{"unifiedAppId": "app", "appKey": "key"})},
			"delete_dev_app": {map[string]any{"success": true}},
			"list_dev_app":   {{"success": true, "list": "bad", "hasMore": false}},
		}}
		if err := runDevAppCoverage(t, DeleteApp, malformed, "--unified-app-id", "app", "--yes"); err == nil {
			t.Fatal("delete malformed readback was accepted")
		}
		badPage := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"get_dev_app":    {devAppSuccessResult(map[string]any{"unifiedAppId": "app", "appKey": "key"})},
			"delete_dev_app": {map[string]any{"success": true}},
			"list_dev_app":   {{"success": true, "list": []any{map[string]any{"unifiedAppId": "other"}}}},
		}}
		if err := runDevAppCoverage(t, DeleteApp, badPage, "--unified-app-id", "app", "--yes"); err == nil {
			t.Fatal("delete pagination without termination was accepted")
		}
		pages := make([]map[string]any, 0, devAppMaxReadbackPages)
		for page := 0; page < devAppMaxReadbackPages; page++ {
			pages = append(pages, devAppSuccessList("list", []any{map[string]any{"unifiedAppId": "other"}}, true, "cursor-"+string(rune('a'+page))))
		}
		pageLimitCaller := &devAppCoverageCaller{responses: map[string][]map[string]any{"list_dev_app": pages}}
		helpers.InitDeps(pageLimitCaller)
		if err := verifyDeletedDevApp(shortcut.RuntimeContextForTest(&cobra.Command{Use: "+delete"}, DeleteApp), "app", "key"); err == nil {
			t.Fatal("delete verification page limit was accepted")
		}
		helpers.InitDeps(&devAppCoverageCaller{})
		if err := verifyDeletedDevApp(shortcut.RuntimeContextForTest(&cobra.Command{Use: "+delete"}, DeleteApp), "app", "key"); err == nil {
			t.Fatal("delete verification transport failure was accepted")
		}
	})

	t.Run("credentials require client id", func(t *testing.T) {
		caller := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"get_dev_app_credentials": {devAppSuccessResult(map[string]any{"unifiedAppId": "app", "appSecret": "secret"})},
		}}
		if err := runDevAppCoverage(t, GetCredentials, caller, "--unified-app-id", "app"); err == nil {
			t.Fatal("credential without client id was accepted")
		}
	})

	t.Run("webapp all fields and failures", func(t *testing.T) {
		args := []string{"--unified-app-id", "app", "--h5-page-type", "H5", "--homepage-url", "https://example.invalid/h5", "--pc-homepage-url", "https://example.invalid/pc", "--omp-url", "https://example.invalid/omp", "--yes"}
		resource := map[string]any{"unifiedAppId": "app", "h5PageType": "H5", "homepageUrl": "https://example.invalid/h5", "pcHomepageUrl": "https://example.invalid/pc", "ompUrl": "https://example.invalid/omp"}
		caller := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"set_extension_webapp_config": {map[string]any{"success": true}},
			"get_extension_webapp_config": {devAppSuccessResult(resource)},
		}}
		if err := runDevAppCoverage(t, WebappConfig, caller, args...); err != nil {
			t.Fatal(err)
		}
		if err := runDevAppCoverage(t, WebappConfig, &devAppCoverageCaller{}, "--unified-app-id", "app", "--homepage-url", "https://example.invalid", "--dry-run"); err != nil {
			t.Fatal(err)
		}
		if err := runDevAppCoverage(t, WebappConfig, &devAppCoverageCaller{}, args...); err == nil {
			t.Fatal("webapp write failure was accepted")
		}
		badRead := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"set_extension_webapp_config": {map[string]any{"success": true}},
		}}
		if err := runDevAppCoverage(t, WebappConfig, badRead, args...); err == nil {
			t.Fatal("webapp readback failure was accepted")
		}
		mismatch := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"set_extension_webapp_config": {map[string]any{"success": true}},
			"get_extension_webapp_config": {devAppSuccessResult(map[string]any{"unifiedAppId": "app", "h5PageType": "other"})},
		}}
		if err := runDevAppCoverage(t, WebappConfig, mismatch, "--unified-app-id", "app", "--h5-page-type", "H5", "--yes"); err == nil {
			t.Fatal("webapp mismatch was accepted")
		}
	})
}

func TestCrossPlatformCoverageDevAppChangedRobotEventAndVersionBranches(t *testing.T) {
	t.Run("robot validate and all fields", func(t *testing.T) {
		for _, args := range [][]string{
			{"--unified-app-id", "app", "--skills", "skill", "--yes"},
			{"--unified-app-id", "app", "--add-scope=false", "--yes"},
		} {
			caller := &devAppCoverageCaller{responses: map[string][]map[string]any{
				"set_extension_robot_config": {map[string]any{"success": true}},
				"get_extension_robot_config": {devAppSuccessResult(map[string]any{"unifiedAppId": "app", "skills": []any{"skill"}, "addScope": false})},
			}}
			if err := runDevAppCoverage(t, RobotConfig, caller, args...); err != nil {
				t.Fatal(err)
			}
		}
		args := []string{"--unified-app-id", "app", "--name", "bot", "--brief", "brief", "--desc", "desc", "--icon-media-id", "icon", "--outgoing-url", "https://example.invalid/out", "--event-callback-url", "https://example.invalid/event", "--mode", "STREAM", "--skills", "skill", "--add-scope=false", "--disable-ssl-verify=false", "--yes"}
		resource := map[string]any{"unifiedAppId": "app", "name": "bot", "brief": "brief", "desc": "desc", "iconMediaId": "icon", "outgoingUrl": "https://example.invalid/out", "eventCallbackUrl": "https://example.invalid/event", "mode": "STREAM", "skills": []string{"skill"}, "addScope": false, "disableSSLVerify": false}
		caller := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"set_extension_robot_config": {map[string]any{"success": true}},
			"get_extension_robot_config": {devAppSuccessResult(resource)},
		}}
		if err := runDevAppCoverage(t, RobotConfig, caller, args...); err != nil {
			t.Fatal(err)
		}
		if err := runDevAppCoverage(t, RobotConfig, &devAppCoverageCaller{}, "--unified-app-id", "app", "--name", "bot", "--dry-run"); err != nil {
			t.Fatal(err)
		}
		if err := runDevAppCoverage(t, RobotConfig, &devAppCoverageCaller{}, args...); err == nil {
			t.Fatal("robot write failure was accepted")
		}
		badRead := &devAppCoverageCaller{responses: map[string][]map[string]any{"set_extension_robot_config": {map[string]any{"success": true}}}}
		if err := runDevAppCoverage(t, RobotConfig, badRead, args...); err == nil {
			t.Fatal("robot readback failure was accepted")
		}
		mismatch := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"set_extension_robot_config": {map[string]any{"success": true}},
			"get_extension_robot_config": {devAppSuccessResult(map[string]any{"unifiedAppId": "app", "name": "other"})},
		}}
		if err := runDevAppCoverage(t, RobotConfig, mismatch, "--unified-app-id", "app", "--name", "bot", "--yes"); err == nil {
			t.Fatal("robot mismatch was accepted")
		}
		boolMismatch := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"set_extension_robot_config": {map[string]any{"success": true}},
			"get_extension_robot_config": {devAppSuccessResult(map[string]any{"unifiedAppId": "app", "addScope": true})},
		}}
		if err := runDevAppCoverage(t, RobotConfig, boolMismatch, "--unified-app-id", "app", "--add-scope=false", "--yes"); err == nil {
			t.Fatal("robot boolean mismatch was accepted")
		}
		skillsMismatch := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"set_extension_robot_config": {map[string]any{"success": true}},
			"get_extension_robot_config": {devAppSuccessResult(map[string]any{"unifiedAppId": "app", "skills": []string{"other"}})},
		}}
		if err := runDevAppCoverage(t, RobotConfig, skillsMismatch, "--unified-app-id", "app", "--skills", "skill", "--yes"); err == nil {
			t.Fatal("robot skills mismatch was accepted")
		}
	})

	t.Run("status previews and readback failures", func(t *testing.T) {
		if err := runDevAppCoverage(t, EnableApp, &devAppCoverageCaller{}, "--unified-app-id", "app", "--dry-run"); err != nil {
			t.Fatal(err)
		}
		readFailure := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"enable_dev_app": {map[string]any{"success": true}},
		}}
		if err := runDevAppCoverage(t, EnableApp, readFailure, "--unified-app-id", "app", "--yes"); err == nil {
			t.Fatal("app status readback failure was accepted")
		}
		mismatch := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"enable_dev_app": {map[string]any{"success": true}},
			"get_dev_app":    {devAppSuccessResult(map[string]any{"unifiedAppId": "app", "appStatus": "disabled"})},
		}}
		if err := runDevAppCoverage(t, EnableApp, mismatch, "--unified-app-id", "app", "--yes"); err == nil {
			t.Fatal("app status mismatch was accepted")
		}
		if err := runDevAppCoverage(t, RobotEnable, &devAppCoverageCaller{}, "--unified-app-id", "app", "--dry-run"); err != nil {
			t.Fatal(err)
		}
		robotReadFailure := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"enable_dev_app_robot": {map[string]any{"success": true}},
		}}
		if err := runDevAppCoverage(t, RobotEnable, robotReadFailure, "--unified-app-id", "app", "--yes"); err == nil {
			t.Fatal("robot status readback failure was accepted")
		}
	})

	t.Run("member mutations validate receipts and exact readback", func(t *testing.T) {
		member := map[string]any{"userId": "user", "memberType": "DEVELOPER"}
		add := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"add_dev_app_members":  {{"success": true}},
			"list_dev_app_members": {devAppSuccessList("members", []any{member}, false, "")},
		}}
		if err := runDevAppCoverage(t, MemberAdd, add,
			"--unified-app-id", "app", "--user-ids", " user ", "--member-type", " DEVELOPER ", "--yes"); err != nil {
			t.Fatal(err)
		}
		if len(add.calls) != 2 || add.calls[0].tool != "add_dev_app_members" ||
			!reflect.DeepEqual(add.calls[0].params["userIds"], []string{"user"}) ||
			add.calls[0].params["memberType"] != "DEVELOPER" || add.calls[1].tool != "list_dev_app_members" {
			t.Fatalf("member add calls=%#v", add.calls)
		}

		remove := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"remove_dev_app_members": {{"success": true}},
			"list_dev_app_members":   {devAppSuccessList("members", []any{map[string]any{"userId": "owner", "memberType": "OWNER"}}, false, "")},
		}}
		if err := runDevAppCoverage(t, MemberRemove, remove,
			"--unified-app-id", "app", "--user-ids", "user", "--member-type", "DEVELOPER", "--yes"); err != nil {
			t.Fatal(err)
		}
		if len(remove.calls) != 2 || remove.calls[0].tool != "remove_dev_app_members" || remove.calls[1].tool != "list_dev_app_members" {
			t.Fatalf("member remove calls=%#v", remove.calls)
		}

		for _, declaration := range []shortcut.Shortcut{MemberAdd, MemberRemove} {
			unconfirmed := &devAppCoverageCaller{}
			if err := runDevAppCoverage(t, declaration, unconfirmed,
				"--unified-app-id", "app", "--user-ids", "user", "--member-type", "DEVELOPER"); err == nil {
				t.Fatalf("%s succeeded without confirmation", declaration.Command)
			}
			if len(unconfirmed.calls) != 0 {
				t.Fatalf("%s made remote calls before confirmation: %#v", declaration.Command, unconfirmed.calls)
			}
			dryRun := &devAppCoverageCaller{}
			if err := runDevAppCoverage(t, declaration, dryRun,
				"--unified-app-id", "app", "--user-ids", "user", "--member-type", "DEVELOPER", "--dry-run"); err != nil {
				t.Fatal(err)
			}
			if len(dryRun.calls) != 0 {
				t.Fatalf("%s dry-run made remote calls: %#v", declaration.Command, dryRun.calls)
			}
		}
	})

	t.Run("member mutations reject invalid input and false success", func(t *testing.T) {
		for _, values := range [][]string{nil, {""}, {"user", "user"}} {
			if _, err := validatedDevAppValues(values, "--user-ids"); err == nil {
				t.Fatalf("invalid member IDs accepted: %#v", values)
			}
		}
		if _, err := validatedDevAppMemberType(" "); err == nil {
			t.Fatal("blank member type was accepted")
		}
		if got, err := validatedDevAppMemberType(" DEVELOPER "); err != nil || got != "DEVELOPER" {
			t.Fatalf("normalized member type=%q err=%v", got, err)
		}
		for _, args := range [][]string{
			{"--unified-app-id", "app", "--user-ids", "user,user", "--member-type", "DEVELOPER", "--yes"},
			{"--unified-app-id", "app", "--user-ids", "user", "--member-type", " ", "--yes"},
		} {
			caller := &devAppCoverageCaller{}
			if err := runDevAppCoverage(t, MemberAdd, caller, args...); err == nil {
				t.Fatalf("invalid member arguments accepted: %#v", args)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("invalid member arguments made calls: %#v", caller.calls)
			}
		}

		baseArgs := []string{"--unified-app-id", "app", "--user-ids", "user", "--member-type", "DEVELOPER", "--yes"}
		badAddResponses := []map[string][]map[string]any{
			{},
			{"add_dev_app_members": {{"success": false}}},
			{"add_dev_app_members": {{"success": true}}},
			{"add_dev_app_members": {{"success": true}}, "list_dev_app_members": {{"success": true, "members": "bad"}}},
			{"add_dev_app_members": {{"success": true}}, "list_dev_app_members": {devAppSuccessList("members", []any{map[string]any{"userId": "other", "memberType": "DEVELOPER"}}, false, "")}},
			{"add_dev_app_members": {{"success": true}}, "list_dev_app_members": {devAppSuccessList("members", []any{map[string]any{"userId": "user"}}, false, "")}},
			{"add_dev_app_members": {{"success": true}}, "list_dev_app_members": {devAppSuccessList("members", []any{map[string]any{"userId": "user", "memberType": "OWNER"}}, false, "")}},
			{"add_dev_app_members": {{"success": true}}, "list_dev_app_members": {devAppSuccessList("members", []any{
				map[string]any{"userId": "user", "memberType": "DEVELOPER"},
				map[string]any{"userId": "user", "memberType": "DEVELOPER"},
			}, false, "")}},
		}
		for index, responses := range badAddResponses {
			if err := runDevAppCoverage(t, MemberAdd, &devAppCoverageCaller{responses: responses}, baseArgs...); err == nil {
				t.Fatalf("member add false success case %d was accepted", index)
			}
		}

		removeStillPresent := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"remove_dev_app_members": {{"success": true}},
			"list_dev_app_members": {devAppSuccessList("members", []any{
				map[string]any{"userId": "user", "memberType": "DEVELOPER"},
			}, false, "")},
		}}
		if err := runDevAppCoverage(t, MemberRemove, removeStillPresent, baseArgs...); err == nil {
			t.Fatal("member remove accepted target still present")
		}
	})

	for _, tc := range []struct {
		name        string
		declaration shortcut.Shortcut
		writeTool   string
		status      any
	}{
		{"enable missing status", RobotEnable, "enable_dev_app_robot", nil},
		{"disable wrong status", RobotDisable, "disable_dev_app_robot", "ONLINE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resource := map[string]any{"unifiedAppId": "app"}
			if tc.status != nil {
				resource["robotStatus"] = tc.status
			}
			caller := &devAppCoverageCaller{responses: map[string][]map[string]any{
				tc.writeTool:                 {map[string]any{"success": true}},
				"get_extension_robot_config": {devAppSuccessResult(resource)},
			}}
			if err := runDevAppCoverage(t, tc.declaration, caller, "--unified-app-id", "app", "--yes"); err == nil {
				t.Fatal("invalid robot status was accepted")
			}
		})
	}

	t.Run("event validation and readback failures", func(t *testing.T) {
		for _, values := range [][]string{nil, {""}, {"event", "event"}} {
			if _, err := validatedDevAppValues(values, "--event-codes"); err == nil {
				t.Fatalf("invalid event values accepted: %#v", values)
			}
		}
		if got, err := validatedDevAppValues([]string{" event "}, "--event-codes"); err != nil || !reflect.DeepEqual(got, []string{"event"}) {
			t.Fatalf("normalized event values=%#v err=%v", got, err)
		}
		if err := runDevAppCoverage(t, EventSubscribe, &devAppCoverageCaller{}, "--unified-app-id", "app", "--event-codes", "event", "--yes"); err == nil {
			t.Fatal("event write failure was accepted")
		}
		if err := runDevAppCoverage(t, EventSubscribe, &devAppCoverageCaller{}, "--unified-app-id", "app", "--event-codes", "event", "--dry-run"); err != nil {
			t.Fatal(err)
		}
		for name, listResponses := range map[string][]map[string]any{
			"transport":  nil,
			"malformed":  {{"success": true, "list": "bad", "hasMore": false}},
			"projection": {devAppSuccessList("list", []any{map[string]any{"eventCode": "other"}}, true, "")},
			"stalled":    {devAppSuccessList("list", []any{map[string]any{"eventCode": "other"}}, true, "same")},
		} {
			t.Run(name, func(t *testing.T) {
				caller := &devAppCoverageCaller{responses: map[string][]map[string]any{
					"subscribe_dev_app_events": {map[string]any{"success": true}},
					"list_dev_app_events":      listResponses,
				}}
				if err := runDevAppCoverage(t, EventSubscribe, caller, "--unified-app-id", "app", "--event-codes", "event", "--yes"); err == nil {
					t.Fatal("invalid event readback was accepted")
				}
			})
		}
		stalled := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"subscribe_dev_app_events": {map[string]any{"success": true}},
			"list_dev_app_events": {
				devAppSuccessList("list", []any{map[string]any{"eventCode": "other"}}, true, "same"),
				devAppSuccessList("list", []any{map[string]any{"eventCode": "other"}}, true, "same"),
			},
		}}
		if err := runDevAppCoverage(t, EventSubscribe, stalled, "--unified-app-id", "app", "--event-codes", "event", "--yes"); err == nil {
			t.Fatal("stalled event cursor was accepted")
		}
		pages := make([]map[string]any, 0, devAppMaxReadbackPages)
		for page := 0; page < devAppMaxReadbackPages; page++ {
			pages = append(pages, devAppSuccessList("list", []any{map[string]any{"eventCode": "other"}}, true, "cursor-"+string(rune('a'+page))))
		}
		pageLimit := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"subscribe_dev_app_events": {map[string]any{"success": true}},
			"list_dev_app_events":      pages,
		}}
		if err := runDevAppCoverage(t, EventSubscribe, pageLimit, "--unified-app-id", "app", "--event-codes", "event", "--yes"); err == nil {
			t.Fatal("event readback page limit was accepted")
		}
	})

	t.Run("version create all fields and failures", func(t *testing.T) {
		args := []string{"--unified-app-id", "app", "--version", "1.0", "--desc", "desc", "--yes"}
		caller := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"create_dev_app_version":     {devAppSuccessResult(map[string]any{"versionId": "version"})},
			"get_dev_app_version_detail": {devAppSuccessResult(map[string]any{"unifiedAppId": "app", "versionId": "version", "version": "1.0", "desc": "desc"})},
		}}
		if err := runDevAppCoverage(t, VersionCreate, caller, args...); err != nil {
			t.Fatal(err)
		}
		if err := runDevAppCoverage(t, VersionCreate, &devAppCoverageCaller{}, "--unified-app-id", "app", "--desc", "desc", "--dry-run"); err != nil {
			t.Fatal(err)
		}
		if err := runDevAppCoverage(t, VersionCreate, &devAppCoverageCaller{}, args...); err == nil {
			t.Fatal("version write failure was accepted")
		}
		for name, responses := range map[string]map[string][]map[string]any{
			"receipt object": {"create_dev_app_version": {{"success": true, "result": map[string]any{}}}},
			"missing id":     {"create_dev_app_version": {devAppSuccessResult(map[string]any{"version": "1.0"})}},
			"readback":       {"create_dev_app_version": {devAppSuccessResult(map[string]any{"versionId": "version"})}},
			"version mismatch": {
				"create_dev_app_version":     {devAppSuccessResult(map[string]any{"versionId": "version"})},
				"get_dev_app_version_detail": {devAppSuccessResult(map[string]any{"unifiedAppId": "app", "versionId": "version", "version": "other"})},
			},
			"description mismatch": {
				"create_dev_app_version":     {devAppSuccessResult(map[string]any{"versionId": "version"})},
				"get_dev_app_version_detail": {devAppSuccessResult(map[string]any{"unifiedAppId": "app", "versionId": "version", "version": "1.0", "desc": "other"})},
			},
		} {
			t.Run(name, func(t *testing.T) {
				if err := runDevAppCoverage(t, VersionCreate, &devAppCoverageCaller{responses: responses}, args...); err == nil {
					t.Fatal("invalid version create sequence was accepted")
				}
			})
		}
	})

	t.Run("list pagination projection failures", func(t *testing.T) {
		for _, tc := range []struct {
			declaration shortcut.Shortcut
			tool        string
			args        []string
			key         string
			idKey       string
		}{
			{PermissionList, "list_dev_app_permissions", []string{"--unified-app-id", "app"}, "permissions", "scopeValue"},
			{EventList, "list_dev_app_events", []string{"--unified-app-id", "app"}, "events", "eventCode"},
			{VersionList, "list_dev_app_versions", []string{"--unified-app-id", "app"}, "versions", "versionId"},
		} {
			caller := &devAppCoverageCaller{responses: map[string][]map[string]any{
				tc.tool: {{"success": true, tc.key: []any{map[string]any{tc.idKey: "id"}}}},
			}}
			if err := runDevAppCoverage(t, tc.declaration, caller, tc.args...); err == nil {
				t.Fatalf("%s accepted missing pagination", tc.declaration.Command)
			}
		}
		malformedEvent := &devAppCoverageCaller{responses: map[string][]map[string]any{
			"list_dev_app_events": {{"success": true, "events": "bad", "hasMore": false}},
		}}
		if err := runDevAppCoverage(t, EventList, malformedEvent, "--unified-app-id", "app"); err == nil {
			t.Fatal("malformed event collection was accepted")
		}
	})
}
