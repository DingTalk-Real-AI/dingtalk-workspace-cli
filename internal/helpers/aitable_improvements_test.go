// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"bytes"
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

func TestCrossPlatformCoverageAitableImprovementTemplateDocumentationReachesCLI(t *testing.T) {
	common := "// 公共说明\n"
	line := "{\n// 原始注释\n\"LINE\":{\"chartType\":\"LINE\",\"url\":\"https://example.com/a//b\"}\n}"
	for _, name := range []string{"with documentation", "old server", "malformed documentation", "malformed object", "malformed common", "malformed byType", "malformed type text", "incomplete documentation", "wrong data type", "missing template", "invalid template"} {
		t.Run(name, func(t *testing.T) {
			body := map[string]any{"LINE": map[string]any{"chartType": "LINE"}}
			var data any = body
			chartTypes := "LINE"
			wantTemplates := 1
			switch name {
			case "with documentation":
				body["documentation"] = map[string]any{"common": common, "byType": map[string]any{"LINE": line, "PIE": "not requested"}}
			case "malformed documentation":
				body["documentation"] = nil
			case "malformed object":
				body["documentation"] = "upstream content must not be logged"
			case "malformed common":
				body["documentation"] = map[string]any{"common": " ", "byType": map[string]any{"LINE": line}}
			case "malformed byType":
				body["documentation"] = map[string]any{"common": common, "byType": nil}
			case "malformed type text":
				body["documentation"] = map[string]any{"common": common, "byType": map[string]any{"LINE": 1}}
			case "incomplete documentation":
				chartTypes, wantTemplates = "LINE,PIE", 2
				body["PIE"] = map[string]any{"chartType": "PIE"}
				body["documentation"] = map[string]any{"common": common, "byType": map[string]any{"LINE": line}}
			case "wrong data type":
				data = []any{}
			case "missing template":
				delete(body, "LINE")
			case "invalid template":
				body["LINE"] = map[string]any{"chartType": "PIE"}
			}
			receipt, err := json.Marshal(map[string]any{"status": "success", "data": data})
			if err != nil {
				t.Fatal(err)
			}
			caller := &aitableAppCaller{response: string(receipt)}
			InitDepsForTest(t, caller)
			var stdout, stderr bytes.Buffer
			GetFormatter().SetWriters(&stdout, &stderr)
			root := newAitableCommand()
			root.SilenceErrors = true
			root.SilenceUsage = true
			root.PersistentFlags().String("format", "json", "output")
			ctx, _ := output.WithResultStore(context.Background())
			root.SetContext(ctx)
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			root.SetArgs([]string{"chart", "templates", "--chart-types", chartTypes})
			cmd, err := corecmd.ExecuteCForTest(root)
			wantErr := name == "wrong data type" || name == "missing template" || name == "invalid template"
			if (err != nil) != wantErr || len(caller.calls) != 1 || caller.calls[0].toolName != "get_dashboard_widget_templates" {
				t.Fatalf("err=%v calls=%#v", err, caller.calls)
			}
			if wantErr {
				if stdout.Len() != 0 {
					t.Fatal("partial output leaked")
				}
				return
			}
			wantDiagnostic := strings.HasPrefix(name, "malformed") || name == "incomplete documentation"
			if wantDiagnostic {
				if !strings.Contains(stderr.String(), "说明已整体丢弃，已保留合法 templates") || strings.Count(stderr.String(), "\n") != 1 || strings.Contains(stderr.String(), "upstream content must not be logged") {
					t.Fatalf("missing or unsafe diagnostic: %s", stderr.String())
				}
			} else if stderr.Len() != 0 {
				t.Fatalf("unexpected diagnostic: %s", stderr.String())
			}
			if code, emitted, err := output.EmitStoredResult(cmd); err != nil || !emitted || code != 0 {
				t.Fatalf("emit: code=%d err=%v", code, err)
			}
			var envelope map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatalf("decode: %v stdout=%s", err, stdout.String())
			}
			result := envelope["data"].(map[string]any)
			if envelope["ok"] != true || envelope["outcome"] != "success" || len(result["templates"].(map[string]any)) != wantTemplates {
				t.Fatalf("unexpected result: %#v", envelope)
			}
			documentation, exists := result["documentation"]
			if exists != (name == "with documentation") {
				t.Fatalf("documentation availability changed: %#v", result)
			}
			if exists {
				docs := documentation.(map[string]any)
				byType := docs["byType"].(map[string]any)
				if docs["common"] != common || len(byType) != 1 || byType["LINE"] != line {
					t.Fatalf("raw documentation changed: %#v", docs)
				}
			}
		})
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
