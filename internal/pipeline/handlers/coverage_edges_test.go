package handlers

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
)

func TestCrossPlatformCoverageParamValueCoverageEdges(t *testing.T) {
	ctx := &pipeline.Context{
		Params: map[string]any{"ignored": "value", "unknown": "value"},
		Schema: makeSchema(map[string]any{"ignored": "not-a-schema"}),
	}
	if err := (ParamValueHandler{}).Handle(ctx); err != nil {
		t.Fatalf("Handle(): %v", err)
	}
	if err := (ParamValueHandler{}).Handle(&pipeline.Context{Params: map[string]any{"x": 1}, Schema: map[string]any{"type": "object"}}); err != nil {
		t.Fatalf("Handle(schema without properties): %v", err)
	}

	cases := []struct {
		name   string
		value  any
		schema map[string]any
		want   any
	}{
		{"bool non-string", 1, map[string]any{"type": "boolean"}, 1},
		{"integer parse error", "1,broken", map[string]any{"type": "integer"}, "1,broken"},
		{"number parse error", "1,broken", map[string]any{"type": "number"}, "1,broken"},
		{"date non-string", 1, map[string]any{"type": "string", "format": "date"}, 1},
		{"date empty", " ", map[string]any{"type": "string", "format": "date"}, " "},
		{"unix seconds", "1711699200", map[string]any{"type": "string", "format": "date-time"}, "2024-03-29T08:00:00Z"},
		{"invalid date", "not-a-date", map[string]any{"type": "string", "format": "date"}, "not-a-date"},
		{"ambiguous enum", "a", map[string]any{"enum": []any{"A", "a"}}, "a"},
		{"enum ignores non-string", "A", map[string]any{"enum": []any{1, "a"}}, "a"},
		{"unknown type", "value", map[string]any{"type": "object"}, "value"},
	}
	for _, tc := range cases {
		got, _ := normaliseValue(tc.value, tc.schema)
		if got != tc.want {
			t.Errorf("%s: got %#v, want %#v", tc.name, got, tc.want)
		}
	}
	if got := classifyCorrection(map[string]any{"type": "object"}); got != "value" {
		t.Fatalf("unknown correction class = %q", got)
	}
}

func TestCrossPlatformCoverageNameAndStickyCoverageEdges(t *testing.T) {
	if _, ok := tryFuzzyMatch("--", map[string]bool{}, nil); ok {
		t.Fatal("bare -- should not fuzzy match")
	}
	if _, ok := trySplitSticky("--", nil); ok {
		t.Fatal("bare -- should not split")
	}
	if _, ok := trySplitSticky("--unknown42", map[string]pipeline.FlagInfo{"known": {Type: "integer"}}); ok {
		t.Fatal("unknown sticky prefix should not split")
	}
	if _, ok := trySplitSticky("--pageSize", map[string]pipeline.FlagInfo{"page-size": {Type: "integer"}}); ok {
		t.Fatal("known camel-case flag should not split")
	}
}
