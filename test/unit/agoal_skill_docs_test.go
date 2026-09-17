// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package unit_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageAgoalSkillPinsGoldenRoutesAndBusinessSOPs(t *testing.T) {
	root := repoRoot(t)
	references := []string{
		filepath.Join(root, "skills", "multi", "dingtalk-misc", "references", "agoal.md"),
		filepath.Join(root, "skills", "mono", "references", "products", "agoal.md"),
	}
	required := []string{
		"dws agoal +contract-fields",
		"dws agoal +user-rules",
		"dws agoal +report-statistics-list",
		"dws agoal +report-submit-detail",
		"dws agoal +obj-template-list",
		"dws contact user get-self --format json",
		"content.preference.ruleId",
		"content.preference.periodId",
		"enableStatistic=false",
		"Asia/Shanghai",
		"`templateId`",
	}
	for _, path := range references {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(body)
		for _, fragment := range required {
			if !strings.Contains(text, fragment) {
				t.Errorf("%s missing reviewed Agoal route fragment %q", path, fragment)
			}
		}
		for _, forbidden := range []string{
			"--user-id me",
			`--user-id ""`,
		} {
			if strings.Contains(text, forbidden) {
				t.Errorf("%s retains unsafe Agoal identity placeholder %q", path, forbidden)
			}
		}
	}
}

func TestCrossPlatformCoverageAgoalAndReportRoutingBoundary(t *testing.T) {
	root := repoRoot(t)
	paths := []string{
		filepath.Join(root, "skills", "multi", "dingtalk-misc", "SKILL.md"),
		filepath.Join(root, "skills", "mono", "SKILL.md"),
	}
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(body)
		for _, fragment := range []string{
			"经营合约",
			"目标规则",
			"迟交",
			"未提交",
			"跟催",
			"Agoal",
			"Report",
		} {
			if !strings.Contains(text, fragment) {
				t.Errorf("%s missing Agoal dispatch boundary fragment %q", path, fragment)
			}
		}
	}
}
