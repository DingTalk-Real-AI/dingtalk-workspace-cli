package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func TestDingTalkTagManageLoginPersistsExactProfileWithoutDSHEffects(t *testing.T) {
	caller := newSuccessfulLoginCaller(1)
	InitDepsForTest(t, caller)
	setupConnectSupervisorSeams(t)

	var exchange auth.ManagedExchangeRequest
	testseam.Swap(t, &deapConnectManagedExchange, func(ctx context.Context, _ string, request auth.ManagedExchangeRequest) (*auth.TokenData, error) {
		exchange = request
		identity, err := request.ResolveIdentity(ctx, "managed-access-secret", request.ExpectedCorpID)
		if err != nil {
			return nil, err
		}
		return &auth.TokenData{
			AccessToken: "managed-access-secret", CorpID: identity.CorpID, CorpName: identity.CorpName,
			UserID: identity.UserID, UserName: identity.UserName, ClientID: request.ClientID, Source: "mcp",
		}, nil
	})
	testseam.Swap(t, &deapConnectSaveBinding, func(string, digitalEmployeeBinding) error {
		t.Fatal("manage login persisted a DSH binding")
		return nil
	})
	testseam.Swap(t, &deapConnectRegisterDSH, func(context.Context, map[string]any) (string, error) {
		t.Fatal("manage login registered DSH")
		return "", nil
	})

	leaf := newManageLoginTestCommand(t, false)
	var output bytes.Buffer
	leaf.SetOut(&output)
	if err := leaf.RunE(leaf, nil); err != nil {
		t.Fatalf("manage login RunE() error = %v", err)
	}
	if exchange.ClientID != "returned-client" || exchange.AuthCode != "one-time-secret" ||
		exchange.ExpectedCorpID != "employee-corp" || exchange.ExpectedUserID != "employee-user" ||
		exchange.PreserveProfile != "supervisor-corp:supervisor-user" {
		t.Fatalf("managed exchange = %#v", exchange)
	}
	if auth.RuntimeProfile() != "" {
		t.Fatalf("manage login changed current runtime profile to %q", auth.RuntimeProfile())
	}
	combined := output.String()
	for _, secret := range []string{"one-time-secret", "returned-client", "managed-access-secret", "robot-uid", "439446171"} {
		if strings.Contains(combined, secret) {
			t.Fatalf("manage login leaked %q: %s", secret, combined)
		}
	}
	var envelope struct {
		OK   bool                       `json:"ok"`
		Data digitalEmployeeLoginResult `json:"data"`
	}
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK || envelope.Data.Status != "profile_saved" || envelope.Data.AgentUUID != "agent-1" ||
		envelope.Data.DWSProfile != "employee-corp:employee-user" || !envelope.Data.CurrentProfilePreserved ||
		envelope.Data.UseOnce != "dws --profile employee-corp:employee-user <command>" ||
		envelope.Data.SelectProfile != "dws profile use employee-corp:employee-user" {
		t.Fatalf("manage login envelope = %#v", envelope)
	}
	if len(caller.calls) != 2 || caller.calls[0].toolName != deapAgentDetailTool || caller.calls[1].toolName != deapAgentAuthCodeTool {
		t.Fatalf("ordinary calls = %#v", caller.calls)
	}
	if len(caller.tokenCalls) != 1 || caller.tokenCalls[0].toolName != "get_current_user_profile" {
		t.Fatalf("token-scoped calls = %#v", caller.tokenCalls)
	}
}

func TestDingTalkTagManageLoginRejectsMissingOrWrongIdentityBeforePersistence(t *testing.T) {
	tests := []struct {
		name       string
		published  string
		authorized string
		want       string
	}{
		{
			name:       "published identity missing",
			published:  `{"success":true,"data":{"status":"online","profile":{"corpId":"employee-corp"}}}`,
			authorized: successfulAuthResponse(), want: "发布详情缺少",
		},
		{
			name:       "robot uid mismatch",
			published:  successfulPublishedDetail(),
			authorized: `{"success":true,"data":{"dwsClientId":"returned-client","uid":"other-robot","staffId":"employee-user","dwsAuthCode":"one-time-secret","orgId":"439446171"}}`,
			want:       "profile.robotUid 不一致",
		},
		{
			name:       "staff user id mismatch",
			published:  successfulPublishedDetail(),
			authorized: `{"success":true,"data":{"dwsClientId":"returned-client","uid":"robot-uid","staffId":"other-user","dwsAuthCode":"one-time-secret","orgId":"439446171"}}`,
			want:       "profile.staffId 不一致",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{
				"deap-dev/get_digital_employee_detail": {tc.published},
				"deap-dev/get_dws_auth_code":           {tc.authorized},
			}}
			InitDepsForTest(t, caller)
			setupConnectSupervisorSeams(t)
			persisted := false
			testseam.Swap(t, &deapConnectManagedExchange, func(context.Context, string, auth.ManagedExchangeRequest) (*auth.TokenData, error) {
				persisted = true
				return nil, errors.New("unexpected exchange")
			})
			leaf := newManageLoginTestCommand(t, false)
			if err := leaf.RunE(leaf, nil); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("manage login error = %v, want %q", err, tc.want)
			}
			if persisted {
				t.Fatal("invalid identity reached persistence")
			}
		})
	}
}

func TestDingTalkTagManageLoginRefreshesSameProfileAndPreservesSupervisor(t *testing.T) {
	caller := newSuccessfulLoginCaller(2)
	InitDepsForTest(t, caller)
	setupConnectSupervisorSeams(t)
	exchanges := 0
	testseam.Swap(t, &deapConnectManagedExchange, func(_ context.Context, _ string, request auth.ManagedExchangeRequest) (*auth.TokenData, error) {
		exchanges++
		if request.PreserveProfile != "supervisor-corp:supervisor-user" {
			t.Fatalf("refresh preserve profile = %q", request.PreserveProfile)
		}
		return &auth.TokenData{CorpID: request.ExpectedCorpID, UserID: request.ExpectedUserID}, nil
	})
	for i := 0; i < 2; i++ {
		leaf := newManageLoginTestCommand(t, false)
		leaf.SetOut(&bytes.Buffer{})
		if err := leaf.RunE(leaf, nil); err != nil {
			t.Fatalf("manage login attempt %d error = %v", i+1, err)
		}
	}
	if exchanges != 2 {
		t.Fatalf("managed exchanges = %d, want 2", exchanges)
	}
	if auth.RuntimeProfile() != "" {
		t.Fatalf("refresh changed current runtime profile to %q", auth.RuntimeProfile())
	}
}

func TestDingTalkTagManageLoginDryRunHasNoExternalEffects(t *testing.T) {
	caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{}}
	InitDepsForTest(t, caller)
	testseam.Swap(t, &deapConnectManagedExchange, func(context.Context, string, auth.ManagedExchangeRequest) (*auth.TokenData, error) {
		t.Fatal("dry-run exchanged authorization code")
		return nil, nil
	})
	leaf := newManageLoginTestCommand(t, true)
	var output bytes.Buffer
	leaf.SetOut(&output)
	if err := leaf.RunE(leaf, nil); err != nil {
		t.Fatalf("dry-run error = %v", err)
	}
	if len(caller.calls) != 0 || strings.Contains(output.String(), "dwsAuthCode") {
		t.Fatalf("dry-run calls=%#v output=%s", caller.calls, output.String())
	}
}

func newSuccessfulLoginCaller(count int) *digitalEmployeeProtocolCaller {
	details := make([]string, count)
	authorizations := make([]string, count)
	identities := make([]string, count)
	for i := 0; i < count; i++ {
		details[i] = successfulPublishedDetail()
		authorizations[i] = successfulAuthResponse()
		identities[i] = `{"result":[{"orgEmployeeModel":{"corpId":"employee-corp","orgName":"员工企业","userId":"employee-user","orgUserName":"数字员工"}}]}`
	}
	return &digitalEmployeeProtocolCaller{responses: map[string][]string{
		"deap-dev/get_digital_employee_detail": details,
		"deap-dev/get_dws_auth_code":           authorizations,
		"contact/get_current_user_profile":     identities,
	}}
}

func successfulPublishedDetail() string {
	return `{"success":true,"data":{"status":"online","profile":{"corpId":"employee-corp","robotUid":"robot-uid","staffId":"employee-user"}}}`
}

func newManageLoginTestCommand(t *testing.T, dryRun bool) *cobra.Command {
	t.Helper()
	root := deapHandler{}.Command(&captureRunner{})
	root.PersistentFlags().Bool("dry-run", false, "test dry-run")
	leaf, _, err := root.Find([]string{"manage", "login"})
	if err != nil {
		t.Fatal(err)
	}
	if err := leaf.Flags().Set("agent-uuid", "agent-1"); err != nil {
		t.Fatal(err)
	}
	if dryRun {
		if err := leaf.InheritedFlags().Set("dry-run", "true"); err != nil {
			t.Fatal(err)
		}
	}
	return leaf
}
