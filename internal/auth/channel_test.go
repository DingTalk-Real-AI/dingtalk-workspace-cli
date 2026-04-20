package auth

import (
	"net/http/httptest"
	"testing"
)

func TestParseChannelConfig(t *testing.T) {
	t.Parallel()

	cfg := ParseChannelConfig("Qoderwork;host-control;preview")
	if cfg.Code != "Qoderwork" {
		t.Fatalf("Code = %q, want Qoderwork", cfg.Code)
	}
	if !cfg.HasTag(DWSChannelTagHostControl) {
		t.Fatal("HasTag(host-control) = false, want true")
	}
	if !cfg.HasTag("preview") {
		t.Fatal("HasTag(preview) = false, want true")
	}
	if !cfg.HostPATPassthroughEnabled() {
		t.Fatal("HostPATPassthroughEnabled() = false, want true")
	}
}

func TestParseChannelConfig_InvalidTaggedGrammarPreservesRawValue(t *testing.T) {
	t.Parallel()

	raw := "Qoderwork;host/control"
	cfg := ParseChannelConfig(raw)
	if cfg.Code != raw {
		t.Fatalf("Code = %q, want raw value %q", cfg.Code, raw)
	}
	if len(cfg.Tags) != 0 {
		t.Fatalf("Tags = %#v, want no parsed tags", cfg.Tags)
	}
}

func TestApplyChannelHeaderUsesParsedCodeOnly(t *testing.T) {
	t.Setenv(DWSChannelEnv, "Qoderwork;host-control")
	req := httptest.NewRequest("GET", "https://example.com", nil)

	ApplyChannelHeader(req)

	if got := req.Header.Get("x-dws-channel"); got != "Qoderwork" {
		t.Fatalf("x-dws-channel = %q, want Qoderwork", got)
	}
}
