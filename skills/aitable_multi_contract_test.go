// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package skills

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

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
