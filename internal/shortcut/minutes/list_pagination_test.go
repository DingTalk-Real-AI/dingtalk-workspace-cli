// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package minutes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageMinutesPublishedListRoutesUseUnifiedPagination(t *testing.T) {
	for _, declaration := range []struct {
		name    string
		rollout output.RolloutState
		result  string
		page    bool
	}{
		{ListMine.Command, ListMine.OutputRollout, string(ListMine.Contract.Result.DataSchema), ListMine.Contract.Pagination != nil},
		{ListShared.Command, ListShared.OutputRollout, string(ListShared.Contract.Result.DataSchema), ListShared.Contract.Pagination != nil},
		{ListAll.Command, ListAll.OutputRollout, string(ListAll.Contract.Result.DataSchema), ListAll.Contract.Pagination != nil},
		{Search.Command, Search.OutputRollout, string(Search.Contract.Result.DataSchema), Search.Contract.Pagination != nil},
	} {
		if declaration.rollout != output.RolloutUnifiedActive || !declaration.page {
			t.Errorf("%s rollout=%q pagination=%t", declaration.name, declaration.rollout, declaration.page)
		}
		for _, leaked := range []string{`"endpointExhausted"`, `"nextToken"`} {
			if strings.Contains(declaration.result, leaked) {
				t.Errorf("%s Result data_schema leaked pagination field %s", declaration.name, leaked)
			}
		}
	}
}

func TestCrossPlatformCoverageMinutesListPreviewAndPageAllE2E(t *testing.T) {
	previewCaller := &minutesE2ECaller{responses: map[string][]string{
		"minutes/list_by_keyword_and_time_range": {`{"success":true,"result":{"itemList":[{"taskUuid":"u1","title":"第一条"}],"hasNext":true,"nextToken":"n2"}}`},
	}}
	preview, raw, err := runMinutesAlignmentCLI(t, previewCaller, "minutes", "+list-mine", "--limit", "1")
	if err != nil || preview["complete"] != false || preview["endpointExhausted"] != nil || preview["nextToken"] != nil || preview["pages"] != float64(1) {
		t.Fatalf("preview=%#v err=%v", preview, err)
	}
	pagination := minutesPaginationFromOutput(t, raw)
	if pagination["endpoint_exhausted"] != false || pagination["next_token"] != "n2" || pagination["pages"] != float64(1) || pagination["items"] != float64(1) {
		t.Fatalf("preview pagination=%#v", pagination)
	}

	allCaller := &minutesE2ECaller{responses: map[string][]string{
		"minutes/list_by_keyword_and_time_range": {
			`{"success":true,"result":{"itemList":[{"taskUuid":"u1","title":"第一条"}],"hasNext":true,"nextToken":"n2"}}`,
			`{"success":true,"result":{"itemList":[{"taskUuid":"u1","title":"第一条"},{"taskUuid":"u2","title":"第二条"}],"hasNext":false}}`,
		},
	}}
	all, raw, err := runMinutesAlignmentCLI(t, allCaller, "minutes", "+list-mine", "--limit", "1", "--page-all")
	if err != nil || all["complete"] != true || all["endpointExhausted"] != nil || all["count"] != float64(2) || all["pages"] != float64(2) {
		t.Fatalf("page-all=%#v err=%v", all, err)
	}
	pagination = minutesPaginationFromOutput(t, raw)
	if pagination["endpoint_exhausted"] != true || pagination["next_token"] != nil || pagination["pages"] != float64(2) || pagination["items"] != float64(2) {
		t.Fatalf("page-all pagination=%#v", pagination)
	}
	if calls := allCaller.arguments["minutes/list_by_keyword_and_time_range"]; len(calls) != 2 || calls[0]["belongingConditionId"] != "created" || calls[1]["nextToken"] != "n2" {
		t.Fatalf("page-all calls=%#v", calls)
	}
}

func TestCrossPlatformCoverageMinutesAccessibleListMergesMineAndSharedE2E(t *testing.T) {
	caller := &minutesE2ECaller{responses: map[string][]string{
		"minutes/list_by_keyword_and_time_range": {
			`{"success":true,"result":{"itemList":[{"taskUuid":"u1","title":"自有"}],"hasNext":false}}`,
			`{"success":true,"result":{"itemList":[{"taskUuid":"u1","title":"重复"},{"taskUuid":"u2","title":"共享"}],"hasNext":false}}`,
		},
	}}
	payload, raw, err := runMinutesAlignmentCLI(t, caller, "minutes", "+list-all", "--page-all")
	if err != nil || payload["complete"] != true || payload["count"] != float64(2) || payload["pages"] != float64(2) {
		t.Fatalf("accessible=%#v err=%v", payload, err)
	}
	if pagination := minutesPaginationFromOutput(t, raw); pagination["endpoint_exhausted"] != true || pagination["pages"] != float64(2) || pagination["items"] != float64(2) {
		t.Fatalf("accessible pagination=%#v", pagination)
	}
	ledger, ok := payload["scopeLedger"].([]any)
	if !ok || len(ledger) != 2 {
		t.Fatalf("scope ledger=%#v", payload["scopeLedger"])
	}
	calls := caller.arguments["minutes/list_by_keyword_and_time_range"]
	if len(calls) != 2 || calls[0]["belongingConditionId"] != "created" || calls[1]["belongingConditionId"] != "shared" {
		t.Fatalf("accessible calls=%#v", calls)
	}
}

func TestCrossPlatformCoverageMinutesAccessiblePreviewNeverClaimsUnionCompleteE2E(t *testing.T) {
	caller := &minutesE2ECaller{responses: map[string][]string{
		"minutes/list_by_keyword_and_time_range": {`{"success":true,"result":{"itemList":[],"hasNext":false}}`},
	}}
	payload, raw, err := runMinutesAlignmentCLI(t, caller, "minutes", "+list-all")
	if err != nil || payload["complete"] != false || payload["endpointExhausted"] != nil || payload["nextAction"] == "" {
		t.Fatalf("accessible preview=%#v err=%v", payload, err)
	}
	if pagination := minutesPaginationFromOutput(t, raw); pagination["endpoint_exhausted"] != true || pagination["next_token"] != nil {
		t.Fatalf("accessible preview pagination=%#v", pagination)
	}
	calls := caller.arguments["minutes/list_by_keyword_and_time_range"]
	if len(calls) != 1 || calls[0]["belongingConditionId"] != "noLimit" {
		t.Fatalf("preview calls=%#v", calls)
	}
}

func TestCrossPlatformCoverageMinutesListPaginationFailsClosedE2E(t *testing.T) {
	t.Run("invalid limits", func(t *testing.T) {
		for _, args := range [][]string{
			{"minutes", "+list-mine", "--limit", "0"},
			{"minutes", "+list-shared", "--page-limit", "0"},
		} {
			caller := &minutesE2ECaller{}
			if _, _, err := runMinutesAlignmentCLI(t, caller, args...); err == nil || len(caller.counts) != 0 {
				t.Fatalf("invalid limits accepted: args=%v err=%v calls=%#v", args, err, caller.counts)
			}
		}
	})

	t.Run("aggregate first scope failure", func(t *testing.T) {
		caller := &minutesE2ECaller{failAt: map[string]int{"minutes/list_by_keyword_and_time_range": 1}}
		if _, _, err := runMinutesAlignmentCLI(t, caller, "minutes", "+list-all", "--page-all"); err == nil {
			t.Fatal("first aggregate scope failure accepted")
		}
	})

	t.Run("missing token", func(t *testing.T) {
		caller := &minutesE2ECaller{responses: map[string][]string{
			"minutes/list_by_keyword_and_time_range": {`{"success":true,"result":{"itemList":[],"hasNext":true}}`},
		}}
		if _, _, err := runMinutesAlignmentCLI(t, caller, "minutes", "+list-mine", "--page-all"); err == nil {
			t.Fatal("missing nextToken accepted")
		}
	})

	t.Run("cursor cycle", func(t *testing.T) {
		caller := &minutesE2ECaller{responses: map[string][]string{
			"minutes/list_by_keyword_and_time_range": {
				`{"success":true,"result":{"itemList":[{"taskUuid":"u1"}],"hasNext":true,"nextToken":"same"}}`,
				`{"success":true,"result":{"itemList":[{"taskUuid":"u2"}],"hasNext":true,"nextToken":"same"}}`,
			},
		}}
		payload, raw, err := runMinutesAlignmentCLI(t, caller, "minutes", "+list-mine", "--page-all")
		pagination := minutesPaginationFromOutput(t, raw)
		if err == nil || payload["outcome"] != "failure" || pagination["next_token"] != "same" || pagination["endpoint_exhausted"] != false {
			t.Fatalf("cycle payload=%#v err=%v", payload, err)
		}
	})

	t.Run("page limit", func(t *testing.T) {
		caller := &minutesE2ECaller{responses: map[string][]string{
			"minutes/list_by_keyword_and_time_range": {`{"success":true,"result":{"itemList":[{"taskUuid":"u1"}],"hasNext":true,"nextToken":"n2"}}`},
		}}
		payload, raw, err := runMinutesAlignmentCLI(t, caller, "minutes", "+list-shared", "--page-all", "--page-limit", "1")
		pagination := minutesPaginationFromOutput(t, raw)
		if err == nil || payload["outcome"] != "failure" || pagination["next_token"] != "n2" || pagination["endpoint_exhausted"] != false {
			t.Fatalf("limit payload=%#v err=%v", payload, err)
		}
	})

	t.Run("aggregate internal cursor is not published", func(t *testing.T) {
		caller := &minutesE2ECaller{
			responses: map[string][]string{
				"minutes/list_by_keyword_and_time_range": {`{"success":true,"result":{"itemList":[{"taskUuid":"u1"}],"hasNext":true,"nextToken":"mine-n2"}}`},
			},
			failAt: map[string]int{"minutes/list_by_keyword_and_time_range": 2},
		}
		payload, raw, err := runMinutesAlignmentCLI(t, caller, "minutes", "+list-all", "--page-all")
		if err == nil || payload["outcome"] != "failure" {
			t.Fatalf("aggregate failure payload=%#v err=%v", payload, err)
		}
		var envelope map[string]any
		if decodeErr := json.Unmarshal([]byte(raw), &envelope); decodeErr != nil {
			t.Fatal(decodeErr)
		}
		meta, _ := envelope["meta"].(map[string]any)
		if meta == nil || meta["pagination"] != nil {
			t.Fatalf("aggregate failure published non-reusable cursor: %#v", envelope)
		}
	})

	t.Run("accessible cursor conflict", func(t *testing.T) {
		caller := &minutesE2ECaller{}
		if _, _, err := runMinutesAlignmentCLI(t, caller, "minutes", "+list-all", "--cursor", "n2", "--page-all"); err == nil || len(caller.counts) != 0 {
			t.Fatalf("cursor conflict err=%v calls=%#v", err, caller.counts)
		}
	})
}

func TestCrossPlatformCoverageMinutesListResultDefensiveBranches(t *testing.T) {
	legacy := &cobra.Command{Use: "legacy"}
	var legacyOut bytes.Buffer
	legacy.SetOut(&legacyOut)
	legacyRT := shortcut.RuntimeContextForTest(legacy, ListMine)
	readErr := errors.New("fixture read failure")
	if err := outputMinutesListResult(legacyRT, map[string]any{"scope": "mine"}, minutesListCollection{}, readErr); !errors.Is(err, readErr) {
		t.Fatalf("legacy read error=%v", err)
	}

	failing := &cobra.Command{Use: "legacy-failing"}
	failing.SetOut(minutesFailWriter{})
	if err := outputMinutesListResult(shortcut.RuntimeContextForTest(failing, ListMine), map[string]any{"scope": "mine"}, minutesListCollection{}, nil); err == nil {
		t.Fatal("legacy output failure accepted")
	}

	unified := &cobra.Command{Use: "unified"}
	ctx, _ := output.WithResultStore(context.Background())
	unified.SetContext(ctx)
	output.SetCommandRollout(unified, output.RolloutUnifiedActive)
	invalid := minutesListCollection{EndpointExhausted: true, NextToken: "unexpected"}
	if err := outputMinutesListResult(shortcut.RuntimeContextForTest(unified, ListMine), map[string]any{"scope": "mine"}, invalid, nil); err == nil {
		t.Fatal("inconsistent unified pagination accepted")
	}
}
