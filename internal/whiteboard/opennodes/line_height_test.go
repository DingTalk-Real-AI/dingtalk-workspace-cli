// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package opennodes

import (
	"errors"
	"math"
	"testing"
)

func TestCrossPlatformCoverageOpenNodesLineHeight(t *testing.T) {
	for _, blockType := range []string{"paragraph", "bulletList", "orderedList"} {
		for _, value := range []any{nil, "2", true, -1.0, 0.0, 0.9, math.NaN(), math.Inf(1), 1e308, 1.0, 1.15, 20.0 / 14.0, 2.0} {
			for _, title := range []bool{false, true} {
				block := map[string]any{"type": blockType, "lineHeight": value, "runs": []any{map[string]any{"text": "检验流程配置", "marks": map[string]any{"fontSize": 16.0}}}}
				text := map[string]any{"blocks": []any{block}}
				node := map[string]any{"id": "line-height", "text": text}
				path := "/source/nodes/0/text/blocks/0/lineHeight"
				if title {
					node = map[string]any{"id": "line-height", "title": map[string]any{"text": text}}
					path = "/source/nodes/0/title/text/blocks/0/lineHeight"
				}
				err := ValidateText([]map[string]any{node})
				number, ok := value.(float64)
				valid := ok && number >= 1 && !math.IsNaN(number) && !math.IsInf(number*16, 0)
				if valid {
					if err != nil {
						t.Fatalf("valid %v: %v", value, err)
					}
					continue
				}
				var issue *NodeValidationError
				if !errors.As(err, &issue) || issue.Code != "invalidValue" || issue.Path != path {
					t.Fatalf("value=%v title=%v: want invalidValue@%s, got %v", value, title, path, err)
				}
			}
		}
	}
	// Products may be finite individually but overflow when paragraph heights accumulate.
	text := map[string]any{"blocks": []any{
		map[string]any{"type": "paragraph", "runs": []any{map[string]any{"text": "", "marks": map[string]any{"fontSize": 1e308}}}},
		map[string]any{"type": "paragraph", "runs": []any{map[string]any{"text": "", "marks": map[string]any{"fontSize": 1e308}}}},
	}}
	var issue *NodeValidationError
	if err := ValidateText([]map[string]any{{"text": text}}); !errors.As(err, &issue) || issue.Path != "/source/nodes/0/text/blocks" {
		t.Fatalf("aggregate overflow: %v", err)
	}
	text["blocks"] = []any{map[string]any{"type": "paragraph", "runs": []any{map[string]any{"text": ""}}}}
	text["padding"] = math.MaxFloat64
	if err := ValidateText([]map[string]any{{"text": text}}); !errors.As(err, &issue) || issue.Path != "/source/nodes/0/text/padding" {
		t.Fatalf("padding overflow: %v", err)
	}
}
