package aitable

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageFormShareUpdatePhaseOne(t *testing.T) {
	for _, tc := range []struct {
		flags   []string
		invalid bool
	}{
		{[]string{"--form-start-at", "2026-09-22T09:00:00+08:00", "--form-end-at", "0", "--submit-record-view-mode", "1", "--form-cover-resource-id", "rid-1"}, false},
		{[]string{"--form-start-at", "bad"}, true},
		{[]string{"--form-start-at", "0", "--form-start-time", "0"}, true},
		{[]string{"--form-end-at", "0", "--form-end-time", "0"}, true},
		{[]string{"--form-cover", "https://example.test/cover.png"}, true},
		{[]string{"--form-cover", "https://img.alicdn.com:8443/cover.png"}, true},
		{[]string{"--form-cover", "https://img.alicdn.com/cover.png),url(https://127.0.0.1/private"}, true},
		{[]string{"--form-cover", "https://example.test/cover.png", "--form-cover-file", "x.png"}, true},
		{[]string{"--submit-record-view-mode", "2"}, true},
		{[]string{"--submit-record-view-mode", "-1"}, true},
		{[]string{"--submit-record-view-mode", "99"}, true},
		{[]string{"--form-cover", "https://example.test/cover.png?signature=secret"}, true},
		{[]string{"--form-cover", ""}, true},
		{[]string{"--form-cover", " \t"}, true},
		{[]string{"--form-cover", " https://example.test/cover.png"}, true},
		{[]string{"--form-cover", "https://example.test/core/api/resources/rid"}, true},
		{[]string{"--form-cover", "https://example.test/cover.png?"}, true},
		{[]string{"--form-cover", "https://example.test/cover.png#"}, true},
		{[]string{"--form-cover-resource-id", ""}, true},
		{[]string{"--form-cover-resource-id", " \t"}, true},
		{[]string{"--form-cover-resource-id", " rid"}, true},
	} {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"success":true,"data":{"baseId":"base","tableId":"t","viewId":"v","enabled":true,"cpSynced":true,"shareFormUuid":"s","status":1,"formCover":"https://example.test/cover.png"}}`}}}
		args := append([]string{"--base-id", "base", "--table-id", "t", "--view-id", "v", "--yes"}, tc.flags...)
		out, err := runAITableCompositeCLI(t, caller, "+form-share-update", args...)
		if (err != nil) != tc.invalid {
			t.Fatalf("%v: %v", tc.flags, err)
		}
		if tc.invalid {
			if apperrors.ExitCode(err) != apperrors.ExitCodeValidation {
				t.Fatalf("invalid form configuration lost validation classification: %v", err)
			}
			if len(caller.calls) != 0 {
				t.Fatal("invalid input made RPC")
			}
			continue
		}
		got := caller.calls[0]
		if !strings.Contains(out, `"outcome": "success"`) || !strings.Contains(out, `"verified": false`) {
			t.Fatalf("new options must retain unified success with unverified fields: %s", out)
		}
		if got.product != "aitable-helper" || got.args["formStartTime"] != int64(1790038800000) || got.args["formEndTime"] != int64(0) || got.args["submitRecordViewMode"] != 1 || got.args["formCoverResourceId"] != "rid-1" {
			t.Fatalf("wrong request: %#v", got)
		}
	}
}

func TestCrossPlatformCoverageFormCoverOmissionAndDryRun(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		for _, flags := range [][]string{
			{"--form-cover", ""}, {"--form-cover", "\u00a0"},
			{"--form-cover", "https://example.test/cover.png "},
			{"--form-cover-resource-id", ""}, {"--form-cover-resource-id", "rid "},
			{"--form-name", "new title"},
		} {
			caller := &upsertByKeyCaller{dryRun: dryRun, steps: []upsertByKeyStep{{text: `{"success":true,"data":{"baseId":"base","tableId":"t","viewId":"v","enabled":true,"status":1,"cpSynced":true,"formName":"new title"}}`}}}
			args := append([]string{"--base-id", "base", "--table-id", "t", "--view-id", "v", "--yes"}, flags...)
			if dryRun {
				args = append(args, "--dry-run")
			}
			out, err := runAITableCompositeCLI(t, caller, "+form-share-update", args...)
			invalid := flags[0] != "--form-name"
			if (err != nil) != invalid {
				t.Fatalf("dryRun=%v flags=%q: %v", dryRun, flags, err)
			}
			if invalid || dryRun {
				if len(caller.calls) != 0 {
					t.Fatal("invalid input or dry-run made RPC")
				}
			} else if len(caller.calls) != 1 || caller.calls[0].args["formName"] != "new title" {
				t.Fatalf("wrong partial update: %#v", caller.calls)
			} else if _, present := caller.calls[0].args["formCover"]; present {
				t.Fatal("omitted cover was sent in partial update")
			} else if !strings.Contains(out, `"outcome": "success"`) || !strings.Contains(out, `"verified": true`) {
				t.Fatalf("name-only update must be verified: %s", out)
			}
		}
	}
}

type formShareUploadRoundTripper func(*http.Request) (*http.Response, error)

func (f formShareUploadRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestCrossPlatformCoverageFormShareUpdateLocalCoverThroughShortcut(t *testing.T) {
	for _, synced := range []bool{true, false} {
		t.Run(fmt.Sprintf("cpSynced=%t", synced), func(t *testing.T) {
			var imageBytes bytes.Buffer
			if err := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
				t.Fatal(err)
			}
			cover := filepath.Join(t.TempDir(), "cover-with-wrong-extension.jpg")
			if err := os.WriteFile(cover, imageBytes.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}

			var uploaded bool
			transport := http.RoundTripper(formShareUploadRoundTripper(func(r *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("read upload body: %v", err)
				}
				if r.Method != http.MethodPut || r.URL.String() != "https://upload.example.test/form-cover" ||
					r.Header.Get("Content-Type") != "image/png" || !bytes.Equal(body, imageBytes.Bytes()) {
					t.Fatalf("unexpected upload request: method=%s url=%s content-type=%s body=%d bytes", r.Method, r.URL, r.Header.Get("Content-Type"), len(body))
				}
				uploaded = true
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
			}))
			testseam.Swap(t, &http.DefaultTransport, transport)

			caller := &upsertByKeyCaller{callFn: func(index int, product, tool string, args map[string]any) (string, error) {
				switch index {
				case 0:
					if product != "doc" || tool != "get_doc_attachment_upload_info" || args["nodeId"] != "base" || args["fileName"] != "cover-with-wrong-extension.png" || args["mimeType"] != "image/png" {
						t.Fatalf("wrong upload credential request: %s/%s %#v", product, tool, args)
					}
					return `{"uploadUrl":"https://upload.example.test/form-cover","resourceId":"rid-cover","resourceUrl":"/resource/rid-cover"}`, nil
				case 1:
					if product != "aitable-helper" || tool != "update_share_form" || args["formCoverResourceId"] != "rid-cover" {
						t.Fatalf("wrong form update request: %s/%s %#v", product, tool, args)
					}
					return fmt.Sprintf(`{"success":true,"data":{"baseId":"base","tableId":"table","viewId":"view","enabled":true,"status":1,"cpSynced":%t,"shareFormUuid":"share-1","formCover":"https://example.test/cover.png"}}`, synced), nil
				default:
					t.Fatalf("unexpected call %d: %s/%s %#v", index, product, tool, args)
					return "", nil
				}
			}}
			out, err := runAITableCompositeCLI(t, caller, "+form-share-update", "--base-id", "base", "--table-id", "table", "--view-id", "view", "--form-cover-file", cover, "--yes")
			if err != nil {
				t.Fatal(err)
			}
			if synced && (!strings.Contains(out, `"outcome": "success"`) || !strings.Contains(out, `"verified": false`) || !strings.Contains(out, `"formCoverResourceId"`)) {
				t.Fatalf("local cover must retain the unified result contract: %s", out)
			}
			if !synced {
				var env map[string]any
				if err := json.Unmarshal([]byte(out), &env); err != nil {
					t.Fatal(err)
				}
				partial := env["data"].(map[string]any)
				info := partial["failed"].([]any)[0].(map[string]any)["error"].(map[string]any)
				if env["ok"] != false || env["outcome"] != "partial_failure" || info["details"].(map[string]any)["formCoverResourceId"] != "rid-cover" {
					t.Fatalf("shortcut lost partial failure recovery ID: %s", out)
				}
			}
			if !uploaded || len(caller.calls) != 2 {
				t.Fatalf("cover upload did not complete: uploaded=%v calls=%#v", uploaded, caller.calls)
			}
		})
	}
}

// Both real execution and dry-run reject invalid files before obtaining upload credentials.
func TestCrossPlatformCoverageFormShareInvalidFileExitCode(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "empty.png")
	if err := os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, dryRun := range []bool{false, true} {
		for _, file := range []string{"", filepath.Join(t.TempDir(), "missing.png"), t.TempDir(), empty} {
			caller := &upsertByKeyCaller{dryRun: dryRun}
			args := []string{"--base-id=b", "--table-id=t", "--view-id=v", "--form-cover-file=" + file, "--yes"}
			if dryRun {
				args = append(args, "--dry-run")
			}
			_, err := runAITableCompositeCLI(t, caller, "+form-share-update", args...)
			if apperrors.ExitCode(err) != apperrors.ExitCodeValidation || len(caller.calls) != 0 {
				t.Fatalf("file=%q dryRun=%v error=%v calls=%d", file, dryRun, err, len(caller.calls))
			}
		}
	}
}

func TestCrossPlatformCoverageFormShareUpdateLocalCoverUploadFailureSkipsUpdate(t *testing.T) {
	cover := filepath.Join(t.TempDir(), "cover.png")
	if err := os.WriteFile(cover, []byte("not a real image"), 0o600); err != nil {
		t.Fatal(err)
	}
	caller := &upsertByKeyCaller{}
	_, err := runAITableCompositeCLI(t, caller, "+form-share-update", "--base-id", "base", "--table-id", "table", "--view-id", "view", "--form-cover-file", cover, "--yes")
	if err == nil || len(caller.calls) != 0 || !strings.Contains(err.Error(), "有效的 PNG/JPEG/GIF") {
		t.Fatalf("invalid cover = err:%v calls:%#v", err, caller.calls)
	}
}
