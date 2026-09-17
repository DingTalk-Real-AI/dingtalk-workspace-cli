// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package whiteboard

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	whiteboardcore "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/whiteboard"
)

func decodeWhiteboardBusinessData(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode unified output: %v; output=%s", err, raw)
	}
	if envelope.Data == nil {
		t.Fatalf("unified output omitted data: %s", raw)
	}
	return envelope.Data
}

func TestCrossPlatformCoverageWhiteboardDiffStandaloneExecutionLogicalMediaAndRisk(t *testing.T) {
	current := `[
		{"id":"real-title","type":"text","source":"page","x":10,"y":20,"text":"old"},
		{"id":"real-image","type":"image","source":"page","writeSupport":"readOnly","resource":{"resourceId":"asset-1"}}
	]`
	source := `{"overwrite":true,"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"title","type":"text","x":11,"y":20,"text":"new"}]}}`
	identity := `{"version":1,"target":{"nodeId":"wb","pageId":"page-1","revision":11},"nodes":{"title":"real-title"}}`
	caller := &whiteboardCoverageCaller{responses: map[string][]string{
		toolQueryStandalone: {validStandaloneWhiteboardQueryResponse("wb", "page", 12, current)},
	}}
	raw, err := runWhiteboardCoverageOutput(t, Diff, caller, "",
		"--node", "wb", "--page-id", "page-1", "--source", source, "--identity-map", identity)
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != toolQueryStandalone || caller.calls[0].args["view"] != "page" || caller.calls[0].args["pageId"] != "page-1" {
		t.Fatalf("diff query calls=%#v", caller.calls)
	}
	data := decodeWhiteboardBusinessData(t, raw)
	if data["mode"] != "overwrite" || data["identityStrategy"] != "explicit_id_map" || data["complete"] != true {
		t.Fatalf("diff identity/mode=%#v", data)
	}
	target := data["target"].(map[string]any)
	if target["revision"] != float64(12) || target["pageId"] != "page-1" {
		t.Fatalf("target=%#v", target)
	}
	summary := data["summary"].(map[string]any)
	for key, want := range map[string]float64{
		"executionAdded": 1, "executionDeleted": 2, "executionPreserved": 0,
		"logicalModified": 1, "logicalUnmatched": 1,
		"mediaRemoved": 1, "mediaUnsupported": 1,
		"blockerCount": 1, "warningCount": 1,
	} {
		if summary[key] != want {
			t.Errorf("summary[%s]=%v, want %v", key, summary[key], want)
		}
	}
	if _, exists := data["logicalDiff"].(map[string]any)["unchanged"]; exists {
		t.Fatal("logicalDiff unexpectedly returned unchanged details")
	}
	if _, exists := data["executionDiff"].(map[string]any)["preserved"]; exists {
		t.Fatal("executionDiff unexpectedly returned preserved details")
	}
	if !strings.HasPrefix(data["sourceDigest"].(string), "sha256:") || !strings.HasPrefix(data["snapshotDigest"].(string), "sha256:") {
		t.Fatalf("digests=%v/%v", data["sourceDigest"], data["snapshotDigest"])
	}
}

func TestCrossPlatformCoverageWhiteboardDiffEmbeddedIsBestEffortAndNeverWrites(t *testing.T) {
	source := `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"new","type":"text"}]}}`
	caller := &whiteboardCoverageCaller{responses: map[string][]string{
		toolQuery: {validWhiteboardQueryResponse(`[{"id":"old","type":"text","source":"page"}]`)},
	}}
	raw, err := runWhiteboardCoverageOutput(t, Diff, caller, "", "--node", "doc", "--part-id", "part", "--source", source)
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != toolQuery {
		t.Fatalf("embedded diff crossed unexpected RPC boundary: %#v", caller.calls)
	}
	data := decodeWhiteboardBusinessData(t, raw)
	summary := data["summary"].(map[string]any)
	if summary["warningCount"] != float64(1) {
		t.Fatalf("summary=%#v", summary)
	}
	warningSummary := data["warningSummary"].([]any)
	if len(warningSummary) != 1 || warningSummary[0].(map[string]any)["reason"] != "embedded_preview_not_atomic" {
		t.Fatalf("warningSummary=%#v", warningSummary)
	}
}

func TestCrossPlatformCoverageWhiteboardDiffFailsClosedForDownloadOnlySnapshot(t *testing.T) {
	response := `{"success":true,"nodeId":"wb","revision":12,"view":"page","resultDownloadUrl":"https://example.invalid/sensitive","resultSummary":{"nodeCount":100,"pageCount":1,"readOnlyNodeCount":0,"unknownNodeCount":0,"resultBytes":20000000,"resultSha256":"0123456789abcdef"}}`
	caller := &whiteboardCoverageCaller{responses: map[string][]string{toolQueryStandalone: {response}}}
	source := `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"new","type":"text"}]}}`
	err := runWhiteboardCoverage(t, Diff, caller, "", "--node", "wb", "--page-id", "page-1", "--source", source)
	var structured *apperrors.Error
	if !errors.As(err, &structured) || structured.Reason != "snapshot_download_required" {
		t.Fatalf("error=%#v", err)
	}
	if strings.Contains(err.Error(), "example.invalid") {
		t.Fatalf("download URL leaked in error: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("download-only diff made extra calls: %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageWhiteboardExpectedSourceDigestStopsBeforeRPC(t *testing.T) {
	source := `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n1","type":"text"}]}}`
	parsed, err := parseWhiteboardSource(source)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := whiteboardSourceDigest(parsed)
	if err != nil {
		t.Fatal(err)
	}
	for name, expected := range map[string][2]string{
		"invalid":  {"invalid_expected_source_digest", "bad"},
		"mismatch": {"source_digest_mismatch", "sha256:" + strings.Repeat("0", 64)},
	} {
		t.Run(name, func(t *testing.T) {
			caller := &whiteboardCoverageCaller{dry: true, responses: map[string][]string{}}
			err := runWhiteboardCoverage(t, Update, caller, "", "--node", "doc", "--part-id", "part", "--source", source, "--expected-source-digest", expected[1], "--dry-run")
			var structured *apperrors.Error
			if !errors.As(err, &structured) || structured.Reason != expected[0] || structured.ExecutionStarted == nil || *structured.ExecutionStarted {
				t.Fatalf("error=%#v", err)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("digest failure reached RPC: %#v", caller.calls)
			}
		})
	}
	caller := &whiteboardCoverageCaller{dry: true, responses: map[string][]string{}}
	if err := runWhiteboardCoverage(t, Update, caller, "", "--node", "doc", "--part-id", "part", "--source", source, "--expected-source-digest", digest, "--dry-run"); err != nil {
		t.Fatalf("matching digest rejected: %v", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("matching digest dry-run reached RPC: %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageWhiteboardDiffIdentityValidationAndDetailLimitStopBeforeRPC(t *testing.T) {
	source := `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n1","type":"text"}]}}`
	for name, args := range map[string]struct {
		args   []string
		reason string
	}{
		"detail low":              {[]string{"--detail-limit", "0"}, "invalid_detail_limit"},
		"detail high":             {[]string{"--detail-limit", "1001"}, "invalid_detail_limit"},
		"duplicate real identity": {[]string{"--identity-map", `{"version":1,"target":{"nodeId":"wb","pageId":"page-1"},"nodes":{"one":"real","two":"real"}}`}, "identity_map_not_bijective"},
	} {
		t.Run(name, func(t *testing.T) {
			caller := &whiteboardCoverageCaller{responses: map[string][]string{}}
			base := []string{"--node", "wb", "--page-id", "page-1", "--source", source}
			err := runWhiteboardCoverage(t, Diff, caller, "", append(base, args.args...)...)
			var structured *apperrors.Error
			if !errors.As(err, &structured) || structured.Reason != args.reason {
				t.Fatalf("error=%#v", err)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("local validation reached RPC: %#v", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageWhiteboardDiffComparisonAndPayloadBudget(t *testing.T) {
	identity := &identityMapFile{Version: 1, Nodes: map[string]string{"node": "real"}}
	before := map[string]any{"id": "real", "type": "text", "x": json.Number("10"), "text": "same"}
	after := map[string]any{"id": "node", "type": "text", "x": json.Number("10.4"), "text": "same"}
	semantic, unresolved := compareMappedNodes(before, after, identity, "semantic")
	if len(semantic) != 0 || len(unresolved) != 0 {
		t.Fatalf("semantic changes=%#v unresolved=%#v", semantic, unresolved)
	}
	exact, unresolved := compareMappedNodes(before, after, identity, "exact")
	if len(exact) != 1 || exact[0]["path"] != "/x" || len(unresolved) != 0 {
		t.Fatalf("exact changes=%#v unresolved=%#v", exact, unresolved)
	}

	largeBefore := strings.Repeat("a", maximumDiffOutputBytes/2+1000)
	largeAfter := strings.Repeat("b", maximumDiffOutputBytes/2+1000)
	projected := map[string]any{
		"nodeId": "wb", "revision": 1,
		"source": map[string]any{
			"schemaVersion": "1.0", "catalogVersion": "dml-v1",
			"pages": []any{map[string]any{
				"id": "page-1",
				"nodes": []any{map[string]any{
					"id": "real", "type": "text", "source": "page", "text": largeBefore,
				}},
			}},
		},
	}
	proposed, err := parseWhiteboardSource(`{"overwrite":true,"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"node","type":"text","text":"` + largeAfter + `"}]}}`)
	if err != nil {
		t.Fatal(err)
	}
	identity.Target = identityMapTarget{NodeID: "wb", PageID: "page-1"}
	result, err := buildWhiteboardDiff(projected, whiteboardStandaloneDiffCall(), proposed, identity, "exact", 100)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > maximumDiffOutputBytes || result["detailsTruncated"] != true {
		t.Fatalf("payload bytes=%d truncated=%v", len(encoded), result["detailsTruncated"])
	}
	reasons := result["truncationReasons"].([]any)
	if !containsAnyString(reasons, "payload_bytes") {
		t.Fatalf("truncationReasons=%#v", reasons)
	}
}

func TestCrossPlatformCoverageWhiteboardDiffNormalizesMappedRelationships(t *testing.T) {
	identity := &identityMapFile{Version: 1, Nodes: map[string]string{
		"frame": "real-frame", "left": "real-left", "line": "real-line",
	}}
	before := map[string]any{
		"id": "real-line", "type": "connector", "parentId": "real-frame", "routing": "straight",
		"start": map[string]any{"type": "node", "nodeRef": map[string]any{"scope": "document", "id": "real-left"}},
		"end":   map[string]any{"type": "point", "point": map[string]any{"x": json.Number("1"), "y": json.Number("2")}},
	}
	after := map[string]any{
		"id": "line", "type": "connector", "parentId": "frame", "routing": "straight",
		"start": map[string]any{"type": "node", "nodeRef": map[string]any{"scope": "request", "id": "left"}},
		"end":   map[string]any{"type": "point", "point": map[string]any{"x": json.Number("1.0"), "y": json.Number("2.0")}},
	}
	changes, unresolved := compareMappedNodes(before, after, identity, "exact")
	if len(changes) != 0 || len(unresolved) != 0 {
		t.Fatalf("relationship normalization changes=%#v unresolved=%#v", changes, unresolved)
	}
}

func whiteboardStandaloneDiffCall() whiteboardcore.Call {
	return whiteboardcore.Call{Kind: whiteboardcore.KindStandalone, Tool: toolQueryStandalone, Args: map[string]any{"nodeId": "wb", "view": "page", "pageId": "page-1"}}
}

func containsAnyString(values []any, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func TestCrossPlatformCoverageWhiteboardDiffWithoutViewEchoIsReadOnly(t *testing.T) {
	var response map[string]any
	if err := json.Unmarshal([]byte(validStandaloneWhiteboardQueryResponse("wb", "page", 2, `[{"id":"existing","type":"text","source":"page"}]`)), &response); err != nil {
		t.Fatal(err)
	}
	delete(response, "view")
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	caller := &whiteboardCoverageCaller{responses: map[string][]string{toolQueryStandalone: {string(encoded)}}}
	raw, err := runWhiteboardCoverageOutput(t, Diff, caller, "", "--node", "wb", "--page-id", "page-1", "--source", `{"overwrite":false,"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"new","type":"text"}]}}`)
	if err != nil {
		t.Fatal(err)
	}
	result := decodeWhiteboardBusinessData(t, raw)
	summary := result["summary"].(map[string]any)
	if result["complete"] != true || summary["executionAdded"] != float64(1) || summary["executionDeleted"] != float64(0) || summary["executionPreserved"] != float64(1) {
		t.Fatalf("result=%#v", result)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != toolQueryStandalone {
		t.Fatalf("diff must only read once: %#v", caller.calls)
	}
}
