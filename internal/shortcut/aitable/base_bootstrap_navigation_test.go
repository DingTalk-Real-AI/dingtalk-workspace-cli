// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCrossPlatformCoverageBaseBootstrapNavigation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		receipt string
		read    string
		want    map[string]any
	}{
		{
			name:    "readback navigation wins",
			receipt: `,"url":"https://alidocs.dingtalk.com/i/nodes/b?receipt","links":[{"objectType":"table","objectId":"t","relation":"target"}]`,
			read:    `,"url":"https://alidocs.dingtalk.com/i/nodes/b?readback","links":[],"appId":"app"`,
			want:    map[string]any{"url": "https://alidocs.dingtalk.com/i/nodes/b?readback", "links": []any{}, "appId": "app"},
		},
		{
			name:    "old reader retains creation link",
			receipt: `,"url":"https://alidocs.dingtalk.com/i/nodes/b?iframeQuery=entrance%3Ddata%26sheetId%3Dt","links":[{"objectType":"table","objectId":"t","relation":"target"}]`,
			want: map[string]any{
				"url":   "https://alidocs.dingtalk.com/i/nodes/b?iframeQuery=entrance%3Ddata%26sheetId%3Dt",
				"links": []any{map[string]any{"objectType": "table", "objectId": "t", "relation": "target"}},
			},
		},
		{
			name:    "explicit empty readback is authoritative",
			receipt: `,"url":"receipt","links":[{"objectId":"t"}],"appId":"app"`,
			read:    `,"url":null,"links":[],"appId":""`,
			want:    map[string]any{"url": nil, "links": []any{}, "appId": ""},
		},
		{name: "old server does not invent navigation", want: map[string]any{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
				{text: `{"baseId":"b"}`},
				{text: `{"baseId":"b","tables":[]}`},
				{text: `{"data":{"tableId":"t"` + tc.receipt + `}}`},
				{text: `{"tables":[{"tableId":"t","name":"任务"` + tc.read + `}]}`},
				{text: `{"fields":[]}`},
			}}
			out, err := runAITableCompositeCLI(t, caller, "+base-bootstrap", "--name", "项目", "--tables", marshalBootstrapTables(t, nil), "--yes")
			if err != nil {
				t.Fatal(err)
			}
			var envelope struct {
				Data compositeResult `json:"data"`
			}
			if err := json.Unmarshal([]byte(out), &envelope); err != nil {
				t.Fatal(err)
			}
			result := envelope.Data
			table := result.Result["tables"].([]any)[0].(map[string]any)
			got := map[string]any{}
			for _, key := range []string{"url", "links", "appId"} {
				if value, exists := table[key]; exists {
					got[key] = value
				}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("navigation = %#v, want %#v", got, tc.want)
			}
			if table["tableId"] != "t" || result.Verification["status"] != "verified" || len(caller.calls) != 5 {
				t.Fatalf("creation/readback contract changed: result=%#v calls=%#v", result, caller.calls)
			}
		})
	}
}
