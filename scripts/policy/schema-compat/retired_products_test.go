// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"strings"
	"testing"
)

func recruitRetirementContract() schemaContract {
	return schemaContract{Version: schemaContractVersion, Products: map[string]productSchema{
		"recruit": {Tools: map[string]toolSchema{
			"recruit.list_jobs":      {PrimaryCLIPath: "recruit job list"},
			"recruit.get_job_detail": {PrimaryCLIPath: "recruit job get"},
			"recruit.create_job":     {PrimaryCLIPath: "recruit job create"},
		}},
	}}
}

func TestCrossPlatformCoverageReviewedRecruitSchemaRetirement(t *testing.T) {
	baseline := recruitRetirementContract()
	current := schemaContract{Version: schemaContractVersion, Products: map[string]productSchema{}}
	if failures := checkCompatibility(baseline, current); len(failures) != 0 {
		t.Fatalf("PR #1485 retirement still blocks unrelated PRs: %v", failures)
	}
	if failures := checkCompatibility(current, current); len(failures) != 0 {
		t.Fatalf("already retired main: %v", failures)
	}
}

func TestCrossPlatformCoverageReviewedRecruitSchemaRetirementBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*schemaContract, *schemaContract)
	}{
		{"historical empty product", func(b, c *schemaContract) { b.Products["recruit"] = productSchema{} }},
		{"historical missing tool", func(b, c *schemaContract) { delete(b.Products["recruit"].Tools, "recruit.create_job") }},
		{"historical extra tool", func(b, c *schemaContract) { b.Products["recruit"].Tools["recruit.other"] = toolSchema{} }},
		{"historical unknown tool", func(b, c *schemaContract) {
			delete(b.Products["recruit"].Tools, "recruit.create_job")
			b.Products["recruit"].Tools["recruit.other"] = toolSchema{PrimaryCLIPath: "recruit job create"}
		}},
		{"historical wrong path", func(b, c *schemaContract) {
			b.Products["recruit"].Tools["recruit.create_job"] = toolSchema{PrimaryCLIPath: "recruit job other"}
		}},
		{"partial product retirement", func(b, c *schemaContract) {
			c.Products["recruit"] = productSchema{Tools: map[string]toolSchema{"recruit.list_jobs": {PrimaryCLIPath: "recruit job list"}}}
		}},
		{"renamed product", func(b, c *schemaContract) { c.Products["hiring"] = b.Products["recruit"] }},
		{"renamed tool keeps path", func(b, c *schemaContract) {
			c.Products["hiring"] = productSchema{Tools: map[string]toolSchema{"hiring.list": {PrimaryCLIPath: "recruit job list"}}}
		}},
		{"renamed tool keeps root", func(b, c *schemaContract) {
			c.Products["hiring"] = productSchema{Tools: map[string]toolSchema{"hiring.list": {PrimaryCLIPath: "recruit"}}}
		}},
		{"similar product removed", func(b, c *schemaContract) { b.Products["recruiter"] = productSchema{} }},
		{"other product removed", func(b, c *schemaContract) { b.Products["contact"] = productSchema{} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseline := recruitRetirementContract()
			current := schemaContract{Version: schemaContractVersion, Products: map[string]productSchema{}}
			tt.mutate(&baseline, &current)
			if failures := checkCompatibility(baseline, current); len(failures) == 0 {
				t.Fatal("retirement hid unapproved Schema deletion")
			}
		})
	}
	baseline := baselineContract()
	baseline.Products["recruit"] = recruitRetirementContract().Products["recruit"]
	current := baselineContract()
	// 其他产品仍在且不变时可以通过；它的安全和参数变化仍然必须失败。
	if failures := checkCompatibility(baseline, current); len(failures) != 0 {
		t.Fatalf("unrelated product: %v", failures)
	}
	mutateTool(&current, func(tool *toolSchema) { tool.Risk = "high"; delete(tool.Parameters, "title") })
	failures := strings.Join(checkCompatibility(baseline, current), "\n")
	if !strings.Contains(failures, "changed risk") || !strings.Contains(failures, "lost parameter") {
		t.Fatalf("unrelated Schema regression hidden: %s", failures)
	}
}
