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

// TestHostOwnsPATFlow_OnlyAgentCodeMatters is the package-local mirror of
// test/unit/pat_host_owned_signal_test.go. It locks in the invariant that
// HostOwnsPATFlow consults ONLY DINGTALK_DWS_AGENTCODE — DINGTALK_AGENT
// and DWS_CHANNEL must never sway the decision, in either direction.
func TestHostOwnsPATFlow_OnlyAgentCodeMatters(t *testing.T) {
	cases := []struct {
		name      string
		agentCode string
		setAgent  bool
		agentEnv  string
		want      bool
	}{
		{"no signal → CLI-owned", "", false, "", false},
		{"agent code only → host-owned", "agt-cursor", false, "", true},
		{"agent code + DINGTALK_AGENT business → host-owned", "agt-cursor", true, "sales-copilot", true},
		{"DINGTALK_AGENT alone → CLI-owned", "", true, "sales-copilot", false},
		{"whitespace-only agent code → CLI-owned", "   ", true, "sales-copilot", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(AgentCodeEnv, tc.agentCode)
			if tc.setAgent {
				t.Setenv("DINGTALK_AGENT", tc.agentEnv)
			}
			if got := HostOwnsPATFlow(); got != tc.want {
				t.Fatalf("HostOwnsPATFlow() = %v, want %v (agentCode=%q, DINGTALK_AGENT set=%v value=%q)",
					got, tc.want, tc.agentCode, tc.setAgent, tc.agentEnv)
			}
		})
	}
}
