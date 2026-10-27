// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package mind

import (
	"fmt"
	"strings"
	"testing"
)

const validSource = `{"schemaVersion":"1.0","catalogVersion":"mind-v1","pages":[{"id":"p","title":"计划","nodes":[{"id":"child","type":"topic","text":"","parentId":"root"},{"id":"root","type":"topic","text":"根主题"}]}]}`

func TestCrossPlatformCoverageMindSourceValidatesCompleteTree(t *testing.T) {
	source, err := ParseSource([]byte(validSource))
	if err != nil {
		t.Fatal(err)
	}
	if source.Pages[0].Nodes[0].ID != "child" || source.Pages[0].Nodes[0].Text != "" {
		t.Fatal("input order or empty text changed")
	}
	tests := []struct{ name, source string }{
		{"duplicate key", strings.Replace(validSource, `"schemaVersion":"1.0"`, `"schemaVersion":"1.0","schemaVersion":"1.0"`, 1)},
		{"wrong casing", strings.Replace(validSource, `"text":"根主题"`, `"Text":"根主题"`, 1)},
		{"unknown field", strings.Replace(validSource, `"text":"根主题"`, `"text":"根主题","checkpoint":{}`, 1)},
		{"null text", strings.Replace(validSource, `"text":"根主题"`, `"text":null`, 1)},
		{"duplicate topic", strings.Replace(validSource, `"id":"child"`, `"id":"root"`, 1)},
		{"missing parent", strings.Replace(validSource, `"parentId":"root"`, `"parentId":"missing"`, 1)},
		{"multiple roots", strings.Replace(validSource, `,"parentId":"root"`, "", 1)},
		{"no root", strings.Replace(validSource, `"text":"根主题"`, `"text":"根主题","parentId":"child"`, 1)},
		{"version", strings.Replace(validSource, `mind-v1`, `mind-node-v2`, 1)},
		{"empty page", `{"schemaVersion":"1.0","catalogVersion":"mind-v1","pages":[{"id":"p","title":"x","nodes":[]}]}`},
		{"trailing JSON", validSource + `{}`},
		{"oversized", strings.Repeat(" ", MaxSourceBytes+1)},
		{"invalid utf8", strings.Replace(validSource, "根主题", string([]byte{0xff}), 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseSource([]byte(tt.source)); err == nil {
				t.Fatal("invalid source accepted")
			}
		})
	}
}
func TestCrossPlatformCoverageMindSourceDepthAndCrossPageBoundaries(t *testing.T) {
	makeSource := func(count int) string {
		nodes := []string{`{"id":"n0","type":"topic","text":"root"}`}
		for i := 1; i < count; i++ {
			nodes = append(nodes, fmt.Sprintf(`{"id":"n%d","type":"topic","text":"x","parentId":"n%d"}`, i, i-1))
		}
		return `{"schemaVersion":"1.0","catalogVersion":"mind-v1","pages":[{"id":"p","title":"x","nodes":[` + strings.Join(nodes, ",") + `]}]}`
	}
	if _, err := ParseSource([]byte(makeSource(64))); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSource([]byte(makeSource(65))); err == nil {
		t.Fatal("depth 65 accepted")
	}
	cycle := strings.Replace(makeSource(4), `"parentId":"n0"`, `"parentId":"n3"`, 1)
	if _, err := ParseSource([]byte(cycle)); err == nil {
		t.Fatal("cycle accepted")
	}
	two := strings.TrimSuffix(validSource, `]}`) + `,{"id":"p2","title":"second","nodes":[{"id":"other","type":"topic","text":"root"},{"id":"orphan","type":"topic","text":"x","parentId":"root"}]}]}`
	if _, err := ParseSource([]byte(two)); err == nil {
		t.Fatal("cross-page parent accepted")
	}
}
func TestCrossPlatformCoverageMindTopicAndVersionBoundaries(t *testing.T) {
	if _, err := ParseTopic([]byte(`{"id":"tmp","type":"topic","text":""}`)); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"id":"tmp","type":"topic","text":"x","parentId":"r"}`, `{"id":"tmp","type":"topic"}`, `{"id":"tmp","type":"topic","text":null}`, `{"id":"bad id","type":"topic","text":"x"}`} {
		if _, err := ParseTopic([]byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	for _, version := range []string{"0", "2147483647"} {
		if err := ValidateVersion(version); err != nil {
			t.Fatal(err)
		}
	}
	for _, version := range []string{"", "-1", "1.0", "2147483648", "00", "01", " 1", "+1"} {
		if err := ValidateVersion(version); err == nil {
			t.Fatalf("accepted version %q", version)
		}
	}
}

func TestCrossPlatformCoverageMindRejectsUnsupportedTopicTypes(t *testing.T) {
	for _, topicType := range []string{"", "Topic", "image"} {
		t.Run(fmt.Sprintf("type=%q", topicType), func(t *testing.T) {
			raw := fmt.Sprintf(`{"id":"tmp","type":%q,"text":""}`, topicType)
			if _, err := ParseTopic([]byte(raw)); err == nil || !strings.Contains(err.Error(), "type must be topic") {
				t.Fatalf("expected unsupported topic type error, got %v", err)
			}
			source := strings.Replace(validSource, `"type":"topic"`, fmt.Sprintf(`"type":%q`, topicType), 1)
			if _, err := ParseSource([]byte(source)); err == nil || !strings.Contains(err.Error(), "type must be topic") {
				t.Fatalf("expected unsupported source topic type error, got %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageMindSourcePageAndTopicCountLimits(t *testing.T) {
	source := func(pages []string) string {
		return `{"schemaVersion":"1.0","catalogVersion":"mind-v1","pages":[` + strings.Join(pages, ",") + `]}`
	}
	var pages []string
	for i := 0; i < 20; i++ {
		pages = append(pages, fmt.Sprintf(`{"id":"p%d","title":"Page","nodes":[{"id":"r%d","type":"topic","text":"Root"}]}`, i, i))
	}
	if _, err := ParseSource([]byte(source(pages))); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSource([]byte(source(append(pages, `{"id":"p20","title":"Page","nodes":[{"id":"r20","type":"topic","text":"Root"}]}`)))); err == nil {
		t.Fatal("21 pages accepted")
	}
	nodes := []string{`{"id":"root","type":"topic","text":"Root"}`}
	for i := 1; i <= 10000; i++ {
		nodes = append(nodes, fmt.Sprintf(`{"id":"n%d","type":"topic","text":"","parentId":"root"}`, i))
	}
	page := func(n int) string { return `{"id":"p","title":"Page","nodes":[` + strings.Join(nodes[:n], ",") + `]}` }
	if _, err := ParseSource([]byte(source([]string{page(10000)}))); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSource([]byte(source([]string{page(10001)}))); err == nil {
		t.Fatal("10001 topics accepted")
	}
}

func TestCrossPlatformCoverageMindCurrentNameTitleAndCursorBoundaries(t *testing.T) {
	for _, value := range []string{strings.Repeat("中", 128), strings.Repeat("A", 128), " 计划 "} {
		if err := ValidateName(value); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []string{"", "  ", strings.Repeat("中", 129), strings.Repeat("A", 129), string([]byte{0xff})} {
		if err := ValidateName(value); err == nil {
			t.Fatalf("accepted name %q", value)
		}
	}
	for _, title := range []string{strings.Repeat("中", 255), " "} {
		if _, err := ParseSource([]byte(strings.Replace(validSource, `"title":"计划"`, `"title":"`+title+`"`, 1))); err != nil {
			t.Fatal(err)
		}
	}
	for _, title := range []string{"", strings.Repeat("中", 256)} {
		if _, err := ParseSource([]byte(strings.Replace(validSource, `"title":"计划"`, `"title":"`+title+`"`, 1))); err == nil {
			t.Fatal("accepted invalid page title")
		}
	}
	for _, cursor := range []string{"", "mindc1_" + strings.Repeat("a", 43)} {
		if err := ValidateCursor(cursor); err != nil {
			t.Fatal(err)
		}
	}
	for _, cursor := range []string{"opaque", " ", "mindc1_" + strings.Repeat("a", 42), "mindc1_" + strings.Repeat("a", 44), "mindc1_" + strings.Repeat("+", 43)} {
		if err := ValidateCursor(cursor); err == nil {
			t.Fatalf("accepted cursor %q", cursor)
		}
	}
	if _, err := ParseTopic([]byte(`{"id":"n","type":"topic","text":""}` + strings.Repeat(" ", MaxNodeJSONBytes))); err == nil {
		t.Fatal("accepted oversized node JSON")
	}
}

// Reject malformed trees before any downstream RPC, including errors inside
// otherwise valid pages rather than only invalid top-level JSON.
func TestCrossPlatformCoverageMindNestedInputFailures(t *testing.T) {
	cases := []string{
		`null`, `[]`,
		strings.Replace(validSource, `"schemaVersion":"1.0"`, `"schemaVersion":1`, 1),
		strings.Replace(validSource, `"pages":[`, `"pages":true,"unused":[`, 1),
		strings.Replace(validSource, `"id":"p"`, `"id":"bad id"`, 1),
		strings.Replace(validSource, `"id":"p"`, `"id":4`, 1),
		strings.Replace(validSource, `"title":"计划"`, `"title":false`, 1),
		strings.Replace(validSource, `"nodes":[`, `"nodes":true,"unused":[`, 1),
		strings.Replace(validSource, `"text":"根主题"`, `"text":42`, 1),
		strings.Replace(validSource, `"text":"根主题"`, `"text":"`+strings.Repeat("x", 10001)+`"`, 1),
		strings.Replace(validSource, `"parentId":"root"`, `"parentId":"bad id"`, 1),
		strings.Replace(validSource, `"parentId":"root"`, `"parentId":null`, 1),
		strings.Replace(validSource, `"parentId":"root"`, `"parentId":""`, 1),
		`{"schemaVersion":"1.0","catalogVersion":"mind-v1","pages":{}}`,
		`{"schemaVersion":"1.0","catalogVersion":"mind-v1","pages":[null]}`,
		`{"schemaVersion":"1.0","catalogVersion":"mind-v1","pages":[{"id":"p","title":"x","nodes":{}}]}`,
		`{"schemaVersion":"1.0","catalogVersion":"mind-v1","pages":[{"id":"p","title":"x","nodes":[{"id":"r","type":"topic","text":""}]},{"id":"p","title":"x","nodes":[]}]}`,
	}
	for i, raw := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			if _, err := ParseSource([]byte(raw)); err == nil {
				t.Fatalf("accepted malformed tree: %.150s", raw)
			}
		})
	}
	// The object decoder is shared with topic parsing; scalars and null must
	// never become a topic with implicit zero-valued required fields.
	for _, raw := range []string{`{"id":"r","id":"other","type":"topic","text":""}`, `[]`, `null`, `{"id":2,"type":"topic","text":""}`} {
		if _, err := ParseTopic([]byte(raw)); err == nil {
			t.Fatalf("accepted topic %s", raw)
		}
	}
}
