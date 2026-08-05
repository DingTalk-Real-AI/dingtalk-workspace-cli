// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package smart

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func docAccessCoverageResponses(permission string) map[string][]string {
	return map[string][]string{
		"contact/search_contact_by_key_word": {`{"result":[{"userId":"u1","name":"张三","openDingTalkId":"open1"}]}`},
		"doc/list_permission":                {permission},
		"doc/add_permission":                 {`{"result":{"ok":true}}`},
		"doc/update_permission":              {`{"result":{"ok":true}}`},
		"doc/remove_permission":              {`{"result":{"ok":true}}`},
		"chat/send_personal_message":         {`{"result":{"messageId":"m1"}}`},
	}
}

type docAccessErrorWriter struct{}

func (docAccessErrorWriter) Write([]byte) (int, error) { return 0, errors.New("output failure") }

func runDocAccessCoverage(t *testing.T, caller *smartCoverageCaller, args ...string) error {
	t.Helper()
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	root.SetIn(bytes.NewReader(nil))
	root.SetArgs(args)
	return root.Execute()
}

func runDocAccessCoverageOutput(t *testing.T, caller *smartCoverageCaller, args ...string) (map[string]any, error) {
	t.Helper()
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetIn(bytes.NewReader(nil))
	root.SetArgs(args)
	err := root.Execute()
	if stdout.Len() == 0 {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode output %q: %v", stdout.String(), err)
	}
	return payload, err
}

func runDocAccessDeclaration(t *testing.T, declaration shortcut.Shortcut, caller *smartCoverageCaller, args ...string) error {
	t.Helper()
	helpers.InitDeps(caller)
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	service := &cobra.Command{Use: "doc"}
	service.AddCommand(corecmd.New(shortcut.FromShortcut(declaration)))
	root.AddCommand(service)
	root.SetArgs(append([]string{"doc", declaration.Command}, args...))
	return root.Execute()
}

func TestCrossPlatformCoverageDocAccessSuccessDryRunAndPreflight(t *testing.T) {
	if docSmartContract("+test", "d", "i", nil, true).DryRun == nil {
		t.Fatal("explicit doc smart dry-run declaration missing")
	}
	present := `{"result":[{"userId":"u1","roleId":"READER"}]}`
	empty := `{"result":[]}`
	cases := []struct {
		name       string
		permission string
		args       []string
		wantErr    bool
	}{
		{"grant", present, []string{"doc", "+access-grant", "--node", "n", "--to", "张三", "--role", "READER", "--workspace", "w", "--yes"}, false},
		{"grant dry", present, []string{"doc", "+access-grant", "--node", "n", "--to", "张三", "--dry-run", "--yes"}, false},
		{"change", present, []string{"doc", "+access-change", "--node", "n", "--to", "张三", "--role", "EDITOR", "--yes"}, false},
		{"change dry", present, []string{"doc", "+access-change", "--node", "n", "--to", "张三", "--dry-run", "--yes"}, false},
		{"change missing", empty, []string{"doc", "+access-change", "--node", "n", "--to", "张三", "--yes"}, true},
		{"revoke", present, []string{"doc", "+access-revoke", "--node", "n", "--to", "张三", "--workspace", "w", "--yes"}, false},
		{"revoke dry", present, []string{"doc", "+access-revoke", "--node", "n", "--to", "张三", "--dry-run", "--yes"}, false},
		{"revoke missing", empty, []string{"doc", "+access-revoke", "--node", "n", "--to", "张三", "--yes"}, true},
		{"share", present, []string{"doc", "+share", "--to", "张三", "--url", "https://example.com/doc", "--note", "看一下", "--yes"}, false},
		{"share dry", present, []string{"doc", "+share", "--to", "张三", "--url", "https://example.com/doc", "--dry-run", "--yes"}, false},
		{"grant share existing", present, []string{"doc", "+grant-and-share", "--node", "n", "--to", "张三", "--url", "https://example.com/doc", "--yes"}, false},
		{"grant share missing", empty, []string{"doc", "+grant-and-share", "--node", "n", "--to", "张三", "--url", "https://example.com/doc", "--yes"}, false},
		{"grant share dry", empty, []string{"doc", "+grant-and-share", "--node", "n", "--to", "张三", "--url", "https://example.com/doc", "--dry-run", "--yes"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &smartCoverageCaller{responses: docAccessCoverageResponses(tc.permission), failAt: map[string]int{}}
			err := runDocAccessCoverage(t, caller, tc.args...)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestCrossPlatformCoverageDocGrantAndShareProjectionMatchesWrite(t *testing.T) {
	permission := func(payload map[string]any) string {
		t.Helper()
		data, _ := payload["data"].(map[string]any)
		recipients, _ := data["recipients"].([]any)
		if len(recipients) != 1 {
			t.Fatalf("recipients = %#v", data["recipients"])
		}
		row, _ := recipients[0].(map[string]any)
		value, _ := row["permission"].(string)
		return value
	}

	missingCaller := &smartCoverageCaller{responses: docAccessCoverageResponses(`{"result":[]}`), failAt: map[string]int{}}
	missing, err := runDocAccessCoverageOutput(t, missingCaller, "doc", "+grant-and-share", "--node", "n", "--to", "张三", "--url", "https://example.com/doc", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if missing["operation"] != "doc.grant_and_share" || permission(missing) != "granted" {
		t.Fatalf("missing projection = %#v", missing)
	}
	if missingCaller.counts["doc/add_permission"] != 1 {
		t.Fatalf("missing add calls = %d", missingCaller.counts["doc/add_permission"])
	}

	presentCaller := &smartCoverageCaller{responses: docAccessCoverageResponses(`{"result":[{"id":"u1","roleId":"READER"}]}`), failAt: map[string]int{}}
	present, err := runDocAccessCoverageOutput(t, presentCaller, "doc", "+grant-and-share", "--node", "n", "--to", "张三", "--url", "https://example.com/doc", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if present["operation"] != "doc.grant_and_share" || permission(present) != "unchanged" {
		t.Fatalf("present projection = %#v", present)
	}
	if presentCaller.counts["doc/add_permission"] != 0 || presentCaller.counts["doc/update_permission"] != 0 {
		t.Fatalf("present writes = %#v", presentCaller.counts)
	}

	upgradeCaller := &smartCoverageCaller{responses: docAccessCoverageResponses(`{"result":[{"userId":"u1","roleId":"READER"}]}`), failAt: map[string]int{}}
	upgraded, err := runDocAccessCoverageOutput(t, upgradeCaller, "doc", "+grant-and-share", "--node", "n", "--to", "张三", "--url", "https://example.com/doc", "--role", "EDITOR", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if permission(upgraded) != "upgraded" || upgradeCaller.counts["doc/add_permission"] != 0 || upgradeCaller.counts["doc/update_permission"] != 1 || upgradeCaller.counts["chat/send_personal_message"] != 1 {
		t.Fatalf("upgrade projection=%#v calls=%#v", upgraded, upgradeCaller.counts)
	}
	wantUpgrade := map[string]any{"nodeId": "n", "roleId": "EDITOR", "userIds": []string{"u1"}}
	if got := upgradeCaller.arguments["doc/update_permission"]; len(got) != 1 || !reflect.DeepEqual(got[0], wantUpgrade) {
		t.Fatalf("upgrade params = %#v, want %#v", got, wantUpgrade)
	}

	unknownCaller := &smartCoverageCaller{responses: docAccessCoverageResponses(`{"result":[{"userId":"u1"}]}`), failAt: map[string]int{}}
	if _, err := runDocAccessCoverageOutput(t, unknownCaller, "doc", "+grant-and-share", "--node", "n", "--to", "张三", "--url", "https://example.com/doc", "--yes"); err == nil {
		t.Fatal("unknown existing role must stop before permission writes and messaging")
	}
	if unknownCaller.counts["doc/add_permission"] != 0 || unknownCaller.counts["doc/update_permission"] != 0 || unknownCaller.counts["chat/send_personal_message"] != 0 {
		t.Fatalf("unknown role continued: %#v", unknownCaller.counts)
	}

	shareCaller := &smartCoverageCaller{responses: docAccessCoverageResponses(`{"result":[]}`), failAt: map[string]int{}}
	shared, err := runDocAccessCoverageOutput(t, shareCaller, "doc", "+share", "--to", "张三", "--url", "https://example.com/doc", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if shared["operation"] != "doc.share" || permission(shared) != "unchanged" {
		t.Fatalf("share projection = %#v", shared)
	}

	if missing := usersMissingPermission(map[string]any{"result": []any{map[string]any{"userId": "u10"}}}, []contactUser{{name: "张三", userID: "u1"}}); len(missing) != 1 {
		t.Fatalf("substring permission match must not pass: %#v", missing)
	}

	roles := map[string]int{"READER": 1, "DOWNLOADER": 2, "EDITOR": 3, "MANAGER": 4, "OWNER": 5, "unknown": 0}
	for role, want := range roles {
		if got := permissionRoleRank(role); got != want {
			t.Fatalf("permissionRoleRank(%q) = %d, want %d", role, got, want)
		}
	}
	collected := map[string]string{}
	collectPermissionRoles(map[string]any{"result": []any{
		map[string]any{"userID": "u2", "permissionRole": "READER"},
		map[string]any{"memberId": "u2", "permissionType": "EDITOR"},
		map[string]any{"targetId": "u3", "roleType": "DOWNLOADER"},
		map[string]any{"uid": "u4", "roleID": "MANAGER"},
		map[string]any{"id": "u5", "role": "OWNER"},
	}}, collected)
	if !reflect.DeepEqual(collected, map[string]string{"u2": "EDITOR", "u3": "DOWNLOADER", "u4": "MANAGER", "u5": "OWNER"}) {
		t.Fatalf("collected roles = %#v", collected)
	}
}

func TestCrossPlatformCoverageDocAccessRevokeConfirmationBoundary(t *testing.T) {
	present := `{"result":[{"userId":"u1","roleId":"READER"}]}`
	unconfirmed := &smartCoverageCaller{responses: docAccessCoverageResponses(present), failAt: map[string]int{}}
	if err := runDocAccessCoverage(t, unconfirmed, "doc", "+access-revoke", "--node", "n", "--to", "张三"); err == nil {
		t.Fatal("access revoke without --yes must reject")
	}
	if len(unconfirmed.counts) != 0 {
		t.Fatalf("unconfirmed access revoke called MCP: %#v", unconfirmed.counts)
	}

	confirmed := &smartCoverageCaller{responses: docAccessCoverageResponses(present), failAt: map[string]int{}}
	if err := runDocAccessCoverage(t, confirmed, "doc", "+access-revoke", "--node", "n", "--to", "张三", "--yes"); err != nil {
		t.Fatal(err)
	}
	wantCounts := map[string]int{
		"contact/search_contact_by_key_word": 1,
		"doc/list_permission":                1,
		"doc/remove_permission":              1,
	}
	if !reflect.DeepEqual(confirmed.counts, wantCounts) {
		t.Fatalf("confirmed calls = %#v, want %#v", confirmed.counts, wantCounts)
	}
	wantRemove := map[string]any{"nodeId": "n", "userIds": []string{"u1"}}
	if got := confirmed.arguments["doc/remove_permission"]; len(got) != 1 || !reflect.DeepEqual(got[0], wantRemove) {
		t.Fatalf("remove params = %#v, want %#v", got, wantRemove)
	}
}

func TestCrossPlatformCoverageDocShareKeepsLegacyStringFlagAndCSV(t *testing.T) {
	testseam.Swap(t, &resolveDocShareUser, func(_ *shortcut.RuntimeContext, name string) (contactUser, error) {
		return contactUser{name: name, userID: "user-" + name, openDingTalkID: "open-" + name}, nil
	})
	responses := docAccessCoverageResponses(`{"result":[]}`)
	responses["chat/send_personal_message"] = []string{
		`{"result":{"messageId":"m1"}}`,
		`{"result":{"messageId":"m2"}}`,
	}
	caller := &smartCoverageCaller{responses: responses, failAt: map[string]int{}}
	payload, err := runDocAccessCoverageOutput(t, caller, "doc", "+share-doc", "--to", "alice,bob", "--url", "https://example.com/doc", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := payload["data"].(map[string]any)
	recipients, _ := data["recipients"].([]any)
	if len(recipients) != 2 || caller.counts["chat/send_personal_message"] != 2 {
		t.Fatalf("recipients=%#v send calls=%d", recipients, caller.counts["chat/send_personal_message"])
	}
}

func TestCrossPlatformCoverageDocShareFailureExitContracts(t *testing.T) {
	resolve := func(_ *shortcut.RuntimeContext, name string) (contactUser, error) {
		return contactUser{name: name, userID: "user-" + name, openDingTalkID: "open-" + name}, nil
	}
	testseam.Swap(t, &resolveDocShareUser, resolve)
	testseam.Swap(t, &resolveDocPermissionUser, resolve)

	assertFailure := func(t *testing.T, payload map[string]any, err error, wantStatus string, wantSucceeded, wantFailed int) {
		t.Helper()
		if err == nil {
			t.Fatal("message failure must return a non-zero exit error")
		}
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Reason != "doc_share_message_failed" || typed.ExitCode() != 1 {
			t.Fatalf("message error = %#v", err)
		}
		if payload["ok"] != false || payload["status"] != wantStatus {
			t.Fatalf("failure payload = %#v", payload)
		}
		data, _ := payload["data"].(map[string]any)
		if data["succeededCount"] != float64(wantSucceeded) || data["failedCount"] != float64(wantFailed) {
			t.Fatalf("failure counts = %#v", data)
		}
	}

	allShareCaller := &smartCoverageCaller{responses: docAccessCoverageResponses(`{"result":[]}`), failAt: map[string]int{"chat/send_personal_message": -1}}
	allShare, err := runDocAccessCoverageOutput(t, allShareCaller, "doc", "+share", "--to", "alice", "--url", "https://example.com/doc", "--yes")
	assertFailure(t, allShare, err, "failed", 0, 1)

	permissions := `{"result":[{"userId":"user-alice","roleId":"READER"},{"userId":"user-bob","roleId":"READER"}]}`
	allGrantCaller := &smartCoverageCaller{responses: docAccessCoverageResponses(permissions), failAt: map[string]int{"chat/send_personal_message": -1}}
	allGrant, err := runDocAccessCoverageOutput(t, allGrantCaller, "doc", "+grant-and-share", "--node", "n", "--to", "alice,bob", "--url", "https://example.com/doc", "--yes")
	assertFailure(t, allGrant, err, "failed", 0, 2)

	partialCaller := &smartCoverageCaller{responses: docAccessCoverageResponses(permissions), failAt: map[string]int{"chat/send_personal_message": 2}}
	partial, err := runDocAccessCoverageOutput(t, partialCaller, "doc", "+grant-and-share", "--node", "n", "--to", "alice,bob", "--url", "https://example.com/doc", "--yes")
	assertFailure(t, partial, err, "partial_success", 1, 1)

	helpers.InitDeps(&smartCoverageCaller{responses: docAccessCoverageResponses(`{"result":[]}`), failAt: map[string]int{}})
	root := newPlatformCoverageRoot()
	root.SetOut(docAccessErrorWriter{})
	root.SetErr(io.Discard)
	root.SetIn(bytes.NewReader(nil))
	root.SetArgs([]string{"doc", "+share", "--to", "alice", "--url", "https://example.com/doc", "--yes"})
	if err := root.Execute(); err == nil || err.Error() != "output failure" {
		t.Fatalf("output failure = %v", err)
	}
}

func TestCrossPlatformCoverageDocGrantAndSharePartialPermissionFailure(t *testing.T) {
	resolve := func(_ *shortcut.RuntimeContext, name string) (contactUser, error) {
		return contactUser{name: name, userID: "user-" + name, openDingTalkID: "open-" + name}, nil
	}
	testseam.Swap(t, &resolveDocPermissionUser, resolve)

	responses := docAccessCoverageResponses(`{"result":[{"userId":"user-bob","roleId":"READER"}]}`)
	caller := &smartCoverageCaller{responses: responses, failAt: map[string]int{"doc/update_permission": 1}}
	err := runDocAccessCoverage(t, caller, "doc", "+grant-and-share", "--node", "n", "--to", "alice,bob", "--url", "https://example.com/doc", "--role", "EDITOR", "--yes")
	if err == nil {
		t.Fatal("partial permission write unexpectedly succeeded")
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "doc_grant_permission_partial_failure" || typed.FailureStage != "update_permission" || typed.ExecutionStarted == nil || !*typed.ExecutionStarted || !typed.RetryableSet || typed.Retryable {
		t.Fatalf("partial permission error = %#v", err)
	}
	if typed.Details["status"] != "partial_success" {
		t.Fatalf("partial permission details = %#v", typed.Details)
	}
	steps, _ := typed.Details["steps"].([]map[string]any)
	if len(steps) != 3 || steps[0]["status"] != "success" || steps[1]["status"] != "failed" || steps[2]["status"] != "not_started" {
		t.Fatalf("partial permission steps = %#v", steps)
	}
	if caller.counts["doc/add_permission"] != 1 || caller.counts["doc/update_permission"] != 1 || caller.counts["chat/send_personal_message"] != 0 {
		t.Fatalf("partial permission calls = %#v", caller.counts)
	}
}

func TestCrossPlatformCoverageDocAccessFailureBoundaries(t *testing.T) {
	present := `{"result":[{"userId":"u1","roleId":"READER"}]}`
	commands := []struct {
		args  []string
		fails []string
	}{
		{[]string{"doc", "+access-grant", "--node", "n", "--to", "张三", "--yes"}, []string{"contact/search_contact_by_key_word", "doc/add_permission"}},
		{[]string{"doc", "+access-change", "--node", "n", "--to", "张三", "--yes"}, []string{"contact/search_contact_by_key_word", "doc/list_permission", "doc/update_permission"}},
		{[]string{"doc", "+access-revoke", "--node", "n", "--to", "张三", "--yes"}, []string{"contact/search_contact_by_key_word", "doc/list_permission", "doc/remove_permission"}},
		{[]string{"doc", "+share", "--to", "张三", "--url", "https://example.com/doc", "--yes"}, []string{"contact/search_contact_by_key_word", "chat/send_personal_message"}},
		{[]string{"doc", "+grant-and-share", "--node", "n", "--to", "张三", "--url", "https://example.com/doc", "--yes"}, []string{"contact/search_contact_by_key_word", "doc/list_permission", "chat/send_personal_message"}},
	}
	for _, command := range commands {
		for _, fail := range command.fails {
			responses := docAccessCoverageResponses(present)
			if fail == "doc/add_permission" {
				responses["doc/list_permission"] = []string{`{"result":[]}`}
			}
			caller := &smartCoverageCaller{responses: responses, failAt: map[string]int{fail: 1}}
			_ = runDocAccessCoverage(t, caller, command.args...)
		}
	}

	external := &smartCoverageCaller{responses: docAccessCoverageResponses(present), failAt: map[string]int{}}
	external.responses["contact/search_contact_by_key_word"] = []string{`{"result":[{"name":"外部","openDingTalkId":"open-ext"}]}`}
	_ = runDocAccessCoverage(t, external, "doc", "+access-grant", "--node", "n", "--to", "外部", "--yes")
	noOpenID := &smartCoverageCaller{responses: docAccessCoverageResponses(present), failAt: map[string]int{}}
	noOpenID.responses["contact/search_contact_by_key_word"] = []string{`{"result":[{"name":"张三","userId":"u1"}]}`}
	_ = runDocAccessCoverage(t, noOpenID, "doc", "+grant-and-share", "--node", "n", "--to", "张三", "--url", "https://example.com/doc", "--yes")
	emptyName := &smartCoverageCaller{responses: docAccessCoverageResponses(present), failAt: map[string]int{}}
	_ = runDocAccessCoverage(t, emptyName, "doc", "+access-grant", "--node", "n", "--to", " ", "--yes")
	duplicate := &smartCoverageCaller{responses: docAccessCoverageResponses(present), failAt: map[string]int{}}
	_ = runDocAccessCoverage(t, duplicate, "doc", "+access-grant", "--node", "n", "--to", "张三,张三", "--yes")
	_ = usersMissingPermission(map[string]any{}, []contactUser{{name: "empty"}})

	optionalTo := AccessGrant
	optionalTo.Flags = append([]shortcut.Flag(nil), AccessGrant.Flags...)
	optionalTo.Flags[1].Required = false
	_ = runDocAccessDeclaration(t, optionalTo, &smartCoverageCaller{responses: docAccessCoverageResponses(present), failAt: map[string]int{}}, "--node", "n", "--to", " ", "--yes")
	_ = runDocAccessDeclaration(t, optionalTo, &smartCoverageCaller{responses: docAccessCoverageResponses(present), failAt: map[string]int{}}, "--node", "n", "--yes")

	t.Run("permission resolver defensive identity check", func(t *testing.T) {
		testseam.Swap(t, &resolveDocPermissionUser, func(*shortcut.RuntimeContext, string) (contactUser, error) {
			return contactUser{name: "external", openDingTalkID: "open"}, nil
		})
		_ = runDocAccessCoverage(t, &smartCoverageCaller{responses: docAccessCoverageResponses(present), failAt: map[string]int{}}, "doc", "+access-grant", "--node", "n", "--to", "external", "--yes")
	})
	t.Run("permission resolver error", func(t *testing.T) {
		testseam.Swap(t, &resolveDocPermissionUser, func(*shortcut.RuntimeContext, string) (contactUser, error) {
			return contactUser{}, errors.New("resolve")
		})
		_ = runDocAccessCoverage(t, &smartCoverageCaller{responses: docAccessCoverageResponses(present), failAt: map[string]int{}}, "doc", "+access-grant", "--node", "n", "--to", "x", "--yes")
	})

	grantShareAddFailure := &smartCoverageCaller{responses: docAccessCoverageResponses(`{"result":[]}`), failAt: map[string]int{"doc/add_permission": 1}}
	_ = runDocAccessCoverage(t, grantShareAddFailure, "doc", "+grant-and-share", "--node", "n", "--to", "张三", "--url", "https://example.com/doc", "--yes")
	grantShareUpdateFailure := &smartCoverageCaller{responses: docAccessCoverageResponses(`{"result":[{"userId":"u1","roleId":"READER"}]}`), failAt: map[string]int{"doc/update_permission": 1}}
	_ = runDocAccessCoverage(t, grantShareUpdateFailure, "doc", "+grant-and-share", "--node", "n", "--to", "张三", "--url", "https://example.com/doc", "--role", "EDITOR", "--yes")
	if grantShareUpdateFailure.counts["chat/send_personal_message"] != 0 {
		t.Fatalf("message sent after permission upgrade failure: %#v", grantShareUpdateFailure.counts)
	}
}
