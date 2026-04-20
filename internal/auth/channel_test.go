package auth

import (
	"net/http/httptest"
	"testing"
)

func TestCurrentChannelCode_ReturnsRawValue(t *testing.T) {
	t.Setenv(DWSChannelEnv, "Qoderwork;preview")

	if got := CurrentChannelCode(); got != "Qoderwork;preview" {
		t.Fatalf("CurrentChannelCode() = %q, want raw DWS_CHANNEL value", got)
	}
}

func TestApplyChannelHeader_ForwardsRawChannelCode(t *testing.T) {
	t.Setenv(DWSChannelEnv, "Qoderwork;preview")
	req := httptest.NewRequest("GET", "https://example.com", nil)

	ApplyChannelHeader(req)

	if got := req.Header.Get("x-dws-channel"); got != "Qoderwork;preview" {
		t.Fatalf("x-dws-channel = %q, want raw DWS_CHANNEL value", got)
	}
}

func TestCurrentClawType_NormalizesAliases(t *testing.T) {
	t.Setenv(CLAWTypeEnv, "Rewind_Desktop")

	if got := CurrentClawType(); got != "rewind-desktop" {
		t.Fatalf("CurrentClawType() = %q, want rewind-desktop", got)
	}
}

func TestCurrentHostPATClawType_UsesCLAWTypeAllowlist(t *testing.T) {
	t.Setenv(CLAWTypeEnv, CLAWTypeDWSWukong)

	if got := CurrentHostPATClawType(); got != CLAWTypeDWSWukong {
		t.Fatalf("CurrentHostPATClawType() = %q, want %q", got, CLAWTypeDWSWukong)
	}
}

func TestCurrentHostPATClawType_IgnoresTaggedDWSChannel(t *testing.T) {
	t.Setenv(DWSChannelEnv, "Qoderwork;host-control")

	if got := CurrentHostPATClawType(); got != "" {
		t.Fatalf("CurrentHostPATClawType() = %q, want empty when only DWS_CHANNEL is tagged", got)
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
