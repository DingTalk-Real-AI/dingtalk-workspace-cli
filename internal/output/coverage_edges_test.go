package output

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

var errOutputInjected = errors.New("injected output failure")

type failNthWriter struct {
	nth   int
	calls int
}

func (w *failNthWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.calls >= w.nth {
		return 0, errOutputInjected
	}
	return len(p), nil
}

func TestCrossPlatformCoverageOutputJSONFailureInjection(t *testing.T) {
	origMarshal := marshalJSON
	origUnmarshal := unmarshalJSON
	origOutput := marshalJSONOutput
	origIndent := marshalJSONIndent
	t.Cleanup(func() {
		marshalJSON = origMarshal
		unmarshalJSON = origUnmarshal
		marshalJSONOutput = origOutput
		marshalJSONIndent = origIndent
	})

	marshalJSON = func(any) ([]byte, error) { return nil, errOutputInjected }
	if _, err := normalizePayload(struct{}{}); err == nil {
		t.Fatal("normalizePayload should report marshal failure")
	}
	for name, call := range map[string]func() error{
		"csv":    func() error { return writeCSV(io.Discard, struct{}{}) },
		"ndjson": func() error { return writeNDJSON(io.Discard, struct{}{}) },
		"pretty": func() error { return writePretty(io.Discard, struct{}{}) },
		"table":  func() error { return writeTableish(io.Discard, struct{}{}) },
	} {
		if err := call(); err == nil {
			t.Fatalf("%s should report normalization failure", name)
		}
	}
	if _, err := roundTripJSON(struct{}{}); err == nil {
		t.Fatal("roundTripJSON should report marshal failure")
	}
	if got := toGeneric(make(chan int)); got == nil {
		t.Fatal("toGeneric should preserve an unmarshalable value")
	}
	if got := toGeneric(nil); got != nil {
		t.Fatalf("toGeneric(nil) = %v", got)
	}

	marshalJSON = func(any) ([]byte, error) { return []byte("{"), nil }
	if _, err := normalizePayload(struct{}{}); err == nil {
		t.Fatal("normalizePayload should report unmarshal failure")
	}
	if _, err := roundTripJSON(struct{}{}); err == nil {
		t.Fatal("roundTripJSON should report unmarshal failure")
	}
	if got := toGeneric("original"); got != "original" {
		t.Fatalf("toGeneric fallback = %v", got)
	}

	marshalJSON = origMarshal
	marshalJSONIndent = func(any, string, string) ([]byte, error) { return nil, errOutputInjected }
	if err := WriteJSON(io.Discard, map[string]any{}); err == nil {
		t.Fatal("WriteJSON should report marshal failure")
	}
	if err := ApplyJQ(io.Discard, map[string]any{"x": 1}, "."); err == nil {
		t.Fatal("ApplyJQ should report marshal failure")
	}

	marshalJSONIndent = origIndent
	marshalJSONOutput = func(any) ([]byte, error) { return nil, errOutputInjected }
	if err := writeRaw(io.Discard, map[string]any{}); err == nil {
		t.Fatal("writeRaw should report marshal failure")
	}
	if got := formatValue(make(chan int)); got == "" {
		t.Fatalf("formatValue fallback = %q", got)
	}
}

func TestCrossPlatformCoverageOutputFlagEdges(t *testing.T) {
	if _, ok := formatFromFlagSet(nil, FormatRaw); ok {
		t.Fatal("nil flag set should not resolve")
	}
	cmd := &cobra.Command{Use: "cmd"}
	if _, ok := formatFromFlagSet(cmd.Flags(), FormatRaw); ok {
		t.Fatal("missing flag should not resolve")
	}
	cmd.Flags().Int("format", 1, "wrong type")
	if _, ok := formatFromFlagSet(cmd.Flags(), FormatRaw); ok {
		t.Fatal("non-string format should not resolve")
	}

	for _, raw := range []string{"", "json", "raw", "table", "pretty", "ndjson", "csv", "unknown"} {
		_ = normalizeFormat(raw, FormatRaw)
	}

	if rootPersistentFlags(nil) != nil {
		t.Fatal("nil command should not have root flags")
	}
	if ResolveFields(nil) != "" || ResolveJQ(nil) != "" {
		t.Fatal("nil command filters should be empty")
	}

	root := &cobra.Command{Use: "root"}
	child := &cobra.Command{Use: "child"}
	root.AddCommand(child)
	if ResolveFields(child) != "" || ResolveJQ(child) != "" {
		t.Fatal("missing global filters should be empty")
	}

	root.PersistentFlags().String("fields", "", "global fields")
	root.PersistentFlags().String("jq", "", "global jq")
	if err := root.PersistentFlags().Set("fields", "id,name"); err != nil {
		t.Fatal(err)
	}
	if err := root.PersistentFlags().Set("jq", ".id"); err != nil {
		t.Fatal(err)
	}
	if got := ResolveFields(child); got != "id,name" {
		t.Fatalf("ResolveFields = %q", got)
	}
	if got := ResolveJQ(child); got != ".id" {
		t.Fatalf("ResolveJQ = %q", got)
	}

	wrong := &cobra.Command{Use: "wrong"}
	wrong.PersistentFlags().Int("fields", 1, "global fields")
	wrong.PersistentFlags().Int("jq", 1, "global jq")
	_ = wrong.PersistentFlags().Set("fields", "2")
	_ = wrong.PersistentFlags().Set("jq", "2")
	if ResolveFields(wrong) != "" || ResolveJQ(wrong) != "" {
		t.Fatal("wrongly typed filters should be ignored")
	}
}

func TestCrossPlatformCoverageOutputWriterFailures(t *testing.T) {
	filtered := filterSlice([]any{"scalar", map[string]any{"keep": 1}}, map[string]bool{"keep": true})
	if len(filtered) != 2 || filtered[0] != "scalar" {
		t.Fatalf("filterSlice = %#v", filtered)
	}
	payload := map[string]any{
		"items": []any{
			map[string]any{"a": "one", "b": strings.Repeat("x", 80)},
			map[string]any{"a": "two", "b": "three"},
		},
		"total": 2,
	}
	for nth := 1; nth <= 80; nth++ {
		_ = writeTableish(&failNthWriter{nth: nth}, payload)
		_ = writeTable(&failNthWriter{nth: nth}, []string{"a", "b"}, [][]string{{"1", "2"}, {"3", "4"}})
		_ = writeKeyValues(&failNthWriter{nth: nth}, map[string]any{strings.Repeat("k", 30): "v", "z": 2})
	}
	if err := writeTable(io.Discard, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := writeTable(io.Discard, []string{"a"}, [][]string{{"1", "ignored"}}); err != nil {
		t.Fatal(err)
	}

	for _, call := range []func(io.Writer) error{
		func(w io.Writer) error { return WriteJSON(w, map[string]any{"x": 1}) },
		func(w io.Writer) error { return writeRaw(w, "text") },
		func(w io.Writer) error { return writeRaw(w, map[string]any{"x": 1}) },
		func(w io.Writer) error { return writeTableish(w, payload) },
		func(w io.Writer) error { return ApplyJQ(w, []any{1, 2}, ".[]") },
	} {
		for nth := 1; nth <= 8; nth++ {
			_ = call(&failNthWriter{nth: nth})
		}
	}
	if err := ApplyJQ(io.Discard, 1, ".foo"); err == nil {
		t.Fatal("jq evaluation error expected")
	}
	if err := ApplyJQ(io.Discard, nil, "["); err == nil {
		t.Fatal("jq parse error expected")
	}
}

func TestCrossPlatformCoverageCSVFailureAndShapeEdges(t *testing.T) {
	origWrite := writeCSVRecord
	t.Cleanup(func() { writeCSVRecord = origWrite })

	for failAt := 1; failAt <= 3; failAt++ {
		calls := 0
		writeCSVRecord = func(cw *csv.Writer, row []string) error {
			calls++
			if calls == failAt {
				return errOutputInjected
			}
			return origWrite(cw, row)
		}
		_ = writeTableCSV(csv.NewWriter(io.Discard), []string{"h"}, [][]string{{"a"}, {"b"}})
		calls = 0
		_ = writeKeyValueCSV(csv.NewWriter(io.Discard), map[string]any{"a": 1, "b": 2})
		calls = 0
		_ = writeCSV(io.Discard, "scalar")
	}
	writeCSVRecord = origWrite

	_ = writeCSV(&failNthWriter{nth: 1}, nil)
	if err := writeCSV(&failNthWriter{nth: 1}, "scalar"); err == nil {
		t.Fatal("scalar CSV flush failure expected")
	}
	if err := writeTableCSV(csv.NewWriter(&failNthWriter{nth: 1}), []string{"h"}, [][]string{{"v"}}); err == nil {
		t.Fatal("table CSV flush failure expected")
	}
	if err := writeKeyValueCSV(csv.NewWriter(&failNthWriter{nth: 1}), map[string]any{"a": 1}); err == nil {
		t.Fatal("key/value CSV flush failure expected")
	}

	headers, rows := broadcastMeta([]string{"id"}, [][]string{{"1"}}, nil)
	if len(headers) != 1 || len(rows) != 1 {
		t.Fatal("empty metadata changed rows")
	}
	headers, rows = broadcastMeta([]string{"id"}, [][]string{{"1"}}, map[string]any{"id": "collision"})
	if len(headers) != 1 || len(rows) != 1 {
		t.Fatal("colliding metadata should be ignored")
	}
	_, rows = broadcastMeta([]string{"id"}, nil, map[string]any{"total": 0})
	if len(rows) != 1 {
		t.Fatal("empty rows should retain metadata")
	}
	if err := writeCSV(io.Discard, map[string]any{"data": map[string]any{"x": 1}}); err != nil {
		t.Fatal(err)
	}
	if err := writeTableCSV(csv.NewWriter(io.Discard), []string{"a", "b"}, [][]string{{"one"}}); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageNDJSONFailureEdges(t *testing.T) {
	origEncode := encodeNDJSON
	t.Cleanup(func() { encodeNDJSON = origEncode })
	encodeNDJSON = func(*json.Encoder, any) error { return errOutputInjected }
	for _, payload := range []any{
		[]any{1},
		map[string]any{"items": []any{map[string]any{"x": 1}}},
		map[string]any{"x": 1},
		"scalar",
	} {
		if err := writeNDJSON(io.Discard, payload); err == nil {
			t.Fatalf("encode failure expected for %T", payload)
		}
	}
	encodeNDJSON = origEncode
	if err := writeNDJSON(&failNthWriter{nth: 1}, map[string]any{"x": 1}); err == nil {
		t.Fatal("flush failure expected")
	}
}

func TestCrossPlatformCoveragePrettyAndFormattingEdges(t *testing.T) {
	forceNoColor(t)
	for _, payload := range []map[string]any{
		{"kind": "schema", "degraded": true, "reason": "offline", "hint": "retry"},
		{"kind": "schema", "degraded": true, "reason": "offline"},
		{"kind": "schema", "other": true},
		{"kind": "schema", "products": []any{"invalid", map[string]any{
			"id": "p", "name": "same", "description": "same", "tools": []any{
				"invalid",
				map[string]any{"name": "same", "cli_name": "same"},
				map[string]any{"name": "rpc", "cli_name": "cli"},
				map[string]any{"name": "3"}, map[string]any{"name": "4"}, map[string]any{"name": "5"},
				map[string]any{"name": "6"}, map[string]any{"name": "7"}, map[string]any{"name": "8"},
			}},
		}},
	} {
		if err := writePretty(io.Discard, payload); err != nil {
			t.Fatal(err)
		}
	}

	toolPayload := map[string]any{
		"kind":    "schema",
		"product": "invalid",
		"tool": map[string]any{
			"name":        "rpc",
			"cli_name":    "rpc",
			"title":       "rpc",
			"description": "rpc",
			"parameters": map[string]any{
				"any":      "invalid",
				"array":    map[string]any{"type": "array", "items": map[string]any{}},
				"enum":     map[string]any{"enum": []any{"a", 2}, "description": "line1\nline2"},
				"required": map[string]any{"type": "string"},
			},
			"required": []any{"required", 3},
			"flag_overlay": map[string]any{
				"any": "invalid",
				"enum": map[string]any{
					"alias":          "enum-alias",
					"transform":      "map",
					"transform_args": map[string]any{"z": 2},
					"env_default":    "ENUM_ENV",
					"default":        "a",
				},
			},
			"output_schema": map[string]any{"type": "object"},
		},
	}
	if err := writePretty(io.Discard, toolPayload); err != nil {
		t.Fatal(err)
	}

	for _, value := range []any{nil, "text", map[string]any{"x": 1}} {
		_ = formatValue(value)
	}
	for _, tc := range []struct {
		s string
		w int
	}{{"abc", 0}, {"abc", 1}, {"abcdef", 3}, {"你好世界", 4}} {
		_ = truncate(tc.s, tc.w)
	}
	for _, prop := range []map[string]any{
		{"type": "array", "items": map[string]any{"type": "string"}},
		{"type": "array"},
		{},
		{"type": "number"},
	} {
		_ = describeType(prop)
	}
	if got, err := normalizePayload("text"); err != nil || got != "text" {
		t.Fatalf("normalize string = %v, %v", got, err)
	}
	if got, err := normalizePayload(nil); err != nil || got != nil {
		t.Fatalf("normalize nil = %v, %v", got, err)
	}
}

func TestCrossPlatformCoverageExtractRowsNestedMetadataCollision(t *testing.T) {
	payload := map[string]any{
		"result": map[string]any{
			"items": []any{map[string]any{"id": 1}},
			"same":  "inner",
			"inner": true,
		},
		"same":  "outer",
		"outer": true,
	}
	_, _, meta, ok := extractRowsFromMap(payload)
	if !ok || meta["same"] != "outer" || meta["inner"] != true || meta["outer"] != true {
		t.Fatalf("unexpected metadata: %#v", meta)
	}
	if _, _, _, ok := extractRowsFromMap(map[string]any{"x": 1}); ok {
		t.Fatal("non-list map should not extract rows")
	}

	var buf bytes.Buffer
	if err := Write(&buf, FormatPretty, map[string]any{"x": 1}); err != nil {
		t.Fatal(err)
	}
}
