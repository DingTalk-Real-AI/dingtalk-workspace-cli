package app

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/safety"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type runnerCoverageFallback struct {
	result executor.Result
	err    error
}

func (f runnerCoverageFallback) Run(context.Context, executor.Invocation) (executor.Result, error) {
	return f.result, f.err
}

func TestCrossPlatformCoverageRunnerRemainingRoutingCoverage(t *testing.T) {
	oldResolveMulti := runnerResolveMultiProfileSelections
	oldResolveProfile := runnerResolveProfile
	oldCachedToken := runnerGetCachedRuntimeToken
	t.Cleanup(func() {
		runnerResolveMultiProfileSelections = oldResolveMulti
		runnerResolveProfile = oldResolveProfile
		runnerGetCachedRuntimeToken = oldCachedToken
	})

	created := newCommandRunnerWithFlags(cli.StaticLoader{}, &GlobalFlags{Timeout: 2})
	if created.(*runtimeRunner).transport == nil {
		t.Fatal("runner transport was not created")
	}

	wantErr := errors.New("profiles failed")
	runnerResolveMultiProfileSelections = func(string, string) ([]multiProfileSelection, bool, error) {
		return nil, false, wantErr
	}
	if _, err := (&runtimeRunner{}).Run(context.Background(), executor.Invocation{}); err == nil {
		t.Fatal("profile resolution error was accepted")
	}

	inv := executor.Invocation{CanonicalProduct: "product", Tool: "tool"}
	prefetched := make(chan struct{}, 1)
	runnerGetCachedRuntimeToken = func(context.Context) (string, error) {
		prefetched <- struct{}{}
		return "", nil
	}
	r := &runtimeRunner{
		loader:    cli.CatalogLoaderFrom(cli.Catalog{}, wantErr),
		transport: transport.NewClient(nil),
		fallback:  runnerCoverageFallback{},
	}
	directMiss := inv
	directMiss.Kind = "helper_invocation"
	if _, err := r.runSingle(context.Background(), directMiss, false); !errors.Is(err, wantErr) {
		t.Fatalf("direct runtime miss load error = %v", err)
	}
	directHit := executor.Invocation{Kind: "helper_invocation", CanonicalProduct: defaultPATProductID, Tool: "pat", DryRun: true}
	if got, err := r.runSingle(context.Background(), directHit, false); err != nil || got.Response["dry_run"] != true {
		t.Fatalf("direct runtime hit = %#v, %v", got, err)
	}
	if _, err := r.runSingle(context.Background(), inv, true); !errors.Is(err, wantErr) {
		t.Fatalf("runSingle error = %v", err)
	}
	<-prefetched

	runnerResolveProfile = func(_ string, selector string) (*authpkg.Profile, error) {
		if selector == "a,b" {
			return nil, errors.New("not a combined profile")
		}
		if selector == "a" {
			return nil, wantErr
		}
		return nil, nil
	}
	if _, _, err := resolveMultiProfileSelections("", "a,b"); !errors.Is(err, wantErr) {
		t.Fatalf("profile error = %v", err)
	}
	runnerResolveProfile = func(_ string, selector string) (*authpkg.Profile, error) {
		if selector == "a,b" {
			return nil, errors.New("not a combined profile")
		}
		if selector == "a" {
			return &authpkg.Profile{CorpID: "corp-a"}, nil
		}
		return nil, nil
	}
	if _, _, err := resolveMultiProfileSelections("", "a,b"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("nil profile error = %v", err)
	}

	selections := []multiProfileSelection{
		{Selector: "bad", Profile: authpkg.Profile{CorpID: "bad"}},
		{Selector: "good", Profile: authpkg.Profile{CorpID: "good"}},
	}
	r = &runtimeRunner{fallback: runnerCoverageFallback{err: wantErr}}
	result, err := r.runMultiProfile(context.Background(), inv, selections[:1])
	if err != nil || result.Response == nil {
		t.Fatalf("multi failure aggregation = %#v, %v", result, err)
	}
	r.fallback = runnerCoverageFallback{result: executor.Result{Response: map[string]any{"endpoint": "local", "value": 1}}}
	result, err = r.runMultiProfile(context.Background(), inv, selections[1:])
	if err != nil || result.Response == nil {
		t.Fatalf("multi success aggregation = %#v, %v", result, err)
	}

	product := cli.CanonicalProduct{ID: "product", Endpoint: "https://catalog.test", Tools: []cli.ToolDescriptor{{RPCName: "tool"}}}
	r = &runtimeRunner{
		loader:      cli.StaticLoader{Catalog: cli.Catalog{Products: []cli.CanonicalProduct{product}}},
		transport:   transport.NewClient(nil),
		globalFlags: &GlobalFlags{DryRun: true},
		fallback:    runnerCoverageFallback{},
	}
	t.Setenv("DINGTALK_PRODUCT_MCP_URL", "https://override.test")
	got, err := r.runSingle(context.Background(), inv, false)
	if err != nil || got.Response["endpoint"] != "https://override.test" {
		t.Fatalf("catalog override = %#v, %v", got, err)
	}
}

func TestCrossPlatformCoverageRunnerRemainingExecutionCoverage(t *testing.T) {
	oldEdition := edition.Get()
	oldPreflight := runnerPreflightDocDownload
	oldCall := runnerCallTool
	oldHandle := runnerHandlePatAuthCheck
	oldRetry := runnerRetryWithPatAuthRetry
	oldCapture := runnerCaptureRuntimeFailure
	t.Cleanup(func() {
		edition.Override(oldEdition)
		runnerPreflightDocDownload = oldPreflight
		runnerCallTool = oldCall
		runnerHandlePatAuthCheck = oldHandle
		runnerRetryWithPatAuthRetry = oldRetry
		runnerCaptureRuntimeFailure = oldCapture
	})

	pluginAuthMu.Lock()
	oldPluginRegistry := pluginAuthRegistry
	pluginAuthRegistry = make(map[string]*PluginAuth)
	pluginAuthMu.Unlock()
	t.Cleanup(func() {
		pluginAuthMu.Lock()
		pluginAuthRegistry = oldPluginRegistry
		pluginAuthMu.Unlock()
	})

	runnerCaptureRuntimeFailure = func(executor.Invocation, error, error) {}
	runnerPreflightDocDownload = func(*runtimeRunner, context.Context, *transport.Client, string, executor.Invocation) error {
		return nil
	}
	runnerCallTool = func(*transport.Client, context.Context, string, string, map[string]any) (transport.ToolCallResult, error) {
		return transport.ToolCallResult{Content: map[string]any{"value": 1}}, nil
	}
	r := &runtimeRunner{
		transport:   transport.NewClient(nil),
		globalFlags: &GlobalFlags{Token: "token", Timeout: 1},
	}
	inv := executor.Invocation{CanonicalProduct: "product", Tool: "tool", Params: map[string]any{"x": 1}}

	if got, err := r.executeInvocation(context.Background(), "https://example.test", inv); err != nil || !got.Invocation.Implemented {
		t.Fatalf("default auth execution = %#v, %v", got, err)
	}

	wantErr := errors.New("preflight")
	runnerPreflightDocDownload = func(*runtimeRunner, context.Context, *transport.Client, string, executor.Invocation) error {
		return wantErr
	}
	if _, err := r.executeInvocation(context.Background(), "https://example.test", inv); !errors.Is(err, wantErr) {
		t.Fatalf("preflight error = %v", err)
	}
	patErr := &apperrors.PATError{RawJSON: `{"code":"PAT_NO_PERMISSION"}`}
	runnerPreflightDocDownload = func(*runtimeRunner, context.Context, *transport.Client, string, executor.Invocation) error {
		return patErr
	}
	retrying := context.WithValue(context.Background(), patRetryingKey, true)
	if _, err := r.executeInvocation(retrying, "https://example.test", inv); !errors.Is(err, patErr) {
		t.Fatalf("retrying preflight PAT = %v", err)
	}
	handled := errors.New("handled PAT")
	runnerHandlePatAuthCheck = func(context.Context, *runtimeRunner, executor.Invocation, *apperrors.PATError, string, io.Writer) (executor.Result, error) {
		return executor.Result{}, handled
	}
	if _, err := r.executeInvocation(context.Background(), "https://example.test", inv); !errors.Is(err, handled) {
		t.Fatalf("handled preflight PAT = %v", err)
	}
	runnerPreflightDocDownload = oldPreflight
	runnerPreflightDocDownload = func(*runtimeRunner, context.Context, *transport.Client, string, executor.Invocation) error {
		return nil
	}

	authErr := apperrors.NewAuth("expired", apperrors.WithReason("http_401"))
	runnerCallTool = func(*transport.Client, context.Context, string, string, map[string]any) (transport.ToolCallResult, error) {
		return transport.ToolCallResult{}, authErr
	}
	overrideErr := errors.New("auth override")
	edition.Override(&edition.Hooks{OnAuthError: func(string, error) error { return overrideErr }})
	if _, err := r.executeInvocation(context.Background(), "https://example.test", inv); !errors.Is(err, overrideErr) {
		t.Fatalf("auth override = %v", err)
	}
	edition.Override(&edition.Hooks{OnAuthError: func(string, error) error { return nil }})
	if _, err := r.executeInvocation(context.Background(), "https://example.test", inv); !errors.Is(err, authErr) {
		t.Fatalf("auth passthrough = %v", err)
	}

	scopeErr := errors.New("missing_scope calendar:read")
	runnerCallTool = func(*transport.Client, context.Context, string, string, map[string]any) (transport.ToolCallResult, error) {
		return transport.ToolCallResult{}, scopeErr
	}
	retried := errors.New("scope retried")
	runnerRetryWithPatAuthRetry = func(context.Context, executor.Runner, executor.Invocation, *PatScopeError, string, io.Writer) (executor.Result, error) {
		return executor.Result{}, retried
	}
	if _, err := r.executeInvocation(context.Background(), "https://example.test", inv); !errors.Is(err, retried) {
		t.Fatalf("scope retry = %v", err)
	}

	runnerCallTool = func(*transport.Client, context.Context, string, string, map[string]any) (transport.ToolCallResult, error) {
		return transport.ToolCallResult{}, wantErr
	}
	edition.Override(&edition.Hooks{})
	if _, err := r.executeInvocation(context.Background(), "https://example.test", inv); !errors.Is(err, wantErr) {
		t.Fatalf("generic call error = %v", err)
	}

	patContent := map[string]any{"code": "PAT_NO_PERMISSION", "data": map[string]any{"flowId": "f"}}
	runnerCallTool = func(*transport.Client, context.Context, string, string, map[string]any) (transport.ToolCallResult, error) {
		return transport.ToolCallResult{Content: patContent}, nil
	}
	edition.Override(&edition.Hooks{ClassifyToolResult: func(map[string]any) error { return patErr }})
	if _, err := r.executeInvocation(retrying, "https://example.test", inv); !errors.Is(err, patErr) {
		t.Fatalf("edition retry PAT = %v", err)
	}
	if _, err := r.executeInvocation(context.Background(), "https://example.test", inv); !errors.Is(err, handled) {
		t.Fatalf("edition handled PAT = %v", err)
	}
	edition.Override(&edition.Hooks{ClassifyToolResult: func(map[string]any) error { return wantErr }})
	if _, err := r.executeInvocation(context.Background(), "https://example.test", inv); !errors.Is(err, wantErr) {
		t.Fatalf("edition classification = %v", err)
	}

	edition.Override(&edition.Hooks{})
	if _, err := r.executeInvocation(retrying, "https://example.test", inv); err == nil {
		t.Fatal("built-in retrying PAT succeeded")
	}
	if _, err := r.executeInvocation(context.Background(), "https://example.test", inv); !errors.Is(err, handled) {
		t.Fatalf("built-in handled PAT = %v", err)
	}

	callCount := 0
	edition.Override(&edition.Hooks{ClassifyToolResult: func(map[string]any) error {
		callCount++
		if callCount%2 == 0 {
			return wantErr
		}
		return nil
	}})
	runnerCallTool = func(*transport.Client, context.Context, string, string, map[string]any) (transport.ToolCallResult, error) {
		return transport.ToolCallResult{IsError: true, Content: map[string]any{"message": "business"}}, nil
	}
	if _, err := r.executeInvocation(context.Background(), "https://example.test", inv); !errors.Is(err, wantErr) {
		t.Fatalf("business hook = %v", err)
	}

	edition.Override(&edition.Hooks{})
	runnerCallTool = func(*transport.Client, context.Context, string, string, map[string]any) (transport.ToolCallResult, error) {
		return transport.ToolCallResult{IsError: true, Blocks: []transport.ContentBlock{{Text: "missing_scope mail:read"}}, Content: map[string]any{}}, nil
	}
	if _, err := r.executeInvocation(context.Background(), "https://example.test", inv); !errors.Is(err, retried) {
		t.Fatalf("business scope retry = %v", err)
	}

	r.scanner = coverageScanner{report: safety.Report{Scanned: true, Findings: []safety.Finding{{Pattern: "bad"}}}}
	r.enforceContentScan = true
	runnerCallTool = func(*transport.Client, context.Context, string, string, map[string]any) (transport.ToolCallResult, error) {
		return transport.ToolCallResult{Content: map[string]any{"value": 1}}, nil
	}
	if _, err := r.executeInvocation(context.Background(), "https://example.test", inv); err == nil {
		t.Fatal("scan failure succeeded")
	}
}

func TestCrossPlatformCoverageRunnerRemainingStdioAuthAndHeadersCoverage(t *testing.T) {
	oldStdioInit := runnerStdioEnsureInitialized
	oldStdioCall := runnerStdioCallTool
	oldEdition := edition.Get()
	t.Cleanup(func() {
		runnerStdioEnsureInitialized = oldStdioInit
		runnerStdioCallTool = oldStdioCall
		edition.Override(oldEdition)
		StopAllStdioClients()
	})

	client := transport.NewStdioClient("unused", nil, nil)
	RegisterStdioClient("stdio-product", client)
	r := &runtimeRunner{globalFlags: &GlobalFlags{Timeout: 1}}
	inv := executor.Invocation{CanonicalProduct: "stdio-product", Tool: "tool"}
	wantErr := errors.New("stdio failed")
	runnerStdioEnsureInitialized = func(*transport.StdioClient, context.Context) error { return wantErr }
	if _, err := r.executeStdioInvocation(context.Background(), inv); err == nil || !strings.Contains(err.Error(), "stdio initialize failed") {
		t.Fatalf("stdio initialize error = %v", err)
	}
	runnerStdioEnsureInitialized = func(*transport.StdioClient, context.Context) error { return nil }
	runnerStdioCallTool = func(*transport.StdioClient, context.Context, string, map[string]any) (transport.ToolCallResult, error) {
		return transport.ToolCallResult{}, wantErr
	}
	if _, err := r.executeStdioInvocation(context.Background(), inv); err == nil || !strings.Contains(err.Error(), "stdio failed") {
		t.Fatalf("stdio call error = %v", err)
	}
	runnerStdioCallTool = func(*transport.StdioClient, context.Context, string, map[string]any) (transport.ToolCallResult, error) {
		return transport.ToolCallResult{IsError: true, Content: map[string]any{"message": "tool failed"}}, nil
	}
	if _, err := r.executeStdioInvocation(context.Background(), inv); err == nil || !strings.Contains(err.Error(), "tool failed") {
		t.Fatalf("stdio tool error = %v", err)
	}
	runnerStdioCallTool = func(*transport.StdioClient, context.Context, string, map[string]any) (transport.ToolCallResult, error) {
		return transport.ToolCallResult{Content: map[string]any{"ok": true}}, nil
	}
	if got, err := r.executeStdioInvocation(context.Background(), inv); err != nil || !got.Invocation.Implemented {
		t.Fatalf("stdio success = %#v, %v", got, err)
	}

	r.globalFlags.Token = " explicit "
	if got, err := r.resolveAuthToken(context.Background()); err != nil || got != "explicit" {
		t.Fatalf("explicit auth token = %q, %v", got, err)
	}
	edition.Override(&edition.Hooks{TokenProvider: func(_ context.Context, fallback func() (string, error)) (string, error) {
		_, _ = fallback()
		return "provided", nil
	}})
	r.globalFlags.Token = ""
	if got, err := r.resolveAuthToken(context.Background()); err != nil || got != "provided" {
		t.Fatalf("provided auth token = %q, %v", got, err)
	}
	if got, err := resolveRuntimeAuthToken(context.Background(), " runtime "); err != nil || got != "runtime" {
		t.Fatalf("runtime explicit token = %q, %v", got, err)
	}

	t.Setenv(envDWSChannel, "channel")
	edition.Override(&edition.Hooks{
		MergeHeaders: func(headers map[string]string) map[string]string { return headers },
		EnterpriseCredentialHeaders: func(headers map[string]string) map[string]string {
			headers["x-enterprise"] = "yes"
			return headers
		},
	})
	headers := resolveIdentityHeaders()
	if headers["x-dws-channel"] != "channel" || headers["x-enterprise"] != "yes" {
		t.Fatalf("identity headers = %#v", headers)
	}

	if got := detectBusinessError(map[string]any{"content": map[string]any{"success": false, "errorMsg": "nested"}}); got != "nested" {
		t.Fatalf("nested business error = %q", got)
	}
	if got := detectBusinessError(map[string]any{"success": false, "data": map[string]any{"success": false, "errorMsg": "nested-first"}}); got != "nested-first" {
		t.Fatalf("nested-first business error = %q", got)
	}
}
