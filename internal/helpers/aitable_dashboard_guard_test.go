// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"strings"
	"testing"
)

func TestCrossPlatformCoverageAitablePersistentMetadataGuard(t *testing.T) {
	for _, value := range []map[string]any{
		{"schemaVersion": 2},
		{"isAppMode": true},
		{"meta": map[string]any{"schemaVersion": 2}},
		{"meta": map[string]any{"isAppMode": false}},
	} {
		if err := validateAitablePersistentMetadata("config", value); err == nil {
			t.Fatalf("read-only metadata accepted: %#v", value)
		}
	}
	if err := validateAitablePersistentMetadata("config", map[string]any{"name": "运营", "meta": map[string]any{"theme": "light"}}); err != nil {
		t.Fatalf("business config rejected: %v", err)
	}
}

func TestCrossPlatformCoverageAitablePersistentMetadataGuardIsWired(t *testing.T) {
	for _, args := range [][]string{
		{"dashboard", "create", "--base-id=b", `--config={"meta":{"schemaVersion":2}}`},
		{"dashboard", "update", "--base-id=b", "--dashboard-id=d", `--config={"isAppMode":true}`},
		{"chart", "create", "--base-id=b", "--dashboard-id=d", `--config={"name":"n"}`, `--layout={"x":0,"schemaVersion":2}`},
		{"chart", "update", "--base-id=b", "--dashboard-id=d", "--chart-id=c", `--config={"name":"n","meta":{"isAppMode":false}}`},
	} {
		caller := &aitableCommandCoverageCaller{}
		err := runAitableCoverageCommand(t, caller, args...)
		if err == nil || !strings.Contains(err.Error(), "只读运行时信息") {
			t.Fatalf("args %v error = %v", args, err)
		}
	}
}
