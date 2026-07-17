package helpers

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageMCPInternalClassificationAndUnescapeRemainingCoverage(t *testing.T) {
	caller := &helpersCoreCaller{format: "raw"}
	out, _ := installHelpersCoreDeps(t, caller)

	caller.err = errors.New("request failed with PAT_HIGH_RISK_NO_PERMISSION")
	if err := callMCPToolInternalOpts("server", "tool", nil, false); err == nil {
		t.Fatal("PAT transport error returned nil")
	}
	caller.err = nil
	for _, response := range []string{
		`{"errorCode":"DWS_SERVICE_UNAUTHORIZED"}`,
		`{"error":"Missing service_id or access_key"}`,
		`{"code":"PAT_NO_PERMISSION"}`,
	} {
		caller.result = textToolResult(response)
		if err := callMCPToolInternalOpts("server", "tool", nil, false); err == nil {
			t.Fatalf("classified response %s returned nil", response)
		}
	}

	out.Reset()
	caller.result = textToolResult(`{"url":"https://example.test/?a=1\u0026b=2"}`)
	if err := callMCPToolInternalOpts("server", "tool", nil, true); err != nil || !strings.Contains(out.String(), "&") {
		t.Fatalf("unescaped raw output=%q err=%v", out.String(), err)
	}
}

func TestCrossPlatformCoverageCurrentUserMalformedTailFallbackCoverage(t *testing.T) {
	caller := &helpersCoreCaller{result: textToolResult(`{"result":[{"orgEmployeeModel":{"userId":"fallback-user"}},{"orgEmployeeModel":"malformed"}]}`)}
	installHelpersCoreDeps(t, caller)
	if got, err := getCurrentUserID(context.Background()); err != nil || got != "fallback-user" {
		t.Fatalf("fallback user=%q err=%v", got, err)
	}
}
