// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package skills

import (
	"bytes"
	"strings"
	"testing"
)

// Keep both distributions aligned with the runtime's fail-closed recovery
// fields, rather than allowing generic resume guidance to hide invalidation.
func TestCrossPlatformCoverageAITableCursorRecoveryReferences(t *testing.T) {
	paths := []string{
		"mono/references/products/aitable/aitable-record-query.md",
		"multi/dingtalk-aitable/references/aitable/aitable-record-query.md",
	}
	var previous []byte
	for _, path := range paths {
		data, err := FS.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if previous != nil && !bytes.Equal(previous, data) {
			t.Fatal("mono/multi record pagination contracts diverge")
		}
		previous = data
		for _, marker := range []string{
			"INVALID_CURSOR", "CURSOR_SNAPSHOT_CHANGED", "CURSOR_SNAPSHOT_UNAVAILABLE",
			"retryable=false", "discard_previous_results=true", "restart_from_first_page=true",
		} {
			if !strings.Contains(string(data), "`"+marker+"`") {
				t.Errorf("%s omits runtime recovery contract %s", path, marker)
			}
		}
	}
	root, err := FS.ReadFile("multi/dingtalk-aitable/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(root), "references/aitable/aitable-record-query.md") {
		t.Fatal("record pagination recovery reference is not discoverable")
	}
}
