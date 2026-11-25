package helpers_test

import (
	"strings"
	"testing"

	previewfixture "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/test/testsupport/mobile-image-preview-noauth"
)

// The media-insert receipt claims `verified` after a structural server-side
// readback only; client rendering depends on local network reachability of
// OSS/alidocs preview domains and must stay explicitly unknown.
func TestDocMediaInsertReceiptClientVisibility(t *testing.T) {
	for _, tc := range []struct {
		name, file, mime, warningKeyword string
		size                             int64
		inline                           bool
	}{
		{"attachment", "asset.pdf", "application/pdf", "附件", 4, false},
		{"inline-image", "asset.png", "image/png", "图片", 4, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := previewfixture.New(t)
			previewfixture.MustRunShared(t, caller, tc.file, tc.mime, tc.size)
			previewfixture.AssertElement(t, caller, 0, tc.inline, true, tc.mime, tc.file)
			previewfixture.AssertCalls(t, caller, false, 1)

			receipt, ok := caller.Result.(map[string]any)
			if !ok {
				t.Fatalf("receipt is not a map: %#v", caller.Result)
			}
			if receipt["ok"] != true || receipt["status"] != "success" || receipt["complete"] != true {
				t.Fatalf("receipt status regressed: %#v", receipt)
			}
			data, _ := receipt["data"].(map[string]any)
			if data == nil {
				t.Fatalf("receipt missing data: %#v", receipt)
			}
			if data["verified"] != true || data["inserted"] != true {
				t.Fatalf("server-side verification no longer reported: %#v", data)
			}
			if data["verificationScope"] != "structural" {
				t.Fatalf("verificationScope = %#v, want structural", data["verificationScope"])
			}
			if data["clientVisibility"] != "unknown" {
				t.Fatalf("clientVisibility = %#v, want unknown", data["clientVisibility"])
			}
			note, _ := data["clientVisibilityNote"].(string)
			if note == "" {
				t.Fatalf("clientVisibilityNote empty: %#v", data)
			}
			warnings, _ := receipt["warnings"].([]string)
			if len(warnings) != 1 || !strings.Contains(warnings[0], tc.warningKeyword) {
				t.Fatalf("warnings = %#v, want one mentioning %q", receipt["warnings"], tc.warningKeyword)
			}
			if !strings.Contains(warnings[0], "不要重复插入") {
				t.Fatalf("warning lacks do-not-reinsert guidance: %#v", warnings)
			}
			steps, _ := receipt["steps"].([]map[string]any)
			var verifyStep map[string]any
			for _, step := range steps {
				if step["name"] == "verify" {
					verifyStep = step
				}
			}
			if verifyStep == nil || verifyStep["status"] != "success" || verifyStep["scope"] != "structural" {
				t.Fatalf("verify step missing or unscoped: %#v", steps)
			}
		})
	}
}
