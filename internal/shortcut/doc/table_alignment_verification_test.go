// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package doc

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

const alignedTable = "| Metric | Value |\n| --- | ---: |\n| A | 12 |"
const exportedTable = "| Metric | Value |\n| --- | --- |\n| A | 12 |"

func tableJSONML(alignment string) string {
	rows := []any{}
	for _, texts := range [][]string{{"Metric", "Value"}, {"A", "12"}} {
		row := []any{"tr", map[string]any{}}
		for column, text := range texts {
			attrs := map[string]any{}
			if column == 1 && alignment != "" {
				attrs["jc"] = alignment
			}
			row = append(row, []any{"tc", map[string]any{}, []any{"p", attrs, []any{"span", map[string]any{}, text}}})
		}
		rows = append(rows, row)
	}
	table := append([]any{"table", map[string]any{}}, rows...)
	b, _ := json.Marshal([]any{"root", map[string]any{}, table})
	return string(b)
}

func TestCrossPlatformCoverageTableAlignmentUsesJSONML(t *testing.T) {
	for _, tc := range []struct {
		name, markdown, jsonml string
		want                   bool
	}{
		{"export omits alignment", exportedTable, tableJSONML("right"), true},
		{"actual alignment missing", alignedTable, tableJSONML(""), false},
		{"actual alignment wrong", alignedTable, tableJSONML("center"), false},
		{"value changed", strings.Replace(exportedTable, "12", "13", 1), tableJSONML("right"), false},
		{"JSONML missing", exportedTable, "", false},
		{"JSONML malformed", exportedTable, "[", false},
		{"JSONML is fragment", exportedTable, `["p",{},"12"]`, false},
		{"JSONML row missing", exportedTable, `["root",{},["table",{}]]`, false},
		{"body missing", "| Metric | Value |\n| --- | --- |", tableJSONML("right"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := map[string]any{"markdown": tc.markdown, "tableAlignmentJSONML": map[string]any{"jsonml": tc.jsonml}}
			if got := verifyUpdatedDocumentContent(value, alignedTable, "overwrite", "markdown"); got != tc.want {
				t.Fatalf("verified=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageTableAlignmentAppendAndMultipleTables(t *testing.T) {
	var root []any
	if err := json.Unmarshal([]byte(tableJSONML("right")), &root); err != nil {
		t.Fatal(err)
	}
	full, _ := json.Marshal(append(root, root[2]))
	value := map[string]any{
		"markdown":             exportedTable + "\n\n" + exportedTable,
		tableAlignmentReadback: map[string]any{"jsonml": string(full)},
	}
	if !verifyUpdatedDocumentContent(value, alignedTable+"\n\n"+alignedTable, "overwrite", "markdown") {
		t.Fatal("two independent tables rejected")
	}
	if !verifyUpdatedDocumentContent(value, alignedTable, "append", "markdown") {
		t.Fatal("appended table rejected")
	}
	if verifyUpdatedDocumentContent(value, alignedTable, "overwrite", "markdown") {
		t.Fatal("extra table accepted for overwrite")
	}
	value[tableAlignmentReadback] = map[string]any{"jsonml": string(full), "complete": false}
	if verifyUpdatedDocumentContent(value, alignedTable, "append", "markdown") {
		t.Fatal("incomplete readback accepted")
	}
}

func TestCrossPlatformCoverageAlignedCreateAndReadFailure(t *testing.T) {
	testseam.Swap(t, &docVerifyWait, func(context.Context, time.Duration) error { return nil })
	testseam.Swap(t, &docVerifyDelays, []time.Duration{})
	caller := &docCoverageCaller{responses: map[string][]map[string]any{
		"get_document_content": {{"markdown": exportedTable}, {"jsonml": tableJSONML("right")}},
	}}
	result := runDocCoverageEnvelope(t, Create, caller, "--name", "Report", "--content", alignedTable, "--yes")
	encoded, _ := json.Marshal(result)
	if !strings.Contains(string(encoded), `"tableAlignment"`) {
		t.Fatal("receipt lacks JSONML alignment evidence")
	}
	caller = &docCoverageCaller{failAt: 3, responses: map[string][]map[string]any{
		"get_document_content": {{"markdown": alignedTable}},
	}}
	if err := runDocCoverage(t, Update, caller, "--node", "n", "--command", "overwrite", "--content", alignedTable, "--yes"); err == nil {
		t.Fatal("failed JSONML read accepted")
	}
	writes := 0
	for _, call := range caller.history {
		if call.tool == "update_document" {
			writes++
		}
	}
	if writes != 1 {
		t.Fatalf("writes=%d", writes)
	}
}

func TestCrossPlatformCoverageAlignedWriteReadsJSONMLWithoutRewriting(t *testing.T) {
	testseam.Swap(t, &docVerifyWait, func(context.Context, time.Duration) error { return nil })
	testseam.Swap(t, &docVerifyDelays, []time.Duration{})
	caller := &docCoverageCaller{responses: map[string][]map[string]any{
		"get_document_content": {{"markdown": exportedTable}, {"jsonml": tableJSONML("right")}},
	}}
	if err := runDocCoverage(t, Update, caller, "--node", "n", "--command", "overwrite", "--content", alignedTable, "--yes"); err != nil {
		t.Fatal(err)
	}
	writes, markdownReads, jsonmlReads := 0, 0, 0
	for _, call := range caller.history {
		if call.tool == "update_document" {
			writes++
		}
		if call.tool == "get_document_content" {
			if call.params["format"] == "jsonml" {
				jsonmlReads++
			} else {
				markdownReads++
			}
		}
	}
	if writes != 1 || markdownReads != 1 || jsonmlReads != 1 {
		t.Fatalf("writes=%d markdown=%d jsonml=%d", writes, markdownReads, jsonmlReads)
	}
}

func TestMarkdownEscapedLiteralDoesNotInventWhitespace(t *testing.T) {
	if !markdownSemanticallyEquivalent("Profit A\\*B=C", "Profit A*B=C") {
		t.Fatal("escaping a literal asterisk changed verification")
	}
	if markdownSemanticallyEquivalent("Profit A B=C", "Profit AB=C") {
		t.Fatal("authored whitespace difference was lost")
	}
}

func TestMarkdownEscapedArithmeticInTable(t *testing.T) {
	expected := "| Detail |\n| --- |\n| 预计 R3*5=15分 |"
	actual := "| Detail |\n| --- |\n| 预计 R3\\*5=15分 |"
	a, aOK := markdownServiceSemanticFingerprint(actual)
	e, eOK := markdownServiceSemanticFingerprint(expected)
	if !aOK || !eOK || a != e {
		t.Fatal("escaped multiplication changes structural text fingerprint")
	}
}

func TestCrossPlatformCoverageAlignedTableEntityEncoding(t *testing.T) {
	expected := strings.Replace(alignedTable, "Metric", "A & B", 1)
	actual := strings.Replace(exportedTable, "Metric", "A &amp; B", 1)
	value := map[string]any{"markdown": actual, tableAlignmentReadback: map[string]any{"jsonml": tableJSONML("right")}}
	if !verifyUpdatedDocumentContent(value, expected, "overwrite", "markdown") {
		t.Fatal("equivalent entity encoding rejected")
	}
}

func TestCrossPlatformCoverageEmptyOverwriteStillReadsContent(t *testing.T) {
	testseam.Swap(t, &docVerifyWait, func(context.Context, time.Duration) error { return nil })
	testseam.Swap(t, &docVerifyDelays, []time.Duration{})
	caller := &docCoverageCaller{responses: map[string][]map[string]any{
		"get_document_content": {{"markdown": ""}},
	}}
	if err := runDocCoverageInput(t, Update, caller, strings.NewReader(""), "--node", "n", "--command", "overwrite", "--content", "-", "--yes"); err != nil {
		t.Fatal(err)
	}
	for _, call := range caller.history {
		if call.tool == "get_document_info" {
			t.Fatal("content verification replaced by metadata")
		}
	}
}
