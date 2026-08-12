// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package minutesdata

import (
	"encoding/json"
	"testing"
)

func TestCrossPlatformCoverageMinutesWorkflowShapesRejectUnknownE2E(t *testing.T) {
	if err := RequireWriteAcknowledgement("write", map[string]any{}); err == nil {
		t.Fatal("bare write response accepted")
	}
	if _, err := HotWords(map[string]any{"success": true, "result": map[string]any{}}); err == nil {
		t.Fatal("missing hot-word list accepted")
	}
	if _, _, err := MindGraphStatus(map[string]any{"success": true, "result": map[string]any{}}); err == nil {
		t.Fatal("missing mind graph status accepted")
	}
	if _, _, err := MindGraphStatus(map[string]any{"success": true, "result": map[string]any{"taskStatus": 1.5}}); err == nil {
		t.Fatal("fractional mind graph status accepted")
	}
	if _, err := SpeakerSummaryResult(map[string]any{"success": true, "result": map[string]any{}}); err == nil {
		t.Fatal("empty speaker summary accepted")
	}
}

func TestCrossPlatformCoverageMinutesWorkflowObservedShapesE2E(t *testing.T) {
	record, err := RecordResult("pause", "u1", map[string]any{"success": true, "result": map[string]any{"cmd": "pause", "uuid": "u1"}})
	if err != nil || record["cmd"] != "pause" {
		t.Fatalf("record=%#v err=%v", record, err)
	}
	status, _, err := MindGraphStatus(map[string]any{"success": true, "result": map[string]any{"taskStatus": float64(1)}})
	if err != nil || status != 1 {
		t.Fatalf("mind status=%d err=%v", status, err)
	}
	status, _, err = MindGraphStatus(map[string]any{"success": true, "result": map[string]any{"taskStatus": json.Number("1")}})
	if err != nil || status != 1 {
		t.Fatalf("number-preserving mind status=%d err=%v", status, err)
	}
	taskID, state, err := SpeakerSummaryTask(map[string]any{"success": true, "result": map[string]any{"taskId": "job-1", "status": "processing"}})
	if err != nil || taskID != "job-1" || state != "processing" {
		t.Fatalf("speaker task=%q state=%q err=%v", taskID, state, err)
	}
	words, err := HotWords(map[string]any{"success": true, "result": map[string]any{"hotWordList": []any{"DWS", map[string]any{"hotWord": "听记"}}}})
	if err != nil || len(words) != 2 || words[1] != "听记" {
		t.Fatalf("words=%#v err=%v", words, err)
	}
}
