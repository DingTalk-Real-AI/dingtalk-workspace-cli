// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package unit_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageReportSenderRouteAndHelper(t *testing.T) {
	root := repoRoot(t)
	paths := []string{
		filepath.Join(root, "skills", "multi", "dingtalk-misc", "references", "report.md"),
		filepath.Join(root, "skills", "mono", "references", "products", "report.md"),
	}
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(body)
		for _, required := range []string{
			`dws aisearch person --query "<姓名>" --dimension name --format json`,
			"dws report +inbox-list",
			"--sender-user-ids <USER_ID>",
			"零命中或多候选",
			"report_received_today.py",
		} {
			if !strings.Contains(text, required) {
				t.Errorf("%s missing reviewed Report route fragment %q", path, required)
			}
		}
	}
}
