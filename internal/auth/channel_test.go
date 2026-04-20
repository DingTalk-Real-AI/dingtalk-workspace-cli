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

func TestCurrentClawType_NormalizesDINGTALKAgent(t *testing.T) {
	t.Setenv(DingTalkAgentEnv, "Sales_Copilot")

	if got := CurrentClawType(); got != "sales-copilot" {
		t.Fatalf("CurrentClawType() = %q, want sales-copilot", got)
	}
}

func TestCurrentHostPATClawType_UsesNonDefaultDINGTALKAgent(t *testing.T) {
	t.Setenv(DingTalkAgentEnv, "sales-copilot")

	if got := CurrentHostPATClawType(); got != "sales-copilot" {
		t.Fatalf("CurrentHostPATClawType() = %q, want sales-copilot", got)
	}
}

func TestCurrentHostPATClawType_IgnoresTaggedDWSChannel(t *testing.T) {
	t.Setenv(DWSChannelEnv, "Qoderwork;host-control")

	if got := CurrentHostPATClawType(); got != "" {
		t.Fatalf("CurrentHostPATClawType() = %q, want empty when only DWS_CHANNEL is tagged", got)
	}
}

func TestCurrentHostPATClawType_IgnoresDefaultDINGTALKAgent(t *testing.T) {
	t.Setenv(DingTalkAgentEnv, "default")

	if got := CurrentHostPATClawType(); got != "" {
		t.Fatalf("CurrentHostPATClawType() = %q, want empty for default", got)
	}
}

func TestIsHostPATClawType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		value string
		want  bool
	}{
		{"sales-copilot", true},
		{"customer-support", true},
		{"custom-agent", true},
		{"default", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsHostPATClawType(tc.value); got != tc.want {
			t.Fatalf("IsHostPATClawType(%q) = %v, want %v", tc.value, got, tc.want)
		}
	}
}
