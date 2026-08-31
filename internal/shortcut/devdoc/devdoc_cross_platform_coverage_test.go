// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package devdoc

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type devdocCoverageCaller struct {
	response string
	err      error
	calls    int
}

func (caller *devdocCoverageCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	caller.calls++
	if caller.err != nil {
		return nil, caller.err
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: caller.response}}}, nil
}
func (*devdocCoverageCaller) Format() string { return "json" }
func (*devdocCoverageCaller) DryRun() bool   { return false }
func (*devdocCoverageCaller) Fields() string { return "" }
func (*devdocCoverageCaller) JQ() string     { return "" }

func runDevdocCoverage(t *testing.T, caller *devdocCoverageCaller, args ...string) (string, error) {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	service := &cobra.Command{Use: "devdoc"}
	service.AddCommand(corecmd.New(shortcut.FromShortcut(SearchDocs)))
	root.AddCommand(service)
	ctx, _ := output.WithResultStore(context.Background())
	root.SetContext(ctx)
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs(append([]string{"devdoc", SearchDocs.Command}, args...))
	executed, err := root.ExecuteC()
	if err == nil {
		_, _, err = output.EmitStoredResult(executed)
	}
	return stdout.String(), err
}

func TestCrossPlatformCoverageDevdocUnavailableContractIsUnifiedTypedAndTruthful(t *testing.T) {
	if SearchDocs.OutputRollout != output.RolloutUnifiedActive {
		t.Fatalf("rollout = %q", SearchDocs.OutputRollout)
	}
	if !SearchDocs.Hidden || SearchDocs.Availability != "unavailable" || SearchDocs.Contract.Interface == nil || SearchDocs.Contract.Interface.Availability != "unavailable" {
		t.Fatal("Devdoc semantic search must remain hidden/unavailable until a guaranteed zero-match query is provable")
	}
	if SearchDocs.Contract.Empty() || SearchDocs.Contract.Result == nil || SearchDocs.Contract.Pagination == nil {
		t.Fatal("Devdoc Shortcut must retain Contract, Result, and Pagination for audit")
	}
	resultSchema := string(SearchDocs.Contract.Result.DataSchema)
	if !strings.Contains(resultSchema, "matchSemantics") || !strings.Contains(resultSchema, "zeroMatchProven") || strings.TrimSpace(SearchDocs.Safety.Effect) == "" {
		t.Fatal("Devdoc Result must declare semantic matching and the unproven zero-match boundary")
	}
}

func TestCrossPlatformCoverageDevdocSearchRejectsBrokenResponses(t *testing.T) {
	explicitEmpty := map[string]any{
		"success": true,
		"result": map[string]any{
			"items": []any{}, "currentPage": float64(3), "pageSize": float64(10),
			"totalCount": float64(2), "hasMore": false,
		},
	}
	documents, page, err := devdocProjectSearch(explicitEmpty, 3, 10)
	if err != nil || len(documents) != 0 || page.HasMore {
		t.Fatalf("explicit empty page: documents=%v page=%+v err=%v", documents, page, err)
	}
	// This proves only that an explicitly empty terminal page is parsed
	// truthfully. It is deliberately not evidence for a zero-match query.

	broken := map[string]map[string]any{
		"empty response":        {},
		"missing success":       {"result": map[string]any{"items": []any{}}},
		"success false":         {"success": false, "result": map[string]any{"items": []any{}}},
		"wrong success type":    {"success": "true", "result": map[string]any{"items": []any{}}},
		"missing collection":    {"success": true, "result": map[string]any{"hasMore": false}},
		"wrong collection type": {"success": true, "result": map[string]any{"items": map[string]any{}, "currentPage": float64(1), "pageSize": float64(10), "totalCount": float64(0), "hasMore": false}},
		"bad item":              {"success": true, "result": map[string]any{"items": []any{"bad"}, "currentPage": float64(1), "pageSize": float64(10), "totalCount": float64(1), "hasMore": false}},
		"missing item identity": {"success": true, "result": map[string]any{"items": []any{map[string]any{"title": "doc"}}, "currentPage": float64(1), "pageSize": float64(10), "totalCount": float64(1), "hasMore": false}},
		"duplicate identity":    {"success": true, "result": map[string]any{"items": []any{map[string]any{"title": "one", "url": "https://example.invalid/doc"}, map[string]any{"title": "two", "url": "https://example.invalid/doc"}}, "currentPage": float64(1), "pageSize": float64(10), "totalCount": float64(2), "hasMore": false}},
		"missing pagination":    {"success": true, "result": map[string]any{"items": []any{}}},
		"mismatched page":       {"success": true, "result": map[string]any{"items": []any{}, "currentPage": float64(2), "pageSize": float64(10), "totalCount": float64(0), "hasMore": false}},
		"wrong hasMore type":    {"success": true, "result": map[string]any{"items": []any{}, "currentPage": float64(1), "pageSize": float64(10), "totalCount": float64(0), "hasMore": "false"}},
		"false completion":      {"success": true, "result": map[string]any{"items": []any{}, "currentPage": float64(1), "pageSize": float64(10), "totalCount": float64(11), "hasMore": false}},
		"false continuation":    {"success": true, "result": map[string]any{"items": []any{}, "currentPage": float64(1), "pageSize": float64(10), "totalCount": float64(10), "hasMore": true}},
	}
	for name, payload := range broken {
		t.Run(name, func(t *testing.T) {
			if got, _, err := devdocProjectSearch(payload, 1, 10); err == nil {
				t.Fatalf("broken response accepted: %#v", got)
			}
		})
	}
}

func TestCrossPlatformCoverageDevdocSearchProjectsKnownNonemptyPage(t *testing.T) {
	payload := map[string]any{
		"success": true,
		"result": map[string]any{
			"items": []any{
				map[string]any{"title": "Authentication", "url": "https://example.invalid/auth", "desc": "Guide"},
				map[string]any{"title": "Errors", "url": "https://example.invalid/errors", "desc": nil},
			},
			"currentPage": float64(1), "pageSize": float64(2), "totalCount": float64(3), "hasMore": true,
		},
	}
	documents, page, err := devdocProjectSearch(payload, 1, 2)
	if err != nil || len(documents) != 2 || !page.HasMore {
		t.Fatalf("documents=%#v page=%+v err=%v", documents, page, err)
	}
	if documents[0]["description"] != "Guide" {
		t.Fatalf("first document=%#v", documents[0])
	}
	if _, exists := documents[1]["description"]; exists {
		t.Fatalf("nil description must be omitted: %#v", documents[1])
	}
}

func TestCrossPlatformCoverageDevdocExecuteValidationAndPagination(t *testing.T) {
	blankCommand := corecmd.New(shortcut.FromShortcut(SearchDocs))
	if err := blankCommand.Flags().Set("query", "   "); err != nil {
		t.Fatal(err)
	}
	if err := SearchDocs.Validate(shortcut.RuntimeContextForTest(blankCommand, SearchDocs)); err == nil {
		t.Fatal("blank query validation succeeded")
	}
	for name, args := range map[string][]string{
		"blank query": {"--query", "   "},
		"zero page":   {"--query", "query", "--page", "0"},
		"zero size":   {"--query", "query", "--size", "0"},
		"large size":  {"--query", "query", "--size", "51"},
	} {
		t.Run(name, func(t *testing.T) {
			caller := &devdocCoverageCaller{}
			if _, err := runDevdocCoverage(t, caller, args...); err == nil {
				t.Fatal("invalid input succeeded")
			}
			if caller.calls != 0 {
				t.Fatalf("remote calls before validation = %d", caller.calls)
			}
		})
	}

	for name, response := range map[string]string{
		"terminal":   `{"success":true,"result":{"items":[{"title":"Doc","url":"https://example.invalid/doc"}],"currentPage":1,"pageSize":10,"totalCount":1,"hasMore":false}}`,
		"continuing": `{"success":true,"result":{"items":[{"title":"Doc","url":"https://example.invalid/doc"}],"currentPage":1,"pageSize":1,"totalCount":2,"hasMore":true}}`,
	} {
		t.Run(name, func(t *testing.T) {
			caller := &devdocCoverageCaller{response: response}
			args := []string{"--query", " query "}
			if name == "continuing" {
				args = append(args, "--size", "1")
			}
			if _, err := runDevdocCoverage(t, caller, args...); err != nil {
				t.Fatal(err)
			}
			if caller.calls != 1 {
				t.Fatalf("calls = %d", caller.calls)
			}
		})
	}

	caller := &devdocCoverageCaller{err: errors.New("transport")}
	if _, err := runDevdocCoverage(t, caller, "--query", "query"); err == nil {
		t.Fatal("transport failure succeeded")
	}
	malformed := &devdocCoverageCaller{response: `{"success":true,"result":{"items":"bad"}}`}
	if _, err := runDevdocCoverage(t, malformed, "--query", "query"); err == nil {
		t.Fatal("malformed projection succeeded")
	}
}

func TestCrossPlatformCoverageDevdocPageAndScalarEdges(t *testing.T) {
	base := func() map[string]any {
		return map[string]any{
			"success": true,
			"result": map[string]any{
				"items": []any{}, "currentPage": float64(1), "pageSize": float64(2),
				"totalCount": float64(0), "hasMore": false,
			},
		}
	}
	for name, mutate := range map[string]func(map[string]any){
		"page size mismatch": func(result map[string]any) { result["pageSize"] = float64(3) },
		"negative total":     func(result map[string]any) { result["totalCount"] = float64(-1) },
		"total below items": func(result map[string]any) {
			result["items"] = []any{map[string]any{"title": "one", "url": "https://example.invalid/one"}}
			result["totalCount"] = float64(0)
		},
		"items exceed size": func(result map[string]any) {
			result["items"] = []any{
				map[string]any{"title": "one", "url": "https://example.invalid/one"},
				map[string]any{"title": "two", "url": "https://example.invalid/two"},
				map[string]any{"title": "three", "url": "https://example.invalid/three"},
			}
			result["totalCount"] = float64(3)
		},
	} {
		t.Run(name, func(t *testing.T) {
			payload := base()
			mutate(payload["result"].(map[string]any))
			if _, _, err := devdocProjectSearch(payload, 1, 2); err == nil {
				t.Fatal("invalid page succeeded")
			}
		})
	}

	for _, value := range []any{math.NaN(), math.Inf(1), 1.5, float64(math.MaxInt) * 2, "1"} {
		if _, ok := devdocInteger(value); ok {
			t.Fatalf("invalid integer accepted: %#v", value)
		}
	}
	if got, ok := devdocInteger(7); !ok || got != 7 {
		t.Fatalf("int projection = %d, %v", got, ok)
	}
	if _, ok := devdocOptionalString(7); ok {
		t.Fatal("non-string optional value accepted")
	}
}
