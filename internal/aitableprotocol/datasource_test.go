// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitableprotocol

import (
	"encoding/json"
	"testing"
)

func TestCrossPlatformCoverageDatasourceSourceConfigPreservesOpaqueJSON(t *testing.T) {
	raw := " \n" + `{"processCode":"P","future":{"id":9007199254740993,"items":[null,true]}}` + " \n"
	got, err := DatasourceSourceConfig(map[string]any{"status": "success", "data": map[string]any{"datasourceType": "OA", "sourceConfig": raw}})
	if err != nil || got != raw {
		t.Fatalf("config = %q, err = %v", got, err)
	}
}

func TestCrossPlatformCoverageDatasourceSourceConfigFailsClosed(t *testing.T) {
	for _, raw := range []string{
		`null`, `{}`, `{"status":"error"}`, `{"status":"success","error":{"code":"denied"}}`,
		`{"status":"success","data":null}`, `{"status":"success","data":[]}`,
		`{"status":"success","data":{"sourceConfig":"{\"x\":1}"}}`,
		`{"status":"success","data":{"datasourceType":"OTHER","sourceConfig":"{\"x\":1}"}}`,
		`{"status":"success","data":{"datasourceType":"OA"}}`,
		`{"status":"success","data":{"datasourceType":"OA","sourceConfig":{}}}`,
	} {
		t.Run(raw, func(t *testing.T) {
			var payload map[string]any
			if err := json.Unmarshal([]byte(raw), &payload); err != nil {
				t.Fatal(err)
			}
			if got, err := DatasourceSourceConfig(payload); err == nil || got != "" {
				t.Fatalf("got %q, err %v", got, err)
			}
		})
	}
	for _, raw := range []string{"", " ", "null", "{}", "[]", `"text"`, "true", "1", "invalid", `{"x":1} {"y":2}`} {
		payload := map[string]any{"status": "success", "data": map[string]any{"datasourceType": "OA", "sourceConfig": raw}}
		if got, err := DatasourceSourceConfig(payload); err == nil || got != "" {
			t.Fatalf("sourceConfig %q: got %q, err %v", raw, got, err)
		}
	}
}
