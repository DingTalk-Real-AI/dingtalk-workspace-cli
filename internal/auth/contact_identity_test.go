// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package auth

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCrossPlatformCoverageContactIdentityCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name, body, expectedCorp, wantUser string
		ok                                 bool
	}{
		{"invalid", `{`, "", "", false},
		{"empty result", `{"result":[]}`, "", "", false},
		{"empty identity", `{"result":[{}]}`, "", "", false},
		{"legacy fallback", `{"result":[{"orgEmployeeModel":{"orgUserId":" old ","name":"Old"}}]}`, "corp", "old", true},
		{"lowercase", `{"result":[{"orgEmployeeModel":{"corpId":"corp","userid":" lower "}}]}`, "", "lower", true},
		{"exact organization", `{"result":[{"orgEmployeeModel":{"corpId":"other","userId":"wrong"}},{"orgEmployeeModel":{"corpId":"corp","userId":" user ","orgName":" Corp ","orgUserName":" Name "}}]}`, "corp", "user", true},
		{"ambiguous legacy", `{"result":[{"orgEmployeeModel":{"userId":"one"}},{"orgEmployeeModel":{"userId":"two"}}]}`, "corp", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ContactProfileIdentityFromJSON([]byte(tc.body), tc.expectedCorp)
			if ok != tc.ok || got.UserID != tc.wantUser {
				t.Fatalf("identity=%+v ok=%v", got, ok)
			}
		})
	}
	if _, ok := ContactProfileIdentityFromToolResult(nil); ok {
		t.Fatal("accepted nil result")
	}
	if _, ok := ContactProfileIdentityFromToolResult(&edition.ToolResult{Content: []edition.ContentBlock{{Text: " "}, {Text: "{"}}}); ok {
		t.Fatal("accepted invalid blocks")
	}
	got, ok := ContactProfileIdentityFromToolResult(&edition.ToolResult{Content: []edition.ContentBlock{{Text: `{"result":[{"orgEmployeeModel":{"corpId":"corp","userId":"user"}}]}`}}}, "corp")
	if !ok || got.CorpID != "corp" || got.UserID != "user" {
		t.Fatalf("identity=%+v ok=%v", got, ok)
	}
}

func TestCrossPlatformCoverageManagedIdentityRejectsAmbiguousEvidence(t *testing.T) {
	valid := `{"result":[{"orgEmployeeModel":{"corpId":"corp","userId":"user","userid":"user","orgName":" Org ","name":" Name "}}]}`
	for _, tc := range []struct {
		name, corp string
		blocks     []edition.ContentBlock
		ok         bool
	}{
		{"missing corporation", "", nil, false},
		{"no evidence", "corp", nil, false},
		{"non text and blank", "corp", []edition.ContentBlock{{Type: "image", Text: valid}, {Type: "text", Text: " "}}, false},
		{"invalid json", "corp", []edition.ContentBlock{{Type: "text", Text: "{"}}, false},
		{"wrong corporation", "other", []edition.ContentBlock{{Type: "text", Text: valid}}, false},
		{"duplicate blocks", "corp", []edition.ContentBlock{{Type: "text", Text: valid}, {Type: "text", Text: valid}}, false},
		{"missing staff identity", "corp", []edition.ContentBlock{{Type: "text", Text: `{"result":[{"orgEmployeeModel":{"corpId":"corp","orgUserId":"user"}}]}`}}, false},
		{"conflicting staff identity", "corp", []edition.ContentBlock{{Type: "text", Text: `{"result":[{"orgEmployeeModel":{"corpId":"corp","userId":"one","userid":"two"}}]}`}}, false},
		{"exact identity", " corp ", []edition.ContentBlock{{Type: "text", Text: valid}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ManagedIdentityFromToolResult(&edition.ToolResult{Content: tc.blocks}, tc.corp)
			if (err == nil) != tc.ok {
				t.Fatalf("identity=%+v error=%v", got, err)
			}
			if tc.ok && (got.CorpID != "corp" || got.UserID != "user" || got.CorpName != "Org" || got.UserName != "Name") {
				t.Fatalf("identity=%+v", got)
			}
		})
	}
	if _, err := ManagedIdentityFromToolResult(nil, "corp"); err == nil {
		t.Fatal("accepted nil")
	}
}
