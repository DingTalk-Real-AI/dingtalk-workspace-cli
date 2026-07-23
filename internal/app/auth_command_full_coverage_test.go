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
	authDeviceLogin = func(provider *authpkg.DeviceFlowProvider, _ context.Context) (*authpkg.TokenData, error) {
		if provider.IdentityEnricher == nil {
			t.Error("device login missing shared identity enricher")
		}
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
	authOAuthLogin = func(provider *authpkg.OAuthProvider, _ context.Context, _ bool) (*authpkg.TokenData, error) {
		if provider.IdentityEnricher == nil {
			t.Error("OAuth login missing shared identity enricher")
		}
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
	if err := enrichAuthLoginProfileFromContact(ctx, "cfg", &authCoverageCaller{err: errors.New("call")}, &authpkg.TokenData{CorpID: "ding"}); err != nil {
		t.Fatalf("contact failure must remain best effort: %v", err)
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
	if err := enrichAuthLoginProfileFromContact(ctx, "cfg", mismatch, &authpkg.TokenData{CorpID: "ding", AccessToken: "token"}); err != nil {
		t.Fatalf("contact corp mismatch must remain best effort: %v", err)
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
	known := &authpkg.TokenData{CorpID: "ding", UserID: "exchange-user", AccessToken: "token"}
	differentContactUser := &authCoverageCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{Text: `{"result":[{"orgEmployeeModel":{"corpId":"ding","orgName":"Corp","userid":"other-user","name":"Other User"}}]}`}}}}
	if err := enrichAuthLoginProfileFromContact(ctx, "cfg", differentContactUser, known); err != nil {
		t.Fatal(err)
	}
	if known.UserID != "exchange-user" || known.UserName != "" || known.CorpName != "Corp" {
		t.Fatalf("token-exchange identity was overwritten: %#v", known)
	}
	multiOrg := &authCoverageCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{Text: `{"result":[{"orgEmployeeModel":{"corpId":"other","userid":"other-user"}},{"orgEmployeeModel":{"corpId":"ding","orgName":"Target Corp","userid":"target-user","name":"Target User"}}]}`}}}}
	multiOrgData := &authpkg.TokenData{CorpID: "ding", AccessToken: "token"}
	if err := enrichAuthLoginProfileFromContact(ctx, "cfg", multiOrg, multiOrgData); err != nil || multiOrgData.UserID != "target-user" || multiOrgData.CorpName != "Target Corp" {
		t.Fatalf("multi-org contact selection = %#v, %v", multiOrgData, err)
	}
	if _, ok := contactProfileIdentityFromJSON(
		[]byte(`{"result":[{"orgEmployeeModel":{"corpId":"other-a","userid":"user-a"}},{"orgEmployeeModel":{"corpId":"other-b","userid":"user-b"}}]}`),
		"ding",
	); ok {
		t.Fatal("multiple nonmatching organizations must not select an arbitrary contact identity")
	}
	if _, ok := contactProfileIdentityFromToolResult(nil); ok {
		t.Fatal("nil result should not parse")
	}
	if got := firstNonEmptyString(" ", " value ", "later"); got != "value" {
		t.Fatalf("first non-empty = %q", got)
	}
}

func TestCrossPlatformCoverageContactFailureReusesOnlySameCorpHistoricalDisplayMetadata(t *testing.T) {
	configDir := t.TempDir()
	if err := authpkg.SaveProfiles(configDir, &authpkg.ProfilesConfig{
		Version: 1,
		Profiles: []authpkg.Profile{{
			CorpID:   "ding_ecological_worker",
			CorpName: "Historical Corp",
			UserID:   "external-user",
			UserName: "Historical Worker",
		}},
	}); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}

	for _, tc := range []struct {
		name     string
		caller   edition.ToolCaller
		wantCorp string
	}{
		{
			name: "contact business error",
			caller: &authCoverageCaller{err: apperrors.NewAPI(
				"business error: success=false",
				apperrors.WithReason("business_error"),
			)},
			wantCorp: "Fresh Corp",
		},
		{
			name:     "contact has no identity",
			caller:   &authCoverageCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{Text: `{"success":false}`}}}},
			wantCorp: "Fresh Corp",
		},
		{
			name: "contact identity is missing user id",
			caller: &authCoverageCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{
				Text: `{"result":[{"orgEmployeeModel":{"corpId":"ding_ecological_worker","orgName":"Contact Corp"}}]}`,
			}}}},
			wantCorp: "Contact Corp",
		},
		{
			name:     "ordinary contact error",
			caller:   &authCoverageCaller{err: errors.New("network failure")},
			wantCorp: "Fresh Corp",
		},
		{
			name: "other contact business error",
			caller: &authCoverageCaller{err: apperrors.NewAPI(
				"permission denied",
				apperrors.WithReason("business_error"),
			)},
			wantCorp: "Fresh Corp",
		},
		{
			name:     "contact caller unavailable",
			caller:   nil,
			wantCorp: "Fresh Corp",
		},
		{
			name: "contact returns another organization",
			caller: &authCoverageCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{
				Text: `{"result":[{"orgEmployeeModel":{"corpId":"ding_other","userid":"other-user"}}]}`,
			}}}},
			wantCorp: "Fresh Corp",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := &authpkg.TokenData{
				AccessToken:  "new-access",
				RefreshToken: "new-refresh",
				CorpID:       "ding_ecological_worker",
				CorpName:     "Fresh Corp",
			}
			if err := enrichAuthLoginProfileFromContact(context.Background(), configDir, tc.caller, data); err != nil {
				t.Fatalf("contact failure blocked historical identity recovery: %v", err)
			}
			if data.UserID != "" || data.UserName != "Historical Worker" {
				t.Fatalf("historical metadata supplied UID evidence: %#v", data)
			}
			if data.CorpName != tc.wantCorp {
				t.Fatalf("corp name = %q, want %q", data.CorpName, tc.wantCorp)
			}
			if data.AccessToken != "new-access" || data.RefreshToken != "new-refresh" {
				t.Fatalf("new token material was changed: %#v", data)
			}
		})
	}
}

func TestCrossPlatformCoverageContactFailureDoesNotGuessHistoricalIdentity(t *testing.T) {
	businessErr := apperrors.NewAPI(
		"business error: success=false",
		apperrors.WithReason("business_error"),
	)
	for _, tc := range []struct {
		name     string
		corpID   string
		profiles []authpkg.Profile
		callErr  error
	}{
		{
			name:   "same corp has two identities",
			corpID: "ding_ecological_worker",
			profiles: []authpkg.Profile{
				{CorpID: "ding_ecological_worker", UserID: "external-user"},
				{CorpID: "ding_ecological_worker", UserID: "external-user-b"},
			},
			callErr: businessErr,
		},
		{
			name:   "same corp has one identity and one blank profile",
			corpID: "ding_ecological_worker",
			profiles: []authpkg.Profile{
				{CorpID: "ding_ecological_worker", UserID: "external-user"},
				{CorpID: "ding_ecological_worker"},
			},
			callErr: businessErr,
		},
		{
			name:   "identity belongs to another corp",
			corpID: "ding_ecological_worker",
			profiles: []authpkg.Profile{
				{CorpID: "ding_other", UserID: "external-user"},
			},
			callErr: businessErr,
		},
		{
			name:    "no historical identity",
			corpID:  "ding_ecological_worker",
			callErr: businessErr,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configDir := t.TempDir()
			if err := authpkg.SaveProfiles(configDir, &authpkg.ProfilesConfig{Version: 2, Profiles: tc.profiles}); err != nil {
				t.Fatalf("SaveProfiles() error = %v", err)
			}
			data := &authpkg.TokenData{AccessToken: "new-access", CorpID: tc.corpID}
			err := enrichAuthLoginProfileFromContact(
				context.Background(),
				configDir,
				&authCoverageCaller{err: tc.callErr},
				data,
			)
			if err != nil {
				t.Fatalf("contact failure must not block unresolved legacy login: %v", err)
			}
			if data.UserID != "" {
				t.Fatalf("ambiguous/cross-corp identity was reused: %#v", data)
			}
		})
	}
}

func TestCrossPlatformCoverageContactHistoryFallbackEdges(t *testing.T) {
	for _, data := range []*authpkg.TokenData{
		nil,
		{UserID: "known"},
		{},
	} {
		reused, err := enrichAuthLoginProfileFromHistory(t.TempDir(), data)
		if reused || err != nil {
			t.Fatalf("ineligible history fallback = %v, %v", reused, err)
		}
	}

	configDir := t.TempDir()
	if err := authpkg.SaveProfiles(configDir, &authpkg.ProfilesConfig{
		Version: 2,
		Profiles: []authpkg.Profile{{
			CorpID:   "ding_external",
			CorpName: "Historical Corp",
			UserID:   "external-user",
			UserName: "Historical Worker",
		}},
	}); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}
	data := &authpkg.TokenData{CorpID: "ding_external"}
	reused, err := enrichAuthLoginProfileFromHistory(configDir, data)
	if err != nil || !reused {
		t.Fatalf("history fallback = %v, %v", reused, err)
	}
	if data.CorpName != "Historical Corp" || data.UserName != "Historical Worker" || data.UserID != "" {
		t.Fatalf("history metadata = %#v", data)
	}

	corruptDir := t.TempDir()
	if err := os.Mkdir(authpkg.ProfilesPath(corruptDir), 0o700); err != nil {
		t.Fatalf("create unreadable profiles path: %v", err)
	}
	if reused, err := enrichAuthLoginProfileFromHistory(corruptDir, &authpkg.TokenData{CorpID: "ding_external"}); reused || err == nil {
		t.Fatalf("corrupt history fallback = %v, %v; want load error", reused, err)
	}

	businessErr := apperrors.NewAPI(
		"business error: success=false",
		apperrors.WithReason("business_error"),
	)
	for _, tc := range []struct {
		name   string
		caller *authCoverageCaller
	}{
		{
			name:   "contact business error",
			caller: &authCoverageCaller{err: businessErr},
		},
		{
			name:   "contact has no identity",
			caller: &authCoverageCaller{result: &edition.ToolResult{}},
		},
		{
			name: "contact identity is missing user id",
			caller: &authCoverageCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{
				Text: `{"result":[{"orgEmployeeModel":{"corpId":"ding_external"}}]}`,
			}}}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := enrichAuthLoginProfileFromContact(
				context.Background(),
				corruptDir,
				tc.caller,
				&authpkg.TokenData{CorpID: "ding_external", AccessToken: "new-access"},
			)
			if err != nil {
				t.Fatalf("best-effort contact/history lookup blocked login: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageAuthLoginConfigPreservesHistoryIdentityHint(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	oldResolve := authResolveProfile
	oldLoad := authLoadProfiles
	t.Cleanup(func() {
		authResolveProfile = oldResolve
		authLoadProfiles = oldLoad
	})

	explicit := &authpkg.Profile{CorpID: "ding_same", UserID: "user_2", Name: "second"}
	current := &authpkg.Profile{CorpID: "ding_current", UserID: "current_user"}
	authResolveProfile = func(_ string, selector string) (*authpkg.Profile, error) {
		switch selector {
		case "ding_same:user_2":
			clone := *explicit
			return &clone, nil
		case "external-worker":
			return &authpkg.Profile{Name: "external-worker", CorpID: "ding_external"}, nil
		case "":
			clone := *current
			return &clone, nil
		default:
			return nil, errors.New("missing")
		}
	}
	authLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) {
		return &authpkg.ProfilesConfig{}, nil
	}

	cmd := newAuthLoginCommand(nil)
	root, _, _ := authCoverageRoot(cmd, "table", true)
	if err := root.PersistentFlags().Set("profile", "ding_same:user_2"); err != nil {
		t.Fatal(err)
	}
	cfg, err := resolveAuthLoginConfig(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TargetCorpID != "ding_same" || cfg.HistoryProfileSelector != "ding_same:user_2" || !cfg.HistoryProfileSelectorExplicit {
		t.Fatalf("explicit login config = %#v", cfg)
	}
	if target, hint, exact, err := resolveAuthLoginTarget("cfg", "external-worker"); err != nil ||
		target != "ding_external" || hint != "ding_external" || !exact {
		t.Fatalf("blank-userId profile target = %q/%q/%v, %v", target, hint, exact, err)
	}

	if err := root.PersistentFlags().Set("profile", ""); err != nil {
		t.Fatal(err)
	}
	cfg, err = resolveAuthLoginConfig(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TargetCorpID != "" || cfg.HistoryProfileSelector != "ding_current:current_user" || cfg.HistoryProfileSelectorExplicit {
		t.Fatalf("implicit login config constrained authorization target: %#v", cfg)
	}

	if _, _, _, err := resolveAuthLoginTarget("cfg", "ding_same:missing"); err == nil {
		t.Fatal("missing exact profile must not be reinterpreted as a corpId")
	}
	if target, hint, explicitHint, err := resolveAuthLoginTarget("cfg", "ding_new"); err != nil || target != "ding_new" || hint != "" || explicitHint {
		t.Fatalf("new organization target = %q/%q/%v, %v", target, hint, explicitHint, err)
	}
	authLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) {
		return &authpkg.ProfilesConfig{Profiles: []authpkg.Profile{
			{CorpID: "ding_ambiguous", UserID: "user_1"},
			{CorpID: "ding_ambiguous", UserID: "user_2"},
		}}, nil
	}
	if _, _, _, err := resolveAuthLoginTarget("cfg", "ding_ambiguous"); err == nil {
		t.Fatal("ambiguous known organization must require an exact profile")
	}
}

func TestCrossPlatformCoverageOAuthAndDeviceKeepFreshUnknownIdentityIsolatedFromExactHistory(t *testing.T) {
	oldResolve := authResolveProfile
	oldLoad := authLoadProfiles
	oldDevice := authDeviceLogin
	oldOAuth := authOAuthLogin
	oldInteractive := authLoginInteractiveTerminal
	t.Cleanup(func() {
		authResolveProfile = oldResolve
		authLoadProfiles = oldLoad
		authDeviceLogin = oldDevice
		authOAuthLogin = oldOAuth
		authLoginInteractiveTerminal = oldInteractive
	})

	authResolveProfile = authpkg.ResolveProfile
	authLoadProfiles = authpkg.LoadProfiles
	authLoginInteractiveTerminal = func() bool { return false }
	t.Cleanup(func() { authpkg.SetRuntimeProfile("") })

	for _, flow := range []string{"oauth", "device"} {
		t.Run(flow, func(t *testing.T) {
			configDir := t.TempDir()
			keychainDir := t.TempDir()
			t.Setenv("DWS_CONFIG_DIR", configDir)
			t.Setenv(keychain.DisableKeychainEnv, "1")
			t.Setenv(keychain.StorageDirEnv, keychainDir)
			// StorageDirEnv isolates file-backed keychains, while Windows uses
			// DPAPI-protected HKCU values. Give every flow its own namespace so
			// OAuth/device fixtures cannot leak into each other or later tests.
			t.Setenv(keychain.TestNamespaceEnv, keychainDir)
			t.Cleanup(func() {
				if err := keychain.RemoveAuthTokenEntries(keychain.Service); err != nil {
					t.Errorf("clean auth keychain fixture: %v", err)
				}
			})
			authpkg.SetRuntimeProfile("")

			const (
				corpID        = "ding_same"
				historicalUID = "user_a"
				exactSelector = corpID + ":" + historicalUID
			)
			oldToken := &authpkg.TokenData{
				AccessToken:  "old-user-a-access",
				RefreshToken: "old-user-a-refresh",
				ExpiresAt:    time.Now().Add(time.Hour),
				RefreshExpAt: time.Now().Add(24 * time.Hour),
				CorpID:       corpID,
				CorpName:     "Same Corp",
				UserID:       historicalUID,
				UserName:     "Historical User A",
			}
			if err := authpkg.SaveTokenData(configDir, oldToken); err != nil {
				t.Fatalf("persist historical exact identity: %v", err)
			}

			caller := &authCoverageCaller{err: errors.New("contact unavailable")}
			var enriched *authpkg.TokenData
			freshToken := func() *authpkg.TokenData {
				return &authpkg.TokenData{
					AccessToken:  "fresh-user-b-access-" + flow,
					RefreshToken: "fresh-user-b-refresh-" + flow,
					ExpiresAt:    time.Now().Add(time.Hour),
					RefreshExpAt: time.Now().Add(24 * time.Hour),
					CorpID:       corpID,
				}
			}
			persistUnknown := func(ctx context.Context, identityEnricher func(context.Context, *authpkg.TokenData) error) (*authpkg.TokenData, error) {
				if identityEnricher == nil {
					return nil, errors.New("missing identity enricher")
				}
				data := freshToken()
				if err := identityEnricher(ctx, data); err != nil {
					return nil, err
				}
				enriched = data
				if data.UserID != "" {
					return nil, fmt.Errorf("historical profile supplied unproven userId %q", data.UserID)
				}
				if err := authpkg.SaveTokenData(configDir, data); err != nil {
					return nil, err
				}
				return data, nil
			}

			flags := map[string]string{"profile": exactSelector}
			switch flow {
			case "device":
				flags["device"] = "true"
				authDeviceLogin = func(provider *authpkg.DeviceFlowProvider, ctx context.Context) (*authpkg.TokenData, error) {
					return persistUnknown(ctx, provider.IdentityEnricher)
				}
			case "oauth":
				authOAuthLogin = func(provider *authpkg.OAuthProvider, ctx context.Context, _ bool) (*authpkg.TokenData, error) {
					if provider.TargetCorpID != corpID {
						return nil, fmt.Errorf("OAuth target corp = %q", provider.TargetCorpID)
					}
					return persistUnknown(ctx, provider.IdentityEnricher)
				}
			}
			if _, _, err := authCoverageRunLogin(t, caller, "table", true, flags); err != nil {
				t.Fatalf("%s login with unresolved fresh identity: %v", flow, err)
			}
			if enriched == nil || enriched.UserID != "" ||
				enriched.LegacyOrgScopedProfile != exactSelector ||
				enriched.CorpName != "Same Corp" ||
				enriched.UserName != "Historical User A" {
				t.Fatalf("%s history hint became identity evidence: %#v", flow, enriched)
			}

			historical, err := authpkg.LoadTokenDataForProfile(configDir, exactSelector)
			if err != nil {
				t.Fatalf("load historical exact identity: %v", err)
			}
			if historical.AccessToken != oldToken.AccessToken || historical.UserID != historicalUID {
				t.Fatalf("historical exact slot was overwritten: %#v", historical)
			}

			profiles, err := authpkg.LoadProfiles(configDir)
			if err != nil {
				t.Fatalf("load profiles: %v", err)
			}
			var unresolved *authpkg.Profile
			for i := range profiles.Profiles {
				profile := &profiles.Profiles[i]
				if profile.CorpID == corpID && profile.UserID == "" {
					unresolved = profile
					break
				}
			}
			if unresolved == nil {
				t.Fatalf("fresh UID-less token did not create an unresolved profile: %#v", profiles.Profiles)
			}
			unresolvedSelector := authpkg.ProfileSelectionSelector(*unresolved, profiles)
			if unresolvedSelector == "" || unresolvedSelector == exactSelector {
				t.Fatalf("unresolved selector = %q", unresolvedSelector)
			}
			fresh, err := authpkg.LoadTokenDataForProfile(configDir, unresolvedSelector)
			if err != nil {
				t.Fatalf("load fresh unresolved identity: %v", err)
			}
			if fresh.AccessToken != "fresh-user-b-access-"+flow || fresh.UserID != "" {
				t.Fatalf("fresh token was not isolated in unresolved org slot: %#v", fresh)
			}
		})
	}
}

func TestCrossPlatformCoverageHistoricalIdentityPriorityAndBlankUserID(t *testing.T) {
	oldLoad := authLoadProfiles
	t.Cleanup(func() { authLoadProfiles = oldLoad })
	authLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) { return nil, nil }
	if reused, err := enrichAuthLoginProfileFromHistory("cfg", &authpkg.TokenData{CorpID: "ding_same"}); reused || err != nil {
		t.Fatalf("nil history registry = reused=%v err=%v", reused, err)
	}

	cfg := &authpkg.ProfilesConfig{
		CurrentProfile: "ding_same:user_1",
		OrgCurrentProfiles: map[string]string{
			"ding_same": "ding_same:user_2",
		},
		Profiles: []authpkg.Profile{
			{CorpID: "ding_same", CorpName: "Same Corp", UserID: "user_1", UserName: "First"},
			{CorpID: "ding_same", CorpName: "Same Corp", UserID: "user_2", UserName: "Second"},
		},
	}
	authLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) { return cfg, nil }

	explicitData := &authpkg.TokenData{CorpID: "ding_same"}
	reused, err := enrichAuthLoginProfileFromHistory("cfg", explicitData, authLoginHistoryHint{Selector: "ding_same:user_1", Explicit: true})
	if err != nil || !reused || explicitData.UserID != "" ||
		explicitData.LegacyOrgScopedProfile != "ding_same:user_1" ||
		explicitData.CorpName != "Same Corp" || explicitData.UserName != "First" {
		t.Fatalf("explicit history selection = %#v, reused=%v err=%v", explicitData, reused, err)
	}
	mismatchedHintData := &authpkg.TokenData{CorpID: "ding_same"}
	reused, err = enrichAuthLoginProfileFromHistory("cfg", mismatchedHintData, authLoginHistoryHint{Selector: "ding_other:user_9", Explicit: true})
	if err != nil || reused || mismatchedHintData.UserID != "" {
		t.Fatalf("cross-corp explicit hint reused another identity: %#v, reused=%v err=%v", mismatchedHintData, reused, err)
	}
	orgCurrentData := &authpkg.TokenData{CorpID: "ding_same"}
	reused, err = enrichAuthLoginProfileFromHistory("cfg", orgCurrentData)
	if err != nil || reused || orgCurrentData.UserID != "" {
		t.Fatalf("implicit multi-account org-current was treated as identity proof: %#v, reused=%v err=%v", orgCurrentData, reused, err)
	}

	cfg.Profiles = []authpkg.Profile{cfg.Profiles[1]}
	soleData := &authpkg.TokenData{CorpID: "ding_same"}
	reused, err = enrichAuthLoginProfileFromHistory("cfg", soleData)
	if err != nil || !reused || soleData.UserID != "" ||
		soleData.CorpName != "Same Corp" || soleData.UserName != "Second" {
		t.Fatalf("sole history selection = %#v, reused=%v err=%v", soleData, reused, err)
	}

	cfg.Profiles = []authpkg.Profile{
		{CorpID: "ding_same", CorpName: "Same Corp", UserID: "user_1", UserName: "First"},
		{CorpID: "ding_same", CorpName: "Same Corp", UserID: "user_2", UserName: "Second"},
	}
	cfg.OrgCurrentProfiles = nil
	currentData := &authpkg.TokenData{CorpID: "ding_same"}
	reused, err = enrichAuthLoginProfileFromHistory("cfg", currentData)
	if err != nil || reused || currentData.UserID != "" {
		t.Fatalf("implicit multi-account current was treated as identity proof: %#v, reused=%v err=%v", currentData, reused, err)
	}

	cfg.CurrentProfile = ""
	ambiguousData := &authpkg.TokenData{CorpID: "ding_same"}
	reused, err = enrichAuthLoginProfileFromHistory("cfg", ambiguousData)
	if err != nil || reused || ambiguousData.UserID != "" {
		t.Fatalf("ambiguous history selection = %#v, reused=%v err=%v", ambiguousData, reused, err)
	}

	cfg.Profiles = []authpkg.Profile{{
		Name: "external-worker", CorpID: "ding_same", CorpName: "Legacy Corp", UserName: "Legacy Worker",
	}}
	blankData := &authpkg.TokenData{CorpID: "ding_same"}
	reused, err = enrichAuthLoginProfileFromHistory("cfg", blankData, authLoginHistoryHint{Selector: "external-worker", Explicit: true})
	if err != nil || !reused || blankData.UserID != "" || blankData.LegacyOrgScopedProfile != "external-worker" || blankData.CorpName != "Legacy Corp" || blankData.UserName != "Legacy Worker" {
		t.Fatalf("blank-userId history selection = %#v, reused=%v err=%v", blankData, reused, err)
	}
	contactBlankData := &authpkg.TokenData{CorpID: "ding_same", AccessToken: "new-token"}
	if err := enrichAuthLoginProfileFromContact(
		context.Background(),
		"cfg",
		&authCoverageCaller{err: errors.New("contact unavailable")},
		contactBlankData,
		authLoginHistoryHint{Selector: "external-worker", Explicit: true},
	); err != nil {
		t.Fatalf("blank-userId history must keep contact best effort: %v", err)
	}
	if contactBlankData.LegacyOrgScopedProfile != "external-worker" {
		t.Fatalf("blank-userId contact fallback did not authorize the historical organization slot: %#v", contactBlankData)
	}

	profiles := []*authpkg.Profile{
		nil,
		{Name: "duplicate", CorpID: "ding_same", UserID: "user_1"},
		{Name: "duplicate", CorpID: "ding_same", UserID: "user_2"},
	}
	for _, tc := range []struct {
		name     string
		selector string
		profiles []*authpkg.Profile
		want     *authpkg.Profile
	}{
		{name: "empty selector", selector: "", profiles: profiles},
		{name: "missing exact identity", selector: "ding_same:missing", profiles: profiles},
		{name: "duplicate name", selector: "duplicate", profiles: profiles},
		{name: "unmatched name", selector: "not-found", profiles: profiles},
		{name: "sole organization selector", selector: "ding_same", profiles: profiles[1:2], want: profiles[1]},
	} {
		t.Run("selector "+tc.name, func(t *testing.T) {
			if got := historicalProfileForSelector("ding_same", tc.selector, tc.profiles); got != tc.want {
				t.Fatalf("historicalProfileForSelector(%q) = %#v, want %#v", tc.selector, got, tc.want)
			}
		})
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
	_, _, _ = authCoverageRoot(importCmd, "table", false)
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
