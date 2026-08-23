// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package unit_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWhiteboardQuickExamplesUseWritableTextNodes(t *testing.T) {
	paths := []string{
		"../../skills/mono/references/products/whiteboard.md",
		"../../skills/multi/dingtalk-misc/references/whiteboard.md",
	}
	for _, path := range paths {
		path := path
		t.Run(filepath.Base(filepath.Dir(path))+"/"+filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			payload := firstJSONFence(t, string(data))
			source, _ := payload["source"].(map[string]any)
			nodes, _ := source["nodes"].([]any)
			if len(nodes) == 0 {
				t.Fatalf("quick example has no source.nodes: %#v", payload)
			}
			node, _ := nodes[0].(map[string]any)
			if node["type"] != "text" {
				t.Fatalf("quick example first node type = %#v, want text", node["type"])
			}
			for _, dimension := range []string{"width", "height"} {
				value, ok := node[dimension].(float64)
				if !ok || value <= 0 {
					t.Errorf("quick example %s = %#v, want positive number", dimension, node[dimension])
				}
			}
			text, ok := node["text"].(map[string]any)
			if !ok {
				t.Fatalf("quick example text = %#v, want OpenNodes text object", node["text"])
			}
			blocks, _ := text["blocks"].([]any)
			if len(blocks) == 0 {
				t.Fatalf("quick example text.blocks = %#v, want non-empty array", text["blocks"])
			}
		})
	}
}

func TestWhiteboardRecipesAreDeliveredToBothSkillSurfaces(t *testing.T) {
	monoPath := "../../skills/mono/references/products/whiteboard/recipes.md"
	multiPath := "../../skills/multi/dingtalk-misc/references/whiteboard/recipes.md"
	mono, err := os.ReadFile(monoPath)
	if err != nil {
		t.Fatal(err)
	}
	multi, err := os.ReadFile(multiPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(mono) != string(multi) {
		t.Fatal("mono and multi whiteboard recipes differ")
	}
	content := string(mono)
	for _, required := range []string{
		"## 1. 追加两个流程节点和一条箭头",
		"## 2. 追加带渐变和阴影的卡片",
		"## 3. Frame 中放置分支流程",
		"## 4. 上传 SVG 并追加 Vector",
		"## 5. 整页替换或清空",
		"执行任何远端写入前，必须先向用户展示影响并取得明确确认",
		`"resourceUrl": "https://resource.example/resource-stable-id?resourceId=resource-stable-id"`,
		`"url": "https://resource.example/resource-stable-id?resourceId=resource-stable-id"`,
	} {
		if !strings.Contains(content, required) {
			t.Errorf("whiteboard recipes missing %q", required)
		}
	}
	for _, fence := range bashFences(content) {
		if strings.Contains(fence, "dws whiteboard update") || strings.Contains(fence, "dws doc media upload") {
			if !strings.Contains(fence, "--yes") {
				t.Errorf("whiteboard write example lacks --yes:\n%s", fence)
			}
		}
	}
}

func bashFences(markdown string) []string {
	const marker = "```bash\n"
	var fences []string
	for {
		start := strings.Index(markdown, marker)
		if start < 0 {
			return fences
		}
		markdown = markdown[start+len(marker):]
		end := strings.Index(markdown, "\n```")
		if end < 0 {
			return fences
		}
		fences = append(fences, markdown[:end])
		markdown = markdown[end+len("\n```"):]
	}
}

func firstJSONFence(t *testing.T, markdown string) map[string]any {
	t.Helper()
	const marker = "```json\n"
	start := strings.Index(markdown, marker)
	if start < 0 {
		t.Fatal("markdown has no JSON fence")
	}
	start += len(marker)
	end := strings.Index(markdown[start:], "\n```")
	if end < 0 {
		t.Fatal("JSON fence is not closed")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(markdown[start:start+end]), &payload); err != nil {
		t.Fatalf("decode first JSON fence: %v", err)
	}
	return payload
}
