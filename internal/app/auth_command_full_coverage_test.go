package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

type authCoverageCaller struct {
	result *edition.ToolResult
	err    error
}

func (c *authCoverageCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	return c.result, c.err
}
func (c *authCoverageCaller) CallToolWithToken(ctx context.Context, _ string, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	return c.CallTool(ctx, productID, toolName, args)
}
func (*authCoverageCaller) Format() string { return "json" }
func (*authCoverageCaller) DryRun() bool   { return false }
func (*authCoverageCaller) Fields() string { return "" }
func (*authCoverageCaller) JQ() string     { return "" }

func authCoverageRoot(child *cobra.Command, format string, yes bool) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	root := &cobra.Command{Use: "dws"}
	root.SetContext(context.Background())
	child.SetContext(context.Background())
	root.PersistentFlags().String("format", format, "")
	root.PersistentFlags().Bool("yes", yes, "")
	root.PersistentFlags().String("profile", "", "")
	root.AddCommand(child)
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(errOut)
	return root, out, errOut
}

func authCoverageRunLogin(t *testing.T, caller edition.ToolCaller, format string, yes bool, flags map[string]string) (string, string, error) {
	t.Helper()
	cmd := newAuthLoginCommand(caller)
	root, out, errOut := authCoverageRoot(cmd, format, yes)
	for name, value := range flags {
		flagSet := cmd.Flags()
		if root.PersistentFlags().Lookup(name) != nil {
			flagSet = root.PersistentFlags()
		}
		if err := flagSet.Set(name, value); err != nil {
			t.Fatalf("set %s: %v", name, err)
		}
	}
	err := cmd.RunE(cmd, nil)
	return out.String(), errOut.String(), err
}

func TestCrossPlatformCoverageAuthCoverageFormsParentAndTargets(t *testing.T) {
	oldEdition := edition.Get()
	oldClientID := authpkg.ClientID()
	oldClientSecret := authpkg.ClientSecret()
	oldRunForm := authRunForm
	oldPrompt := authLoginManualCredentialsPrompt
	oldSaveConfig := authSaveAppConfig
	oldResolve := authResolveProfile
	t.Cleanup(func() {
		edition.Override(oldEdition)
		authpkg.SetClientID(oldClientID)
		authpkg.SetClientSecret(oldClientSecret)
		authRunForm = oldRunForm
		authLoginManualCredentialsPrompt = oldPrompt
		authSaveAppConfig = oldSaveConfig
		authResolveProfile = oldResolve
	})

	edition.Override(&edition.Hooks{})
	parent := buildAuthCommand(nil)
	if parent.CommandPath() == "" || parent.RunE(parent, nil) != nil {
		t.Fatal("auth parent should render help")
	}
	edition.Override(&edition.Hooks{HideAuthLogin: true})
	if got := buildAuthCommand(nil).Commands(); len(got) != 7 {
		t.Fatalf("hidden-login subcommands = %d, want 7", len(got))
	}

	authRunForm = func(*huh.Form) error { return nil }
	if choice, err := selectAuthLoginGuideAction(); err != nil || choice != authLoginGuideDirectCLI {
		t.Fatalf("guide choice = %q, %v", choice, err)
	}
	if id, secret, err := promptAuthLoginManualCredentials(); err != nil || id != "" || secret != "" {
		t.Fatalf("manual prompt = %q/%q, %v", id, secret, err)
	}
	if mode, err := selectLoginRecommendScopeMode(); err != nil || mode != pat.LoginRecommendScopeRecommended {
		t.Fatalf("scope mode = %q, %v", mode, err)
	}
	products := []pat.LoginRecommendProduct{{ProductCode: "doc"}, {ProductCode: ""}}
	if selected, err := selectLoginRecommendProducts(products); err != nil || len(selected) != 1 || selected[0] != "doc" {
		t.Fatalf("selected products = %#v, %v", selected, err)
	}
	if selected, err := selectLoginRecommendProducts(nil); err != nil || selected != nil {
		t.Fatalf("empty selected products = %#v, %v", selected, err)
	}
	if selected, err := selectLoginRecommendProducts([]pat.LoginRecommendProduct{{}}); err != nil || selected != nil {
		t.Fatalf("blank selected products = %#v, %v", selected, err)
	}
	many := make([]pat.LoginRecommendProduct, 16)
	for i := range many {
		many[i].ProductCode = fmt.Sprintf("p%d", i)
	}
	if selected, err := selectLoginRecommendProducts(many); err != nil || len(selected) != 16 {
		t.Fatalf("many selected products = %d, %v", len(selected), err)
	}
	if authLoginProductsNonEmpty(nil) == nil || authLoginProductsNonEmpty([]string{"doc"}) != nil {
		t.Fatal("product validator mismatch")
	}

	authRunForm = func(*huh.Form) error { return errors.New("cancel") }
	if _, err := selectAuthLoginGuideAction(); err == nil {
		t.Fatal("guide cancellation should fail")
	}
	if _, _, err := promptAuthLoginManualCredentials(); err == nil {
		t.Fatal("credential cancellation should fail")
	}
	if _, err := selectLoginRecommendScopeMode(); err == nil {
		t.Fatal("scope cancellation should fail")
	}
	if _, err := selectLoginRecommendProducts(products); err == nil {
		t.Fatal("product cancellation should fail")
	}
	if authLoginNonEmpty("field")(" ") == nil || authLoginNonEmpty("field")("value") != nil {
		t.Fatal("non-empty validator mismatch")
	}

	cmd := &cobra.Command{}
	cmd.SetErr(io.Discard)
	if err := applyAuthLoginGuideAction(cmd, t.TempDir(), authLoginGuideDirectCLI); err != nil {
		t.Fatal(err)
	}
	if err := applyAuthLoginGuideAction(cmd, t.TempDir(), authLoginGuideConfigureAgentApp); err != nil {
		t.Fatal(err)
	}
	if err := applyAuthLoginGuideAction(cmd, t.TempDir(), "unknown"); err == nil {
		t.Fatal("unknown guide action should fail")
	}
	authLoginManualCredentialsPrompt = func() (string, string, error) { return "", "", errors.New("cancel") }
	if err := applyAuthLoginGuideAction(cmd, t.TempDir(), authLoginGuideManualCredentials); err == nil {
		t.Fatal("manual prompt error should propagate")
	}
	authLoginManualCredentialsPrompt = func() (string, string, error) { return "id", "secret", nil }
	authSaveAppConfig = func(string, *authpkg.AppConfig) error { return errors.New("save") }
	if err := applyAuthLoginGuideAction(cmd, t.TempDir(), authLoginGuideManualCredentials); err == nil {
		t.Fatal("app-config save error should propagate")
	}
	authSaveAppConfig = func(string, *authpkg.AppConfig) error { return nil }
	if err := applyAuthLoginGuideAction(cmd, t.TempDir(), authLoginGuideManualCredentials); err != nil {
		t.Fatal(err)
	}

	authResolveProfile = func(string, string) (*authpkg.Profile, error) {
		return &authpkg.Profile{CorpID: " ding-profile "}, nil
	}
	if got, err := resolveAuthLoginTargetCorpID("cfg", "name"); err != nil || got != "ding-profile" {
		t.Fatalf("resolved target = %q, %v", got, err)
	}
	authResolveProfile = func(string, string) (*authpkg.Profile, error) { return nil, errors.New("missing") }
	for selector, want := range map[string]struct {
		value string
		err   bool
	}{"": {"", false}, "ding-direct": {"ding-direct", false}, "other": {"", true}} {
		got, err := resolveAuthLoginTargetCorpID("cfg", selector)
		if got != want.value || (err != nil) != want.err {
			t.Fatalf("target %q = %q, %v", selector, got, err)
		}
	}
	badToken := &cobra.Command{}
	badToken.Flags().Bool("token", false, "")
	if _, err := resolveAuthLoginConfig(badToken); err == nil {
		t.Fatal("invalid token flag should fail")
	}
	badDevice := &cobra.Command{}
	badDevice.Flags().String("token", "", "")
	badDevice.Flags().String("device", "", "")
	if _, err := resolveAuthLoginConfig(badDevice); err == nil {
		t.Fatal("invalid device flag should fail")
	}
	badForce := &cobra.Command{}
	badForce.Flags().String("token", "", "")
	badForce.Flags().Bool("device", false, "")
	badForce.Flags().String("force", "", "")
	if _, err := resolveAuthLoginConfig(badForce); err == nil {
		t.Fatal("invalid force flag should fail")
	}
	badRecommend := &cobra.Command{}
	badRecommend.Flags().String("token", "", "")
	badRecommend.Flags().Bool("device", false, "")
	badRecommend.Flags().Bool("force", false, "")
	badRecommend.Flags().String("recommend", "", "")
	if _, err := resolveAuthLoginConfig(badRecommend); err == nil {
		t.Fatal("invalid recommend flag should fail")
	}
}

func TestCrossPlatformCoverageAuthCoverageLoginFlows(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	oldSave := authSaveTokenData
	oldDevice := authDeviceLogin
	oldOAuth := authOAuthLogin
	oldRunRecommend := authRunLoginRecommend
	oldRunWait := authRunDirectPATWait
	oldPlan := authPlanLoginRecommend
	oldScope := loginRecommendScopeModeSelector
	oldProducts := loginRecommendProductSelector
	oldInteractive := authLoginInteractiveTerminal
	oldResolve := authResolveProfile
	t.Cleanup(func() {
		authSaveTokenData = oldSave
		authDeviceLogin = oldDevice
		authOAuthLogin = oldOAuth
		authRunLoginRecommend = oldRunRecommend
		authRunDirectPATWait = oldRunWait
		authPlanLoginRecommend = oldPlan
		loginRecommendScopeModeSelector = oldScope
		loginRecommendProductSelector = oldProducts
		authLoginInteractiveTerminal = oldInteractive
		authResolveProfile = oldResolve
	})
	authInteractiveFalse := func() bool { return false }
	authLoginInteractiveTerminal = authInteractiveFalse
	authResolveProfile = func(string, string) (*authpkg.Profile, error) { return nil, errors.New("missing") }
	if _, _, err := authCoverageRunLogin(t, nil, "table", true, map[string]string{"profile": "bad"}); err == nil {
		t.Fatal("invalid login profile should fail")
	}

	authSaveTokenData = func(string, *authpkg.TokenData) error { return errors.New("save") }
	if _, _, err := authCoverageRunLogin(t, nil, "table", true, map[string]string{"token": "token"}); err == nil {
		t.Fatal("token save should fail")
	}
	authSaveTokenData = func(string, *authpkg.TokenData) error { return nil }
	if out, _, err := authCoverageRunLogin(t, nil, "table", true, map[string]string{"token": " token "}); err != nil || !strings.Contains(out, "登录成功") {
		t.Fatalf("token login = %q, %v", out, err)
	}
	if out, _, err := authCoverageRunLogin(t, nil, "json", true, map[string]string{"token": "token"}); err != nil || !strings.Contains(out, `"token_valid": true`) {
		t.Fatalf("json token login = %q, %v", out, err)
	}

	authDeviceLogin = func(*authpkg.DeviceFlowProvider, context.Context) (*authpkg.TokenData, error) {
		return nil, errors.New("device")
	}
	if _, _, err := authCoverageRunLogin(t, nil, "table", true, map[string]string{"device": "true"}); err == nil {
		t.Fatal("device error should propagate")
	}
	authDeviceLogin = func(*authpkg.DeviceFlowProvider, context.Context) (*authpkg.TokenData, error) {
		return &authpkg.TokenData{AccessToken: "a", ExpiresAt: time.Now().Add(time.Hour)}, nil
	}
	if _, _, err := authCoverageRunLogin(t, nil, "table", true, map[string]string{"device": "true", "no-browser": "true"}); err != nil {
		t.Fatal(err)
	}

	authOAuthLogin = func(*authpkg.OAuthProvider, context.Context, bool) (*authpkg.TokenData, error) {
		return nil, errors.New("oauth")
	}
	if _, _, err := authCoverageRunLogin(t, nil, "table", true, nil); err == nil {
		t.Fatal("oauth error should propagate")
	}
	authOAuthLogin = func(*authpkg.OAuthProvider, context.Context, bool) (*authpkg.TokenData, error) {
		return &authpkg.TokenData{
			AccessToken: "a", ExpiresAt: time.Now().Add(time.Hour), RefreshToken: "r", RefreshExpAt: time.Now().Add(48 * time.Hour),
			CorpName: "Corp", CorpID: "ding1", UserName: "User", UserID: "u",
		}, nil
	}
	caller := &authCoverageCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{Text: `{}`}}}}
	if out, _, err := authCoverageRunLogin(t, caller, "table", true, map[string]string{"no-browser": "true"}); err != nil || !strings.Contains(out, "Corp") {
		t.Fatalf("oauth success = %q, %v", out, err)
	}

	authRunLoginRecommend = func(context.Context, edition.ToolCaller, io.Writer, pat.LoginRecommendOptions) error {
		return errors.New("recommend")
	}
	if _, _, err := authCoverageRunLogin(t, nil, "table", true, map[string]string{"token": "x", "recommend": "true"}); err == nil {
		t.Fatal("recommend error should propagate")
	}
	if _, _, err := authCoverageRunLogin(t, nil, "json", true, map[string]string{"token": "x", "recommend": "true"}); err == nil {
		t.Fatal("JSON recommend error should propagate")
	}
	authRunLoginRecommend = func(context.Context, edition.ToolCaller, io.Writer, pat.LoginRecommendOptions) error {
		return &apperrors.PATError{RawJSON: `{"code":"PAT_SCOPE_AUTH_REQUIRED"}`}
	}
	waited := false
	authRunDirectPATWait = func(context.Context, *GlobalFlags, *apperrors.PATError, io.Writer) error {
		waited = true
		return nil
	}
	if _, _, err := authCoverageRunLogin(t, nil, "json", true, map[string]string{"token": "x", "recommend": "true"}); err != nil || !waited {
		t.Fatalf("PAT wait = %v, waited=%v", err, waited)
	}

	authLoginInteractiveTerminal = func() bool { return true }
	authPlanLoginRecommend = func(context.Context, edition.ToolCaller) (*pat.LoginRecommendPlan, error) {
		return nil, errors.New("plan")
	}
	if _, _, err := authCoverageRunLogin(t, nil, "table", false, map[string]string{"token": "x"}); err == nil {
		t.Fatal("plan error should propagate")
	}
	authPlanLoginRecommend = func(context.Context, edition.ToolCaller) (*pat.LoginRecommendPlan, error) {
		return &pat.LoginRecommendPlan{AllGranted: true}, nil
	}
	if _, stderr, err := authCoverageRunLogin(t, nil, "table", false, map[string]string{"token": "x"}); err != nil || !strings.Contains(stderr, "全部授权") {
		t.Fatalf("all-granted plan = %q, %v", stderr, err)
	}
	authPlanLoginRecommend = func(context.Context, edition.ToolCaller) (*pat.LoginRecommendPlan, error) {
		return &pat.LoginRecommendPlan{Scopes: []string{"scope"}, Products: []pat.LoginRecommendProduct{{ProductCode: "doc"}}}, nil
	}
	loginRecommendScopeModeSelector = func() (pat.LoginRecommendScopeMode, error) { return "", errors.New("scope") }
	if _, _, err := authCoverageRunLogin(t, nil, "table", false, map[string]string{"token": "x"}); err == nil {
		t.Fatal("scope selector error should propagate")
	}
	loginRecommendScopeModeSelector = func() (pat.LoginRecommendScopeMode, error) { return pat.LoginRecommendScopeAll, nil }
	loginRecommendProductSelector = func([]pat.LoginRecommendProduct) ([]string, error) { return []string{"doc"}, nil }
	selected := false
	authRunLoginRecommend = func(_ context.Context, _ edition.ToolCaller, _ io.Writer, opts pat.LoginRecommendOptions) error {
		if opts.ProductSelector != nil {
			_, err := opts.ProductSelector(opts.InitialPlan.Products)
			selected = err == nil
		}
		return nil
	}
	if _, _, err := authCoverageRunLogin(t, nil, "table", false, map[string]string{"token": "x"}); err != nil || !selected {
		t.Fatalf("interactive recommendation = %v, selected=%v", err, selected)
	}
}

func TestCrossPlatformCoverageAuthCoverageContactEnrichment(t *testing.T) {
	oldSave := authSaveTokenData
	t.Cleanup(func() { authSaveTokenData = oldSave })
	ctx := context.Background()
	if err := enrichAuthLoginProfileFromContact(ctx, "cfg", nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := enrichAuthLoginProfileFromContact(ctx, "cfg", &authCoverageCaller{}, &authpkg.TokenData{}); err != nil {
		t.Fatal(err)
	}
	complete := &authpkg.TokenData{CorpID: "ding", CorpName: "Corp", UserID: "u", UserName: "User"}
	if err := enrichAuthLoginProfileFromContact(ctx, "cfg", &authCoverageCaller{}, complete); err != nil {
		t.Fatal(err)
	}
	if err := enrichAuthLoginProfileFromContact(ctx, "cfg", &authCoverageCaller{err: errors.New("call")}, &authpkg.TokenData{CorpID: "ding"}); err == nil {
		t.Fatal("caller error should propagate")
	}
	if err := enrichAuthLoginProfileFromContact(
		ctx,
		"cfg",
		&authCoverageCaller{err: errors.New("call")},
		&authpkg.TokenData{CorpID: "ding", UserID: "known", AccessToken: "token"},
	); err != nil {
		t.Fatalf("optional contact metadata failure with known userId = %v", err)
	}
	for _, text := range []string{"", "not-json", `{"result":[]}`, `{"result":[{"orgEmployeeModel":{}}]}`} {
		caller := &authCoverageCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{Text: text}}}}
		if err := enrichAuthLoginProfileFromContact(ctx, "cfg", caller, &authpkg.TokenData{CorpID: "ding", AccessToken: "token"}); err != nil {
			t.Fatalf("invalid contact %q: %v", text, err)
		}
	}
	mismatch := &authCoverageCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{Text: `{"result":[{"orgEmployeeModel":{"corpId":"other"}}]}`}}}}
	if err := enrichAuthLoginProfileFromContact(ctx, "cfg", mismatch, &authpkg.TokenData{CorpID: "ding", AccessToken: "token"}); err == nil {
		t.Fatal("corp mismatch should fail")
	}
	same := &authCoverageCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{Text: `{"result":[{"orgEmployeeModel":{"corpId":"ding","orgName":"Corp","userid":"u","name":"User"}}]}`}}}}
	if err := enrichAuthLoginProfileFromContact(ctx, "cfg", same, complete); err != nil {
		t.Fatal(err)
	}
	unchangedPartial := &authCoverageCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{Text: `{"result":[{"orgEmployeeModel":{"corpId":"ding","orgName":"Corp","userid":"u"}}]}`}}}}
	if err := enrichAuthLoginProfileFromContact(ctx, "cfg", unchangedPartial, &authpkg.TokenData{CorpID: "ding", CorpName: "Corp", UserID: "u"}); err != nil {
		t.Fatal(err)
	}
	authSaveTokenData = func(string, *authpkg.TokenData) error { return errors.New("save") }
	data := &authpkg.TokenData{CorpID: "ding", AccessToken: "token"}
	if err := enrichAuthLoginProfileFromContact(ctx, "cfg", same, data); err != nil || data.CorpName != "Corp" || data.UserID != "u" {
		t.Fatalf("enriched = %#v, %v", data, err)
	}
	if _, ok := contactProfileIdentityFromToolResult(nil); ok {
		t.Fatal("nil result should not parse")
	}
	if got := firstNonEmptyString(" ", " value ", "later"); got != "value" {
		t.Fatalf("first non-empty = %q", got)
	}
}

func TestCrossPlatformCoverageAuthCoverageDefaultSeamClosures(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	configDir := t.TempDir()
	device := authpkg.NewDeviceFlowProvider(configDir, nil)
	_, _ = authDeviceLogin(device, ctx)
	oauth := authpkg.NewOAuthProvider(configDir, nil)
	oauth.NoBrowser = true
	_, _ = authOAuthStatus(oauth)
	_, _ = authOAuthAccessToken(oauth, ctx)
	_, _ = authOAuthLogin(oauth, ctx, true)
	_, _ = authOAuthExchange(oauth, ctx, "code", "uid")
	_ = fmt.Sprintf("%v", os.ErrNotExist)
}

func TestCrossPlatformCoverageAuthCoverageStatusAndLogout(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	oldEdition := edition.Get()
	oldStatus := authOAuthStatus
	oldAccess := authOAuthAccessToken
	oldDelete := authDeleteTokenData
	oldMark := authMarkProfileStatus
	oldResolve := authResolveProfile
	oldResolveDeletion := authResolveProfileDeletion
	oldRevoke := authRevokeToken
	oldRevokeForData := authRevokeTokenForData
	oldLoadTokenForProfile := authLoadTokenForProfile
	oldDeleteProfile := authDeleteProfileToken
	oldMigrate := authEnsureProfilesMigration
	oldLoadProfiles := authLoadProfiles
	oldDeleteAll := authDeleteAllTokenData
	t.Cleanup(func() {
		edition.Override(oldEdition)
		authOAuthStatus = oldStatus
		authOAuthAccessToken = oldAccess
		authDeleteTokenData = oldDelete
		authMarkProfileStatus = oldMark
		authResolveProfile = oldResolve
		authResolveProfileDeletion = oldResolveDeletion
		authRevokeToken = oldRevoke
		authRevokeTokenForData = oldRevokeForData
		authLoadTokenForProfile = oldLoadTokenForProfile
		authDeleteProfileToken = oldDeleteProfile
		authEnsureProfilesMigration = oldMigrate
		authLoadProfiles = oldLoadProfiles
		authDeleteAllTokenData = oldDeleteAll
	})

	badStatus := newAuthStatusCommand()
	bad := &cobra.Command{}
	bad.Flags().Bool("profile", false, "")
	if err := badStatus.RunE(bad, nil); err == nil {
		t.Fatal("invalid profile flag should fail")
	}

	runStatus := func(format string) (string, error) {
		cmd := newAuthStatusCommand()
		_, out, _ := authCoverageRoot(cmd, format, false)
		err := cmd.RunE(cmd, nil)
		return out.String(), err
	}
	authOAuthStatus = func(*authpkg.OAuthProvider) (*authpkg.TokenData, error) { return nil, nil }
	edition.Override(&edition.Hooks{})
	if out, err := runStatus("table"); err != nil || !strings.Contains(out, "auth login") {
		t.Fatalf("plain unauthenticated status = %q, %v", out, err)
	}
	authOAuthStatus = func(*authpkg.OAuthProvider) (*authpkg.TokenData, error) {
		return nil, keychain.NewUnavailableError("read", errors.New("status"))
	}
	edition.Override(&edition.Hooks{})
	if out, err := runStatus("table"); err != nil || !strings.Contains(out, "未登录") {
		t.Fatalf("status error = %q, %v", out, err)
	}
	if out, err := runStatus("json"); err != nil || !strings.Contains(out, `"authenticated": false`) {
		t.Fatalf("json status error = %q, %v", out, err)
	}

	now := time.Now()
	valid := &authpkg.TokenData{AccessToken: "a", ExpiresAt: now.Add(time.Hour), CorpID: "ding", CorpName: "Corp"}
	authOAuthStatus = func(*authpkg.OAuthProvider) (*authpkg.TokenData, error) { return valid, nil }
	if out, err := runStatus("table"); err != nil || !strings.Contains(out, "已登录") || !strings.Contains(out, "缺失或已过期") {
		t.Fatalf("valid access status = %q, %v", out, err)
	}
	valid.RefreshToken = "r"
	valid.RefreshExpAt = now.Add(time.Hour)
	if out, err := runStatus("table"); err != nil || !strings.Contains(out, "Refresh Token:") {
		t.Fatalf("valid refresh status = %q, %v", out, err)
	}

	expired := &authpkg.TokenData{AccessToken: "a", ExpiresAt: now.Add(-time.Hour), RefreshToken: "r", RefreshExpAt: now.Add(time.Hour), CorpID: "ding"}
	updated := &authpkg.TokenData{AccessToken: "new", ExpiresAt: now.Add(time.Hour), RefreshToken: "r", RefreshExpAt: now.Add(time.Hour), CorpID: "ding"}
	calls := 0
	authOAuthStatus = func(*authpkg.OAuthProvider) (*authpkg.TokenData, error) {
		calls++
		if calls == 1 {
			return expired, nil
		}
		return updated, nil
	}
	authOAuthAccessToken = func(*authpkg.OAuthProvider, context.Context) (string, error) { return "new", nil }
	if out, err := runStatus("table"); err != nil || !strings.Contains(out, "自动刷新") {
		t.Fatalf("refreshed status = %q, %v", out, err)
	}
	calls = 0
	authOAuthStatus = func(*authpkg.OAuthProvider) (*authpkg.TokenData, error) {
		calls++
		if calls == 1 {
			return expired, nil
		}
		return nil, errors.New("second status")
	}
	if _, err := runStatus("table"); err != nil {
		t.Fatal(err)
	}

	authOAuthStatus = func(*authpkg.OAuthProvider) (*authpkg.TokenData, error) { return expired, nil }
	authOAuthAccessToken = func(*authpkg.OAuthProvider, context.Context) (string, error) { return "", errors.New("refresh") }
	deleted := false
	marked := false
	authDeleteTokenData = func(string) error { deleted = true; return errors.New("ignored") }
	authMarkProfileStatus = func(string, string, string) error { marked = true; return errors.New("ignored") }
	edition.Override(&edition.Hooks{AutoPurgeToken: true})
	if _, err := runStatus("table"); err != nil || !deleted {
		t.Fatalf("auto-purge = %v, deleted=%v", err, deleted)
	}
	edition.Override(&edition.Hooks{})
	if _, err := runStatus("table"); err != nil || !marked {
		t.Fatalf("mark-expired = %v, marked=%v", err, marked)
	}

	authResolveProfileDeletion = func(string, string) (*authpkg.Profile, bool, error) { return nil, false, errors.New("missing") }
	if err := logoutOneProfile(nil, context.Background(), "cfg", "x"); err == nil {
		t.Fatal("missing profile should fail")
	}
	authResolveProfileDeletion = func(string, string) (*authpkg.Profile, bool, error) {
		return &authpkg.Profile{CorpID: "ding", UserID: "user"}, true, nil
	}
	authLoadTokenForProfile = func(string, string) (*authpkg.TokenData, error) {
		return &authpkg.TokenData{CorpID: "ding", UserID: "user"}, nil
	}
	authRevokeTokenForData = func(context.Context, *authpkg.TokenData) error { return errors.New("ignored") }
	var deletedSelector string
	authDeleteProfileToken = func(_ string, selector string) error {
		deletedSelector = selector
		return errors.New("delete")
	}
	if err := logoutOneProfile(nil, context.Background(), "cfg", "x"); err == nil {
		t.Fatal("profile delete should fail")
	}
	if deletedSelector != "ding:user" {
		t.Fatalf("exact deletion selector = %q, want stable identity selector", deletedSelector)
	}
	authDeleteProfileToken = func(_ string, selector string) error {
		deletedSelector = selector
		return nil
	}
	if err := logoutOneProfile(nil, context.Background(), "cfg", "x"); err != nil {
		t.Fatal(err)
	}
	if deletedSelector != "ding:user" {
		t.Fatalf("exact deletion selector = %q, want stable identity selector", deletedSelector)
	}

	authResolveProfileDeletion = func(string, string) (*authpkg.Profile, bool, error) {
		return &authpkg.Profile{CorpID: "ding", UserID: "user"}, false, nil
	}
	authLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) {
		return &authpkg.ProfilesConfig{Profiles: []authpkg.Profile{{CorpID: "ding", UserID: "user"}}}, nil
	}
	if err := logoutOneProfile(nil, context.Background(), "cfg", "organization-name"); err != nil {
		t.Fatal(err)
	}
	if deletedSelector != "ding" {
		t.Fatalf("organization deletion selector = %q, want stable corpId", deletedSelector)
	}

	authEnsureProfilesMigration = func(string) error { return errors.New("migrate") }
	if err := logoutAllProfiles(nil, context.Background(), "cfg"); err == nil {
		t.Fatal("migration should fail")
	}
	authEnsureProfilesMigration = func(string) error { return nil }
	authLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) { return nil, errors.New("load") }
	if err := logoutAllProfiles(nil, context.Background(), "cfg"); err == nil {
		t.Fatal("load profiles should fail")
	}
	revokes := 0
	authRevokeToken = func(context.Context) error { revokes++; return nil }
	authRevokeTokenForData = func(context.Context, *authpkg.TokenData) error { revokes++; return nil }
	authLoadTokenForProfile = func(string, string) (*authpkg.TokenData, error) {
		return &authpkg.TokenData{AccessToken: "token"}, nil
	}
	authLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) { return nil, nil }
	authDeleteAllTokenData = func(string) error { return nil }
	if err := logoutAllProfiles(nil, context.Background(), "cfg"); err != nil || revokes != 1 {
		t.Fatalf("empty profiles = %v, revokes=%d", err, revokes)
	}
	authLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) {
		return &authpkg.ProfilesConfig{Profiles: []authpkg.Profile{{CorpID: "a"}, {CorpID: "b"}}}, nil
	}
	if err := logoutAllProfiles(nil, context.Background(), "cfg"); err != nil || revokes != 3 {
		t.Fatalf("profile revokes = %v, revokes=%d", err, revokes)
	}
	authDeleteAllTokenData = func(string) error { return errors.New("delete all") }
	if err := logoutAllProfiles(nil, context.Background(), "cfg"); err == nil {
		t.Fatal("delete-all should fail")
	}
	logoutFailure := newAuthLogoutCommand()
	_, _, _ = authCoverageRoot(logoutFailure, "table", false)
	if err := logoutFailure.RunE(logoutFailure, nil); err == nil {
		t.Fatal("logout-all command failure should propagate")
	}
	logoutFailure = newAuthLogoutCommand()
	_, _, _ = authCoverageRoot(logoutFailure, "table", false)
	_ = logoutFailure.Flags().Set("profile", "ding")
	authDeleteProfileToken = func(string, string) error { return errors.New("delete") }
	if err := logoutFailure.RunE(logoutFailure, nil); err == nil {
		t.Fatal("logout-one command failure should propagate")
	}

	logout := newAuthLogoutCommand()
	badLogout := &cobra.Command{}
	badLogout.Flags().Bool("profile", false, "")
	badLogout.SetContext(context.Background())
	if err := logout.RunE(badLogout, nil); err == nil {
		t.Fatal("invalid logout profile flag should fail")
	}
	authDeleteAllTokenData = func(string) error { return nil }
	authLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) { return &authpkg.ProfilesConfig{}, nil }
	_, out, _ := authCoverageRoot(logout, "table", false)
	if err := logout.RunE(logout, nil); err != nil || !strings.Contains(out.String(), "重新登录") {
		t.Fatalf("logout = %q, %v", out.String(), err)
	}
	edition.Override(&edition.Hooks{IsEmbedded: true})
	authDeleteProfileToken = func(string, string) error { return nil }
	logout = newAuthLogoutCommand()
	_, out, _ = authCoverageRoot(logout, "table", false)
	if err := logout.Flags().Set("profile", "ding"); err != nil {
		t.Fatal(err)
	}
	if err := logout.RunE(logout, nil); err != nil || strings.Contains(out.String(), "重新登录") {
		t.Fatalf("embedded logout = %q, %v", out.String(), err)
	}
}

func TestCrossPlatformCoverageAuthCoveragePortableExchangeAndReset(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	oldEdition := edition.Get()
	oldSupported := authPortableExportSupported
	oldReady := authPortableSourceReady
	oldTarget := authPortableTargetPopulated
	oldExport := authExportPortableBundle
	oldImport := authImportPortableBundle
	oldAtomic := authAtomicWrite
	oldRead := authReadFile
	oldExchange := authOAuthExchange
	oldDeleteAll := authDeleteAllTokenData
	oldRemove := authRemove
	oldDeleteConfig := authDeleteAppConfig
	t.Cleanup(func() {
		edition.Override(oldEdition)
		authPortableExportSupported = oldSupported
		authPortableSourceReady = oldReady
		authPortableTargetPopulated = oldTarget
		authExportPortableBundle = oldExport
		authImportPortableBundle = oldImport
		authAtomicWrite = oldAtomic
		authReadFile = oldRead
		authOAuthExchange = oldExchange
		authDeleteAllTokenData = oldDeleteAll
		authRemove = oldRemove
		authDeleteAppConfig = oldDeleteConfig
	})

	export := newAuthExportCommandWithSupport(func() error { return nil })
	badString := &cobra.Command{}
	badString.Flags().Bool("output", false, "")
	if err := export.RunE(badString, nil); err == nil {
		t.Fatal("invalid output flag should fail")
	}
	badBool := &cobra.Command{}
	badBool.Flags().String("output", "x", "")
	badBool.Flags().String("base64", "", "")
	if err := export.RunE(badBool, nil); err == nil {
		t.Fatal("invalid base64 flag should fail")
	}
	_, _, _ = authCoverageRoot(export, "table", false)
	if err := export.RunE(export, nil); err == nil {
		t.Fatal("missing export output should fail")
	}
	authPortableExportSupported = func() bool { return false }
	_ = export.Flags().Set("output", "out")
	if err := export.RunE(export, nil); err == nil {
		t.Fatal("unsupported export should fail")
	}
	authPortableExportSupported = func() bool { return true }
	authPortableSourceReady = func() bool { return false }
	if err := export.RunE(export, nil); err == nil {
		t.Fatal("unready export should fail")
	}
	authPortableSourceReady = func() bool { return true }
	authExportPortableBundle = func(string, io.Writer) error { return errors.New("export") }
	if err := export.RunE(export, nil); err == nil {
		t.Fatal("export failure should propagate")
	}
	authExportPortableBundle = func(_ string, w io.Writer) error { _, _ = io.WriteString(w, "bundle"); return nil }
	authAtomicWrite = func(string, []byte, os.FileMode) error { return errors.New("write") }
	if err := export.RunE(export, nil); err == nil {
		t.Fatal("raw write should fail")
	}
	authAtomicWrite = func(string, []byte, os.FileMode) error { return nil }
	if err := export.RunE(export, nil); err != nil {
		t.Fatal(err)
	}
	export = newAuthExportCommandWithSupport(func() error { return nil })
	_, out, _ := authCoverageRoot(export, "table", false)
	_ = export.Flags().Set("base64", "true")
	export.SetOut(&appFailWriter{err: errors.New("stdout")})
	if err := export.RunE(export, nil); err == nil {
		t.Fatal("stdout failure should propagate")
	}
	export.SetOut(out)
	_ = export.Flags().Set("output", "encoded")
	authAtomicWrite = func(string, []byte, os.FileMode) error { return errors.New("write") }
	if err := export.RunE(export, nil); err == nil {
		t.Fatal("base64 write should fail")
	}
	authAtomicWrite = func(string, []byte, os.FileMode) error { return nil }
	if err := export.RunE(export, nil); err != nil {
		t.Fatal(err)
	}

	importCmd := newAuthImportCommandWithSupport(func() error { return nil })
	badInput := &cobra.Command{}
	badInput.Flags().Bool("input", false, "")
	if err := importCmd.RunE(badInput, nil); err == nil {
		t.Fatal("invalid input flag should fail")
	}
	badImportBase64 := &cobra.Command{}
	badImportBase64.Flags().String("input", "x", "")
	badImportBase64.Flags().String("base64", "", "")
	if err := importCmd.RunE(badImportBase64, nil); err == nil {
		t.Fatal("invalid import base64 flag should fail")
	}
	badForce := &cobra.Command{}
	badForce.Flags().String("input", "x", "")
	badForce.Flags().Bool("base64", false, "")
	badForce.Flags().String("force", "", "")
	if err := importCmd.RunE(badForce, nil); err == nil {
		t.Fatal("invalid force flag should fail")
	}
	_, out, _ = authCoverageRoot(importCmd, "table", false)
	if err := importCmd.RunE(importCmd, nil); err == nil {
		t.Fatal("missing input should fail")
	}
	_ = importCmd.Flags().Set("input", "bundle")
	authPortableTargetPopulated = func(string) bool { return true }
	if err := importCmd.RunE(importCmd, nil); err == nil {
		t.Fatal("populated target should require force")
	}
	authPortableTargetPopulated = func(string) bool { return false }
	authReadFile = func(string) ([]byte, error) { return nil, errors.New("read") }
	if err := importCmd.RunE(importCmd, nil); err == nil {
		t.Fatal("read failure should propagate")
	}
	authReadFile = func(string) ([]byte, error) { return []byte("%%%"), nil }
	_ = importCmd.Flags().Set("base64", "true")
	if err := importCmd.RunE(importCmd, nil); err == nil {
		t.Fatal("invalid base64 should fail")
	}
	authReadFile = func(string) ([]byte, error) { return []byte("YnVuZGxl"), nil }
	authImportPortableBundle = func(string, io.Reader) (authpkg.PortableImportReport, error) {
		return authpkg.PortableImportReport{}, errors.New("import")
	}
	if err := importCmd.RunE(importCmd, nil); err == nil {
		t.Fatal("import failure should propagate")
	}
	authImportPortableBundle = func(string, io.Reader) (authpkg.PortableImportReport, error) {
		return authpkg.PortableImportReport{BundleOS: "other", OSMismatch: true}, nil
	}
	if err := importCmd.RunE(importCmd, nil); err != nil {
		t.Fatal(err)
	}
	authImportPortableBundle = func(string, io.Reader) (authpkg.PortableImportReport, error) {
		return authpkg.PortableImportReport{}, nil
	}
	if err := importCmd.RunE(importCmd, nil); err != nil {
		t.Fatal(err)
	}

	exchange := newAuthExchangeCommand(nil)
	badCode := &cobra.Command{}
	badCode.Flags().Bool("code", false, "")
	if err := exchange.RunE(badCode, nil); err == nil {
		t.Fatal("invalid code flag should fail")
	}
	badUID := &cobra.Command{}
	badUID.Flags().String("code", "code", "")
	badUID.Flags().Bool("uid", false, "")
	if err := exchange.RunE(badUID, nil); err == nil {
		t.Fatal("invalid uid flag should fail")
	}
	_, out, _ = authCoverageRoot(exchange, "table", false)
	if err := exchange.RunE(exchange, nil); err == nil {
		t.Fatal("missing code should fail")
	}
	_ = exchange.Flags().Set("code", "code")
	authOAuthExchange = func(*authpkg.OAuthProvider, context.Context, string, string) (*authpkg.TokenData, error) {
		return nil, errors.New("exchange")
	}
	if err := exchange.RunE(exchange, nil); err == nil {
		t.Fatal("exchange error should propagate")
	}
	authOAuthExchange = func(*authpkg.OAuthProvider, context.Context, string, string) (*authpkg.TokenData, error) {
		return &authpkg.TokenData{CorpID: "ding", ExpiresAt: time.Now().Add(time.Hour)}, nil
	}
	_ = exchange.Flags().Set("uid", " user ")
	if err := exchange.RunE(exchange, nil); err != nil || !strings.Contains(out.String(), "ding") {
		t.Fatalf("exchange = %q, %v", out.String(), err)
	}

	reset := newAuthResetCommand()
	_, out, _ = authCoverageRoot(reset, "table", false)
	authDeleteAllTokenData = func(string) error { return errors.New("reset") }
	if err := reset.RunE(reset, nil); err == nil {
		t.Fatal("reset delete should fail")
	}
	removed := 0
	authDeleteAllTokenData = func(string) error { return nil }
	authRemove = func(string) error { removed++; return errors.New("ignored") }
	authDeleteAppConfig = func(string) error { removed++; return errors.New("ignored") }
	edition.Override(&edition.Hooks{})
	if err := reset.RunE(reset, nil); err != nil || removed != 3 || !strings.Contains(out.String(), "重新登录") {
		t.Fatalf("reset = %q, %v, removed=%d", out.String(), err, removed)
	}
	edition.Override(&edition.Hooks{IsEmbedded: true})
	reset = newAuthResetCommand()
	_, out, _ = authCoverageRoot(reset, "table", false)
	if err := reset.RunE(reset, nil); err != nil || strings.Contains(out.String(), "重新登录") {
		t.Fatalf("embedded reset = %q, %v", out.String(), err)
	}
}
