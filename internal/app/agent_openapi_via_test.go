package app

import "testing"

func TestCrossPlatformCoverageAgentOpenAPIViaLeavesDingTalkAgentHeaderUnchanged(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	for _, raw := range []string{"", " \t ", " example-agent ", "Future-Agent"} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("DINGTALK_AGENT", raw)
			if got := resolveIdentityHeaders()["x-dingtalk-agent"]; got != raw {
				t.Fatalf("x-dingtalk-agent = %q, want original environment %q", got, raw)
			}
		})
	}
}
