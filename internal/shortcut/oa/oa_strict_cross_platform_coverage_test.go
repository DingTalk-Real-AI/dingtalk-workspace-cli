// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package oa

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type oaCoverageCaller struct {
	responses map[string][]string
	history   []string
	arguments []map[string]any
}

func (caller *oaCoverageCaller) CallTool(_ context.Context, _, tool string, arguments map[string]any) (*edition.ToolResult, error) {
	caller.history = append(caller.history, tool)
	caller.arguments = append(caller.arguments, arguments)
	queue := caller.responses[tool]
	if len(queue) == 0 {
		return nil, errors.New("missing OA fake response for " + tool)
	}
	caller.responses[tool] = queue[1:]
	if queue[0] == "__ERROR__" {
		return nil, errors.New("injected OA failure")
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: queue[0]}}}, nil
}

func (*oaCoverageCaller) Format() string { return "json" }
func (*oaCoverageCaller) DryRun() bool   { return false }
func (*oaCoverageCaller) Fields() string { return "" }
func (*oaCoverageCaller) JQ() string     { return "" }

func runOACoverage(t *testing.T, declaration shortcut.Shortcut, caller *oaCoverageCaller, args ...string) (*cobra.Command, error) {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	cmd := corecmd.New(shortcut.FromShortcut(declaration))
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	return cmd, cmd.Execute()
}

func runOAConfirmedCoverage(t *testing.T, caller *oaCoverageCaller, args ...string) error {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	root := &cobra.Command{Use: "dws", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	ctx, _ := output.WithResultStore(context.Background())
	root.SetContext(ctx)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	service := &cobra.Command{Use: "oa"}
	service.AddCommand(corecmd.New(shortcut.FromShortcut(Approve)))
	root.AddCommand(service)
	root.SetArgs(append([]string{"oa", "+approve-by"}, args...))
	return root.Execute()
}

func TestCrossPlatformCoverageOAContractsAreTypedAndUnified(t *testing.T) {
	for _, declaration := range []shortcut.Shortcut{
		ListPending, ListForms, SearchForms, ListExecuted, ListSubmitted,
		ListCc, PendingApprovals, DoneApprovals, Approve, MyInitiated,
	} {
		if declaration.Contract.Empty() || declaration.Contract.Result == nil {
			t.Errorf("%s lacks Contract.Result", declaration.Command)
		}
		if declaration.Safety.Effect == "" || declaration.Safety.Confirmation == "" {
			t.Errorf("%s lacks Safety", declaration.Command)
		}
		if declaration.OutputRollout != output.RolloutUnifiedActive {
			t.Errorf("%s rollout=%q", declaration.Command, declaration.OutputRollout)
		}
	}
	if Approve.Safety.Confirmation != "user_required" || Approve.Risk != shortcut.RiskHighWrite {
		t.Fatal("+approve-by must require explicit high-risk confirmation")
	}
	unavailable := oaContract("+fixture-unavailable", "fixture unavailable", "reviewed fixture is unavailable", false, oaCollectionResult("items", "fixture items"), nil, nil)
	if unavailable.Interface == nil || unavailable.Interface.Availability != "unavailable" || unavailable.Interface.Reason != "reviewed fixture is unavailable" {
		t.Fatalf("unavailable OA contract interface=%+v", unavailable.Interface)
	}
	for _, declaration := range []shortcut.Shortcut{ListPending, ListForms, ListExecuted, ListSubmitted, ListCc, MyInitiated} {
		if declaration.Contract.Pagination == nil {
			t.Errorf("%s lacks Pagination", declaration.Command)
		}
	}
}

func TestCrossPlatformCoverageOAStrictResponseMatrix(t *testing.T) {
	validEmpty := map[string]any{"success": "true", "result": map[string]any{"values": []any{}}}
	items, err := oaProjectInstances(validEmpty, "oa/test", "result.values")
	if err != nil || len(items) != 0 {
		t.Fatalf("explicit empty must succeed: items=%v err=%v", items, err)
	}
	for name, fixture := range map[string]map[string]any{
		"empty":              {},
		"missing success":    {"result": map[string]any{"values": []any{}}},
		"false success":      {"success": false, "result": map[string]any{"values": []any{}}},
		"missing collection": {"success": true, "result": map[string]any{}},
		"wrong collection":   {"success": true, "result": map[string]any{"values": map[string]any{}}},
		"bad item":           {"success": true, "result": map[string]any{"values": []any{"bad"}}},
		"missing identity":   {"success": true, "result": map[string]any{"values": []any{map[string]any{"title": "fixture"}}}},
	} {
		if projected, err := oaProjectInstances(fixture, "oa/test", "result.values"); err == nil {
			t.Errorf("%s returned success: %#v", name, projected)
		}
	}
	forms := map[string]any{"success": true, "result": map[string]any{"processCodeList": []any{map[string]any{"processCode": "p", "processName": "fixture"}}}}
	if projected, err := oaProjectForms(forms, "oa/forms", "result.processCodeList"); err != nil || len(projected) != 1 {
		t.Fatalf("strict forms projection=%#v err=%v", projected, err)
	}
	badForm := map[string]any{"success": true, "result": map[string]any{"processCodeList": []any{map[string]any{"processName": "fixture"}}}}
	if projected, err := oaProjectForms(badForm, "oa/forms", "result.processCodeList"); err == nil {
		t.Fatalf("form without processCode returned success: %#v", projected)
	}
	search := map[string]any{"success": true, "result": []any{map[string]any{"processCode": "p", "processName": "fixture"}}}
	if projected, err := oaProjectForms(search, "oa/search", "result"); err != nil || len(projected) != 1 {
		t.Fatalf("strict search projection=%#v err=%v", projected, err)
	}
}

func TestCrossPlatformCoverageOAPaginationFailsClosed(t *testing.T) {
	if _, err := oaHasMorePage(map[string]any{}, "oa/page", 1); err == nil {
		t.Fatal("numbered page without hasMore was accepted")
	}
	if _, err := oaHasMorePage(map[string]any{"hasMore": "false"}, "oa/page", 1); err == nil {
		t.Fatal("wrong hasMore type was accepted")
	}
	page, err := oaHasMorePage(map[string]any{"hasMore": true}, "oa/page", 2)
	if err != nil || !page.HasMore || page.Next != "3" {
		t.Fatalf("numbered continuation=%+v err=%v", page, err)
	}
	if _, err := oaCursorPage(map[string]any{}, "oa/cursor", 0); err == nil {
		t.Fatal("cursor response without pagination was accepted")
	}
	if _, err := oaCursorPage(map[string]any{"hasMore": true}, "oa/cursor", 0); err == nil {
		t.Fatal("hasMore without nextCursor was accepted")
	}
	if _, err := oaCursorPage(map[string]any{"hasMore": true, "nextCursor": float64(1)}, "oa/cursor", 1); err == nil {
		t.Fatal("stalled cursor was accepted")
	}
	if _, err := oaCursorPage(map[string]any{"hasMore": true, "nextCursor": float64(1)}, "oa/cursor", 2); err == nil {
		t.Fatal("backward cursor was accepted")
	}
	if _, err := oaCursorPage(map[string]any{"hasMore": true, "nextCursor": "not-a-number"}, "oa/cursor", 1); err == nil {
		t.Fatal("non-integer cursor was accepted")
	}
	page, err = oaCursorPage(map[string]any{"hasMore": true, "nextCursor": float64(2)}, "oa/cursor", 1)
	if err != nil || page.Next != "2" {
		t.Fatalf("cursor continuation=%+v err=%v", page, err)
	}
}

func TestCrossPlatformCoverageMyInitiatedNeverFallsBackToRawResponse(t *testing.T) {
	caller := &oaCoverageCaller{responses: map[string][]string{
		"get_submitted_instances": {`{"success":"true","result":{"values":[],"hasMore":false}}`},
	}}
	cmd, err := runOACoverage(t, MyInitiated, caller, "--page", "1", "--limit", "20")
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	code, emitted, err := output.EmitStoredResult(cmd)
	if err != nil || !emitted || code != 0 {
		t.Fatalf("emit code=%d emitted=%v err=%v", code, emitted, err)
	}
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if _, raw := envelope.Data["result"]; raw {
		t.Fatalf("raw result leaked: %s", stdout.String())
	}
	initiated, ok := envelope.Data["initiated"].([]any)
	if !ok || len(initiated) != 0 || envelope.Data["complete"] != true {
		t.Fatalf("strict empty initiated payload=%#v", envelope.Data)
	}
}

func TestCrossPlatformCoverageOAApproveConfirmationAndReadback(t *testing.T) {
	unconfirmed := &oaCoverageCaller{responses: map[string][]string{}}
	helpers.InitDepsForTest(t, unconfirmed)
	root := &cobra.Command{Use: "dws", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	ctx, _ := output.WithResultStore(context.Background())
	root.SetContext(ctx)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	service := &cobra.Command{Use: "oa"}
	service.AddCommand(corecmd.New(shortcut.FromShortcut(Approve)))
	root.AddCommand(service)
	root.SetArgs([]string{"oa", "+approve-by", "--keyword", "fixture"})
	if err := root.Execute(); err == nil {
		t.Fatal("unconfirmed approval unexpectedly succeeded")
	}
	if len(unconfirmed.history) != 0 {
		t.Fatalf("unconfirmed approval made calls: %v", unconfirmed.history)
	}

	confirmed := &oaCoverageCaller{responses: map[string][]string{
		"list_pending_approvals": {`{"success":true,"result":{"hasMore":false,"values":[{"processInstanceId":"instance-1","title":"fixture"}]}}`},
		"list_pending_tasks": {
			`{"success":true,"result":{"taskIdList":[{"taskId":1}]}}`,
			`{"success":true,"result":{"taskIdList":[]}}`,
		},
		"approve_processInstance": {`{"success":true,"result":{"accepted":true}}`},
	}}
	helpers.InitDepsForTest(t, confirmed)
	confirmedRoot := &cobra.Command{Use: "dws", SilenceUsage: true, SilenceErrors: true}
	confirmedRoot.PersistentFlags().Bool("yes", false, "")
	confirmedRoot.PersistentFlags().Bool("dry-run", false, "")
	confirmedRoot.PersistentFlags().String("format", "json", "")
	confirmedCtx, _ := output.WithResultStore(context.Background())
	confirmedRoot.SetContext(confirmedCtx)
	confirmedRoot.SetOut(io.Discard)
	confirmedRoot.SetErr(io.Discard)
	confirmedService := &cobra.Command{Use: "oa"}
	confirmedService.AddCommand(corecmd.New(shortcut.FromShortcut(Approve)))
	confirmedRoot.AddCommand(confirmedService)
	confirmedRoot.SetArgs([]string{"oa", "+approve-by", "--keyword", "fixture", "--yes"})
	if err := confirmedRoot.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(confirmed.history, ","); got != "list_pending_approvals,list_pending_tasks,approve_processInstance,list_pending_tasks" {
		t.Fatalf("call history=%s", got)
	}
	if len(confirmed.arguments) != 4 || confirmed.arguments[2]["processInstanceId"] != "instance-1" || confirmed.arguments[3]["processInstanceId"] != "instance-1" {
		t.Fatalf("identity was not preserved: %#v", confirmed.arguments)
	}
}

func TestCrossPlatformCoverageOACommonHelperBranches(t *testing.T) {
	if oaObjectResult("fixture") == nil || oaPostWriteError("oa/test", "fixture", "fixture") == nil {
		t.Fatal("typed object result and post-write error must be constructed")
	}

	for name, fixture := range map[string]map[string]any{
		"false with backend message": {"success": false, "errorMessage": "fixture"},
		"false without message":      {"success": false},
		"null success":               {"success": nil},
		"wrong success string":       {"success": "TRUE"},
	} {
		if err := oaRequireSuccess(fixture, "oa/test"); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}

	if _, ok := oaLookup(map[string]any{"result": "bad"}, "result.values"); ok {
		t.Fatal("lookup traversed a non-object")
	}
	if _, ok := oaLookup(map[string]any{"result": map[string]any{}}, "result.values"); ok {
		t.Fatal("lookup accepted a missing segment")
	}
	for name, fixture := range map[string]map[string]any{
		"nil object":   {"success": true, "result": nil},
		"wrong object": {"success": true, "result": "bad"},
		"empty object": {"success": true, "result": map[string]any{}},
	} {
		if value, err := oaRequireObject(fixture, "oa/test", "result"); err == nil {
			t.Errorf("%s returned object %#v", name, value)
		}
	}
	if value, err := oaRequireObject(map[string]any{"success": false}, "oa/test", "result"); err == nil {
		t.Fatalf("failed envelope returned object %#v", value)
	}
	object, err := oaRequireObject(map[string]any{"success": true, "result": map[string]any{"id": "fixture"}}, "oa/test", "result")
	if err != nil || object["id"] != "fixture" {
		t.Fatalf("valid object=%#v err=%v", object, err)
	}

	for name, value := range map[string]any{
		"string":      " fixture ",
		"json number": json.Number("42"),
		"float":       float64(42),
		"int":         int(42),
		"int64":       int64(42),
		"unsupported": true,
	} {
		got := oaScalarString(value)
		if name == "unsupported" && got != "" {
			t.Errorf("unsupported scalar=%q", got)
		}
		if name != "unsupported" && got == "" {
			t.Errorf("%s scalar was empty", name)
		}
	}

	identityFixture := map[string]any{"id": int64(42)}
	if err := oaRequireIdentity(identityFixture, "oa/test", "42", "id"); err != nil {
		t.Fatal(err)
	}
	if err := oaRequireIdentity(map[string]any{}, "oa/test", "", "id"); err == nil {
		t.Fatal("missing identity was accepted")
	}
	if err := oaRequireIdentity(identityFixture, "oa/test", "43", "id"); err == nil {
		t.Fatal("mismatched identity was accepted")
	}

	formsFixture := map[string]any{"success": true, "result": []any{map[string]any{
		"processCode": "p", "name": "fixture", "iconUrl": "icon",
	}}}
	forms, err := oaProjectForms(formsFixture, "oa/forms", "result")
	if err != nil || forms[0]["iconUrl"] != "icon" {
		t.Fatalf("fallback form fields=%#v err=%v", forms, err)
	}

	instanceFixture := map[string]any{"success": true, "result": map[string]any{"values": []any{map[string]any{
		"processInstanceId": "i", "processInstanceTitle": "title", "status": json.Number("2"),
		"businessId": 7, "originatorUserName": "owner", "createTime": int64(9),
	}}}}
	instances, err := oaProjectInstances(instanceFixture, "oa/instances", "result.values")
	if err != nil {
		t.Fatal(err)
	}
	wantInstance := map[string]any{"processInstanceId": "i", "title": "title", "status": "2", "businessId": "7", "originatorName": "owner", "createTime": int64(9)}
	if !reflect.DeepEqual(instances[0], wantInstance) {
		t.Fatalf("projected instance=%#v want=%#v", instances[0], wantInstance)
	}

	for name, fixture := range map[string]map[string]any{
		"task projection error": {"success": true, "result": map[string]any{}},
		"missing task id":       {"success": true, "result": map[string]any{"taskIdList": []any{map[string]any{"x": 1}}}},
		"duplicate task id": {"success": true, "result": map[string]any{"taskIdList": []any{
			map[string]any{"taskId": 1}, map[string]any{"taskId": 1},
		}}},
	} {
		if tasks, err := oaProjectTasks(fixture, "oa/tasks"); err == nil {
			t.Errorf("%s returned %#v", name, tasks)
		}
	}

	for name, fixture := range map[string]map[string]any{
		"wrong cursor hasMore": {"hasMore": "true"},
		"terminal conflict":    {"hasMore": false, "nextCursor": 1},
	} {
		if page, err := oaCursorPage(fixture, "oa/cursor", 0); err == nil {
			t.Errorf("%s returned %+v", name, page)
		}
	}
	terminal, err := oaCursorPage(map[string]any{"hasMore": false}, "oa/cursor", 0)
	if err != nil || !terminal.Known || terminal.HasMore || terminal.Next != "" {
		t.Fatalf("terminal cursor=%+v err=%v", terminal, err)
	}
	if _, err := oaHasMorePage(map[string]any{"hasMore": true}, "oa/page", 0); err == nil {
		t.Fatal("continuation from invalid current page was accepted")
	}
	terminal, err = oaHasMorePage(map[string]any{"hasMore": false}, "oa/page", 1)
	if err != nil || !terminal.Known || terminal.HasMore {
		t.Fatalf("terminal numbered page=%+v err=%v", terminal, err)
	}

	if err := validateOAPage(0, 1); err == nil {
		t.Fatal("zero page was accepted")
	}
	if err := validateOAPage(1, 0); err == nil {
		t.Fatal("zero limit was accepted")
	}
	if err := validateOAPage(1, 101); err == nil {
		t.Fatal("oversized limit was accepted")
	}
}

func TestCrossPlatformCoverageOAOutputBranches(t *testing.T) {
	legacyPage := shortcut.Shortcut{
		Service: "oa", Command: "+coverage-legacy-page", Product: "oa", Description: "fixture", Risk: shortcut.RiskRead,
		Execute: func(rt *shortcut.RuntimeContext) error {
			return outputOAPage(rt, "items", []map[string]any{{"id": "1"}}, oaPageEvidence{Known: true, HasMore: true, Next: "2"})
		},
	}
	_, err := runOACoverage(t, legacyPage, &oaCoverageCaller{responses: map[string][]string{}})
	if err != nil {
		t.Fatal(err)
	}
	unknownPage := legacyPage
	unknownPage.Command = "+coverage-unknown-page"
	unknownPage.Execute = func(rt *shortcut.RuntimeContext) error {
		return outputOAPage(rt, "items", nil, oaPageEvidence{})
	}
	if _, err := runOACoverage(t, unknownPage, &oaCoverageCaller{responses: map[string][]string{}}); err == nil {
		t.Fatal("unknown pagination was accepted")
	}

	legacyComplete := shortcut.Shortcut{
		Service: "oa", Command: "+coverage-legacy-complete", Product: "oa", Description: "fixture", Risk: shortcut.RiskRead,
		Execute: func(rt *shortcut.RuntimeContext) error { return outputOACompleteCollection(rt, "items", nil) },
	}
	if _, err := runOACoverage(t, legacyComplete, &oaCoverageCaller{responses: map[string][]string{}}); err != nil {
		t.Fatal(err)
	}

	unifiedComplete := legacyComplete
	unifiedComplete.Command = "+coverage-unified-complete"
	unifiedComplete.OutputRollout = output.RolloutUnifiedActive
	if _, err := runOACoverage(t, unifiedComplete, &oaCoverageCaller{responses: map[string][]string{}}); err != nil {
		t.Fatal(err)
	}

	badUnifiedPage := legacyPage
	badUnifiedPage.Command = "+coverage-unified-bad-page"
	badUnifiedPage.OutputRollout = output.RolloutUnifiedActive
	badUnifiedPage.Execute = func(rt *shortcut.RuntimeContext) error {
		return outputOAPage(rt, "items", nil, oaPageEvidence{Known: true, HasMore: true})
	}
	if _, err := runOACoverage(t, badUnifiedPage, &oaCoverageCaller{responses: map[string][]string{}}); err == nil {
		t.Fatal("unified continuation without token was accepted")
	}
}

func TestCrossPlatformCoverageOAReadShortcutBranches(t *testing.T) {
	expectError := func(t *testing.T, declaration shortcut.Shortcut, responses map[string][]string, args ...string) *oaCoverageCaller {
		t.Helper()
		caller := &oaCoverageCaller{responses: responses}
		if _, err := runOACoverage(t, declaration, caller, args...); err == nil {
			t.Fatalf("%s unexpectedly succeeded with args %v", declaration.Command, args)
		}
		return caller
	}
	expectSuccess := func(t *testing.T, declaration shortcut.Shortcut, responses map[string][]string, args ...string) *oaCoverageCaller {
		t.Helper()
		caller := &oaCoverageCaller{responses: responses}
		if _, err := runOACoverage(t, declaration, caller, args...); err != nil {
			t.Fatalf("%s failed with args %v: %v", declaration.Command, args, err)
		}
		return caller
	}

	validPage := `{"success":true,"result":{"hasMore":false,"values":[{"processInstanceId":"i","title":"fixture"}]}}`
	for name, args := range map[string][]string{
		"bad interval": {"--start", "2", "--end", "1"},
		"bad page":     {"--start", "1", "--end", "2", "--page", "bad"},
		"bad limit":    {"--start", "1", "--end", "2", "--limit", "bad"},
		"zero page":    {"--start", "1", "--end", "2", "--page", "0"},
	} {
		t.Run("list-pending validate "+name, func(t *testing.T) {
			caller := expectError(t, ListPending, map[string][]string{}, args...)
			if len(caller.history) != 0 {
				t.Fatalf("validation made calls: %v", caller.history)
			}
		})
	}
	t.Run("list-pending call failure", func(t *testing.T) {
		expectError(t, ListPending, map[string][]string{"list_pending_approvals": {"__ERROR__"}}, "--start", "1", "--end", "2")
	})
	t.Run("list-pending projection failure", func(t *testing.T) {
		expectError(t, ListPending, map[string][]string{"list_pending_approvals": {`{"success":true,"result":{"hasMore":false}}`}}, "--start", "1", "--end", "2")
	})
	t.Run("list-pending null result fails closed", func(t *testing.T) {
		expectError(t, ListPending, map[string][]string{"list_pending_approvals": {`{"success":true,"result":null}`}}, "--start", "1", "--end", "2")
	})
	t.Run("list-pending pagination failure", func(t *testing.T) {
		expectError(t, ListPending, map[string][]string{"list_pending_approvals": {`{"success":true,"result":{"values":[]}}`}}, "--start", "1", "--end", "2")
	})
	t.Run("list-pending full params", func(t *testing.T) {
		caller := expectSuccess(t, ListPending, map[string][]string{"list_pending_approvals": {validPage}},
			"--start", "1", "--end", "2", "--page", "2", "--limit", "3", "--query", "fixture")
		want := map[string]any{"starTime": 1, "endTime": 2, "pageNum": "2", "pageSize": "3", "query": "fixture"}
		if !reflect.DeepEqual(caller.arguments[0], want) {
			t.Fatalf("list pending params=%#v want=%#v", caller.arguments[0], want)
		}
	})

	for name, args := range map[string][]string{
		"negative cursor": {"--cursor", "-1"},
		"zero limit":      {"--limit", "0"},
		"oversized limit": {"--limit", "101"},
	} {
		t.Run("list-forms validate "+name, func(t *testing.T) {
			expectError(t, ListForms, map[string][]string{}, args...)
		})
	}
	t.Run("list-forms call failure", func(t *testing.T) {
		expectError(t, ListForms, map[string][]string{"list_user_visible_process": {"__ERROR__"}})
	})
	t.Run("list-forms projection failure", func(t *testing.T) {
		expectError(t, ListForms, map[string][]string{"list_user_visible_process": {`{"success":true,"result":{"hasMore":false}}`}})
	})
	t.Run("list-forms null result fails closed", func(t *testing.T) {
		expectError(t, ListForms, map[string][]string{"list_user_visible_process": {`{"success":true,"result":null}`}})
	})
	t.Run("list-forms pagination failure", func(t *testing.T) {
		expectError(t, ListForms, map[string][]string{"list_user_visible_process": {`{"success":true,"result":{"processCodeList":[]}}`}})
	})
	t.Run("list-forms success", func(t *testing.T) {
		expectSuccess(t, ListForms, map[string][]string{"list_user_visible_process": {`{"success":true,"result":{"hasMore":true,"nextCursor":2,"processCodeList":[{"processCode":"p","processName":"fixture"}]}}`}}, "--cursor", "1", "--limit", "2")
	})

	t.Run("search empty query", func(t *testing.T) {
		caller := expectError(t, SearchForms, map[string][]string{}, "--query", " ")
		if len(caller.history) != 0 {
			t.Fatalf("blank query made calls: %v", caller.history)
		}
	})
	t.Run("search call failure", func(t *testing.T) {
		expectError(t, SearchForms, map[string][]string{"search_form": {"__ERROR__"}}, "--query", "fixture")
	})
	t.Run("search projection failure", func(t *testing.T) {
		expectError(t, SearchForms, map[string][]string{"search_form": {`{"success":true,"result":{}}`}}, "--query", "fixture")
	})
	t.Run("search known nonempty", func(t *testing.T) {
		caller := expectSuccess(t, SearchForms, map[string][]string{"search_form": {`{"success":true,"result":[{"processCode":"p","processName":"fixture"}]}`}}, "--query", " fixture ")
		if got := caller.arguments[0]["query"]; got != "fixture" {
			t.Fatalf("normalized query=%#v, want fixture", got)
		}
	})
	t.Run("search legitimate empty", func(t *testing.T) {
		expectSuccess(t, SearchForms, map[string][]string{"search_form": {`{"success":true,"result":[]}`}}, "--query", "guaranteed-zero-fixture")
	})

	for _, declaration := range []shortcut.Shortcut{ListExecuted, ListSubmitted, ListCc} {
		declaration := declaration
		tool := map[string]string{"+list-executed": "get_done_tasks", "+list-submitted": "get_submitted_instances", "+list-cc": "get_noticed_instances"}[declaration.Command]
		t.Run(declaration.Command+" invalid page", func(t *testing.T) {
			expectError(t, declaration, map[string][]string{}, "--page", "bad")
		})
		t.Run(declaration.Command+" invalid limit", func(t *testing.T) {
			expectError(t, declaration, map[string][]string{}, "--limit", "bad")
		})
		t.Run(declaration.Command+" call failure", func(t *testing.T) {
			expectError(t, declaration, map[string][]string{tool: {"__ERROR__"}})
		})
		t.Run(declaration.Command+" full params", func(t *testing.T) {
			caller := expectSuccess(t, declaration, map[string][]string{tool: {validPage}}, "--page", "2", "--limit", "3", "--query", " fixture ")
			want := map[string]any{"pageNumber": "2", "pageSize": "3", "query": "fixture"}
			if !reflect.DeepEqual(caller.arguments[0], want) {
				t.Fatalf("params=%#v want=%#v", caller.arguments[0], want)
			}
		})
	}
}

func TestCrossPlatformCoverageOACompatibilityReadBranches(t *testing.T) {
	expectError := func(t *testing.T, declaration shortcut.Shortcut, responses map[string][]string, args ...string) *oaCoverageCaller {
		t.Helper()
		caller := &oaCoverageCaller{responses: responses}
		if _, err := runOACoverage(t, declaration, caller, args...); err == nil {
			t.Fatalf("%s unexpectedly succeeded with args %v", declaration.Command, args)
		}
		return caller
	}
	expectSuccess := func(t *testing.T, declaration shortcut.Shortcut, responses map[string][]string, args ...string) *oaCoverageCaller {
		t.Helper()
		caller := &oaCoverageCaller{responses: responses}
		if _, err := runOACoverage(t, declaration, caller, args...); err != nil {
			t.Fatalf("%s failed with args %v: %v", declaration.Command, args, err)
		}
		return caller
	}
	terminal := `{"success":true,"result":{"hasMore":false,"values":[{"processInstanceId":"i","title":"fixture"}]}}`

	for _, declaration := range []shortcut.Shortcut{PendingApprovals, DoneApprovals} {
		declaration := declaration
		tool := map[string]string{"+pending": "list_pending_approvals", "+done-approvals": "get_done_tasks"}[declaration.Command]
		t.Run(declaration.Command+" invalid limit", func(t *testing.T) {
			caller := expectError(t, declaration, map[string][]string{}, "--limit", "0")
			if len(caller.history) != 0 {
				t.Fatalf("validation made calls: %v", caller.history)
			}
		})
		t.Run(declaration.Command+" call failure", func(t *testing.T) {
			expectError(t, declaration, map[string][]string{tool: {"__ERROR__"}})
		})
		t.Run(declaration.Command+" projection failure", func(t *testing.T) {
			expectError(t, declaration, map[string][]string{tool: {`{"success":true,"result":{"hasMore":false}}`}})
		})
		t.Run(declaration.Command+" null result fails closed", func(t *testing.T) {
			expectError(t, declaration, map[string][]string{tool: {`{"success":true,"result":null}`}})
		})
		t.Run(declaration.Command+" pagination failure", func(t *testing.T) {
			expectError(t, declaration, map[string][]string{tool: {`{"success":true,"result":{"values":[]}}`}})
		})
		t.Run(declaration.Command+" refuses incomplete first page", func(t *testing.T) {
			expectError(t, declaration, map[string][]string{tool: {`{"success":true,"result":{"hasMore":true,"values":[]}}`}})
		})
		t.Run(declaration.Command+" default limit", func(t *testing.T) {
			expectSuccess(t, declaration, map[string][]string{tool: {terminal}})
		})
		t.Run(declaration.Command+" explicit limit", func(t *testing.T) {
			caller := expectSuccess(t, declaration, map[string][]string{tool: {terminal}}, "--limit", "3")
			if caller.arguments[0]["pageSize"] != float64(3) {
				t.Fatalf("explicit limit params=%#v", caller.arguments[0])
			}
		})
	}

	for name, args := range map[string][]string{
		"bad page":  {"--page", "0"},
		"bad limit": {"--limit", "101"},
	} {
		t.Run("my-initiated validate "+name, func(t *testing.T) {
			expectError(t, MyInitiated, map[string][]string{}, args...)
		})
	}
	t.Run("my-initiated call failure", func(t *testing.T) {
		expectError(t, MyInitiated, map[string][]string{"get_submitted_instances": {"__ERROR__"}})
	})
	t.Run("my-initiated projection failure", func(t *testing.T) {
		expectError(t, MyInitiated, map[string][]string{"get_submitted_instances": {`{"success":true,"result":{"hasMore":false}}`}})
	})
	t.Run("my-initiated null result fails closed", func(t *testing.T) {
		expectError(t, MyInitiated, map[string][]string{"get_submitted_instances": {`{"success":true,"result":null}`}})
	})
	t.Run("my-initiated pagination failure", func(t *testing.T) {
		expectError(t, MyInitiated, map[string][]string{"get_submitted_instances": {`{"success":true,"result":{"values":[]}}`}})
	})
	t.Run("my-initiated query and continuation", func(t *testing.T) {
		caller := expectSuccess(t, MyInitiated, map[string][]string{"get_submitted_instances": {`{"success":true,"result":{"hasMore":true,"values":[]}}`}}, "--query", "fixture", "--page", "2", "--limit", "3")
		want := map[string]any{"pageNumber": float64(2), "pageSize": float64(3), "query": "fixture"}
		if !reflect.DeepEqual(caller.arguments[0], want) {
			t.Fatalf("my initiated params=%#v want=%#v", caller.arguments[0], want)
		}
	})
}

func TestCrossPlatformCoverageOARequiredFlagValidationClosures(t *testing.T) {
	for _, tc := range []struct {
		name     string
		flagName string
		validate func(*shortcut.RuntimeContext) error
	}{
		{name: "search forms", flagName: "query", validate: SearchForms.Validate},
		{name: "approve", flagName: "keyword", validate: Approve.Validate},
	} {
		t.Run(tc.name, func(t *testing.T) {
			declaration := shortcut.Shortcut{
				Service: "oa", Command: "+coverage-" + strings.ReplaceAll(tc.name, " ", "-"), Product: "oa", Description: "fixture", Risk: shortcut.RiskRead,
				Flags:   []shortcut.Flag{{Name: tc.flagName, Type: shortcut.FlagString}},
				Execute: tc.validate,
			}
			caller := &oaCoverageCaller{responses: map[string][]string{}}
			if _, err := runOACoverage(t, declaration, caller); err == nil {
				t.Fatal("empty required semantic value was accepted")
			}
			if len(caller.history) != 0 {
				t.Fatalf("validation made calls: %v", caller.history)
			}
		})
	}
}

func TestCrossPlatformCoverageOAApproveFailureLedger(t *testing.T) {
	pending := func(values string, hasMore bool) string {
		return `{"success":true,"result":{"hasMore":` + map[bool]string{true: "true", false: "false"}[hasMore] + `,"values":` + values + `}}`
	}
	tasks := func(values string) string {
		return `{"success":true,"result":{"taskIdList":` + values + `}}`
	}
	validPending := pending(`[{"processInstanceId":"instance-1","title":"fixture"}]`, false)
	validTasks := tasks(`[{"taskId":1}]`)

	cases := []struct {
		name      string
		responses map[string][]string
	}{
		{name: "pending call failure", responses: map[string][]string{"list_pending_approvals": {"__ERROR__"}}},
		{name: "pending projection failure", responses: map[string][]string{"list_pending_approvals": {`{"success":true,"result":{"hasMore":false}}`}}},
		{name: "pending null result", responses: map[string][]string{"list_pending_approvals": {`{"success":true,"result":null}`}}},
		{name: "pending pagination failure", responses: map[string][]string{"list_pending_approvals": {`{"success":true,"result":{"values":[]}}`}}},
		{name: "pending is incomplete", responses: map[string][]string{"list_pending_approvals": {pending(`[{"processInstanceId":"instance-1","title":"fixture"}]`, true)}}},
		{name: "zero matches", responses: map[string][]string{"list_pending_approvals": {pending(`[{"processInstanceId":"instance-1","title":"other"}]`, false)}}},
		{name: "task call failure", responses: map[string][]string{"list_pending_approvals": {validPending}, "list_pending_tasks": {"__ERROR__"}}},
		{name: "task projection failure", responses: map[string][]string{"list_pending_approvals": {validPending}, "list_pending_tasks": {`{"success":true,"result":{}}`}}},
		{name: "task count is not one", responses: map[string][]string{"list_pending_approvals": {validPending}, "list_pending_tasks": {tasks(`[]`)}}},
		{name: "task identity is not numeric", responses: map[string][]string{"list_pending_approvals": {validPending}, "list_pending_tasks": {tasks(`[{"taskId":"not-numeric"}]`)}}},
		{name: "write call failure", responses: map[string][]string{"list_pending_approvals": {validPending}, "list_pending_tasks": {validTasks}, "approve_processInstance": {"__ERROR__"}}},
		{name: "write business failure", responses: map[string][]string{"list_pending_approvals": {validPending}, "list_pending_tasks": {validTasks}, "approve_processInstance": {`{"success":false}`}}},
		{name: "write missing success", responses: map[string][]string{"list_pending_approvals": {validPending}, "list_pending_tasks": {validTasks}, "approve_processInstance": {`{"result":{"accepted":true}}`}}},
		{name: "write null success", responses: map[string][]string{"list_pending_approvals": {validPending}, "list_pending_tasks": {validTasks}, "approve_processInstance": {`{"success":null}`}}},
		{name: "readback call failure", responses: map[string][]string{"list_pending_approvals": {validPending}, "list_pending_tasks": {validTasks, "__ERROR__"}, "approve_processInstance": {`{"success":true}`}}},
		{name: "readback malformed", responses: map[string][]string{"list_pending_approvals": {validPending}, "list_pending_tasks": {validTasks, `{"success":true,"result":{}}`}, "approve_processInstance": {`{"success":true}`}}},
		{name: "write not observed", responses: map[string][]string{"list_pending_approvals": {validPending}, "list_pending_tasks": {validTasks, validTasks}, "approve_processInstance": {`{"success":true}`}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &oaCoverageCaller{responses: tc.responses}
			if err := runOAConfirmedCoverage(t, caller, "--keyword", "fixture", "--yes"); err == nil {
				t.Fatalf("failure scenario succeeded; calls=%v", caller.history)
			}
		})
	}

	t.Run("empty keyword validates before calls", func(t *testing.T) {
		caller := &oaCoverageCaller{responses: map[string][]string{}}
		if err := runOAConfirmedCoverage(t, caller, "--keyword", " ", "--yes"); err == nil {
			t.Fatal("blank keyword succeeded")
		}
		if len(caller.history) != 0 {
			t.Fatalf("blank keyword made calls: %v", caller.history)
		}
	})

	t.Run("comment is sent only after confirmation", func(t *testing.T) {
		caller := &oaCoverageCaller{responses: map[string][]string{
			"list_pending_approvals":  {validPending},
			"list_pending_tasks":      {validTasks, tasks(`[]`)},
			"approve_processInstance": {`{"success":true}`},
		}}
		if err := runOAConfirmedCoverage(t, caller, "--keyword", "fixture", "--comment", "approved", "--yes"); err != nil {
			t.Fatal(err)
		}
		if caller.arguments[2]["remark"] != "approved" {
			t.Fatalf("write args=%#v", caller.arguments[2])
		}
	})
}
