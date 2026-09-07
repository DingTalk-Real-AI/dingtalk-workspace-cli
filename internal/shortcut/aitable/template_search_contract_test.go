// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"fmt"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageTemplateSearchRequiresNonEmptyQueryBeforeMCP(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "missing"},
		{name: "blank", args: []string{"--query", "   "}},
	} {
		t.Run(test.name, func(t *testing.T) {
			caller := &upsertByKeyCaller{}
			out, err := runAITableCompositeCLI(t, caller, "+template-search", test.args...)
			if err == nil || out != "" {
				t.Fatalf("query validation = output:%q err:%v", out, err)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("invalid query made %d MCP calls", len(caller.calls))
			}
		})
	}
}

func TestCrossPlatformCoverageTemplateSearchTrimsQueryPayload(t *testing.T) {
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{
		text: `{"data":{"templates":[{"templateId":"template-1","name":"项目模板"}],"hasMore":false}}`,
	}}}
	out, err := runAITableCompositeCLI(t, caller, "+template-search", "--query", "  项目  ")
	if err != nil {
		t.Fatalf("template search error = %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "search_templates" || caller.calls[0].args["query"] != "项目" {
		t.Fatalf("template search calls = %#v", caller.calls)
	}
	if !strings.Contains(out, `"templateId": "template-1"`) {
		t.Fatalf("template search output = %s", out)
	}
}

func TestCrossPlatformCoverageTemplateSearchSelectionDoesNotClaimPopularFallback(t *testing.T) {
	for _, text := range append([]string{TemplateSearch.Intent}, TemplateSearch.Contract.Selection.UseWhen...) {
		if strings.Contains(text, "返回热门") || strings.Contains(text, "不传关键词") {
			t.Fatalf("template search still claims unsupported fallback: %q", text)
		}
	}
	if len(TemplateSearch.Flags) == 0 || TemplateSearch.Flags[0].Name != "query" || !TemplateSearch.Flags[0].Required {
		t.Fatalf("template search query flag is not required: %#v", TemplateSearch.Flags)
	}
}

func TestCrossPlatformCoverageTemplateSearchPublishesAdvancingPagination(t *testing.T) {
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{
		text: `{"data":{"templates":[{"templateId":"template-1","name":"项目模板"}],"hasMore":true,"nextCursor":"cursor-2"}}`,
	}}}
	out, err := runAITableCompositeCLI(t, caller, "+template-search", "--query", "项目")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"endpoint_exhausted": false`, `"next_token": "cursor-2"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("template pagination missing %s: %s", want, out)
		}
	}
}

func TestCrossPlatformCoverageTemplateSearchRejectsInvalidPaginationEvidence(t *testing.T) {
	for _, test := range []struct {
		name     string
		cursor   string
		response string
	}{
		{name: "missing hasMore", response: `{"data":{"templates":[]}}`},
		{name: "missing next cursor", response: `{"data":{"templates":[],"hasMore":true}}`},
		{name: "non advancing cursor", cursor: "same", response: `{"data":{"templates":[],"hasMore":true,"nextCursor":"same"}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: test.response}}}
			args := []string{"--query", "项目"}
			if test.cursor != "" {
				args = append(args, "--cursor", test.cursor)
			}
			out, err := runAITableCompositeCLI(t, caller, "+template-search", args...)
			if err == nil || out != "" || len(caller.calls) != 1 {
				t.Fatalf("pagination validation = output:%q err:%v calls:%#v", out, err, caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageTemplateSearchRejectsMalformedStableIdentity(t *testing.T) {
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{
		text: `{"data":{"templates":[{"templateId":42,"name":"bad"}],"hasMore":false}}`,
	}}}
	out, err := runAITableCompositeCLI(t, caller, "+template-search", "--query", "项目")
	if err == nil || out != "" || len(caller.calls) != 1 {
		t.Fatalf("malformed identity = output:%q err:%v calls:%#v", out, err, caller.calls)
	}
}

func TestCrossPlatformCoverageTemplateSearchRejectsConflictingPageEnvelopes(t *testing.T) {
	responses := []string{
		`{"hasMore":false,"data":{"templates":[],"hasMore":true,"nextCursor":"next"}}`,
		`{"data":{"templates":[],"hasMore":true,"nextCursor":"next"},"result":{"hasMore":false}}`,
		`{"data":{"templates":[],"list":[],"hasMore":false}}`,
		`{"nextCursor":"outer","data":{"templates":[],"hasMore":true}}`,
	}
	for index, response := range responses {
		t.Run(fmt.Sprintf("conflict-%d", index), func(t *testing.T) {
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: response}}}
			out, err := runAITableCompositeCLI(t, caller, "+template-search", "--query", "项目")
			if err == nil || out != "" || len(caller.calls) != 1 {
				t.Fatalf("conflicting page envelope = output:%q err:%v calls:%#v", out, err, caller.calls)
			}
		})
	}
}
