// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package minutesdata

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

func TestCrossPlatformCoverageMinutesResponseCompletion(t *testing.T) {
	for _, data := range []map[string]any{
		{},
		{"success": false, "message": "denied"},
		{"errorCode": "E", "msg": "bad"},
		{"dingOpenErrcode": float64(1)},
	} {
		if err := ValidateEnvelope(data); err == nil {
			t.Fatalf("invalid envelope accepted: %#v", data)
		}
	}
	for _, value := range []any{nil, "", "0", float64(0), 0, int64(0), json.Number("0")} {
		if !isZeroCode(value) {
			t.Fatalf("zero code rejected: %#v", value)
		}
	}
	if isZeroCode(true) {
		t.Fatal("unsupported code treated as zero")
	}
	if got := envelopeMessage(map[string]any{}); got != "no backend error message" {
		t.Fatalf("fallback message = %q", got)
	}

	list, err := ParseListPage(map[string]any{
		"hasNext": true, "nextCursor": "outer",
		"data": map[string]any{"result": map[string]any{"records": []any{map[string]any{"taskUUID": "u1"}}}},
	})
	if err != nil || !list.HasMoreKnown || list.NextToken != "outer" || len(list.Items) != 1 {
		t.Fatalf("nested list = %#v, %v", list, err)
	}
	if _, err := ParseListPage(map[string]any{"result": map[string]any{"items": []any{map[string]any{}, "bad"}}}); err == nil {
		t.Fatal("non-object list item accepted")
	}
	transcript, err := ParseTranscriptPage(map[string]any{
		"hasMore": false,
		"data":    map[string]any{"data": map[string]any{"paragraphList": []any{map[string]any{"sentenceId": "p"}}}},
	})
	if err != nil || !transcript.HasMoreKnown || transcript.HasMore || len(transcript.Items) != 1 {
		t.Fatalf("nested transcript = %#v, %v", transcript, err)
	}
	if _, err := ParseTranscriptPage(map[string]any{"result": map[string]any{"paragraphList": []any{"bad"}}}); err == nil {
		t.Fatal("non-object paragraph accepted")
	}

	for _, tc := range []struct {
		name string
		data map[string]any
	}{
		{"no result", map[string]any{"success": true}},
		{"basic missing", map[string]any{"result": map[string]any{}}},
		{"basic mismatch", map[string]any{"result": map[string]any{"taskUuid": "other"}}},
		{"summary missing", map[string]any{"result": map[string]any{}}},
		{"keywords wrong", map[string]any{"result": map[string]any{"keywords": "bad"}}},
		{"todos wrong", map[string]any{"result": map[string]any{"actions": "bad", "dingtalkTodoList": "bad"}}},
		{"other empty", map[string]any{"result": map[string]any{}}},
	} {
		artifact := map[string]string{"no result": "basic", "basic missing": "basic", "basic mismatch": "basic", "summary missing": "summary", "keywords wrong": "keywords", "todos wrong": "todos", "other empty": "custom"}[tc.name]
		if err := ValidateArtifact(artifact, "u1", tc.data); err == nil {
			t.Fatalf("%s accepted", tc.name)
		}
	}
	if err := ValidateArtifact("basic", "u1", map[string]any{}); err == nil {
		t.Fatal("artifact envelope failure ignored")
	}
	if err := ValidateArtifact("todos", "", map[string]any{"result": map[string]any{"dingtalkTodoList": []any{}}}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateArtifact("custom", "", map[string]any{"result": map[string]any{"value": true}}); err != nil {
		t.Fatal(err)
	}
	if _, err := Basic("u1", map[string]any{"result": map[string]any{}}); err == nil {
		t.Fatal("invalid basic accepted")
	}
	if basic, err := Basic("u1", map[string]any{"result": map[string]any{"uuid": "u1"}}); err != nil || basic["uuid"] != "u1" {
		t.Fatalf("basic = %#v, %v", basic, err)
	}
	if _, err := SummaryText(map[string]any{"result": map[string]any{}}); err == nil {
		t.Fatal("invalid summary accepted")
	}
	if _, err := SummaryText(map[string]any{"result": map[string]any{"fullSummary": 1}}); err == nil {
		t.Fatal("non-string summary accepted")
	}
	if text, err := SummaryText(map[string]any{"result": map[string]any{"fullSummary": "ok"}}); err != nil || text != "ok" {
		t.Fatalf("summary = %q, %v", text, err)
	}

	for _, data := range []map[string]any{{}, {"result": "bad"}, {"result": map[string]any{}}} {
		if _, err := MediaURL(data); err == nil {
			t.Fatalf("invalid media accepted: %#v", data)
		}
	}
	if value, err := MediaURL(map[string]any{"result": map[string]any{"audioUrl": " https://example.invalid/a "}}); err != nil || value == "" {
		t.Fatalf("media = %q, %v", value, err)
	}
	for _, data := range []map[string]any{{}, {"result": "bad"}, {"result": map[string]any{"sessionId": "s"}}} {
		if _, _, err := UploadSession(data); err == nil {
			t.Fatalf("invalid upload session accepted: %#v", data)
		}
	}
	if session, url, err := UploadSession(map[string]any{"result": map[string]any{"uploadId": 7, "uploadUrl": "https://example.invalid"}}); err != nil || session != "7" || url == "" {
		t.Fatalf("upload session = %q/%q, %v", session, url, err)
	}
	if _, err := CompletedTaskUUID(map[string]any{}); err == nil {
		t.Fatal("invalid completed envelope accepted")
	}
	if _, err := CompletedTaskUUID(map[string]any{"result": map[string]any{}}); err == nil {
		t.Fatal("missing completed UUID accepted")
	}
	if id, err := CompletedTaskUUID(map[string]any{"data": map[string]any{"result": map[string]any{"task_uuid": "done"}}}); err != nil || id != "done" {
		t.Fatalf("completed UUID = %q, %v", id, err)
	}

	rows, err := ProjectList(Page{Items: []map[string]any{{
		"uuid": "u", "name": "n", "creatorNick": "c", "createTime": 1,
		"deadline": 2, "link": "l", "state": "ready",
	}}})
	if err != nil || rows[0]["title"] != "n" || rows[0]["creator"] != "c" || rows[0]["url"] != "l" {
		t.Fatalf("projected fallback fields = %#v, %v", rows, err)
	}
	if got := TaskUUID(map[string]any{"taskUUID": json.Number("9")}); got != "9" {
		t.Fatalf("numeric task UUID = %q", got)
	}
	if id, err := LatestTaskUUID(Page{}); err != nil || id != "" {
		t.Fatalf("empty latest = %q, %v", id, err)
	}
	if _, err := LatestTaskUUID(Page{Items: []map[string]any{{"startTime": 1}}}); err == nil {
		t.Fatal("latest missing UUID accepted")
	}
	latest, err := LatestTaskUUID(Page{Items: []map[string]any{
		{"uuid": "skip", "createTime": "bad"},
		{"uuid": "int", "createTime": 1},
		{"uuid": "int64", "gmtCreate": int64(2)},
		{"uuid": "number", "startTime": json.Number("3")},
		{"uuid": "rfc", "createTimeStart": time.Now().UTC().Format(time.RFC3339Nano)},
	}})
	if err != nil || latest != "rfc" {
		t.Fatalf("typed latest = %q, %v", latest, err)
	}
	if key, err := StableItemKey(map[string]any{"text": "same"}); err != nil || !strings.HasPrefix(key, "json:") {
		t.Fatalf("JSON stable key = %q, %v", key, err)
	}
	if _, err := StableItemKey(map[string]any{"bad": func() {}}); err == nil {
		t.Fatal("unencodable stable key accepted")
	}

	for _, value := range []any{"value", json.Number("4"), float64(5), 6, int64(7)} {
		if got := stringField(map[string]any{"x": value}, "missing", "x"); got == "" {
			t.Fatalf("stringField(%#v) empty", value)
		}
	}
	if got := stringField(map[string]any{"x": true}, "x"); got != "" {
		t.Fatalf("unsupported string field = %q", got)
	}
	for _, value := range []any{float64(1), 2, int64(3), json.Number("4"), "5", "2026-08-11T00:00:00Z"} {
		if _, ok := timestamp(map[string]any{"x": value}, "x"); !ok {
			t.Fatalf("timestamp(%#v) rejected", value)
		}
	}
	if _, ok := timestamp(map[string]any{"x": json.Number("bad")}, "x"); ok {
		t.Fatal("invalid timestamp number accepted")
	}
}

func TestCrossPlatformCoverageMinutesPagerCompletion(t *testing.T) {
	if _, err := CollectTranscript(nil, "", false, 1); err == nil {
		t.Fatal("nil transcript caller accepted")
	}
	defaulted, err := CollectTranscript(func(string) (map[string]any, error) {
		return map[string]any{"result": map[string]any{"paragraphList": []any{}}}, nil
	}, "", false, 0)
	if err != nil || !defaulted.Complete {
		t.Fatalf("default max pages = %#v, %v", defaulted, err)
	}
	invalid, err := CollectTranscript(func(string) (map[string]any, error) { return map[string]any{}, nil }, "", false, 1)
	if err == nil || invalid.FailurePage != 1 || invalid.FailureMessage == "" {
		t.Fatalf("invalid page = %#v, %v", invalid, err)
	}
	badKey, err := CollectTranscript(func(string) (map[string]any, error) {
		return map[string]any{"result": map[string]any{"paragraphList": []any{map[string]any{"bad": func() {}}}}}, nil
	}, "", false, 1)
	if err == nil || badKey.FailurePage != 1 {
		t.Fatalf("bad key = %#v, %v", badKey, err)
	}
	one, err := CollectTranscript(func(string) (map[string]any, error) {
		return map[string]any{"result": map[string]any{"paragraphList": []any{}, "hasNext": false}}, nil
	}, "", true, 1)
	if err != nil || !one.Complete {
		t.Fatalf("complete single page = %#v, %v", one, err)
	}
	limited, err := CollectTranscript(func(token string) (map[string]any, error) {
		return map[string]any{"result": map[string]any{"paragraphList": []any{}, "hasNext": true, "nextToken": token + "n"}}, nil
	}, "", false, 1)
	if err == nil || limited.FailurePage != 2 || limited.FailureMessage == "" {
		t.Fatalf("page limit = %#v, %v", limited, err)
	}
	payload := TranscriptPayload("u", "1", TranscriptResult{
		Paragraphs: []map[string]any{{"id": "p"}}, Pages: 1, Duplicates: 2,
		NextToken: "n", FailurePage: 2, FailureMessage: "failed",
	})
	if payload["nextToken"] != "n" || payload["failurePage"] != 2 || payload["failure"] != "failed" {
		t.Fatalf("transcript payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageMinutesWorkflowCompletion(t *testing.T) {
	for _, data := range []map[string]any{{}, {"other": true}, {"success": "yes"}, {"success": false}} {
		if err := RequireWriteAcknowledgement("write", data); err == nil {
			t.Fatalf("invalid acknowledgement accepted: %#v", data)
		}
	}
	if err := RequireWriteAcknowledgement("write", map[string]any{"result": map[string]any{"updated": true}}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		cmd, id string
		data    map[string]any
	}{
		{"pause", "u", map[string]any{}},
		{"pause", "u", map[string]any{"success": true}},
		{"pause", "u", map[string]any{"success": true, "result": map[string]any{"cmd": "resume", "uuid": "u"}}},
		{"pause", "u", map[string]any{"success": true, "result": map[string]any{"cmd": "pause", "uuid": "other"}}},
	} {
		if _, err := RecordResult(tc.cmd, tc.id, tc.data); err == nil {
			t.Fatalf("invalid record accepted: %#v", tc)
		}
	}
	if _, err := RecordResult("create", "", map[string]any{"success": true, "result": map[string]any{"cmd": "create"}}); err != nil {
		t.Fatal(err)
	}
	for _, data := range []map[string]any{{}, {"result": "bad"}, {"result": map[string]any{}}, {"result": map[string]any{"taskStatus": 3}}} {
		if _, _, err := MindGraphStatus(data); err == nil {
			t.Fatalf("invalid mind graph accepted: %#v", data)
		}
	}
	for _, value := range []any{0, "1", json.Number("2")} {
		status, _, err := MindGraphStatus(map[string]any{"result": map[string]any{"taskStatus": value}})
		if err != nil || status < 0 || status > 2 {
			t.Fatalf("mind status %#v = %d, %v", value, status, err)
		}
	}
	for _, data := range []map[string]any{{}, {"success": true}, {"success": true, "result": map[string]any{}}} {
		if _, _, err := SpeakerSummaryTask(data); err == nil {
			t.Fatalf("invalid speaker task accepted: %#v", data)
		}
	}
	if id, status, err := SpeakerSummaryTask(map[string]any{"result": map[string]any{"taskID": int64(7), "status": "READY"}}); err != nil || id != "7" || status != "ready" {
		t.Fatalf("speaker task = %q/%q, %v", id, status, err)
	}
	for _, data := range []map[string]any{{}, {"result": nil}, {"result": []any(nil)}, {"result": "bad"}} {
		if _, err := SpeakerSummaryResult(data); err == nil {
			t.Fatalf("invalid speaker result accepted: %#v", data)
		}
	}
	if value, err := SpeakerSummaryResult(map[string]any{"result": []any{}}); err != nil || value == nil {
		t.Fatalf("empty speaker list = %#v, %v", value, err)
	}
	if value, err := SpeakerSummaryResult(map[string]any{"result": map[string]any{"summary": "ok"}}); err != nil || value == nil {
		t.Fatalf("speaker map = %#v, %v", value, err)
	}
	for _, data := range []map[string]any{{}, {"result": "bad"}, {"result": map[string]any{}}, {"result": map[string]any{"hotWordList": "bad"}}, {"result": map[string]any{"hotWordList": []any{true}}}} {
		if _, err := HotWords(data); err == nil {
			t.Fatalf("invalid hot words accepted: %#v", data)
		}
	}
	words, err := HotWords(map[string]any{"result": map[string]any{"hotWordList": []any{" A ", "A", map[string]any{"content": "B"}}}})
	if err != nil || len(words) != 2 {
		t.Fatalf("deduplicated hot words = %#v, %v", words, err)
	}
	for _, value := range []any{math.NaN(), math.Inf(1), 1.5, float64(-1), float64(3), json.Number("bad"), "bad", true} {
		if _, err := intValue(value); err == nil {
			t.Fatalf("invalid int value accepted: %#v", value)
		}
	}
	if _, err := intValue(int64(1)); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unsupported int64 result = %v", err)
	}
}
