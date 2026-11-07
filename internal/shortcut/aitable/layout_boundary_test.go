// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"errors"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageLayoutReadBoundaries(t *testing.T) {
	layout := `{"i":"C","x":0,"y":0,"w":12,"h":4}`
	dashboard := `{"data":{"baseId":"B","dashboardId":"D","meta":{"schemaVersion":2,"schemaVersionTypeVerified":true},"layout":[` + layout + `,{"i":"child","parentId":"P"}]}}`
	cases := []struct {
		name, command, receipt string
		args                   []string
		callErr                error
		wantErr                bool
	}{
		{name: "diagnose with design and region", command: "+dashboard-diagnose", receipt: dashboard, args: []string{"--expected-layout", "[" + layout + "]", "--regions", `[{"id":"r","x":0,"y":0,"w":12,"h":4}]`}},
		{name: "diagnose dependency", command: "+dashboard-diagnose", callErr: errors.New("read timeout"), wantErr: true},
		{name: "diagnose wrong target", command: "+dashboard-diagnose", receipt: `{"baseId":"other"}`, wantErr: true},
		{name: "diagnose missing layout", command: "+dashboard-diagnose", receipt: `{"data":{"baseId":"B","dashboardId":"D","meta":{"schemaVersion":2,"schemaVersionTypeVerified":true}}}`, wantErr: true},
		{name: "bad expected JSON", command: "+dashboard-diagnose", receipt: dashboard, args: []string{"--expected-layout", "{}"}, wantErr: true},
		{name: "bad expected shape", command: "+dashboard-diagnose", receipt: dashboard, args: []string{"--expected-layout", `[{"i":"C"}]`}, wantErr: true},
		{name: "bad region JSON", command: "+dashboard-diagnose", receipt: dashboard, args: []string{"--regions", "{}"}, wantErr: true},
		{name: "bad region geometry", command: "+dashboard-diagnose", receipt: dashboard, args: []string{"--regions", `[{"id":"r","x":0,"y":0,"w":0,"h":4}]`}, wantErr: true},
		{name: "bad region field", command: "+dashboard-diagnose", receipt: dashboard, args: []string{"--regions", `[{"id":"r","x":0,"y":0,"w":1,"h":1,"unexpected":true}]`}, wantErr: true},
		{name: "bad region identity", command: "+dashboard-diagnose", receipt: dashboard, args: []string{"--regions", `[{"id":"","x":0,"y":0,"w":1,"h":1}]`}, wantErr: true},
		{name: "preview dry", command: "+dashboard-preview", args: []string{"--dry-run"}, wantErr: true},
		{name: "preview dependency", command: "+dashboard-preview", callErr: errors.New("timeout"), wantErr: true},
		{name: "preview mismatch", command: "+dashboard-preview", receipt: `{}`, wantErr: true},
		{name: "preview no before", command: "+dashboard-preview", receipt: `{"baseId":"B","dashboardId":"D","preview":true,"layoutFingerprint":"fp","layout":[]}`, wantErr: true},
		{name: "preview no after", command: "+dashboard-preview", receipt: `{"baseId":"B","dashboardId":"D","preview":true,"layoutFingerprint":"fp","beforeLayout":[]}`, wantErr: true},
		{name: "preview valid", command: "+dashboard-preview", receipt: `{"baseId":"B","dashboardId":"D","preview":true,"layoutFingerprint":"fp","beforeLayout":[],"layout":[]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: tc.receipt, err: tc.callErr}}}
			args := append([]string{"--base-id", "B", "--dashboard-id", "D"}, tc.args...)
			out, err := runAITableCompositeCLI(t, caller, tc.command, args...)
			if (err != nil) != tc.wantErr {
				t.Fatalf("out=%s err=%v", out, err)
			}
			if !tc.wantErr && tc.command == "+dashboard-diagnose" && (!strings.Contains(out, `"excludedChildItems": 1`) || !strings.Contains(out, `"designVerified": true`) || !strings.Contains(out, `"regionCoverageVerified": true`)) {
				t.Fatalf("lost design evidence: %s", out)
			}
		})
	}
}

func TestCrossPlatformCoverageReadBatchFailures(t *testing.T) {
	for _, tc := range []struct {
		name, requests, width string
		dry                   bool
		readErr               error
	}{
		{name: "dry", requests: `[{"tool":"get_base","arguments":{"baseId":"B"}}]`, width: "1", dry: true},
		{name: "malformed", requests: "{}", width: "1"},
		{name: "bad width", requests: `[{"tool":"get_base","arguments":{"baseId":"B"}}]`, width: "5"},
		{name: "unsafe tool", requests: `[{"tool":"create_dashboard","arguments":{"baseId":"B"}}]`, width: "1"},
		{name: "partial failure", requests: `[{"tool":"get_base","arguments":{"baseId":"B"}}]`, width: "1", readErr: errors.New("timeout")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{err: tc.readErr}}}
			args := []string{"--base-id", "B", "--requests", tc.requests, "--concurrency", tc.width}
			if tc.dry {
				args = append(args, "--dry-run")
			}
			if _, err := runAITableCompositeCLI(t, caller, "+read-batch", args...); err == nil {
				t.Fatal("unsafe request succeeded")
			}
		})
	}
}
