// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/whiteboard"
)

func TestCrossPlatformCoverageWhiteboardReceiptDeliveredInFullAndCompactSchema(t *testing.T) {
	want, err := contract.NormalizeResultSpec(whiteboard.Update.Contract.Result, "whiteboard.shortcut_update")
	if err != nil {
		t.Fatal(err)
	}
	for _, compact := range []bool{false, true} {
		root := NewRootCommand()
		var stdout, stderr bytes.Buffer
		root.SetOut(&stdout)
		root.SetErr(&stderr)
		args := []string{"schema", "--cli-path", "whiteboard +update", "--format", "json"}
		if compact {
			args = append(args, "--compact")
		}
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatalf("schema compact=%v: %v; %s", compact, err, stderr.String())
		}
		var payload struct {
			Result *contract.ResultSpec `json:"result"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		got, err := contract.NormalizeResultSpec(payload.Result, "whiteboard.shortcut_update")
		if err != nil || got == nil {
			t.Fatalf("result missing or invalid, compact=%v: %v", compact, err)
		}
		var gotSchema, wantSchema any
		if err := json.Unmarshal(got.DataSchema, &gotSchema); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(want.DataSchema, &wantSchema); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(gotSchema, wantSchema) || !reflect.DeepEqual(got.Outcomes, want.Outcomes) || !reflect.DeepEqual(got.SensitivePaths, want.SensitivePaths) {
			t.Fatalf("compact=%v lost or changed receipt branch constraints", compact)
		}
	}
}
