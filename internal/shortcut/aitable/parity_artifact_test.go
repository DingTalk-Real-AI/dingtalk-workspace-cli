// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package aitable

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageParityAttachmentDownloadDispatch(t *testing.T) {
	t.Chdir(t.TempDir())
	testseam.Swap(t, &downloadAITableAttachment, func(ctx context.Context, url string, opts localio.DownloadOptions) (localio.DownloadResult, error) {
		if url != "https://example.com/file" || opts.ExpectedSize == nil || *opts.ExpectedSize != 3 || opts.Overwrite {
			t.Fatal("incorrect download contract", url, opts)
		}
		return localio.DownloadResult{RelativePath: "file.bin", SizeBytes: 3, SHA256: "hash"}, nil
	})
	c := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: attachmentRecordJSON(t, "f", []any{map[string]any{"resourceId": "id", "filename": "file", "size": 3, "url": "https://example.com/file"}})}}}
	out, err := runAITableCompositeCLI(t, c, "+record-download-attachment", "--base-id", "b", "--table-id", "t", "--record-id", "record", "--field-id", "f", "--resource-id", "id", "--output", "file.bin")
	if err != nil || !strings.Contains(out, `"sha256": "hash"`) {
		t.Fatal(out, err)
	}
	for _, tc := range []struct {
		item map[string]any
		path string
		dry  bool
		fail bool
	}{
		{map[string]any{"resourceId": "id", "size": 3, "url": "https://example.com/file"}, "preview.bin", true, false},
		{map[string]any{"resourceId": "other", "size": 3, "url": "https://example.com/file"}, "file.bin", false, true},
		{map[string]any{"resourceId": "id", "url": "https://example.com/file"}, "file.bin", false, true},
		{map[string]any{"resourceId": "id", "size": 3, "url": "http://example.com/file"}, "file.bin", false, true},
		{map[string]any{}, "../escape.bin", false, true},
	} {
		c := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: attachmentRecordJSON(t, "f", []any{tc.item})}}}
		args := []string{"--base-id", "b", "--table-id", "t", "--record-id", "record", "--field-id", "f", "--resource-id", "id", "--output", tc.path}
		if tc.dry {
			args = append(args, "--dry-run")
		}
		out, err := runAITableCompositeCLI(t, c, "+record-download-attachment", args...)
		if (err != nil) != tc.fail {
			t.Fatal(out, err, tc)
		}
	}
	testseam.Swap(t, &downloadAITableAttachment, func(context.Context, string, localio.DownloadOptions) (localio.DownloadResult, error) {
		return localio.DownloadResult{}, fmt.Errorf("stream failed")
	})
	c = &upsertByKeyCaller{steps: []upsertByKeyStep{{text: attachmentRecordJSON(t, "f", []any{map[string]any{"resourceId": "id", "size": 3, "url": "https://example.com/file"}})}}}
	out, err = runAITableCompositeCLI(t, c, "+record-download-attachment", "--base-id", "b", "--table-id", "t", "--record-id", "record", "--field-id", "f", "--resource-id", "id", "--output", "file.bin")
	if err == nil || out != "" {
		t.Fatal("published failed download", out, err)
	}
}
func TestCrossPlatformCoverageParityRecordExportNonemptyAndNoClobber(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	steps := []upsertByKeyStep{parityStep(map[string]any{"records": []any{map[string]any{"recordId": "r", "cells": map[string]any{"f": "value"}}}, "hasMore": false}), parityStep(map[string]any{"fields": []any{map[string]any{"fieldId": "f", "fieldName": "Title", "type": "text"}, map[string]any{"fieldId": "other", "fieldName": "Other", "type": "text"}}})}
	c := &upsertByKeyCaller{steps: steps}
	out, err := runAITableCompositeCLI(t, c, "+record-query", "--base-id", "b", "--table-id", "t", "--all", "--export-output", "rows.ndjson", "--field-ids", "f")
	if err != nil {
		t.Fatal(out, err)
	}
	var manifest struct {
		RecordCount int              `json:"recordCount"`
		Complete    bool             `json:"complete"`
		Columns     []map[string]any `json:"columns"`
		Hash        string           `json:"sha256"`
	}
	if json.Unmarshal([]byte(out), &manifest) != nil || manifest.RecordCount != 1 || !manifest.Complete || len(manifest.Columns) != 1 || manifest.Columns[0]["fieldId"] != "f" || len(manifest.Hash) != 64 {
		t.Fatal(out)
	}
	b, e := os.ReadFile(filepath.Join(dir, "rows.ndjson"))
	if e != nil || !strings.Contains(string(b), `"recordId":"r"`) {
		t.Fatal(string(b), e)
	}
	c = &upsertByKeyCaller{steps: steps}
	if _, e = runAITableCompositeCLI(t, c, "+record-query", "--base-id", "b", "--table-id", "t", "--all", "--export-output", "rows.ndjson"); e == nil {
		t.Fatal("overwrote file")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "rows.ndjson"))
	if string(after) != string(b) {
		t.Fatal("existing bytes changed")
	}
	for _, fields := range []string{`{}`, `{"fields":[{"fieldId":"f"}]}`, `{"fields":[{"fieldId":"other","fieldName":"Other","type":"text"}]}`} {
		c = &upsertByKeyCaller{steps: []upsertByKeyStep{steps[0], {text: fields}}}
		out, e := runAITableCompositeCLI(t, c, "+record-query", "--base-id", "b", "--table-id", "t", "--all", "--export-output", "bad.ndjson", "--field-ids", "f")
		if e == nil || out != "" {
			t.Fatal("invalid manifest published", out, e)
		}
		if _, e = os.Stat(filepath.Join(dir, "bad.ndjson")); !os.IsNotExist(e) {
			t.Fatal("artifact created", e)
		}
	}
}
func TestCrossPlatformCoverageParityDirectoryFiltering(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		fail bool
	}{
		{`{"items":[{"nodeId":"a","nodeType":"table","parentSectionId":"p"},{"nodeId":"b","nodeType":"dashboard","parentSectionId":""}]}`, false},
		{`{}`, true}, {`{"items":[{}]}`, true}, {`{"items":[{"nodeId":"a"}]}`, true}, {`{"items":[{"nodeId":"a","nodeType":"table"}]}`, true},
	} {
		c := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: tc.raw}}}
		out, err := runAITableCompositeCLI(t, c, "+base-block-list", "--base-id", "b", "--type", "table", "--parent-id", "p")
		if (err != nil) != tc.fail {
			t.Fatal(out, err)
		}
		if !tc.fail && (!strings.Contains(out, `"count": 1`) || strings.Contains(out, `"nodeId": "b"`)) {
			t.Fatal("wrong directory scope", out)
		}
	}
}

func TestCrossPlatformCoverageParityArtifactsFailWithoutWorkingDirectory(t *testing.T) {
	testseam.Swap(t, &aitableWorkingDirectory, func() (string, error) { return "", fmt.Errorf("working directory removed") })
	for _, tc := range []struct {
		command string
		args    []string
	}{
		{"+record-query", []string{"--base-id", "b", "--table-id", "t", "--all", "--export-output", "rows.ndjson"}},
		{"+record-download-attachment", []string{"--base-id", "b", "--table-id", "t", "--record-id", "r", "--field-id", "f", "--resource-id", "a", "--output", "file.bin"}},
	} {
		c := &upsertByKeyCaller{}
		out, err := runAITableCompositeCLI(t, c, tc.command, tc.args...)
		if err == nil || out != "" || len(c.calls) != 0 {
			t.Fatal(out, err, c.calls)
		}
	}
}
func TestCrossPlatformCoverageParityArtifactRaceRetainsConcurrentFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	c := &upsertByKeyCaller{callFn: func(_ int, _, tool string, _ map[string]any) (string, error) {
		if tool == "query_records" {
			return `{"records":[],"hasMore":false}`, nil
		}
		if e := os.WriteFile(filepath.Join(dir, "rows.ndjson"), []byte("concurrent writer"), 0600); e != nil {
			t.Fatal(e)
		}
		return `{"fields":[]}`, nil
	}}
	out, err := runAITableCompositeCLI(t, c, "+record-query", "--base-id", "b", "--table-id", "t", "--all", "--export-output", "rows.ndjson")
	if err == nil || out != "" {
		t.Fatal(out, err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "rows.ndjson"))
	if string(b) != "concurrent writer" {
		t.Fatal(string(b))
	}
}

func TestCrossPlatformCoverageParityExportPrepublicationFailures(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, tc := range []struct {
		name          string
		rows          []map[string]any
		failRead      bool
		failDirectory bool
	}{
		{name: "unencodable-value", rows: []map[string]any{{"recordId": "r", "cells": map[string]any{"f": make(chan int)}}}},
		{name: "size-limit", rows: []map[string]any{{"recordId": "r", "cells": map[string]any{"f": strings.Repeat("x", maxRecordArtifactBytes)}}}},
		{name: "catalog-unavailable", failRead: true},
		{name: "directory-removed", failDirectory: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"fields":[]}`}}}
			if tc.failRead {
				c.steps[0] = upsertByKeyStep{err: fmt.Errorf("catalog offline")}
			}
			helpers.InitDepsForTest(t, c)
			cmd := &cobra.Command{Use: "export"}
			cmd.Flags().String("export-output", "", "")
			_ = cmd.Flags().Set("export-output", "rows.ndjson")
			if tc.failDirectory {
				testseam.Swap(t, &aitableWorkingDirectory, func() (string, error) { return "", fmt.Errorf("directory removed") })
			}
			rt := shortcut.RuntimeContextForTest(cmd, RecordQuery)
			if err := outputRecordQuery(rt, tc.rows, nil); err == nil {
				t.Fatal("published invalid artifact")
			}
			if _, err := os.Stat("rows.ndjson"); !os.IsNotExist(err) {
				t.Fatal("file appeared", err)
			}
		})
	}
	c := &upsertByKeyCaller{steps: []upsertByKeyStep{{err: fmt.Errorf("record offline")}}}
	out, err := runAITableCompositeCLI(t, c, "+record-download-attachment", "--base-id", "b", "--table-id", "t", "--record-id", "r", "--field-id", "f", "--resource-id", "a", "--output", "file.bin")
	if err == nil || out != "" {
		t.Fatal(out, err)
	}
}
