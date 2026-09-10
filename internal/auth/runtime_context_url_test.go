package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtimecontext"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageOAuthRuntimeURLs(t *testing.T) {
	for _, state := range []runtimecontext.State{runtimecontext.StateReady, runtimecontext.StateUnavailable, runtimecontext.StateTimeout, runtimecontext.StateError} {
		t.Run(string(state), func(t *testing.T) {
			secret := "private-runtime+value/&=测试"
			snapshot := runtimecontext.Result{State: state}
			if state == runtimecontext.StateReady {
				snapshot = runtimecontext.ReadyResultForTest(secret)
			}
			var resolutions atomic.Int32
			testseam.Swap(t, &resolveAuthRuntimeContext, func() runtimecontext.Result { resolutions.Add(1); return snapshot })
			f := newOAuthLoginFixture(t, func(int32) CLIAuthStatus { return CLIAuthStatus{Success: true} })
			var output bytes.Buffer
			f.provider.Output = &output
			f.provider.logger = slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
			f.provider.NoBrowser = false
			opened := make(chan string, 1)
			testseam.Swap(t, &oauthOpenBrowser, func(raw string) error { opened <- raw; return errors.New(raw) })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := startOAuthLogin(t, ctx, f)
			var browserURL string
			select {
			case browserURL = <-opened:
			case result := <-done:
				t.Fatalf("login exited: %v", result.err)
			}
			parsed, err := url.Parse(browserURL)
			if err != nil {
				t.Fatal("invalid browser URL")
			}
			want := ""
			if state == runtimecontext.StateReady {
				want = secret
			}
			if parsed.Query().Get("callerUmt") != want {
				t.Fatal("wrong browser runtime context")
			}
			if want != "" && parsed.Query().Get("caller") != "dws" {
				t.Fatal("missing caller")
			}
			redirect, _ := url.Parse(parsed.Query().Get("redirect_uri"))
			if redirect.Query().Has("callerUmt") || redirect.Query().Has("caller") {
				t.Fatal("redirect URI contains context")
			}
			_, body := httpGetBody(t, f.callbackBase+"/api/status")
			var status struct {
				AuthorizeURL string `json:"authorizeUrl"`
			}
			if json.Unmarshal([]byte(body), &status) != nil || status.AuthorizeURL != browserURL {
				t.Fatal("reauthorization URL differs from browser URL")
			}
			if !strings.Contains(notEnabledHTML, "backLink.href = authorizeUrl;") || strings.Contains(notEnabledHTML, `"?client_id="`) {
				t.Fatal("page must use server URL")
			}
			cancel()
			awaitOAuthLogin(t, done)
			if resolutions.Load() != 1 {
				t.Fatal("OAuth snapshot resolved repeatedly")
			}
			if strings.Contains(output.String(), secret) || strings.Contains(output.String(), url.QueryEscape(secret)) || strings.Contains(output.String(), "callerUmt") {
				t.Fatal("runtime value leaked into output")
			}
			if !strings.Contains(output.String(), "client_id=") {
				t.Fatal("manual authorization link missing")
			}
		})
	}
}

func TestCrossPlatformCoverageDeviceRuntimeURLRetrySnapshot(t *testing.T) {
	for _, state := range []runtimecontext.State{runtimecontext.StateReady, runtimecontext.StateTimeout, runtimecontext.StateError} {
		t.Run(string(state), func(t *testing.T) {
			isolateOAuthPersistence(t)
			SetClientID("")
			SetClientSecret("")
			resetClientIDFromMCP()
			t.Cleanup(func() { SetClientID(""); SetClientSecret(""); resetClientIDFromMCP() })
			secret := "private-device+context/&="
			snapshot := runtimecontext.Result{State: state}
			if state == runtimecontext.StateReady {
				snapshot = runtimecontext.ReadyResultForTest(secret)
			}
			resolutions := 0
			testseam.Swap(t, &resolveAuthRuntimeContext, func() runtimecontext.Result { resolutions++; return snapshot })
			testseam.Swap(t, &deviceFetchClientID, func(context.Context) (string, error) { return "client", nil })
			raw := "https://example.test/verify?user_code=ABCD#section"
			response := &DeviceAuthResponse{VerificationURIComplete: raw, VerificationURI: "https://example.test/verify", UserCode: "ABCD", Interval: 1}
			testseam.Swap(t, &deviceRequestCode, func(*DeviceFlowProvider, context.Context) (*DeviceAuthResponse, error) { return response, nil })
			testseam.Swap(t, &deviceWaitAuth, func(_ *DeviceFlowProvider, _ context.Context, got *DeviceAuthResponse) (*DeviceTokenResponse, error) {
				if got.VerificationURIComplete != raw {
					t.Fatal("server response mutated")
				}
				return nil, errors.New("invalid_grant")
			})
			opens := 0
			testseam.Swap(t, &deviceOpenBrowser, func(got string) error {
				opens++
				parsed, err := url.Parse(got)
				if err != nil {
					t.Fatal(err)
				}
				q := parsed.Query()
				if q.Get("user_code") != "ABCD" || parsed.Fragment != "section" {
					t.Fatal("verification URL changed")
				}
				if state == runtimecontext.StateReady {
					if q.Get("callerUmt") != secret || q.Get("caller") != "dws" || len(q["callerUmt"]) != 1 {
						t.Fatal("wrong device context")
					}
				} else if got != raw {
					t.Fatal("failed context must preserve original URL")
				}
				return errors.New(got)
			})
			var output bytes.Buffer
			p := NewDeviceFlowProvider(t.TempDir(), slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))
			p.Output = &output
			if _, err := p.Login(context.Background()); err == nil || !isInvalidGrantError(err) {
				t.Fatal("unexpected retry outcome")
			}
			if opens != 3 || resolutions != 1 {
				t.Fatalf("opens=%d resolutions=%d", opens, resolutions)
			}
			if strings.Contains(output.String(), secret) || strings.Contains(output.String(), url.QueryEscape(secret)) || strings.Contains(output.String(), "callerUmt") {
				t.Fatal("device value leaked")
			}
			if !strings.Contains(output.String(), raw) {
				t.Fatal("original manual URL missing")
			}
		})
	}
}
