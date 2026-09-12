// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package doc

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageDocDownloadConfirmationBeforeResolveAndPublish(t *testing.T) {
	for _, source := range []string{"media", "cover"} {
		decl := DownloadOverwrite
		t.Run(source, func(t *testing.T) {
			t.Chdir(t.TempDir())
			if err := os.WriteFile("existing.bin", []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"--node", "node-1", "--output", "existing.bin", "--source", source}
			if source == "media" {
				args = append(args, "--resource-id", "ca246787-99c8-4b8e-9d8f-3f6a2b1c0d4e")
			}
			caller := &docCoverageCaller{responses: map[string][]map[string]any{
				"download_doc_attachment": {{"downloadUrl": "https://example.com/asset.bin"}},
				"get_document_style":      {{"cover": map[string]any{"url": "https://example.com/asset.bin"}}},
			}}
			downloads := 0
			testseam.Swap(t, &docDownload, func(_ context.Context, url string, opts localio.DownloadOptions) (localio.DownloadResult, error) {
				downloads++
				if !opts.Overwrite || opts.Output != "existing.bin" || url != "https://example.com/asset.bin" {
					t.Fatalf("wrong publication request: url=%q options=%+v", url, opts)
				}
				path := filepath.Join(opts.BaseDir, opts.Output)
				if err := os.WriteFile(path, []byte("replacement"), 0600); err != nil {
					return localio.DownloadResult{}, err
				}
				return localio.DownloadResult{AbsolutePath: path, RelativePath: opts.Output, SizeBytes: 11}, nil
			})
			err := runDocCoverage(t, decl, caller, args...)
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Reason != "confirmation_required" || caller.calls != 0 || downloads != 0 {
				t.Fatalf("unconfirmed download: err=%v RPCs=%d downloads=%d", err, caller.calls, downloads)
			}
			data, err := os.ReadFile("existing.bin")
			if err != nil || string(data) != "original" {
				t.Fatalf("unconfirmed file changed: %q %v", data, err)
			}
			if err := runDocCoverage(t, decl, caller, append(args, "--yes")...); err != nil {
				t.Fatal(err)
			}
			data, err = os.ReadFile("existing.bin")
			if err != nil || string(data) != "replacement" || downloads != 1 || caller.calls != 1 {
				t.Fatalf("confirmed download: file=%q err=%v RPCs=%d downloads=%d", data, err, caller.calls, downloads)
			}
		})
	}
}

func TestCrossPlatformCoverageDocPreviewOutputShorthandRemainsExecutable(t *testing.T) {
	caller := &docCoverageCaller{dryRun: true}
	got := runDocCoverageEnvelope(t, MediaPreview, caller, "--node", "n", "--resource-id", "ca246787-99c8-4b8e-9d8f-3f6a2b1c0d4e", "-o", "preview.bin", "--dry-run")
	encoded, err := json.Marshal(got)
	if err != nil || caller.calls != 0 || !strings.Contains(string(encoded), "preview.bin") {
		t.Fatalf("-o preview did not preserve output without RPC: %#v, %v", got, err)
	}
}

func TestCrossPlatformCoverageDocOverwritePreviewAndInputBoundaries(t *testing.T) {
	for _, args := range [][]string{
		{"--source", "media"},
		{"--source", "media", "--resource-id", "bad"},
		{"--source", "cover", "--resource-id="},
		{"--source", "other"},
		{"--source", "cover", "--output", "../escape"},
	} {
		caller := &docCoverageCaller{}
		err := runDocCoverage(t, DownloadOverwrite, caller, append([]string{"--node", "n", "--output", "out.bin"}, args...)...)
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Reason == "confirmation_required" || caller.calls != 0 {
			t.Fatalf("invalid input must fail validation before confirmation/RPC: %v", err)
		}
	}
	caller := &docCoverageCaller{}
	got := runDocCoverageEnvelope(t, DownloadOverwrite, caller, "--node", "n", "--source", "cover", "-o", "out.bin", "--dry-run")
	data, ok := got["data"].(map[string]any)
	if !ok || data["executed"] != false || data["preview_kind"] != "plan" || data["localPath"] != "out.bin" || caller.calls != 0 {
		t.Fatalf("dry-run must return a local plan only: %#v", got)
	}
	for _, decl := range []shortcut.Shortcut{MediaDownload, MediaPreview, ResourceDownload} {
		caller := &docCoverageCaller{}
		err := runDocCoverage(t, decl, caller, "--node", "n", "--overwrite")
		if err == nil || !strings.Contains(err.Error(), "unknown flag") || caller.calls != 0 {
			t.Fatalf("legacy command must reject overwrite before RPC: %s %v", decl.Command, err)
		}
	}
}
