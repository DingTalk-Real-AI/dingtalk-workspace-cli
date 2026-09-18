// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package whiteboard

import "testing"

func TestCrossPlatformCoverageTemplateValidationBoundaries(t *testing.T) {
	if ValidateTemplateRequestID("valid-1") != nil {
		t.Fatal("valid request rejected")
	}
	for _, n := range []int{0, 1, 50, 51} {
		if (ValidateTemplatePageSize(n) == nil) != (n >= 1 && n <= 50) {
			t.Fatalf("limit %d", n)
		}
	}
	for _, n := range []int{0, 1, 100, 101} {
		if (ValidateTemplateMaxPages(n) == nil) != (n >= 1 && n <= 100) {
			t.Fatalf("max-pages %d", n)
		}
	}
	for _, tc := range []struct {
		cursor string
		valid  bool
	}{{"", true}, {" 0 ", true}, {"12", true}, {"-1", false}, {"bad", false}} {
		if (ValidateTemplateCursor(tc.cursor) == nil) != tc.valid {
			t.Fatal(tc)
		}
	}
	for _, tool := range []string{PersonalTemplateSaveTool, TeamTemplateSaveTool, PersonalTemplateCreateTool, TeamTemplateCreateTool, PublicTemplateCreateTool} {
		if ValidateTemplatePreviewCall(ServerID, tool, map[string]any{"dryRun": true}) != nil {
			t.Fatal(tool)
		}
	}
	if ValidateTemplatePreviewCall("other", PersonalTemplateSaveTool, nil) == nil || ValidateTemplatePreviewCall(ServerID, "other", map[string]any{"dryRun": true}) == nil {
		t.Fatal("unsafe preview accepted")
	}
}
