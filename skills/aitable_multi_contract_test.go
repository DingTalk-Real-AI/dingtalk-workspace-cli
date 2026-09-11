// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package skills

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

func TestAITableDatasourceUpdateDocumentsOptionalConfigPreservation(t *testing.T) {
	data, err := FS.ReadFile("multi/dingtalk-aitable/references/aitable/aitable-datasource.md")
	if err != nil {
		t.Fatal(err)
	}
	_, update, ok := strings.Cut(string(data), "### +datasource-update")
	if !ok {
		t.Fatal("missing datasource update guide")
	}
	update, _, _ = strings.Cut(update, "### +datasource-sync")
	for _, required := range []string{
		"| `--source-config` | 否 |",
		"省略时先读取当前 sourceConfig 并原样提交",
		"读取失败、配置缺失或格式异常时不执行更新",
		"显式提供配置时不进行这次读取",
		"dws aitable datasource update",
	} {
		if !strings.Contains(update, required) {
			t.Errorf("datasource update guide missing %q", required)
		}
	}
}

func TestAITableImportGoldenRoutesDoNotBypassConfirmation(t *testing.T) {
	unsafeExample := regexp.MustCompile("dws aitable \\+import-file[^`\\n]*--yes")
	if err := fs.WalkDir(FS, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, err := FS.ReadFile(path)
		if err != nil {
			return err
		}
		lines := strings.Split(string(data), "\n")
		for lineNumber := 0; lineNumber < len(lines); lineNumber++ {
			command := lines[lineNumber]
			if !strings.Contains(command, "dws aitable +import-file") {
				continue
			}
			for strings.HasSuffix(strings.TrimSpace(command), "\\") && lineNumber+1 < len(lines) {
				lineNumber++
				command += " " + strings.TrimSpace(lines[lineNumber])
			}
			if unsafeExample.MatchString(command) {
				t.Errorf("%s:%d pre-populates --yes: %s", path, lineNumber+1, command)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		path     string
		required []string
	}{
		{
			path: "multi/dingtalk-aitable/SKILL.md",
			required: []string{
				"dws aitable +import-file --base-id <BASE_ID> --file <FILE_PATH>",
				"首次调用先触发 Runtime 确认门禁",
			},
		},
		{
			path: "mono/references/products/aitable/aitable-export-import.md",
			required: []string{
				"dws aitable +import-file --base-id <BASE_ID> --file data.xlsx",
				"首次调用先触发 Runtime 确认门禁",
				"再由执行方追加 `--yes`",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			data, err := FS.ReadFile(tt.path)
			if err != nil {
				t.Fatal(err)
			}
			content := string(data)
			for _, required := range tt.required {
				if !strings.Contains(content, required) {
					t.Fatalf("%s missing %q", tt.path, required)
				}
			}
		})
	}
}

func TestAITableWorkflowDisableGoldenRoutesDoNotBypassConfirmation(t *testing.T) {
	if err := fs.WalkDir(FS, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, err := FS.ReadFile(path)
		if err != nil {
			return err
		}
		lines := strings.Split(string(data), "\n")
		for lineNumber := 0; lineNumber < len(lines); lineNumber++ {
			command := lines[lineNumber]
			if !strings.Contains(command, "workflow disable") {
				continue
			}
			for strings.HasSuffix(strings.TrimSpace(command), "\\") && lineNumber+1 < len(lines) {
				lineNumber++
				command += " " + strings.TrimSpace(lines[lineNumber])
			}
			if strings.Contains(command, "--yes") {
				t.Errorf("%s:%d pre-populates --yes for workflow disable: %s", path, lineNumber+1, command)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAITableCommentDeleteGoldenRoutesDoNotBypassConfirmation(t *testing.T) {
	for _, path := range []string{
		"multi/dingtalk-aitable/references/aitable.md",
		"multi/dingtalk-aitable/references/aitable/aitable-comment.md",
		"mono/references/products/aitable.md",
		"mono/references/products/aitable/aitable-comment.md",
	} {
		t.Run(path, func(t *testing.T) {
			data, err := FS.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for lineNumber, line := range strings.Split(string(data), "\n") {
				if !strings.Contains(line, "comment delete") {
					continue
				}
				found = true
				if strings.Contains(line, "--comment-key --yes") {
					t.Fatalf("%s:%d pre-populates --yes in required parameters: %s", path, lineNumber+1, line)
				}
				if !strings.Contains(line, "确认后再追加 `--yes`") {
					t.Fatalf("%s:%d must explain the post-confirmation --yes step: %s", path, lineNumber+1, line)
				}
			}
			if !found {
				t.Fatalf("%s missing comment delete guidance", path)
			}
		})
	}
}
