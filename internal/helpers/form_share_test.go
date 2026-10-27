// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package helpers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/skip2/go-qrcode"
	"github.com/spf13/cobra"
	"golang.org/x/image/bmp"
)

func TestCrossPlatformCoverageFormShareTime(t *testing.T) {
	if got, err := ParseFormShareTime("0", "form-end-at"); err != nil || got != 0 {
		t.Fatalf("clear time = %d, %v", got, err)
	}
	if _, err := ParseFormShareTime("", "form-end-at"); err == nil {
		t.Fatal("empty time must not clear")
	}
	if got, err := ParseFormShareTime("1788307200000", "form-start-at"); err != nil || got != 1788307200000 {
		t.Fatalf("timestamp = %d, %v", got, err)
	}
	if got, err := ParseFormShareTime("2026-09-22T09:00:00+08:00", "form-start-at"); err != nil || got != 1790038800000 {
		t.Fatalf("RFC3339 = %d, %v", got, err)
	}
	if _, err := ParseFormShareTime("2026-09-22 09:00:00", "form-start-at"); err == nil {
		t.Fatal("timezone-less time unexpectedly accepted")
	}
}

func TestCrossPlatformCoverageFormShareValidation(t *testing.T) {
	for _, value := range []int{-1, 2, 99} {
		if err := ValidateFormShareViewMode(value); err == nil {
			t.Fatalf("accepted invalid view mode %d", value)
		}
	}
	for _, value := range []string{
		"",
		" ",
		"\t\r\n",
		" ",
		" ",
		"\u0085",
		" https://img.alicdn.com/cover.png",
		"https://img.alicdn.com/cover.png ",
		"http://img.alicdn.com/cover.png",
		"HTTPS://img.alicdn.com/cover.png",
		"/core/api/resources/rid",
		"https://img.alicdn.com/cover.png?signature=secret",
		"https://img.alicdn.com/cover.png?",
		"https://img.alicdn.com/cover.png#fragment",
		"https://img.alicdn.com/cover.png#",
		"https://user:pass@img.alicdn.com/cover.png",
		"https://:443/cover.png",
		"https://img.alicdn.com/cover a.png",
		"https://img.alicdn.com/\\core/api/resources/rid",
		"https://img.alicdn.com/core/api/resources",
		"https://img.alicdn.com/core/api/resources/rid",
		"https://img.alicdn.com/%63ore/api/resources/rid",
		"https://img.alicdn.com/x/../core/api/resources/rid",
		"https://img.alicdn.com/%2e%2e/core/api/resources/rid",
		"https://img.alicdn.com//core//api/resources/rid",
		"https://img.alicdn.com/%2fcore%2fapi%2fresources%2frid",
		"https://example.test/cover.png",
		"https://localhost/cover.png",
		"https://127.0.0.1/cover.png",
		"https://[::1]/cover.png",
		"https://127.0.0.1.nip.io/cover.png",
		"https://sub.img.alicdn.com/cover.png",
		"https://img.alicdn.com.attacker.test/cover.png",
		"https://img.alicdn.com./cover.png",
		"https://attacker.oss-cn-hangzhou.aliyuncs.com/cover.png",
		"https://img.alicdn.com:8443/cover.png",
		"https://img.alicdn.com:0/cover.png",
		"https://img.alicdn.com:65536/cover.png",
		"https://img.alicdn.com/cover.png),url(https://127.0.0.1/private",
		"https://img.alicdn.com/cover.png'),url('https://127.0.0.1/private",
		`https://img.alicdn.com/cover".png`,
	} {
		if err := ValidateFormShareURL(value); err == nil {
			t.Fatalf("accepted invalid cover URL %q", value)
		}
	}
	for _, value := range []string{"https://img.alicdn.com/cover.png", "https://img.alicdn.com/cover%20a.png",
		"https://img.alicdn.com/cover%3Fname%23image.png", "https://img.alicdn.com/core/api/resources-preview/cover.png",
		"https://img.alicdn.com/cover%28a%29%27%22.png"} {
		if err := ValidateFormShareURL(value); err != nil {
			t.Fatalf("rejected valid cover URL %q: %v", value, err)
		}
	}
	for _, host := range []string{"img.alicdn.com", "static.dingtalk.com", "down-cdn.dingtalk.com",
		"alidocs.oss-cn-zhangjiakou.aliyuncs.com", "alidocs2.oss-cn-zhangjiakou.aliyuncs.com", "alidocs.oss-cn-hangzhou.aliyuncs.com"} {
		for _, authority := range []string{host, strings.ToUpper(host), host + ":443", host + ":0443"} {
			if err := ValidateFormShareURL("https://" + authority + "/cover.png"); err != nil {
				t.Fatalf("rejected trusted host %q: %v", authority, err)
			}
		}
		if !strings.Contains(FormCoverURLDescription, host) {
			t.Fatalf("trusted host missing from Help/Schema: %s", host)
		}
	}
}

// The shared uploader must reject malformed programmatic input before any upload or update.
func TestCrossPlatformCoverageFormCoverRejectsNonString(t *testing.T) {
	caller := &aitableTestCaller{}
	installAitableDeps(t, caller)
	for _, value := range []any{nil, true, 123, []string{"https://example.test/cover.png"}} {
		updated := false
		err := RunAitableFormShareUpdate(&cobra.Command{}, map[string]any{"formCover": value}, func(map[string]any) error {
			updated = true
			return nil
		})
		if err == nil || !strings.Contains(err.Error(), "--form-cover 必须是非空 HTTPS URL") || updated || len(caller.calls) != 0 {
			t.Fatalf("malformed cover reached a side effect: value=%#v err=%v updated=%v calls=%d", value, err, updated, len(caller.calls))
		}
	}
}

func TestCrossPlatformCoverageFormShareUpdateTimeFlags(t *testing.T) {
	caller := &aitableTestCaller{dryRun: true}
	installAitableDeps(t, caller)
	t.Chdir(t.TempDir())
	if err := runAitableCoverageCommand(t, caller, "form", "share", "url", "--base-id=b", "--table-id=t", "--view-id=v", "--qr-file=preview.png"); err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 0 {
		t.Fatal("dry-run called server")
	}
	if _, err := os.Stat("preview.png"); !os.IsNotExist(err) {
		t.Fatal("dry-run created QR")
	}
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
		caller := &aitableTestCaller{}
		installAitableDeps(t, caller)
		args := append([]string{"form", "share", "update", "--base-id", "base", "--table-id", "t", "--view-id", "v", "--yes"}, tc.flags...)
		err := runAitableCoverageCommand(t, caller, args...)
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
		if got.server != "aitable-helper" || got.args["formStartTime"] != int64(1790038800000) || got.args["formEndTime"] != int64(0) || got.args["submitRecordViewMode"] != 1 || got.args["formCoverResourceId"] != "rid-1" {
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
			caller := &aitableTestCaller{dryRun: dryRun}
			installAitableDeps(t, caller)
			args := append([]string{"form", "share", "update", "--base-id", "base", "--table-id", "t", "--view-id", "v", "--yes"}, flags...)
			if dryRun {
				args = append(args, "--dry-run")
			}
			err := runAitableCoverageCommand(t, caller, args...)
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
			}
		}
	}
}

func TestCrossPlatformCoverageFormShareURL(t *testing.T) {
	buf := installAitableDeps(t, &aitableTestCaller{})
	cmd := &cobra.Command{}
	cmd.SetOut(buf)
	cmd.Flags().String("qr-file", "", "")
	output.SetCommandRollout(cmd, output.RolloutUnifiedActive)
	write := func(raw string) error {
		ctx, _ := output.WithResultStore(context.Background())
		cmd.SetContext(ctx)
		if err := writeFormShareURL(cmd, raw); err != nil {
			return err
		}
		_, _, err := output.EmitStoredResult(cmd)
		return err
	}
	for _, raw := range []string{`no json`, `{"shareFormUuid":3}`, `{"enabled":false,"shareFormUuid":"s"}`, `{"enabled":true,"punished":true,"shareFormUuid":"s"}`, `{"enabled":true}`, `{"error":{"shareFormUuid":"s"}}`} {
		if err := write(raw); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	const raw = `{"result":{"data":{"enabled":true,"shareFormUuid":"share-123"}}}`
	if err := write(raw); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), formShareURLPrefix+"share-123") {
		t.Fatal(buf.String())
	}
	buf.Reset()
	const escapedUUID = "share/a b?x"
	if err := write(`{"enabled":true,"shareFormUuid":"share/a b?x"}`); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), formShareURLPrefix+url.PathEscape(escapedUUID)) {
		t.Fatalf("special UUID was not path-escaped: %s", buf.String())
	}
	t.Chdir(t.TempDir())
	_ = cmd.Flags().Set("qr-file", "form.png")
	if err := write(raw); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("form.png")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := qrcode.Encode(formShareURLPrefix+"share-123", qrcode.Medium, 512)
	if !bytes.Equal(got, want) {
		t.Fatal("QR payload does not match the delivered canonical URL")
	}
	if _, err := png.Decode(bytes.NewReader(got)); err != nil {
		t.Fatal(err)
	}
	if err := write(raw); err == nil {
		t.Fatal("overwrote existing QR")
	}
	if err := write(`{"enabled":true,"shareFormUuid":"` + strings.Repeat("x", 10000) + `"}`); err == nil {
		t.Fatal("oversized QR accepted")
	}
}

func TestCrossPlatformCoverageFormCoverUpload(t *testing.T) {
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "cover.png")
	if err := os.WriteFile(path, imageBytes.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"plain", "missing", "directory", "empty", "invalid", "large", "dry-run", "credential-error", "put-error", "update-error", "success"} {
		t.Run(scenario, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.SetContext(context.Background())
			cmd.Flags().String("form-cover-file", "", "")
			caller := &floatImageTestCaller{dryRun: scenario == "dry-run", call: func(_ context.Context, product, tool string, args map[string]any, _ int) (*edition.ToolResult, error) {
				if product != "doc" || tool != "get_doc_attachment_upload_info" || args["nodeId"] != "base" {
					t.Fatalf("wrong upload: %s %s %#v", product, tool, args)
				}
				if scenario == "credential-error" {
					return nil, errors.New("credentials failed")
				}
				return floatImageCredentialResult("https://upload.example.test/object", true), nil
			}}
			installFloatImageDeps(t, caller, nil)
			var transport http.RoundTripper = floatImageRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				body, _ := io.ReadAll(r.Body)
				if r.Method != "PUT" || r.Header.Get("Content-Type") != "image/png" || !bytes.Equal(body, imageBytes.Bytes()) {
					t.Fatal("wrong uploaded bytes or content type")
				}
				if scenario == "put-error" {
					return nil, errors.New("upload failed")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
			})
			testseam.Swap(t, &http.DefaultTransport, transport)
			file := path
			switch scenario {
			case "directory":
				file = t.TempDir()
			case "empty":
				file = writeFloatImageFixture(t, "cover.png", "")
			case "missing":
				file = filepath.Join(t.TempDir(), "missing.png")
			case "invalid":
				file = writeFloatImageFixture(t, "cover.png", "not an image")
			case "large":
				file = writeFloatImageFixture(t, "cover.png", strings.Repeat("x", (10<<20)+1))
			}
			if scenario != "plain" {
				_ = cmd.Flags().Set("form-cover-file", file)
			}
			updates := 0
			err := RunAitableFormShareUpdate(cmd, map[string]any{"baseId": "base"}, func(args map[string]any) error {
				updates++
				if scenario != "plain" && scenario != "dry-run" && args["formCoverResourceId"] != "rid-1" {
					t.Fatalf("wrong resource: %#v", args)
				}
				if scenario == "update-error" {
					return errors.New("update failed")
				}
				return nil
			})
			wantError := scenario != "plain" && scenario != "dry-run" && scenario != "success"
			if (err != nil) != wantError {
				t.Fatalf("error = %v", err)
			}
			if scenario == "directory" || scenario == "empty" || scenario == "missing" {
				if apperrors.ExitCode(err) != apperrors.ExitCodeValidation {
					t.Fatalf("invalid file must be a validation error: %v", err)
				}
				if !strings.Contains(err.Error(), "--form-cover-file") {
					t.Fatalf("error refers to the wrong flag: %v", err)
				}
			}
			if scenario == "directory" || scenario == "empty" || scenario == "missing" || scenario == "invalid" || scenario == "large" {
				if len(caller.calls) != 0 {
					t.Fatal("invalid file requested upload credentials")
				}
			}
			if scenario == "update-error" && !strings.Contains(err.Error(), "resourceId=rid-1") {
				t.Fatal(err)
			}
			if scenario == "dry-run" && len(caller.calls) != 0 {
				t.Fatal("dry-run uploaded")
			}
			if wantError && scenario != "update-error" && updates != 0 {
				t.Fatal("updated after failed upload")
			}
		})
	}
}

// Exercise the real atomic command after a successful upload; storing partial_failure returns nil.
func TestCrossPlatformCoverageFormCoverPartialResultKeepsResource(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "cover.png")
	if err := os.WriteFile(file, encoded.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	caller := &floatImageTestCaller{call: func(_ context.Context, product, tool string, args map[string]any, index int) (*edition.ToolResult, error) {
		if index == 0 {
			if product != "doc" || tool != "get_doc_attachment_upload_info" {
				t.Fatalf("unexpected upload call: %s/%s", product, tool)
			}
			return floatImageCredentialResult("https://upload.example.test/cover?signature=secret", true), nil
		}
		if product != "aitable-helper" || tool != "update_share_form" || args["formCoverResourceId"] != "rid-1" {
			t.Fatalf("unexpected update: %s/%s %#v", product, tool, args)
		}
		return floatImageTextResult(`{"status":"success","data":{"baseId":"base","tableId":"table","viewId":"view","enabled":true,"status":1,"cpSynced":false}}`), nil
	}}
	var out bytes.Buffer
	installFloatImageDeps(t, caller, &out)
	uploads := 0
	var transport http.RoundTripper = floatImageRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		uploads++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	testseam.Swap(t, &http.DefaultTransport, transport)
	root := newAitableCommand()
	installExampleGlobalFlags(root)
	root.SetOut(&out)
	root.SetArgs([]string{"form", "share", "update", "--base-id=base", "--table-id=table", "--view-id=view", "--form-cover-file=" + file, "--yes"})
	ctx, _ := output.WithResultStore(context.Background())
	leaf, err := corecmd.ExecuteContextCForTest(root, ctx)
	if err != nil {
		t.Fatal(err)
	}
	code, handled, err := output.EmitStoredResult(leaf)
	if err != nil || !handled || code != 7 || uploads != 1 || len(caller.calls) != 2 {
		t.Fatalf("unexpected partial execution: code=%d handled=%v err=%v uploads=%d calls=%d", code, handled, err, uploads, len(caller.calls))
	}
	var env map[string]any
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	partial := env["data"].(map[string]any)
	info := partial["failed"].([]any)[0].(map[string]any)["error"].(map[string]any)
	if env["ok"] != false || env["outcome"] != "partial_failure" || info["details"].(map[string]any)["formCoverResourceId"] != "rid-1" {
		t.Fatalf("lost failure or recovery ID: %s", out.String())
	}
	if strings.Contains(out.String(), "signature") || strings.Contains(out.String(), "secret") || strings.Contains(out.String(), "upload.example.test") {
		t.Fatalf("upload credentials leaked: %s", out.String())
	}
}

// Invalid local inputs must retain exit code 3 through the prepared atomic command.
func TestCrossPlatformCoverageFormCoverInvalidFileExitCode(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		for _, file := range []string{"", filepath.Join(t.TempDir(), "missing.png"), t.TempDir(), writeFloatImageFixture(t, "empty.png", "")} {
			caller := &aitableTestCaller{dryRun: dryRun}
			args := []string{"form", "share", "update", "--base-id=b", "--table-id=t", "--view-id=v", "--form-cover-file=" + file, "--yes"}
			if dryRun {
				args = append(args, "--dry-run")
			}
			err := runAitableCoverageCommand(t, caller, args...)
			if apperrors.ExitCode(err) != apperrors.ExitCodeValidation || len(caller.calls) != 0 {
				t.Fatalf("file=%q dryRun=%v error=%v calls=%d", file, dryRun, err, len(caller.calls))
			}
		}
	}
}

func TestCrossPlatformCoverageFormCoverFormatAllowlist(t *testing.T) {
	picture := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for _, format := range []string{"png", "jpeg", "gif", "bmp", "webp"} {
		t.Run(format, func(t *testing.T) {
			var encoded bytes.Buffer
			var err error
			switch format {
			case "png":
				err = png.Encode(&encoded, picture)
			case "jpeg":
				err = jpeg.Encode(&encoded, picture, nil)
			case "gif":
				err = gif.Encode(&encoded, picture, nil)
			case "bmp":
				err = bmp.Encode(&encoded, picture)
			case "webp":
				// A minimal 1x1 VP8 WebP; the globally registered decoder must recognize it.
				var raw []byte
				raw, err = base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
				encoded.Write(raw)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, got, err := image.DecodeConfig(bytes.NewReader(encoded.Bytes())); err != nil || got != format {
				t.Fatalf("fixture must be recognized before testing the allowlist: format=%q err=%v", got, err)
			}
			// The filename must not allow BMP/WebP through or change the detected MIME type.
			file := filepath.Join(t.TempDir(), "cover.png")
			if err := os.WriteFile(file, encoded.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}
			allowed := format == "png" || format == "jpeg" || format == "gif"
			for _, dryRun := range []bool{false, true} {
				t.Run(map[bool]string{false: "execute", true: "dry-run"}[dryRun], func(t *testing.T) {
					caller := &floatImageTestCaller{dryRun: dryRun, call: func(_ context.Context, product, tool string, args map[string]any, _ int) (*edition.ToolResult, error) {
						if product != "doc" || tool != "get_doc_attachment_upload_info" || args["mimeType"] != "image/"+format || args["fileName"] != "cover."+format {
							t.Fatalf("unexpected upload request: %s/%s %#v", product, tool, args)
						}
						return floatImageCredentialResult("https://upload.example.test/cover", true), nil
					}}
					installFloatImageDeps(t, caller, nil)
					uploads := 0
					var transport http.RoundTripper = floatImageRoundTripFunc(func(r *http.Request) (*http.Response, error) {
						uploads++
						body, err := io.ReadAll(r.Body)
						if err != nil || r.Method != http.MethodPut || r.Header.Get("Content-Type") != "image/"+format || !bytes.Equal(body, encoded.Bytes()) {
							t.Fatalf("unexpected image upload: method=%s err=%v", r.Method, err)
						}
						return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(""))}, nil
					})
					testseam.Swap(t, &http.DefaultTransport, transport)
					cmd := &cobra.Command{}
					cmd.SetContext(context.Background())
					cmd.Flags().String("form-cover-file", "", "")
					if err := cmd.Flags().Set("form-cover-file", file); err != nil {
						t.Fatal(err)
					}
					updates := 0
					err := RunAitableFormShareUpdate(cmd, map[string]any{"baseId": "base"}, func(args map[string]any) error {
						updates++
						return nil
					})
					if !allowed {
						if err == nil || !strings.Contains(err.Error(), "PNG/JPEG/GIF") || len(caller.calls) != 0 || uploads != 0 || updates != 0 {
							t.Fatalf("unsupported format was not rejected before side effects: err=%v calls=%d uploads=%d updates=%d", err, len(caller.calls), uploads, updates)
						}
						return
					}
					wantRequests := 1
					if dryRun {
						wantRequests = 0
					}
					if err != nil || len(caller.calls) != wantRequests || uploads != wantRequests || updates != 1 {
						t.Fatalf("allowed format failed: err=%v calls=%d uploads=%d updates=%d", err, len(caller.calls), uploads, updates)
					}
				})
			}
		})
	}
}

func TestCrossPlatformCoverageFormShareURLUnifiedExecution(t *testing.T) {
	testseam.Swap(t, &helperAfter, func(time.Duration) <-chan time.Time {
		ready := make(chan time.Time, 1)
		ready <- time.Time{}
		return ready
	})
	for _, tc := range []struct {
		name  string
		qr    bool
		retry bool
	}{{"link", false, false}, {"qr", true, false}, {"retry-link", false, true}, {"retry-qr", true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			const response = `{"data":{"enabled":true,"shareFormUuid":"share-123"}}`
			caller := &aitableTestCaller{responses: []string{response, response}}
			if tc.retry {
				caller.errors = []error{apperrors.NewAPI("temporary read failure", apperrors.WithRetryable(true))}
			}
			out := installAitableDeps(t, caller)
			root := newAitableCommand()
			installExampleGlobalFlags(root)
			root.SetOut(out)
			args := []string{"form", "share", "url", "--base-id=b", "--table-id=t", "--view-id=v"}
			if tc.qr {
				args = append(args, "--qr-file=form.png")
			}
			root.SetArgs(args)
			ctx, _ := output.WithResultStore(context.Background())
			leaf, err := corecmd.ExecuteContextCForTest(root, ctx)
			if err != nil {
				t.Fatal(err)
			}
			if output.CommandRollout(leaf) != output.RolloutUnifiedActive || out.Len() != 0 {
				t.Fatalf("result bypassed unified execution: rollout=%s output=%s", output.CommandRollout(leaf), out.String())
			}
			if _, handled, err := output.EmitStoredResult(leaf); err != nil || !handled {
				t.Fatalf("emit stored result: handled=%v err=%v", handled, err)
			}
			var envelope map[string]any
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			data := envelope["data"].(map[string]any)
			if envelope["ok"] != true || envelope["outcome"] != "success" || data["shareFormUuid"] != "share-123" || data["shareUrl"] != formShareURLPrefix+"share-123" || data["copyableUrl"] != data["shareUrl"] {
				t.Fatalf("unexpected unified result: %#v", envelope)
			}
			if _, present := data["qrFile"]; present != tc.qr {
				t.Fatalf("unexpected qrFile presence: %#v", data)
			}
			if tc.qr {
				file, ok := data["qrFile"].(string)
				if !ok || !filepath.IsAbs(file) {
					t.Fatalf("qrFile must be an absolute path: %#v", data)
				}
				if _, err := os.Stat(file); err != nil {
					t.Fatal(err)
				}
			}
			wantReads := 1
			if tc.retry {
				wantReads++
			}
			if len(caller.calls) != wantReads {
				t.Fatalf("unexpected read count: %#v", caller.calls)
			}
			for _, call := range caller.calls {
				if call.server != "aitable-helper" || call.tool != "get_share_form_config" {
					t.Fatalf("retry changed the read route: %#v", call)
				}
			}
		})
	}
}
