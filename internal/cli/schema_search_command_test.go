// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func TestSchemaSearchCommandExactAndVersioned(t *testing.T) {
	installSchemaSearchTestEngine(t)
	root := &cobra.Command{Use: "dws"}
	root.AddCommand(NewSchemaCommand())
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetArgs([]string{"schema", "search", "--query", "chat.read_status"})
	if err := root.Execute(); err != nil {
		t.Fatalf("schema search execute: %v", err)
	}
	var response ToolSearchResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v\n%s", err, stdout.String())
	}
	if response.Version != "tool-search.v1" || response.Strategy != "exact_guard" {
		t.Fatalf("response = %#v", response)
	}
	if len(response.Candidates) != 1 || response.Candidates[0].CanonicalPath != "chat.read_status" {
		t.Fatalf("candidates = %#v", response.Candidates)
	}
	wantCatalog := CatalogVersionRef{SourceHash: "source-test", SurfaceHash: "surface-test"}
	if response.Catalog != wantCatalog {
		t.Fatalf("Catalog = %#v, want %#v", response.Catalog, wantCatalog)
	}
	for _, removed := range []string{"external_ranking", "degraded", "degraded_reason_code", "warning_codes", "_fallback"} {
		if strings.Contains(stdout.String(), removed) {
			t.Fatalf("removed fallback field %q leaked into response: %s", removed, stdout.String())
		}
	}
}

func TestSchemaSearchRequestRejectsUnboundedOrAmbiguousInput(t *testing.T) {
	if _, err := decodeToolSearchV1Request(strings.NewReader(`{"version":"tool-search.v1","query":"x","unknown":true}`)); validationReason(err) != "invalid_request_json" {
		t.Fatal("unknown field was accepted")
	}
	if _, err := decodeToolSearchV1Request(strings.NewReader(`{"version":"tool-search.v1","query":"x","external_ranking":{}}`)); validationReason(err) != "invalid_request_json" {
		t.Fatal("removed external_ranking field was accepted")
	}
	oversized := strings.NewReader(strings.Repeat("x", maxToolSearchRequestBytes+1))
	if _, err := decodeToolSearchV1Request(oversized); validationReason(err) != "request_too_large" {
		t.Fatal("oversized request was accepted")
	}
	_, err := validateToolSearchV1Request(ToolSearchV1Request{Query: "x", Subqueries: []string{"x"}})
	if validationReason(err) != "unsupported_version" || !strings.Contains(err.Error(), `"version":"tool-search.v1"`) || !strings.Contains(err.Error(), "subqueries") {
		t.Fatalf("missing-version guidance = %v", err)
	}
}

func TestSchemaSearchHelpDocumentsStructuredRequest(t *testing.T) {
	command := newSchemaSearchCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"--help"})
	if err := command.Execute(); err != nil {
		t.Fatalf("schema search help: %v", err)
	}
	for _, required := range []string{`"version":"tool-search.v1"`, `"subqueries"`, "--request-json -", "不支持 --fields、--jq", "非 JSON --format"} {
		if !strings.Contains(output.String(), required) {
			t.Fatalf("schema search help omits %q:\n%s", required, output.String())
		}
	}
}

func installSchemaSearchTestEngine(t *testing.T) {
	t.Helper()
	testseam.Swap(t, &schemaSearchNewEngine, func() (*ToolSearchEngine, error) {
		return newToolSearchTestEngine(t), nil
	})
}

func validationReason(err error) string {
	var typed *apperrors.Error
	if errors.As(err, &typed) && typed.Category == apperrors.CategoryValidation {
		return typed.Reason
	}
	return ""
}
