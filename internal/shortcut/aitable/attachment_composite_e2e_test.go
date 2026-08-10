// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func writeAttachmentFixture(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func attachmentRecordJSON(t *testing.T, fieldID string, attachments []any) string {
	t.Helper()
	return mustJSONText(t, map[string]any{"records": []any{map[string]any{
		"recordId": "record", "cells": map[string]any{fieldID: attachments},
	}}})
}

func TestCrossPlatformCoverageAttachmentPutPerformsHTTPAndReadBackE2E(t *testing.T) {
	filePath := writeAttachmentFixture(t, "actual attachment bytes")
	fileName := filepath.Base(filePath)
	uploaded := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		if request.Method != http.MethodPut || request.Header.Get("Content-Type") != "text/plain" || string(body) != "actual attachment bytes" {
			t.Errorf("upload request = method:%s type:%s body:%q", request.Method, request.Header.Get("Content-Type"), body)
			http.Error(w, "bad upload", http.StatusBadRequest)
			return
		}
		uploaded = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: attachmentRecordJSON(t, "field", []any{})},
		{text: mustJSONText(t, map[string]any{"uploadUrl": server.URL + "/put", "fileToken": "ft1"})},
		{text: ""},
		{text: attachmentRecordJSON(t, "field", []any{map[string]any{"filename": fileName, "size": len("actual attachment bytes")}})},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+attachment-put",
		"--base-id", "base", "--table-id", "table", "--record-id", "record", "--field-id", "field", "--file", filePath, "--mode", "replace", "--mime-type", "text/plain", "--yes")
	if err != nil || !uploaded {
		t.Fatalf("attachment put = output:%q err:%v uploaded:%v", out, err, uploaded)
	}
	for _, want := range []string{`"status": "recovered"`, `"fileToken": "ft1"`, `"attachmentCount": 1`, `"status": "verified"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("attachment output missing %s: %s", want, out)
		}
	}
	if len(caller.calls) != 4 || caller.calls[1].tool != "prepare_attachment_upload" || caller.calls[2].tool != "update_records" {
		t.Fatalf("attachment calls = %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageAttachmentPutFailuresDoNotBecomeSuccessE2E(t *testing.T) {
	filePath := writeAttachmentFixture(t, "bytes")
	t.Run("append stops before upload when existing tokens unavailable", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: attachmentRecordJSON(t, "field", []any{map[string]any{"filename": "old.pdf", "url": "https://download"}})},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+attachment-put",
			"--base-id", "base", "--table-id", "table", "--record-id", "record", "--field-id", "field", "--file", filePath, "--yes")
		if err == nil || out != "" || len(caller.calls) != 1 || !strings.Contains(err.Error(), "fileToken") {
			t.Fatalf("unsafe append = output:%q err:%v calls:%#v", out, err, caller.calls)
		}
	})

	t.Run("HTTP failure leaves no record write", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "no", http.StatusBadGateway) }))
		defer server.Close()
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: attachmentRecordJSON(t, "field", []any{})},
			{text: mustJSONText(t, map[string]any{"uploadUrl": server.URL, "fileToken": "ft1"})},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+attachment-put",
			"--base-id", "base", "--table-id", "table", "--record-id", "record", "--field-id", "field", "--file", filePath, "--mode", "replace", "--yes")
		if err == nil || out != "" || len(caller.calls) != 2 || !strings.Contains(err.Error(), "partial_success") {
			t.Fatalf("HTTP failure = output:%q err:%v calls:%#v", out, err, caller.calls)
		}
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Retryable {
			t.Fatalf("partially uploaded attachment must not be blindly retryable: %#v", err)
		}
	})

	t.Run("prepare response must carry both values", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: attachmentRecordJSON(t, "field", []any{})}, {text: `{"uploadUrl":"https://example.invalid"}`},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+attachment-put",
			"--base-id", "base", "--table-id", "table", "--record-id", "record", "--field-id", "field", "--file", filePath, "--mode", "replace", "--yes")
		if err == nil || out != "" || len(caller.calls) != 2 {
			t.Fatalf("bad prepare = output:%q err:%v calls:%#v", out, err, caller.calls)
		}
	})
}

func TestCrossPlatformCoverageAttachmentPutRejectsHTTPSRedirectToPlaintextNonLoopbackE2E(t *testing.T) {
	filePath := writeAttachmentFixture(t, "redirect-sensitive bytes")
	sourceHits := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sourceHits++
		w.Header().Set("Location", "http://example.com/upload")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()
	client := server.Client()
	client.CheckRedirect = rejectAttachmentUploadRedirect
	testseam.Swap(t, &attachmentHTTPDo, client.Do)

	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: attachmentRecordJSON(t, "field", []any{})},
		{text: mustJSONText(t, map[string]any{"uploadUrl": server.URL + "/put", "fileToken": "ft-redirect"})},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+attachment-put",
		"--base-id", "base", "--table-id", "table", "--record-id", "record", "--field-id", "field", "--file", filePath, "--mode", "replace", "--yes")
	var typed *apperrors.Error
	if err == nil || out != "" || !errors.As(err, &typed) || typed.Reason != "aitable_composite_partial_success" ||
		typed.Cause == nil || !strings.Contains(typed.Cause.Error(), "status 302") {
		t.Fatalf("redirect upload = output:%q err:%v", out, err)
	}
	if sourceHits != 1 || len(caller.calls) != 2 {
		t.Fatalf("redirect upload reached an unexpected target: sourceHits=%d calls=%#v", sourceHits, caller.calls)
	}
}

func TestCrossPlatformCoverageAttachmentRemoveClearAndSelectiveE2E(t *testing.T) {
	t.Run("clear empty write response recovered by empty read-back", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: attachmentRecordJSON(t, "field", []any{map[string]any{"filename": "a.pdf", "url": "https://download"}})},
			{text: ""},
			{text: attachmentRecordJSON(t, "field", []any{})},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+attachment-remove",
			"--base-id", "base", "--table-id", "table", "--record-id", "record", "--field-id", "field", "--clear-all", "--yes")
		if err != nil || !strings.Contains(out, `"status": "recovered"`) || !strings.Contains(out, `"removedCount": 1`) {
			t.Fatalf("clear attachments = output:%q err:%v", out, err)
		}
	})

	t.Run("selective removal preserves tokenized remainder", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: attachmentRecordJSON(t, "field", []any{
				map[string]any{"fileToken": "remove-token", "filename": "remove.pdf"},
				map[string]any{"fileToken": "keep-token", "filename": "keep.pdf"},
			})},
			{text: `{"updatedCount":1}`},
			{text: attachmentRecordJSON(t, "field", []any{map[string]any{"filename": "keep.pdf", "size": 10}})},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+attachment-remove",
			"--base-id", "base", "--table-id", "table", "--record-id", "record", "--field-id", "field", "--remove-name", "remove.pdf", "--yes")
		if err != nil || !strings.Contains(out, `"removedCount": 1`) {
			t.Fatalf("selective remove = output:%q err:%v", out, err)
		}
		written := caller.calls[1].args["records"].([]any)[0].(map[string]any)["cells"].(map[string]any)["field"].([]any)
		if len(written) != 1 || written[0].(map[string]any)["fileToken"] != "keep-token" {
			t.Fatalf("selective desired attachments = %#v", written)
		}
	})
}

func TestCrossPlatformCoverageAttachmentRemoveExplainsTokenBoundaryE2E(t *testing.T) {
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: attachmentRecordJSON(t, "field", []any{
		map[string]any{"filename": "remove.pdf", "url": "https://one"},
		map[string]any{"filename": "keep.pdf", "url": "https://two"},
	})}}}
	out, err := runAITableCompositeCLI(t, caller, "+attachment-remove",
		"--base-id", "base", "--table-id", "table", "--record-id", "record", "--field-id", "field", "--remove-name", "remove.pdf", "--yes")
	if err == nil || out != "" || len(caller.calls) != 1 || !strings.Contains(err.Error(), "fileToken") || !strings.Contains(err.Error(), "增量删除") {
		t.Fatalf("token boundary = output:%q err:%v calls:%#v", out, err, caller.calls)
	}
}

func TestCrossPlatformCoverageAttachmentPutRemainingOutcomesE2E(t *testing.T) {
	filePath := writeAttachmentFixture(t, "bytes")

	t.Run("file open failure", func(t *testing.T) {
		out, err := runAITableCompositeCLI(t, &upsertByKeyCaller{}, "+attachment-put",
			"--base-id", "base", "--table-id", "table", "--record-id", "record", "--field-id", "field", "--file", filepath.Join(t.TempDir(), "missing"), "--yes")
		if err == nil || out != "" {
			t.Fatalf("file open = output:%q err:%v", out, err)
		}
	})

	t.Run("initial read failure", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{err: errors.New("read failed")}}}
		out, err := runAITableCompositeCLI(t, caller, "+attachment-put",
			"--base-id", "base", "--table-id", "table", "--record-id", "record", "--field-id", "field", "--file", filePath, "--yes")
		if err == nil || out != "" || !strings.Contains(err.Error(), "read failed") {
			t.Fatalf("initial read = output:%q err:%v", out, err)
		}
	})

	t.Run("append dry run preserves token", func(t *testing.T) {
		caller := &upsertByKeyCaller{dryRun: true, steps: []upsertByKeyStep{{text: attachmentRecordJSON(t, "field", []any{
			map[string]any{"file_token": "old-token", "filename": "old.txt"},
		})}}}
		out, err := runAITableCompositeCLI(t, caller, "+attachment-put",
			"--base-id", "base", "--table-id", "table", "--record-id", "record", "--field-id", "field", "--file", filePath, "--dry-run", "--yes")
		if err != nil || !strings.Contains(out, `"status": "planned"`) || len(caller.calls) != 1 {
			t.Fatalf("append dry run = output:%q err:%v calls:%#v", out, err, caller.calls)
		}
	})

	t.Run("prepare call failure", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: attachmentRecordJSON(t, "field", []any{})},
			{err: errors.New("prepare failed")},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+attachment-put",
			"--base-id", "base", "--table-id", "table", "--record-id", "record", "--field-id", "field", "--file", filePath, "--mode", "replace", "--yes")
		if err == nil || out != "" {
			t.Fatalf("prepare failure = output:%q err:%v", out, err)
		}
	})

	t.Run("invalid upload URL", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: attachmentRecordJSON(t, "field", []any{})},
			{text: `{"uploadUrl":"http://example.com/upload","fileToken":"ft1"}`},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+attachment-put",
			"--base-id", "base", "--table-id", "table", "--record-id", "record", "--field-id", "field", "--file", filePath, "--mode", "replace", "--yes")
		if err == nil || out != "" {
			t.Fatalf("invalid upload URL = output:%q err:%v", out, err)
		}
	})

	t.Run("HTTP transport failure", func(t *testing.T) {
		testseam.Swap(t, &attachmentHTTPDo, func(*http.Request) (*http.Response, error) {
			return nil, errors.New("transport failed")
		})
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: attachmentRecordJSON(t, "field", []any{})},
			{text: `{"uploadUrl":"https://uploads.example.test/put","fileToken":"ft1"}`},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+attachment-put",
			"--base-id", "base", "--table-id", "table", "--record-id", "record", "--field-id", "field", "--file", filePath, "--mode", "replace", "--yes")
		if err == nil || out != "" {
			t.Fatalf("transport failure = output:%q err:%v", out, err)
		}
	})

	t.Run("write and read-back both fail", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
		defer server.Close()
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: attachmentRecordJSON(t, "field", []any{})},
			{text: mustJSONText(t, map[string]any{"uploadUrl": server.URL, "fileToken": "ft1"})},
			{err: errors.New("write reply failed")},
			{err: errors.New("verification read failed")},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+attachment-put",
			"--base-id", "base", "--table-id", "table", "--record-id", "record", "--field-id", "field", "--file", filePath, "--mode", "replace", "--yes")
		if err == nil || out != "" {
			t.Fatalf("verification failure = output:%q err:%v", out, err)
		}
	})
}

func TestCrossPlatformCoverageAttachmentRemoveRemainingOutcomesE2E(t *testing.T) {
	run := func(t *testing.T, caller *upsertByKeyCaller, extra ...string) (string, error) {
		args := []string{"--base-id", "base", "--table-id", "table", "--record-id", "record", "--field-id", "field"}
		args = append(args, extra...)
		args = append(args, "--yes")
		return runAITableCompositeCLI(t, caller, "+attachment-remove", args...)
	}

	t.Run("initial read failure", func(t *testing.T) {
		out, err := run(t, &upsertByKeyCaller{steps: []upsertByKeyStep{{err: errors.New("read failed")}}}, "--clear-all")
		if err == nil || out != "" || !strings.Contains(err.Error(), "read failed") {
			t.Fatalf("initial read = output:%q err:%v", out, err)
		}
	})

	t.Run("unchanged selection", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: attachmentRecordJSON(t, "field", []any{
			map[string]any{"fileToken": "keep", "filename": "keep.pdf"},
		})}}}
		out, err := run(t, caller, "--remove-name", "missing.pdf")
		if err != nil || !strings.Contains(out, `"status": "unchanged"`) || len(caller.calls) != 1 {
			t.Fatalf("unchanged = output:%q err:%v", out, err)
		}
	})

	t.Run("dry run", func(t *testing.T) {
		caller := &upsertByKeyCaller{dryRun: true, steps: []upsertByKeyStep{{text: attachmentRecordJSON(t, "field", []any{
			map[string]any{"fileToken": "remove", "filename": "remove.pdf"},
		})}}}
		out, err := run(t, caller, "--remove-name", "remove.pdf", "--dry-run")
		if err != nil || !strings.Contains(out, `"status": "planned"`) || len(caller.calls) != 1 {
			t.Fatalf("dry run = output:%q err:%v calls:%#v", out, err, caller.calls)
		}
	})

	t.Run("read-back count mismatch", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: attachmentRecordJSON(t, "field", []any{map[string]any{"fileToken": "remove", "filename": "remove.pdf"}})},
			{text: `{"updatedCount":1}`},
			{text: attachmentRecordJSON(t, "field", []any{map[string]any{"filename": "other.pdf"}})},
		}}
		out, err := run(t, caller, "--clear-all")
		if err == nil || out != "" {
			t.Fatalf("count mismatch = output:%q err:%v", out, err)
		}
	})

	t.Run("removed name remains", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: attachmentRecordJSON(t, "field", []any{
				map[string]any{"fileToken": "remove", "filename": "remove.pdf"},
				map[string]any{"fileToken": "keep", "filename": "keep.pdf"},
			})},
			{text: `{"updatedCount":1}`},
			{text: attachmentRecordJSON(t, "field", []any{map[string]any{"filename": "remove.pdf"}})},
		}}
		out, err := run(t, caller, "--remove-name", "remove.pdf")
		if err == nil || out != "" {
			t.Fatalf("removed item remains = output:%q err:%v", out, err)
		}
	})

	t.Run("write and verification fail", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: attachmentRecordJSON(t, "field", []any{map[string]any{"fileToken": "remove", "filename": "remove.pdf"}})},
			{err: errors.New("write reply failed")},
			{err: errors.New("verification failed")},
		}}
		out, err := run(t, caller, "--clear-all")
		if err == nil || out != "" {
			t.Fatalf("verification failure = output:%q err:%v", out, err)
		}
	})
}

func TestCrossPlatformCoverageAttachmentParsingAndValidation(t *testing.T) {
	t.Run("file validation and MIME sniffing", func(t *testing.T) {
		if file, _, _, err := openAttachmentFile(filepath.Join(t.TempDir(), "missing"), ""); err == nil || file != nil {
			t.Fatalf("missing file must be rejected: file:%v err:%v", file, err)
		}
		if file, _, _, err := openAttachmentFile(t.TempDir(), ""); err == nil || file != nil {
			t.Fatalf("directory must be rejected: file:%v err:%v", file, err)
		}
		empty := filepath.Join(t.TempDir(), "empty")
		if err := os.WriteFile(empty, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if file, _, _, err := openAttachmentFile(empty, ""); err == nil || file != nil {
			t.Fatalf("empty file must be rejected: file:%v err:%v", file, err)
		}
		unknown := filepath.Join(t.TempDir(), "payload.unknown-extension")
		if err := os.WriteFile(unknown, []byte("plain text"), 0o600); err != nil {
			t.Fatal(err)
		}
		file, _, mimeType, err := openAttachmentFile(unknown, "")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if mimeType == "" {
			t.Fatal("sniffed MIME type is empty")
		}
	})

	for name, raw := range map[string]string{
		"missing host": "https:///upload",
		"credentials":  "https://user:pass@example.com/upload",
		"wrong scheme": "ftp://example.com/upload",
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateAttachmentUploadURL(raw); err == nil {
				t.Fatalf("%q must be rejected", raw)
			}
		})
	}
	for _, raw := range []string{"http://localhost/upload", "http://127.0.0.1/upload", "https://example.com/upload"} {
		if err := validateAttachmentUploadURL(raw); err != nil {
			t.Fatalf("%q = %v", raw, err)
		}
	}

	if err := verifyAttachmentPut(nil, 1, "token", "name", 1); err == nil {
		t.Fatal("count mismatch must fail")
	}
	if err := verifyAttachmentPut([]map[string]any{{"fileToken": "token"}}, 1, "token", "name", 1); err != nil {
		t.Fatal(err)
	}
	if err := verifyAttachmentPut([]map[string]any{{"filename": "other", "size": 1}}, 1, "token", "name", 1); err == nil {
		t.Fatal("identity mismatch must fail")
	}
	for value, want := range map[any]bool{int(1): true, int64(1): true, "1": false} {
		_, ok := numericInt64(value)
		if ok != want {
			t.Fatalf("numericInt64(%T) = %v, want %v", value, ok, want)
		}
	}
}

func TestCrossPlatformCoverageAttachmentRemoveSelectionValidationE2E(t *testing.T) {
	for name, extra := range map[string][]string{
		"neither": nil,
		"both":    {"--remove-name", "a.pdf", "--clear-all"},
	} {
		t.Run(name, func(t *testing.T) {
			args := []string{"--base-id", "base", "--table-id", "table", "--record-id", "record", "--field-id", "field"}
			args = append(args, extra...)
			args = append(args, "--yes")
			out, err := runAITableCompositeCLI(t, &upsertByKeyCaller{}, "+attachment-remove", args...)
			if err == nil || out != "" {
				t.Fatalf("selection validation = output:%q err:%v", out, err)
			}
		})
	}
}

func TestCrossPlatformCoverageAttachmentReadBackShapesE2E(t *testing.T) {
	cases := []struct {
		name string
		step upsertByKeyStep
		want string
	}{
		{name: "query error", step: upsertByKeyStep{err: errors.New("query failed")}, want: "query failed"},
		{name: "wrong record", step: upsertByKeyStep{text: `{"records":[{"recordId":"other","cells":{}}]}`}, want: "exact records"},
		{name: "missing cells", step: upsertByKeyStep{text: `{"records":[{"recordId":"record"}]}`}, want: "missing cells"},
		{name: "non-array field", step: upsertByKeyStep{text: `{"records":[{"recordId":"record","cells":{"field":"bad"}}]}`}, want: "not an array"},
		{name: "non-object item", step: upsertByKeyStep{text: `{"records":[{"recordId":"record","cells":{"field":["bad"]}}]}`}, want: "not an object"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{tc.step}}
			out, err := runAITableCompositeCLI(t, caller, "+attachment-remove",
				"--base-id", "base", "--table-id", "table", "--record-id", "record", "--field-id", "field", "--clear-all", "--yes")
			if err == nil || out != "" || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("shape = output:%q err:%v", out, err)
			}
		})
	}
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"records":[{"recordId":"record","cells":{}}]}`}}}
	out, err := runAITableCompositeCLI(t, caller, "+attachment-remove",
		"--base-id", "base", "--table-id", "table", "--record-id", "record", "--field-id", "field", "--clear-all", "--yes")
	if err != nil || !strings.Contains(out, `"status": "unchanged"`) {
		t.Fatalf("missing field = output:%q err:%v", out, err)
	}
}
