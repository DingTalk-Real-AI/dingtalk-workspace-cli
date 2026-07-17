package helpers

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type aitableTestCall struct {
	server string
	tool   string
	args   map[string]any
}

type aitableTestCaller struct {
	responses []string
	errors    []error
	calls     []aitableTestCall
}

func (c *aitableTestCaller) CallTool(_ context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, aitableTestCall{server: server, tool: tool, args: args})
	index := len(c.calls) - 1
	if index < len(c.errors) && c.errors[index] != nil {
		return nil, c.errors[index]
	}
	response := `{"success":true}`
	if index < len(c.responses) {
		response = c.responses[index]
	}
	return textToolResult(response), nil
}
func (*aitableTestCaller) Format() string { return "json" }
func (*aitableTestCaller) DryRun() bool   { return false }
func (*aitableTestCaller) Fields() string { return "" }
func (*aitableTestCaller) JQ() string     { return "" }

func installAitableDeps(t *testing.T, caller *aitableTestCaller) *bytes.Buffer {
	t.Helper()
	oldDeps, oldArgs := deps, os.Args
	t.Cleanup(func() { deps, os.Args = oldDeps, oldArgs })
	InitDeps(caller)
	out := &bytes.Buffer{}
	deps.Out.w, deps.Out.errW = out, out
	os.Args = []string{"dws", "aitable"}
	return out
}

func TestCrossPlatformCoverageAitableFlagAndJSONNormalizers(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("enabled", "", "")
	cmd.Flags().String("records", "", "")
	cmd.Flags().String("records-file", "", "")
	cmd.Flags().String("fields", "", "")
	if err := cmd.Flags().Set("enabled", "true"); err != nil {
		t.Fatal(err)
	}
	if got, err := parseBoolFlag(cmd, "enabled"); err != nil || !got {
		t.Fatalf("parseBoolFlag true = %v, %v", got, err)
	}
	_ = cmd.Flags().Set("enabled", "invalid")
	if _, err := parseBoolFlag(cmd, "enabled"); err == nil {
		t.Fatal("invalid boolean should fail")
	}

	if _, err := resolveRecordsFlag(cmd); err == nil {
		t.Fatal("missing records should fail")
	}
	_ = cmd.Flags().Set("fields", `[{"cells":{}}]`)
	if got, err := resolveRecordsFlag(cmd); err != nil || got == "" {
		t.Fatalf("fields alias = %q, %v", got, err)
	}
	_ = cmd.Flags().Set("records", `[{"cells":{"x":1}}]`)
	if got, _ := resolveRecordsFlag(cmd); !strings.Contains(got, "x") {
		t.Fatalf("records value = %q", got)
	}
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Flags().Set("records-file", empty)
	if _, err := resolveRecordsFlag(cmd); err == nil {
		t.Fatal("empty records file should fail")
	}
	_ = cmd.Flags().Set("records-file", filepath.Join(dir, "missing"))
	if _, err := resolveRecordsFlag(cmd); err == nil {
		t.Fatal("missing records file should fail")
	}
	valid := filepath.Join(dir, "records.json")
	if err := os.WriteFile(valid, []byte("  [] \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Flags().Set("records-file", valid)
	if got, err := resolveRecordsFlag(cmd); err != nil || got != "[]" {
		t.Fatalf("records file = %q, %v", got, err)
	}

	filterCases := []any{
		nil,
		[]any{},
		map[string]any{},
		map[string]any{"operator": 1},
		map[string]any{"operator": "eq"},
		map[string]any{"operator": "and"},
		map[string]any{"operator": "and", "operands": "bad"},
		map[string]any{"operator": "and", "operands": []any{"skip", map[string]any{}, map[string]any{"operator": 1}}},
		map[string]any{"operator": "and", "operands": []any{map[string]any{"operator": "date_between"}}},
		map[string]any{"operator": "and", "operands": []any{map[string]any{"operator": "equals"}}},
		map[string]any{"operator": "and", "operands": []any{map[string]any{"operator": "eq"}}},
	}
	for index, value := range filterCases {
		err := validateFiltersStructure(value, "unused")
		if index == 0 || index == len(filterCases)-1 || index == 7 {
			if err != nil {
				t.Errorf("valid filter case %d: %v", index, err)
			}
		} else if err == nil {
			t.Errorf("invalid filter case %d should fail", index)
		}
	}

	input := map[string]any{"operator": "and", "operands": []any{
		"plain",
		map[string]any{"fieldId": "f1", "operator": "eq", "value": "v"},
		map[string]any{"fieldId": "f2", "operator": "exist"},
		map[string]any{"operator": "or", "operands": []any{map[string]any{"fieldId": "f3", "operator": "eq", "value": 3}}},
		map[string]any{"operator": "eq"},
	}}
	normalized := normalizeFilters(input).(map[string]any)
	if len(normalized["operands"].([]any)) != 5 {
		t.Fatalf("normalized filters = %#v", normalized)
	}
	for _, value := range []any{nil, "plain", map[string]any{}, map[string]any{"operands": "bad"}} {
		_ = normalizeFilters(value)
	}
	if got := normalizeViewConfigFilter(input); reflect.TypeOf(got).Kind() != reflect.Slice {
		t.Fatalf("view filter object = %#v", got)
	}
	_ = normalizeViewConfigFilter([]any{input})
	_ = normalizeViewConfigFilter("plain")
	for _, value := range []any{[]any{1}, map[string]any{"x": 1}, "plain"} {
		_ = ensureArray(value)
	}
}

func TestCrossPlatformCoverageAitableViewConfigAndHelpers(t *testing.T) {
	config := map[string]any{
		"filter": map[string]any{"operator": "and", "operands": []any{}},
		"sort":   map[string]any{"fieldId": "f"}, "group": []any{},
		"flags": true, "unknown": true,
	}
	if err := normalizeViewConfigBlock(config); err != nil {
		t.Fatalf("normalize view config: %v", err)
	}
	for _, key := range []string{"filter", "sort", "group"} {
		if reflect.TypeOf(config[key]).Kind() != reflect.Slice {
			t.Errorf("config %s = %#v", key, config[key])
		}
	}
	for _, bad := range []map[string]any{{"filter": 1}, {"sort": 1}, {"group": 1}} {
		if err := normalizeViewConfigBlock(bad); err == nil {
			t.Errorf("invalid view config %#v should fail", bad)
		}
	}

	for op, want := range map[string]string{"eq": "", "equals": "eq", "unknown": "eq"} {
		if got := suggestOperator(op); got != want {
			t.Errorf("suggestOperator(%q) = %q, want %q", op, got, want)
		}
	}
	for _, tc := range []struct {
		err  error
		want bool
	}{{nil, false}, {errors.New("timeout"), true}, {errors.New("SYSTEM_ERROR"), true}, {errors.New("retryable: true"), true}, {errors.New("bad request"), false}} {
		if got := isAitableRetryableError(tc.err); got != tc.want {
			t.Errorf("isAitableRetryableError(%v) = %v", tc.err, got)
		}
	}

	if requireViewType("Grid", "sort", []string{"Grid"}) != nil || requireViewType("Gallery", "sort", []string{"Grid"}) == nil {
		t.Fatal("view type requirement mismatch")
	}
	for viewType, want := range map[string]string{"Kanban": "kanbanCard", "Gallery": "galleryCard"} {
		got, err := dispatchCardKey(viewType)
		if err != nil || got != want {
			t.Errorf("dispatchCardKey(%q) = %q, %v", viewType, got, err)
		}
	}
	if _, err := dispatchCardKey("Grid"); err == nil {
		t.Fatal("Grid card should fail")
	}
	view := map[string]any{"custom": map[string]any{"width": 10}, "scalar": 1}
	if walkViewPath(view, "custom.width") != 10 || walkViewPath(view, "") != nil || walkViewPath(view, "scalar.child") != nil {
		t.Fatal("walkViewPath mismatch")
	}
	forms := map[string]any{"data": []any{map[string]any{"viewId": "target"}}}
	if found, ok := findFormViewByID(forms, "target"); !ok || found["viewId"] != "target" {
		t.Fatalf("find form = %#v, %v", found, ok)
	}
	if _, ok := findFormViewByID([]any{"plain"}, "missing"); ok {
		t.Fatal("missing form should not be found")
	}

	cmd := &cobra.Command{Use: "flags"}
	cmd.Flags().String("name", "", "")
	cmd.Flags().String("title", "", "")
	cmd.Flags().String("text", "", "")
	cmd.Flags().Bool("enabled", false, "")
	out := map[string]any{}
	collectStringFlag(cmd, "text", "text", out)
	collectBoolFlag(cmd, "enabled", "enabled", out)
	_ = cmd.Flags().Set("text", "value")
	_ = cmd.Flags().Set("enabled", "false")
	collectStringFlag(cmd, "text", "text", out)
	collectBoolFlag(cmd, "enabled", "enabled", out)
	if out["text"] != "value" || out["enabled"] != false {
		t.Fatalf("collected flags = %#v", out)
	}
	if resolveFormUpdateTitle(cmd) != "" {
		t.Fatal("empty form title should stay empty")
	}
	_ = cmd.Flags().Set("name", "name")
	if resolveFormUpdateTitle(cmd) != "name" {
		t.Fatal("form name fallback failed")
	}
	_ = cmd.Flags().Set("title", "title")
	if resolveFormUpdateTitle(cmd) != "title" {
		t.Fatal("form title priority failed")
	}

	if _, err := mergeUpdateBlock(`[]`, nil); err == nil {
		t.Fatal("non-object update JSON should fail")
	}
	merged, err := mergeUpdateBlock(`{"x":1,"same":"json"}`, map[string]any{"same": "typed", "y": 2})
	if err != nil || merged["same"] != "typed" || merged["y"] != 2 {
		t.Fatalf("merged update = %#v, %v", merged, err)
	}
	if merged, err := mergeUpdateBlock("", map[string]any{"x": 1}); err != nil || merged["x"] != 1 {
		t.Fatalf("typed-only update = %#v, %v", merged, err)
	}

	for _, raw := range []string{`[1]`, `{"fields":[1]}`, `{}`, `{`} {
		fields, err := parseFieldsJSON(raw)
		if (raw == `[1]` || strings.Contains(raw, "fields")) && (err != nil || len(fields) != 1) {
			t.Errorf("parseFieldsJSON(%q) = %#v, %v", raw, fields, err)
		}
		if (raw == `{}` || raw == `{`) && err == nil {
			t.Errorf("parseFieldsJSON(%q) should fail", raw)
		}
	}
}

func TestCrossPlatformCoverageAitableToolResponseAndPaginationHelpers(t *testing.T) {
	caller := &aitableTestCaller{responses: []string{`{"data":{"views":[{"viewId":"v","viewType":"Grid"}]}}`}}
	installAitableDeps(t, caller)
	view, viewType, err := getViewRaw(context.Background(), "b", "t", "v")
	if err != nil || viewType != "Grid" || view["viewId"] != "v" {
		t.Fatalf("getViewRaw success = %#v, %q, %v", view, viewType, err)
	}

	for _, response := range []string{"{", `{"data":{"views":[]}}`, `{"data":{"views":["bad"]}}`} {
		caller = &aitableTestCaller{responses: []string{response}}
		installAitableDeps(t, caller)
		if _, _, err := getViewRaw(context.Background(), "b", "t", "v"); err == nil {
			t.Errorf("getViewRaw(%q) should fail", response)
		}
	}
	caller = &aitableTestCaller{errors: []error{errors.New("offline")}}
	installAitableDeps(t, caller)
	if _, _, err := getViewRaw(context.Background(), "b", "t", "v"); err == nil {
		t.Fatal("getViewRaw transport error should fail")
	}

	caller = &aitableTestCaller{}
	out := installAitableDeps(t, caller)
	if err := printViewSubBlock(nil); err != nil || !strings.Contains(out.String(), "status") {
		t.Fatalf("print view sub-block = %q, %v", out.String(), err)
	}
	if err := callUpdateViewWithBlock("b", "t", "v", "kanbanCard", map[string]any{"x": 1}, map[string]any{"extra": true}); err != nil {
		t.Fatalf("update view block: %v", err)
	}
	if err := callUpdateViewWithBlock("b", "t", "v", "", nil, map[string]any{"newViewName": "name"}); err != nil {
		t.Fatalf("update view top-level: %v", err)
	}

	caller = &aitableTestCaller{responses: []string{`{"data":{"records":[{"id":1}],"nextCursor":"next"}}`}}
	out = installAitableDeps(t, caller)
	if err := recordQueryFetchAll(map[string]any{}, 1); err != nil || !strings.Contains(out.String(), "totalCount") {
		t.Fatalf("paginated records = %q, %v", out.String(), err)
	}
	caller = &aitableTestCaller{responses: []string{"not-json"}}
	out = installAitableDeps(t, caller)
	if err := recordQueryFetchAll(map[string]any{}, 1); err != nil || !strings.Contains(out.String(), "not-json") {
		t.Fatalf("raw first page = %q, %v", out.String(), err)
	}
	caller = &aitableTestCaller{responses: []string{`{"records":[{"id":1}]}`}}
	installAitableDeps(t, caller)
	if err := recordQueryFetchAll(map[string]any{}, 0); err != nil {
		t.Fatalf("flat records pagination: %v", err)
	}
	caller = &aitableTestCaller{errors: []error{errors.New("offline")}}
	installAitableDeps(t, caller)
	if err := recordQueryFetchAll(map[string]any{}, 1); err == nil {
		t.Fatal("first-page pagination error should fail")
	}
}
