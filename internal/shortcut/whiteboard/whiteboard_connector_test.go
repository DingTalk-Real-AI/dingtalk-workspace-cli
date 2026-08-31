// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package whiteboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageWhiteboardConnectorWriteReadback(t *testing.T) {
	const requested = `[{"id":"left","type":"shape","geometry":"dml:rect"},{"id":"right","type":"shape","geometry":"dml:rect"},{"id":"line","type":"connector","start":{"type":"node","nodeRef":{"scope":"request","id":"left"},"anchor":{"mode":"fixed","side":"right"}},"end":{"type":"node","nodeRef":{"scope":"request","id":"right"},"anchor":{"mode":"auto"},"marker":{"catalogId":"arrow.filled"}},"routing":"straight"}]`
	// Independent server fixture: never use the production normalizer to build
	// expected evidence. Query-only fields may be added, not silently compared away.
	const readback = `[{"id":"real-right","type":"shape","geometry":"dml:rect"},{"id":"real-line","type":"connector","start":{"type":"node","nodeRef":{"scope":"document","id":"real-left"},"anchor":{"mode":"fixed","side":"right","position":0.5},"resolvedPoint":{"x":100,"y":50}},"end":{"type":"node","nodeRef":{"scope":"document","id":"real-right"},"anchor":{"mode":"auto"},"marker":{"catalogId":"arrow.filled"},"resolvedPoint":{"x":200,"y":50}},"routing":"straight"},{"id":"real-left","type":"shape","geometry":"dml:rect"}]`
	requestIDs := []string{"left", "right", "line"}
	realIDs := []string{"real-left", "real-right", "real-line"}
	for _, mode := range []string{"append", "overwrite"} {
		for _, tc := range []struct {
			name, nodes, reason string
		}{
			{"mapped endpoints", readback, ""},
			{"wrong start ID", strings.Replace(readback, `"id":"real-left"`, `"id":"real-right"`, 1), "readback_field_mismatch"},
			{"wrong end ID", strings.Replace(readback, `"scope":"document","id":"real-right"`, `"scope":"document","id":"real-left"`, 1), "readback_field_mismatch"},
			{"temporary ID retained", strings.Replace(readback, `"scope":"document","id":"real-left"`, `"scope":"document","id":"left"`, 1), "readback_field_mismatch"},
			{"wrong start scope", strings.Replace(readback, `"scope":"document"`, `"scope":"request"`, 1), "readback_field_mismatch"},
			{"missing end scope", strings.Replace(readback, `"scope":"document","id":"real-right"`, `"id":"real-right"`, 1), "readback_field_missing"},
			{"wrong marker", strings.Replace(readback, "arrow.filled", "arrow.open", 1), "readback_field_mismatch"},
			{"wrong anchor", strings.Replace(readback, `"side":"right"`, `"side":"left"`, 1), "readback_field_mismatch"},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				caller := &whiteboardCoverageCaller{responses: map[string][]string{
					toolUpdate: {validWhiteboardUpdateResponse(mode, requestIDs, realIDs, 0)},
					toolQuery:  {validWhiteboardQueryResponse(tc.nodes)},
				}}
				source := fmt.Sprintf(`{"overwrite":%t,"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":%s}}`, mode == "overwrite", requested)
				stdout, err := runWhiteboardCoverageOutput(t, Update, caller, "", "--node", "doc", "--part-id", "part", "--source", source, "--yes")
				if len(caller.calls) != 2 || caller.calls[0].tool != toolUpdate || caller.calls[1].tool != toolQuery {
					t.Fatalf("want exactly one write and one read, got %#v; err=%v", caller.calls, err)
				}
				for _, call := range caller.calls {
					if call.server != serverWhiteboard || call.args["nodeId"] != "doc" || call.args["partId"] != "part" {
						t.Fatalf("changed server or target: %#v", call)
					}
				}
				if caller.calls[0].args["mode"] != mode {
					t.Fatalf("changed write mode: %#v", caller.calls[0])
				}
				// Verification must not rewrite request-scoped references on the wire.
				var sent, want any
				if err := json.Unmarshal([]byte(caller.calls[0].args["nodes"].(string)), &sent); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal([]byte(requested), &want); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(sent, want) {
					t.Fatalf("mutated submitted nodes: %#v", sent)
				}
				if tc.reason != "" {
					var failure *apperrors.Error
					if !errors.As(err, &failure) || failure.Reason != tc.reason || !failure.RetryableSet || failure.Retryable || failure.ExecutionStarted == nil || !*failure.ExecutionStarted || failure.Details["commitState"] != "committed" {
						t.Fatalf("lost committed/no-replay failure: %#v, err=%v", failure, err)
					}
					receipt := failure.Details["receipt"].(map[string]any)
					if !reflect.DeepEqual(receipt["createdNodeIds"], realIDs) || !reflect.DeepEqual(receipt["idMap"], map[string]string{"left": "real-left", "right": "real-right", "line": "real-line"}) || len(stdout) != 0 {
						t.Fatalf("lost reconciliation evidence or emitted success: %#v; output=%s", receipt, stdout)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				var result struct {
					OK      bool           `json:"ok"`
					Outcome string         `json:"outcome"`
					Data    map[string]any `json:"data"`
				}
				if err := json.Unmarshal(stdout, &result); err != nil {
					t.Fatal(err)
				}
				if !result.OK || result.Outcome != "success" || result.Data["verified"] != true || result.Data["verifiedNodeCount"] != float64(3) || result.Data["nodeId"] != "doc" || result.Data["partId"] != "part" || result.Data["mode"] != mode {
					t.Fatalf("missing verified success: %s", stdout)
				}
				if _, exists := result.Data["source"]; exists {
					t.Fatalf("success leaked full board snapshot: %s", stdout)
				}
				if result.Data["receipt"].(map[string]any)["idMap"].(map[string]any)["line"] != "real-line" {
					t.Fatalf("success lost real connector identity: %s", stdout)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageWhiteboardConnectorValidationBranches(t *testing.T) {
	const pointStart = `{"type":"point","point":{"x":0,"y":0}}`
	const pointEnd = `{"type":"point","point":{"x":100,"y":100}}`
	const nodeStart = `{"type":"node","nodeRef":{"scope":"request","id":"left"}}`
	for _, tc := range []struct {
		name, start, end, routing, extra, errorField string
	}{
		{"missing endpoint", `null`, pointEnd, "straight", "", ".start"},
		{"empty endpoint", `{}`, pointEnd, "straight", "", ".start"},
		{"missing type", `{"point":{"x":0,"y":0}}`, pointEnd, "straight", "", ".type"},
		{"unknown type", `{"type":"other"}`, pointEnd, "straight", "", ".type"},
		{"point with node ref", `{"type":"point","point":{"x":0,"y":0},"nodeRef":{}}`, pointEnd, "straight", "", "nodeRef"},
		{"point with anchor", `{"type":"point","point":{"x":0,"y":0},"anchor":{}}`, pointEnd, "straight", "", "anchor"},
		{"point missing coordinates", `{"type":"point","point":{}}`, pointEnd, "straight", "", ".point"},
		{"node with point", `{"type":"node","point":{"x":0,"y":0}}`, pointEnd, "straight", "", "point"},
		{"node missing ref", `{"type":"node"}`, pointEnd, "straight", "", ".nodeRef"},
		{"empty anchor", `{"type":"node","nodeRef":{"scope":"request","id":"left"},"anchor":{}}`, pointEnd, "straight", "", ".anchor"},
		{"anchor missing mode", `{"type":"node","nodeRef":{"scope":"request","id":"left"},"anchor":{"side":"left"}}`, pointEnd, "straight", "", ".mode"},
		{"auto anchor with side", `{"type":"node","nodeRef":{"scope":"request","id":"left"},"anchor":{"mode":"auto","side":"left"}}`, pointEnd, "straight", "", ".side"},
		{"unknown anchor mode", `{"type":"node","nodeRef":{"scope":"request","id":"left"},"anchor":{"mode":"other"}}`, pointEnd, "straight", "", ".mode"},
		{"empty marker", pointStart, `{"type":"point","point":{"x":100,"y":100},"marker":{}}`, "straight", "", ".marker"},
		{"invalid routing", pointStart, pointEnd, "other", "", ".routing"},
		{"invalid waypoints type", pointStart, pointEnd, "polyline", `,"waypoints":{}`, ".waypoints"},
		{"empty polyline", pointStart, pointEnd, "polyline", `,"waypoints":[]`, ".waypoints"},
		{"invalid waypoint", pointStart, pointEnd, "curve", `,"waypoints":[{"x":1}]`, ".waypoints[0].y"},
		{"valid polyline", pointStart, pointEnd, "polyline", `,"waypoints":[{"x":50,"y":50}]`, ""},
		{"valid mixed endpoints", nodeStart, pointEnd, "curve", `,"waypoints":[{"x":50,"y":50}]`, ""},
		{"valid reverse mixed endpoints", pointStart, nodeStart, "straight", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nodes := fmt.Sprintf(`[{"id":"left","type":"shape"},{"id":"line","type":"connector","start":%s,"end":%s,"routing":%q%s}]`, tc.start, tc.end, tc.routing, tc.extra)
			source := `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":` + nodes + `}}`
			caller := &whiteboardCoverageCaller{responses: map[string][]string{}}
			if tc.errorField == "" {
				readback := strings.ReplaceAll(nodes, `"id":"left"`, `"id":"real-left"`)
				readback = strings.ReplaceAll(readback, `"id":"line"`, `"id":"real-line"`)
				readback = strings.ReplaceAll(readback, `"scope":"request"`, `"scope":"document"`)
				caller.responses[toolUpdate] = []string{validWhiteboardUpdateResponse("append", []string{"left", "line"}, []string{"real-left", "real-line"}, 0)}
				caller.responses[toolQuery] = []string{validWhiteboardQueryResponse(readback)}
			}
			err := runWhiteboardCoverage(t, Update, caller, "", "--node", "doc", "--part-id", "part", "--source", source, "--yes")
			if tc.errorField != "" {
				var failure *apperrors.Error
				if !errors.As(err, &failure) || failure.Category != apperrors.CategoryValidation || !strings.Contains(err.Error(), tc.errorField) || len(caller.calls) != 0 {
					t.Fatalf("want local validation for %s, got err=%v calls=%#v", tc.errorField, err, caller.calls)
				}
			} else if err != nil || len(caller.calls) != 2 {
				t.Fatalf("valid connector did not complete one write/read: err=%v calls=%#v", err, caller.calls)
			}
		})
	}
}
