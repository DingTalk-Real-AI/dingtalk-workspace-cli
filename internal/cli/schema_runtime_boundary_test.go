// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemaruntime"
)

func TestSchemaRuntimeParentKeepsPositionalListCompatibility(t *testing.T) {
	render := func(args []string) []byte {
		t.Helper()
		command := NewSchemaCommand()
		var output bytes.Buffer
		command.SetOut(&output)
		if err := command.RunE(command, args); err != nil {
			t.Fatalf("schema %v error = %v", args, err)
		}
		return output.Bytes()
	}
	if overview, list := render(nil), render([]string{"list"}); !bytes.Equal(overview, list) {
		t.Fatal("schema list differs from the no-argument overview")
	}
}

func TestSchemaRuntimeChildQueryParityAllAssembledLocators(t *testing.T) {
	loaded := deliverySchemaCatalog()
	if err := deliverySchemaCatalogError(); err != nil {
		t.Fatalf("delivery Schema Catalog error = %v", err)
	}
	checked := 0
	for _, product := range loaded.Registry.Products {
		for _, tool := range product.Tools {
			locators := []string{
				tool.Identity.CanonicalPath,
				tool.Identity.Path,
				tool.Identity.CLIPath,
				tool.Identity.PrimaryCLIPath,
			}
			if tool.Identity.SourceProductID != "" && tool.Identity.SourceProductID != tool.Identity.ProductID {
				locators = append(locators, tool.Identity.SourceProductID+"."+tool.Identity.Name)
			}
			locators = append(locators, tool.Identity.Aliases...)
			for _, path := range append([]string(nil), locators...) {
				normalized := normalizeSchemaCLIPath(path)
				if strings.Contains(normalized, " ") {
					locators = append(locators,
						strings.ReplaceAll(normalized, " ", "."),
						strings.ReplaceAll(normalized, " ", "/"),
						strings.ReplaceAll(normalized, " ", "  "),
					)
				}
			}
			seen := map[string]bool{}
			for _, locator := range locators {
				locator = strings.TrimSpace(locator)
				if locator == "" || seen[locator] {
					continue
				}
				seen[locator] = true
				resolved, ok := loaded.Index.ResolveQuery(locator)
				if !ok {
					// Dot/slash normalization is compatibility for CLI paths, not
					// an inference layer for unrelated contract identifiers.
					continue
				}
				if resolved.Identity.CanonicalPath != tool.Identity.CanonicalPath {
					t.Fatalf("ResolveQuery(%q) = %s, want %s", locator, resolved.Identity.CanonicalPath, tool.Identity.CanonicalPath)
				}
				parent, parentErr := schemaPayloadFromLoadedCatalog(loaded, []string{locator})
				child, childErr := schemaruntime.RenderQuery(loaded.Registry, loaded.Index, locator)
				if parentErr != nil || childErr != nil {
					t.Fatalf("query %q errors: parent=%v child=%v", locator, parentErr, childErr)
				}
				if !reflect.DeepEqual(parent, child) {
					t.Fatalf("query %q differs between parent and child", locator)
				}
				checked++
			}
		}
	}
	if checked < len(loaded.Index.CanonicalPaths()) {
		t.Fatalf("checked %d locators for %d tools", checked, len(loaded.Index.CanonicalPaths()))
	}
}
