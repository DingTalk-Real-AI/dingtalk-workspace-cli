package output

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

func TestWriteJSON_Map(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := WriteJSON(&buf, map[string]any{"key": "value"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), `"key"`) {
		t.Fatalf("missing key in output: %s", buf.String())
	}
}

func TestWriteJSON_DoesNotEscapeAmpersands(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	url := "https://open-dev.dingtalk.com/fe/old#/personalAuthorization?flowId=flow&userCode=CODE"
	if err := WriteJSON(&buf, map[string]any{"authorizationUrl": url}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(buf.String(), `\u0026`) {
		t.Fatalf("WriteJSON escaped ampersand: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "&userCode=CODE") {
		t.Fatalf("WriteJSON output missing literal ampersand: %s", buf.String())
	}
}

func TestWriteJSON_Nil(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := WriteJSON(&buf, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "null") {
		t.Fatalf("expected null, got: %s", buf.String())
	}
}

func TestWriteJSON_Slice(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := WriteJSON(&buf, []string{"a", "b"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), `"a"`) {
		t.Fatalf("missing element: %s", buf.String())
	}
}

func TestWriteJSON_Unmarshalable(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := WriteJSON(&buf, make(chan int))
	if err == nil {
		t.Fatal("expected error for unmarshalable type")
	}
}

func TestWrite_AllFormats(t *testing.T) {
	t.Parallel()
	payload := map[string]any{"name": "test"}

	tests := []struct {
		name   string
		format Format
	}{
		{"json", FormatJSON},
		{"raw", FormatRaw},
		{"table", FormatTable},
		{"unknown_defaults_to_json", Format("unknown")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := Write(&buf, tt.format, payload); err != nil {
				t.Fatalf("Write(%s) error: %v", tt.format, err)
			}
			if buf.Len() == 0 {
				t.Fatalf("Write(%s) produced empty output", tt.format)
			}
		})
	}
}

func TestWriteCommandPayload_NilCmd(t *testing.T) {
	t.Parallel()
	err := WriteCommandPayload(nil, map[string]any{"a": 1}, FormatJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWriteCommandPayload_WithCmd(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{Use: "test"}
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	err := WriteCommandPayload(cmd, map[string]any{"a": 1}, FormatJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), `"a"`) {
		t.Fatalf("missing key: %s", buf.String())
	}
}

func TestResolveFormat_NilCmd(t *testing.T) {
	t.Parallel()
	f := ResolveFormat(nil, FormatJSON)
	if f != FormatJSON {
		t.Fatalf("expected json, got %s", f)
	}
}

func TestResolveFormat_WithFlag(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("format", "table", "output format")
	f := ResolveFormat(cmd, FormatJSON)
	if f != FormatTable {
		t.Fatalf("expected table, got %s", f)
	}
}

func TestResolveFormat_NoFlag(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{Use: "test"}
	f := ResolveFormat(cmd, FormatRaw)
	if f != FormatRaw {
		t.Fatalf("expected raw, got %s", f)
	}
}

func TestWriteRaw_String(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := writeRaw(&buf, "hello world"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "hello world") {
		t.Fatalf("missing text: %s", buf.String())
	}
}

func TestWriteRaw_NonString(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := writeRaw(&buf, map[string]any{"x": 1}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), `"x"`) {
		t.Fatalf("missing key: %s", buf.String())
	}
}

func TestWriteRaw_DoesNotEscapeAmpersands(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := writeRaw(&buf, map[string]any{"url": "https://example.test/auth?a=1&b=2"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(buf.String(), `\u0026`) {
		t.Fatalf("writeRaw escaped ampersand: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "a=1&b=2") {
		t.Fatalf("writeRaw output missing literal ampersand: %s", buf.String())
	}
}

func TestWriteTableish_Map(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := writeTableish(&buf, map[string]any{"name": "alice", "age": 30})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "name") {
		t.Fatalf("missing key: %s", buf.String())
	}
}

func TestWriteTableish_SliceOfMaps(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	payload := []any{
		map[string]any{"name": "alice", "age": float64(30)},
		map[string]any{"name": "bob", "age": float64(25)},
	}
	if err := writeTableish(&buf, payload); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "name") || !strings.Contains(out, "alice") {
		t.Fatalf("missing table content: %s", out)
	}
}

func TestWriteTableish_SliceOfScalars(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := writeTableish(&buf, []any{"a", "b", "c"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "value") {
		t.Fatalf("missing header: %s", buf.String())
	}
}

func TestWriteTableish_EmptySlice(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := writeTableish(&buf, []any{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWriteTableish_MapWithList(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	payload := map[string]any{
		"items": []any{
			map[string]any{"id": float64(1), "name": "x"},
		},
		"total": float64(1),
	}
	if err := writeTableish(&buf, payload); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "id") || !strings.Contains(out, "total") {
		t.Fatalf("missing table/meta content: %s", out)
	}
}

func TestWriteTableish_Nil(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := writeTableish(&buf, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWriteTableish_PrimaryObject(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	payload := map[string]any{
		"data": map[string]any{"key": "val"},
	}
	if err := writeTableish(&buf, payload); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "key") {
		t.Fatalf("missing key: %s", buf.String())
	}
}

func TestUnwrapCompatRuntimePayload_NonResult(t *testing.T) {
	t.Parallel()
	val := "hello"
	if got := unwrapCompatRuntimePayload(val); got != val {
		t.Fatalf("expected passthrough, got %v", got)
	}
}

func TestUnwrapCompatRuntimePayload_NotImplemented(t *testing.T) {
	t.Parallel()
	r := executor.Result{Invocation: executor.Invocation{Implemented: false}}
	if got := unwrapCompatRuntimePayload(r); !reflect.DeepEqual(got, r) {
		t.Fatal("expected passthrough for non-implemented")
	}
}

func TestUnwrapCompatRuntimePayload_CompatInvocation(t *testing.T) {
	t.Parallel()
	content := map[string]any{"msg": "hi"}
	r := executor.Result{
		Invocation: executor.Invocation{Implemented: true, Kind: "compat_invocation"},
		Response:   map[string]any{"content": content},
	}
	got := unwrapCompatRuntimePayload(r)
	gotMap, ok := got.(map[string]any)
	if !ok || gotMap["msg"] != "hi" {
		t.Fatalf("expected unwrapped content, got %v", got)
	}
}

func TestUnwrapCompatRuntimePayload_HelperInvocation(t *testing.T) {
	t.Parallel()
	content := []any{"a"}
	r := executor.Result{
		Invocation: executor.Invocation{Implemented: true, Kind: "helper_invocation"},
		Response:   map[string]any{"content": content},
	}
	got := unwrapCompatRuntimePayload(r)
	gotSlice, ok := got.([]any)
	if !ok || len(gotSlice) != 1 {
		t.Fatalf("expected unwrapped content, got %v", got)
	}
}

func TestUnwrapCompatRuntimePayload_OtherKind(t *testing.T) {
	t.Parallel()
	r := executor.Result{
		Invocation: executor.Invocation{Implemented: true, Kind: "other"},
		Response:   map[string]any{"content": "x"},
	}
	got := unwrapCompatRuntimePayload(r)
	if _, ok := got.(executor.Result); !ok {
		t.Fatal("expected passthrough for unknown kind")
	}
}

func TestNormalizeFormat(t *testing.T) {
	t.Parallel()
	tests := []struct {
		raw      string
		fallback Format
		want     Format
	}{
		{"json", FormatTable, FormatJSON},
		{"JSON", FormatTable, FormatJSON},
		{"raw", FormatJSON, FormatRaw},
		{"table", FormatJSON, FormatTable},
		{"", FormatJSON, FormatJSON},
		{"unknown", FormatRaw, FormatRaw},
	}
	for _, tt := range tests {
		got := normalizeFormat(tt.raw, tt.fallback)
		if got != tt.want {
			t.Errorf("normalizeFormat(%q, %s) = %s, want %s", tt.raw, tt.fallback, got, tt.want)
		}
	}
}

func TestFormatValue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input any
		want  string
	}{
		{nil, ""},
		{"hello", "hello"},
		{float64(42), "42"},
		{true, "true"},
		{map[string]any{"k": "v"}, `{"k":"v"}`},
	}
	for _, tt := range tests {
		got := formatValue(tt.input)
		if got != tt.want {
			t.Errorf("formatValue(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	t.Parallel()
	if got := truncate("short", 10); got != "short" {
		t.Errorf("expected short, got %s", got)
	}
	if got := truncate("a very long string indeed", 10); len([]rune(got)) > 10 {
		t.Errorf("expected truncated, got %s (len %d)", got, len([]rune(got)))
	}
}

func TestRuneWidth(t *testing.T) {
	t.Parallel()
	if got := runeWidth("abc"); got != 3 {
		t.Errorf("expected 3, got %d", got)
	}
	// CJK character should count as 2
	if got := runeWidth("中"); got != 2 {
		t.Errorf("expected 2, got %d", got)
	}
	if got := runeWidth("a中b"); got != 4 {
		t.Errorf("expected 4, got %d", got)
	}
}

func TestSortedKeys(t *testing.T) {
	t.Parallel()
	keys := map[string]struct{}{"c": {}, "a": {}, "b": {}}
	got := sortedKeys(keys)
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("expected [a b c], got %v", got)
	}
}
