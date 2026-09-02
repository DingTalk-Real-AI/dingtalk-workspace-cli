// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"errors"
	"strings"
	"testing"
)

func TestCompositeFieldAndChartValidationBranches(t *testing.T) {
	if _, err := runAITableCompositeCLI(t, &upsertByKeyCaller{}, "+field-update",
		"--base-id", "b", "--table-id", "t", "--field-id", "f", "--name", strings.Repeat("字", 151), "--yes"); err == nil {
		t.Fatal("oversized field name succeeded")
	}
	for name, args := range map[string][]string{
		"config metadata": {"--base-id", "b", "--dashboard-id", "d", "--chart-id", "c", "--config", `{"schemaVersion":2}`, "--yes"},
		"layout metadata": {"--base-id", "b", "--dashboard-id", "d", "--chart-id", "c", "--config", `{"chartName":"x"}`, "--layout", `{"isAppMode":true}`, "--yes"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := runAITableCompositeCLI(t, &upsertByKeyCaller{}, "+chart-update", args...); err == nil {
				t.Fatal("metadata write succeeded")
			}
		})
	}
	for name, step := range map[string]upsertByKeyStep{
		"dashboard transport": {err: errors.New("read failed")},
		"dashboard protocol":  {text: `{"data":{}}`},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := runAITableCompositeCLI(t, &upsertByKeyCaller{steps: []upsertByKeyStep{step}}, "+chart-update",
				"--base-id", "b", "--dashboard-id", "d", "--chart-id", "c", "--config", `{"chartName":"x"}`,
				"--layout", `{"x":0,"y":0,"w":1,"h":1}`, "--yes")
			if err == nil {
				t.Fatal("dashboard preflight failure succeeded")
			}
		})
	}
}

func TestBootstrapAndViewPresetValidationBranches(t *testing.T) {
	if _, err := validateBootstrapField(map[string]any{"fieldName": strings.Repeat("字", 151), "type": "text"}, "fields[0]"); err == nil {
		t.Fatal("oversized bootstrap field succeeded")
	}
	if _, err := runAITableCompositeCLI(t, &upsertByKeyCaller{}, "+view-preset-apply",
		"--base-id", "b", "--table-id", "t", "--name", "G", "--view-type", "Gantt",
		"--config", `{"visibleFieldIds":[]}`, "--timebar", `{`, "--yes"); err == nil {
		t.Fatal("invalid Gantt timebar JSON succeeded")
	}
}
