// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package whiteboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageImageReceiptPersistenceFailuresLeaveNoPartialReceipt(t *testing.T) {
	rt := directWhiteboardRuntime(t, InsertImage, &whiteboardCoverageCaller{}, "--node", "doc", "--part-id", "part")
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("existing user data"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Run("invalid-directory", func(t *testing.T) {
		testseam.Swap(t, &imageReceiptDirectory, func() (string, error) { return blocked, nil })
		path, err := persistImageReceipt(rt, &imagePreflight{}, map[string]any{"resourceId": "res-1"})
		if err == nil || path != "" {
			t.Fatalf("receipt failure returned an apparent recovery file: %q, %v", path, err)
		}
		data, err := os.ReadFile(blocked)
		if err != nil || string(data) != "existing user data" {
			t.Fatalf("failed receipt creation damaged existing data: %q, %v", data, err)
		}
	})
	t.Run("unserializable-upload", func(t *testing.T) {
		dir := t.TempDir()
		testseam.Swap(t, &imageReceiptDirectory, func() (string, error) { return dir, nil })
		path, err := persistImageReceipt(rt, &imagePreflight{}, map[string]any{"resourceId": make(chan int)})
		if err == nil || path != "" {
			t.Fatalf("invalid upload produced a receipt: %q, %v", path, err)
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 0 {
			t.Fatalf("failed serialization left a partial receipt: %#v, %v", entries, err)
		}
	})
	t.Run("new-receipt", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "private-receipts")
		testseam.Swap(t, &imageReceiptDirectory, func() (string, error) { return dir, nil })
		path, err := persistImageReceipt(rt, &imagePreflight{fileDigest: "fixture-digest"}, map[string]any{"resourceId": "res-1"})
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var receipt map[string]any
		if err := json.Unmarshal(data, &receipt); err != nil {
			t.Fatal(err)
		}
		if receipt["uploaded"] != true || receipt["committed"] != false || receipt["verified"] != false || receipt["fileDigest"] != "fixture-digest" {
			t.Fatalf("new receipt invented a committed state: %#v", receipt)
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
			t.Fatalf("atomic creation left temporary files: %#v, %v", entries, err)
		}
	})
}
