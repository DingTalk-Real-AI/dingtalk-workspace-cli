package jsonutil

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageMarshalPreservesHTMLCharacters(t *testing.T) {
	got, err := Marshal(map[string]string{"url": "https://example.test/?a=1&b=<tag>"})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if strings.Contains(string(got), `\u0026`) || !strings.Contains(string(got), `&b=<tag>`) {
		t.Fatalf("Marshal() = %s", got)
	}

	got, err = MarshalIndent(map[string]any{"nested": map[string]string{"value": "a&b"}}, "prefix:", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	if !strings.Contains(string(got), "\n") || strings.Contains(string(got), `\u0026`) {
		t.Fatalf("MarshalIndent() = %s", got)
	}
}

func TestCrossPlatformCoverageMarshalReturnsEncoderErrors(t *testing.T) {
	unsupported := make(chan int)
	if _, err := Marshal(unsupported); err == nil {
		t.Fatal("Marshal(channel) error = nil")
	}
	if _, err := MarshalIndent(unsupported, "", "  "); err == nil {
		t.Fatal("MarshalIndent(channel) error = nil")
	}
}

func TestCrossPlatformCoverageRejectDuplicateObjectKeys(t *testing.T) {
	for _, input := range []string{
		`{"outer":{"value":1,"value":2}}`,
		`[{"value":1},{"value":2,"value":3}]`,
	} {
		if err := RejectDuplicateObjectKeys([]byte(input)); err == nil || !strings.Contains(err.Error(), "duplicate JSON object key") {
			t.Fatalf("RejectDuplicateObjectKeys(%s) error = %v", input, err)
		}
	}
	if err := RejectDuplicateObjectKeys([]byte(`{"outer":{"value":1},"items":[1,2]}`)); err != nil {
		t.Fatalf("RejectDuplicateObjectKeys(valid) error = %v", err)
	}
}

func TestCrossPlatformCoverageRejectDuplicateObjectKeysMalformed(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: ``, want: "EOF"},
		{name: "trailing value", input: `1 2`, want: "unexpected trailing JSON value"},
		{name: "invalid trailing token", input: `1 x`, want: "invalid character"},
		{name: "truncated key", input: `{"`, want: "unexpected EOF"},
		{name: "truncated object", input: `{"x":1`, want: "EOF"},
		{name: "truncated array", input: `[1`, want: "EOF"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := RejectDuplicateObjectKeys([]byte(test.input))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("RejectDuplicateObjectKeys(%q) error = %v, want %q", test.input, err, test.want)
			}
		})
	}
}

func TestCrossPlatformCoverageDuplicateScannerBudgets(t *testing.T) {
	tests := []struct {
		input string
		depth int
		state duplicateScanState
		want  string
	}{
		{input: `1`, state: duplicateScanState{tokens: maxDuplicateScanTokens}, want: "token count"},
		{input: `{"x":1}`, state: duplicateScanState{tokens: maxDuplicateScanTokens - 1}, want: "token count"},
		{input: `{"x":1}`, state: duplicateScanState{keyBytes: maxDuplicateScanKeyBytes}, want: "object key data"},
		{input: `"x"`, state: duplicateScanState{stringBytes: maxDuplicateScanStringBytes}, want: "string data"},
		{input: `null`, depth: 257, want: "nesting exceeds limit"},
	}
	for _, test := range tests {
		decoder := json.NewDecoder(bytes.NewBufferString(test.input))
		if err := rejectDuplicateValue(decoder, test.depth, &test.state); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("rejectDuplicateValue(%s) error = %v, want %q", test.input, err, test.want)
		}
	}
}
