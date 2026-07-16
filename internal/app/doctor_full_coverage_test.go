package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	upgradepkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/upgrade"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCrossPlatformCoverageDoctorRemainingCoverage(t *testing.T) {
	oldEdition := edition.Get()
	oldDiagnose := doctorKeychainDiagnose
	oldStatus := doctorAuthStatus
	oldAccess := doctorAuthAccessToken
	oldHTTP := doctorHTTPDo
	oldLatest := doctorFetchLatestRelease
	oldNeeds := doctorNeedsUpgrade
	oldRead := timingReadFile
	t.Cleanup(func() {
		edition.Override(oldEdition)
		doctorKeychainDiagnose = oldDiagnose
		doctorAuthStatus = oldStatus
		doctorAuthAccessToken = oldAccess
		doctorHTTPDo = oldHTTP
		doctorFetchLatestRelease = oldLatest
		doctorNeedsUpgrade = oldNeeds
		timingReadFile = oldRead
	})
	edition.Override(&edition.Hooks{})
	doctorKeychainDiagnose = func() keychain.Diagnostic { return keychain.Diagnostic{OK: true, Message: "ok"} }
	buf := &bytes.Buffer{}

	doctorAuthStatus = func(*authpkg.OAuthProvider) (*authpkg.TokenData, error) { return nil, nil }
	if got := doctorCheckAuth(context.Background(), buf, false); got.Status != statusFail || got.Hint == "" {
		t.Fatalf("missing auth = %#v", got)
	}
	edition.Override(&edition.Hooks{IsEmbedded: true})
	if got := doctorCheckAuth(context.Background(), io.Discard, true); got.Status != statusFail || got.Hint != "" {
		t.Fatalf("embedded missing auth = %#v", got)
	}
	edition.Override(&edition.Hooks{})
	now := time.Now()
	valid := &authpkg.TokenData{AccessToken: "a", ExpiresAt: now.Add(time.Hour)}
	doctorAuthStatus = func(*authpkg.OAuthProvider) (*authpkg.TokenData, error) { return valid, nil }
	if got := doctorCheckAuth(context.Background(), buf, false); got.Status != statusPass {
		t.Fatalf("valid auth = %#v", got)
	}
	refresh := &authpkg.TokenData{RefreshToken: "r", RefreshExpAt: now.Add(time.Hour)}
	doctorAuthStatus = func(*authpkg.OAuthProvider) (*authpkg.TokenData, error) { return refresh, nil }
	doctorAuthAccessToken = func(*authpkg.OAuthProvider, context.Context) (string, error) { return "", errors.New("refresh") }
	if got := doctorCheckAuth(context.Background(), buf, false); got.Status != statusWarn {
		t.Fatalf("refresh failure = %#v", got)
	}
	doctorAuthAccessToken = func(*authpkg.OAuthProvider, context.Context) (string, error) { return "a", nil }
	if got := doctorCheckAuth(context.Background(), buf, false); got.Status != statusPass {
		t.Fatalf("refresh success = %#v", got)
	}
	expired := &authpkg.TokenData{AccessToken: "a", ExpiresAt: now.Add(-time.Hour)}
	doctorAuthStatus = func(*authpkg.OAuthProvider) (*authpkg.TokenData, error) { return expired, nil }
	if got := doctorCheckAuth(context.Background(), buf, false); got.Status != statusFail || got.Hint == "" {
		t.Fatalf("expired auth = %#v", got)
	}

	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)
	if err := os.WriteFile(filepath.Join(configDir, "mcp_url"), []byte(":"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := doctorCheckNetwork(context.Background(), buf, false, time.Second); got.Status != statusFail {
		t.Fatalf("invalid network URL = %#v", got)
	}
	if err := os.WriteFile(filepath.Join(configDir, "mcp_url"), []byte("https://example.test"), 0o600); err != nil {
		t.Fatal(err)
	}
	doctorHTTPDo = func(*http.Client, *http.Request) (*http.Response, error) { return nil, errors.New("network") }
	if got := doctorCheckNetwork(context.Background(), buf, false, time.Second); got.Status != statusFail {
		t.Fatalf("network failure = %#v", got)
	}
	doctorHTTPDo = func(*http.Client, *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	}
	if got := doctorCheckNetwork(context.Background(), buf, false, time.Second); got.Status != statusPass {
		t.Fatalf("network success = %#v", got)
	}

	doctorFetchLatestRelease = func() (*upgradepkg.ReleaseInfo, error) { return nil, errors.New("latest") }
	if got := doctorCheckVersion(buf, false, time.Second); got.Status != statusFail {
		t.Fatalf("version failure = %#v", got)
	}
	doctorFetchLatestRelease = func() (*upgradepkg.ReleaseInfo, error) { return &upgradepkg.ReleaseInfo{Version: "99.0.0"}, nil }
	doctorNeedsUpgrade = func(string, string) bool { return true }
	if got := doctorCheckVersion(buf, false, time.Second); got.Status != statusWarn {
		t.Fatalf("version warning = %#v", got)
	}
	doctorNeedsUpgrade = func(string, string) bool { return false }
	if got := doctorCheckVersion(buf, false, time.Second); got.Status != statusPass {
		t.Fatalf("version pass = %#v", got)
	}

	doctorAuthStatus = func(*authpkg.OAuthProvider) (*authpkg.TokenData, error) { return valid, nil }
	doctorFetchLatestRelease = func() (*upgradepkg.ReleaseInfo, error) { return &upgradepkg.ReleaseInfo{Version: version}, nil }
	timingReadFile = func(string) ([]byte, error) {
		return []byte(`{"command":"test","timestamp":"2026-01-01T00:00:00Z","phases":[]}`), nil
	}
	doctor := newDoctorCommand()
	doctor.SetContext(context.Background())
	doctor.SetOut(io.Discard)
	_ = doctor.Flags().Set("json", "true")
	_ = doctor.Flags().Set("perf", "true")
	_ = doctor.Flags().Set("timeout", "0")
	if err := doctor.RunE(doctor, nil); err != nil {
		t.Fatal(err)
	}

	doctorAuthStatus = func(*authpkg.OAuthProvider) (*authpkg.TokenData, error) { return nil, errors.New("not logged in") }
	doctor = newDoctorCommand()
	doctor.SetContext(context.Background())
	doctor.SetOut(io.Discard)
	if err := doctor.RunE(doctor, nil); err == nil {
		t.Fatal("doctor failures should return an error")
	}
}
