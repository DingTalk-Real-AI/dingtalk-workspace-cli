// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package a2ui

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCrossPlatformCoverageAicardPreflightBoundaries(t *testing.T) {
	p := testProtocol(t)
	create := `{"version":"v1.0","createSurface":{"surfaceId":"s","catalogId":"https://dingtalk.com/card/a2ui/catalogs/public/catalog.json"}}`
	data := `{"version":"v1.0","updateDataModel":{"surfaceId":"s","path":"/","value":{}}}`
	content := `{"version":"v1.0","updateComponents":{"surfaceId":"s","components":[{"id":"root","component":"Text","text":"测试"}]}}`
	for _, msgs := range [][]string{{create, data, content}, {create, content, data}, {create, data, content, data}} {
		if e := p.CheckNewCard(msgs); e != nil {
			t.Fatal(e)
		}
	}
	if p.CheckNewCard([]string{create, content}) == nil {
		t.Fatal("preflight must reject missing data initialization")
	}
	if p.CheckNewCard([]string{create, data, content, `{"version":"v1.0","updateDataModel":{"surfaceId":"s","path":"/"}}`}) == nil {
		t.Fatal("deleting the root must not count as initialization")
	}
	bad := `{"version":"v1.0","updateComponents":{"surfaceId":"s","components":[{"id":"root","component":"Image","url":"data:image/jpeg;base64,tool error!"}]}}`
	r, e := p.Preflight([]string{bad}, "resources")
	if e != nil || r["valid"] != false {
		t.Fatal(r, e)
	}
	r, e = p.Preflight([]string{content}, "resources")
	if e != nil || r["valid"] != true {
		t.Fatal(r, e)
	}
}

func TestCrossPlatformCoverageAicardBundleRoundtrip(t *testing.T) {
	p := testProtocol(t)
	names := []string{"Text", "Row", "Column", "ButtonGroup", "Tabs", "CheckBox", "promptText"}
	ordinary := p.ExplainMany(names, false)
	packed := p.ExplainMany(names, true)
	defs := packed["definitions"].(map[string]any)
	var unpack func(any) any
	unpack = func(v any) any {
		switch n := v.(type) {
		case map[string]any:
			if ref, ok := n["$contractRef"]; ok && len(n) == 1 {
				return unpack(defs[ref.(string)])
			}
			out := map[string]any{}
			for k, c := range n {
				out[k] = unpack(c)
			}
			return out
		case []any:
			out := []any{}
			for _, c := range n {
				out = append(out, unpack(c))
			}
			return out
		default:
			return v
		}
	}
	if !reflect.DeepEqual(ordinary["contracts"], unpack(packed["contracts"])) {
		t.Fatal("query deduplication lost structure")
	}
	a, _ := json.Marshal(ordinary)
	b, _ := json.Marshal(packed)
	if len(b) >= len(a) {
		t.Fatal("batch query did not reduce duplication")
	}
}
