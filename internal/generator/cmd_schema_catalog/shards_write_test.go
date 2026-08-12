// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
)

func TestWriteSchemaCatalogShardsClearFailure(t *testing.T) {
	// 输出路径的父级是普通文件：RemoveAll 报非 IsNotExist 错误。
	parent := filepath.Join(t.TempDir(), "occupied")
	if err := os.WriteFile(parent, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := writeSchemaCatalogShards(cli.SchemaCatalogSnapshot{}, filepath.Join(parent, "schema_catalog"))
	if err == nil {
		t.Fatal("err = nil, want clear/create failure under a plain file")
	}
}

func TestWriteSchemaCatalogShardsShardWriteFailure(t *testing.T) {
	// canonical 前缀带路径分隔符时目标目录不存在，shard 写入必须失败。
	snapshot := cli.SchemaCatalogSnapshot{
		Version: 1,
		Tools: map[string]map[string]any{
			"bad/prod.tool": {"title": "x"},
		},
	}
	err := writeSchemaCatalogShards(snapshot, filepath.Join(t.TempDir(), "schema_catalog"))
	if err == nil || !strings.Contains(err.Error(), "write schema catalog tools/") {
		t.Fatalf("err = %v, want shard write failure", err)
	}
}

func TestWriteSchemaCatalogJSONEncodeFailure(t *testing.T) {
	err := writeSchemaCatalogJSON(filepath.Join(t.TempDir(), "catalog.json"), map[string]any{"bad": math.NaN()})
	if err == nil || !strings.Contains(err.Error(), "encode catalog.json") {
		t.Fatalf("err = %v, want encode failure", err)
	}
}

func TestWriteSchemaCatalogShardsEnvelopeWriteFailure(t *testing.T) {
	// envelope 自身编码失败（Catalog 含 NaN）走 write catalog.json 错误分支。
	snapshot := cli.SchemaCatalogSnapshot{
		Version: 1,
		Catalog: map[string]any{"bad": math.NaN()},
	}
	err := writeSchemaCatalogShards(snapshot, filepath.Join(t.TempDir(), "schema_catalog"))
	if err == nil || !strings.Contains(err.Error(), "write schema catalog.json") {
		t.Fatalf("err = %v, want envelope write failure", err)
	}
}
