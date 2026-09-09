package mock_mcp_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
)

const (
	mockMCPSmokeHelperEnv       = "DWS_MOCK_MCP_SMOKE_HELPER"
	mockMCPSmokeCAEnv           = "DWS_MOCK_MCP_SMOKE_CA_FILE"
	mockMCPSmokeDownloadDialEnv = "DWS_MOCK_MCP_SMOKE_DOWNLOAD_DIAL"
	mockCurrentDOpenID          = "DAAAAAAAAAAAiE"
	// helper 会启动第二个启用 race 的测试进程。保持有界，同时为 remaining
	// shard 与其他 CI race 包并行时的调度抖动预留空间。
	mockMCPCLIExecutionTimeout = 30 * time.Second
)

type recordedToolCall struct {
	path          string
	method        string
	authorization string
	jsonrpc       string
	tool          string
	arguments     map[string]any
	err           error
}

// TestCLIHelperProcess runs the production CLI entrypoint with real os.Args.
// The parent test supplies only isolated temp directories, a loopback endpoint,
// and a synthetic token accepted by the local fake server.
func TestCLIHelperProcess(t *testing.T) {
	if os.Getenv(mockMCPSmokeHelperEnv) != "1" {
		return
	}
	if caFile := strings.TrimSpace(os.Getenv(mockMCPSmokeCAEnv)); caFile != "" {
		certificatePEM, err := os.ReadFile(caFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Mock MCP smoke helper: read CA: %v\n", err)
			os.Exit(2)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(certificatePEM) {
			fmt.Fprintln(os.Stderr, "Mock MCP smoke helper: invalid CA file")
			os.Exit(2)
		}
		_ = os.Setenv("GODEBUG", "x509usefallbackroots=1")
		x509.SetFallbackRoots(pool)
	}
	if dialAddr := strings.TrimSpace(os.Getenv(mockMCPSmokeDownloadDialEnv)); dialAddr != "" {
		localio.SetSecureDownloadDialTargetForTest(dialAddr)
	}

	marker := -1
	for i, arg := range os.Args {
		if arg == "--" {
			marker = i
			break
		}
	}
	if marker < 0 {
		fmt.Fprintln(os.Stderr, "Mock MCP smoke helper: missing -- argument marker")
		os.Exit(2)
	}
	os.Args = append([]string{"dws"}, os.Args[marker+1:]...)
	os.Exit(app.Execute())
}

func TestMockMCPSmoke_CLIRoutesSerializedArgumentsAndPrintsJSON(t *testing.T) {
	var requestsMu sync.Mutex
	var requests []recordedToolCall
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := recordedToolCall{
			path:          r.URL.Path,
			method:        r.Method,
			authorization: r.Header.Get("Authorization"),
		}

		var envelope struct {
			JSONRPC string `json:"jsonrpc"`
			ID      int    `json:"id"`
			Method  string `json:"method"`
			Params  struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
			call.err = err
		} else {
			call.jsonrpc = envelope.JSONRPC
			call.tool = envelope.Params.Name
			call.arguments = envelope.Params.Arguments
			if envelope.Method != "tools/call" {
				call.err = fmt.Errorf("JSON-RPC method = %q, want tools/call", envelope.Method)
			}
		}
		requestsMu.Lock()
		requests = append(requests, call)
		requestsMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      envelope.ID,
			"result": map[string]any{
				"content": []map[string]any{{
					"type": "text",
					"text": `{"success":true,"result":[{"userId":"mock-user-1","name":"Local Mock"}]}`,
				}},
			},
		})
	}))
	defer server.Close()

	env := isolatedCLIEnv(t, map[string]string{
		"DINGTALK_CONTACT_MCP_URL": server.URL + "/mcp/contact",
	})
	args := []string{
		"--token", "ci-smoke-token",
		"--format", "json",
		"contact", "user", "get",
		"--ids", "user-001,user-002",
	}
	stdout, stderr, err := runCLI(t, env, args...)
	if err != nil {
		t.Fatalf("dws %s failed: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, stdout, stderr)
	}

	requestsMu.Lock()
	recorded := append([]recordedToolCall(nil), requests...)
	requestsMu.Unlock()
	if len(recorded) != 1 {
		t.Fatalf("local fake MCP server received %d requests, want exactly one tools/call: %#v", len(recorded), recorded)
	}
	call := recorded[0]
	if call.err != nil {
		t.Fatal(call.err)
	}
	if call.path != "/mcp/contact" {
		t.Fatalf("request path = %q, want /mcp/contact", call.path)
	}
	if call.method != http.MethodPost {
		t.Fatalf("HTTP method = %q, want POST", call.method)
	}
	if call.authorization != "Bearer ci-smoke-token" {
		t.Fatalf("Authorization = %q, want synthetic smoke token", call.authorization)
	}
	if call.jsonrpc != "2.0" {
		t.Fatalf("jsonrpc = %q, want 2.0", call.jsonrpc)
	}
	if call.tool != "get_user_info_by_user_ids" {
		t.Fatalf("tool = %q, want get_user_info_by_user_ids", call.tool)
	}
	wantArgs := map[string]any{"user_id_list": []any{"user-001", "user-002"}}
	if !reflect.DeepEqual(call.arguments, wantArgs) {
		t.Fatalf("arguments = %#v, want %#v", call.arguments, wantArgs)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("CLI returned non-JSON stdout: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if payload["success"] != true {
		t.Fatalf("CLI success = %#v, want true; payload=%#v", payload["success"], payload)
	}
	result, ok := payload["result"].([]any)
	if !ok || len(result) != 1 {
		t.Fatalf("CLI result = %#v, want one mock user", payload["result"])
	}
	user, _ := result[0].(map[string]any)
	if user["userId"] != "mock-user-1" {
		t.Fatalf("CLI userId = %#v, want mock-user-1; payload=%#v", user["userId"], payload)
	}
}

func TestMultiIME2E_NaturalTargetsCompletenessAndWriteBoundaries(t *testing.T) {
	var requestsMu sync.Mutex
	var requests []recordedToolCall
	var uploadedBody []byte
	download := newTrustedDownloadFixture(t, []byte("downloaded-resource-body"))
	authConfigDir := t.TempDir()
	authKeychainDir := filepath.Join(t.TempDir(), "keychain")
	t.Setenv("DWS_CONFIG_DIR", authConfigDir)
	t.Setenv("DWS_KEYCHAIN_DIR", authKeychainDir)
	t.Setenv("DWS_DISABLE_KEYCHAIN", "1")
	if err := authpkg.SaveTokenData(authConfigDir, &authpkg.TokenData{
		AccessToken:  "ci-smoke-token",
		RefreshToken: "ci-refresh-token",
		ExpiresAt:    time.Now().Add(time.Hour),
		RefreshExpAt: time.Now().Add(2 * time.Hour),
		ClientID:     "ci-client",
		CorpID:       "ci-corp",
		UserID:       "ci-user",
	}); err != nil {
		t.Fatalf("save isolated event auth: %v", err)
	}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/upload/conversation" {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			requestsMu.Lock()
			uploadedBody = append([]byte(nil), body...)
			requestsMu.Unlock()
			w.WriteHeader(http.StatusOK)
			return
		}
		var envelope struct {
			JSONRPC string `json:"jsonrpc"`
			ID      int    `json:"id"`
			Method  string `json:"method"`
			Params  struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		decodeErr := json.NewDecoder(r.Body).Decode(&envelope)
		call := recordedToolCall{
			path:          r.URL.Path,
			method:        r.Method,
			authorization: r.Header.Get("Authorization"),
			jsonrpc:       envelope.JSONRPC,
			tool:          envelope.Params.Name,
			arguments:     envelope.Params.Arguments,
			err:           decodeErr,
		}
		requestsMu.Lock()
		requests = append(requests, call)
		requestsMu.Unlock()

		response := multiIMMockResponse(
			envelope.Params.Name,
			envelope.Params.Arguments,
			server.URL,
			download.URL,
		)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      envelope.ID,
			"result": map[string]any{
				"content": []map[string]any{{"type": "text", "text": response}},
			},
		})
	}))
	defer server.Close()

	env := isolatedCLIEnv(t, map[string]string{
		"DINGTALK_CONTACT_MCP_URL":  server.URL + "/mcp/contact",
		"DINGTALK_CHAT_MCP_URL":     server.URL + "/mcp/chat",
		"DINGTALK_IM_MCP_URL":       server.URL + "/mcp/im",
		mockMCPSmokeDownloadDialEnv: download.DialAddr,
		"SSL_CERT_FILE":             download.CAFile,
		mockMCPSmokeCAEnv:           download.CAFile,
		"DWS_CONFIG_DIR":            authConfigDir,
		"DWS_KEYCHAIN_DIR":          authKeychainDir,
	})
	reset := func() {
		requestsMu.Lock()
		requests = nil
		uploadedBody = nil
		requestsMu.Unlock()
	}
	snapshot := func() []recordedToolCall {
		requestsMu.Lock()
		defer requestsMu.Unlock()
		return append([]recordedToolCall(nil), requests...)
	}

	t.Run("natural user advanced send resolves then writes once", func(t *testing.T) {
		reset()
		stdout, stderr, err := runCLI(t, env,
			"--token", "ci-smoke-token", "--format", "json",
			"chat", "+messages-send", "--as", "user",
			"--user-query", "测试用户甲", "--text", "你好", "--yes",
		)
		if err != nil {
			t.Fatalf("natural send failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
		}
		calls := snapshot()
		if len(calls) != 2 || calls[0].tool != "search_contact_by_key_word" || calls[1].tool != "send_personal_message" {
			t.Fatalf("calls = %#v, want resolve + one send", calls)
		}
		if calls[1].arguments["receiverOpenDingTalkId"] != mockCurrentDOpenID {
			t.Fatalf("send target = %#v", calls[1].arguments)
		}
		assertMultiIMCallsAreLocalAndAuthorized(t, calls)
	})

	t.Run("natural group file send uploads commits and writes once", func(t *testing.T) {
		reset()
		workdir := t.TempDir()
		fileBody := []byte("multi-im-file-upload")
		if err := os.WriteFile(filepath.Join(workdir, "report.txt"), fileBody, 0o600); err != nil {
			t.Fatal(err)
		}
		stdout, stderr, err := runCLIInDir(t, env, workdir,
			"--token", "ci-smoke-token", "--format", "json",
			"chat", "+messages-send", "--as", "user",
			"--chat-query", "项目群", "--file", "./report.txt",
			"--idempotency-key", "file-key", "--yes",
		)
		if err != nil {
			t.Fatalf("file send failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
		}
		calls := snapshot()
		wantTools := []string{
			"search_groups",
			"init_conversation_file_upload",
			"commit_conversation_file_upload",
			"send_personal_message",
		}
		if got := recordedToolNames(calls); !reflect.DeepEqual(got, wantTools) {
			t.Fatalf("file send tools = %#v, want %#v", got, wantTools)
		}
		requestsMu.Lock()
		gotUpload := append([]byte(nil), uploadedBody...)
		requestsMu.Unlock()
		if !bytes.Equal(gotUpload, fileBody) {
			t.Fatalf("uploaded body = %q, want %q", gotUpload, fileBody)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
			t.Fatalf("file send output is not JSON: %v\n%s", err, stdout)
		}
		if payload["ok"] != true || payload["effectiveMessageType"] != "file" {
			t.Fatalf("file send result = %#v", payload)
		}
	})

	t.Run("ambiguous user has zero write side effects", func(t *testing.T) {
		reset()
		_, _, err := runCLI(t, env,
			"--token", "ci-smoke-token", "--format", "json",
			"chat", "+messages-send", "--as", "user",
			"--user-query", "同名用户", "--text", "不能发", "--yes",
		)
		if err == nil {
			t.Fatal("ambiguous user unexpectedly succeeded")
		}
		calls := snapshot()
		if len(calls) != 1 || calls[0].tool != "search_contact_by_key_word" {
			t.Fatalf("ambiguous calls = %#v, want resolver only", calls)
		}
	})

	t.Run("natural group resource read downloads atomically", func(t *testing.T) {
		reset()
		workdir := t.TempDir()
		stdout, stderr, err := runCLIInDir(t, env, workdir,
			"--token", "ci-smoke-token", "--format", "json",
			"chat", "+chat-messages", "--chat-query", "资源群",
			"--download-resources", "--output-dir", "./downloads",
		)
		if err != nil {
			t.Fatalf("resource read failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
		}
		calls := snapshot()
		wantTools := []string{"search_groups", "list_conversation_message_v2", "get_resource_download_url"}
		if got := recordedToolNames(calls); !reflect.DeepEqual(got, wantTools) {
			t.Fatalf("resource tools = %#v, want %#v", got, wantTools)
		}
		downloaded, err := os.ReadFile(filepath.Join(workdir, "downloads", "artifact.bin"))
		if err != nil {
			t.Fatalf("read downloaded resource: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
		}
		if string(downloaded) != "downloaded-resource-body" {
			t.Fatalf("downloaded resource = %q", downloaded)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
			t.Fatalf("resource output is not JSON: %v\n%s", err, stdout)
		}
		ledger, _ := payload["resourceDownloads"].(map[string]any)
		if payload["complete"] != true || ledger["downloadedCount"] != float64(1) || ledger["failedCount"] != float64(0) {
			t.Fatalf("resource completion = %#v", payload)
		}
	})

	t.Run("search later page failure preserves results and marks incomplete", func(t *testing.T) {
		reset()
		stdout, stderr, err := runCLI(t, env,
			"--token", "ci-smoke-token", "--format", "json",
			"chat", "+search-msg", "--query", "分页失败",
			"--page-all", "--no-enrich",
		)
		if err != nil {
			t.Fatalf("partial search failed as a command: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
		}
		calls := snapshot()
		if got := recordedToolNames(calls); !reflect.DeepEqual(got, []string{"search_messages", "search_messages"}) {
			t.Fatalf("search tools = %#v", got)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
			t.Fatalf("search output is not JSON: %v\n%s", err, stdout)
		}
		if payload["complete"] != false || payload["count"] != float64(1) ||
			payload["pagesFetched"] != float64(1) || payload["failedCount"] != float64(1) {
			t.Fatalf("partial search contract = %#v", payload)
		}
		failures, _ := payload["failures"].([]any)
		if len(failures) != 1 || failures[0].(map[string]any)["stage"] != "search-page" {
			t.Fatalf("partial search failures = %#v", failures)
		}
	})

	t.Run("event listen im process dry-run resolves and creates no subscription", func(t *testing.T) {
		reset()
		stdout, stderr, err := runCLI(t, env,
			"--token", "ci-smoke-token", "--client-id", "ci-client",
			"event", "+listen-im", "--kind", "group",
			"--events", "message,reaction", "--chat-query", "项目群",
			"--dry-run",
		)
		if err != nil {
			t.Fatalf("event dry-run failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
		}
		calls := snapshot()
		if got := recordedToolNames(calls); !reflect.DeepEqual(got, []string{"search_groups"}) {
			t.Fatalf("event dry-run tools = %#v", got)
		}
		if !strings.Contains(stderr, "subscription[0]") || !strings.Contains(stderr, "subscription[1]") ||
			!strings.Contains(stderr, "user_im_message_receive_group") ||
			!strings.Contains(stderr, "user_im_message_reaction_group") {
			t.Fatalf("event dry-run plan missing:\nstdout=%s\nstderr=%s", stdout, stderr)
		}
	})

	t.Run("natural group read returns complete shared message contract", func(t *testing.T) {
		reset()
		stdout, stderr, err := runCLI(t, env,
			"--token", "ci-smoke-token", "--format", "json",
			"chat", "+chat-messages", "--chat-query", "项目群",
		)
		if err != nil {
			t.Fatalf("natural read failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
		}
		calls := snapshot()
		if len(calls) != 2 || calls[0].tool != "search_groups" || calls[1].tool != "list_conversation_message_v2" {
			t.Fatalf("read calls = %#v", calls)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
			t.Fatalf("read output is not JSON: %v\n%s", err, stdout)
		}
		if payload["contractVersion"] != "im.message-list.v1" || payload["complete"] != true || payload["failedCount"] != float64(0) {
			t.Fatalf("message contract = %#v", payload)
		}
		messages, _ := payload["messages"].([]any)
		if len(messages) != 1 {
			t.Fatalf("messages = %#v", payload["messages"])
		}
		message, _ := messages[0].(map[string]any)
		if message["messageId"] != "msg-1" || message["senderId"] != mockCurrentDOpenID || message["senderType"] != "user" {
			t.Fatalf("projected message = %#v", message)
		}
	})

	t.Run("natural member create dry-run resolves but never creates", func(t *testing.T) {
		reset()
		stdout, stderr, err := runCLI(t, env,
			"--token", "ci-smoke-token", "--format", "json",
			"chat", "+chat-create", "--name", "新群",
			"--member-query", "测试用户甲", "--dry-run", "--yes",
		)
		if err != nil {
			t.Fatalf("chat-create dry-run failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
		}
		calls := snapshot()
		if len(calls) != 2 || calls[0].tool != "search_contact_by_key_word" || calls[1].tool != "get_current_user_profile" {
			t.Fatalf("dry-run calls = %#v, want resolver/profile and no create", calls)
		}
		for _, call := range calls {
			if call.tool == "create_group_conversation" {
				t.Fatalf("dry-run created a group: %#v", calls)
			}
		}
	})

	t.Run("reply returns continuation context", func(t *testing.T) {
		reset()
		stdout, stderr, err := runCLI(t, env,
			"--token", "ci-smoke-token", "--format", "json",
			"chat", "+messages-reply",
			"--conversation-id", "cid-1", "--message-id", "msg-1",
			"--ref-sender", mockCurrentDOpenID, "--text", "收到",
			"--idempotency-key", "reply-key", "--yes",
		)
		if err != nil {
			t.Fatalf("reply failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
		}
		calls := snapshot()
		if len(calls) != 1 || calls[0].tool != "send_personal_message" {
			t.Fatalf("reply calls = %#v", calls)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
			t.Fatalf("reply output is not JSON: %v\n%s", err, stdout)
		}
		if payload["contractVersion"] != "im.message-reply.v1" || payload["messageId"] != "sent-msg-1" ||
			payload["conversationId"] != "cid-1" || payload["idempotencyKey"] != "reply-key" {
			t.Fatalf("reply continuation = %#v", payload)
		}
	})
}

func multiIMMockResponse(tool string, arguments map[string]any, mcpBaseURL, resourceURL string) string {
	switch tool {
	case "search_contact_by_key_word":
		if arguments["keyword"] == "同名用户" {
			return `{"result":[{"name":"同名用户","userId":"u1","openDingTalkId":"D1"},{"name":"同名用户","userId":"u2","openDingTalkId":"D2"}]}`
		}
		return `{"result":[{"name":"测试用户甲","userId":"user-1","openDingTalkId":"` + mockCurrentDOpenID + `"}]}`
	case "search_groups":
		if arguments["keyword"] == "资源群" {
			return `{"result":[{"title":"资源群","openConversationId":"cid-resources"}]}`
		}
		return `{"result":[{"title":"项目群","openConversationId":"cid-1"}]}`
	case "list_conversation_message_v2":
		if arguments["openconversation_id"] == "cid-resources" {
			return `{"result":{"hasMore":false,"messages":[{"openMessageId":"msg-resource","openConversationId":"cid-resources","sender":{"name":"测试用户甲","openDingTalkId":"` + mockCurrentDOpenID + `","senderType":"user"},"msgType":"file","content":"{\"mediaId\":\"@media-1\"}","createTime":"2026-08-03 10:00:00"}]}}`
		}
		return `{"result":{"hasMore":false,"messages":[{"openMessageId":"msg-1","openConversationId":"cid-1","sender":{"name":"测试用户甲","openDingTalkId":"` + mockCurrentDOpenID + `","senderType":"user"},"msgType":"text","content":"进度正常","createTime":"2026-08-03 10:00:00"}]}}`
	case "get_resource_download_url":
		body, _ := json.Marshal(map[string]any{"result": map[string]any{
			"resourceUrl": resourceURL,
			"fileName":    "artifact.bin",
		}})
		return string(body)
	case "init_conversation_file_upload":
		body, _ := json.Marshal(map[string]any{
			"resourceUrl": mcpBaseURL + "/upload/conversation",
			"uploadKey":   "upload-key-1",
		})
		return string(body)
	case "commit_conversation_file_upload":
		return `{"result":{"dentryId":101,"spaceId":202}}`
	case "search_messages":
		if arguments["keyword"] == "分页失败" && fmt.Sprint(arguments["cursor"]) == "c2" {
			return `{"success":false,"code":"MOCK_PAGE_FAILURE","message":"simulated second page failure"}`
		}
		return `{"result":{"messages":[{"openMessageId":"search-msg-1","openConversationId":"cid-1","senderOpenDingTalkId":"` + mockCurrentDOpenID + `","content":"first page"}],"hasMore":true,"nextCursor":"c2"}}`
	case "get_current_user_profile":
		return `{"result":[{"orgEmployeeModel":{"userId":"self-user"}}]}`
	case "create_group_conversation":
		return `{"result":{"openCid":"created-cid"}}`
	case "send_personal_message":
		return `{"result":{"openMessageId":"sent-msg-1","sendStatus":"accepted"}}`
	default:
		return `{"success":true}`
	}
}

func recordedToolNames(calls []recordedToolCall) []string {
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		names = append(names, call.tool)
	}
	return names
}

type trustedDownloadFixture struct {
	URL      string
	DialAddr string
	CAFile   string
}

func newTrustedDownloadFixture(t *testing.T, body []byte) trustedDownloadFixture {
	t.Helper()
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "DWS Multi IM Test CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCertificate, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "download.dingtalk.com"},
		DNSNames:     []string{"download.dingtalk.com"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caCertificate, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	leafPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
	leafPEM = append(leafPEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})...)
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(leafKey)})
	certificate, err := tls.X509KeyPair(leafPEM, privateKeyPEM)
	if err != nil {
		t.Fatal(err)
	}

	downloadServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(body)
	}))
	downloadServer.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}
	downloadServer.StartTLS()
	t.Cleanup(downloadServer.Close)

	caFile := filepath.Join(t.TempDir(), "download-ca.pem")
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	if err := os.WriteFile(caFile, caPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	return trustedDownloadFixture{
		URL:      "https://download.dingtalk.com/artifact.bin",
		DialAddr: downloadServer.Listener.Addr().String(),
		CAFile:   caFile,
	}
}

func assertMultiIMCallsAreLocalAndAuthorized(t *testing.T, calls []recordedToolCall) {
	t.Helper()
	for _, call := range calls {
		if call.err != nil {
			t.Fatal(call.err)
		}
		if call.method != http.MethodPost || call.jsonrpc != "2.0" || call.authorization != "Bearer ci-smoke-token" {
			t.Fatalf("invalid local JSON-RPC call = %#v", call)
		}
		if !strings.HasPrefix(call.path, "/mcp/") {
			t.Fatalf("unexpected MCP path = %q", call.path)
		}
	}
}

func TestMockMCPSmoke_ChatDownloadMediaJSONCLI(t *testing.T) {
	type mediaRequest struct {
		method string
		path   string
		query  string
		header string
	}

	mediaPayload := []byte("synthetic media payload")
	var mediaRequestsMu sync.Mutex
	var mediaRequests []mediaRequest
	mediaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mediaRequestsMu.Lock()
		mediaRequests = append(mediaRequests, mediaRequest{
			method: r.Method,
			path:   r.URL.Path,
			query:  r.URL.RawQuery,
			header: r.Header.Get("X-Download-Fixture"),
		})
		mediaRequestsMu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(mediaPayload)
	}))
	defer mediaServer.Close()

	downloadURL := mediaServer.URL + "/photo.jpg?fixture=one&part=two"
	downloadInfo, err := json.Marshal(map[string]any{
		"resourceUrl": downloadURL,
		"headers": map[string]string{
			"X-Download-Fixture": "synthetic",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	var requestsMu sync.Mutex
	var requests []recordedToolCall
	mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := recordedToolCall{
			path:          r.URL.Path,
			method:        r.Method,
			authorization: r.Header.Get("Authorization"),
		}

		var envelope struct {
			JSONRPC string `json:"jsonrpc"`
			ID      int    `json:"id"`
			Method  string `json:"method"`
			Params  struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
			call.err = err
		} else {
			call.jsonrpc = envelope.JSONRPC
			call.tool = envelope.Params.Name
			call.arguments = envelope.Params.Arguments
			if envelope.Method != "tools/call" {
				call.err = fmt.Errorf("JSON-RPC method = %q, want tools/call", envelope.Method)
			}
		}
		requestsMu.Lock()
		requests = append(requests, call)
		requestsMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      envelope.ID,
			"result": map[string]any{
				"content": []map[string]any{{
					"type": "text",
					"text": string(downloadInfo),
				}},
			},
		})
	}))
	defer mcpServer.Close()

	outputDir := t.TempDir()
	env := isolatedCLIEnv(t, map[string]string{
		"DINGTALK_IM_MCP_URL": mcpServer.URL + "/mcp/im",
	})
	args := []string{
		"--token", "ci-smoke-token",
		"--format", "json",
		"chat", "message", "download-media",
		"--type", "mediaId",
		"--resource-id", "resource-001",
		"--message-id", "message-001",
		"--open-conversation-id", "conversation-001",
		"--output", outputDir,
	}
	stdout, stderr, err := runCLI(t, env, args...)
	if err != nil {
		t.Fatalf("dws chat message download-media failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	requestsMu.Lock()
	recorded := append([]recordedToolCall(nil), requests...)
	requestsMu.Unlock()
	if len(recorded) != 1 {
		t.Fatalf("local fake MCP server received %d requests, want exactly one tools/call: %#v", len(recorded), recorded)
	}
	call := recorded[0]
	if call.err != nil {
		t.Fatal(call.err)
	}
	if call.path != "/mcp/im" || call.method != http.MethodPost || call.jsonrpc != "2.0" {
		t.Fatalf("MCP request = path %q method %q jsonrpc %q", call.path, call.method, call.jsonrpc)
	}
	if call.authorization != "Bearer ci-smoke-token" {
		t.Fatalf("Authorization = %q, want synthetic smoke token", call.authorization)
	}
	if call.tool != "get_resource_download_url" {
		t.Fatalf("tool = %q, want get_resource_download_url", call.tool)
	}
	wantArgs := map[string]any{
		"resourceType":       "mediaId",
		"resourceId":         "resource-001",
		"openMessageId":      "message-001",
		"openConversationId": "conversation-001",
	}
	if !reflect.DeepEqual(call.arguments, wantArgs) {
		t.Fatalf("arguments = %#v, want %#v", call.arguments, wantArgs)
	}

	mediaRequestsMu.Lock()
	recordedMedia := append([]mediaRequest(nil), mediaRequests...)
	mediaRequestsMu.Unlock()
	if len(recordedMedia) != 1 {
		t.Fatalf("media server received %d requests, want exactly one: %#v", len(recordedMedia), recordedMedia)
	}
	gotMedia := recordedMedia[0]
	if gotMedia.method != http.MethodGet || gotMedia.path != "/photo.jpg" {
		t.Fatalf("media request = method %q path %q", gotMedia.method, gotMedia.path)
	}
	if gotMedia.query != "fixture=one&part=two" {
		t.Fatalf("media query = %q, want fixture=one&part=two", gotMedia.query)
	}
	if gotMedia.header != "synthetic" {
		t.Fatalf("media header = %q, want synthetic", gotMedia.header)
	}

	wantOutput := filepath.Join(outputDir, "photo.jpg")
	gotPayload, err := os.ReadFile(wantOutput)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if !bytes.Equal(gotPayload, mediaPayload) {
		t.Fatalf("downloaded payload = %q, want %q", gotPayload, mediaPayload)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("CLI returned non-JSON stdout: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if len(result) != 3 || result["success"] != true || result["downloadUrl"] != downloadURL || result["output"] != wantOutput {
		t.Fatalf("CLI result = %#v, want exact success/downloadUrl/output contract", result)
	}
	if strings.Contains(stdout, "[INFO]") {
		t.Fatalf("JSON stdout contains progress text: %s", stdout)
	}
	if !strings.Contains(stdout, "?fixture=one&part=two") {
		t.Fatalf("downloadUrl was escaped or changed: %s", stdout)
	}

	publicResult := map[string]any{
		"success":     true,
		"downloadUrl": "http://127.0.0.1:<fixture-port>/photo.jpg?fixture=one&part=two",
		"output":      "./downloads/photo.jpg",
	}
	var publicOutput bytes.Buffer
	publicEncoder := json.NewEncoder(&publicOutput)
	publicEncoder.SetEscapeHTML(false)
	publicEncoder.SetIndent("", "  ")
	if err := publicEncoder.Encode(publicResult); err != nil {
		t.Fatal(err)
	}
	t.Log("CLI (public-safe): dws chat message download-media --type mediaId --resource-id resource-001 --message-id message-001 --open-conversation-id conversation-001 --output ./downloads/ --format json")
	t.Logf("stdout (public-safe):\n%s", strings.TrimSpace(publicOutput.String()))
	t.Logf("downloaded file: photo.jpg (%d bytes, content verified)", len(gotPayload))
}

func isolatedCLIEnv(t *testing.T, extra map[string]string) []string {
	t.Helper()

	root := t.TempDir()
	controlled := map[string]string{
		"HOME":                     root,
		"USERPROFILE":              root,
		"DWS_CONFIG_DIR":           filepath.Join(root, "config"),
		"DWS_KEYCHAIN_DIR":         filepath.Join(root, "keychain"),
		"DWS_DISABLE_KEYCHAIN":     "1",
		"HTTP_PROXY":               "http://127.0.0.1:1",
		"HTTPS_PROXY":              "http://127.0.0.1:1",
		"http_proxy":               "http://127.0.0.1:1",
		"https_proxy":              "http://127.0.0.1:1",
		"NO_PROXY":                 "127.0.0.1,localhost,::1",
		"no_proxy":                 "127.0.0.1,localhost,::1",
		mockMCPSmokeHelperEnv:      "1",
		"DWS_ALLOW_HTTP_ENDPOINTS": "1",
		"DWS_TRUSTED_DOMAINS":      "127.0.0.1,localhost,::1",
	}
	for key, value := range extra {
		controlled[key] = value
	}

	env := make([]string, 0, len(controlled)+8)
	for _, key := range []string{"PATH", "TMPDIR", "TEMP", "TMP", "LANG", "LC_ALL", "TZ", "SYSTEMROOT"} {
		if value := os.Getenv(key); value != "" {
			env = append(env, key+"="+value)
		}
	}
	for key, value := range controlled {
		env = append(env, key+"="+value)
	}
	sort.Strings(env)
	return env
}

func runCLI(t *testing.T, env []string, args ...string) (string, string, error) {
	return runCLIInDir(t, env, "", args...)
}

func runCLIInDir(t *testing.T, env []string, dir string, args ...string) (string, string, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), mockMCPCLIExecutionTimeout)
	defer cancel()
	processArgs := append([]string{"-test.run=^TestCLIHelperProcess$", "--"}, args...)
	cmd := exec.CommandContext(ctx, os.Args[0], processArgs...)
	cmd.Env = env
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("dws %s timed out: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), ctx.Err(), stdout.String(), stderr.String())
	}
	return stdout.String(), stderr.String(), err
}
