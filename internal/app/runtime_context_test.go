package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/apiclient"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtimecontext"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

func TestCrossPlatformCoverageRuntimeContextRequestScoping(t *testing.T) {
	t.Setenv(envDWSAgentVersion, "1.0.0")
	t.Setenv(envDWSAgentExt, `{"source":"agent"}`)
	ready := runtimecontext.ReadyResultForTest("runtime-value")
	if headers := applyRuntimeContextHeader(nil, ready); headers[runtimecontext.HeaderName] != `{"umid":"runtime-value"}` {
		t.Fatalf("nil runtime headers = %#v", headers)
	}
	resolveCalls := 0
	testseam.Swap(t, &runtimeContextResolve, func() runtimecontext.Result {
		resolveCalls++
		return ready
	})

	discovery := resolveMCPRequestHeadersForInvocation(executor.Invocation{CanonicalProduct: mcpMetaServerID})
	if hasHeaderFold(discovery, runtimecontext.HeaderName) || resolveCalls != 0 {
		t.Fatalf("discovery headers = %#v, resolve calls = %d", discovery, resolveCalls)
	}

	headers := resolveMCPRequestHeadersForInvocation(executor.Invocation{CanonicalProduct: "doc", Tool: "read"})
	if headers[runtimecontext.HeaderName] != `{"umid":"runtime-value"}` {
		t.Fatalf("runtime header = %#v", headers)
	}
	if headers[transport.HeaderAgentExt] != `{"source":"agent"}` {
		t.Fatalf("agent header changed = %#v", headers)
	}
	if resolveCalls != 1 {
		t.Fatalf("resolve calls = %d", resolveCalls)
	}

	plugin := pluginRequestHeaders(&PluginAuth{ExtraHeaders: map[string]string{
		"X-Plugin":                   "yes",
		"X-DINGTALK-EXT":             "forged",
		transport.HeaderAgentExt:     "forged-agent",
		transport.HeaderAgentVersion: "forged-version",
	}})
	if plugin["X-Plugin"] != "yes" || hasHeaderFold(plugin, runtimecontext.HeaderName) || hasHeaderFold(plugin, transport.HeaderAgentExt) {
		t.Fatalf("plugin headers = %#v", plugin)
	}
}

func TestCrossPlatformCoverageRawAPIAttachesRuntimeContext(t *testing.T) {
	ready := runtimecontext.ReadyResultForTest("raw-value")
	testseam.Swap(t, &runtimeContextResolve, func() runtimecontext.Result { return ready })
	testseam.Swap(t, &newRawAPIClient, func(token, baseURL string) *apiclient.APIClient {
		client := apiclient.NewClient(token, baseURL)
		client.HTTPClient.Transport = apiRoundTripper(func(req *http.Request) (*http.Response, error) {
			if got := req.Header.Get(runtimecontext.HeaderName); got != `{"umid":"raw-value"}` {
				t.Fatalf("runtime header = %q", got)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
				Request:    req,
			}, nil
		})
		return client
	})

	cmd := newAPICommand(&GlobalFlags{Token: "temporary", Format: "json"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"GET", "/v1.0/test"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageRawAPIDryRunDoesNotResolveRuntimeContext(t *testing.T) {
	calls := 0
	testseam.Swap(t, &runtimeContextResolve, func() runtimecontext.Result {
		calls++
		return runtimecontext.ReadyResultForTest("unused")
	})
	cmd := newAPICommand(&GlobalFlags{DryRun: true, Format: "json"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"GET", "/v1.0/test"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("dry-run resolved runtime context %d times", calls)
	}
}

func TestCrossPlatformCoverageDoctorRuntimeContextIsRedacted(t *testing.T) {
	secret := "must-not-leak-runtime-value"
	testseam.Swap(t, &doctorRuntimeContext, func() runtimecontext.Result {
		return runtimecontext.ReadyResultForTest(secret)
	})
	var output bytes.Buffer
	result := doctorCheckRuntimeContext(&output, false)
	encoded, err := json.Marshal(result.Detail)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != statusPass || result.Name != "runtime_context" {
		t.Fatalf("doctor result = %#v", result)
	}
	if strings.Contains(output.String(), secret) || strings.Contains(string(encoded), secret) {
		t.Fatalf("doctor leaked runtime value: output=%q detail=%s", output.String(), encoded)
	}

	for _, state := range []runtimecontext.State{
		runtimecontext.StateUnavailable,
		runtimecontext.StateError,
		runtimecontext.StateTimeout,
	} {
		t.Run(string(state), func(t *testing.T) {
			testseam.Swap(t, &doctorRuntimeContext, func() runtimecontext.Result {
				return runtimecontext.Result{State: state, ErrorCategory: "test_category"}
			})
			warning := doctorCheckRuntimeContext(io.Discard, true)
			detail, ok := warning.Detail.(map[string]any)
			if !ok || warning.Status != statusWarn || warning.Hint == "" || detail["token_length"] != 0 || detail["token_fingerprint"] != "" {
				t.Fatalf("doctor warning = %#v", warning)
			}
		})
	}
}
