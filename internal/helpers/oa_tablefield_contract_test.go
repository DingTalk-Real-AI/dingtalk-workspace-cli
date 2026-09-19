// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/skills"
)

// Use the shipped examples as inputs so a regression in either skill layout
// fails before an agent sends an incorrectly encoded table to DingTalk.
func TestCrossPlatformCoverageOATableFieldDocumentedPayloads(t *testing.T) {
	purchaseRows := [][]map[string]string{
		{{"name": "商品名", "value": "笔记本"}, {"name": "数量", "value": "2"}},
		{{"name": "商品名", "value": "钢笔"}, {"name": "数量", "value": "1"}},
	}
	for _, tc := range []struct {
		path     string
		summary  bool
		wantRows [][]map[string]string
	}{
		{"multi/dingtalk-misc/references/oa/oa-form-components.md", false, purchaseRows},
		{"mono/references/products/oa/oa-form-components.md", false, purchaseRows},
		{"multi/dingtalk-misc/references/oa-create.md", false, purchaseRows},
		{"mono/references/products/oa.md", true, [][]map[string]string{
			{{"name": "子控件名", "value": "值1"}},
			{{"name": "子控件名", "value": "值2"}},
		}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			doc, err := skills.FS.ReadFile(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			component := documentedOATableField(t, string(doc), tc.summary)
			var rows [][]map[string]string
			if err := json.Unmarshal([]byte(component["value"]), &rows); err != nil {
				t.Fatalf("TableField value must encode a two-dimensional name/value array: %v", err)
			}
			if !reflect.DeepEqual(rows, tc.wantRows) {
				t.Fatalf("documented rows = %#v, want %#v", rows, tc.wantRows)
			}

			for _, mode := range []string{"form-values", "request"} {
				t.Run(mode, func(t *testing.T) {
					args := []string{"approval", "create-instance", "--yes"}
					if mode == "form-values" {
						values, err := json.Marshal(map[string]string{component["name"]: component["value"]})
						if err != nil {
							t.Fatal(err)
						}
						args = append(args, "--process-code", "PROC-TEST", "--form-values", string(values))
					} else {
						request, err := json.Marshal(map[string]any{
							"processCode":         "PROC-TEST",
							"deptId":              -1,
							"formComponentValues": []map[string]string{component},
						})
						if err != nil {
							t.Fatal(err)
						}
						args = append(args, "--request", string(request))
					}

					caller := &scriptedToolCaller{}
					if err := executeOACommand(t, caller, args...); err != nil {
						t.Fatal(err)
					}
					if caller.calls != 1 || caller.server != "oa" || caller.tool != "start_process_instance" {
						t.Fatalf("call = %s/%s (%d calls)", caller.server, caller.tool, caller.calls)
					}
					// Decode the wire shape, independent of the Go slice type used
					// internally by the simple and advanced request builders.
					wire, err := json.Marshal(caller.args["ProcessInstanceCreationPopRequest"])
					if err != nil {
						t.Fatal(err)
					}
					var got struct {
						ProcessCode         string              `json:"processCode"`
						FormComponentValues []map[string]string `json:"formComponentValues"`
					}
					if err := json.Unmarshal(wire, &got); err != nil {
						t.Fatal(err)
					}
					if got.ProcessCode != "PROC-TEST" || !reflect.DeepEqual(got.FormComponentValues, []map[string]string{component}) {
						t.Fatalf("documented TableField changed during request construction: %s", wire)
					}
				})
			}
		})
	}
}

func documentedOATableField(t *testing.T, doc string, summary bool) map[string]string {
	t.Helper()
	if summary {
		for _, line := range strings.Split(doc, "\n") {
			cells := strings.Split(line, "|")
			if len(cells) >= 6 && strings.TrimSpace(cells[2]) == "`TableField`" {
				return map[string]string{
					"name":  "采购明细",
					"value": strings.Trim(strings.TrimSpace(cells[4]), "`'"),
				}
			}
		}
	} else {
		for _, block := range strings.Split(doc, "```json\n")[1:] {
			raw, _, found := strings.Cut(block, "\n```")
			if !found {
				t.Fatal("unclosed JSON example")
			}
			var component map[string]string
			if err := json.Unmarshal([]byte(raw), &component); err == nil && component["name"] == "采购明细" {
				return component
			}
		}
	}
	t.Fatal("missing TableField submission example")
	return nil
}
