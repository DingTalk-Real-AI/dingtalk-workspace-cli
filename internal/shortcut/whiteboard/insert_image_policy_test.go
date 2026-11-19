// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package whiteboard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	whiteboardcore "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/whiteboard"
)

func TestCrossPlatformCoverageInsertImagePrivateSnapshotWriteStopsBeforeUpload(t *testing.T) {
	caller := &whiteboardCoverageCaller{}
	rt := directWhiteboardRuntime(t, InsertImage, caller, "--node", "doc", "--part-id", "part", "--file", testPNG(t))
	occupied := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(occupied, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	err := uploadStagedImage(rt, &imagePreflight{fileBytes: []byte("validated image")}, occupied, nil)
	if err == nil || len(caller.calls) != 0 {
		t.Fatalf("private snapshot failure reached remote upload: %v, calls=%#v", err, caller.calls)
	}
	data, err := os.ReadFile(occupied)
	if err != nil || string(data) != "existing" {
		t.Fatalf("staging failure damaged existing path: %q, %v", data, err)
	}
}

func TestCrossPlatformCoverageInsertImageFinalDiffRiskPolicyStopsRemoteWrite(t *testing.T) {
	for _, tc := range []struct {
		name, node string
		blockers   int
		warnings   []any
	}{
		{name: "blocker", node: "doc", blockers: 1},
		{name: "unexpected-warning", node: "doc", warnings: []any{map[string]any{"reason": "future-risk"}}},
		{name: "invalid-update-target", node: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &whiteboardCoverageCaller{}
			rt := directWhiteboardRuntime(t, InsertImage, caller, "--node", tc.node, "--part-id", "part", "--file", testPNG(t))
			target := map[string]any{"kind": string(whiteboardcore.KindEmbedded), "nodeId": tc.node, "partId": "part", "pageId": "page-1"}
			preflight := &imagePreflight{target: target, snapshotDigest: "same", queryCall: whiteboardcore.Call{Kind: whiteboardcore.KindEmbedded}}
			diff := map[string]any{"target": target, "snapshotDigest": "same", "summary": map[string]any{"blockerCount": tc.blockers}, "warningSummary": tc.warnings}
			err := commitImageAfterDiff(rt, preflight, &parsedUpdate{NodesJSON: "[]"}, "res-1", "receipt.json", nil, diff)
			if err == nil || !strings.Contains(err.Error(), "图片资源已上传") || len(caller.calls) != 0 {
				t.Fatalf("unsafe final diff reached remote write: %v, calls=%#v", err, caller.calls)
			}
		})
	}
}
