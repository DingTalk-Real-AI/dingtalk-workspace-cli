// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package smart

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

func runMinutesCLI(t *testing.T, caller *smartCoverageCaller, args ...string) (map[string]any, string, error) {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	root := newPlatformCoverageRoot()
	ctx, _ := output.WithResultStore(context.Background())
	root.SetContext(ctx)
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetArgs(args)
	executed, err := root.ExecuteC()
	if err == nil && output.UsesUnifiedResult(executed) {
		code, _, emitErr := output.EmitStoredResult(executed)
		if emitErr != nil {
			err = emitErr
		} else if code != 0 {
			err = fmt.Errorf("command result exit code %d", code)
		}
	}
	if stdout.Len() == 0 {
		return nil, "", err
	}
	var envelope map[string]any
	if decodeErr := json.Unmarshal(stdout.Bytes(), &envelope); decodeErr != nil {
		t.Fatalf("decode output %q: %v", stdout.String(), decodeErr)
	}
	if payload, ok := envelope["data"].(map[string]any); ok {
		return payload, stdout.String(), err
	}
	return envelope, stdout.String(), err
}

func smartMinutesPaginationFromOutput(t *testing.T, raw string) map[string]any {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatalf("decode pagination envelope %q: %v", raw, err)
	}
	meta, ok := envelope["meta"].(map[string]any)
	if !ok {
		t.Fatalf("missing meta in envelope: %#v", envelope)
	}
	pagination, ok := meta["pagination"].(map[string]any)
	if !ok {
		t.Fatalf("missing meta.pagination in envelope: %#v", envelope)
	}
	return pagination
}

func TestCrossPlatformCoverageMinutesDetailPaginatesRealTranscriptShapeE2E(t *testing.T) {
	caller := &smartCoverageCaller{responses: map[string][]string{
		"minutes/get_minutes_transcription": {
			`{"success":true,"result":{"paragraphList":[{"paragraphId":"p1"},{"paragraphId":"p2"}],"hasNext":true,"nextToken":"n2"}}`,
			`{"success":true,"result":{"paragraphList":[{"paragraphId":"p2"},{"paragraphId":"p3"}],"hasNext":false,"nextToken":""}}`,
		},
	}}
	payload, _, err := runMinutesCLI(t, caller, "minutes", "+detail", "--id", "u1", "--artifacts", "transcript")
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	transcript := payload["transcript"].(map[string]any)
	if payload["complete"] != true || transcript["complete"] != true || transcript["pages"] != float64(2) || transcript["paragraphCount"] != float64(3) || transcript["duplicateCount"] != float64(1) {
		t.Fatalf("detail payload = %#v", payload)
	}
	args := caller.arguments["minutes/get_minutes_transcription"]
	if len(args) != 2 || args[1]["nextToken"] != "n2" {
		t.Fatalf("transcript calls = %#v", args)
	}
}

func TestCrossPlatformCoverageMinutesDetailUnknownOrPartialTranscriptIsNotSuccessE2E(t *testing.T) {
	tests := []struct {
		name      string
		responses []string
		failAt    int
		wantCount float64
	}{
		{name: "missing collection", responses: []string{`{"success":true,"result":{}}`}, wantCount: 0},
		{name: "later page failure", responses: []string{`{"success":true,"result":{"paragraphList":[{"paragraphId":"p1"}],"hasNext":true,"nextToken":"n2"}}`}, failAt: 2, wantCount: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &smartCoverageCaller{
				responses: map[string][]string{"minutes/get_minutes_transcription": test.responses},
				failAt:    map[string]int{"minutes/get_minutes_transcription": test.failAt},
			}
			payload, output, err := runMinutesCLI(t, caller, "minutes", "+detail", "--id", "u1", "--artifacts", "transcript")
			if err == nil || output == "" || payload["complete"] != false || payload["failureCount"] != float64(1) {
				t.Fatalf("incomplete detail accepted = payload:%#v output:%q err:%v", payload, output, err)
			}
			transcript := payload["transcript"].(map[string]any)
			if transcript["complete"] != false || transcript["paragraphCount"] != test.wantCount {
				t.Fatalf("incomplete transcript = %#v", transcript)
			}
		})
	}
}

func TestCrossPlatformCoverageMinutesDetailBatchWritesVerifiedTranscriptFilesE2E(t *testing.T) {
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	caller := &smartCoverageCaller{responses: map[string][]string{
		"minutes/get_minutes_transcription": {
			`{"success":true,"result":{"paragraphList":[{"paragraphId":"p1","paragraph":"one"}],"hasNext":false}}`,
			`{"success":true,"result":{"paragraphList":[{"paragraphId":"p2","paragraph":"two"}],"hasNext":false}}`,
		},
	}}
	payload, _, err := runMinutesCLI(t, caller, "minutes", "+detail", "--ids", "u1,u2", "--artifacts", "transcript", "--transcript-output", "file", "--output-dir", "transcripts")
	if err != nil || payload["complete"] != true || payload["requested"] != float64(2) || payload["succeeded"] != float64(2) {
		t.Fatalf("batch detail=%#v err=%v", payload, err)
	}
	for _, result := range payload["results"].([]any) {
		transcript := result.(map[string]any)["transcript"].(map[string]any)
		if transcript["inline"] != false || transcript["fileComplete"] != true || transcript["paragraphList"] != nil {
			t.Fatalf("file transcript=%#v", transcript)
		}
		path := transcript["path"].(string)
		info, statErr := os.Stat(filepath.Join(work, filepath.FromSlash(path)))
		if statErr != nil || info.Size() <= 0 || float64(info.Size()) != transcript["sizeBytes"] {
			t.Fatalf("transcript file path=%q info=%#v err=%v", path, info, statErr)
		}
	}
}

func TestCrossPlatformCoverageMinutesDetailBatchPartialIsNonZeroE2E(t *testing.T) {
	caller := &smartCoverageCaller{responses: map[string][]string{
		"minutes/get_minutes_transcription": {
			`{"success":true,"result":{"paragraphList":[{"paragraphId":"p1","paragraph":"one"}],"hasNext":false}}`,
			`{"success":true,"result":{}}`,
		},
	}}
	payload, output, err := runMinutesCLI(t, caller, "minutes", "+detail", "--ids", "u1,u2", "--artifacts", "transcript")
	if err == nil || output == "" || payload["complete"] != false || payload["requested"] != float64(2) || payload["succeeded"] != float64(1) || payload["failed"] != float64(1) {
		t.Fatalf("batch partial accepted: payload=%#v output=%q err=%v", payload, output, err)
	}
}

func TestCrossPlatformCoverageMinutesSearchAndLatestRejectUnknownButAcceptItemListE2E(t *testing.T) {
	unknown := &smartCoverageCaller{responses: map[string][]string{
		"minutes/list_by_keyword_and_time_range": {`{"success":true,"result":{}}`},
	}}
	if payload, output, err := runMinutesCLI(t, unknown, "minutes", "+minutes-search", "--query", "weekly"); err == nil || payload != nil || output != "" {
		t.Fatalf("unknown search accepted = payload:%#v output:%q err:%v", payload, output, err)
	}

	valid := &smartCoverageCaller{responses: map[string][]string{
		"minutes/list_by_keyword_and_time_range": {`{"success":true,"result":{"itemList":[{"uuid":"u1","title":"weekly","startTime":1}]}}`},
		"minutes/get_minutes_basic_info":         {`{"success":true,"result":{"taskUuid":"u1","title":"weekly"}}`},
	}}
	_, _, err := runMinutesCLI(t, valid, "minutes", "+latest-minutes")
	if err != nil {
		t.Fatalf("latest itemList: %v", err)
	}
	if calls := valid.arguments["minutes/get_minutes_basic_info"]; len(calls) != 1 || calls[0]["taskUuid"] != "u1" {
		t.Fatalf("latest readback calls = %#v", calls)
	}
}

func TestCrossPlatformCoverageMinutesTranscriptSinglePagePublishesIncompleteCursorE2E(t *testing.T) {
	caller := &smartCoverageCaller{responses: map[string][]string{
		"minutes/get_minutes_transcription": {`{"success":true,"result":{"paragraphList":[{"paragraphId":"p1"}],"hasNext":true,"nextToken":"n2"}}`},
	}}
	payload, raw, err := runMinutesCLI(t, caller, "minutes", "+transcript", "--id", "u1", "--single-page")
	if err != nil || raw == "" || payload["complete"] != false || payload["nextToken"] != nil || payload["paragraphCount"] != float64(1) {
		t.Fatalf("single page = payload:%#v output:%q err:%v", payload, raw, err)
	}
	pagination := smartMinutesPaginationFromOutput(t, raw)
	if pagination["endpoint_exhausted"] != false || pagination["next_token"] != "n2" || pagination["pages"] != float64(1) || pagination["items"] != float64(1) {
		t.Fatalf("single-page pagination=%#v", pagination)
	}
	if strings.Contains(raw, `"complete": true`) {
		t.Fatalf("single page falsely complete: %s", raw)
	}
}

func TestCrossPlatformCoverageMinutesReplaceBatchPartialWriteIsNonZeroE2E(t *testing.T) {
	for _, policy := range []struct {
		name            string
		args            []string
		wantCalls       int
		wantApplied     float64
		wantUnattempted float64
	}{
		{name: "stop", wantCalls: 2, wantApplied: 1, wantUnattempted: 1},
		{name: "continue", args: []string{"--failure-policy", "continue"}, wantCalls: 3, wantApplied: 2, wantUnattempted: 0},
	} {
		t.Run(policy.name, func(t *testing.T) {
			caller := &smartCoverageCaller{
				responses: map[string][]string{
					"minutes/replace_minutes_text": {`{"success":true}`, `{"success":true}`, `{"success":true}`},
					"minutes/get_minutes_transcription": {
						`{"success":true,"result":{"paragraphList":[{"paragraphId":"p1","paragraph":"a b c"}],"hasNext":false}}`,
						`{"success":true,"result":{"paragraphList":[{"paragraphId":"p1","paragraph":"A b c"}],"hasNext":false}}`,
						`{"success":true,"result":{"paragraphList":[{"paragraphId":"p1","paragraph":"A b C"}],"hasNext":false}}`,
					},
				},
				failAt: map[string]int{"minutes/replace_minutes_text": 2},
			}
			args := []string{"minutes", "+replace-batch", "--id", "u1", "--pair", "a=>A", "--pair", "b=>B", "--pair", "c=>C", "--yes"}
			args = append(args, policy.args...)
			payload, output, err := runMinutesCLI(t, caller, args...)
			if err == nil || output == "" || payload["ok"] != false || payload["failed"] != float64(1) || payload["applied"] != policy.wantApplied || payload["unattempted"] != policy.wantUnattempted {
				t.Fatalf("partial replace = payload:%#v output:%q err:%v", payload, output, err)
			}
			if got := caller.counts["minutes/replace_minutes_text"]; got != policy.wantCalls {
				t.Fatalf("replace calls = %d, want %d", got, policy.wantCalls)
			}
		})
	}
}

func TestCrossPlatformCoverageMinutesReplaceAcknowledgedButUnchangedReadbackIsNonZeroE2E(t *testing.T) {
	caller := &smartCoverageCaller{responses: map[string][]string{
		"minutes/replace_minutes_text": {`{"success":true}`},
		"minutes/get_minutes_transcription": {
			`{"success":true,"result":{"paragraphList":[{"paragraphId":"p1","paragraph":"a"}],"hasNext":false}}`,
			`{"success":true,"result":{"paragraphList":[{"paragraphId":"p1","paragraph":"a"}],"hasNext":false}}`,
		},
	}}
	payload, output, err := runMinutesCLI(t, caller, "minutes", "+replace-batch", "--id", "u1", "--pair", "a=>A", "--yes")
	if err == nil || output == "" || payload["ok"] != false || payload["failed"] != float64(1) || payload["applied"] != float64(0) {
		t.Fatalf("unchanged readback accepted: payload=%#v output=%q err=%v", payload, output, err)
	}
	result := payload["results"].([]any)[0].(map[string]any)
	if result["acknowledged"] != true || result["remoteEffectUnknown"] != true || result["verified"] == true {
		t.Fatalf("unchanged result=%#v", result)
	}
}

func TestCrossPlatformCoverageMinutesReplaceJSONDryRunDoesNotWriteE2E(t *testing.T) {
	caller := &smartCoverageCaller{}
	payload, output, err := runMinutesCLI(t, caller,
		"minutes", "+replace-batch", "--id", "u1",
		"--json", `[{"source":"a","replacement":"A"},{"originalText":"b","replacedText":"B"}]`,
		"--dry-run")
	if err != nil || output == "" || payload["dryRun"] != true || payload["executed"] != false || payload["total"] != float64(2) {
		t.Fatalf("replace dry-run = payload:%#v output:%q err:%v", payload, output, err)
	}
	if caller.counts["minutes/replace_minutes_text"] != 0 {
		t.Fatalf("dry-run performed writes: %#v", caller.counts)
	}
}
