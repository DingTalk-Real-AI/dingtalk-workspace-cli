// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"reflect"
	"testing"
)

// TestCrossPlatformCoverageSetShareScopeFinalSchemaResultDelivery 断言
// drive.set_share_scope 与 wiki.set_space_share_scope 的最终交付 Schema
// 含 result 且 outcomes 非空（统一 output rollout 激活后的回归守护）。
func TestCrossPlatformCoverageSetShareScopeFinalSchemaResultDelivery(t *testing.T) {
	snapshot := fullSchemaSnapshotForTest(t)

	for _, canonical := range []string{"drive.set_share_scope", "wiki.set_space_share_scope"} {
		tool := snapshot.Tools[canonical]
		if tool == nil {
			t.Fatalf("%s absent from final Schema snapshot", canonical)
		}
		result, ok := tool["result"].(map[string]any)
		if !ok || result == nil {
			t.Fatalf("%s final Schema has no result declaration", canonical)
		}
		outcomes := schemaContractStringSlice(result["outcomes"])
		want := []string{"success", "failure"}
		if !reflect.DeepEqual(outcomes, want) {
			t.Fatalf("%s result outcomes = %#v, want %#v", canonical, outcomes, want)
		}
		dataSchema := schemaContractMap(result["data_schema"])
		if dataSchema == nil {
			t.Fatalf("%s result has no data_schema", canonical)
		}
		properties := schemaContractMap(dataSchema["properties"])
		if len(properties) == 0 {
			t.Fatalf("%s result data_schema has no properties", canonical)
		}
	}
}

// TestCrossPlatformCoverageSetShareScopeFinalSchemaDryRunDelivery 断言
// drive.set_share_scope 与 wiki.set_space_share_scope 的最终交付 Schema
// 均声明 dry_run 能力，且与 LeafContract 的 DryRunSpec 一致：preview_kind=request、
// remote_reads 缺省或 false（两个 ResultCall 的 DryRun() 分支只回显请求参数、不发 RPC）。
// dry-run 显式 opt-in：未声明时 Schema 会省略 dry_run、Agent 示例门禁不验证预览路径。
func TestCrossPlatformCoverageSetShareScopeFinalSchemaDryRunDelivery(t *testing.T) {
	snapshot := fullSchemaSnapshotForTest(t)

	for _, canonical := range []string{"drive.set_share_scope", "wiki.set_space_share_scope"} {
		tool := snapshot.Tools[canonical]
		if tool == nil {
			t.Fatalf("%s absent from final Schema snapshot", canonical)
		}
		dryRun, ok := tool["dry_run"].(map[string]any)
		if !ok || dryRun == nil {
			t.Fatalf("%s final Schema does not deliver a dry_run capability: %#v", canonical, tool["dry_run"])
		}
		if dryRun["preview_kind"] != "request" {
			t.Fatalf("%s dry_run preview_kind = %#v, want %q", canonical, dryRun["preview_kind"], "request")
		}
		// RemoteReads:false 经 omitempty 投影后通常缺省；若存在则必须为 false。
		if remoteReads, exists := dryRun["remote_reads"]; exists && remoteReads != false {
			t.Fatalf("%s dry_run remote_reads = %#v, want false or absent", canonical, remoteReads)
		}
	}
}
