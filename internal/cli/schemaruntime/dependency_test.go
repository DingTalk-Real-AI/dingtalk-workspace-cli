// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemaruntime

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Check actual transitive dependencies as well as direct source imports: an
// allowed DTO/helper must not silently bring the assembler or network back in.
func TestCrossPlatformCoverageThinSchemaDependencyClosure(t *testing.T) {
	const module = "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/"
	allowed := map[string]bool{
		module + "internal/cli/schemaruntime": true,
		module + "internal/cli/schemacachepb": true,
		module + "internal/corecmd/contract":  true,
		module + "internal/schemacache":       true,
		module + "internal/launcher":          true,
		module + "internal/schemareader":      true,
		module + "internal/skillpaths":        true,
		module + "internal/jsonutil":          true,
	}
	command := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", ".", module+"internal/schemacache", module+"internal/launcher")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("resolve production dependency closure: %v\n%s", err, output)
	}
	for _, path := range strings.Fields(string(output)) {
		if strings.HasPrefix(path, module) && !allowed[path] {
			t.Errorf("thin Schema path reaches forbidden repository package %q", path)
		}
		if path == "net" || strings.HasPrefix(path, "net/") || strings.HasPrefix(path, "github.com/spf13/cobra") {
			t.Errorf("thin Schema path reaches forbidden network/framework package %q", path)
		}
	}
}

func TestProductionImportsStayFrameworkFree(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{
		"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemacachepb": true,
		"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract":  true,
		"google.golang.org/protobuf/proto":                                              true,
		"google.golang.org/protobuf/reflect/protoreflect":                               true,
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), filepath.Clean(name), nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", name, parseErr)
		}
		for _, spec := range file.Imports {
			path, unquoteErr := strconv.Unquote(spec.Path.Value)
			if unquoteErr != nil {
				t.Fatalf("unquote import in %s: %v", name, unquoteErr)
			}
			if allowed[path] || !strings.Contains(path, ".") {
				continue
			}
			t.Errorf("%s imports forbidden non-stdlib dependency %q", name, path)
		}
	}
}
