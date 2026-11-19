// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package chartgen

import (
	"strings"
	"testing"
)

func TestReviewLargeNumbersAndFourSeries(t *testing.T) {
	input := []byte(`{"specVersion":"1.0","presetId":"G10","title":"Revenue","data":{"kind":"category","categories":["East"],"series":[{"name":"A","unit":"CNY","values":[1500000]},{"name":"B","values":[23456789]},{"name":"C","values":[3]},{"name":"D","values":[4]}]}}`)
	artifact, err := Build(input, "review")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(artifact.Summary, "1500000CNY") || !strings.Contains(artifact.Summary, "23456789") || strings.Contains(artifact.Summary, "e+") {
		t.Fatal(artifact.Summary)
	}
	if strings.Count(artifact.Summary, "(+1)") != 1 || strings.Contains(artifact.Summary, "more series") {
		t.Fatal(artifact.Summary)
	}
	for _, value := range []float64{1500000, 23456789, 0.0000001, -23456789} {
		if strings.ContainsAny(displayNumber(value), "eE") {
			t.Fatal(displayNumber(value))
		}
	}
}

func TestReviewExactFieldCasingAndMarkdown(t *testing.T) {
	var input Input
	if err := decodeStrict([]byte(`{"title":"one","TITLE":"two"}`), &input); err == nil {
		t.Fatal("accepted TITLE alias")
	}
	if got := escapeMarkdown("<img src=x>"); !strings.HasPrefix(got, `\<`) {
		t.Fatal(got)
	}
}
