package logging

import (
	"strings"
	"testing"
)

func TestCrossPlatformCoverageOutboundCallRedaction(t *testing.T) {
	args := map[string]any{"calleeInfos": []map[string]any{{"callee": "private-callee", "prompt": "private-prompt", "greeting": "private-greeting"}}, "nested": []any{map[string]any{"callee": "private-nested"}}}
	got := SanitizeArguments(args, 4096)
	if strings.Contains(got, "private-") || !strings.Contains(got, "***") {
		t.Fatalf("outbound payload leaked: %s", got)
	}
	if args["calleeInfos"].([]map[string]any)[0]["callee"] != "private-callee" {
		t.Fatal("redaction mutated invocation payload")
	}
}
