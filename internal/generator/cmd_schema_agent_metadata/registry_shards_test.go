// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMergeRegistryShardsFailureModes drives every error path of the split
// registry merge used by validateCommandRegistryFile.
func TestMergeRegistryShardsFailureModes(t *testing.T) {
	t.Run("missing envelope", func(t *testing.T) {
		if _, err := mergeRegistryShards(t.TempDir()); err == nil || !strings.Contains(err.Error(), "read registry envelope") {
			t.Fatalf("err = %v, want envelope read failure", err)
		}
	})
	t.Run("bad envelope json", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "registry.json"), []byte("{bad"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := mergeRegistryShards(dir); err == nil || !strings.Contains(err.Error(), "decode registry envelope") {
			t.Fatalf("err = %v, want envelope decode failure", err)
		}
	})
	t.Run("missing products dir", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "registry.json"), []byte(`{"version":1}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := mergeRegistryShards(dir); err == nil || !strings.Contains(err.Error(), "read registry products dir") {
			t.Fatalf("err = %v, want products dir failure", err)
		}
	})
	t.Run("unreadable shard", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "registry.json"), []byte(`{"version":1}`), 0o600); err != nil {
			t.Fatal(err)
		}
		products := filepath.Join(dir, "products")
		if err := os.MkdirAll(products, 0o755); err != nil {
			t.Fatal(err)
		}
		// 断链符号链接：ReadDir 能列出，ReadFile 必失败。
		if err := os.Symlink(filepath.Join(dir, "nonexistent"), filepath.Join(products, "doc.json")); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if _, err := mergeRegistryShards(dir); err == nil || !strings.Contains(err.Error(), "read registry shard") {
			t.Fatalf("err = %v, want shard read failure", err)
		}
	})
	t.Run("invalid raw shard fails products marshal", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "registry.json"), []byte(`{"version":1}`), 0o600); err != nil {
			t.Fatal(err)
		}
		products := filepath.Join(dir, "products")
		if err := os.MkdirAll(products, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(products, "doc.json"), []byte("{bad"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := mergeRegistryShards(dir); err == nil {
			t.Fatal("err = nil, want RawMessage marshal failure for invalid shard JSON")
		}
	})
}

func TestMergeRegistryShardsSkipsNonShardEntries(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "registry.json"), []byte(`{"$schema":"./s.json","version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	products := filepath.Join(dir, "products")
	if err := os.MkdirAll(filepath.Join(products, "nested.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(products, "README.md"), []byte("skip"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(products, "doc.json"), []byte(`{"id":"doc","tools":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := mergeRegistryShards(dir)
	if err != nil {
		t.Fatalf("mergeRegistryShards() error = %v", err)
	}
	merged := string(data)
	if !strings.Contains(merged, `"id":"doc"`) || strings.Contains(merged, "skip") {
		t.Fatalf("merged = %s, want doc shard only", merged)
	}
}

func TestValidateCommandRegistryFileShardDirectoryErrors(t *testing.T) {
	// 目录形态但缺 envelope：mergeRegistryShards 的错误必须透传。
	dir := t.TempDir()
	if err := validateCommandRegistryFile("", dir); err == nil || !strings.Contains(err.Error(), "read registry envelope") {
		t.Fatalf("err = %v, want merge failure passthrough", err)
	}
	// 单文件形态但不可读（无读权限）。
	if os.Getuid() == 0 {
		t.Skip("running as root; permission-based read failure not enforceable")
	}
	unreadable := filepath.Join(t.TempDir(), "registry.json")
	if err := os.WriteFile(unreadable, []byte(`{"version":1}`), 0o000); err != nil {
		t.Fatal(err)
	}
	if err := validateCommandRegistryFile("", unreadable); err == nil {
		t.Fatal("err = nil, want read failure for unreadable registry file")
	}
}
