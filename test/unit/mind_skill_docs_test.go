// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.

package unit_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	mindcore "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/mind"
)

func TestCrossPlatformCoverageMindSkillExampleUsesCurrentOpenNodesContract(t *testing.T) {
	root := repoRoot(t)
	p := filepath.Join(root, "skills", "multi", "dingtalk-misc", "references", "mind", "open-nodes-v1.md")
	content, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	examples := regexp.MustCompile("(?s)```json\\n(.*?)\\n```").FindAllSubmatch(content, -1)
	if len(examples) != 1 {
		t.Fatalf("want one complete source example, got %d", len(examples))
	}
	source, err := mindcore.ParseSource(examples[0][1])
	if err != nil {
		t.Fatalf("documented source rejected by CLI: %v", err)
	}
	if len(source.Pages) != 1 || len(source.Pages[0].Nodes) != 10 || source.Pages[0].Nodes[9].Text != "" {
		t.Fatal("ten-topic example or empty text drifted")
	}
}

func TestCrossPlatformCoverageMindSkillRoutingSeparatesFileCanvasAndMinutes(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range []string{"skills/multi/dingtalk-misc/SKILL.md", "skills/mono/SKILL.md"} {
		raw, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		content := string(raw)
		for _, term := range []string{"mind.md", "wiki node create --type amind", "内置思维导图", "mind-v1"} {
			if !strings.Contains(content, term) {
				t.Errorf("%s missing routing distinction %q", rel, term)
			}
		}
		if strings.Contains(content, "画布/OpenNodes/白板节点") || strings.Contains(content, "画布 / OpenNodes / 白板节点") {
			t.Errorf("%s still routes generic OpenNodes to whiteboard", rel)
		}
	}
	for _, rel := range []string{"skills/multi/dingtalk-minutes/SKILL.md", "skills/mono/references/products/minutes.md"} {
		raw, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		content := string(raw)
		if !strings.Contains(content, "mind.md") || !strings.Contains(content, "独立") || !strings.Contains(content, "内置") {
			t.Errorf("%s lacks independent-file boundary", rel)
		}
		if strings.Contains(content, `当用户要求"生成思维导图/创建脑图"时，必须`) || strings.Contains(content, "**思维导图必须且只能通过") {
			t.Errorf("%s still captures all mind-map requests", rel)
		}
	}
}
