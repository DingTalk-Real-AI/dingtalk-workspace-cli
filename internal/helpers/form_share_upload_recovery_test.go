// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageFormCoverUploadFailureRecovery(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "cover.png")
	if err := os.WriteFile(file, encoded.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"structured", "legacy"} {
		for _, withDetails := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/details=%t", kind, withDetails), func(t *testing.T) {
				var details map[string]any
				if withDetails {
					details = map[string]any{"traceId": "trace-original"}
				}
				retryAfter := int64(30)
				nextRetry := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
				typed := &apperrors.Error{
					Category: apperrors.CategoryAPI, Message: "configuration timed out",
					Operation: "update_share_form", Reason: "network_timeout", Hint: "original diagnostic",
					Retryable: true, RetryableSet: true, RetryAfterSeconds: &retryAfter, NextRetryAt: &nextRetry,
					Details: details,
				}
				legacy := &CLIError{
					Code: CodeAuthPermission, Message: "configuration denied", Operation: "update_share_form",
					ServerCode: "FORBIDDEN", Suggestion: "original diagnostic", Details: details,
				}
				var original error = typed
				if kind == "legacy" {
					original = legacy
				}
				before, err := json.Marshal(original)
				if err != nil {
					t.Fatal(err)
				}
				failure := fmt.Errorf("share configuration: %w", original)
				caller := &floatImageTestCaller{call: func(_ context.Context, product, tool string, _ map[string]any, _ int) (*edition.ToolResult, error) {
					if product != "doc" || tool != "get_doc_attachment_upload_info" {
						t.Fatalf("unexpected dependency: %s/%s", product, tool)
					}
					return floatImageCredentialResult("https://upload.example.test/object", true), nil
				}}
				installFloatImageDeps(t, caller, nil)
				uploads := 0
				var transport http.RoundTripper = floatImageRoundTripFunc(func(request *http.Request) (*http.Response, error) {
					uploads++
					body, err := io.ReadAll(request.Body)
					if err != nil || request.Method != "PUT" || !bytes.Equal(body, encoded.Bytes()) {
						t.Fatalf("unexpected upload: method=%s body=%x err=%v", request.Method, body, err)
					}
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
				})
				testseam.Swap(t, &http.DefaultTransport, transport)
				cmd := &cobra.Command{}
				cmd.SetContext(context.Background())
				cmd.Flags().String("form-cover-file", "", "")
				if err := cmd.Flags().Set("form-cover-file", file); err != nil {
					t.Fatal(err)
				}
				updates := 0
				err = RunAitableFormShareUpdate(cmd, map[string]any{"baseId": "base"}, func(args map[string]any) error {
					updates++
					if args["formCoverResourceId"] != "rid-1" {
						t.Fatalf("update did not use the uploaded resource: %#v", args)
					}
					return failure
				})
				if !errors.Is(err, failure) || uploads != 1 || updates != 1 || len(caller.calls) != 1 {
					t.Fatalf("failure must preserve its cause without replay: err=%v uploads=%d updates=%d calls=%d", err, uploads, updates, len(caller.calls))
				}
				if apperrors.ExitCode(err) != apperrors.ExitCode(original) || !strings.Contains(err.Error(), "form share get") || !strings.Contains(err.Error(), "勿重复上传或原样重放") {
					t.Fatalf("lost classification or read-only recovery guidance: %v", err)
				}
				var recoveredDetails map[string]any
				if kind == "structured" {
					var recovered *apperrors.Error
					if !errors.As(err, &recovered) || recovered == typed || recovered.Retryable || !recovered.RetryableSet || recovered.RetryAfterSeconds != nil || recovered.NextRetryAt != nil {
						t.Fatalf("uploaded resource failure still advertises automatic retry: %#v", recovered)
					}
					if recovered.Operation != typed.Operation || recovered.Reason != typed.Reason || !strings.Contains(recovered.Hint, typed.Hint) {
						t.Fatalf("lost original diagnostics: %#v", recovered)
					}
					recoveredDetails = recovered.Details
				} else {
					var recovered *CLIError
					if !errors.As(err, &recovered) || recovered == legacy || recovered.ServerCode != legacy.ServerCode || recovered.Operation != legacy.Operation || !strings.Contains(recovered.Suggestion, legacy.Suggestion) {
						t.Fatalf("lost legacy diagnostics: %#v", recovered)
					}
					recoveredDetails = recovered.Details
				}
				if recoveredDetails["formCoverResourceId"] != "rid-1" || (withDetails && recoveredDetails["traceId"] != "trace-original") {
					t.Fatalf("missing resource recovery identity or original trace: %#v", recoveredDetails)
				}
				recoveredDetails["traceId"] = "changed-by-consumer"
				after, marshalErr := json.Marshal(original)
				if marshalErr != nil || !bytes.Equal(before, after) {
					t.Fatalf("recovery mutated the original dependency error: before=%s after=%s err=%v", before, after, marshalErr)
				}
			})
		}
	}
}
