package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type digitalEmployeeProtocolCaller struct {
	responses      map[string][]string
	tokenResponses map[string][]string
	calls          []deapAgentCall
	tokenCalls     []deapAgentCall
	tokens         []string
}

func (c *digitalEmployeeProtocolCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, deapAgentCall{productID: productID, toolName: toolName, args: args})
	key := productID + "/" + toolName
	queue := c.responses[key]
	if len(queue) == 0 {
		return nil, errors.New("unexpected MCP call")
	}
	c.responses[key] = queue[1:]
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: queue[0]}}}, nil
}

func (*digitalEmployeeProtocolCaller) Format() string { return "json" }
func (*digitalEmployeeProtocolCaller) DryRun() bool   { return false }
func (*digitalEmployeeProtocolCaller) Fields() string { return "" }
func (*digitalEmployeeProtocolCaller) JQ() string     { return "" }

func (c *digitalEmployeeProtocolCaller) CallToolWithToken(_ context.Context, token, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.tokenCalls = append(c.tokenCalls, deapAgentCall{productID: productID, toolName: toolName, args: args})
	c.tokens = append(c.tokens, token)
	key := productID + "/" + toolName
	queue := c.tokenResponses[key]
	tokenScopedResponse := len(queue) > 0
	if !tokenScopedResponse {
		queue = c.responses[key]
	}
	if len(queue) == 0 {
		return nil, errors.New("unexpected token-scoped MCP call")
	}
	if tokenScopedResponse {
		c.tokenResponses[key] = queue[1:]
	} else {
		c.responses[key] = queue[1:]
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: queue[0]}}}, nil
}

type digitalEmployeeIdentityBlocksCaller struct {
	blocks []edition.ContentBlock
}

func (*digitalEmployeeIdentityBlocksCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	return nil, errors.New("unexpected ordinary MCP call")
}
func (c *digitalEmployeeIdentityBlocksCaller) CallToolWithToken(context.Context, string, string, string, map[string]any) (*edition.ToolResult, error) {
	return &edition.ToolResult{Content: c.blocks}, nil
}
func (*digitalEmployeeIdentityBlocksCaller) Format() string { return "json" }
func (*digitalEmployeeIdentityBlocksCaller) DryRun() bool   { return false }
func (*digitalEmployeeIdentityBlocksCaller) Fields() string { return "" }
func (*digitalEmployeeIdentityBlocksCaller) JQ() string     { return "" }

func TestDigitalEmployeeManagedIdentityRequiresOneExactUserIDAcrossAllBlocks(t *testing.T) {
	t.Run("orgUserId is not a userId proof", func(t *testing.T) {
		InitDepsForTest(t, &digitalEmployeeIdentityBlocksCaller{blocks: []edition.ContentBlock{{
			Type: "text", Text: `{"result":[{"orgEmployeeModel":{"corpId":"employee-corp","orgUserId":"employee-user"}}]}`,
		}}})
		if _, err := resolveDigitalEmployeeManagedIdentity(context.Background(), "access-secret", "employee-corp"); err == nil {
			t.Fatal("orgUserId-only identity was accepted")
		}
	})

	t.Run("different identities in separate blocks are ambiguous", func(t *testing.T) {
		InitDepsForTest(t, &digitalEmployeeIdentityBlocksCaller{blocks: []edition.ContentBlock{
			{Type: "text", Text: `{"result":[{"orgEmployeeModel":{"corpId":"employee-corp","userId":"employee-one"}}]}`},
			{Type: "text", Text: `{"result":[{"orgEmployeeModel":{"corpId":"employee-corp","userId":"employee-two"}}]}`},
		}})
		if _, err := resolveDigitalEmployeeManagedIdentity(context.Background(), "access-secret", "employee-corp"); err == nil {
			t.Fatal("multiple text-block identities were accepted")
		}
	})

	t.Run("multiple identities in the same organization are ambiguous", func(t *testing.T) {
		InitDepsForTest(t, &digitalEmployeeIdentityBlocksCaller{blocks: []edition.ContentBlock{{
			Type: "text", Text: `{"result":[{"orgEmployeeModel":{"corpId":"employee-corp","userId":"employee-one"}},{"orgEmployeeModel":{"corpId":"employee-corp","userId":"employee-two"}}]}`,
		}}})
		if _, err := resolveDigitalEmployeeManagedIdentity(context.Background(), "access-secret", "employee-corp"); err == nil {
			t.Fatal("multiple same-organization identities were accepted")
		}
	})

	t.Run("missing UID record does not disappear from ambiguity check", func(t *testing.T) {
		InitDepsForTest(t, &digitalEmployeeIdentityBlocksCaller{blocks: []edition.ContentBlock{{
			Type: "text", Text: `{"result":[{"orgEmployeeModel":{"corpId":"employee-corp","userId":"employee-one"}},{"orgEmployeeModel":{"corpId":"employee-corp"}}]}`,
		}}})
		if _, err := resolveDigitalEmployeeManagedIdentity(context.Background(), "access-secret", "employee-corp"); err == nil {
			t.Fatal("same-organization record without UID was ignored")
		}
	})

	t.Run("conflicting userId aliases are ambiguous", func(t *testing.T) {
		InitDepsForTest(t, &digitalEmployeeIdentityBlocksCaller{blocks: []edition.ContentBlock{{
			Type: "text", Text: `{"result":[{"orgEmployeeModel":{"corpId":"employee-corp","userId":"employee-one","userid":"employee-two"}}]}`,
		}}})
		if _, err := resolveDigitalEmployeeManagedIdentity(context.Background(), "access-secret", "employee-corp"); err == nil {
			t.Fatal("conflicting userId aliases were accepted")
		}
	})
}

func TestDingTalkTagExposesIndependentConnectAndChannelProtocols(t *testing.T) {
	newDeapAgentTestTree(t, false)
	root := deapHandler{}.Command(&captureRunner{})
	root.PersistentFlags().Bool("dry-run", false, "test dry-run")
	root.PersistentFlags().Bool("yes", false, "test confirmation")

	connect, rest, err := root.Find([]string{"connect"})
	if err != nil || len(rest) != 0 || connect == root {
		t.Fatalf("find connect: command=%v rest=%v err=%v", connect, rest, err)
	}
	if !connect.Runnable() {
		t.Fatal("dingtalk-tag connect must be runnable")
	}
	for _, flag := range []string{"agent-uuid", "channel", "profile-only", "client-id", "dry-run", "yes"} {
		if connect.Flags().Lookup(flag) == nil && connect.InheritedFlags().Lookup(flag) == nil {
			t.Errorf("dingtalk-tag connect missing --%s", flag)
		}
	}
	if final, ok := contractfinal.RuntimeContractFinal(connect); !ok || final.Identity == nil || final.Safety == nil {
		t.Fatalf("connect ContractFinal incomplete: %+v ok=%v", final, ok)
	}

	channel, rest, err := root.Find([]string{"channel"})
	if err != nil || len(rest) != 0 || channel == root || !channel.HasSubCommands() {
		t.Fatalf("find channel group: command=%v rest=%v err=%v", channel, rest, err)
	}
	for _, name := range []string{"capabilities", "reply", "operator-private"} {
		leaf, remaining, findErr := channel.Find([]string{name})
		if findErr != nil || len(remaining) != 0 || leaf == channel || !leaf.Runnable() {
			t.Fatalf("find channel leaf %q: command=%v rest=%v err=%v", name, leaf, remaining, findErr)
		}
		if final, ok := contractfinal.RuntimeContractFinal(leaf); !ok || final.Identity == nil || final.Safety == nil {
			t.Errorf("channel %s ContractFinal incomplete: %+v ok=%v", name, final, ok)
		}
	}
}

func TestDingTalkTagChannelCapabilitiesUsesDWSMachineEnvelope(t *testing.T) {
	newDeapAgentTestTree(t, false)
	root := deapHandler{}.Command(&captureRunner{})
	leaf, _, err := root.Find([]string{"channel", "capabilities"})
	if err != nil {
		t.Fatal(err)
	}
	if err := leaf.Flags().Set("channel", "dsh"); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	leaf.SetOut(&output)
	if err := leaf.RunE(leaf, nil); err != nil {
		t.Fatalf("capabilities RunE() error = %v", err)
	}
	var envelope struct {
		OK      bool   `json:"ok"`
		Outcome string `json:"outcome"`
		Data    struct {
			ProtocolVersion int    `json:"protocolVersion"`
			AuditMode       string `json:"auditMode"`
			Capabilities    struct {
				EventConsume         bool `json:"eventConsume"`
				ReplyStdin           bool `json:"replyStdin"`
				OperatorPrivateStdin bool `json:"operatorPrivateStdin"`
			} `json:"capabilities"`
		} `json:"data"`
	}
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output %q: %v", output.String(), err)
	}
	if !envelope.OK || envelope.Outcome != "success" || envelope.Data.ProtocolVersion != 1 || envelope.Data.AuditMode != "local_required" ||
		!envelope.Data.Capabilities.EventConsume || !envelope.Data.Capabilities.ReplyStdin || !envelope.Data.Capabilities.OperatorPrivateStdin {
		t.Fatalf("capability envelope = %#v", envelope)
	}
}

func TestDingTalkTagChannelReplyReadsBoundedStrictStdinAndNormalizesEnvelope(t *testing.T) {
	installEmployeeReplyBinding(t)
	caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{
		"im/list_messages_by_ids":    {`{"result":[{"openMessageId":"message-1","senderOpenDingTalkId":"operator-open"}]}`},
		"chat/send_personal_message": {`{"result":{"openMessageId":"reply-1","sendStatus":"SUCCESS"}}`},
	}}
	InitDepsForTest(t, caller)
	root := deapHandler{}.Command(&captureRunner{})
	leaf, _, err := root.Find([]string{"channel", "reply"})
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"channel": "dsh", "stdin": "true"} {
		if err := leaf.Flags().Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	input := `{"schemaVersion":1,"protocolVersion":1,"agentUuid":"agent-1","eventId":"event-1","sessionId":"session-1","conversationId":"conversation-1","referenceMessageId":"message-1","text":"正文不应进入 argv 或输出","idempotencyKey":"idem-1"}`
	leaf.SetIn(strings.NewReader(input))
	var output bytes.Buffer
	leaf.SetOut(&output)
	if err := leaf.RunE(leaf, nil); err != nil {
		t.Fatalf("reply RunE() error = %v", err)
	}
	if len(caller.calls) != 2 || caller.calls[1].toolName != "send_personal_message" {
		t.Fatalf("calls = %#v", caller.calls)
	}
	if caller.calls[1].args["uuid"] != "idem-1" || caller.calls[1].args["openConversationId"] != "conversation-1" {
		t.Fatalf("send args = %#v", caller.calls[1].args)
	}
	if strings.Contains(output.String(), "正文不应进入") {
		t.Fatalf("machine output leaked message body: %s", output.String())
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			OpenMessageID  string `json:"openMessageId"`
			ConversationID string `json:"conversationId"`
			DeliveryStatus string `json:"deliveryStatus"`
			IdempotencyKey string `json:"idempotencyKey"`
		} `json:"data"`
	}
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK || envelope.Data.OpenMessageID != "reply-1" || envelope.Data.ConversationID != "conversation-1" ||
		envelope.Data.DeliveryStatus != "delivered" || envelope.Data.IdempotencyKey != "idem-1" {
		t.Fatalf("reply envelope = %#v", envelope)
	}

	bad := deapHandler{}.Command(&captureRunner{})
	badLeaf, _, _ := bad.Find([]string{"channel", "reply"})
	_ = badLeaf.Flags().Set("channel", "dsh")
	_ = badLeaf.Flags().Set("stdin", "true")
	badLeaf.SetIn(strings.NewReader(`{"schemaVersion":1,"protocolVersion":1,"agentUuid":"a","eventId":"e","conversationId":"c","referenceMessageId":"m","text":"secret","idempotencyKey":"i","unknown":true}`))
	if err := badLeaf.RunE(badLeaf, nil); err == nil || !strings.Contains(err.Error(), "invalid digital employee stdin JSON") {
		t.Fatalf("unknown field error = %v", err)
	}
	if len(caller.calls) != 2 {
		t.Fatalf("bad stdin made MCP call: %#v", caller.calls)
	}

	oversized := deapHandler{}.Command(&captureRunner{})
	oversizedLeaf, _, _ := oversized.Find([]string{"channel", "reply"})
	_ = oversizedLeaf.Flags().Set("channel", "dsh")
	_ = oversizedLeaf.Flags().Set("stdin", "true")
	oversizedLeaf.SetIn(strings.NewReader(strings.Repeat("x", digitalEmployeeStdinLimit+1)))
	if err := oversizedLeaf.RunE(oversizedLeaf, nil); err == nil || !strings.Contains(err.Error(), "exceeds 256 KiB") {
		t.Fatalf("oversized stdin error = %v", err)
	}
	if len(caller.calls) != 2 {
		t.Fatalf("oversized stdin made MCP call: %#v", caller.calls)
	}
}

func TestDingTalkTagChannelReplyResolvesAsyncSendReceipt(t *testing.T) {
	installEmployeeReplyBinding(t)
	caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{
		"im/list_messages_by_ids":    {`{"result":[{"openMessageId":"message-1","senderOpenDingTalkId":"operator-open"}]}`},
		"chat/send_personal_message": {`{"result":{"openTaskId":"task-1"}}`},
		"im/query_message_send_status": {
			`{"result":{"openTaskId":"task-1","status":"PROCESSING"}}`,
			`{"result":{"openTaskId":"task-1","openMessageId":"reply-1","openConversationId":"conversation-1","status":"SUCCESS"}}`,
		},
	}}
	InitDepsForTest(t, caller)
	testseam.Swap(t, &deapChannelReceiptWait, func(context.Context, time.Duration) error { return nil })
	root := deapHandler{}.Command(&captureRunner{})
	leaf, _, err := root.Find([]string{"channel", "reply"})
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"channel": "dsh", "stdin": "true"} {
		if err := leaf.Flags().Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	leaf.SetIn(strings.NewReader(`{"schemaVersion":1,"protocolVersion":1,"agentUuid":"agent-1","eventId":"event-1","sessionId":"session-1","conversationId":"conversation-1","referenceMessageId":"message-1","text":"异步回执正文","idempotencyKey":"idem-1"}`))
	var output bytes.Buffer
	leaf.SetOut(&output)
	if err := leaf.RunE(leaf, nil); err != nil {
		t.Fatalf("reply RunE() error = %v", err)
	}
	if len(caller.calls) != 4 || caller.calls[3].productID != "im" || caller.calls[3].toolName != "query_message_send_status" || caller.calls[3].args["openTaskId"] != "task-1" {
		t.Fatalf("async receipt calls = %#v", caller.calls)
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			OpenMessageID  string `json:"openMessageId"`
			ConversationID string `json:"conversationId"`
			DeliveryStatus string `json:"deliveryStatus"`
			IdempotencyKey string `json:"idempotencyKey"`
		} `json:"data"`
	}
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK || envelope.Data.OpenMessageID != "reply-1" || envelope.Data.ConversationID != "conversation-1" ||
		envelope.Data.DeliveryStatus != "delivered" || envelope.Data.IdempotencyKey != "idem-1" {
		t.Fatalf("async reply envelope = %#v", envelope)
	}
}

func TestDigitalEmployeeDeliveryRejectsMissingMessageIDAfterBoundedPolling(t *testing.T) {
	statuses := make([]string, digitalEmployeeReceiptAttempts)
	for i := range statuses {
		statuses[i] = `{"result":{"openTaskId":"task-1","status":"PROCESSING"}}`
	}
	caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{
		"im/query_message_send_status": statuses,
	}}
	InitDepsForTest(t, caller)
	testseam.Swap(t, &deapChannelReceiptWait, func(context.Context, time.Duration) error { return nil })

	_, err := resolveDigitalEmployeeDelivery(context.Background(), map[string]any{
		"result": map[string]any{"openTaskId": "task-1"},
	}, "conversation-1", "idem-1")
	if err == nil || !strings.Contains(err.Error(), "openMessageId") {
		t.Fatalf("missing message id error = %v", err)
	}
	if len(caller.calls) != digitalEmployeeReceiptAttempts {
		t.Fatalf("status query count = %d, want %d", len(caller.calls), digitalEmployeeReceiptAttempts)
	}
}

func TestDingTalkTagConnectDryRunHasNoExternalEffects(t *testing.T) {
	caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{}}
	InitDepsForTest(t, caller)
	testseam.Swap(t, &deapConnectManagedExchange, func(context.Context, string, auth.ManagedExchangeRequest) (*auth.TokenData, error) {
		t.Fatal("dry-run exchanged an authorization code")
		return nil, nil
	})
	testseam.Swap(t, &deapConnectSaveBinding, func(string, digitalEmployeeBinding) error {
		t.Fatal("dry-run persisted a binding")
		return nil
	})
	testseam.Swap(t, &deapConnectRegisterDSH, func(context.Context, map[string]any) (string, error) {
		t.Fatal("dry-run registered DSH")
		return "", nil
	})

	leaf := newConnectTestCommand(t, true)
	var output bytes.Buffer
	leaf.SetOut(&output)
	if err := leaf.RunE(leaf, nil); err != nil {
		t.Fatalf("dry-run error = %v", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("dry-run made MCP calls: %#v", caller.calls)
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil || !envelope.OK || envelope.Data.Status != "planned" ||
		strings.Contains(output.String(), "dwsAuthCode") {
		t.Fatalf("dry-run output = %s, decode error = %v", output.String(), err)
	}
}

func TestDingTalkTagConnectProfileOnlyDryRunStopsAtProfilePersistence(t *testing.T) {
	caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{}}
	InitDepsForTest(t, caller)
	testseam.Swap(t, &deapConnectManagedExchange, func(context.Context, string, auth.ManagedExchangeRequest) (*auth.TokenData, error) {
		t.Fatal("profile-only dry-run exchanged an authorization code")
		return nil, nil
	})
	testseam.Swap(t, &deapConnectSaveBinding, func(string, digitalEmployeeBinding) error {
		t.Fatal("profile-only dry-run persisted a DSH binding")
		return nil
	})
	testseam.Swap(t, &deapConnectRegisterDSH, func(context.Context, map[string]any) (string, error) {
		t.Fatal("profile-only dry-run registered DSH")
		return "", nil
	})

	leaf := newProfileOnlyConnectTestCommand(t, true)
	var output bytes.Buffer
	leaf.SetOut(&output)
	if err := leaf.RunE(leaf, nil); err != nil {
		t.Fatalf("profile-only dry-run error = %v", err)
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			ProfileOnly     bool     `json:"profileOnly"`
			RestartRequired bool     `json:"restartRequired"`
			Steps           []string `json:"steps"`
		} `json:"data"`
	}
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK || !envelope.Data.ProfileOnly || envelope.Data.RestartRequired ||
		strings.Join(envelope.Data.Steps, ",") != "validate_draft,validate_published,request_auth_code,managed_exchange,persist_profile" ||
		strings.Contains(output.String(), `"channel"`) || strings.Contains(output.String(), "register_dsh") {
		t.Fatalf("profile-only dry-run envelope = %s", output.String())
	}
	if len(caller.calls) != 0 {
		t.Fatalf("profile-only dry-run made MCP calls: %#v", caller.calls)
	}
}

func TestDingTalkTagConnectProfileOnlyPersistsProfileWithoutDSHEffects(t *testing.T) {
	caller := newSuccessfulConnectCaller(successfulAuthResponse(), "")
	caller.responses["contact/get_current_user_profile"] = []string{
		`{"result":[{"orgEmployeeModel":{"corpId":"employee-corp","orgName":"员工企业","userId":"employee-user","orgUserName":"本地员工"}}]}`,
	}
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
			CorpID: identity.CorpID, UserID: identity.UserID,
			ClientID: request.ClientID, Source: "mcp",
		}, nil
	})
	testseam.Swap(t, &deapConnectSaveBinding, func(string, digitalEmployeeBinding) error {
		t.Fatal("profile-only persisted a DSH binding")
		return nil
	})
	testseam.Swap(t, &deapConnectRegisterDSH, func(context.Context, map[string]any) (string, error) {
		t.Fatal("profile-only registered DSH")
		return "", nil
	})

	leaf := newProfileOnlyConnectTestCommand(t, false)
	var output bytes.Buffer
	leaf.SetOut(&output)
	if err := leaf.RunE(leaf, nil); err != nil {
		t.Fatalf("profile-only RunE() error = %v", err)
	}
	if exchange.ClientID != "returned-client" || exchange.AuthCode != "one-time-secret" || exchange.PreserveProfile != "supervisor-corp:supervisor-user" {
		t.Fatalf("managed exchange = %#v", exchange)
	}
	if len(caller.calls) != 3 {
		t.Fatalf("profile-only ordinary MCP calls = %#v, want draft, published, auth only", caller.calls)
	}
	if len(caller.tokenCalls) != 1 || caller.tokenCalls[0].toolName != "get_current_user_profile" {
		t.Fatalf("profile-only token-scoped calls = %#v", caller.tokenCalls)
	}
	if auth.RuntimeProfile() != "" {
		t.Fatalf("profile-only changed runtime profile to %q", auth.RuntimeProfile())
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Status                 string `json:"status"`
			AgentUUID              string `json:"agentUuid"`
			Channel                string `json:"channel"`
			DWSProfile             string `json:"dwsProfile"`
			OperatorOpenDingTalkID string `json:"operatorOpenDingTalkId"`
			ProfileOnly            bool   `json:"profileOnly"`
			RestartRequired        bool   `json:"restartRequired"`
		} `json:"data"`
	}
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK || envelope.Data.Status != "profile_saved" || envelope.Data.AgentUUID != "agent-1" ||
		envelope.Data.DWSProfile != "employee-corp:employee-user" || !envelope.Data.ProfileOnly ||
		envelope.Data.Channel != "" || envelope.Data.OperatorOpenDingTalkID != "" || envelope.Data.RestartRequired {
		t.Fatalf("profile-only envelope = %#v", envelope)
	}
}

func TestDingTalkTagConnectRequiresOneExplicitMode(t *testing.T) {
	tests := []struct {
		name        string
		channel     string
		profileOnly bool
		want        string
	}{
		{name: "missing mode", want: "无法选择受支持的数字员工 Agent"},
		{name: "conflicting modes", channel: "dsh", profileOnly: true, want: "不能同时使用"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{}}
			InitDepsForTest(t, caller)
			leaf := newConnectTestCommandWithMode(t, true, tc.channel, tc.profileOnly)
			if err := leaf.RunE(leaf, nil); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("mode validation error = %v, want %q", err, tc.want)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("invalid mode made MCP calls: %#v", caller.calls)
			}
		})
	}
}

func TestDingTalkTagConnectRejectsInvalidPrerequisitesBeforeExchange(t *testing.T) {
	tests := []struct {
		name      string
		responses []string
		want      string
	}{
		{
			name:      "non local agent",
			responses: []string{`{"success":true,"data":{"digitalTagEmployeeProfile":{"mainProgramType":"deap_cloud"}}}`},
			want:      "不是 local_agent",
		},
		{
			name: "unpublished",
			responses: []string{
				`{"success":true,"data":{"digitalTagEmployeeProfile":{"mainProgramType":"local_agent"}}}`,
				`{"success":true,"data":null}`,
			},
			want: "尚未发布",
		},
		{
			name: "published service error",
			responses: []string{
				`{"success":true,"data":{"digitalTagEmployeeProfile":{"mainProgramType":"local_agent"}}}`,
				`{"success":false,"errorMsg":"permission denied"}`,
			},
			want: "尚未发布",
		},
		{
			name: "published profile missing corp id",
			responses: []string{
				`{"success":true,"data":{"digitalTagEmployeeProfile":{"mainProgramType":"local_agent"}}}`,
				`{"success":true,"data":{"status":"online"}}`,
			},
			want: "缺少 profile.corpId",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{
				"deap-dev/get_digital_employee_detail": append([]string(nil), tc.responses...),
			}}
			InitDepsForTest(t, caller)
			setupConnectSupervisorSeams(t)
			exchanged := false
			testseam.Swap(t, &deapConnectManagedExchange, func(context.Context, string, auth.ManagedExchangeRequest) (*auth.TokenData, error) {
				exchanged = true
				return nil, errors.New("unexpected exchange")
			})
			leaf := newConnectTestCommand(t, false)
			if err := leaf.RunE(leaf, nil); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("connect error = %v, want %q", err, tc.want)
			}
			if exchanged {
				t.Fatal("invalid prerequisite reached managed exchange")
			}
		})
	}
}

func TestDingTalkTagConnectFailureBoundaries(t *testing.T) {
	t.Run("authorization missing fields", func(t *testing.T) {
		caller := newSuccessfulConnectCaller(`{"success":true,"data":{"uid":"employee-user","dwsAuthCode":"secret","orgId":"employee-corp"}}`,
			`{"result":[{"userId":"supervisor-user","openDingTalkId":"operator-open"}]}`)
		InitDepsForTest(t, caller)
		setupConnectSupervisorSeams(t)
		leaf := newConnectTestCommand(t, false)
		if err := leaf.RunE(leaf, nil); err == nil || !strings.Contains(err.Error(), "授权响应缺少") {
			t.Fatalf("missing auth fields error = %v", err)
		}
	})

	t.Run("profile persistence failure", func(t *testing.T) {
		caller := newSuccessfulConnectCaller(successfulAuthResponse(),
			`{"result":[{"userId":"supervisor-user","openDingTalkId":"operator-open"}]}`)
		InitDepsForTest(t, caller)
		setupConnectSupervisorSeams(t)
		testseam.Swap(t, &deapConnectManagedExchange, func(context.Context, string, auth.ManagedExchangeRequest) (*auth.TokenData, error) {
			return nil, errors.New("profile persistence failed")
		})
		leaf := newConnectTestCommand(t, false)
		if err := leaf.RunE(leaf, nil); err == nil || !strings.Contains(err.Error(), "profile persistence failed") {
			t.Fatalf("profile persistence error = %v", err)
		}
		if len(caller.calls) != 3 {
			t.Fatalf("profile failure should stop before contact: %#v", caller.calls)
		}
	})

	t.Run("managed identity lookup failure", func(t *testing.T) {
		caller := newSuccessfulConnectCaller(successfulAuthResponse(),
			`{"result":[{"userId":"supervisor-user","openDingTalkId":"operator-open"}]}`)
		caller.responses["contact/get_current_user_profile"] = []string{
			`{"result":[{"orgEmployeeModel":{"corpId":"other-corp","userId":"employee-user"}}]}`,
		}
		InitDepsForTest(t, caller)
		setupConnectSupervisorSeams(t)
		testseam.Swap(t, &deapConnectManagedExchange, func(ctx context.Context, _ string, request auth.ManagedExchangeRequest) (*auth.TokenData, error) {
			if request.ResolveIdentity == nil {
				return nil, errors.New("managed identity resolver is missing")
			}
			_, err := request.ResolveIdentity(ctx, "managed-access-secret", request.ExpectedCorpID)
			return nil, err
		})
		leaf := newConnectTestCommand(t, false)
		if err := leaf.RunE(leaf, nil); err == nil || !strings.Contains(err.Error(), "identity") {
			t.Fatalf("identity lookup error = %v", err)
		}
		if len(caller.tokenCalls) != 1 || caller.tokenCalls[0].productID != "contact" || caller.tokenCalls[0].toolName != "get_current_user_profile" {
			t.Fatalf("identity lookup calls = %#v", caller.tokenCalls)
		}
	})

	t.Run("operator cannot be resolved uniquely", func(t *testing.T) {
		caller := newSuccessfulConnectCaller(successfulAuthResponse(),
			`{"result":[{"userId":"supervisor-user","openDingTalkId":"operator-one"},{"userId":"supervisor-user","openDingTalkId":"operator-two"}]}`)
		InitDepsForTest(t, caller)
		setupSuccessfulConnectSeams(t)
		leaf := newConnectTestCommand(t, false)
		if err := leaf.RunE(leaf, nil); err == nil || !strings.Contains(err.Error(), "唯一 operatorOpenDingTalkId") {
			t.Fatalf("operator resolution error = %v", err)
		}
	})

	t.Run("operator search result must match exact supervisor user id", func(t *testing.T) {
		caller := newSuccessfulConnectCaller(successfulAuthResponse(),
			`{"result":[{"userId":"different-user","openDingTalkId":"operator-open"}]}`)
		InitDepsForTest(t, caller)
		setupSuccessfulConnectSeams(t)
		leaf := newConnectTestCommand(t, false)
		if err := leaf.RunE(leaf, nil); err == nil || !strings.Contains(err.Error(), "唯一 operatorOpenDingTalkId") {
			t.Fatalf("operator exact-match error = %v", err)
		}
	})

	t.Run("DSH registration failure is safely retryable", func(t *testing.T) {
		caller := newSuccessfulConnectCaller(successfulAuthResponse(),
			`{"result":[{"userId":"supervisor-user","openDingTalkId":"operator-open"}]}`)
		InitDepsForTest(t, caller)
		setupSuccessfulConnectSeams(t)
		saved := false
		testseam.Swap(t, &deapConnectSaveBinding, func(string, digitalEmployeeBinding) error {
			saved = true
			return nil
		})
		testseam.Swap(t, &deapConnectRegisterDSH, func(context.Context, map[string]any) (string, error) {
			return "", errors.New("dsh unavailable")
		})
		leaf := newConnectTestCommand(t, false)
		err := leaf.RunE(leaf, nil)
		if err == nil || !strings.Contains(err.Error(), "employee-corp:employee-user") || !strings.Contains(err.Error(), "connect restart --agent-uuid agent-1") {
			t.Fatalf("DSH retry error = %v", err)
		}
		if !saved {
			t.Fatal("DSH failure did not preserve the local binding")
		}
	})
}

func TestDingTalkTagConnectRejectsAuthorizationIdentityMismatchBeforeExchange(t *testing.T) {
	tests := []struct {
		name          string
		authorization string
		want          string
	}{
		{
			name:          "robot uid mismatch",
			authorization: `{"success":true,"data":{"dwsClientId":"returned-client","uid":"other-robot","staffId":"employee-user","dwsAuthCode":"one-time-secret","orgId":"439446171"}}`,
			want:          "uid 与发布详情 profile.robotUid 不一致",
		},
		{
			name:          "staff id mismatch",
			authorization: `{"success":true,"data":{"dwsClientId":"returned-client","uid":"robot-uid","staffId":"other-user","dwsAuthCode":"one-time-secret","orgId":"439446171"}}`,
			want:          "staffId 与发布详情 profile.staffId 不一致",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := newSuccessfulConnectCaller(tc.authorization,
				`{"result":[{"userId":"supervisor-user","openDingTalkId":"operator-open"}]}`)
			InitDepsForTest(t, caller)
			setupConnectSupervisorSeams(t)
			exchanged := false
			testseam.Swap(t, &deapConnectManagedExchange, func(context.Context, string, auth.ManagedExchangeRequest) (*auth.TokenData, error) {
				exchanged = true
				return nil, errors.New("unexpected exchange")
			})
			leaf := newConnectTestCommand(t, false)
			if err := leaf.RunE(leaf, nil); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("connect error = %v, want %q", err, tc.want)
			}
			if exchanged {
				t.Fatal("authorization identity mismatch reached managed exchange")
			}
		})
	}
}

func newSuccessfulConnectCaller(authResponse, contactResponse string) *digitalEmployeeProtocolCaller {
	return &digitalEmployeeProtocolCaller{responses: map[string][]string{
		"deap-dev/bind_local_agent": {`{"success":true,"data":"binding-created"}`},
		"deap-dev/get_digital_employee_detail": {
			`{"success":true,"data":{"name":"本地员工","digitalTagEmployeeProfile":{"mainProgramType":"local_agent"}}}`,
			`{"success":true,"data":{"status":"online","profile":{"corpId":"employee-corp","robotUid":"robot-uid","staffId":"employee-user"}}}`,
		},
		"deap-dev/get_dws_auth_code":         {authResponse},
		"contact/search_contact_by_key_word": {contactResponse},
	}}
}

func successfulAuthResponse() string {
	return `{"success":true,"data":{"dwsClientId":"returned-client","uid":"robot-uid","staffId":"employee-user","dwsAuthCode":"one-time-secret","orgId":"439446171"}}`
}

func setupConnectSupervisorSeams(t *testing.T) {
	t.Helper()
	auth.SetRuntimeProfile("")
	t.Cleanup(func() { auth.SetRuntimeProfile("") })
	configDir := t.TempDir()
	testseam.Swap(t, &deapConnectConfigDir, func() string { return configDir })
	testseam.Swap(t, &deapConnectLoadProfiles, func(string) (*auth.ProfilesConfig, error) {
		return &auth.ProfilesConfig{CurrentProfile: "supervisor-corp:supervisor-user"}, nil
	})
	testseam.Swap(t, &deapConnectLoadToken, func(string, string) (*auth.TokenData, error) {
		return &auth.TokenData{CorpID: "supervisor-corp", UserID: "supervisor-user", AccessToken: "supervisor-test-token"}, nil
	})
	testseam.Swap(t, &deapConnectLoadSupervisorToken, func(context.Context, string) (*auth.TokenData, error) {
		return &auth.TokenData{CorpID: "supervisor-corp", UserID: "supervisor-user", AccessToken: "supervisor-test-token"}, nil
	})
}

func setupSuccessfulConnectSeams(t *testing.T) {
	t.Helper()
	setupConnectSupervisorSeams(t)
	testseam.Swap(t, &deapConnectManagedExchange, func(_ context.Context, _ string, request auth.ManagedExchangeRequest) (*auth.TokenData, error) {
		return &auth.TokenData{AccessToken: "managed-access-secret", CorpID: request.ExpectedCorpID, UserID: request.ExpectedUserID, ClientID: request.ClientID, Source: "mcp"}, nil
	})
	testseam.Swap(t, &deapConnectSaveBinding, func(string, digitalEmployeeBinding) error { return nil })
	testseam.Swap(t, &deapConnectRegisterDSH, func(context.Context, map[string]any) (string, error) { return "created", nil })
}

func newConnectTestCommand(t *testing.T, dryRun bool) *cobra.Command {
	return newConnectTestCommandWithMode(t, dryRun, "dsh", false)
}

func newProfileOnlyConnectTestCommand(t *testing.T, dryRun bool) *cobra.Command {
	return newConnectTestCommandWithMode(t, dryRun, "", true)
}

func newConnectTestCommandWithMode(t *testing.T, dryRun bool, channel string, profileOnly bool) *cobra.Command {
	t.Helper()
	root := deapHandler{}.Command(&captureRunner{})
	root.PersistentFlags().Bool("yes", false, "test confirmation")
	root.PersistentFlags().Bool("dry-run", false, "test dry-run")
	leaf, _, err := root.Find([]string{"connect"})
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{"agent-uuid": "agent-1", "yes": "true"}
	if channel != "" {
		values["channel"] = channel
	}
	if profileOnly {
		values["profile-only"] = "true"
	}
	for name, value := range values {
		flags := leaf.Flags()
		if flags.Lookup(name) == nil {
			flags = leaf.InheritedFlags()
		}
		if err := flags.Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	if dryRun {
		if err := leaf.InheritedFlags().Set("dry-run", "true"); err != nil {
			t.Fatal(err)
		}
	}
	return leaf
}

func TestDingTalkTagConnectKeepsSupervisorCurrentAndUsesReturnedClientID(t *testing.T) {
	testseam.Swap(t, &employeeDSHControl, func(context.Context, digitalEmployeeBinding, string) (employeeDSHState, error) {
		return employeeDSHState{}, fmt.Errorf("host unavailable")
	})
	caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{
		"deap-dev/bind_local_agent": {`{"success":true,"data":"binding-created"}`},
		"deap-dev/get_digital_employee_detail": {
			`{"success":true,"data":{"name":"本地员工","digitalTagEmployeeProfile":{"mainProgramType":"local_agent"}}}`,
			`{"success":true,"data":{"status":"online","profile":{"corpId":"employee-corp","robotUid":"robot-uid","staffId":"employee-user"}}}`,
		},
		"deap-dev/get_dws_auth_code":         {`{"success":true,"data":{"dwsClientId":"returned-client","uid":"robot-uid","staffId":"employee-user","dwsAuthCode":"one-time-secret","orgId":"439446171"}}`},
		"contact/search_contact_by_key_word": {`{"result":[{"userId":"supervisor-user","openDingTalkId":"operator-supervisor-scope"}]}`},
	}, tokenResponses: map[string][]string{
		"contact/get_current_user_profile":   {`{"result":[{"orgEmployeeModel":{"corpId":"employee-corp","orgName":"员工企业","userId":"employee-user","orgUserName":"本地员工"}}]}`},
		"contact/search_contact_by_key_word": {`{"result":[{"userId":"supervisor-user","openDingTalkId":"operator-employee-scope"}]}`},
	}}
	InitDepsForTest(t, caller)
	t.Setenv("DWS_DUMP_RAW", "1")
	auth.SetRuntimeProfile("")
	t.Cleanup(func() { auth.SetRuntimeProfile("") })
	configDir := t.TempDir()
	testseam.Swap(t, &deapConnectConfigDir, func() string { return configDir })
	testseam.Swap(t, &deapConnectLoadProfiles, func(string) (*auth.ProfilesConfig, error) {
		return &auth.ProfilesConfig{CurrentProfile: "supervisor-corp:supervisor-user"}, nil
	})
	testseam.Swap(t, &deapConnectLoadToken, func(_ string, selector string) (*auth.TokenData, error) {
		if selector != "supervisor-corp:supervisor-user" {
			t.Fatalf("supervisor selector = %q", selector)
		}
		return &auth.TokenData{CorpID: "supervisor-corp", UserID: "supervisor-user", AccessToken: "supervisor-test-token"}, nil
	})
	testseam.Swap(t, &deapConnectLoadSupervisorToken, func(context.Context, string) (*auth.TokenData, error) {
		return &auth.TokenData{CorpID: "supervisor-corp", UserID: "supervisor-user", AccessToken: "supervisor-test-token"}, nil
	})
	var exchange auth.ManagedExchangeRequest
	testseam.Swap(t, &deapConnectManagedExchange, func(ctx context.Context, _ string, request auth.ManagedExchangeRequest) (*auth.TokenData, error) {
		exchange = request
		if request.ExpectedCorpID != "employee-corp" {
			return nil, fmt.Errorf("managed exchange expected organization = %q, want published corpId", request.ExpectedCorpID)
		}
		if request.ExpectedUserID != "employee-user" {
			return nil, fmt.Errorf("managed exchange expected user = %q, want authorization staffId", request.ExpectedUserID)
		}
		if request.ResolveIdentity == nil {
			return nil, errors.New("managed identity resolver is missing")
		}
		if request.ExpectedCorpID != "employee-corp" || request.ExpectedUserID != "employee-user" {
			return nil, errors.New("managed exchange received wrong published identity")
		}
		identity, err := request.ResolveIdentity(ctx, "managed-access-secret", request.ExpectedCorpID)
		if err != nil {
			return nil, err
		}
		return &auth.TokenData{
			AccessToken: "managed-access-secret",
			CorpID:      identity.CorpID, CorpName: identity.CorpName,
			UserID: identity.UserID, UserName: identity.UserName,
			ClientID: request.ClientID, Source: "mcp",
		}, nil
	})
	var registration map[string]any
	testseam.Swap(t, &deapConnectSaveBinding, func(_ string, binding digitalEmployeeBinding) error {
		if binding.DWSProfile != "employee-corp:employee-user" || binding.OperatorOpenDingTalkID != "operator-employee-scope" {
			t.Fatalf("binding = %#v", binding)
		}
		return nil
	})
	testseam.Swap(t, &deapConnectRegisterDSH, func(_ context.Context, input map[string]any) (string, error) {
		registration = input
		return "created", nil
	})

	root := deapHandler{}.Command(&captureRunner{})
	root.PersistentFlags().Bool("yes", false, "test confirmation")
	root.PersistentFlags().Bool("dry-run", false, "test dry-run")
	leaf, _, err := root.Find([]string{"connect"})
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"agent-uuid": "agent-1", "channel": "dsh", "client-id": "requested-hint", "yes": "true"} {
		flags := leaf.Flags()
		if flags.Lookup(name) == nil {
			flags = leaf.InheritedFlags()
		}
		if err := flags.Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	var output bytes.Buffer
	var stderr bytes.Buffer
	leaf.SetOut(&output)
	leaf.SetErr(&stderr)
	if err := leaf.RunE(leaf, nil); err != nil {
		t.Fatalf("connect RunE() error = %v", err)
	}
	if exchange.ClientID != "returned-client" || exchange.AuthCode != "one-time-secret" || exchange.PreserveProfile != "supervisor-corp:supervisor-user" {
		t.Fatalf("managed exchange = %#v", exchange)
	}
	if len(caller.tokenCalls) != 3 || len(caller.tokens) != 3 || caller.tokens[2] != "supervisor-test-token" || caller.tokenCalls[2].toolName != "bind_local_agent" ||
		caller.tokens[0] != "managed-access-secret" || caller.tokens[1] != "managed-access-secret" ||
		caller.tokenCalls[0].productID != "contact" || caller.tokenCalls[0].toolName != "get_current_user_profile" ||
		caller.tokenCalls[1].productID != "contact" || caller.tokenCalls[1].toolName != "search_contact_by_key_word" ||
		caller.tokenCalls[1].args["keyword"] != "supervisor-user" {
		t.Fatalf("managed identity lookup calls=%#v tokens=%#v", caller.tokenCalls, caller.tokens)
	}
	for _, call := range caller.calls {
		if call.productID == "contact" && call.toolName == "search_contact_by_key_word" {
			t.Fatalf("operator lookup used supervisor-profile MCP caller: %#v", call)
		}
	}
	if auth.RuntimeProfile() != "" {
		t.Fatalf("connect changed runtime profile to %q", auth.RuntimeProfile())
	}
	if registration["dwsProfile"] != "employee-corp:employee-user" || registration["operatorOpenDingTalkId"] != "operator-employee-scope" {
		t.Fatalf("DSH registration = %#v", registration)
	}
	combined := output.String() + stderr.String()
	for _, secret := range []string{"one-time-secret", "returned-client", "managed-access-secret"} {
		if strings.Contains(combined, secret) {
			t.Fatalf("connect leaked %q: %s", secret, combined)
		}
	}
	var envelope struct {
		OK   bool                         `json:"ok"`
		Data digitalEmployeeConnectResult `json:"data"`
	}
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK || envelope.Data.Status != "created" || envelope.Data.DWSProfile != "employee-corp:employee-user" || !envelope.Data.RestartRequired {
		t.Fatalf("connect envelope = %#v", envelope)
	}
}

func TestDingTalkTagOperatorPrivateRejectsTargetDifferentFromConnectBinding(t *testing.T) {
	caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{}}
	InitDepsForTest(t, caller)
	auth.SetRuntimeProfile("employee-corp:employee-user")
	t.Cleanup(func() { auth.SetRuntimeProfile("") })
	testseam.Swap(t, &deapChannelLoadBinding, func(string, string) (digitalEmployeeBinding, error) {
		return digitalEmployeeBinding{
			SchemaVersion: 1, AgentUUID: "agent-1", DWSProfile: "employee-corp:employee-user", OperatorOpenDingTalkID: "fixed-operator",
		}, nil
	})
	root := deapHandler{}.Command(&captureRunner{})
	leaf, _, err := root.Find([]string{"channel", "operator-private"})
	if err != nil {
		t.Fatal(err)
	}
	_ = leaf.Flags().Set("channel", "dsh")
	_ = leaf.Flags().Set("stdin", "true")
	leaf.SetIn(strings.NewReader(`{"schemaVersion":1,"protocolVersion":1,"agentUuid":"agent-1","operatorOpenDingTalkId":"attacker-target","text":"不应发送","idempotencyKey":"idem-1"}`))
	if err := leaf.RunE(leaf, nil); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mismatched operator error = %v", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("mismatched operator made MCP call: %#v", caller.calls)
	}
}
