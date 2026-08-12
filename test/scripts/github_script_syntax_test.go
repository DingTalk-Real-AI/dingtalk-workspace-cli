// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package scripts_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGitHubScriptWorkflowJavaScriptParses(t *testing.T) {
	t.Parallel()

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is required to syntax-check actions/github-script steps")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	paths, err := filepath.Glob(filepath.Join(root, ".github", "workflows", "*.yml"))
	if err != nil {
		t.Fatalf("glob workflows: %v", err)
	}

	checked := 0
	for _, path := range paths {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read %s: %v", path, readErr)
		}
		var workflow any
		if unmarshalErr := yaml.Unmarshal(data, &workflow); unmarshalErr != nil {
			t.Fatalf("parse %s: %v", path, unmarshalErr)
		}
		var scripts []string
		collectGitHubScripts(workflow, &scripts)
		for index, script := range scripts {
			checked++
			source := fmt.Sprintf(
				"async function workflowScript%d() {\n%s\n}\n",
				index,
				script,
			)
			scriptPath := filepath.Join(
				t.TempDir(),
				fmt.Sprintf("%s-%d.js", filepath.Base(path), index),
			)
			if writeErr := os.WriteFile(scriptPath, []byte(source), 0o600); writeErr != nil {
				t.Fatalf("write syntax fixture for %s: %v", path, writeErr)
			}
			command := exec.Command(node, "--check", scriptPath)
			if output, runErr := command.CombinedOutput(); runErr != nil {
				t.Errorf(
					"github-script syntax failed for %s step %d: %v\n%s",
					path,
					index,
					runErr,
					output,
				)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no actions/github-script steps were found")
	}
}

func collectGitHubScripts(value any, scripts *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		uses, _ := typed["uses"].(string)
		if strings.HasPrefix(uses, "actions/github-script@") {
			if with, ok := typed["with"].(map[string]any); ok {
				if script, ok := with["script"].(string); ok {
					*scripts = append(*scripts, script)
				}
			}
		}
		for _, child := range typed {
			collectGitHubScripts(child, scripts)
		}
	case []any:
		for _, child := range typed {
			collectGitHubScripts(child, scripts)
		}
	}
}
