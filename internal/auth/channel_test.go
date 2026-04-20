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
}

func TestParseChannelConfig_LegacyHostControlFallback(t *testing.T) {
	t.Setenv(DWSChannelEnv, "Qoderwork;host-control")

	cfg := CurrentChannelConfig()
	if !cfg.HostPATPassthroughEnabled() {
		t.Fatal("HostPATPassthroughEnabled() = false, want true for legacy fallback")
	}
}

func TestCurrentClawType_NormalizesAliases(t *testing.T) {
	t.Setenv(CLAWTypeEnv, "Rewind_Desktop")
	if got := CurrentClawType(); got != "rewind-desktop" {
		t.Fatalf("CurrentClawType() = %q, want rewind-desktop", got)
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

func TestHostPATPassthroughEnabled_PrefersCLAWType(t *testing.T) {
	t.Setenv(CLAWTypeEnv, CLAWTypeDWSWukong)
	cfg := ParseChannelConfig("Qoderwork")
	if !cfg.HostPATPassthroughEnabled() {
		t.Fatal("HostPATPassthroughEnabled() = false, want true for CLAW_TYPE")
	}
}

func TestIsHostPATClawType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		value string
		want  bool
	}{
		{"host-control", true},
		{"rewind-desktop", true},
		{"dws-wukong", true},
		{"wukong", true},
		{"openClaw", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsHostPATClawType(tc.value); got != tc.want {
			t.Fatalf("IsHostPATClawType(%q) = %v, want %v", tc.value, got, tc.want)
		}
	}
}
