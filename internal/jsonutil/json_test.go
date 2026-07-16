package jsonutil

import (
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
