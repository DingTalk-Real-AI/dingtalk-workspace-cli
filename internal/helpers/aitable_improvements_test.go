// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

func executeAitableImprovementForTest(t *testing.T, caller *aitableAppCaller, args ...string) error {
	t.Helper()
	InitDepsForTest(t, caller)
	deps.Out.w = io.Discard
	root := newAitableCommand()
	root.PersistentFlags().Bool("yes", false, "confirmation")
	root.PersistentFlags().String("format", "json", "output")
	root.PersistentFlags().Bool("dry-run", false, "preview")
	root.SetIn(strings.NewReader(""))
	ctx, _ := output.WithResultStore(context.Background())
	root.SetContext(ctx)
	root.SetArgs(args)
	return corecmd.ExecuteForTest(root)
}

func TestCrossPlatformCoverageAitableImprovementLeavesRejectBadBatchBeforeDispatch(t *testing.T) {
	valid := `{"config":{"chartType":"LINE"},"layout":{"x":0,"y":0,"w":12,"h":4}}`
	for _, raw := range []string{"[null]", "[]", "[" + strings.Repeat(valid+",", 20) + valid + "]", `[{"config":{"chartType":"line"},"layout":{"x":0,"y":0,"w":12,"h":4}}]`, `[{"config":{"chartType":"LINE"},"layout":{"x":0,"y":0,"w":0,"h":4}}]`} {
		caller := &aitableAppCaller{}
		if err := executeAitableImprovementForTest(t, caller, "chart", "batch-create", "--base-id", "B", "--dashboard-id", "D", "--charts", raw, "--yes"); err == nil || len(caller.calls) != 0 {
			t.Fatalf("invalid batch reached remote: %s, %v, %#v", raw, err, caller.calls)
		}
	}
}

func TestCrossPlatformCoverageAitableImprovementTemplatesKeepTypedPayload(t *testing.T) {
	caller := &aitableAppCaller{response: `{"status":"success","data":{"LINE":{"chartType":"LINE"},"PIE":{"chartType":"PIE"}}}`}
	if err := executeAitableImprovementForTest(t, caller, "chart", "templates", "--chart-types", "LINE,PIE"); err != nil || len(caller.calls) != 1 {
		t.Fatalf("template call: %v %#v", err, caller.calls)
	}
	raw, _ := json.Marshal(caller.calls[0].args["chartTypes"])
	if string(raw) != `["LINE","PIE"]` {
		t.Fatal(string(raw))
	}
}
