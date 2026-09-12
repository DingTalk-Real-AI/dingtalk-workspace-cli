package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		for _, noBrowser := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/no_browser=%t", state, noBrowser), func(t *testing.T) {
				secret := "private-runtime+value/&=测试"
				snapshot := runtimecontext.Result{State: state}
				if state == runtimecontext.StateReady {
					snapshot = runtimecontext.ReadyResultForTest(secret)
				}
				var resolutions atomic.Int32
				resolved := make(chan struct{})
				testseam.Swap(t, &resolveAuthRuntimeContext, func() runtimecontext.Result {
					resolutions.Add(1)
					close(resolved)
					return snapshot
				})
				f := newOAuthLoginFixture(t, func(int32) CLIAuthStatus { return CLIAuthStatus{Success: true} })
				var output, logs bytes.Buffer
				f.provider.Output = &output
				f.provider.logger = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
				f.provider.NoBrowser = noBrowser
				opened := make(chan string, 1)
				testseam.Swap(t, &oauthOpenBrowser, func(raw string) error { opened <- raw; return errors.New(raw) })
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				done := startOAuthLogin(t, ctx, f)
				waitOAuthSignal(t, resolved, done, "runtime snapshot")
				_, body := httpGetBody(t, f.callbackBase+"/api/status")
				var status struct {
					AuthorizeURL string `json:"authorizeUrl"`
				}
				if json.Unmarshal([]byte(body), &status) != nil || status.AuthorizeURL == "" {
					t.Fatal("reauthorization URL missing")
				}
				cancel()
				awaitOAuthLogin(t, done)

				select {
				case browserURL := <-opened:
					if noBrowser {
						t.Fatal("browser opened despite --no-browser")
					}
					if browserURL != status.AuthorizeURL {
						t.Fatal("reauthorization URL differs from browser URL")
					}
				default:
					if !noBrowser {
						t.Fatal("browser authorization missing")
					}
				}
				parsed, err := url.Parse(status.AuthorizeURL)
				if err != nil {
					t.Fatal("invalid authorization URL")
				}
				q := parsed.Query()
				if q.Has("lang") {
					t.Fatal("browser, manual, and reauthorization URLs must not override the login page language")
				}
				if state == runtimecontext.StateReady {
					if q.Get("callerUmt") != secret || q.Get("caller") != "dws" || len(q["callerUmt"]) != 1 || len(q["caller"]) != 1 {
						t.Fatal("wrong authorization runtime context")
					}
				} else if q.Has("callerUmt") || q.Has("caller") {
					t.Fatal("unavailable context changed authorization URL")
				}
				redirect, _ := url.Parse(q.Get("redirect_uri"))
				if redirect.Query().Has("callerUmt") || redirect.Query().Has("caller") {
					t.Fatal("redirect URI contains context")
				}
				if !strings.Contains(notEnabledHTML, "backLink.href = authorizeUrl;") || strings.Contains(notEnabledHTML, `"?client_id="`) {
					t.Fatal("page must use server URL")
				}
				if resolutions.Load() != 1 {
					t.Fatal("OAuth snapshot resolved repeatedly")
				}
				if strings.Count(output.String(), status.AuthorizeURL) != 1 {
					t.Fatal("manual authorization link differs from browser/reauthorization URL")
				}
				assertAuthRuntimeValueAbsent(t, logs.String(), secret)
				assertAuthRuntimeValueAbsent(t, strings.ReplaceAll(output.String(), status.AuthorizeURL, ""), secret)
			})
		}
	}
}

func TestCrossPlatformCoverageDeviceRuntimeURLRetrySnapshot(t *testing.T) {
	for _, state := range []runtimecontext.State{runtimecontext.StateReady, runtimecontext.StateUnavailable, runtimecontext.StateTimeout, runtimecontext.StateError} {
		for _, noBrowser := range []bool{false, true} {
			for _, links := range []struct{ name, base, complete string }{
				{"complete", "https://example.test/verify?source=cli#manual", "https://example.test/verify?user_code=ABCD&callerUmt=old&callerUmt=duplicate&caller=old#section"},
				{"base_only", "https://example.test/verify#manual", ""},
				{"invalid", "https://example.test/verify?bad=%zz#manual", "https://example.test/verify?bad=%zz#section"},
			} {
				t.Run(fmt.Sprintf("%s/no_browser=%t/%s", state, noBrowser, links.name), func(t *testing.T) {
					isolateOAuthPersistence(t)
					SetClientID("")
					SetClientSecret("")
					resetClientIDFromMCP()
					t.Cleanup(func() { SetClientID(""); SetClientSecret(""); resetClientIDFromMCP() })
					secret := "private-device+context/&=测试"
					snapshot := runtimecontext.Result{State: state}
					if state == runtimecontext.StateReady {
						snapshot = runtimecontext.ReadyResultForTest(secret)
					}
					resolutions := 0
					testseam.Swap(t, &resolveAuthRuntimeContext, func() runtimecontext.Result { resolutions++; return snapshot })
					testseam.Swap(t, &deviceFetchClientID, func(context.Context) (string, error) { return "client", nil })
					response := &DeviceAuthResponse{VerificationURIComplete: links.complete, VerificationURI: links.base, UserCode: "ABCD", Interval: 1}
					original := *response
					testseam.Swap(t, &deviceRequestCode, func(*DeviceFlowProvider, context.Context) (*DeviceAuthResponse, error) { return response, nil })
					waits := 0
					testseam.Swap(t, &deviceWaitAuth, func(_ *DeviceFlowProvider, _ context.Context, got *DeviceAuthResponse) (*DeviceTokenResponse, error) {
						waits++
						if got != response || *got != original {
							t.Fatal("polling response mutated")
						}
						return nil, errors.New("invalid_grant")
					})
					var manualURL, completeURL string
					var attached bool
					opens := 0
					testseam.Swap(t, &deviceOpenBrowser, func(got string) error {
						opens++
						if got != completeURL {
							t.Fatal("browser and manual device URLs differ")
						}
						if attached {
							parsed, err := url.Parse(got)
							if err != nil {
								t.Fatal(err)
							}
							q := parsed.Query()
							if q.Get("user_code") != "ABCD" || parsed.Fragment != "section" {
								t.Fatal("verification URL changed")
							}
							if q.Get("callerUmt") != secret || q.Get("caller") != "dws" || len(q["callerUmt"]) != 1 || len(q["caller"]) != 1 {
								t.Fatal("wrong device context")
							}
						}
						return errors.New(got)
					})
					var output, logs bytes.Buffer
					p := NewDeviceFlowProvider(t.TempDir(), slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
					p.Output = &output
					p.NoBrowser = noBrowser
					trusted := TrustedLoginHostsForRegion(p.LoginRegion)
					manualURL, _ = snapshot.AttachToURL(links.base, trusted)
					completeURL, attached = snapshot.AttachToURL(links.complete, trusted)
					if _, err := p.Login(context.Background()); err == nil || !isInvalidGrantError(err) {
						t.Fatal("unexpected retry outcome")
					}
					wantOpens := 0
					if !noBrowser && completeURL != "" {
						wantOpens = 3
					}
					if opens != wantOpens || resolutions != 1 || waits != 3 {
						t.Fatalf("opens=%d resolutions=%d waits=%d", opens, resolutions, waits)
					}
					if strings.Count(output.String(), manualURL) != 3 {
						t.Fatal("manual device URL missing from retries")
					}
					remaining := strings.ReplaceAll(output.String(), manualURL, "")
					if completeURL != "" {
						if strings.Count(output.String(), completeURL) != 3 {
							t.Fatal("complete device URL missing from retries")
						}
						remaining = strings.ReplaceAll(remaining, completeURL, "")
					}
					assertAuthRuntimeValueAbsent(t, logs.String(), secret)
					assertAuthRuntimeValueAbsent(t, remaining, secret)
				})
			}
		}
	}
}

func assertAuthRuntimeValueAbsent(t *testing.T, text, secret string) {
	t.Helper()
	if strings.Contains(text, secret) || strings.Contains(text, url.QueryEscape(secret)) || strings.Contains(text, "callerUmt") {
		t.Fatal("runtime value escaped the authorized manual links")
	}
}

func TestCrossPlatformCoverageDeviceRuntimeURLUntrustedHostKeepsOriginal(t *testing.T) {
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
				if got != raw || q.Has("callerUmt") || q.Has("caller") {
					t.Fatal("untrusted verification host must keep the original URL")
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

func TestCrossPlatformCoverageDeviceRuntimeURLAttachesOnTrustedHost(t *testing.T) {
	isolateOAuthPersistence(t)
	SetClientID("")
	SetClientSecret("")
	resetClientIDFromMCP()
	t.Cleanup(func() { SetClientID(""); SetClientSecret(""); resetClientIDFromMCP() })
	secret := "private-device+trusted/&="
	snapshot := runtimecontext.ReadyResultForTest(secret)
	testseam.Swap(t, &resolveAuthRuntimeContext, func() runtimecontext.Result { return snapshot })
	testseam.Swap(t, &deviceFetchClientID, func(context.Context) (string, error) { return "client", nil })
	raw := "https://login.dingtalk.com/oauth2/device/verify?user_code=ABCD#section"
	response := &DeviceAuthResponse{VerificationURIComplete: raw, VerificationURI: "https://login.dingtalk.com/oauth2/device/verify", UserCode: "ABCD", Interval: 1}
	testseam.Swap(t, &deviceRequestCode, func(*DeviceFlowProvider, context.Context) (*DeviceAuthResponse, error) { return response, nil })
	testseam.Swap(t, &deviceWaitAuth, func(*DeviceFlowProvider, context.Context, *DeviceAuthResponse) (*DeviceTokenResponse, error) {
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
		if q.Get("user_code") != "ABCD" || parsed.Fragment != "section" || q.Get("callerUmt") != secret || q.Get("caller") != "dws" {
			t.Fatalf("trusted host did not attach runtime context: %s", got)
		}
		return errors.New(got)
	})
	var output, logs bytes.Buffer
	p := NewDeviceFlowProvider(t.TempDir(), slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	p.Output = &output
	if _, err := p.Login(context.Background()); err == nil || !isInvalidGrantError(err) {
		t.Fatal("unexpected retry outcome")
	}
	if opens != 3 {
		t.Fatalf("opens=%d", opens)
	}
	if !strings.Contains(output.String(), "callerUmt="+url.QueryEscape(secret)) {
		t.Fatal("trusted manual device URL missing attached runtime context")
	}
	if strings.Contains(logs.String(), secret) || strings.Contains(logs.String(), url.QueryEscape(secret)) {
		t.Fatal("device value leaked into logs")
	}
}
