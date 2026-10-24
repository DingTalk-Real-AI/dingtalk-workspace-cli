package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

const splitOIDCState = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.1792023000.bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func splitOIDCRoot(t *testing.T, caller edition.ToolCaller) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	root := &cobra.Command{Use: "dws", TraverseChildren: true, SilenceErrors: true, SilenceUsage: true}
	root.SetContext(t.Context())
	root.PersistentFlags().String("format", "json", "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("client-id", "", "")
	root.PersistentFlags().String("client-secret", "", "")
	auth := &cobra.Command{Use: "auth", TraverseChildren: true}
	auth.AddCommand(newAuthExchangePrepareCommand(), newAuthTokenExchangeCommand(caller))
	root.AddCommand(auth)
	out := new(bytes.Buffer)
	root.SetOut(out)
	root.SetErr(out)
	return root, out
}

func runSplitOIDC(t *testing.T, root *cobra.Command, args ...string) error {
	t.Helper()
	root.SetArgs(args)
	cmd, err := corecmd.ExecuteCForTest(root)
	if err == nil {
		_, _, err = output.EmitStoredResult(cmd)
	}
	return err
}

func TestCrossPlatformCoverageSplitOIDCCommandsValidation(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	for _, args := range [][]string{
		{"exchange-prepare"}, {"exchange-prepare", "--provider", ""}, {"exchange-prepare", "--provider", "Ant"},
		{"exchange-prepare", "--provider", "ant", "extra"}, {"exchange-prepare", "--provider", "ant", "--client-id", "app", "--client-secret", "secret-canary"},
		{"token-exchange", "--provider", "ant", "--code", "code"},
		{"token-exchange", "--state", splitOIDCState, "--code", "code"},
		{"token-exchange", "--provider", "ant", "--state", splitOIDCState},
		{"token-exchange", "--provider", "ant", "--state", splitOIDCState, "--code-stdin=false"},
		{"token-exchange", "--provider", "ant", "--state", splitOIDCState, "--code", "", "--code-stdin"},
		{"token-exchange", "--provider", "ant", "--state", splitOIDCState, "--code", "code", "--client-id", "app"},
		{"token-exchange", "--provider", "ant", "--state", splitOIDCState, "--code", "code", "--client-secret", "secret-canary"},
		{"token-exchange", "--provider", "ant", "--state", splitOIDCState, "--code", "code", "--client-id", " ", "--client-secret", "secret-canary"},
	} {
		root, out := splitOIDCRoot(t, nil)
		err := runSplitOIDC(t, root, append([]string{"auth"}, args...)...)
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation || typed.ExitCode() != 3 {
			t.Fatalf("args %v: expected validation, got %v", args, err)
		}
		if strings.Contains(out.String()+err.Error(), "secret-canary") {
			t.Fatal("validation leaked secret")
		}
	}
}

func TestCrossPlatformCoverageSplitOIDCDryRunDoesNotReadOrCall(t *testing.T) {
	for _, name := range []string{"exchange-prepare", "token-exchange"} {
		root, out := splitOIDCRoot(t, nil)
		root.SetIn(splitOIDCForbiddenReader{t})
		args := []string{"auth", name, "--provider", "ant", "--dry-run"}
		if name == "token-exchange" {
			args = append(args, "--state", splitOIDCState, "--code-stdin", "--save")
		}
		if err := runSplitOIDC(t, root, args...); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), `"dry_run": true`) || strings.Contains(out.String(), splitOIDCState) {
			t.Fatal("unsafe/missing dry-run output")
		}
	}
}

func TestCrossPlatformCoverageSplitOIDCInvokeRevalidatesProvider(t *testing.T) {
	for _, name := range []string{"exchange-prepare", "token-exchange"} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			testseam.Swap(t, &resolveOIDCExchangeCommandProvider, func(cmd *cobra.Command) (*authpkg.OIDCLoginProvider, error) {
				calls++
				if calls == 1 {
					return oidcExchangeCommandProvider(cmd)
				}
				return nil, apperrors.NewValidation("provider changed after validation")
			})
			root, out := splitOIDCRoot(t, nil)
			args := []string{"auth", name, "--provider", "ant"}
			if name == "token-exchange" {
				args = append(args, "--state", splitOIDCState, "--code", "code")
			}
			err := runSplitOIDC(t, root, args...)
			var typed *apperrors.Error
			if calls != 2 || !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation || out.Len() != 0 {
				t.Fatalf("runtime provider revalidation: calls=%d output=%q error=%v", calls, out.String(), err)
			}
		})
	}
}

func TestCrossPlatformCoverageSplitOIDCSaveRequiresIdentityCaller(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	root, out := splitOIDCRoot(t, nil)
	err := runSplitOIDC(t, root, "auth", "token-exchange", "--provider", "ant", "--state", splitOIDCState, "--code", "code", "--save")
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Category != apperrors.CategoryInternal || out.Len() != 0 {
		t.Fatalf("unsupported identity caller reached exchange: output=%q error=%v", out.String(), err)
	}
}

func TestCrossPlatformCoverageSplitOIDCValidatorRejectsDirectCodeConflict(t *testing.T) {
	root, _ := splitOIDCRoot(t, nil)
	cmd, _, err := root.Find([]string{"auth", "token-exchange"})
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"provider": "ant", "state": splitOIDCState, "code": "code", "code-stdin": "true"} {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	err = validateOIDCTokenExchangeCommand(cmd, nil)
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("direct validation accepted conflicting code sources: %v", err)
	}
}

type splitOIDCForbiddenReader struct{ t *testing.T }

func (r splitOIDCForbiddenReader) Read([]byte) (int, error) {
	r.t.Error("dry-run read stdin")
	return 0, io.EOF
}

func TestCrossPlatformCoverageSplitOIDCRealHTTPOutputAndSave(t *testing.T) {
	for _, scenario := range []string{"prepare", "return", "stdin", "save", "save-fails", "save-readback-fails"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("DWS_CONFIG_DIR", dir)
			t.Setenv(keychain.StorageDirEnv, filepath.Join(dir, "keys"))
			t.Setenv(keychain.DisableKeychainEnv, "1")
			t.Setenv(keychain.TestNamespaceEnv, dir)
			authpkg.SetRuntimeProfile("")
			t.Cleanup(func() { authpkg.SetRuntimeProfile("") })
			if err := authpkg.SaveTokenData(dir, &authpkg.TokenData{AccessToken: "old-access", CorpID: "old-corp", UserID: "old-user", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
				t.Fatal(err)
			}
			caller := &externalIdentityCaller{authCoverageCaller: authCoverageCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{"result":[{"orgEmployeeModel":{"corpId":"new-corp","userId":"new-user","orgUserName":"New User"}}]}`}}}}}
			if scenario == "save-fails" {
				caller.err = errors.New("access-canary")
			}
			if scenario == "save-readback-fails" {
				testseam.Swap(t, &authLoadProfiles, func(string) (*authpkg.ProfilesConfig, error) {
					return nil, errors.New("profile-readback-canary")
				})
			}
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]string
				json.NewDecoder(r.Body).Decode(&body)
				if scenario == "prepare" {
					if r.URL.Path != "/cli/auth/oidc/transactions" || len(body) != 1 || body["provider"] != "ant" {
						t.Error("invalid preparation")
					}
					w.WriteHeader(201)
					json.NewEncoder(w).Encode(map[string]any{"provider": "ant", "expires_in": 600, "authorization_request": map[string]any{"authorization_endpoint": "https://idp.example/authorize", "parameters": map[string]string{"client_id": "idp-app", "response_type": "code", "scope": "openid", "redirect_uri": "ant-sdk://oidc/callback", "state": splitOIDCState, "nonce": strings.Repeat("n", 43), "code_challenge": strings.Repeat("c", 43), "code_challenge_method": "S256"}}})
					return
				}
				if r.URL.Path != "/cli/auth/oidc/exchange" || len(body) != 5 || body["provider"] != "ant" || body["code"] != " original+code%20 " || body["state"] != splitOIDCState || body["client_id"] != "dws-app" || body["client_secret"] != "secret-canary" {
					t.Error("invalid exchange body")
				}
				io.WriteString(w, `{"accessToken":"access-canary","refreshToken":"refresh-canary","expiresIn":3600,"corpId":"new-corp"}`)
			}))
			defer server.Close()
			if err := os.WriteFile(filepath.Join(dir, "mcp_url"), []byte(server.URL+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			root, out := splitOIDCRoot(t, caller)
			args := []string{"auth", "token-exchange", "--provider", "ant", "--state", splitOIDCState, "--client-id", "dws-app", "--client-secret", "secret-canary"}
			if scenario == "prepare" {
				args = []string{"auth", "exchange-prepare", "--provider", "ant"}
			} else if scenario == "stdin" {
				args = append(args, "--code-stdin")
				root.SetIn(strings.NewReader(" original+code%20 \r\n"))
			} else {
				args = append(args, "--code", " original+code%20 ")
			}
			if scenario == "save" || scenario == "save-fails" || scenario == "save-readback-fails" {
				args = append(args, "--save")
			}
			err := runSplitOIDC(t, root, args...)
			switch scenario {
			case "save-fails":
				if err == nil || strings.Contains(err.Error(), "access-canary") {
					t.Fatal("identity failure leaked or succeeded")
				}
			case "save-readback-fails":
				var typed *apperrors.Error
				if !errors.As(err, &typed) || typed.Category != apperrors.CategoryInternal || strings.Contains(err.Error(), "profile-readback-canary") {
					t.Fatalf("readback failure was not sanitized: %v", err)
				}
			default:
				if err != nil {
					t.Fatal(err)
				}
			}
			if calls != 1 {
				t.Fatal("command made extra portal requests")
			}
			if strings.Contains(out.String(), `"oidc_provider"`) {
				t.Fatal("local login metadata changed the command response")
			}
			if base, err := os.ReadFile(filepath.Join(dir, "mcp_url")); err != nil || string(base) != server.URL+"\n" {
				t.Fatalf("exchange changed system MCP configuration: %v", err)
			}
			cfg, err := authpkg.LoadProfiles(dir)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.CurrentProfile != "old-corp:old-user" {
				t.Fatal("existing default changed")
			}
			if scenario == "save" || scenario == "save-readback-fails" {
				if len(cfg.Profiles) != 2 || caller.calls != 1 || caller.token != "access-canary" {
					t.Fatal("missing verified saved profile")
				}
				if (scenario == "save") != strings.Contains(out.String(), "new-corp:new-user") {
					t.Fatal("saved profile output did not match readback result")
				}
			} else if len(cfg.Profiles) != 1 {
				t.Fatal("return/preparation/failure wrote credentials")
			}
			if scenario == "return" || scenario == "stdin" {
				var envelope struct {
					OK   bool              `json:"ok"`
					Data authpkg.TokenData `json:"data"`
				}
				if json.Unmarshal(out.Bytes(), &envelope) != nil || !envelope.OK || envelope.Data.AccessToken != "access-canary" || envelope.Data.RefreshToken != "refresh-canary" || caller.calls != 0 {
					t.Fatal("return did not use unchanged TokenData and unified result")
				}
			} else if strings.Contains(out.String(), "access-canary") || strings.Contains(out.String(), "refresh-canary") {
				t.Fatal("save exposed credentials")
			}
			if strings.Contains(out.String(), "original+code") || strings.Contains(out.String(), "secret-canary") {
				t.Fatal("input leaked")
			}
			if _, err := os.Stat(authpkg.GetAppConfigPath(dir)); !os.IsNotExist(err) {
				t.Fatal("exchange credentials written into app config")
			}
		})
	}
}

func TestCrossPlatformCoverageSplitOIDCPublicRootCleansOneTimeFlags(t *testing.T) {
	for _, suffix := range [][]string{{"--help"}, {"--unknown"}, {"extra"}, {"--dry-run"}} {
		root := newExternalExchangeIntegrationRoot(t)
		root.SetArgs(append([]string{"--client-id", "split-app", "--client-secret", "split-secret", "auth", "token-exchange", "--provider", "ant", "--code", "one-time-code", "--state", splitOIDCState}, suffix...))
		_, _ = root.ExecuteC()
		cmd, _, err := root.Find([]string{"auth", "token-exchange"})
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"code", "state", "provider", "client-id", "client-secret"} {
			flag := cmd.Flag(name)
			if flag == nil || flag.Changed || flag.Value.String() != "" {
				t.Fatalf("invocation retained --%s", name)
			}
		}
		if authpkg.ClientID() == "split-app" || authpkg.ClientSecret() == "split-secret" {
			t.Fatal("exchange changed global credentials")
		}
		// Cobra retains the help flag too; explicitly leave help mode before
		// verifying that one-time inputs cannot be reused by an execution.
		root.SetArgs([]string{"auth", "token-exchange", "--provider", "ant", "--dry-run", "--help=false"})
		if err := root.Execute(); err == nil {
			t.Fatal("second call reused one-time state/code")
		}
	}
}

func TestCrossPlatformCoverageSplitOIDCStdinBoundsAndPortalFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", dir)
	for _, input := range []string{"", "\n", "code\n\n", "code\t", strings.Repeat("c", 8193), string([]byte{0xff})} {
		root, _ := splitOIDCRoot(t, nil)
		root.SetIn(strings.NewReader(input))
		err := runSplitOIDC(t, root, "auth", "token-exchange", "--provider", "ant", "--state", splitOIDCState, "--code-stdin")
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation {
			t.Fatalf("invalid stdin: %v", err)
		}
	}
	for _, status := range []int{400, 401, 409, 429, 500, 503, 307} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.Header().Set("Location", "https://example.invalid/leak")
			w.WriteHeader(status)
			io.WriteString(w, `{"error":{"code":"provider_mismatch","action":"restart_login","message":"code-canary"}}`)
		}))
		if err := os.WriteFile(filepath.Join(dir, "mcp_url"), []byte(server.URL), 0600); err != nil {
			t.Fatal(err)
		}
		root, out := splitOIDCRoot(t, nil)
		err := runSplitOIDC(t, root, "auth", "token-exchange", "--provider", "ant", "--code", "code-canary", "--state", splitOIDCState)
		server.Close()
		var typed *apperrors.Error
		if !errors.As(err, &typed) || !strings.Contains(typed.Hint, "exchange-prepare") || strings.Contains(err.Error()+out.String(), "code-canary") || calls != 1 {
			t.Fatalf("unsafe failure or replay on HTTP %d: %v", status, err)
		}
	}
}

func TestCrossPlatformCoverageSplitOIDCStdinCancellation(t *testing.T) {
	for _, filePipe := range []bool{false, true} {
		for _, deadline := range []bool{false, true} {
			t.Run(fmt.Sprintf("file=%t/deadline=%t", filePipe, deadline), func(t *testing.T) {
				dir := t.TempDir()
				t.Setenv("DWS_CONFIG_DIR", dir)
				var input io.ReadCloser
				var writer io.WriteCloser
				if filePipe {
					r, w, err := os.Pipe()
					if err != nil {
						t.Fatal(err)
					}
					input, writer = r, w
				} else {
					input, writer = io.Pipe()
				}
				defer input.Close()
				defer writer.Close()
				reader := &splitOIDCObservedReader{ReadCloser: input, started: make(chan struct{}), finished: make(chan struct{})}
				ctx, cancel := context.WithCancel(t.Context())
				want := context.Canceled
				if deadline {
					cancel()
					ctx, cancel = context.WithTimeout(t.Context(), 200*time.Millisecond)
					want = context.DeadlineExceeded
				}
				defer cancel()
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					calls.Add(1)
					w.WriteHeader(http.StatusInternalServerError)
				}))
				defer server.Close()
				if err := os.WriteFile(filepath.Join(dir, "mcp_url"), []byte(server.URL), 0600); err != nil {
					t.Fatal(err)
				}
				root, out := splitOIDCRoot(t, nil)
				root.SetContext(ctx)
				root.SetIn(reader)
				result := make(chan error, 1)
				go func() {
					result <- runSplitOIDC(t, root, "auth", "token-exchange", "--provider", "ant", "--state", splitOIDCState, "--code-stdin")
				}()
				select {
				case <-reader.started:
				case <-time.After(3 * time.Second):
					t.Fatal("command did not start reading stdin")
				}
				if !deadline {
					cancel()
				}
				select {
				case err := <-result:
					if !errors.Is(err, want) {
						t.Fatalf("cancellation identity lost: %v", err)
					}
				case <-time.After(3 * time.Second):
					// Release the old blocking implementation before reporting failure.
					_ = writer.Close()
					<-result
					t.Fatal("command remained blocked after context ended")
				}
				select {
				case <-reader.finished:
				case <-time.After(time.Second):
					t.Fatal("cancellation left a blocked stdin reader")
				}
				if calls.Load() != 0 || out.Len() != 0 {
					t.Fatal("canceled input produced an exchange or success output")
				}
			})
		}
	}
}

type splitOIDCObservedReader struct {
	io.ReadCloser
	started, finished chan struct{}
}

func (r *splitOIDCObservedReader) Read(p []byte) (int, error) {
	close(r.started)
	defer close(r.finished)
	return r.ReadCloser.Read(p)
}

func TestCrossPlatformCoverageSplitOIDCStdinReadErrors(t *testing.T) {
	for _, readErr := range []error{
		context.Canceled,
		context.DeadlineExceeded,
		fmt.Errorf("read canceled: %w", context.Canceled),
		fmt.Errorf("read timed out: %w", context.DeadlineExceeded),
		apperrors.NewInternal("stdin source unavailable"),
		errors.New("code-canary input failure"),
	} {
		t.Run(readErr.Error(), func(t *testing.T) {
			root, out := splitOIDCRoot(t, nil)
			root.SetIn(splitOIDCErrorReader{readErr})
			err := runSplitOIDC(t, root, "auth", "token-exchange", "--provider", "ant", "--state", splitOIDCState, "--code-stdin")
			if apperrors.PreserveClassification(readErr) {
				if err != readErr {
					t.Fatalf("read error identity lost: got %v, want %v", err, readErr)
				}
			} else {
				var typed *apperrors.Error
				if !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation {
					t.Fatalf("expected sanitized validation error, got %v", err)
				}
				if strings.Contains(err.Error()+out.String(), "code-canary") {
					t.Fatal("read failure leaked input")
				}
			}
		})
	}
	for _, deadline := range []bool{false, true} {
		ctx, cancel := context.WithCancel(t.Context())
		want := context.Canceled
		if deadline {
			cancel()
			ctx, cancel = context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
			want = context.DeadlineExceeded
		}
		cancel()
		root, _ := splitOIDCRoot(t, nil)
		root.SetContext(ctx)
		root.SetIn(splitOIDCForbiddenReader{t})
		if err := runSplitOIDC(t, root, "auth", "token-exchange", "--provider", "ant", "--state", splitOIDCState, "--code-stdin"); !errors.Is(err, want) {
			t.Fatalf("already canceled command: got %v, want %v", err, want)
		}
	}
}

type splitOIDCErrorReader struct{ err error }

func (r splitOIDCErrorReader) Read(p []byte) (int, error) {
	return copy(p, "partial-code"), r.err
}

func TestCrossPlatformCoverageSplitOIDCStdinNativePipeCancellation(t *testing.T) {
	input, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	defer writer.Close()
	// Fd switches the pipe to blocking mode, like a process's inherited stdin.
	_ = input.Fd()
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	root, _ := splitOIDCRoot(t, nil)
	root.SetContext(ctx)
	root.SetIn(input)
	result := make(chan error, 1)
	go func() {
		result <- runSplitOIDC(t, root, "auth", "token-exchange", "--provider", "ant", "--state", splitOIDCState, "--code-stdin")
	}()
	select {
	case err := <-result:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("native stdin lost deadline: %v", err)
		}
	case <-time.After(3 * time.Second):
		_ = writer.Close()
		<-result
		t.Fatal("native blocking stdin did not respond to its deadline")
	}
	if runtime.GOOS == "windows" {
		if err := input.Close(); !errors.Is(err, os.ErrClosed) {
			t.Fatalf("Windows pipe was not closed to interrupt the read: %v", err)
		}
		return
	}
	// Cancellation must leave native stdin open, with no abandoned reader
	// competing for the next invocation's bytes.
	if _, err := writer.Write([]byte("next")); err != nil {
		t.Fatalf("cancellation closed native stdin: %v", err)
	}
	go func() {
		buf := make([]byte, 4)
		_, err := io.ReadFull(input, buf)
		if err == nil && string(buf) != "next" {
			err = fmt.Errorf("unexpected input %q", buf)
		}
		result <- err
	}()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		_ = writer.Close()
		<-result
		t.Fatal("canceled command consumed the next invocation's stdin")
	}
}

func TestCrossPlatformCoverageSplitOIDCStdinSuccessLeavesInputOpen(t *testing.T) {
	input, err := os.CreateTemp(t.TempDir(), "code")
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	if _, err := input.WriteString(" original+code%20 \r\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := input.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	got, err := readOIDCCodeInput(t.Context(), input)
	if err != nil || string(got) != " original+code%20 \r\n" {
		t.Fatalf("input changed: %q, %v", got, err)
	}
	if _, err := input.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("successful read closed input: %v", err)
	}
}

type splitOIDCCancelAfterReadContext struct {
	context.Context
	done  chan struct{}
	calls atomic.Int32
}

func (c *splitOIDCCancelAfterReadContext) Done() <-chan struct{} { return c.done }

func (c *splitOIDCCancelAfterReadContext) Err() error {
	call := c.calls.Add(1)
	if call < 4 {
		return nil
	}
	if call == 4 {
		close(c.done)
	}
	return context.Canceled
}

type splitOIDCStaticReadCloser struct{}

func (splitOIDCStaticReadCloser) Read(p []byte) (int, error) {
	return copy(p, "code"), io.EOF
}

func (splitOIDCStaticReadCloser) Close() error { return nil }

func TestCrossPlatformCoverageSplitOIDCStdinLowLevelBoundaries(t *testing.T) {
	input, err := os.CreateTemp(t.TempDir(), "closed-code")
	if err != nil {
		t.Fatal(err)
	}
	if err := input.Close(); err != nil {
		t.Fatal(err)
	}
	if data, err := readOIDCCodeInput(t.Context(), input); err == nil || data != nil {
		t.Fatalf("closed native input was accepted: %q, %v", data, err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if n, err := (oidcCodeContextReader{ctx: ctx, input: splitOIDCForbiddenReader{t}}).Read(make([]byte, 1)); n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("context reader ignored pre-cancellation: n=%d error=%v", n, err)
	}

	cancelAfterRead := &splitOIDCCancelAfterReadContext{Context: t.Context(), done: make(chan struct{})}
	data, err := readOIDCCodeInput(cancelAfterRead, splitOIDCStaticReadCloser{})
	if data != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("completed read won over concurrent cancellation: %q, %v", data, err)
	}
}

func TestCrossPlatformCoverageSplitOIDCPublicRootCredentialForwarding(t *testing.T) {
	for _, prefix := range []bool{false, true} {
		root := newExternalExchangeIntegrationRoot(t)
		out := new(bytes.Buffer)
		root.SetOut(out)
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			var body map[string]string
			if json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 5 || body["provider"] != "enterprise_acme" || body["client_id"] != "split-app" || body["client_secret"] != "split-secret" {
				t.Error("public root lost invocation credentials")
			}
			if authpkg.ClientID() == "split-app" || authpkg.ClientSecret() == "split-secret" {
				t.Error("new pair changed global OAuth application")
			}
			io.WriteString(w, `{"accessToken":"public-access","refreshToken":"public-refresh","expiresIn":3600,"corpId":"corp"}`)
		}))
		if err := os.WriteFile(filepath.Join(defaultConfigDir(), "mcp_url"), []byte(server.URL), 0600); err != nil {
			t.Fatal(err)
		}
		args := []string{"auth", "token-exchange", "--provider", "enterprise_acme", "--code", "code", "--state", splitOIDCState, "--format", "json"}
		credentials := []string{"--client-id", "split-app", "--client-secret", "split-secret"}
		if prefix {
			args = append(credentials, args...)
		} else {
			args = append(args, credentials...)
		}
		root.SetArgs(args)
		err := root.Execute()
		server.Close()
		if err != nil || calls != 1 {
			t.Fatalf("public exchange failed: %v", err)
		}
		var envelope struct {
			OK   bool              `json:"ok"`
			Data authpkg.TokenData `json:"data"`
		}
		if json.Unmarshal(out.Bytes(), &envelope) != nil || !envelope.OK || envelope.Data.AccessToken != "public-access" {
			t.Fatal("missing unified output at root boundary")
		}
		cfg, err := authpkg.LoadProfiles(defaultConfigDir())
		if err != nil || len(cfg.Profiles) != 0 {
			t.Fatal("default exchange saved credentials")
		}
	}
}
