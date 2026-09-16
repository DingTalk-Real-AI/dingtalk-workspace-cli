// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package opennodes

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageOpenNodesRunLineSeparators(t *testing.T) {
	for _, separator := range []string{"\n", "\r", "\r\n", "\u2028", "\u2029"} {
		for _, title := range []bool{false, true} {
			text := map[string]any{"blocks": []any{map[string]any{"type": "paragraph", "runs": []any{
				map[string]any{"text": "safe"},
				map[string]any{"text": "上午" + separator + "待安排"},
			}}}}
			node := map[string]any{"id": "day0", "type": "frame", "text": text}
			wantPath := "/source/nodes/0/text/blocks/0/runs/1/text"
			if title {
				delete(node, "text")
				node["title"] = map[string]any{"text": text}
				wantPath = "/source/nodes/0/title/text/blocks/0/runs/1/text"
			}
			raw, err := json.Marshal(Source{SchemaVersion: SchemaVersion, CatalogVersion: CatalogVersion, Nodes: []map[string]any{node}})
			if err != nil {
				t.Fatal(err)
			}
			for _, wrapped := range []bool{false, true} {
				input := raw
				if wrapped {
					input = append(append([]byte("{\"source\":"), raw...), '}')
				}
				_, err := Parse(input)
				if err == nil || !strings.Contains(err.Error(), wantPath) || !strings.Contains(err.Error(), "day0") || !strings.Contains(err.Error(), "paragraph") {
					t.Fatalf("separator=%q title=%v wrapped=%v: %v", separator, title, wrapped, err)
				}
			}
		}
	}
	valid := []byte(`{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"card","text":{"blocks":[{"type":"paragraph","runs":[{"text":"上午","marks":{"bold":true}}]},{"type":"paragraph","runs":[{"text":""}]},{"type":"paragraph","runs":[{"text":"待安排"}]}]}}]}`)
	source, err := Parse(valid)
	if err != nil {
		t.Fatal(err)
	}
	if len(source.Nodes[0]["text"].(map[string]any)["blocks"].([]any)) != 3 {
		t.Fatal("paragraphs must not be rewritten")
	}
}

func TestCrossPlatformCoverageOpenNodesParseCanonicalAndDigest(t *testing.T) {
	direct := []byte(`{"nodes":[{"type":"shape","id":"n1","x":1.50}],"catalogVersion":"dml-v1","schemaVersion":"1.0"}`)
	wrapper := []byte(`{"overwrite":true,"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"x":1.50,"id":"n1","type":"shape"}]}}`)
	left, err := Parse(direct)
	if err != nil {
		t.Fatal(err)
	}
	right, err := Parse(wrapper)
	if err != nil {
		t.Fatal(err)
	}
	leftDigest, err := DigestSource(left)
	if err != nil {
		t.Fatal(err)
	}
	rightDigest, err := DigestSource(right)
	if err != nil {
		t.Fatal(err)
	}
	if leftDigest != rightDigest || !ValidDigest(leftDigest) || !EqualDigest(strings.ToUpper(leftDigest), rightDigest) {
		t.Fatalf("digests left=%q right=%q", leftDigest, rightDigest)
	}
	canonical, err := CanonicalJSON(left)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n1","type":"shape","x":1.50}]}`
	if string(canonical) != want {
		t.Fatalf("canonical=%s want=%s", canonical, want)
	}
	appendDigest, err := DigestUpdate(false, left.Nodes)
	if err != nil {
		t.Fatal(err)
	}
	overwriteDigest, err := DigestUpdate(true, left.Nodes)
	if err != nil {
		t.Fatal(err)
	}
	if appendDigest == overwriteDigest {
		t.Fatal("update digest did not bind overwrite intent")
	}
}

func TestCrossPlatformCoverageOpenNodesParseRejectsInvalidSources(t *testing.T) {
	tests := []string{
		`null`, `{`, `{} {}`, `{}`,
		`{"schemaVersion":"2.0","catalogVersion":"dml-v1","nodes":[]}`,
		`{"schemaVersion":"1.0","catalogVersion":"bad","nodes":[]}`,
		`{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":null}`,
		`{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[1]}`,
		`{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[],"unknown":true}`,
		`{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[]},"unknown":true}`,
	}
	for _, source := range tests {
		if _, err := Parse([]byte(source)); err == nil {
			t.Fatalf("Parse(%s) unexpectedly succeeded", source)
		}
	}
	if ValidDigest("sha256:bad") {
		t.Fatal("short digest accepted")
	}
}
