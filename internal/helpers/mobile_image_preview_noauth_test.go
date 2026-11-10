package helpers_test

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/configmeta"
	previewfixture "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/test/testsupport/mobile-image-preview-noauth"
)

const mobileImagePreviewNoauthEnv = previewfixture.Env

// Contract group and evidence slug: mobile-image-preview-noauth.
// These tests exercise existing public entry points; only the MCP and OSS
// boundaries are replaced.
func TestCrossPlatformCoverageMobileImagePreviewNoauth(t *testing.T) {
	t.Run("mobile-image-preview-noauth", func(t *testing.T) {
		for _, tc := range []struct {
			name, value string
			unset       bool
		}{
			{name: "unset", unset: true},
			{name: "empty"},
			{name: "zero", value: "0"},
			{name: "false", value: "false"},
			{name: "invalid", value: "invalid"},
			{name: "true", value: "true"},
			{name: "whitespace", value: " 1 "},
			{name: "one", value: "1"},
		} {
			t.Run("legacy-environment-ignored/"+tc.name, func(t *testing.T) {
				t.Setenv(mobileImagePreviewNoauthEnv, tc.value)
				if tc.unset {
					if err := os.Unsetenv(mobileImagePreviewNoauthEnv); err != nil {
						t.Fatal(err)
					}
				}
				caller := previewfixture.New(t)
				previewfixture.MustRunShared(t, caller, "asset.png", "image/png", 4)
				previewfixture.AssertElement(t, caller, 0, true, true, "image/png", "asset.png")
				previewfixture.AssertCalls(t, caller, false, 1)
			})
		}

		for _, tc := range []struct {
			name, initial, changed string
		}{
			{"one-to-zero", "1", "0"},
			{"zero-to-one", "0", "1"},
		} {
			t.Run("legacy-environment-change-ignored/"+tc.name, func(t *testing.T) {
				t.Setenv(mobileImagePreviewNoauthEnv, tc.initial)
				caller := previewfixture.New(t)
				// The first network boundary changes the environment after the
				// operation has started, before the element is constructed.
				caller.OnCredentials = func() { t.Setenv(mobileImagePreviewNoauthEnv, tc.changed) }
				previewfixture.MustRunShared(t, caller, "asset.png", "image/png", 4)
				previewfixture.AssertElement(t, caller, 0, true, true, "image/png", "asset.png")
				caller.OnCredentials = nil
				previewfixture.MustRunShared(t, caller, "asset.png", "image/png", 4)
				previewfixture.AssertElement(t, caller, 1, true, true, "image/png", "asset.png")
				previewfixture.AssertCalls(t, caller, false, 2)
			})
		}

		for _, enabled := range []bool{false, true} {
			for _, tc := range []struct {
				name, file, mime string
				size             int64
				noURL, inline    bool
			}{
				{"small", "asset.png", "image/png", 4, false, true},
				{"at-20-mib", "asset.png", "image/png", 20 * 1024 * 1024, false, true},
				{"over-20-mib", "asset.png", "image/png", 20*1024*1024 + 1, false, false},
				{"empty-resource-url", "asset.png", "image/png", 4, true, false},
				{"attachment", "asset.pdf", "application/pdf", 4, false, false},
			} {
				t.Run(fmt.Sprintf("branch-boundaries/enabled-%t/%s", enabled, tc.name), func(t *testing.T) {
					value := "0"
					if enabled {
						value = "1"
					}
					t.Setenv(mobileImagePreviewNoauthEnv, value)
					caller := previewfixture.New(t)
					if tc.noURL {
						caller.ResourceURL = ""
					}
					previewfixture.MustRunShared(t, caller, tc.file, tc.mime, tc.size)
					previewfixture.AssertElement(t, caller, 0, tc.inline, true, tc.mime, tc.file)
					previewfixture.AssertCalls(t, caller, false, 1)
				})
			}
		}

		for _, stage := range []string{"get_doc_attachment_upload_info", "upload", "insert_document_block"} {
			t.Run("failure-no-retry/"+stage, func(t *testing.T) {
				t.Setenv(mobileImagePreviewNoauthEnv, "1")
				caller := previewfixture.New(t)
				caller.FailStage = stage
				err := previewfixture.RunShared(t, caller, "asset.png", "image/png", 4)
				if err == nil {
					t.Fatal("injected boundary failure unexpectedly succeeded")
				}
				wantTools := []string{"get_doc_attachment_upload_info"}
				wantUploads := 0
				if stage != "get_doc_attachment_upload_info" {
					wantUploads = 1
				}
				if stage == "insert_document_block" {
					wantTools = append(wantTools, "insert_document_block")
				}
				if !reflect.DeepEqual(caller.Tools, wantTools) || caller.Uploads != wantUploads {
					t.Fatalf("failure retried or crossed a later boundary: tools=%v uploads=%d", caller.Tools, caller.Uploads)
				}
			})
		}

		t.Run("runtime-config-removed", func(t *testing.T) {
			for _, item := range configmeta.All() {
				if item.Name == mobileImagePreviewNoauthEnv {
					t.Fatalf("obsolete rollout metadata remains registered: %#v", item)
				}
			}
		})
	})
}

func TestCrossPlatformCoverageMobileImagePreviewNoauthLegacyReceipts(t *testing.T) {
	for _, wrapped := range []bool{false, true} {
		for _, missing := range []struct {
			name  string
			value any
			omit  bool
		}{
			{"omitted", nil, true}, {"null", nil, false}, {"empty", "", false},
			{"blank", " \t\n", false}, {"number", 17, false}, {"bool", false, false},
			{"object", map[string]any{}, false}, {"array", []any{}, false},
		} {
			for _, size := range []int64{4, 20 * 1024 * 1024} {
				t.Run(fmt.Sprintf("src-only/result-%t/%s/size-%d", wrapped, missing.name, size), func(t *testing.T) {
					caller := previewfixture.New(t)
					caller.WrapReceipt = wrapped
					caller.UploadInfo = map[string]any{"uploadUrl": "https://upload.example.test/put", "resourceUrl": caller.ResourceURL}
					if !missing.omit {
						caller.UploadInfo["resourceId"] = missing.value
					}
					previewfixture.MustRunShared(t, caller, "asset.png", "image/png", size)
					previewfixture.AssertElement(t, caller, 0, true, false, "image/png", "asset.png")
					previewfixture.AssertCalls(t, caller, false, 1)
					data := caller.Result.(map[string]any)["data"].(map[string]any)
					if data["verified"] != true || data["blockId"] != "media-1" || data["resourceId"] != "" || data["resourceUrl"] != caller.ResourceURL {
						t.Fatalf("URL-only readback did not produce a verified receipt: %#v", data)
					}
				})
			}
			for _, branch := range []struct {
				name, file, mime string
				size             int64
				noURL            bool
			}{
				{"ordinary-file", "asset.pdf", "application/pdf", 4, false},
				{"over-20-mib", "asset.png", "image/png", 20*1024*1024 + 1, false},
				{"no-resource-url", "asset.png", "image/png", 4, true},
			} {
				t.Run(fmt.Sprintf("attachment-rejected/result-%t/%s/%s", wrapped, missing.name, branch.name), func(t *testing.T) {
					caller := previewfixture.New(t)
					caller.WrapReceipt = wrapped
					caller.UploadInfo = map[string]any{"uploadUrl": "https://upload.example.test/put", "resourceUrl": caller.ResourceURL}
					if !missing.omit {
						caller.UploadInfo["resourceId"] = missing.value
					}
					if branch.noURL {
						delete(caller.UploadInfo, "resourceUrl")
					}
					// If old code wrongly inserts, stop its existing readback wait;
					// the assertions still require zero PUTs and insertions.
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					caller.Context, caller.OnReadback = ctx, cancel
					if err := previewfixture.RunShared(t, caller, branch.file, branch.mime, branch.size); err == nil {
						t.Fatal("missing ID attachment unexpectedly succeeded")
					}
					if !reflect.DeepEqual(caller.Tools, []string{"get_doc_attachment_upload_info"}) || caller.Uploads != 0 || len(caller.Elements) != 0 {
						t.Fatalf("invalid attachment crossed upload boundary: %#v", caller)
					}
				})
			}
		}

		for _, missingURL := range []struct {
			name  string
			value any
			omit  bool
		}{
			{"omitted", nil, true}, {"null", nil, false}, {"empty", "", false},
		} {
			t.Run(fmt.Sprintf("URL-missing-ID-present/result-%t/%s", wrapped, missingURL.name), func(t *testing.T) {
				caller := previewfixture.New(t)
				caller.WrapReceipt = wrapped
				caller.UploadInfo = map[string]any{"uploadUrl": "https://upload.example.test/put", "resourceId": "opaque-id:+1"}
				if !missingURL.omit {
					caller.UploadInfo["resourceUrl"] = missingURL.value
				}
				previewfixture.MustRunShared(t, caller, "asset.png", "image/png", 4)
				previewfixture.AssertElement(t, caller, 0, false, true, "image/png", "asset.png")
				previewfixture.AssertCalls(t, caller, false, 1)
			})
		}

		for _, resourceURL := range []string{"http://media.example.test/opaque", "https://media.example.test/opaque"} {
			t.Run(fmt.Sprintf("all-inline/result-%t/%s", wrapped, resourceURL), func(t *testing.T) {
				caller := previewfixture.New(t)
				caller.WrapReceipt = wrapped
				caller.ResourceURL = resourceURL
				caller.UploadInfo = map[string]any{"uploadUrl": "https://upload.example.test/put", "resourceId": " opaque/id:+1 ", "resourceUrl": resourceURL}
				previewfixture.MustRunShared(t, caller, "asset.png", "image/png", 4)
				previewfixture.AssertElement(t, caller, 0, true, true, "image/png", "asset.png")
				previewfixture.AssertCalls(t, caller, false, 1)
			})
		}

		t.Run(fmt.Sprintf("wrong-readback-URL/result-%t", wrapped), func(t *testing.T) {
			caller := previewfixture.New(t)
			caller.WrapReceipt = wrapped
			caller.UploadInfo = map[string]any{"uploadUrl": "https://upload.example.test/put", "resourceUrl": caller.ResourceURL}
			caller.ReadbackURL = "https://media.example.test/different-image"
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			caller.Context, caller.OnReadback = ctx, cancel
			if err := previewfixture.RunShared(t, caller, "asset.png", "image/png", 4); err == nil {
				t.Fatal("same block ID with wrong src unexpectedly verified")
			}
			previewfixture.AssertElement(t, caller, 0, true, false, "image/png", "asset.png")
			previewfixture.AssertCalls(t, caller, false, 1)
			if caller.Result != nil {
				t.Fatalf("mismatched URL emitted success: %#v", caller.Result)
			}
		})
	}

	for _, raw := range []string{`{`, `[]`, `null`, `{}`, `{"resourceId":"real-id","resourceUrl":"https://media.example.test/not-the-resource-id"}`, `{"uploadUrl":null,"resourceId":"real-id","resourceUrl":"https://media.example.test/not-the-resource-id"}`, `{"uploadUrl":"","resourceId":"real-id","resourceUrl":"https://media.example.test/not-the-resource-id"}`, `{"uploadUrl":" ","resourceId":"real-id","resourceUrl":"https://media.example.test/not-the-resource-id"}`, `{"uploadUrl":9,"resourceId":"real-id","resourceUrl":"https://media.example.test/not-the-resource-id"}`, `{"result":{"uploadUrl":"","resourceId":"real-id","resourceUrl":"https://media.example.test/not-the-resource-id"}}`} {
		t.Run("invalid-upload-receipt/"+raw, func(t *testing.T) {
			caller := previewfixture.New(t)
			caller.RawUploadText = raw
			if err := previewfixture.RunShared(t, caller, "asset.png", "image/png", 4); err == nil {
				t.Fatal("invalid upload receipt unexpectedly succeeded")
			}
			if !reflect.DeepEqual(caller.Tools, []string{"get_doc_attachment_upload_info"}) || caller.Uploads != 0 || len(caller.Elements) != 0 {
				t.Fatalf("bad receipt crossed upload boundary: %#v", caller)
			}
		})
	}
}
