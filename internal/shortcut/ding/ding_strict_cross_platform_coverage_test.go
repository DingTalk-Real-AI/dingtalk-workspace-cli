// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package ding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
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

type dingCoverageCaller struct {
	responses map[string][]string
	failures  map[string]error
	history   []string
	arguments []map[string]any
}

func (caller *dingCoverageCaller) CallTool(_ context.Context, _, tool string, arguments map[string]any) (*edition.ToolResult, error) {
	caller.history = append(caller.history, tool)
	caller.arguments = append(caller.arguments, arguments)
	if err := caller.failures[tool]; err != nil {
		return nil, err
	}
	queue := caller.responses[tool]
	if len(queue) == 0 {
		return nil, errors.New("missing DING fake response for " + tool)
	}
	caller.responses[tool] = queue[1:]
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: queue[0]}}}, nil
}

func (*dingCoverageCaller) Format() string { return "json" }
func (*dingCoverageCaller) DryRun() bool   { return false }
func (*dingCoverageCaller) Fields() string { return "" }
func (*dingCoverageCaller) JQ() string     { return "" }

func runDingCoverage(t *testing.T, declaration shortcut.Shortcut, caller *dingCoverageCaller, args ...string) (*cobra.Command, error) {
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

func runDingCoverageOutput(t *testing.T, declaration shortcut.Shortcut, caller *dingCoverageCaller, args ...string) ([]byte, error) {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	cmd := corecmd.New(shortcut.FromShortcut(declaration))
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.Bytes(), err
}

func TestCrossPlatformCoverageDINGContractsAreStrictTypedAndUnified(t *testing.T) {
	for _, declaration := range []shortcut.Shortcut{List, ReceiverStatus, SendPersonal, SendByMessage, RecallPersonal} {
		if declaration.Contract.Empty() {
			t.Errorf("%s lacks Contract", declaration.Command)
		}
		if declaration.Safety.Effect == "" || declaration.Safety.Confirmation == "" {
			t.Errorf("%s lacks Safety", declaration.Command)
		}
		if declaration.OutputRollout != output.RolloutUnifiedActive {
			t.Errorf("%s rollout=%q", declaration.Command, declaration.OutputRollout)
		}
	}
	for _, declaration := range []shortcut.Shortcut{List, ReceiverStatus} {
		if declaration.Contract.Result == nil {
			t.Errorf("%s lacks Result", declaration.Command)
		}
		if declaration.Contract.Interface == nil || declaration.Contract.Interface.Availability != "available" {
			t.Errorf("%s is not interface-available", declaration.Command)
		}
	}
	for _, declaration := range []shortcut.Shortcut{SendPersonal, RecallPersonal} {
		if declaration.Safety.Confirmation != "user_required" || declaration.Contract.Interface == nil || declaration.Contract.Interface.Availability != "available" {
			t.Errorf("%s compatibility write safety/interface drift", declaration.Command)
		}
	}
	if SendByMessage.Safety.Confirmation != "user_required" || SendByMessage.Contract.Interface == nil || SendByMessage.Contract.Interface.Availability != "unavailable" {
		t.Errorf("%s unavailable write safety/interface drift", SendByMessage.Command)
	}
	if List.Contract.Pagination == nil {
		t.Fatal("+list lacks cursor pagination")
	}
	if RecallPersonal.Validate == nil || len(RecallPersonal.Constraints) != 1 ||
		RecallPersonal.Constraints[0].Kind != shortcut.ConstraintCustom ||
		strings.Join(RecallPersonal.Constraints[0].Flags, ",") != "id" ||
		strings.TrimSpace(RecallPersonal.Constraints[0].Description) == "" {
		t.Fatal("recall ID validation must be published as a bound custom constraint")
	}
}

func TestCrossPlatformCoverageDINGListResponseMatrix(t *testing.T) {
	valid := map[string]any{
		"success": true,
		"result": map[string]any{
			"dingMessages": []any{map[string]any{"openDingId": "ding-1", "dingContent": "fixture"}},
			"hasMore":      true,
			"nextCursor":   float64(2),
		},
	}
	messages, page, err := dingProjectMessages(valid, "im/list_ding_messages")
	if err != nil || len(messages) != 1 || !page.HasMore || page.Next != "2" || messages[0]["content"] != "fixture" {
		t.Fatalf("valid list projection messages=%#v page=%+v err=%v", messages, page, err)
	}
	empty := map[string]any{"success": true, "result": map[string]any{"dingMessages": []any{}, "hasMore": false, "nextCursor": nil}}
	if messages, page, err := dingProjectMessages(empty, "im/list_ding_messages"); err != nil || len(messages) != 0 || page.HasMore {
		t.Fatalf("explicit terminal empty page messages=%#v page=%+v err=%v", messages, page, err)
	}
	fixtures := map[string]map[string]any{
		"empty":                  {},
		"missing success":        {"result": map[string]any{"dingMessages": []any{}, "hasMore": false}},
		"wrong success":          {"success": "true", "result": map[string]any{"dingMessages": []any{}, "hasMore": false}},
		"false success":          {"success": false, "errorMsg": "fixture failure", "result": map[string]any{"dingMessages": []any{}, "hasMore": false}},
		"missing result":         {"success": true},
		"missing collection":     {"success": true, "result": map[string]any{"hasMore": false}},
		"wrong collection":       {"success": true, "result": map[string]any{"dingMessages": map[string]any{}, "hasMore": false}},
		"bad item":               {"success": true, "result": map[string]any{"dingMessages": []any{"bad"}, "hasMore": false}},
		"missing item id":        {"success": true, "result": map[string]any{"dingMessages": []any{map[string]any{"dingContent": "fixture"}}, "hasMore": false}},
		"duplicate item id":      {"success": true, "result": map[string]any{"dingMessages": []any{map[string]any{"openDingId": "same"}, map[string]any{"openDingId": "same"}}, "hasMore": false}},
		"missing pagination":     {"success": true, "result": map[string]any{"dingMessages": []any{}}},
		"wrong pagination":       {"success": true, "result": map[string]any{"dingMessages": []any{}, "hasMore": "false"}},
		"empty continuation":     {"success": true, "result": map[string]any{"dingMessages": []any{}, "hasMore": true, "nextCursor": float64(2)}},
		"missing next cursor":    {"success": true, "result": map[string]any{"dingMessages": []any{map[string]any{"openDingId": "ding-1"}}, "hasMore": true}},
		"wrong next cursor":      {"success": true, "result": map[string]any{"dingMessages": []any{map[string]any{"openDingId": "ding-1"}}, "hasMore": true, "nextCursor": "2"}},
		"conflicting terminal":   {"success": true, "result": map[string]any{"dingMessages": []any{}, "hasMore": false, "nextCursor": float64(2)}},
		"wrong optional content": {"success": true, "result": map[string]any{"dingMessages": []any{map[string]any{"openDingId": "ding-1", "dingContent": float64(1)}}, "hasMore": false}},
	}
	for name, fixture := range fixtures {
		if projected, _, projectErr := dingProjectMessages(fixture, "im/list_ding_messages"); projectErr == nil {
			t.Errorf("%s returned success: %#v", name, projected)
		}
	}
	if _, _, err := dingProjectMessages(map[string]any{"success": false, "errorMsg": "fixture failure"}, "im/list_ding_messages"); err == nil || !strings.Contains(err.Error(), "fixture failure") {
		t.Fatalf("remote failure message was not preserved: %v", err)
	}
}

func TestCrossPlatformCoverageDINGIntegerRepresentations(t *testing.T) {
	for name, test := range map[string]struct {
		value any
		want  int64
	}{
		"float64":     {value: float64(1), want: 1},
		"int":         {value: int(2), want: 2},
		"int64":       {value: int64(3), want: 3},
		"json number": {value: json.Number("4"), want: 4},
	} {
		t.Run(name, func(t *testing.T) {
			if got, ok := dingInteger(test.value); !ok || got != test.want {
				t.Fatalf("dingInteger(%T(%v))=(%d,%t), want (%d,true)", test.value, test.value, got, ok, test.want)
			}
		})
	}
	for name, value := range map[string]any{
		"nan":             math.NaN(),
		"fraction":        1.5,
		"overflow":        math.MaxFloat64,
		"bad json number": json.Number("not-an-integer"),
		"wrong type":      "1",
	} {
		t.Run(name, func(t *testing.T) {
			if got, ok := dingInteger(value); ok {
				t.Fatalf("dingInteger(%T(%v))=(%d,true), want invalid", value, value, got)
			}
		})
	}
}

func TestCrossPlatformCoverageDINGReceiverResponseMatrix(t *testing.T) {
	valid := map[string]any{"success": true, "result": map[string]any{"receivers": []any{map[string]any{"openDingId": "ding-1", "confirmedStatus": float64(1), "receiverNick": "fixture"}}}}
	items, err := dingProjectReceivers(valid, "im/list_ding_receiver_status", "ding-1")
	if err != nil || len(items) != 1 || items[0]["confirmedStatus"] != int64(1) {
		t.Fatalf("valid receiver projection=%#v err=%v", items, err)
	}
	for name, fixture := range map[string]map[string]any{
		"empty":              {},
		"missing collection": {"success": true, "result": map[string]any{}},
		"empty collection":   {"success": true, "result": map[string]any{"receivers": []any{}}},
		"bad item":           {"success": true, "result": map[string]any{"receivers": []any{"bad"}}},
		"missing item id":    {"success": true, "result": map[string]any{"receivers": []any{map[string]any{"confirmedStatus": float64(1), "receiverNick": "fixture"}}}},
		"identity mismatch":  {"success": true, "result": map[string]any{"receivers": []any{map[string]any{"openDingId": "other", "confirmedStatus": float64(1), "receiverNick": "fixture"}}}},
		"wrong status":       {"success": true, "result": map[string]any{"receivers": []any{map[string]any{"openDingId": "ding-1", "confirmedStatus": "1", "receiverNick": "fixture"}}}},
		"missing receiver":   {"success": true, "result": map[string]any{"receivers": []any{map[string]any{"openDingId": "ding-1", "confirmedStatus": float64(1)}}}},
	} {
		if projected, projectErr := dingProjectReceivers(fixture, "im/list_ding_receiver_status", "ding-1"); projectErr == nil {
			t.Errorf("%s returned success: %#v", name, projected)
		}
	}
}

func TestCrossPlatformCoverageDINGListExecutionFailuresPaginationAndLegacy(t *testing.T) {
	t.Run("negative cursor validates before call", func(t *testing.T) {
		caller := &dingCoverageCaller{responses: map[string][]string{}}
		if _, err := runDingCoverage(t, List, caller, "--cursor", "-1"); err == nil || len(caller.history) != 0 {
			t.Fatalf("negative cursor error=%v calls=%v", err, caller.history)
		}
	})

	t.Run("transport failure", func(t *testing.T) {
		caller := &dingCoverageCaller{
			responses: map[string][]string{},
			failures:  map[string]error{"list_ding_messages": errors.New("fixture transport failure")},
		}
		if _, err := runDingCoverage(t, List, caller); err == nil || strings.Join(caller.history, ",") != "list_ding_messages" {
			t.Fatalf("transport error=%v calls=%v", err, caller.history)
		}
	})

	t.Run("projection failure", func(t *testing.T) {
		caller := &dingCoverageCaller{responses: map[string][]string{
			"list_ding_messages": {`{"success":true,"result":{"dingMessages":[],"hasMore":true,"nextCursor":2}}`},
		}}
		if _, err := runDingCoverage(t, List, caller); err == nil {
			t.Fatal("malformed empty continuation returned success")
		}
	})

	t.Run("stalled cursor", func(t *testing.T) {
		caller := &dingCoverageCaller{responses: map[string][]string{
			"list_ding_messages": {`{"success":true,"result":{"dingMessages":[{"openDingId":"ding-1"}],"hasMore":true,"nextCursor":2}}`},
		}}
		if _, err := runDingCoverage(t, List, caller, "--cursor", "2"); err == nil || !strings.Contains(err.Error(), "nextCursor") {
			t.Fatalf("stalled cursor error=%v", err)
		}
	})

	t.Run("advancing cursor legacy output", func(t *testing.T) {
		legacy := List
		legacy.OutputRollout = output.RolloutLegacyOnly
		caller := &dingCoverageCaller{responses: map[string][]string{
			"list_ding_messages": {`{"success":true,"result":{"dingMessages":[{"openDingId":"ding-1"}],"hasMore":true,"nextCursor":3}}`},
		}}
		stdout, err := runDingCoverageOutput(t, legacy, caller, "--cursor", "2")
		if err != nil {
			t.Fatalf("legacy advancing page: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(stdout, &payload); err != nil || payload["nextCursor"] != "3" || payload["complete"] != false {
			t.Fatalf("legacy payload=%#v decode=%v", payload, err)
		}
		if len(caller.arguments) != 1 || caller.arguments[0]["cursor"] != 2 {
			t.Fatalf("legacy cursor arguments=%#v", caller.arguments)
		}
	})

	t.Run("invalid unified page evidence", func(t *testing.T) {
		cmd := corecmd.New(shortcut.FromShortcut(List))
		ctx, _ := output.WithResultStore(context.Background())
		cmd.SetContext(ctx)
		rt := shortcut.RuntimeContextForTest(cmd, List)
		if err := outputDingPage(rt, nil, dingPageEvidence{HasMore: true}); err == nil {
			t.Fatal("continuation without next cursor returned success")
		}
	})
}

func TestCrossPlatformCoverageDINGReceiverExecutionFailuresAndLegacy(t *testing.T) {
	t.Run("transport failure", func(t *testing.T) {
		caller := &dingCoverageCaller{
			responses: map[string][]string{},
			failures:  map[string]error{"list_ding_receiver_status": errors.New("fixture transport failure")},
		}
		if _, err := runDingCoverage(t, ReceiverStatus, caller, "--ding-id", "ding-1"); err == nil {
			t.Fatal("receiver transport failure returned success")
		}
	})

	t.Run("projection failure", func(t *testing.T) {
		caller := &dingCoverageCaller{responses: map[string][]string{
			"list_ding_receiver_status": {`{"success":true,"result":{"receivers":[]}}`},
		}}
		if _, err := runDingCoverage(t, ReceiverStatus, caller, "--ding-id", "ding-1"); err == nil {
			t.Fatal("empty receiver response returned success")
		}
	})

	t.Run("legacy output", func(t *testing.T) {
		legacy := ReceiverStatus
		legacy.OutputRollout = output.RolloutLegacyOnly
		caller := &dingCoverageCaller{responses: map[string][]string{
			"list_ding_receiver_status": {`{"success":true,"result":{"receivers":[{"openDingId":"ding-1","confirmedStatus":1,"receiverNick":"fixture"}]}}`},
		}}
		stdout, err := runDingCoverageOutput(t, legacy, caller, "--ding-id", "ding-1")
		if err != nil {
			t.Fatalf("receiver legacy output: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(stdout, &payload); err != nil || payload["count"] != float64(1) {
			t.Fatalf("receiver legacy payload=%#v decode=%v", payload, err)
		}
		if len(caller.arguments) != 1 || caller.arguments[0]["openDingId"] != "ding-1" {
			t.Fatalf("receiver arguments=%#v", caller.arguments)
		}
	})
}

func TestCrossPlatformCoverageDINGExactReadShortcutsProjectUnifiedData(t *testing.T) {
	listCaller := &dingCoverageCaller{responses: map[string][]string{
		"list_ding_messages": {`{"success":true,"result":{"dingMessages":[{"openDingId":"ding-1","dingContent":"fixture"}],"hasMore":false,"nextCursor":null}}`},
	}}
	cmd, err := runDingCoverage(t, List, listCaller, "--type", "ALL")
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	if code, emitted, emitErr := output.EmitStoredResult(cmd); emitErr != nil || !emitted || code != 0 {
		t.Fatalf("emit=(%d,%t,%v)", code, emitted, emitErr)
	}
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	data := envelope["data"].(map[string]any)
	if _, leaked := data["result"]; leaked || data["count"] != float64(1) || data["complete"] != true {
		t.Fatalf("list unified projection=%#v", data)
	}
	if strings.Join(listCaller.history, ",") != "list_ding_messages" {
		t.Fatalf("list call history=%v", listCaller.history)
	}

	receiverCaller := &dingCoverageCaller{responses: map[string][]string{
		"list_ding_receiver_status": {`{"success":true,"result":{"receivers":[{"openDingId":"ding-1","confirmedStatus":1,"receiverNick":"fixture"}]}}`},
	}}
	if _, err := runDingCoverage(t, ReceiverStatus, receiverCaller, "--ding-id", "ding-1"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(receiverCaller.history, ",") != "list_ding_receiver_status" {
		t.Fatalf("receiver call history=%v", receiverCaller.history)
	}
}

func TestCrossPlatformCoverageDINGUnavailableWritesNeverCallMCP(t *testing.T) {
	args := []string{"--group", "cid-fixture", "--message-id", "mid-fixture", "--users", "D-fixture"}
	unconfirmed := &dingCoverageCaller{responses: map[string][]string{}}
	err := runDingRoot(t, SendByMessage, unconfirmed, false, args...)
	if err == nil || len(unconfirmed.history) != 0 {
		t.Fatalf("unconfirmed error=%v calls=%v", err, unconfirmed.history)
	}
	confirmed := &dingCoverageCaller{responses: map[string][]string{}}
	err = runDingRoot(t, SendByMessage, confirmed, true, args...)
	if err == nil || !strings.Contains(err.Error(), "当前不可执行") || len(confirmed.history) != 0 {
		t.Fatalf("confirmed unavailable error=%v calls=%v", err, confirmed.history)
	}
}

func TestCrossPlatformCoverageDINGCompatibilityWritesRequireConfirmationAndExecute(t *testing.T) {
	for input, want := range map[string]string{"app": "APP", " sms ": "SMS", "call": "PHONE"} {
		got, err := dingPersonalRemindType(input)
		if err != nil || got != want {
			t.Errorf("dingPersonalRemindType(%q)=(%q,%v), want (%q,nil)", input, got, err, want)
		}
	}

	sendArgs := []string{"--users", "D-fixture", "--content", "fixture", "--type", "call", "--uuid", "fixture-uuid"}
	unconfirmedSend := &dingCoverageCaller{responses: map[string][]string{}}
	if err := runDingRoot(t, SendPersonal, unconfirmedSend, false, sendArgs...); err == nil || len(unconfirmedSend.history) != 0 {
		t.Fatalf("unconfirmed send error=%v calls=%v", err, unconfirmedSend.history)
	}
	confirmedSend := &dingCoverageCaller{responses: map[string][]string{
		"send_personal_ding": {`{"success":true,"result":{"openDingId":"ding-fixture"}}`},
	}}
	if err := runDingRoot(t, SendPersonal, confirmedSend, true, sendArgs...); err != nil {
		t.Fatalf("confirmed send failed: %v", err)
	}
	if strings.Join(confirmedSend.history, ",") != "send_personal_ding" || len(confirmedSend.arguments) != 1 {
		t.Fatalf("confirmed send calls=%v args=%v", confirmedSend.history, confirmedSend.arguments)
	}
	sendParams := confirmedSend.arguments[0]
	users, _ := sendParams["receiverOpenDingTalkIds"].([]string)
	if strings.Join(users, ",") != "D-fixture" || sendParams["content"] != "fixture" || sendParams["remindType"] != "PHONE" || sendParams["uuid"] != "fixture-uuid" {
		t.Fatalf("confirmed send params=%#v", sendParams)
	}

	invalidType := &dingCoverageCaller{responses: map[string][]string{}}
	if err := runDingRoot(t, SendPersonal, invalidType, true, "--users", "D-fixture", "--content", "fixture", "--type", "other"); err == nil || len(invalidType.history) != 0 {
		t.Fatalf("invalid type error=%v calls=%v", err, invalidType.history)
	}

	recallArgs := []string{"--id", "ding-fixture"}
	unconfirmedRecall := &dingCoverageCaller{responses: map[string][]string{}}
	if err := runDingRoot(t, RecallPersonal, unconfirmedRecall, false, recallArgs...); err == nil || len(unconfirmedRecall.history) != 0 {
		t.Fatalf("unconfirmed recall error=%v calls=%v", err, unconfirmedRecall.history)
	}
	confirmedRecall := &dingCoverageCaller{responses: map[string][]string{
		"recall_personal_ding": {`{"success":true,"result":true}`},
	}}
	if err := runDingRoot(t, RecallPersonal, confirmedRecall, true, recallArgs...); err != nil {
		t.Fatalf("confirmed recall failed: %v", err)
	}
	if strings.Join(confirmedRecall.history, ",") != "recall_personal_ding" || confirmedRecall.arguments[0]["openDingId"] != "ding-fixture" {
		t.Fatalf("confirmed recall calls=%v args=%v", confirmedRecall.history, confirmedRecall.arguments)
	}

	for _, id := range []string{"msgOpaque+/==", "cidOpaque-123", "MSG", "cid"} {
		unconfirmed := &dingCoverageCaller{}
		if err := runDingRoot(t, RecallPersonal, unconfirmed, false, "--id", id); err == nil || len(unconfirmed.history) != 0 {
			t.Fatalf("opaque ID bypassed confirmation: id=%q err=%v calls=%v", id, err, unconfirmed.history)
		}
		confirmed := &dingCoverageCaller{responses: map[string][]string{
			"recall_personal_ding": {`{"success":true,"result":true}`},
		}}
		if err := runDingRoot(t, RecallPersonal, confirmed, true, "--id", id); err != nil {
			t.Fatalf("opaque DING ID %q rejected: %v", id, err)
		}
		if len(confirmed.history) != 1 || confirmed.history[0] != "recall_personal_ding" || confirmed.arguments[0]["openDingId"] != id {
			t.Fatalf("opaque ID changed or recall replayed: id=%q calls=%v args=%v", id, confirmed.history, confirmed.arguments)
		}
	}
	blankRecall := &dingCoverageCaller{}
	if err := runDingRoot(t, RecallPersonal, blankRecall, true, "--id", " \t"); err == nil || len(blankRecall.history) != 0 {
		t.Fatalf("blank ID reached MCP: err=%v calls=%v", err, blankRecall.history)
	}
}

func TestCrossPlatformCoverageDINGRecallIDValidationBoundary(t *testing.T) {
	for _, id := range []string{"", " \t\n"} {
		if err := validateDingRecallID(id); err == nil {
			t.Fatalf("blank recall ID %q was accepted", id)
		}
	}
	for _, id := range []string{"msgOpaque+/==", "cidOpaque-123", "MSG", "cid", "ding-fixture"} {
		if err := validateDingRecallID(id); err != nil {
			t.Fatalf("opaque recall ID %q was rejected: %v", id, err)
		}
	}
}

func TestCrossPlatformCoverageDINGRecallExecuteRejectsBlankID(t *testing.T) {
	// Validate rejects these through Cobra; Execute also defends direct callers.
	for _, id := range []string{"", " \t\n"} {
		t.Run(fmt.Sprintf("id=%q", id), func(t *testing.T) {
			caller := &dingCoverageCaller{}
			helpers.InitDepsForTest(t, caller)
			cmd := corecmd.New(shortcut.FromShortcut(RecallPersonal))
			if err := cmd.Flags().Set("id", id); err != nil {
				t.Fatal(err)
			}
			err := RecallPersonal.Execute(shortcut.RuntimeContextForTest(cmd, RecallPersonal))
			var failure *apperrors.Error
			if !errors.As(err, &failure) || failure.Category != apperrors.CategoryValidation || len(caller.history) != 0 {
				t.Fatalf("blank ID reached execution: err=%v calls=%v", err, caller.history)
			}
		})
	}
}

func runDingRoot(t *testing.T, declaration shortcut.Shortcut, caller *dingCoverageCaller, yes bool, args ...string) error {
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
	service := &cobra.Command{Use: "ding"}
	service.AddCommand(corecmd.New(shortcut.FromShortcut(declaration)))
	root.AddCommand(service)
	argv := []string{"ding", declaration.Command}
	argv = append(argv, args...)
	if yes {
		argv = append(argv, "--yes")
	}
	root.SetArgs(argv)
	return root.Execute()
}
