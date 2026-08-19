// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package unit_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageOAAttachmentDeliveryPolicy(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))

	attachmentFile := filepath.Join(root, "internal", "helpers", "oa_attachment.go")
	if _, err := os.Stat(attachmentFile); !os.IsNotExist(err) {
		t.Fatalf("OA attachment commands must live in internal/helpers/oa.go; separate file still exists: %s", attachmentFile)
	}

	oaSourcePath := filepath.Join(root, "internal", "helpers", "oa.go")
	oaFile, err := parser.ParseFile(token.NewFileSet(), oaSourcePath, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", oaSourcePath, err)
	}
	wantTools := map[string]string{
		"download-url":       "get_attachment_download_url",
		"authorize-download": "auth_download_file",
		"authorize-preview":  "auth_preview_attachment",
	}
	gotTools := make(map[string]string, len(wantTools))
	foundConstructor := false
	for _, declaration := range oaFile.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "newOAAttachmentCommand" {
			continue
		}
		foundConstructor = true
		ast.Inspect(function.Body, func(node ast.Node) bool {
			literal, ok := node.(*ast.CompositeLit)
			if !ok || !isOAAttachmentLeafSpec(literal) {
				return true
			}
			use := oaAttachmentLeafStringField(t, literal, "Use")
			if _, expected := wantTools[use]; expected {
				gotTools[use] = oaAttachmentLeafStringField(t, literal, "Tool")
			}
			return true
		})
	}
	if !foundConstructor {
		t.Fatalf("%s missing newOAAttachmentCommand", oaSourcePath)
	}
	if !reflect.DeepEqual(gotTools, wantTools) {
		t.Fatalf("%s attachment Use/Tool declarations = %#v, want %#v", oaSourcePath, gotTools, wantTools)
	}

	docPaths := []string{
		filepath.Join(root, "skills", "mono", "references", "products", "oa.md"),
		filepath.Join(root, "skills", "multi", "dingtalk-misc", "references", "oa.md"),
	}
	for _, docPath := range docPaths {
		doc, err := os.ReadFile(docPath)
		if err != nil {
			t.Fatalf("read %s: %v", docPath, err)
		}
		for _, required := range []string{
			"dws oa approval attachment download-url",
			"dws oa approval attachment authorize-download",
			"dws oa approval attachment authorize-preview",
			"临时下载链接",
			"最多 10",
			"最多 20",
		} {
			if !strings.Contains(string(doc), required) {
				t.Errorf("%s missing OA attachment Skill guidance %q", docPath, required)
			}
		}
	}
}

func isOAAttachmentLeafSpec(literal *ast.CompositeLit) bool {
	identifier, ok := literal.Type.(*ast.Ident)
	return ok && identifier.Name == "LeafSpec"
}

func oaAttachmentLeafStringField(t *testing.T, literal *ast.CompositeLit, fieldName string) string {
	t.Helper()
	for _, element := range literal.Elts {
		keyValue, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := keyValue.Key.(*ast.Ident)
		if !ok || key.Name != fieldName {
			continue
		}
		value, ok := keyValue.Value.(*ast.BasicLit)
		if !ok || value.Kind != token.STRING {
			t.Fatalf("LeafSpec.%s must be a string literal", fieldName)
		}
		decoded, err := strconv.Unquote(value.Value)
		if err != nil {
			t.Fatalf("decode LeafSpec.%s: %v", fieldName, err)
		}
		return decoded
	}
	return ""
}
