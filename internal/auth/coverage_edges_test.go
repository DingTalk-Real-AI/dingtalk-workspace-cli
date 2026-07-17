package auth

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("read") }

type duplicateFailAfterWriter struct {
	remaining int
}

func (w *duplicateFailAfterWriter) Write(p []byte) (int, error) {
	if w.remaining <= 0 {
		return 0, errors.New("write")
	}
	if len(p) > w.remaining {
		n := w.remaining
		w.remaining = 0
		return n, errors.New("write")
	}
	w.remaining -= len(p)
	return len(p), nil
}

type duplicateFakePortableTemp struct {
	name      string
	writeErr  error
	chmodErr  error
	syncErr   error
	closeErr  error
	closeCall int
}

func (f *duplicateFakePortableTemp) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return len(p), nil
}
func (f *duplicateFakePortableTemp) Name() string            { return f.name }
func (f *duplicateFakePortableTemp) Chmod(os.FileMode) error { return f.chmodErr }
func (f *duplicateFakePortableTemp) Sync() error             { return f.syncErr }
func (f *duplicateFakePortableTemp) Close() error            { f.closeCall++; return f.closeErr }

type fakeSecureTemp struct {
	name     string
	writeErr error
	syncErr  error
	closeErr error
}

func (f *fakeSecureTemp) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return len(p), nil
}
func (f *fakeSecureTemp) Name() string { return f.name }
func (f *fakeSecureTemp) Sync() error  { return f.syncErr }
func (f *fakeSecureTemp) Close() error { return f.closeErr }

type failAfterWriter struct {
	remaining int
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	if w.remaining <= 0 {
		return 0, errors.New("write")
	}
	if len(p) > w.remaining {
		n := w.remaining
		w.remaining = 0
		return n, errors.New("write")
	}
	w.remaining -= len(p)
	return len(p), nil
}

type fakePortableTemp struct {
	name      string
	writeErr  error
	chmodErr  error
	syncErr   error
	closeErr  error
	closeCall int
}

func (f *fakePortableTemp) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return len(p), nil
}
func (f *fakePortableTemp) Name() string            { return f.name }
func (f *fakePortableTemp) Chmod(os.FileMode) error { return f.chmodErr }
func (f *fakePortableTemp) Sync() error             { return f.syncErr }
func (f *fakePortableTemp) Close() error            { f.closeCall++; return f.closeErr }

func TestCrossPlatformCoverageAppTokenEdges(t *testing.T) {
	if err := SaveAppTokenData(&AppTokenData{}); err == nil {
		t.Fatal("empty client ID save succeeded")
	}
	if _, err := LoadAppTokenData(""); err == nil {
		t.Fatal("empty client ID load succeeded")
	}
	if err := DeleteAppTokenData(""); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadAppTokenData("missing"); err != nil || got != nil {
		t.Fatalf("missing app token = %#v, %v", got, err)
	}
	data := &AppTokenData{ClientID: "app-one", AccessToken: "cached", ExpiresAt: time.Now().Add(time.Hour)}
	if err := SaveAppTokenData(data); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadAppTokenData("app-one")
	if err != nil || loaded.AccessToken != "cached" || loaded.UpdatedAt.IsZero() {
		t.Fatalf("loaded app token = %#v, %v", loaded, err)
	}
	if err := keychain.Set(keychain.Service, appTokenPrefix+"malformed", "{"); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAppTokenData("malformed"); err == nil {
		t.Fatal("malformed app token succeeded")
	}
	if err := DeleteAppTokenData("app-one"); err != nil {
		t.Fatal(err)
	}

	oldClient := appTokenHTTPClient
	t.Cleanup(func() { appTokenHTTPClient = oldClient })
	response := func(status int, body string) {
		appTokenHTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: status,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
			}, nil
		})}
	}
	response(http.StatusOK, "{\"accessToken\":\"fresh\",\"expireIn\":0}")
	token, expires, err := FetchAppToken(context.Background(), "key", "secret")
	if err != nil || token != "fresh" || expires <= 0 {
		t.Fatalf("FetchAppToken = %q %d %v", token, expires, err)
	}
	response(http.StatusBadGateway, strings.Repeat("x", 250))
	if _, _, err := FetchAppToken(context.Background(), "key", "secret"); err == nil || !strings.Contains(err.Error(), "...") {
		t.Fatalf("HTTP app token error = %v", err)
	}
	response(http.StatusOK, "{")
	if _, _, err := FetchAppToken(context.Background(), "key", "secret"); err == nil {
		t.Fatal("malformed app token response succeeded")
	}
	response(http.StatusOK, "{}")
	if _, _, err := FetchAppToken(context.Background(), "key", "secret"); err == nil {
		t.Fatal("empty app token response succeeded")
	}
	appTokenHTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network")
	})}
	if _, _, err := FetchAppToken(context.Background(), "key", "secret"); err == nil {
		t.Fatal("network app token request succeeded")
	}
	appTokenHTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(errorReader{}), Header: make(http.Header)}, nil
	})}
	if _, _, err := FetchAppToken(context.Background(), "key", "secret"); err == nil {
		t.Fatal("app token read failure succeeded")
	}

	provider := &AppTokenProvider{}
	if _, err := provider.GetToken(context.Background()); err == nil {
		t.Fatal("missing app credentials succeeded")
	}
	cached := &AppTokenData{ClientID: "cache-key", AccessToken: "cached", ExpiresAt: time.Now().Add(time.Hour)}
	if err := SaveAppTokenData(cached); err != nil {
		t.Fatal(err)
	}
	provider = &AppTokenProvider{AppKey: "cache-key", AppSecret: "secret"}
	if got, err := provider.GetToken(context.Background()); err != nil || got != "cached" {
		t.Fatalf("cached provider token = %q, %v", got, err)
	}
	response(http.StatusOK, "{\"accessToken\":\"provider-fresh\",\"expireIn\":7200}")
	provider = &AppTokenProvider{AppKey: "fresh-key", AppSecret: "secret"}
	if got, err := provider.GetToken(context.Background()); err != nil || got != "provider-fresh" {
		t.Fatalf("fresh provider token = %q, %v", got, err)
	}
	response(http.StatusInternalServerError, "down")
	provider = &AppTokenProvider{AppKey: "failed-key", AppSecret: "secret"}
	if _, err := provider.GetToken(context.Background()); err == nil {
		t.Fatal("failed provider fetch succeeded")
	}
}

func TestCrossPlatformCoverageDeviceFlowRequestAndOutputEdges(t *testing.T) {
	var buf bytes.Buffer
	p := &DeviceFlowProvider{Output: &buf, clientID: "client", scope: "scope", httpClient: http.DefaultClient}
	p.SetScope("custom")
	(*DeviceFlowProvider)(nil).SetScope("ignored")
	if p.scope != "custom" || p.output() != &buf || (*DeviceFlowProvider)(nil).output() != io.Discard {
		t.Fatal("device flow setters/output failed")
	}

	type deviceMode string
	var mode deviceMode
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch mode {
		case "malformed":
			_, _ = io.WriteString(w, "{")
		case "service-error":
			writeServiceResult(w, false, nil, "bad", "failed")
		case "bad-result":
			_, _ = io.WriteString(w, "{\"success\":true,\"result\":\"bad\"}")
		case "empty":
			writeServiceResult(w, true, DeviceAuthResponse{}, "", "")
		case "defaults":
			writeServiceResult(w, true, DeviceAuthResponse{DeviceCode: "dc", UserCode: "uc", Interval: 99, ExpiresIn: -1}, "", "")
		case "token-ok":
			writeServiceResult(w, true, DeviceTokenResponse{AuthCode: "code"}, "", "")
		case "poll-fail":
			_ = json.NewEncoder(w).Encode(DevicePollResponse{Success: false, Code: "bad", Message: "failed"})
		case "poll-business":
			_ = json.NewEncoder(w).Encode(DevicePollResponse{Success: false, Data: DevicePollData{Status: StatusRejected}})
		default:
			http.Error(w, "down", http.StatusBadGateway)
		}
	}))
	defer srv.Close()
	p.SetBaseURL(srv.URL)
	p.SetTerminalBaseURL(srv.URL)
	p.httpClient = srv.Client()

	for _, m := range []deviceMode{"malformed", "service-error", "bad-result", "empty"} {
		mode = m
		if _, err := p.requestDeviceCode(context.Background()); err == nil {
			t.Errorf("requestDeviceCode mode %q succeeded", m)
		}
	}
	mode = "defaults"
	if got, err := p.requestDeviceCode(context.Background()); err != nil || got.Interval != defaultPollInterval || got.ExpiresIn != 900 {
		t.Fatalf("request defaults = %#v, %v", got, err)
	}
	for _, m := range []deviceMode{"malformed", "service-error", "bad-result"} {
		mode = m
		if _, err := p.pollDeviceToken(context.Background(), "dc"); err == nil {
			t.Errorf("pollDeviceToken mode %q succeeded", m)
		}
	}
	mode = "token-ok"
	if got, err := p.pollDeviceToken(context.Background(), "dc"); err != nil || got.AuthCode != "code" {
		t.Fatalf("poll token = %#v, %v", got, err)
	}
	mode = "malformed"
	if _, err := p.pollDeviceStatus(context.Background(), "flow"); err == nil {
		t.Fatal("malformed poll status succeeded")
	}
	mode = "poll-fail"
	if _, err := p.pollDeviceStatus(context.Background(), "flow"); err == nil {
		t.Fatal("failed poll status succeeded")
	}
	mode = "poll-business"
	if got, err := p.pollDeviceStatus(context.Background(), "flow"); err != nil || got.Data.Status != StatusRejected {
		t.Fatalf("business poll status = %#v, %v", got, err)
	}

	badClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network")
	})}
	p.httpClient = badClient
	if _, err := p.postForm(context.Background(), "://bad", url.Values{}); err == nil {
		t.Fatal("invalid form URL succeeded")
	}
	if _, err := p.postForm(context.Background(), "https://example.test", url.Values{}); err == nil {
		t.Fatal("form network error succeeded")
	}
	if _, err := p.doGet(context.Background(), "://bad"); err == nil {
		t.Fatal("invalid GET URL succeeded")
	}
	if _, err := p.doGet(context.Background(), "https://example.test"); err == nil {
		t.Fatal("GET network error succeeded")
	}
	readClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(errorReader{}), Header: make(http.Header)}, nil
	})}
	p.httpClient = readClient
	if _, err := p.postForm(context.Background(), "https://example.test", url.Values{}); err == nil {
		t.Fatal("form read error succeeded")
	}
	if _, err := p.doGet(context.Background(), "https://example.test"); err == nil {
		t.Fatal("GET read error succeeded")
	}

	if got := truncateBody([]byte("short"), 10); got != "short" {
		t.Fatalf("short truncate = %q", got)
	}
	if got := truncateBody([]byte("long-value"), 4); got != "long...(truncated)" {
		t.Fatalf("long truncate = %q", got)
	}
	dfPrintStep(&buf, 1, "step", 2)
	dfPrintDeviceCodeBox(&buf, &DeviceAuthResponse{UserCode: "code", VerificationURI: "url", VerificationURIComplete: "complete", ExpiresIn: 1})
	dfPrintBox(&buf, []string{strings.Repeat("x", 55)})
	dfPrintPollResult(&buf, "authorized", "ok")
	dfPrintPollResult(&buf, "pending", "wait")
	dfPrintPollResult(&buf, "unknown", "bad")
	dfPrintWarning(&buf, "warning")
	if isInvalidGrantError(nil) || !isInvalidGrantError(errors.New("invalid_grant")) ||
		!isInvalidGrantError(errors.New("code expired")) || isInvalidGrantError(errors.New("other")) {
		t.Fatal("invalid grant detection failed")
	}
}

func TestCrossPlatformCoverageDeviceFlowWaitImmediateEdges(t *testing.T) {
	p := &DeviceFlowProvider{Output: io.Discard}
	if _, err := p.waitForAuthorizationByFlowID(context.Background(), &DeviceAuthResponse{ExpiresIn: 0}); err == nil {
		t.Fatal("flow deadline succeeded")
	}
	if _, err := p.waitForAuthorizationByDeviceCode(context.Background(), &DeviceAuthResponse{ExpiresIn: 0}); err == nil {
		t.Fatal("device deadline succeeded")
	}
	oldAfter := deviceFlowAfter
	t.Cleanup(func() { deviceFlowAfter = oldAfter })
	deviceFlowAfter = func(time.Duration) <-chan time.Time {
		return make(chan time.Time)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.waitForAuthorizationByFlowID(ctx, &DeviceAuthResponse{ExpiresIn: 10, Interval: 1, FlowID: "f"}); err == nil {
		t.Fatal("canceled flow wait succeeded")
	}
	if _, err := p.waitForAuthorizationByDeviceCode(ctx, &DeviceAuthResponse{ExpiresIn: 10, Interval: 1, DeviceCode: "d"}); err == nil {
		t.Fatal("canceled device wait succeeded")
	}
}

func TestCrossPlatformCoverageDeviceFlowLoginRetry(t *testing.T) {
	oldClient := oauthHTTPClient
	oldAfter := deviceFlowAfter
	oldBrowser := deviceOpenBrowser
	t.Cleanup(func() {
		oauthHTTPClient = oldClient
		deviceFlowAfter = oldAfter
		deviceOpenBrowser = oldBrowser
		SetClientID("")
		SetClientSecret("")
		resetClientIDFromMCP()
		SetClientID("")
	})
	deviceFlowAfter = func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}
	deviceOpenBrowser = func(string) error { return errors.New("browser") }
	var exchanges int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, ClientIDPath):
			_ = json.NewEncoder(w).Encode(ClientIDResponse{Success: true, Result: "device-client"})
		case strings.HasSuffix(r.URL.Path, DeviceCodePath):
			writeServiceResult(w, true, DeviceAuthResponse{
				DeviceCode: "dc", UserCode: "uc", VerificationURI: "https://verify",
				VerificationURIComplete: "https://verify/full", ExpiresIn: 60, Interval: 1, FlowID: "flow",
			}, "", "")
		case strings.HasSuffix(r.URL.Path, DevicePollPath):
			_ = json.NewEncoder(w).Encode(DevicePollResponse{Success: true, Data: DevicePollData{Status: StatusApproved, AuthCode: "code"}})
		case strings.HasSuffix(r.URL.Path, MCPOAuthTokenPath):
			exchanges++
			if exchanges < 3 {
				http.Error(w, "invalid_grant code expired", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accessToken": "device-access", "refreshToken": "refresh", "expiresIn": 7200,
				"corpId": "corp-device", "userId": "user-device",
			})
		case strings.HasSuffix(r.URL.Path, CLIAuthEnabledPath):
			_ = json.NewEncoder(w).Encode(CLIAuthStatus{Success: true, Result: &CLIAuthResult{CLIAuthEnabled: true}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	dir := setupMCPConfigDir(t, srv.URL)
	oauthHTTPClient = srv.Client()
	p := NewDeviceFlowProvider(dir, slog.New(slog.NewTextHandler(io.Discard, nil)))
	p.SetBaseURL(srv.URL)
	p.SetTerminalBaseURL(srv.URL)
	p.httpClient = srv.Client()
	p.Output = io.Discard
	token, err := p.Login(context.Background())
	if err != nil || token.AccessToken != "device-access" || exchanges != 3 {
		t.Fatalf("device Login = %#v, %v, exchanges=%d", token, err, exchanges)
	}
}

func TestCrossPlatformCoveragePortablePathHelpers(t *testing.T) {
	if _, err := cleanPortableName("../escape"); err == nil {
		t.Fatal("unsafe portable name succeeded")
	}
	if got, err := cleanPortableName("./a/b"); err != nil || got != "a/b" {
		t.Fatalf("clean portable name = %q, %v", got, err)
	}
	root := t.TempDir()
	if _, err := safeJoin(root, "../escape"); err == nil {
		t.Fatal("unsafe join succeeded")
	}
	if got, err := safeJoin(root, "a/b"); err != nil || got != filepath.Join(root, "a", "b") {
		t.Fatalf("safe join = %q, %v", got, err)
	}
	if rel := mustPortableRel(root, filepath.Join(root, "a", "b")); rel != "a/b" {
		t.Fatalf("portable rel = %q", rel)
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := writePortableBytes(tw, "file", []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(bytes.NewReader(buf.Bytes()))
	hdr, err := tr.Next()
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "nested", "file")
	if err := extractPortableEntry(target, hdr, tr); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "x" {
		t.Fatalf("extracted file = %q, %v", data, err)
	}
	if err := extractPortableEntry(target, &tar.Header{Typeflag: tar.TypeSymlink}, strings.NewReader("")); err == nil {
		t.Fatal("unsupported portable entry succeeded")
	}
}

func TestCrossPlatformCoverageProfilesLifecycleEdges(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadProfiles(dir)
	if err != nil || cfg.Version != 1 || len(cfg.Profiles) != 0 {
		t.Fatalf("missing profiles = %#v, %v", cfg, err)
	}
	if err := os.WriteFile(ProfilesPath(dir), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = LoadProfiles(dir)
	if err != nil || len(cfg.Profiles) != 0 {
		t.Fatalf("corrupt profiles = %#v, %v", cfg, err)
	}
	if matches, _ := filepath.Glob(ProfilesPath(dir) + ".corrupt-*"); len(matches) != 1 {
		t.Fatalf("quarantined profiles = %#v", matches)
	}
	if err := SaveProfiles(dir, nil); err != nil {
		t.Fatal(err)
	}

	cfg = &ProfilesConfig{
		PrimaryProfile:  "missing",
		CurrentProfile:  "missing",
		PreviousProfile: "missing",
		Profiles: []Profile{
			{CorpID: " corp-a ", CorpName: "Acme"},
			{CorpID: "corp-a", Name: "duplicate"},
			{CorpID: ""},
		},
	}
	normalizeProfilesConfig(cfg)
	if len(cfg.Profiles) != 1 || cfg.Profiles[0].Name != "Acme" ||
		cfg.PrimaryProfile != "" || cfg.CurrentProfile != "" || cfg.PreviousProfile != "" {
		t.Fatalf("normalized profiles = %#v", cfg)
	}

	if err := upsertProfileFromToken(dir, cfg, nil, true); err != nil {
		t.Fatal(err)
	}
	if err := upsertProfileFromToken(dir, cfg, &TokenData{}, true); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	first := &TokenData{
		CorpID: "corp-a", CorpName: "Acme Updated", UserID: "u1", UserName: "User",
		ClientID: "client", ExpiresAt: now.Add(time.Hour), RefreshExpAt: now.Add(24 * time.Hour),
	}
	if err := UpsertProfileFromToken(dir, first); err != nil {
		t.Fatal(err)
	}
	second := &TokenData{CorpID: "corp-b", CorpName: "Acme Updated", AccessToken: "b"}
	if err := UpsertProfileFromTokenWithCurrent(dir, second, true); err != nil {
		t.Fatal(err)
	}
	if err := SaveTokenDataKeychainForIdentity(first.CorpID, first.UserID, first); err != nil {
		t.Fatal(err)
	}
	if err := SaveTokenDataKeychainForCorpID(second.CorpID, second); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = DeleteTokenDataKeychainForIdentity(first.CorpID, first.UserID)
		_ = DeleteTokenDataKeychainForCorpID(second.CorpID)
	})
	profilesForAmbiguity, err := LoadProfiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i := range profilesForAmbiguity.Profiles {
		profilesForAmbiguity.Profiles[i].Name = "profile-" + profilesForAmbiguity.Profiles[i].CorpID
		profilesForAmbiguity.Profiles[i].CorpName = "Shared Corp"
	}
	if err := SaveProfiles(dir, profilesForAmbiguity); err != nil {
		t.Fatal(err)
	}
	if got, err := ResolveProfile(dir, "Shared Corp"); err == nil || got != nil {
		t.Fatalf("ambiguous corp name = %#v, %v", got, err)
	}
	if got, err := ResolveProfile(dir, "corp-b"); err != nil || got.CorpID != "corp-b" {
		t.Fatalf("explicit profile = %#v, %v", got, err)
	}
	if got, err := ResolveProfile(dir, "missing"); err == nil || got != nil {
		t.Fatalf("missing profile = %#v, %v", got, err)
	}
	if got, err := ResolveProfile(dir, ""); err != nil || got == nil {
		t.Fatalf("default profile = %#v, %v", got, err)
	}

	if _, err := SetCurrentProfile(dir, "missing"); err == nil {
		t.Fatal("setting missing profile succeeded")
	}
	if _, err := SetCurrentProfile(dir, "corp-a"); err != nil {
		t.Fatal(err)
	}
	if got, err := UsePreviousProfile(dir); err != nil || got.CorpID != "corp-b" {
		t.Fatalf("previous profile = %#v, %v", got, err)
	}
	if err := MarkProfileStatus(dir, "", ProfileStatusExpired); err != nil {
		t.Fatal(err)
	}
	if err := MarkProfileStatus(dir, "missing", ProfileStatusExpired); err != nil {
		t.Fatal(err)
	}
	if err := MarkProfileStatus(dir, "corp-a", ProfileStatusExpired); err != nil {
		t.Fatal(err)
	}
	if removed, err := RemoveProfile(dir, "corp-b"); err != nil || removed.CorpID != "corp-b" {
		t.Fatalf("removed profile = %#v, %v", removed, err)
	}
	if _, err := RemoveProfile(dir, "missing"); err == nil {
		t.Fatal("removing missing profile succeeded")
	}
	if _, err := RemoveProfile(dir, "corp-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := UsePreviousProfile(dir); err == nil {
		t.Fatal("empty previous profile succeeded")
	}

	nameCfg := &ProfilesConfig{Profiles: []Profile{
		{CorpID: "one", Name: "Acme"},
		{CorpID: "two", Name: "Acme-12345678"},
		{CorpID: "three", Name: "Acme-12345678-2"},
	}}
	if got := chooseProfileName(nameCfg, &TokenData{CorpID: "corp-12345678", CorpName: "Acme"}); got != "Acme-2" {
		t.Fatalf("collision profile name = %q", got)
	}
	if chooseProfileName(&ProfilesConfig{}, &TokenData{}) != "profile" {
		t.Fatal("fallback profile name failed")
	}
	if shouldRefreshProfileName(nil, first) || shouldRefreshProfileName(&Profile{}, nil) ||
		!shouldRefreshProfileName(&Profile{}, first) {
		t.Fatal("profile refresh-name decisions failed")
	}
	if shortCorpID("short") != "short" || shortCorpID("corp-12345678") != "12345678" {
		t.Fatal("profile formatting helpers failed")
	}
}

func TestCrossPlatformCoverageProfilesMigrationAndMirror(t *testing.T) {
	dir := t.TempDir()
	_ = DeleteTokenDataKeychain()
	legacy := &TokenData{CorpID: "legacy-corp", CorpName: "Legacy", AccessToken: "token"}
	if err := SaveTokenDataKeychain(legacy); err != nil {
		t.Fatal(err)
	}
	if err := EnsureProfilesMigration(dir); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadProfiles(dir)
	if err != nil || len(cfg.Profiles) != 1 {
		t.Fatalf("migrated profiles = %#v, %v", cfg, err)
	}
	if err := SyncLegacyTokenMirror(dir); err != nil {
		t.Fatal(err)
	}
	if err := DeleteTokenDataKeychainForCorpID("legacy-corp"); err != nil {
		t.Fatal(err)
	}
	if err := SyncLegacyTokenMirror(dir); err != nil {
		t.Fatal(err)
	}

	emptyDir := t.TempDir()
	_ = DeleteTokenDataKeychain()
	if err := EnsureProfilesMigration(emptyDir); err != nil {
		t.Fatal(err)
	}
	if err := SyncLegacyTokenMirror(emptyDir); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveProfileForLoad(emptyDir, "missing"); err == nil {
		t.Fatal("missing load profile succeeded")
	}
}

func TestCrossPlatformCoverageTokenEditionHooksAndCleanup(t *testing.T) {
	oldHooks := edition.Get()
	t.Cleanup(func() {
		edition.Override(oldHooks)
		SetRuntimeProfile("")
	})
	dir := t.TempDir()
	var saved []byte
	deleted := false
	edition.Override(&edition.Hooks{
		SaveToken: func(_ string, data []byte) error {
			saved = append([]byte(nil), data...)
			return nil
		},
		LoadToken: func(string) ([]byte, error) { return saved, nil },
		DeleteToken: func(string) error {
			deleted = true
			return nil
		},
	})
	data := &TokenData{AccessToken: "hook-token"}
	if err := SaveTokenData(dir, data); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadTokenData(dir); err != nil || got.AccessToken != "hook-token" {
		t.Fatalf("hook load = %#v, %v", got, err)
	}
	if _, err := LoadTokenDataForProfile(dir, "profile"); err == nil {
		t.Fatal("hook profile selection succeeded")
	}
	if err := DeleteTokenDataForProfile(dir, "profile"); err == nil {
		t.Fatal("hook profile delete succeeded")
	}
	if err := DeleteAllTokenData(dir); err != nil || !deleted {
		t.Fatalf("hook delete all = %v, deleted=%v", err, deleted)
	}
	edition.Override(&edition.Hooks{LoadToken: func(string) ([]byte, error) { return []byte("{"), nil }})
	if _, err := LoadTokenData(dir); err == nil {
		t.Fatal("malformed hook token succeeded")
	}
	edition.Override(&edition.Hooks{LoadToken: func(string) ([]byte, error) { return nil, errors.New("load") }})
	if _, err := LoadTokenData(dir); err == nil {
		t.Fatal("hook token read error succeeded")
	}
}

func TestCrossPlatformCoverageTokenAndKeychainEdges(t *testing.T) {
	dir := t.TempDir()
	if err := SaveTokenDataKeychainForCorpID("", &TokenData{}); err == nil {
		t.Fatal("empty corp token save succeeded")
	}
	if _, err := LoadTokenDataKeychainForCorpID(""); err == nil {
		t.Fatal("empty corp token load succeeded")
	}
	if err := DeleteTokenDataKeychainForCorpID(""); err == nil {
		t.Fatal("empty corp token delete succeeded")
	}
	if TokenDataExistsKeychainForCorpID("") {
		t.Fatal("empty corp token exists")
	}
	if _, err := loadTokenDataKeychainAccount("missing-account"); err == nil {
		t.Fatal("missing keychain account loaded")
	}
	if err := keychain.Set(keychain.Service, "bad-token", "{"); err != nil {
		t.Fatal(err)
	}
	if _, err := loadTokenDataKeychainAccount("bad-token"); err == nil {
		t.Fatal("malformed keychain token loaded")
	}
	if err := SaveClientSecret("", "secret"); err != nil {
		t.Fatal(err)
	}
	if err := SaveClientSecret("client", ""); err != nil {
		t.Fatal(err)
	}
	if err := SaveClientSecret("client", "secret"); err != nil || LoadClientSecret("client") != "secret" {
		t.Fatal("client secret round trip failed")
	}
	if LoadClientSecret("") != "" || LoadClientSecret("missing") != "" {
		t.Fatal("missing client secret was returned")
	}
	if err := DeleteClientSecret(""); err != nil {
		t.Fatal(err)
	}
	if err := DeleteClientSecret("client"); err != nil {
		t.Fatal(err)
	}

	migrationOnce = sync.Once{}
	migrationDone = false
	EnsureMigration(dir, slog.Default())
	if !IsMigrationDone() {
		t.Fatal("migration state not recorded")
	}

	badDir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(badDir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteTokenMarker(badDir); err == nil {
		t.Fatal("token marker in file dir succeeded")
	}
	if err := DeleteTokenMarker(dir); err != nil {
		t.Fatal(err)
	}
	if err := DeleteAllTokenData(dir); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageAppConfigDeleteReloadEdges(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(GetAppConfigPath(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAppConfig(dir); err == nil {
		t.Fatal("directory app config loaded")
	}
	if err := os.RemoveAll(GetAppConfigPath(dir)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(GetAppConfigPath(dir), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAppConfig(dir); err == nil {
		t.Fatal("malformed app config loaded")
	}
	if err := os.Remove(GetAppConfigPath(dir)); err != nil {
		t.Fatal(err)
	}
	config := &AppConfig{ClientID: "client", ClientSecret: PlainSecret("secret")}
	if err := SaveAppConfig(dir, config); err != nil {
		t.Fatal(err)
	}
	if got, err := ReloadAppConfig(dir); err != nil || got.ClientID != "client" {
		t.Fatalf("reload app config = %#v, %v", got, err)
	}
	if err := DeleteAppConfig(dir); err != nil {
		t.Fatal(err)
	}
	if HasAppConfig(dir) {
		t.Fatal("app config remains after delete")
	}
	if err := DeleteAppConfig(dir); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageBasicAuthCoverageEdges(t *testing.T) {
	t.Run("channel and status", func(t *testing.T) {
		t.Setenv(AgentCodeEnv, " agent ")
		if !AgentCodeEnvPresent() || !HostOwnsPATFlow() {
			t.Fatal("host-owned PAT flow was not detected")
		}
		for _, status := range []string{StatusApproved, StatusRejected, StatusExpired, StatusPending, StatusCancelled} {
			if ParseDeviceFlowStatus(status, true) != status {
				t.Fatalf("status %q was not preserved", status)
			}
		}
		if ParseDeviceFlowStatus("", false) != StatusExpired || ParseDeviceFlowStatus("future", true) != "future" {
			t.Fatal("device status fallback failed")
		}
	})

	t.Run("endpoints and sources", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("DWS_CONFIG_DIR", dir)
		t.Setenv("DWS_CLIENT_ID", "")
		t.Setenv("DWS_CLIENT_SECRET", "")
		resetAppConfigCache()
		SetClientID("")
		SetClientSecret("")
		resetClientIDFromMCP()
		oldHome := authUserHomeDir
		oldEdition := edition.Get()
		t.Cleanup(func() {
			authUserHomeDir = oldHome
			edition.Override(oldEdition)
			SetClientID("")
			SetClientSecret("")
			resetClientIDFromMCP()
			resetAppConfigCache()
		})

		if err := os.WriteFile(filepath.Join(dir, "terminal_url"), []byte("https://terminal.test"), 0o600); err != nil {
			t.Fatal(err)
		}
		if GetTerminalBaseURL() != "https://terminal.test" || !strings.Contains(GetDeveloperSettingsURL(), "terminal.test") {
			t.Fatal("terminal endpoint override failed")
		}
		if GetUserAccessTokenURL() != UserAccessTokenURL || GetRefreshTokenURL() != UserAccessTokenURL || GetRevokeTokenURL() != "" {
			t.Fatal("direct token endpoints failed")
		}
		SetClientIDFromMCP("mcp-client")
		if !strings.HasSuffix(GetUserAccessTokenURL(), MCPOAuthTokenPath) || !strings.HasSuffix(GetRefreshTokenURL(), MCPRefreshTokenPath) || !strings.HasSuffix(GetRevokeTokenURL(), MCPRevokeTokenPath) {
			t.Fatal("MCP token endpoints failed")
		}
		resetClientIDFromMCP()
		SetClientID("")

		SetClientSecret("runtime-secret")
		if resolveCredentialSource() != "flag" || !HasValidClientSecret() {
			t.Fatal("runtime credential source failed")
		}
		SetClientSecret("")
		if HasValidClientSecret() {
			t.Fatal("placeholder secret was accepted")
		}
		t.Setenv("DWS_CLIENT_ID", "env-id")
		if resolveCredentialSource() != "env" {
			t.Fatal("environment credential source failed")
		}
		t.Setenv("DWS_CLIENT_ID", "")
		resetAppConfigCache()
		if resolveCredentialSource() != "default" {
			t.Fatal("default credential source failed")
		}

		edition.Override(&edition.Hooks{Name: "coverage", AuthClientID: "edition-id"})
		if ClientID() != "edition-id" {
			t.Fatal("edition client ID failed")
		}
		edition.Override(oldEdition)
		resetAppConfigCache()
		if err := os.WriteFile(GetAppConfigPath(dir), []byte(`{"clientId":"app-id","clientSecret":"plain"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := ReloadAppConfig(dir); err != nil {
			t.Fatal(err)
		}
		resetAppConfigCache()
		if resolveCredentialSource() != "app" {
			t.Fatal("app credential source failed")
		}

		t.Setenv("DWS_CONFIG_DIR", "")
		authUserHomeDir = func() (string, error) { return "", errors.New("home") }
		if getDefaultConfigDir() != ".dws" {
			t.Fatal("home-directory fallback failed")
		}
	})

	t.Run("strict credentials", func(t *testing.T) {
		t.Setenv(EnvClientID, "env-id")
		t.Setenv(EnvClientSecret, "env-secret")
		if id, secret, idSource, secretSource, err := ResolveAppCredentialsStrict(t.TempDir()); err != nil || id != "env-id" || secret != "env-secret" || idSource != CredentialSourceEnv || secretSource != CredentialSourceEnv {
			t.Fatalf("env strict credentials = %q %q %q %q %v", id, secret, idSource, secretSource, err)
		}
		if EnvHalfSet() {
			t.Fatal("complete env pair reported as half set")
		}
		t.Setenv(EnvClientSecret, "")
		if !EnvHalfSet() {
			t.Fatal("half-set env pair was not detected")
		}
		t.Setenv(EnvClientID, "")

		badPath := filepath.Join(t.TempDir(), "config-file")
		if err := os.WriteFile(badPath, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, _, _, err := ResolveAppCredentialsStrict(badPath); err == nil {
			t.Fatal("unreadable app config succeeded")
		}
		missing := t.TempDir()
		if _, _, _, _, err := ResolveAppCredentialsStrict(missing); !errors.Is(err, ErrAppConfigMissing) {
			t.Fatalf("missing app config error = %v", err)
		}
		writeConfig := func(t *testing.T, value string) string {
			t.Helper()
			dir := t.TempDir()
			if err := os.WriteFile(GetAppConfigPath(dir), []byte(value), 0o600); err != nil {
				t.Fatal(err)
			}
			return dir
		}
		if _, _, _, _, err := ResolveAppCredentialsStrict(writeConfig(t, `{"clientId":"","clientSecret":"secret"}`)); !errors.Is(err, ErrClientIDEmpty) {
			t.Fatalf("empty client ID error = %v", err)
		}
		if _, _, _, _, err := ResolveAppCredentialsStrict(writeConfig(t, `{"clientId":"id","clientSecret":""}`)); !errors.Is(err, ErrClientSecretEmpty) {
			t.Fatalf("empty client secret error = %v", err)
		}
		if _, _, _, _, err := ResolveAppCredentialsStrict(writeConfig(t, `{"clientId":"id","clientSecret":{"source":"file","id":"/missing/secret"}}`)); !errors.Is(err, ErrSecretResolve) {
			t.Fatalf("secret resolve error = %v", err)
		}
		secretFile := filepath.Join(t.TempDir(), "secret")
		if err := os.WriteFile(secretFile, []byte(" file-secret \n"), 0o600); err != nil {
			t.Fatal(err)
		}
		dir := writeConfig(t, `{"clientId":"id","clientSecret":{"source":"file","id":`+fmt.Sprintf("%q", secretFile)+`}}`)
		if id, secret, idSource, secretSource, err := ResolveAppCredentialsStrict(dir); err != nil || id != "id" || secret != "file-secret" || idSource != CredentialSourceAppConfig || secretSource != CredentialSourcePlainConfig {
			t.Fatalf("file strict credentials = %q %q %q %q %v", id, secret, idSource, secretSource, err)
		}
	})

	t.Run("manager and secret", func(t *testing.T) {
		dir := t.TempDir()
		m := NewManager(dir, slog.Default())
		if err := m.SaveToken("1234567890"); err != nil {
			t.Fatal(err)
		}
		if err := m.SaveMCPURL("https://mcp.test"); err != nil {
			t.Fatal(err)
		}
		if token, source, err := m.GetToken(); err != nil || token != "1234567890" || source != "file" || !m.IsAuthenticated() {
			t.Fatalf("manager token = %q %q %v", token, source, err)
		}
		if rawURL, err := m.GetMCPURL(); err != nil || rawURL != "https://mcp.test" {
			t.Fatalf("manager MCP URL = %q %v", rawURL, err)
		}
		if ok, source, masked := m.Status(); !ok || source != "file" || masked != "1234...7890" {
			t.Fatalf("manager status = %v %q %q", ok, source, masked)
		}
		if maskToken("short") != "****" {
			t.Fatal("short token masking failed")
		}
		if err := m.DeleteToken(); err != nil || m.IsAuthenticated() {
			t.Fatal("manager token deletion failed")
		}
		if ok, _, _ := m.Status(); ok {
			t.Fatal("missing manager token reported authenticated")
		}
		if _, err := NewManager(t.TempDir(), nil).GetMCPURL(); err == nil {
			t.Fatal("missing manager MCP URL succeeded")
		}

		fileDir := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(fileDir, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		badManager := NewManager(filepath.Join(fileDir, "child"), nil)
		if err := badManager.SaveToken("x"); err == nil {
			t.Fatal("token mkdir failure succeeded")
		}
		if err := badManager.SaveMCPURL("x"); err == nil {
			t.Fatal("MCP URL mkdir failure succeeded")
		}
		writeDir := t.TempDir()
		if err := os.Mkdir(filepath.Join(writeDir, tokenFileName), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := NewManager(writeDir, nil).SaveToken("x"); err == nil {
			t.Fatal("token write failure succeeded")
		}
		if err := os.Mkdir(filepath.Join(writeDir, "mcp_url"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := NewManager(writeDir, nil).SaveMCPURL("x"); err == nil {
			t.Fatal("MCP URL write failure succeeded")
		}
		deleteDir := t.TempDir()
		if err := os.Mkdir(filepath.Join(deleteDir, tokenFileName), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(deleteDir, tokenFileName, "child"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := NewManager(deleteDir, nil).DeleteToken(); err == nil {
			t.Fatal("token remove failure succeeded")
		}

		if !PlainSecret("").IsZero() || PlainSecret("x").IsZero() {
			t.Fatal("secret zero detection failed")
		}
		secretPath := filepath.Join(t.TempDir(), "secret")
		if err := os.WriteFile(secretPath, []byte(" value \n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if value, err := ResolveSecret(SecretInput{Ref: &SecretRef{Source: "file", ID: secretPath}}); err != nil || value != "value" {
			t.Fatalf("file secret = %q %v", value, err)
		}
		if _, err := ResolveSecret(SecretInput{Ref: &SecretRef{Source: "file", ID: filepath.Join(t.TempDir(), "missing")}}); err == nil {
			t.Fatal("missing file secret succeeded")
		}
		if _, err := ResolveSecret(SecretInput{Ref: &SecretRef{Source: "future", ID: "x"}}); err == nil {
			t.Fatal("unknown secret source succeeded")
		}
		ref := SecretInput{Ref: &SecretRef{Source: "file", ID: secretPath}}
		if stored, err := StoreSecret("id", ref); err != nil || stored.Ref != ref.Ref {
			t.Fatal("existing secret reference changed")
		}
	})

	t.Run("headers and stale token", func(t *testing.T) {
		applyEditionEnterpriseCredentialHeaders(nil)
		oldEdition := edition.Get()
		t.Cleanup(func() { edition.Override(oldEdition) })
		edition.Override(&edition.Hooks{Name: "coverage"})
		req, _ := http.NewRequest(http.MethodGet, "https://example.test", nil)
		applyEditionEnterpriseCredentialHeaders(req)
		edition.Override(&edition.Hooks{Name: "coverage", EnterpriseCredentialHeaders: func(map[string]string) map[string]string {
			return map[string]string{" X-Test ": " value ", "": "ignored", "Empty": " "}
		}})
		applyEditionEnterpriseCredentialHeaders(req)
		if req.Header.Get("X-Test") != "value" || req.Header.Get("Empty") != "" {
			t.Fatalf("enterprise headers = %v", req.Header)
		}

		dir := t.TempDir()
		if err := MarkAccessTokenStale(dir); err == nil {
			t.Fatal("missing token stale mark succeeded")
		}
		if err := SaveTokenData(dir, &TokenData{}); err != nil {
			t.Fatal(err)
		}
		if err := MarkAccessTokenStale(dir); err != nil {
			t.Fatal(err)
		}
		data := &TokenData{AccessToken: "access", ExpiresAt: time.Now().Add(time.Hour)}
		if err := SaveTokenData(dir, data); err != nil {
			t.Fatal(err)
		}
		if err := MarkAccessTokenStale(dir); err != nil {
			t.Fatal(err)
		}
		loaded, err := LoadTokenData(dir)
		if err != nil || !loaded.ExpiresAt.Before(time.Now()) {
			t.Fatalf("stale token = %#v %v", loaded, err)
		}
	})
}

func TestCrossPlatformCoveragePortableAuthBundleCoverageEdges(t *testing.T) {
	makeBundle := func(t *testing.T, entries []tar.Header, bodies []string) []byte {
		t.Helper()
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		for i := range entries {
			hdr := entries[i]
			body := bodies[i]
			if hdr.Typeflag == 0 {
				hdr.Typeflag = tar.TypeReg
			}
			if hdr.Typeflag == tar.TypeReg {
				hdr.Size = int64(len(body))
			}
			if err := tw.WriteHeader(&hdr); err != nil {
				t.Fatal(err)
			}
			if body != "" {
				if _, err := tw.Write([]byte(body)); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}

	t.Run("platform and populated states", func(t *testing.T) {
		oldGOOS := portableRuntimeGOOS
		oldGet := authKeychainGet
		oldExists := authKeychainExists
		t.Cleanup(func() {
			portableRuntimeGOOS = oldGOOS
			authKeychainGet = oldGet
			authKeychainExists = oldExists
		})
		authKeychainGet = func(string, string) (string, error) { return "", nil }
		authKeychainExists = func(string, string) bool { return false }
		portableRuntimeGOOS = func() string { return "linux" }
		if !PortableExportSupported() {
			t.Fatal("Linux portable export was disabled")
		}
		portableRuntimeGOOS = func() string { return "darwin" }
		t.Setenv(keychain.DisableKeychainEnv, "")
		if PortableExportSupported() {
			t.Fatal("macOS system-keychain export was enabled")
		}
		t.Setenv(keychain.DisableKeychainEnv, "1")
		if !PortableExportSupported() {
			t.Fatal("macOS file-keychain export was disabled")
		}

		keyDir := t.TempDir()
		t.Setenv(keychain.StorageDirEnv, keyDir)
		configDir := t.TempDir()
		if PortableAuthTargetPopulated(configDir) || PortableAuthSourceReady() {
			t.Fatal("empty portable auth target reported populated")
		}
		if err := os.WriteFile(ProfilesPath(configDir), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		if !PortableAuthTargetPopulated(configDir) {
			t.Fatal("profiles target was not detected")
		}
		if err := os.Remove(ProfilesPath(configDir)); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(configDir, "app.json"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		if !PortableAuthTargetPopulated(configDir) {
			t.Fatal("app target was not detected")
		}
		if err := os.Remove(filepath.Join(configDir, "app.json")); err != nil {
			t.Fatal(err)
		}
		storageDir := keychain.StorageDir(keychain.Service)
		if err := os.MkdirAll(storageDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(storageDir, keychain.AccountToken+".enc"), []byte("encrypted"), 0o600); err != nil {
			t.Fatal(err)
		}
		authKeychainGet = func(string, string) (string, error) {
			return `{"corp_id":"corp","access_token":"portable"}`, nil
		}
		if !PortableAuthTargetPopulated(configDir) || !PortableAuthSourceReady() {
			t.Fatal("encrypted token source was not detected")
		}
	})

	t.Run("export and import", func(t *testing.T) {
		oldGOOS := portableRuntimeGOOS
		oldGet := authKeychainGet
		t.Cleanup(func() {
			portableRuntimeGOOS = oldGOOS
			authKeychainGet = oldGet
		})
		authKeychainGet = func(string, string) (string, error) { return "", nil }
		portableRuntimeGOOS = func() string { return "linux" }
		keyRoot := t.TempDir()
		t.Setenv(keychain.StorageDirEnv, keyRoot)
		t.Setenv(keychain.DisableKeychainEnv, "1")
		configDir := t.TempDir()
		if err := ExportPortableAuthBundle(configDir, nil); err == nil {
			t.Fatal("nil portable writer succeeded")
		}
		portableRuntimeGOOS = func() string { return "darwin" }
		t.Setenv(keychain.DisableKeychainEnv, "")
		if err := ExportPortableAuthBundle(configDir, io.Discard); err == nil {
			t.Fatal("unsupported portable export succeeded")
		}
		portableRuntimeGOOS = func() string { return "linux" }
		t.Setenv(keychain.DisableKeychainEnv, "1")
		t.Setenv(keychain.StorageDirEnv, filepath.Join(t.TempDir(), "missing"))
		if err := ExportPortableAuthBundle(configDir, io.Discard); err == nil {
			t.Fatal("missing keychain export succeeded")
		}
		emptyKeyRoot := t.TempDir()
		t.Setenv(keychain.StorageDirEnv, emptyKeyRoot)
		if err := os.MkdirAll(keychain.StorageDir(keychain.Service), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := ExportPortableAuthBundle(configDir, io.Discard); err == nil {
			t.Fatal("empty keychain export succeeded")
		}

		readyKeyRoot := t.TempDir()
		t.Setenv(keychain.StorageDirEnv, readyKeyRoot)
		keyDir := keychain.StorageDir(keychain.Service)
		if err := os.MkdirAll(filepath.Join(keyDir, "nested"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(keyDir, keychain.AccountToken+".enc"), []byte("encrypted"), 0o600); err != nil {
			t.Fatal(err)
		}
		authKeychainGet = func(string, string) (string, error) {
			return `{"corp_id":"corp","access_token":"portable"}`, nil
		}
		if err := os.WriteFile(filepath.Join(keyDir, "nested", "value"), []byte("nested"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(keyDir, "dek"), filepath.Join(keyDir, "link")); err != nil {
			t.Fatal(err)
		}
		for name, value := range map[string]string{"app.json": "{}", profilesJSONFile: "{}", "mcp_url": "https://mcp.test", "terminal_url": "https://terminal.test"} {
			if err := os.WriteFile(filepath.Join(configDir, name), []byte(value), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Mkdir(filepath.Join(configDir, "app-dir.json"), 0o700); err != nil {
			t.Fatal(err)
		}
		var bundle bytes.Buffer
		if err := ExportPortableAuthBundle(configDir, &bundle); err != nil {
			t.Fatal(err)
		}

		importKeyRoot := t.TempDir()
		t.Setenv(keychain.StorageDirEnv, importKeyRoot)
		importDir := t.TempDir()
		report, err := ImportPortableAuthBundle(importDir, bytes.NewReader(bundle.Bytes()))
		if err != nil || report.BundleOS != "linux" || report.OSMismatch {
			t.Fatalf("portable import report = %#v %v", report, err)
		}
		if _, err := os.Stat(filepath.Join(importDir, "app.json")); err != nil {
			t.Fatal("config file was not imported")
		}
		if _, err := os.Stat(filepath.Join(keychain.StorageDir(keychain.Service), keychain.AccountToken+".enc")); err != nil {
			t.Fatal("encrypted token was not imported")
		}
	})

	t.Run("import validation", func(t *testing.T) {
		oldGOOS := portableRuntimeGOOS
		t.Cleanup(func() {
			portableRuntimeGOOS = oldGOOS
		})
		portableRuntimeGOOS = func() string { return "linux" }

		t.Setenv(keychain.StorageDirEnv, t.TempDir())
		if _, err := ImportPortableAuthBundle(t.TempDir(), nil); err == nil {
			t.Fatal("nil portable reader succeeded")
		}
		if _, err := ImportPortableAuthBundle(t.TempDir(), strings.NewReader("not gzip")); err == nil {
			t.Fatal("invalid gzip bundle succeeded")
		}
		var corrupt bytes.Buffer
		gz := gzip.NewWriter(&corrupt)
		_, _ = gz.Write([]byte("not a tar stream"))
		_ = gz.Close()
		if _, err := ImportPortableAuthBundle(t.TempDir(), bytes.NewReader(corrupt.Bytes())); err == nil {
			t.Fatal("invalid tar bundle succeeded")
		}

		cases := []struct {
			name    string
			header  tar.Header
			body    string
			wantErr bool
		}{
			{"bad manifest", tar.Header{Name: portableAuthManifest}, "{", true},
			{"unsafe parent", tar.Header{Name: "../evil"}, "x", true},
			{"unsafe absolute", tar.Header{Name: "/evil"}, "x", true},
			{"unsupported path", tar.Header{Name: "other/value"}, "x", true},
			{"unsupported type", tar.Header{Name: "config/link", Typeflag: tar.TypeSymlink, Linkname: "x"}, "", true},
			{"directory", tar.Header{Name: "config/nested", Typeflag: tar.TypeDir}, "", false},
			{"no manifest", tar.Header{Name: "config/value"}, "x", false},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := ImportPortableAuthBundle(t.TempDir(), bytes.NewReader(makeBundle(t, []tar.Header{tc.header}, []string{tc.body})))
				if (err != nil) != tc.wantErr {
					t.Fatalf("import error = %v, want error %v", err, tc.wantErr)
				}
			})
		}
		manifest := `{"version":1,"os":"other-os","keychain_service":"dws-cli"}`
		report, err := ImportPortableAuthBundle(t.TempDir(), bytes.NewReader(makeBundle(t, []tar.Header{{Name: portableAuthManifest}}, []string{manifest})))
		if err != nil || report.BundleOS != "other-os" || !report.OSMismatch {
			t.Fatalf("mismatched OS report = %#v %v", report, err)
		}
	})

	t.Run("helper failures", func(t *testing.T) {
		if _, err := cleanPortableName("."); err == nil {
			t.Fatal("empty portable name succeeded")
		}
		if _, err := cleanPortableName("../x"); err == nil {
			t.Fatal("parent portable name succeeded")
		}
		if got, err := cleanPortableName(" config/value "); err != nil || got != "config/value" {
			t.Fatalf("clean portable name = %q %v", got, err)
		}
		for _, rel := range []string{"", ".", "..", "../x", "/absolute"} {
			if _, err := safeJoin(t.TempDir(), rel); err == nil {
				t.Errorf("unsafe relative path %q succeeded", rel)
			}
		}
		safeRoot := t.TempDir()
		if got, err := safeJoin(safeRoot, "nested/value"); err != nil || got != filepath.Join(safeRoot, "nested", "value") {
			t.Fatalf("safe join = %q %v", got, err)
		}
		oldInRoot := portablePathInRoot
		portablePathInRoot = func(string, string) bool { return false }
		if _, err := safeJoin(safeRoot, "value"); err == nil {
			t.Fatal("out-of-root path succeeded")
		}
		portablePathInRoot = oldInRoot

		if _, err := portableConfigFiles("["); err == nil {
			t.Fatal("invalid config glob succeeded")
		}
		dir := t.TempDir()
		file := filepath.Join(dir, "app.json")
		if err := os.WriteFile(file, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		oldStat := portableStat
		portableStat = func(string) (os.FileInfo, error) { return nil, errors.New("stat") }
		if files, err := portableConfigFiles(dir); err != nil || len(files) != 0 {
			t.Fatalf("stat-skipped config files = %v %v", files, err)
		}
		portableStat = oldStat
		oldRel := portableRel
		portableRel = func(string, string) (string, error) { return "", errors.New("rel") }
		if _, err := portableConfigFiles(dir); err == nil {
			t.Fatal("config relative-path failure succeeded")
		}
		if got := mustPortableRel("root", "file"); got != "file" {
			t.Fatalf("portable relative fallback = %q", got)
		}
		portableRel = oldRel

		oldMarshal := portableJSONMarshal
		portableJSONMarshal = func(any, string, string) ([]byte, error) { return nil, errors.New("marshal") }
		if err := writePortableManifest(tar.NewWriter(io.Discard), portableAuthBundleManifest{}); err == nil {
			t.Fatal("manifest marshal failure succeeded")
		}
		portableJSONMarshal = oldMarshal

		closed := tar.NewWriter(io.Discard)
		_ = closed.Close()
		if err := writePortableBytes(closed, "value", []byte("x"), 0o600); err == nil {
			t.Fatal("closed tar byte write succeeded")
		}
		dataFail := &failAfterWriter{remaining: 512}
		if err := writePortableBytes(tar.NewWriter(dataFail), "value", []byte("x"), 0o600); err == nil {
			t.Fatal("tar data write failure succeeded")
		}

		if err := addPortableDir(tar.NewWriter(io.Discard), filepath.Join(t.TempDir(), "missing"), "keychain"); err == nil {
			t.Fatal("missing portable directory succeeded")
		}
		walkDir := t.TempDir()
		if err := os.Mkdir(filepath.Join(walkDir, "nested"), 0o700); err != nil {
			t.Fatal(err)
		}
		portableRel = func(string, string) (string, error) { return "", errors.New("rel") }
		if err := addPortableDir(tar.NewWriter(io.Discard), walkDir, "keychain"); err == nil {
			t.Fatal("directory relative-path failure succeeded")
		}
		portableRel = oldRel

		if err := addPortableFile(tar.NewWriter(io.Discard), filepath.Join(t.TempDir(), "missing"), "value"); err == nil {
			t.Fatal("missing portable file succeeded")
		}
		if err := addPortableFile(tar.NewWriter(io.Discard), t.TempDir(), "dir"); err != nil {
			t.Fatal(err)
		}
		oldOpen := portableOpen
		portableOpen = func(string) (io.ReadCloser, error) { return nil, errors.New("open") }
		if err := addPortableFile(tar.NewWriter(io.Discard), file, "value"); err == nil {
			t.Fatal("portable file open failure succeeded")
		}
		portableOpen = oldOpen
		if err := addPortableFile(closed, file, "value"); err == nil {
			t.Fatal("closed tar file write succeeded")
		}
		portableOpen = func(string) (io.ReadCloser, error) { return io.NopCloser(errorReader{}), nil }
		if err := addPortableFile(tar.NewWriter(io.Discard), file, "value"); err == nil {
			t.Fatal("portable file copy failure succeeded")
		}
		portableOpen = oldOpen

		oldMkdir := portableMkdirAll
		oldChmod := portableChmod
		oldCreate := portableCreateTemp
		oldRename := portableRename
		oldRemove := portableRemove
		t.Cleanup(func() {
			portableStat = oldStat
			portableRel = oldRel
			portableOpen = oldOpen
			portableJSONMarshal = oldMarshal
			portablePathInRoot = oldInRoot
			portableMkdirAll = oldMkdir
			portableChmod = oldChmod
			portableCreateTemp = oldCreate
			portableRename = oldRename
			portableRemove = oldRemove
		})
		portableMkdirAll = func(string, os.FileMode) error { return errors.New("mkdir") }
		if err := extractPortableEntry(filepath.Join(t.TempDir(), "dir"), &tar.Header{Typeflag: tar.TypeDir}, strings.NewReader("")); err == nil {
			t.Fatal("portable directory mkdir failure succeeded")
		}
		if err := extractPortableEntry(filepath.Join(t.TempDir(), "file"), &tar.Header{Typeflag: tar.TypeReg}, strings.NewReader("x")); err == nil {
			t.Fatal("portable file mkdir failure succeeded")
		}
		portableMkdirAll = oldMkdir
		portableChmod = func(string, os.FileMode) error { return errors.New("chmod") }
		if err := extractPortableEntry(t.TempDir(), &tar.Header{Typeflag: tar.TypeDir}, strings.NewReader("")); err == nil {
			t.Fatal("portable directory chmod failure succeeded")
		}
		portableChmod = oldChmod
		portableCreateTemp = func(string, string) (portableTempFile, error) { return nil, errors.New("create") }
		if err := extractPortableEntry(filepath.Join(t.TempDir(), "file"), &tar.Header{Typeflag: tar.TypeReg}, strings.NewReader("x")); err == nil {
			t.Fatal("portable temp create failure succeeded")
		}
		fake := func() *fakePortableTemp { return &fakePortableTemp{name: filepath.Join(t.TempDir(), "tmp")} }
		for name, configure := range map[string]func(*fakePortableTemp){
			"chmod": func(f *fakePortableTemp) { f.chmodErr = errors.New("chmod") },
			"write": func(f *fakePortableTemp) { f.writeErr = errors.New("write") },
			"sync":  func(f *fakePortableTemp) { f.syncErr = errors.New("sync") },
			"close": func(f *fakePortableTemp) { f.closeErr = errors.New("close") },
		} {
			t.Run(name, func(t *testing.T) {
				f := fake()
				configure(f)
				portableCreateTemp = func(string, string) (portableTempFile, error) { return f, nil }
				if err := extractPortableEntry(filepath.Join(t.TempDir(), "file"), &tar.Header{Typeflag: tar.TypeReg}, strings.NewReader("x")); err == nil {
					t.Fatalf("portable temp %s failure succeeded", name)
				}
			})
		}
		f := fake()
		portableCreateTemp = func(string, string) (portableTempFile, error) { return f, nil }
		portableRename = func(string, string) error { return errors.New("rename") }
		portableRemove = func(string) error { return errors.New("remove") }
		if err := extractPortableEntry(filepath.Join(t.TempDir(), "file"), &tar.Header{Typeflag: tar.TypeReg}, strings.NewReader("x")); err == nil {
			t.Fatal("portable rename failure succeeded")
		}
	})
}

func TestCrossPlatformCoverageTokenStorageAndRevocationCoverageEdges(t *testing.T) {
	oldMarshalIndent := tokenJSONMarshalIndent
	oldMarshal := tokenJSONMarshal
	oldMkdir := tokenMkdirAll
	oldRead := tokenReadFile
	oldWrite := tokenWriteFile
	oldRename := tokenRename
	oldRemove := tokenRemove
	oldGlob := tokenGlob
	oldSaveCorp := tokenSaveKeychainForCorpID
	oldSaveIdentity := tokenSaveKeychainForIdentity
	oldSaveLegacy := tokenSaveKeychain
	oldLoadCorp := tokenLoadKeychainForCorpID
	oldLoadIdentity := tokenLoadKeychainIdentity
	oldLoadLegacy := tokenLoadKeychain
	oldExists := tokenKeychainExists
	oldDeleteCorp := tokenDeleteKeychainForCorpID
	oldDeleteIdentity := tokenDeleteKeychainIdentity
	oldDeleteLegacy := tokenDeleteKeychain
	oldRemoveAuthEntries := tokenRemoveAuthTokenEntries
	oldLoadSecure := tokenLoadSecure
	oldDeleteSecure := tokenDeleteSecure
	oldEnsureProfiles := profilesEnsureMigration
	oldResolve := tokenResolveProfile
	oldResolveDeletion := tokenResolveDeletion
	oldResolveSelection := tokenResolveSelection
	oldUpsert := tokenUpsertProfile
	oldRemoveProfile := tokenRemoveProfile
	oldSync := tokenSyncLegacyMirror
	oldSyncOrganization := tokenSyncOrganizationMirror
	oldLoadProfiles := tokenLoadProfiles
	oldSaveProfiles := tokenSaveProfiles
	oldWriteMarker := tokenWriteMarker
	oldWriteManualMarker := tokenWriteManualMarker
	oldDeleteMarker := tokenDeleteMarker
	oldParseURL := tokenParseURL
	oldNewRequest := tokenNewRequest
	oldDefaultDir := tokenDefaultConfigDir
	oldLoadData := tokenLoadData
	oldRevokeURL := tokenRevokeURL
	oldMCPBaseURL := tokenMCPBaseURL
	oldLogoutURL := tokenLogoutURL
	oldLogoutContinue := tokenLogoutContinueURL
	oldLogoutClient := tokenLogoutHTTPClient
	oldRevokeClient := tokenRevokeHTTPClient
	oldEdition := edition.Get()
	t.Cleanup(func() {
		tokenJSONMarshalIndent = oldMarshalIndent
		tokenJSONMarshal = oldMarshal
		tokenMkdirAll = oldMkdir
		tokenReadFile = oldRead
		tokenWriteFile = oldWrite
		tokenRename = oldRename
		tokenRemove = oldRemove
		tokenGlob = oldGlob
		tokenSaveKeychainForCorpID = oldSaveCorp
		tokenSaveKeychainForIdentity = oldSaveIdentity
		tokenSaveKeychain = oldSaveLegacy
		tokenLoadKeychainForCorpID = oldLoadCorp
		tokenLoadKeychainIdentity = oldLoadIdentity
		tokenLoadKeychain = oldLoadLegacy
		tokenKeychainExists = oldExists
		tokenDeleteKeychainForCorpID = oldDeleteCorp
		tokenDeleteKeychainIdentity = oldDeleteIdentity
		tokenDeleteKeychain = oldDeleteLegacy
		tokenRemoveAuthTokenEntries = oldRemoveAuthEntries
		tokenLoadSecure = oldLoadSecure
		tokenDeleteSecure = oldDeleteSecure
		profilesEnsureMigration = oldEnsureProfiles
		tokenResolveProfile = oldResolve
		tokenResolveDeletion = oldResolveDeletion
		tokenResolveSelection = oldResolveSelection
		tokenUpsertProfile = oldUpsert
		tokenRemoveProfile = oldRemoveProfile
		tokenSyncLegacyMirror = oldSync
		tokenSyncOrganizationMirror = oldSyncOrganization
		tokenLoadProfiles = oldLoadProfiles
		tokenSaveProfiles = oldSaveProfiles
		tokenWriteMarker = oldWriteMarker
		tokenWriteManualMarker = oldWriteManualMarker
		tokenDeleteMarker = oldDeleteMarker
		tokenParseURL = oldParseURL
		tokenNewRequest = oldNewRequest
		tokenDefaultConfigDir = oldDefaultDir
		tokenLoadData = oldLoadData
		tokenRevokeURL = oldRevokeURL
		tokenMCPBaseURL = oldMCPBaseURL
		tokenLogoutURL = oldLogoutURL
		tokenLogoutContinueURL = oldLogoutContinue
		tokenLogoutHTTPClient = oldLogoutClient
		tokenRevokeHTTPClient = oldRevokeClient
		edition.Override(oldEdition)
		SetRuntimeProfile("")
		SetClientID("")
		resetClientIDFromMCP()
	})

	t.Run("marker", func(t *testing.T) {
		dir := t.TempDir()
		tokenMkdirAll = func(string, os.FileMode) error { return errors.New("mkdir") }
		if err := WriteTokenMarker(dir); err == nil {
			t.Fatal("marker mkdir failure succeeded")
		}
		tokenMkdirAll = oldMkdir
		tokenWriteFile = func(string, []byte, os.FileMode) error { return errors.New("write") }
		if err := WriteTokenMarker(dir); err == nil {
			t.Fatal("marker write failure succeeded")
		}
		tokenWriteFile = oldWrite
		tokenRename = func(string, string) error { return errors.New("rename") }
		if err := WriteTokenMarker(dir); err == nil {
			t.Fatal("marker rename failure succeeded")
		}
		tokenRename = oldRename
		tokenRemove = func(string) error { return errors.New("remove") }
		if err := DeleteTokenMarker(dir); err == nil {
			t.Fatal("marker delete failure succeeded")
		}
		tokenRemove = oldRemove
	})

	t.Run("save branches", func(t *testing.T) {
		dir := t.TempDir()
		data := &TokenData{CorpID: "corp", AccessToken: "access"}
		fail := errors.New("fail")
		tokenSaveKeychainForCorpID = func(string, *TokenData) error { return fail }
		if err := saveTokenDataLocked(dir, data); !errors.Is(err, fail) {
			t.Fatalf("corp keychain error = %v", err)
		}
		tokenSaveKeychainForCorpID = oldSaveCorp
		tokenUpsertProfile = func(string, *TokenData, bool) error { return fail }
		if err := saveTokenDataLocked(dir, data); !errors.Is(err, fail) {
			t.Fatalf("profile upsert error = %v", err)
		}
		tokenUpsertProfile = oldUpsert
		tokenSaveKeychain = func(*TokenData) error { return fail }
		if err := saveTokenDataLocked(dir, data); !errors.Is(err, fail) {
			t.Fatalf("legacy keychain error = %v", err)
		}
		tokenSaveKeychain = oldSaveLegacy
		SetRuntimeProfile("other")
		tokenSyncLegacyMirror = func(string) error { return fail }
		if err := saveTokenDataLocked(dir, data); !errors.Is(err, fail) {
			t.Fatalf("legacy mirror error = %v", err)
		}
		tokenSyncLegacyMirror = oldSync
		tokenWriteMarker = func(string) error { return fail }
		if err := saveTokenDataLocked(dir, data); !errors.Is(err, fail) {
			t.Fatalf("corp marker error = %v", err)
		}
		SetRuntimeProfile("")
		tokenWriteManualMarker = func(string) error { return fail }
		if err := saveTokenDataLocked(dir, &TokenData{}); !errors.Is(err, fail) {
			t.Fatalf("legacy marker error = %v", err)
		}
		tokenWriteMarker = oldWriteMarker
		tokenWriteManualMarker = oldWriteManualMarker

		edition.Override(&edition.Hooks{Name: "coverage", SaveToken: func(string, []byte) error { return nil }})
		if err := saveTokenDataLocked(dir, data); err != nil {
			t.Fatal(err)
		}
		tokenJSONMarshalIndent = func(any, string, string) ([]byte, error) { return nil, fail }
		if err := saveTokenViaHook(edition.Get(), dir, data); err == nil {
			t.Fatal("hook marshal failure succeeded")
		}
		tokenJSONMarshalIndent = oldMarshalIndent
		edition.Override(oldEdition)
	})

	t.Run("load branches", func(t *testing.T) {
		dir := t.TempDir()
		fail := errors.New("fail")
		selected := &Profile{CorpID: "corp"}
		tokenResolveProfile = func(string, string) (*Profile, error) { return nil, fail }
		if _, err := LoadTokenDataForProfile(dir, ""); !errors.Is(err, fail) {
			t.Fatalf("profile resolution error = %v", err)
		}
		tokenResolveProfile = func(string, string) (*Profile, error) { return selected, nil }
		tokenLoadKeychainForCorpID = func(string) (*TokenData, error) { return nil, fail }
		if _, err := LoadTokenDataForProfile(dir, "explicit"); !errors.Is(err, fail) {
			t.Fatalf("explicit profile load error = %v", err)
		}
		tokenLoadKeychainForCorpID = func(string) (*TokenData, error) { return nil, ErrTokenDataNotFound }
		tokenLoadKeychain = func() (*TokenData, error) { return &TokenData{CorpID: "corp", AccessToken: "legacy"}, nil }
		if got, err := LoadTokenDataForProfile(dir, ""); err != nil || got.AccessToken != "legacy" {
			t.Fatalf("matching legacy fallback = %#v %v", got, err)
		}
		tokenLoadKeychain = func() (*TokenData, error) { return &TokenData{CorpID: "other"}, nil }
		if _, err := LoadTokenDataForProfile(dir, ""); !errors.Is(err, ErrTokenDataNotFound) {
			t.Fatalf("mismatched legacy fallback error = %v", err)
		}

		tokenResolveProfile = func(string, string) (*Profile, error) { return nil, nil }
		tokenKeychainExists = func() bool { return true }
		tokenLoadKeychain = func() (*TokenData, error) { return nil, fail }
		if _, err := LoadTokenDataForProfile(dir, ""); !errors.Is(err, fail) {
			t.Fatalf("legacy slot load error = %v", err)
		}
		tokenKeychainExists = func() bool { return false }
		tokenLoadSecure = func(string) (*TokenData, error) { return nil, fail }
		if _, err := LoadTokenDataForProfile(dir, ""); !errors.Is(err, fail) {
			t.Fatalf("secure load error = %v", err)
		}
		legacy := &TokenData{AccessToken: "secure"}
		tokenLoadSecure = func(string) (*TokenData, error) { return legacy, nil }
		tokenSaveKeychain = func(*TokenData) error { return fail }
		if got, err := LoadTokenDataForProfile(dir, ""); err != nil || got != legacy {
			t.Fatalf("failed migration load = %#v %v", got, err)
		}
		tokenSaveKeychain = func(*TokenData) error { return nil }
		tokenLoadKeychain = func() (*TokenData, error) { return nil, ErrTokenDataNotFound }
		tokenReadFile = func(string) ([]byte, error) { return nil, os.ErrNotExist }
		tokenWriteManualMarker = func(string) error { return nil }
		deleted := false
		tokenDeleteSecure = func(string) error { deleted = true; return nil }
		if _, err := LoadTokenDataForProfile(dir, ""); err != nil || !deleted {
			t.Fatal("successful secure migration did not clean up")
		}

		edition.Override(&edition.Hooks{Name: "coverage", LoadToken: func(string) ([]byte, error) { return nil, fail }})
		if _, err := LoadTokenDataForProfile(dir, "selected"); err == nil {
			t.Fatal("hook profile selection succeeded")
		}
		if _, err := LoadTokenDataForProfile(dir, ""); !errors.Is(err, fail) {
			t.Fatalf("hook load error = %v", err)
		}
		edition.Override(&edition.Hooks{Name: "coverage", LoadToken: func(string) ([]byte, error) { return []byte("{"), nil }})
		if _, err := LoadTokenDataForProfile(dir, ""); err == nil {
			t.Fatal("malformed hook token succeeded")
		}
		edition.Override(oldEdition)
	})

	t.Run("delete branches", func(t *testing.T) {
		dir := t.TempDir()
		fail := errors.New("fail")
		selected := &Profile{CorpID: "corp", UserID: "user"}
		profilesEnsureMigration = func(string) error { return fail }
		if err := deleteTokenDataForProfileLocked(dir, ""); !errors.Is(err, fail) {
			t.Fatalf("delete migration error = %v", err)
		}
		profilesEnsureMigration = func(string) error { return nil }
		tokenLoadProfiles = func(string) (*ProfilesConfig, error) { return nil, fail }
		if err := deleteTokenDataForProfileLocked(dir, ""); !errors.Is(err, fail) {
			t.Fatalf("delete profiles load error = %v", err)
		}
		cfg := &ProfilesConfig{
			CurrentProfile:     ProfileSelector(*selected),
			OrgCurrentProfiles: map[string]string{"corp": ProfileSelector(*selected)},
			Profiles:           []Profile{*selected},
		}
		tokenLoadProfiles = func(string) (*ProfilesConfig, error) { return cfg, nil }
		tokenResolveDeletion = func(*ProfilesConfig, string) (*Profile, bool, error) { return nil, false, fail }
		if err := deleteTokenDataForProfileLocked(dir, ""); !errors.Is(err, fail) {
			t.Fatalf("delete resolution error = %v", err)
		}
		tokenResolveDeletion = func(*ProfilesConfig, string) (*Profile, bool, error) { return selected, true, nil }
		tokenLoadKeychainIdentity = func(string, string) (*TokenData, error) {
			return &TokenData{CorpID: "corp", UserID: "user"}, nil
		}
		tokenLoadKeychainForCorpID = func(string) (*TokenData, error) {
			return &TokenData{CorpID: "corp", UserID: "user"}, nil
		}
		tokenLoadKeychain = func() (*TokenData, error) {
			return &TokenData{CorpID: "corp", UserID: "user"}, nil
		}
		tokenReadFile = func(string) ([]byte, error) { return nil, os.ErrNotExist }
		tokenSaveProfiles = func(string, *ProfilesConfig) error { return nil }
		tokenSaveKeychainForIdentity = func(string, string, *TokenData) error { return nil }
		tokenSaveKeychainForCorpID = func(string, *TokenData) error { return nil }
		tokenSaveKeychain = func(*TokenData) error { return nil }
		tokenWriteMarker = func(string) error { return nil }
		tokenDeleteMarker = func(string) error { return nil }
		tokenRemoveProfile = func(string, string) (*Profile, error) { return selected, nil }
		tokenSyncLegacyMirror = func(string) error { return nil }
		tokenSyncOrganizationMirror = func(Profile) error { return nil }
		tokenDeleteSecure = func(string) error { return nil }
		tokenDeleteKeychainForCorpID = func(string) error { return nil }
		tokenDeleteKeychainIdentity = func(string, string) error { return fail }
		if err := deleteTokenDataForProfileLocked(dir, ""); !errors.Is(err, fail) {
			t.Fatalf("delete identity keychain error = %v", err)
		}
		tokenDeleteKeychainIdentity = func(string, string) error { return nil }
		cfg.OrgCurrentProfiles = map[string]string{"corp": ProfileSelector(*selected)}
		tokenRemoveProfile = func(string, string) (*Profile, error) {
			cfg.OrgCurrentProfiles = nil
			return selected, nil
		}
		tokenDeleteKeychainForCorpID = func(string) error { return fail }
		if err := deleteTokenDataForProfileLocked(dir, ""); !errors.Is(err, fail) {
			t.Fatalf("delete corp keychain error = %v", err)
		}
		tokenDeleteKeychainForCorpID = func(string) error { return nil }
		cfg.OrgCurrentProfiles = map[string]string{"corp": ProfileSelector(*selected)}
		tokenRemoveProfile = func(string, string) (*Profile, error) { return nil, fail }
		if err := deleteTokenDataForProfileLocked(dir, ""); !errors.Is(err, fail) {
			t.Fatalf("remove profile error = %v", err)
		}
		tokenRemoveProfile = func(string, string) (*Profile, error) { return selected, nil }
		cfg.OrgCurrentProfiles = nil
		tokenSyncLegacyMirror = func(string) error { return fail }
		if err := deleteTokenDataForProfileLocked(dir, ""); !errors.Is(err, fail) {
			t.Fatalf("sync mirror error = %v", err)
		}
		tokenSyncLegacyMirror = func(string) error { return nil }
		tokenDeleteSecure = func(string) error { return fail }
		if err := deleteTokenDataForProfileLocked(dir, ""); !errors.Is(err, fail) {
			t.Fatalf("secure cleanup error = %v", err)
		}

		cfg.CurrentProfile = ""
		cfg.Profiles = nil
		tokenDeleteKeychain = func() error { return fail }
		tokenDeleteMarker = func(string) error { return nil }
		if err := deleteTokenDataForProfileLocked(dir, ""); !errors.Is(err, fail) {
			t.Fatalf("legacy keychain delete error = %v", err)
		}
		tokenDeleteKeychain = func() error { return nil }
		tokenDeleteSecure = func(string) error { return fail }
		if err := deleteTokenDataForProfileLocked(dir, ""); !errors.Is(err, fail) {
			t.Fatalf("legacy secure delete error = %v", err)
		}

		edition.Override(&edition.Hooks{Name: "coverage", DeleteToken: func(string) error { return fail }})
		if err := DeleteTokenDataForProfile(dir, "selected"); err == nil {
			t.Fatal("hook profile deletion succeeded")
		}
		if err := DeleteTokenDataForProfile(dir, ""); !errors.Is(err, fail) {
			t.Fatalf("hook token deletion error = %v", err)
		}
		if err := DeleteAllTokenData(dir); !errors.Is(err, fail) {
			t.Fatalf("hook delete-all error = %v", err)
		}
		edition.Override(oldEdition)
	})

	t.Run("delete all", func(t *testing.T) {
		fail := errors.New("fail")
		base := func() string {
			tokenRemoveAuthTokenEntries = func(string) error { return nil }
			tokenRemove = func(string) error { return os.ErrNotExist }
			tokenGlob = func(string) ([]string, error) { return nil, nil }
			tokenDeleteSecure = func(string) error { return nil }
			tokenDeleteMarker = func(string) error { return nil }
			return t.TempDir()
		}
		dir := base()
		tokenRemoveAuthTokenEntries = func(string) error { return fail }
		if err := DeleteAllTokenData(dir); !errors.Is(err, fail) {
			t.Fatalf("delete-all auth namespace error = %v", err)
		}
		dir = base()
		tokenRemove = func(string) error { return fail }
		if err := DeleteAllTokenData(dir); !errors.Is(err, fail) {
			t.Fatalf("delete-all profiles error = %v", err)
		}
		dir = base()
		corrupt := filepath.Join(dir, "profiles.json.corrupt-x")
		tokenGlob = func(string) ([]string, error) { return []string{corrupt}, nil }
		tokenRemove = func(name string) error {
			if name == corrupt {
				return fail
			}
			return os.ErrNotExist
		}
		if err := DeleteAllTokenData(dir); !errors.Is(err, fail) {
			t.Fatalf("delete-all quarantine error = %v", err)
		}
		dir = base()
		tokenDeleteSecure = func(string) error { return fail }
		if err := DeleteAllTokenData(dir); !errors.Is(err, fail) {
			t.Fatalf("delete-all secure error = %v", err)
		}
		dir = base()
		tokenDeleteMarker = func(string) error { return fail }
		if err := DeleteAllTokenData(dir); !errors.Is(err, fail) {
			t.Fatalf("delete-all marker error = %v", err)
		}
	})

	t.Run("remote revoke", func(t *testing.T) {
		resetClientIDFromMCP()
		SetClientID("client")
		fail := errors.New("fail")
		tokenLoadData = func(string) (*TokenData, error) {
			return &TokenData{AccessToken: "access", ClientID: "client", Source: "direct"}, nil
		}
		tokenParseURL = func(string) (*url.URL, error) { return nil, fail }
		if err := RevokeTokenRemote(context.Background()); !errors.Is(err, fail) {
			t.Fatalf("logout parse error = %v", err)
		}
		tokenParseURL = oldParseURL
		tokenLogoutURL = "https://logout.test"
		tokenNewRequest = func(context.Context, string, string, io.Reader) (*http.Request, error) { return nil, fail }
		if err := RevokeTokenRemote(context.Background()); !errors.Is(err, fail) {
			t.Fatalf("logout request error = %v", err)
		}
		tokenNewRequest = oldNewRequest
		tokenLogoutHTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, fail })}
		if err := RevokeTokenRemote(context.Background()); !errors.Is(err, fail) {
			t.Fatalf("logout network error = %v", err)
		}
		response := func(status int) *http.Client {
			return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
			})}
		}
		for _, status := range []int{http.StatusOK, http.StatusFound} {
			tokenLogoutHTTPClient = response(status)
			if err := RevokeTokenRemote(context.Background()); err != nil {
				t.Fatalf("logout status %d = %v", status, err)
			}
		}
		tokenLogoutHTTPClient = response(http.StatusBadGateway)
		if err := RevokeTokenRemote(context.Background()); err == nil {
			t.Fatal("bad logout status succeeded")
		}

		SetClientIDFromMCP("mcp-client")
		tokenLoadData = func(string) (*TokenData, error) { return nil, fail }
		if err := RevokeTokenRemote(context.Background()); err != nil {
			t.Fatal("missing token revoke should be a no-op")
		}
		tokenMCPBaseURL = func() string { return "https://revoke.test" }
		tokenLoadData = func(string) (*TokenData, error) {
			return &TokenData{AccessToken: "access", ClientID: "mcp-client", Source: "mcp"}, nil
		}
		tokenJSONMarshal = func(any) ([]byte, error) { return nil, fail }
		if err := RevokeTokenRemote(context.Background()); !errors.Is(err, fail) {
			t.Fatalf("revoke marshal error = %v", err)
		}
		tokenJSONMarshal = oldMarshal
		tokenNewRequest = func(context.Context, string, string, io.Reader) (*http.Request, error) { return nil, fail }
		if err := RevokeTokenRemote(context.Background()); !errors.Is(err, fail) {
			t.Fatalf("revoke request error = %v", err)
		}
		tokenNewRequest = oldNewRequest
		tokenRevokeHTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, fail })}
		if err := RevokeTokenRemote(context.Background()); !errors.Is(err, fail) {
			t.Fatalf("revoke network error = %v", err)
		}
		tokenRevokeHTTPClient = response(http.StatusBadGateway)
		if err := RevokeTokenRemote(context.Background()); err == nil {
			t.Fatal("bad revoke status succeeded")
		}
		tokenRevokeHTTPClient = response(http.StatusOK)
		if err := RevokeTokenRemote(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
}

func TestCrossPlatformCoverageSmallRemainingAuthEdges(t *testing.T) {
	t.Run("app token hooks", func(t *testing.T) {
		oldIndent := appTokenMarshalIndent
		oldMarshal := appTokenMarshal
		oldRequest := appTokenNewRequest
		oldSet := appTokenKeychainSet
		oldGet := appTokenKeychainGet
		oldClient := appTokenHTTPClient
		t.Cleanup(func() {
			appTokenMarshalIndent = oldIndent
			appTokenMarshal = oldMarshal
			appTokenNewRequest = oldRequest
			appTokenKeychainSet = oldSet
			appTokenKeychainGet = oldGet
			appTokenHTTPClient = oldClient
		})
		fail := errors.New("fail")
		appTokenMarshalIndent = func(any, string, string) ([]byte, error) { return nil, fail }
		if err := SaveAppTokenData(&AppTokenData{ClientID: "id"}); !errors.Is(err, fail) {
			t.Fatalf("app token marshal error = %v", err)
		}
		appTokenMarshalIndent = oldIndent
		appTokenKeychainSet = func(string, string, string) error { return fail }
		if err := SaveAppTokenData(&AppTokenData{ClientID: "id"}); !errors.Is(err, fail) {
			t.Fatalf("app token keychain error = %v", err)
		}
		appTokenKeychainSet = oldSet
		appTokenKeychainGet = func(string, string) (string, error) { return "", nil }
		if got, err := LoadAppTokenData("id"); err != nil || got != nil {
			t.Fatalf("empty app token = %#v %v", got, err)
		}
		appTokenKeychainGet = func(string, string) (string, error) { return "", fail }
		if got, err := LoadAppTokenData("id"); err != nil || got != nil {
			t.Fatalf("missing app token = %#v %v", got, err)
		}
		appTokenKeychainGet = oldGet
		appTokenMarshal = func(any) ([]byte, error) { return nil, fail }
		if _, _, err := FetchAppToken(context.Background(), "id", "secret"); !errors.Is(err, fail) {
			t.Fatalf("app token request marshal error = %v", err)
		}
		appTokenMarshal = oldMarshal
		appTokenNewRequest = func(context.Context, string, string, io.Reader) (*http.Request, error) { return nil, fail }
		if _, _, err := FetchAppToken(context.Background(), "id", "secret"); !errors.Is(err, fail) {
			t.Fatalf("app token request creation error = %v", err)
		}
		appTokenNewRequest = oldRequest

		appTokenKeychainGet = func(string, string) (string, error) { return "{", nil }
		appTokenHTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"accessToken":"fresh","expireIn":7200}`)), Header: make(http.Header)}, nil
		})}
		appTokenKeychainSet = func(string, string, string) error { return fail }
		if got, err := (&AppTokenProvider{AppKey: "id", AppSecret: "secret"}).GetToken(context.Background()); err != nil || got != "fresh" {
			t.Fatalf("uncached app token with save failure = %q %v", got, err)
		}
	})

	t.Run("defaults and strict keychain", func(t *testing.T) {
		oldID := defaultAuthClientID
		oldSecret := defaultAuthSecret
		oldEdition := edition.Get()
		t.Cleanup(func() {
			defaultAuthClientID = oldID
			defaultAuthSecret = oldSecret
			edition.Override(oldEdition)
			SetClientID("")
			SetClientSecret("")
			resetAppConfigCache()
		})
		dir := t.TempDir()
		t.Setenv("DWS_CONFIG_DIR", dir)
		t.Setenv("DWS_CLIENT_ID", "")
		t.Setenv("DWS_CLIENT_SECRET", "")
		SetClientID("")
		SetClientSecret("")
		resetAppConfigCache()
		defaultAuthClientID = "built-in-id"
		defaultAuthSecret = "built-in-secret"
		if ClientID() != "built-in-id" || ClientSecret() != "built-in-secret" {
			t.Fatal("built-in credential fallback failed")
		}

		if err := SaveAppConfig(dir, &AppConfig{ClientID: "saved-id", ClientSecret: PlainSecret("saved-secret")}); err != nil {
			t.Fatal(err)
		}
		resetAppConfigCache()
		if got := ClientSecret(); got != "saved-secret" {
			t.Fatalf("persisted client secret = %q", got)
		}

		account := secretAccountKey("strict-id")
		if err := keychain.Set(keychain.Service, account, "strict-secret"); err != nil {
			t.Fatal(err)
		}
		config := `{"clientId":"strict-id","clientSecret":{"source":"keychain","id":` + fmt.Sprintf("%q", account) + `}}`
		if err := os.WriteFile(GetAppConfigPath(dir), []byte(config), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, secret, _, source, err := ResolveAppCredentialsStrict(dir); err != nil || secret != "strict-secret" || source != CredentialSourceKeychain {
			t.Fatalf("strict keychain credentials = %q %q %v", secret, source, err)
		}
	})

	t.Run("secret hooks", func(t *testing.T) {
		oldGet := secretKeychainGet
		oldSet := secretKeychainSet
		t.Cleanup(func() { secretKeychainGet = oldGet; secretKeychainSet = oldSet })
		var input SecretInput
		if err := input.UnmarshalJSON([]byte("true")); err == nil {
			t.Fatal("invalid secret JSON succeeded")
		}
		fail := errors.New("fail")
		secretKeychainGet = func(string, string) (string, error) { return "", fail }
		if _, err := ResolveSecret(SecretInput{Ref: &SecretRef{Source: "keychain", ID: "id"}}); !errors.Is(err, fail) {
			t.Fatalf("secret keychain get error = %v", err)
		}
		secretKeychainSet = func(string, string, string) error { return fail }
		if _, err := StoreSecret("id", PlainSecret("secret")); !errors.Is(err, fail) {
			t.Fatalf("secret keychain set error = %v", err)
		}
	})

	t.Run("portable export failures", func(t *testing.T) {
		oldGOOS := portableRuntimeGOOS
		oldGlob := portableGlob
		oldMarshal := portableJSONMarshal
		oldWalk := portableWalkDir
		oldOpen := portableOpen
		oldInRoot := portablePathInRoot
		t.Cleanup(func() {
			portableRuntimeGOOS = oldGOOS
			portableGlob = oldGlob
			portableJSONMarshal = oldMarshal
			portableWalkDir = oldWalk
			portableOpen = oldOpen
			portablePathInRoot = oldInRoot
		})
		portableRuntimeGOOS = func() string { return "linux" }
		keyRoot := t.TempDir()
		t.Setenv(keychain.StorageDirEnv, keyRoot)
		keyDir := keychain.StorageDir(keychain.Service)
		if err := os.MkdirAll(keyDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(keyDir, keychain.AccountToken+".enc"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		configDir := t.TempDir()
		portableGlob = func(string) ([]string, error) { return nil, errors.New("glob") }
		if err := ExportPortableAuthBundle(configDir, io.Discard); err == nil {
			t.Fatal("portable config scan failure succeeded")
		}
		portableGlob = oldGlob
		portableJSONMarshal = func(any, string, string) ([]byte, error) { return nil, errors.New("marshal") }
		if err := ExportPortableAuthBundle(configDir, io.Discard); err == nil {
			t.Fatal("portable manifest failure succeeded")
		}
		portableJSONMarshal = oldMarshal
		portableWalkDir = func(string, fs.WalkDirFunc) error { return errors.New("walk") }
		if err := ExportPortableAuthBundle(configDir, io.Discard); err == nil {
			t.Fatal("portable keychain walk failure succeeded")
		}
		portableWalkDir = oldWalk
		configFile := filepath.Join(configDir, "app.json")
		if err := os.WriteFile(configFile, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		portableOpen = func(name string) (io.ReadCloser, error) {
			if name == configFile {
				return nil, errors.New("open")
			}
			return oldOpen(name)
		}
		if err := ExportPortableAuthBundle(configDir, io.Discard); err == nil {
			t.Fatal("portable config add failure succeeded")
		}
		portableOpen = oldOpen

		portablePathInRoot = func(string, string) bool { return false }
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		_ = tw.WriteHeader(&tar.Header{Name: "config/value", Typeflag: tar.TypeReg, Size: 1})
		_, _ = tw.Write([]byte("x"))
		_ = tw.Close()
		_ = gz.Close()
		if _, err := ImportPortableAuthBundle(t.TempDir(), bytes.NewReader(buf.Bytes())); err == nil {
			t.Fatal("portable safe-join failure succeeded")
		}
	})

	if tokenLogoutHTTPClient.CheckRedirect == nil {
		t.Fatal("logout redirect guard is missing")
	}
	if err := tokenLogoutHTTPClient.CheckRedirect(nil, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("logout redirect guard = %v", err)
	}
}

func TestCrossPlatformCoverageProfilesCoverageEdges(t *testing.T) {
	oldAcquire := profilesAcquireDualLock
	oldRead := profilesReadFile
	oldRename := profilesRename
	oldMkdir := profilesMkdirAll
	oldMarshal := profilesMarshalIndent
	oldWrite := profilesWriteFile
	oldRemove := profilesRemove
	oldLoad := profilesLoad
	oldSave := profilesSave
	oldEnsure := profilesEnsureMigration
	oldSync := profilesSyncLegacyMirror
	oldExists := profilesTokenExists
	oldLoadLegacy := profilesLoadLegacy
	oldSaveCorp := profilesSaveCorp
	oldExistsCorp := profilesTokenExistsCorp
	oldLoadCorp := profilesLoadCorp
	oldSaveLegacy := profilesSaveLegacy
	oldMarker := profilesWriteMarker
	oldDeleteLegacy := profilesDeleteLegacy
	oldDeleteMarker := profilesDeleteMarker
	t.Cleanup(func() {
		profilesAcquireDualLock = oldAcquire
		profilesReadFile = oldRead
		profilesRename = oldRename
		profilesMkdirAll = oldMkdir
		profilesMarshalIndent = oldMarshal
		profilesWriteFile = oldWrite
		profilesRemove = oldRemove
		profilesLoad = oldLoad
		profilesSave = oldSave
		profilesEnsureMigration = oldEnsure
		profilesSyncLegacyMirror = oldSync
		profilesTokenExists = oldExists
		profilesLoadLegacy = oldLoadLegacy
		profilesSaveCorp = oldSaveCorp
		profilesTokenExistsCorp = oldExistsCorp
		profilesLoadCorp = oldLoadCorp
		profilesSaveLegacy = oldSaveLegacy
		profilesWriteMarker = oldMarker
		profilesDeleteLegacy = oldDeleteLegacy
		profilesDeleteMarker = oldDeleteMarker
	})
	fail := errors.New("fail")

	profilesAcquireDualLock = func(context.Context, string) (*DualLock, error) { return nil, fail }
	if err := withProfilesLock(t.TempDir(), func() error { return nil }); !errors.Is(err, fail) {
		t.Fatalf("profiles lock error = %v", err)
	}
	profilesAcquireDualLock = oldAcquire

	profilesReadFile = func(string) ([]byte, error) { return nil, fail }
	if _, err := LoadProfiles(t.TempDir()); !errors.Is(err, fail) {
		t.Fatalf("profiles read error = %v", err)
	}
	profilesReadFile = oldRead

	dir := t.TempDir()
	profilesMkdirAll = func(string, os.FileMode) error { return fail }
	if err := SaveProfiles(dir, &ProfilesConfig{}); !errors.Is(err, fail) {
		t.Fatalf("profiles mkdir error = %v", err)
	}
	profilesMkdirAll = oldMkdir
	profilesMarshalIndent = func(any, string, string) ([]byte, error) { return nil, fail }
	if err := SaveProfiles(dir, &ProfilesConfig{}); !errors.Is(err, fail) {
		t.Fatalf("profiles marshal error = %v", err)
	}
	profilesMarshalIndent = oldMarshal
	profilesWriteFile = func(string, []byte, os.FileMode) error { return fail }
	if err := SaveProfiles(dir, &ProfilesConfig{}); !errors.Is(err, fail) {
		t.Fatalf("profiles write error = %v", err)
	}
	profilesWriteFile = oldWrite
	profilesRename = func(string, string) error { return fail }
	profilesRemove = func(string) error { return fail }
	if err := SaveProfiles(dir, &ProfilesConfig{}); !errors.Is(err, fail) {
		t.Fatalf("profiles rename error = %v", err)
	}
	profilesRename = oldRename
	profilesRemove = oldRemove

	profilesLoad = func(string) (*ProfilesConfig, error) { return nil, fail }
	if err := ensureProfilesMigrationLocked(dir); !errors.Is(err, fail) {
		t.Fatalf("migration profile load error = %v", err)
	}
	if err := upsertProfileFromTokenWithCurrentLocked(dir, &TokenData{}, true); !errors.Is(err, fail) {
		t.Fatalf("upsert profile load error = %v", err)
	}
	profilesLoad = func(string) (*ProfilesConfig, error) { return &ProfilesConfig{}, nil }
	profilesTokenExists = func() bool { return true }
	for name, token := range map[string]*TokenData{"nil": nil, "empty corp": {}} {
		t.Run("migration "+name, func(t *testing.T) {
			profilesLoadLegacy = func() (*TokenData, error) { return token, nil }
			if err := ensureProfilesMigrationLocked(dir); err != nil {
				t.Fatal(err)
			}
		})
	}
	profilesLoadLegacy = func() (*TokenData, error) { return nil, fail }
	if err := ensureProfilesMigrationLocked(dir); err != nil {
		t.Fatal("legacy token read errors should be ignored")
	}
	legacy := &TokenData{CorpID: "corp"}
	profilesLoadLegacy = func() (*TokenData, error) { return legacy, nil }
	profilesSaveCorp = func(string, *TokenData) error { return fail }
	if err := ensureProfilesMigrationLocked(dir); !errors.Is(err, fail) {
		t.Fatalf("migration corp save error = %v", err)
	}
	profilesSaveCorp = oldSaveCorp
	profilesSave = func(string, *ProfilesConfig) error { return fail }
	if err := ensureProfilesMigrationLocked(dir); !errors.Is(err, fail) {
		t.Fatalf("migration profile save error = %v", err)
	}

	cfg := &ProfilesConfig{
		PrimaryProfile: "primary", CurrentProfile: "current", PreviousProfile: "previous",
		Profiles: []Profile{{Name: "primary", CorpID: "primary"}, {Name: "current", CorpID: "current"}, {Name: "previous", CorpID: "previous"}},
	}
	profilesEnsureMigration = func(string) error { return fail }
	if _, err := ResolveProfile(dir, ""); !errors.Is(err, fail) {
		t.Fatalf("resolve migration error = %v", err)
	}
	if _, err := resolveProfileForLoad(dir, ""); !errors.Is(err, fail) {
		t.Fatalf("load resolution migration error = %v", err)
	}
	if _, err := setCurrentProfileLocked(dir, "current"); !errors.Is(err, fail) {
		t.Fatalf("set-current migration error = %v", err)
	}
	if _, err := usePreviousProfileLocked(dir); !errors.Is(err, fail) {
		t.Fatalf("previous migration error = %v", err)
	}
	profilesEnsureMigration = func(string) error { return nil }
	profilesLoad = func(string) (*ProfilesConfig, error) { return nil, fail }
	if _, err := ResolveProfile(dir, ""); !errors.Is(err, fail) {
		t.Fatalf("resolve load error = %v", err)
	}
	if _, err := resolveProfileForLoad(dir, ""); !errors.Is(err, fail) {
		t.Fatalf("profile-for-load read error = %v", err)
	}
	if _, err := setCurrentProfileLocked(dir, "current"); !errors.Is(err, fail) {
		t.Fatalf("set-current read error = %v", err)
	}
	if _, err := usePreviousProfileLocked(dir); !errors.Is(err, fail) {
		t.Fatalf("previous read error = %v", err)
	}
	if _, err := removeProfileLocked(dir, "current"); !errors.Is(err, fail) {
		t.Fatalf("remove read error = %v", err)
	}
	if err := markProfileStatusLocked(dir, "current", "expired"); !errors.Is(err, fail) {
		t.Fatalf("mark status read error = %v", err)
	}
	if err := syncLegacyTokenMirrorLocked(dir); !errors.Is(err, fail) {
		t.Fatalf("mirror read error = %v", err)
	}
	profilesLoad = func(string) (*ProfilesConfig, error) {
		return &ProfilesConfig{
			CurrentProfile:  "current",
			PreviousProfile: "missing",
			Profiles:        []Profile{{CorpID: "current"}},
		}, nil
	}
	if _, err := usePreviousProfileLocked(dir); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing previous profile error = %v", err)
	}

	profilesLoad = func(string) (*ProfilesConfig, error) {
		clone := *cfg
		clone.Profiles = append([]Profile(nil), cfg.Profiles...)
		return &clone, nil
	}
	profilesSave = func(string, *ProfilesConfig) error { return nil }
	if got, err := ResolveProfile(dir, ""); err != nil || got == nil || got.CorpID != "current" {
		t.Fatalf("current profile resolution = %#v %v", got, err)
	}
	primaryOnly := &ProfilesConfig{PrimaryProfile: "primary", Profiles: []Profile{{Name: "primary", CorpID: "primary"}}}
	profilesLoad = func(string) (*ProfilesConfig, error) { return primaryOnly, nil }
	if got, err := ResolveProfile(dir, ""); err != nil || got != nil {
		t.Fatalf("primary-only profile resolution = %#v %v", got, err)
	}
	profilesLoad = func(string) (*ProfilesConfig, error) { return &ProfilesConfig{}, nil }
	if got, err := ResolveProfile(dir, ""); err != nil || got != nil {
		t.Fatalf("empty profile resolution = %#v %v", got, err)
	}

	profilesLoad = func(string) (*ProfilesConfig, error) { return cfg, nil }
	profilesTokenExistsCorp = func(string) bool { return false }
	if got, err := resolveProfileForLoad(dir, ""); err != nil || got == nil || got.CorpID != "current" {
		t.Fatalf("profile-for-load current fallback = %#v %v", got, err)
	}
	cfgNoCurrent := &ProfilesConfig{PrimaryProfile: "primary", Profiles: []Profile{{CorpID: "primary"}}}
	profilesLoad = func(string) (*ProfilesConfig, error) { return cfgNoCurrent, nil }
	if got, err := resolveProfileForLoad(dir, ""); err != nil || got != nil {
		t.Fatalf("profile-for-load primary-only result = %#v %v", got, err)
	}

	profilesLoad = func(string) (*ProfilesConfig, error) { return cfg, nil }
	profilesSave = func(string, *ProfilesConfig) error { return fail }
	if _, err := setCurrentProfileLocked(dir, "current"); !errors.Is(err, fail) {
		t.Fatalf("set-current save error = %v", err)
	}
	if _, err := usePreviousProfileLocked(dir); !errors.Is(err, fail) {
		t.Fatalf("previous save error = %v", err)
	}
	if _, err := removeProfileLocked(dir, "current"); !errors.Is(err, fail) {
		t.Fatalf("remove save error = %v", err)
	}
	cfg = &ProfilesConfig{
		PrimaryProfile: "primary", CurrentProfile: "current", PreviousProfile: "previous",
		Profiles: []Profile{{Name: "primary", CorpID: "primary"}, {Name: "current", CorpID: "current"}, {Name: "previous", CorpID: "previous"}},
	}
	profilesSave = func(string, *ProfilesConfig) error { return nil }
	profilesLoadCorp = func(corpID string) (*TokenData, error) {
		return &TokenData{CorpID: corpID, AccessToken: "x"}, nil
	}
	profilesSaveCorp = func(string, *TokenData) error { return nil }
	profilesSyncLegacyMirror = func(string) error { return fail }
	if _, err := setCurrentProfileLocked(dir, "current"); !errors.Is(err, fail) {
		t.Fatalf("set-current mirror error = %v", err)
	}
	if _, err := usePreviousProfileLocked(dir); !errors.Is(err, fail) {
		t.Fatalf("previous mirror error = %v", err)
	}

	profilesLoad = func(string) (*ProfilesConfig, error) { return cfg, nil }
	profilesLoadCorp = func(string) (*TokenData, error) { return &TokenData{AccessToken: "x"}, nil }
	profilesSaveLegacy = func(*TokenData) error { return fail }
	if err := syncLegacyTokenMirrorLocked(dir); !errors.Is(err, fail) {
		t.Fatalf("mirror save error = %v", err)
	}
	profilesSaveLegacy = func(*TokenData) error { return nil }
	profilesWriteMarker = func(string) error { return fail }
	if err := syncLegacyTokenMirrorLocked(dir); !errors.Is(err, fail) {
		t.Fatalf("mirror marker error = %v", err)
	}
	profilesLoadCorp = func(string) (*TokenData, error) { return nil, fail }
	if err := syncLegacyTokenMirrorLocked(dir); err != nil {
		t.Fatal("transient mirror read error should be ignored")
	}
	profilesLoadCorp = func(string) (*TokenData, error) { return nil, nil }
	deletedLegacy, deletedMarker := false, false
	profilesDeleteLegacy = func() error { deletedLegacy = true; return fail }
	profilesDeleteMarker = func(string) error { deletedMarker = true; return fail }
	if err := syncLegacyTokenMirrorLocked(dir); !errors.Is(err, fail) || !deletedLegacy || deletedMarker {
		t.Fatalf("empty mirror cleanup = %v %v %v", err, deletedLegacy, deletedMarker)
	}
	profilesDeleteLegacy = func() error { return nil }
	if err := syncLegacyTokenMirrorLocked(dir); !errors.Is(err, fail) || !deletedMarker {
		t.Fatalf("empty mirror marker cleanup = %v %v", err, deletedMarker)
	}

	normalizeProfilesConfig(nil)
}

func TestCrossPlatformCoverageSecureIdentityKeychainAndLockEdges(t *testing.T) {
	t.Run("secure store", func(t *testing.T) {
		oldGetMAC := secureGetMAC
		oldMkdir := secureMkdirAll
		oldStat := secureStat
		oldChmod := secureChmod
		oldMarshal := secureMarshalIndent
		oldEncrypt := secureEncrypt
		oldCreateTemp := secureCreateTemp
		oldRemove := secureRemove
		oldRename := secureRename
		oldRead := secureReadFile
		oldDecrypt := secureDecrypt
		oldUnmarshal := secureUnmarshal
		t.Cleanup(func() {
			secureGetMAC = oldGetMAC
			secureMkdirAll = oldMkdir
			secureStat = oldStat
			secureChmod = oldChmod
			secureMarshalIndent = oldMarshal
			secureEncrypt = oldEncrypt
			secureCreateTemp = oldCreateTemp
			secureRemove = oldRemove
			secureRename = oldRename
			secureReadFile = oldRead
			secureDecrypt = oldDecrypt
			secureUnmarshal = oldUnmarshal
			cachedMACOnce = sync.Once{}
			cachedMAC, cachedMACErr = "", nil
		})
		resetMAC := func() {
			cachedMACOnce = sync.Once{}
			cachedMAC, cachedMACErr = "", nil
		}
		fail := errors.New("fail")
		secureGetMAC = func() (string, error) { return "", fail }
		resetMAC()
		if _, err := resolvePassword(); !errors.Is(err, fail) {
			t.Fatalf("password MAC error = %v", err)
		}
		if err := SaveSecureTokenData(t.TempDir(), &TokenData{}); !errors.Is(err, fail) {
			t.Fatalf("secure save password error = %v", err)
		}
		resetMAC()
		if _, err := LoadSecureTokenData(t.TempDir()); !errors.Is(err, fail) {
			t.Fatalf("secure load password error = %v", err)
		}
		secureGetMAC = func() (string, error) { return "00:11:22:33:44:55", nil }
		resetMAC()
		dir := t.TempDir()
		secureMkdirAll = func(string, os.FileMode) error { return fail }
		if err := SaveSecureTokenData(dir, &TokenData{}); !errors.Is(err, fail) {
			t.Fatalf("secure mkdir error = %v", err)
		}
		secureMkdirAll = oldMkdir
		if err := os.Chmod(dir, 0o777); err != nil {
			t.Fatal(err)
		}
		secureChmod = func(string, os.FileMode) error { return fail }
		if err := SaveSecureTokenData(dir, &TokenData{}); !errors.Is(err, fail) {
			t.Fatalf("secure chmod error = %v", err)
		}
		secureChmod = oldChmod
		if err := os.Chmod(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		secureMarshalIndent = func(any, string, string) ([]byte, error) { return nil, fail }
		if err := SaveSecureTokenData(dir, &TokenData{}); !errors.Is(err, fail) {
			t.Fatalf("secure marshal error = %v", err)
		}
		secureMarshalIndent = oldMarshal
		secureEncrypt = func([]byte, []byte) ([]byte, error) { return nil, fail }
		if err := SaveSecureTokenData(dir, &TokenData{}); !errors.Is(err, fail) {
			t.Fatalf("secure encrypt error = %v", err)
		}
		secureEncrypt = oldEncrypt
		secureCreateTemp = func(string, string) (secureTempFile, error) { return nil, fail }
		if err := SaveSecureTokenData(dir, &TokenData{}); !errors.Is(err, fail) {
			t.Fatalf("secure temp open error = %v", err)
		}
		for name, configure := range map[string]func(*fakeSecureTemp){
			"write": func(f *fakeSecureTemp) { f.writeErr = fail },
			"sync":  func(f *fakeSecureTemp) { f.syncErr = fail },
			"close": func(f *fakeSecureTemp) { f.closeErr = fail },
		} {
			t.Run(name, func(t *testing.T) {
				f := &fakeSecureTemp{name: filepath.Join(dir, secureDataFile+".tmp-fake")}
				configure(f)
				secureCreateTemp = func(string, string) (secureTempFile, error) { return f, nil }
				if err := SaveSecureTokenData(dir, &TokenData{}); !errors.Is(err, fail) {
					t.Fatalf("secure %s error = %v", name, err)
				}
			})
		}
		secureCreateTemp = func(string, string) (secureTempFile, error) {
			return &fakeSecureTemp{name: filepath.Join(dir, secureDataFile+".tmp-fake")}, nil
		}
		secureRename = func(string, string) error { return fail }
		secureRemove = func(string) error { return fail }
		if err := SaveSecureTokenData(dir, &TokenData{}); !errors.Is(err, fail) {
			t.Fatalf("secure rename error = %v", err)
		}
		secureRename = oldRename
		secureCreateTemp = oldCreateTemp
		secureRemove = oldRemove

		secureReadFile = func(string) ([]byte, error) { return nil, fail }
		if _, err := LoadSecureTokenData(dir); !errors.Is(err, fail) {
			t.Fatalf("secure read error = %v", err)
		}
		secureReadFile = func(string) ([]byte, error) { return []byte("cipher"), nil }
		secureDecrypt = func([]byte, []byte) ([]byte, error) { return nil, fail }
		if _, err := LoadSecureTokenData(dir); !errors.Is(err, ErrTokenDecryption) {
			t.Fatalf("secure decrypt error = %v", err)
		}
		secureDecrypt = func([]byte, []byte) ([]byte, error) { return []byte("{}"), nil }
		secureUnmarshal = func([]byte, any) error { return fail }
		if _, err := LoadSecureTokenData(dir); !errors.Is(err, fail) {
			t.Fatalf("secure unmarshal error = %v", err)
		}
		secureRemove = func(string) error { return fail }
		if err := DeleteSecureData(dir); !errors.Is(err, fail) {
			t.Fatalf("secure delete error = %v", err)
		}
	})

	t.Run("keychain store", func(t *testing.T) {
		oldMarshal := authKeychainMarshal
		oldUnmarshal := authKeychainUnmarshal
		oldSet := authKeychainSet
		oldGet := authKeychainGet
		oldMigrate := authKeychainMigrate
		t.Cleanup(func() {
			authKeychainMarshal = oldMarshal
			authKeychainUnmarshal = oldUnmarshal
			authKeychainSet = oldSet
			authKeychainGet = oldGet
			authKeychainMigrate = oldMigrate
			migrationOnce = sync.Once{}
		})
		fail := errors.New("fail")
		authKeychainMarshal = func(any, string, string) ([]byte, error) { return nil, fail }
		if err := saveTokenDataKeychainAccount("account", &TokenData{}); !errors.Is(err, fail) {
			t.Fatalf("keychain marshal error = %v", err)
		}
		authKeychainMarshal = oldMarshal
		authKeychainSet = func(string, string, string) error { return fail }
		if err := saveTokenDataKeychainAccount("account", &TokenData{}); !errors.Is(err, fail) {
			t.Fatalf("keychain set error = %v", err)
		}
		if err := SaveClientSecret("id", "secret"); !errors.Is(err, fail) {
			t.Fatalf("client-secret set error = %v", err)
		}
		authKeychainSet = oldSet
		authKeychainGet = func(string, string) (string, error) { return "", fail }
		if _, err := loadTokenDataKeychainAccount("account"); !errors.Is(err, fail) {
			t.Fatalf("keychain get error = %v", err)
		}
		if LoadClientSecret("id") != "" {
			t.Fatal("client-secret get error returned a value")
		}
		authKeychainGet = func(string, string) (string, error) { return "{}", nil }
		authKeychainUnmarshal = func([]byte, any) error { return fail }
		if _, err := loadTokenDataKeychainAccount("account"); !errors.Is(err, fail) {
			t.Fatalf("keychain unmarshal error = %v", err)
		}

		for name, result := range map[string]*keychain.MigrationResult{
			"migrated": {Migrated: true, FromPath: "from", BackupPath: "backup"},
			"relogin":  {NeedRelogin: true, Error: fail},
			"error":    {Error: fail},
		} {
			t.Run("migration "+name, func(t *testing.T) {
				migrationOnce = sync.Once{}
				authKeychainMigrate = func(string) *keychain.MigrationResult { return result }
				EnsureMigration(t.TempDir(), slog.Default())
			})
		}
	})

	t.Run("identity", func(t *testing.T) {
		oldRead := identityReadFile
		oldUnmarshal := identityUnmarshal
		oldMkdir := identityMkdirAll
		oldMarshal := identityMarshalIndent
		oldWrite := identityWriteFile
		oldRand := identityRandRead
		oldEdition := edition.Get()
		t.Cleanup(func() {
			identityReadFile = oldRead
			identityUnmarshal = oldUnmarshal
			identityMkdirAll = oldMkdir
			identityMarshalIndent = oldMarshal
			identityWriteFile = oldWrite
			identityRandRead = oldRand
			edition.Override(oldEdition)
		})
		id := &Identity{MachineID: "machine"}
		id.migrate()
		if id.AgentID != "machine" || id.Source != "dws" || id.Agents == nil {
			t.Fatalf("identity migration = %#v", id)
		}
		id.Agents = nil
		if got := id.ResolveAgentID(t.TempDir(), "agent", "test"); got == "" || id.Agents == nil {
			t.Fatal("identity agent map was not initialized")
		}
		edition.Override(&edition.Hooks{Name: "coverage", ScenarioCode: "custom-scenario"})
		if id.Headers()["x-dingtalk-scenario-code"] != "custom-scenario" {
			t.Fatal("identity scenario override failed")
		}
		fail := errors.New("fail")
		identityMkdirAll = func(string, os.FileMode) error { return fail }
		if err := save(t.TempDir(), id); !errors.Is(err, fail) {
			t.Fatalf("identity mkdir error = %v", err)
		}
		identityMkdirAll = oldMkdir
		identityMarshalIndent = func(any, string, string) ([]byte, error) { return nil, fail }
		if err := save(t.TempDir(), id); !errors.Is(err, fail) {
			t.Fatalf("identity marshal error = %v", err)
		}
		identityMarshalIndent = oldMarshal
		identityWriteFile = func(string, []byte, os.FileMode) error { return fail }
		if err := save(t.TempDir(), id); !errors.Is(err, fail) {
			t.Fatalf("identity write error = %v", err)
		}
		if formatDerivedAgentID("x") != "dwsa_00000000000x" || base62Encode([]byte{0}) != "0" {
			t.Fatal("identity short encoding failed")
		}
		identityRandRead = func([]byte) (int, error) { return 0, fail }
		if generateUUID() != "00000000-0000-4000-8000-000000000000" {
			t.Fatal("identity UUID fallback failed")
		}
	})

	t.Run("file lock", func(t *testing.T) {
		oldMkdir := fileLockMkdirAll
		oldOpen := fileLockOpenFile
		oldTry := fileLockTry
		oldNow := fileLockNow
		oldSleep := fileLockSleep
		oldProcess := dualAcquireProcessLock
		oldToken := dualAcquireTokenLock
		t.Cleanup(func() {
			fileLockMkdirAll = oldMkdir
			fileLockOpenFile = oldOpen
			fileLockTry = oldTry
			fileLockNow = oldNow
			fileLockSleep = oldSleep
			dualAcquireProcessLock = oldProcess
			dualAcquireTokenLock = oldToken
			processLocks = sync.Map{}
		})
		fail := errors.New("fail")
		key := processLockKey("bad-type")
		processLocks.Store(key, "not-a-channel")
		release, waited, err := acquireProcessLock(context.Background(), "bad-type")
		if err != nil || waited {
			t.Fatalf("bad process lock recovery = %v %v", waited, err)
		}
		release()
		held, _, err := acquireProcessLock(context.Background(), "held")
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, waited, err := acquireProcessLock(ctx, "held"); err == nil || !waited {
			t.Fatalf("canceled process wait = %v %v", waited, err)
		}
		held()

		fileLockMkdirAll = func(string, os.FileMode) error { return fail }
		if _, err := acquireTokenLock(t.TempDir()); !errors.Is(err, fail) {
			t.Fatalf("token lock mkdir error = %v", err)
		}
		fileLockMkdirAll = oldMkdir
		fileLockOpenFile = func(string, int, os.FileMode) (*os.File, error) { return nil, fail }
		if _, err := acquireTokenLock(t.TempDir()); !errors.Is(err, fail) {
			t.Fatalf("token lock open error = %v", err)
		}
		fileLockOpenFile = oldOpen
		fileLockTry = func(*os.File) error { return fail }
		base := time.Now()
		calls := 0
		fileLockNow = func() time.Time {
			calls++
			if calls < 3 {
				return base
			}
			return base.Add(time.Hour)
		}
		slept := false
		fileLockSleep = func(time.Duration) { slept = true }
		if _, err := acquireTokenLock(t.TempDir()); err == nil || !slept {
			t.Fatalf("token lock timeout = %v, slept=%v", err, slept)
		}

		dualAcquireProcessLock = func(context.Context, string) (func(), bool, error) { return nil, false, fail }
		if _, err := AcquireDualLock(context.Background(), t.TempDir()); !errors.Is(err, fail) {
			t.Fatalf("dual process lock error = %v", err)
		}
		released := false
		dualAcquireProcessLock = func(context.Context, string) (func(), bool, error) {
			return func() { released = true }, true, nil
		}
		dualAcquireTokenLock = func(string) (*tokenFileLock, error) { return nil, fail }
		if _, err := AcquireDualLock(context.Background(), t.TempDir()); !errors.Is(err, fail) || !released {
			t.Fatalf("dual file lock error = %v released=%v", err, released)
		}
	})
}

func TestCrossPlatformCoverageAppConfigHookCoverageEdges(t *testing.T) {
	oldStore := appConfigStoreSecret
	oldMarshal := appConfigMarshalIndent
	oldWrite := appConfigAtomicWrite
	oldRead := appConfigReadFile
	oldRemove := appConfigRemove
	oldLoad := appConfigLoad
	oldResolve := appConfigResolveSecret
	oldBeforeLock := appConfigBeforeResolveLock
	oldEdition := edition.Get()
	t.Cleanup(func() {
		appConfigStoreSecret = oldStore
		appConfigMarshalIndent = oldMarshal
		appConfigAtomicWrite = oldWrite
		appConfigReadFile = oldRead
		appConfigRemove = oldRemove
		appConfigLoad = oldLoad
		appConfigResolveSecret = oldResolve
		appConfigBeforeResolveLock = oldBeforeLock
		edition.Override(oldEdition)
		resetAppConfigCache()
	})
	fail := errors.New("fail")
	dir := t.TempDir()
	appConfigStoreSecret = func(string, SecretInput) (SecretInput, error) { return SecretInput{}, fail }
	if err := SaveAppConfig(dir, &AppConfig{ClientID: "id", ClientSecret: PlainSecret("secret")}); !errors.Is(err, fail) {
		t.Fatalf("app config secret-store error = %v", err)
	}
	appConfigStoreSecret = oldStore
	appConfigMarshalIndent = func(any, string, string) ([]byte, error) { return nil, fail }
	if err := SaveAppConfig(dir, &AppConfig{ClientID: "id"}); !errors.Is(err, fail) {
		t.Fatalf("app config marshal error = %v", err)
	}
	appConfigMarshalIndent = oldMarshal
	appConfigAtomicWrite = func(string, []byte) error { return fail }
	if err := SaveAppConfig(dir, &AppConfig{ClientID: "id"}); !errors.Is(err, fail) {
		t.Fatalf("app config write error = %v", err)
	}
	appConfigAtomicWrite = oldWrite

	cleanupLegacySiblingAppConfig(dir, nil)
	edition.Override(&edition.Hooks{Name: "coverage"})
	cleanupLegacySiblingAppConfig(dir, &AppConfig{ClientID: "id"})
	legacyPath := filepath.Join(dir, appConfigFile)
	if err := os.WriteFile(legacyPath, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	cleanupLegacySiblingAppConfig(dir, &AppConfig{ClientID: "id"})
	if err := os.WriteFile(legacyPath, []byte(`{"clientId":"other"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cleanupLegacySiblingAppConfig(dir, &AppConfig{ClientID: "id"})
	if err := os.WriteFile(legacyPath, []byte(`{"clientId":"id"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	appConfigRemove = func(string) error { return fail }
	cleanupLegacySiblingAppConfig(dir, &AppConfig{ClientID: "id"})
	appConfigReadFile = func(string) ([]byte, error) { return nil, fail }
	cleanupLegacySiblingAppConfig(dir, &AppConfig{ClientID: "id"})
	appConfigReadFile = oldRead

	appConfigRemove = func(string) error { return fail }
	if err := DeleteAppConfig(dir); !errors.Is(err, fail) {
		t.Fatalf("app config delete error = %v", err)
	}
	appConfigRemove = oldRemove
	appConfigLoad = func(string) (*AppConfig, error) { return nil, fail }
	if _, err := ReloadAppConfig(dir); !errors.Is(err, fail) {
		t.Fatalf("app config reload error = %v", err)
	}
	resetAppConfigCache()
	appConfigLoad = oldLoad
	if err := os.WriteFile(GetAppConfigPath(dir), []byte(`{"clientId":"id","clientSecret":"secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReloadAppConfig(dir); err != nil {
		t.Fatal(err)
	}
	resetAppConfigCache()
	appConfigResolveSecret = func(SecretInput) (string, error) { return "", fail }
	if id, secret := ResolveAppCredentials(dir); id != "id" || secret != "" {
		t.Fatalf("app config resolve failure = %q %q", id, secret)
	}
	resetAppConfigCache()
	appConfigBeforeResolveLock = func() {
		cachedResolvedMu.Lock()
		cachedResolvedID = "raced-id"
		cachedResolvedSecret = "raced-secret"
		cachedResolvedValid = true
		cachedResolvedMu.Unlock()
	}
	if id, secret := ResolveAppCredentials(dir); id != "raced-id" || secret != "raced-secret" {
		t.Fatalf("app config double-check = %q %q", id, secret)
	}
}

func TestCrossPlatformCoverageDeviceFlowHighLevelCoverageEdges(t *testing.T) {
	oldFetch := deviceFetchClientID
	oldLogin := deviceLoginOnce
	oldRequest := deviceRequestCode
	oldWait := deviceWaitAuth
	oldExchange := deviceExchangeCode
	oldCheck := deviceCheckCLIAuth
	oldAdmins := deviceGetAdmins
	oldSaveToken := deviceSaveToken
	oldHasApp := deviceHasAppConfig
	oldSaveApp := deviceSaveAppConfig
	oldPollStatus := devicePollStatus
	oldPollToken := devicePollToken
	oldAfter := deviceFlowAfter
	oldBrowser := deviceOpenBrowser
	t.Cleanup(func() {
		deviceFetchClientID = oldFetch
		deviceLoginOnce = oldLogin
		deviceRequestCode = oldRequest
		deviceWaitAuth = oldWait
		deviceExchangeCode = oldExchange
		deviceCheckCLIAuth = oldCheck
		deviceGetAdmins = oldAdmins
		deviceSaveToken = oldSaveToken
		deviceHasAppConfig = oldHasApp
		deviceSaveAppConfig = oldSaveApp
		devicePollStatus = oldPollStatus
		devicePollToken = oldPollToken
		deviceFlowAfter = oldAfter
		deviceOpenBrowser = oldBrowser
		SetClientID("")
		SetClientSecret("")
		resetClientIDFromMCP()
	})
	fail := errors.New("fail")
	p := &DeviceFlowProvider{configDir: t.TempDir(), clientID: "client", Output: io.Discard, logger: slog.Default(), NoBrowser: true}

	SetClientID("runtime-id")
	SetClientSecret("runtime-secret")
	deviceLoginOnce = func(p *DeviceFlowProvider, _ context.Context, _ int) (*TokenData, error) {
		if p.clientID != "runtime-id" {
			t.Fatalf("runtime device client ID = %q", p.clientID)
		}
		return &TokenData{AccessToken: "access"}, nil
	}
	if _, err := p.Login(context.Background()); err != nil {
		t.Fatal(err)
	}
	SetClientID("")
	SetClientSecret("")
	deviceFetchClientID = func(context.Context) (string, error) { return "", fail }
	if _, err := p.Login(context.Background()); !errors.Is(err, fail) {
		t.Fatalf("device client-ID fetch error = %v", err)
	}
	deviceFetchClientID = func(context.Context) (string, error) { return "mcp-id", nil }
	deviceLoginOnce = func(*DeviceFlowProvider, context.Context, int) (*TokenData, error) { return nil, fail }
	if _, err := p.Login(context.Background()); !errors.Is(err, fail) {
		t.Fatalf("device login error = %v", err)
	}

	deviceRequestCode = func(*DeviceFlowProvider, context.Context) (*DeviceAuthResponse, error) { return nil, fail }
	if _, err := p.loginOnce(context.Background(), 1); !errors.Is(err, fail) {
		t.Fatalf("device-code request error = %v", err)
	}
	authResp := &DeviceAuthResponse{DeviceCode: "device", UserCode: "user", VerificationURIComplete: "https://example.test", Interval: 1, ExpiresIn: 10}
	deviceRequestCode = func(*DeviceFlowProvider, context.Context) (*DeviceAuthResponse, error) { return authResp, nil }
	deviceOpenBrowser = func(string) error { return fail }
	p.NoBrowser = false
	deviceWaitAuth = func(*DeviceFlowProvider, context.Context, *DeviceAuthResponse) (*DeviceTokenResponse, error) {
		return nil, fail
	}
	if _, err := p.loginOnce(context.Background(), 1); !errors.Is(err, fail) {
		t.Fatalf("device wait error = %v", err)
	}
	p.NoBrowser = true
	deviceWaitAuth = func(*DeviceFlowProvider, context.Context, *DeviceAuthResponse) (*DeviceTokenResponse, error) {
		return &DeviceTokenResponse{AuthCode: "code"}, nil
	}
	deviceExchangeCode = func(*OAuthProvider, context.Context, string) (*TokenData, error) { return nil, fail }
	if _, err := p.loginOnce(context.Background(), 1); !errors.Is(err, fail) {
		t.Fatalf("device exchange error = %v", err)
	}
	token := &TokenData{AccessToken: "access"}
	deviceExchangeCode = func(*OAuthProvider, context.Context, string) (*TokenData, error) { return token, nil }
	deviceCheckCLIAuth = func(*OAuthProvider, context.Context, string) (*CLIAuthStatus, error) { return nil, fail }
	if _, err := p.loginOnce(context.Background(), 1); !errors.Is(err, fail) {
		t.Fatalf("device CLI-auth check error = %v", err)
	}

	denials := []struct {
		name    string
		channel string
		status  *CLIAuthStatus
	}{
		{"user forbidden", "", &CLIAuthStatus{Success: true, Result: &CLIAuthResult{UserScope: "forbidden"}}},
		{"user excluded", "", &CLIAuthStatus{Success: true, Result: &CLIAuthResult{UserScope: "specified"}}},
		{"channel excluded", "bad", &CLIAuthStatus{Success: true, Result: &CLIAuthResult{ChannelScope: "specified", AllowedChannels: []string{"good"}}}},
		{"channel required", "", &CLIAuthStatus{Success: false, ErrorCode: "CHANNEL_REQUIRED"}},
		{"enterprise default", "", &CLIAuthStatus{Success: false, ErrorCode: "ENTERPRISE_NOT_AUTHORIZED"}},
		{"enterprise message", "", &CLIAuthStatus{Success: false, ErrorCode: "ENTERPRISE_NOT_AUTHORIZED", ErrorMsg: " custom "}},
		{"no auth", "", &CLIAuthStatus{Success: false, ErrorCode: "NO_AUTH"}},
	}
	for _, tc := range denials {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DWS_CHANNEL", tc.channel)
			deviceCheckCLIAuth = func(*OAuthProvider, context.Context, string) (*CLIAuthStatus, error) { return tc.status, nil }
			if _, err := p.loginOnce(context.Background(), 1); err == nil {
				t.Fatal("denied device login succeeded")
			}
		})
	}

	t.Setenv("DWS_CHANNEL", "")
	deviceCheckCLIAuth = func(*OAuthProvider, context.Context, string) (*CLIAuthStatus, error) {
		return &CLIAuthStatus{Success: true, Result: &CLIAuthResult{}}, nil
	}
	deviceGetAdmins = func(context.Context, string) (*SuperAdminResponse, error) {
		return &SuperAdminResponse{Success: true, Result: []SuperAdmin{{Name: "Admin"}}}, nil
	}
	if _, err := p.loginOnce(context.Background(), 1); err == nil {
		t.Fatal("disabled CLI auth device login succeeded")
	}
	deviceCheckCLIAuth = func(*OAuthProvider, context.Context, string) (*CLIAuthStatus, error) {
		return &CLIAuthStatus{Success: true, Result: &CLIAuthResult{CLIAuthEnabled: true}}, nil
	}
	deviceSaveToken = func(string, *TokenData) error { return fail }
	if _, err := p.loginOnce(context.Background(), 1); !errors.Is(err, fail) {
		t.Fatalf("device token save error = %v", err)
	}
	deviceSaveToken = func(string, *TokenData) error { return nil }
	deviceHasAppConfig = func(string) bool { return false }
	appSaved := false
	deviceSaveAppConfig = func(string, *AppConfig) error { appSaved = true; return fail }
	if _, err := p.loginOnce(context.Background(), 1); err != nil || !appSaved {
		t.Fatalf("successful device login = %v appSaved=%v", err, appSaved)
	}

	deviceFlowAfter = func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}
	flowAuth := &DeviceAuthResponse{FlowID: "flow", Interval: 1, ExpiresIn: 30}
	flowCases := []struct {
		name      string
		responses []*DevicePollResponse
		errors    []error
		wantErr   bool
	}{
		{"network then approved", []*DevicePollResponse{nil, {Success: true, Data: DevicePollData{Status: StatusApproved, AuthCode: "code"}}}, []error{fail, nil}, false},
		{"pending then approved", []*DevicePollResponse{{Success: true, Data: DevicePollData{Status: StatusPending}}, {Success: true, Data: DevicePollData{Status: StatusApproved}}}, nil, false},
		{"rejected", []*DevicePollResponse{{Success: true, Data: DevicePollData{Status: StatusRejected}}}, nil, true},
		{"expired", []*DevicePollResponse{{Success: true, Data: DevicePollData{Status: StatusExpired}}}, nil, true},
		{"unknown then approved", []*DevicePollResponse{{Success: true, Data: DevicePollData{Status: "FUTURE"}}, {Success: true, Data: DevicePollData{Status: StatusApproved}}}, nil, false},
	}
	for _, tc := range flowCases {
		t.Run("flow "+tc.name, func(t *testing.T) {
			call := 0
			devicePollStatus = func(*DeviceFlowProvider, context.Context, string) (*DevicePollResponse, error) {
				i := call
				call++
				if i < len(tc.errors) && tc.errors[i] != nil {
					return nil, tc.errors[i]
				}
				return tc.responses[i], nil
			}
			_, err := p.waitForAuthorizationByFlowID(context.Background(), flowAuth)
			if (err != nil) != tc.wantErr {
				t.Fatalf("flow wait error = %v want %v", err, tc.wantErr)
			}
		})
	}
	deviceAuth := &DeviceAuthResponse{DeviceCode: "device", Interval: 30, ExpiresIn: 30}
	deviceCases := []struct {
		name      string
		responses []*DeviceTokenResponse
		errors    []error
		wantErr   bool
	}{
		{"network then success", []*DeviceTokenResponse{nil, {AuthCode: "code"}}, []error{fail, nil}, false},
		{"pending then success", []*DeviceTokenResponse{{Error: "authorization_pending"}, {AuthCode: "code"}}, nil, false},
		{"slow then success", []*DeviceTokenResponse{{Error: "slow_down"}, {AuthCode: "code"}}, nil, false},
		{"denied", []*DeviceTokenResponse{{Error: "access_denied"}}, nil, true},
		{"expired", []*DeviceTokenResponse{{Error: "expired_token"}}, nil, true},
		{"unknown then success", []*DeviceTokenResponse{{Error: "future"}, {AuthCode: "code"}}, nil, false},
	}
	for _, tc := range deviceCases {
		t.Run("device "+tc.name, func(t *testing.T) {
			call := 0
			devicePollToken = func(*DeviceFlowProvider, context.Context, string) (*DeviceTokenResponse, error) {
				i := call
				call++
				if i < len(tc.errors) && tc.errors[i] != nil {
					return nil, tc.errors[i]
				}
				return tc.responses[i], nil
			}
			_, err := p.waitForAuthorizationByDeviceCode(context.Background(), deviceAuth)
			if (err != nil) != tc.wantErr {
				t.Fatalf("device wait error = %v want %v", err, tc.wantErr)
			}
		})
	}

	statusClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("down")), Header: make(http.Header)}, nil
	})}
	p.httpClient = statusClient
	p.baseURL = "https://example.test"
	p.terminalBaseURL = "https://example.test"
	if _, err := p.requestDeviceCode(context.Background()); err == nil {
		t.Fatal("device-code HTTP error succeeded")
	}
	if _, err := p.pollDeviceToken(context.Background(), "device"); err == nil {
		t.Fatal("device-token HTTP error succeeded")
	}
	if _, err := p.pollDeviceStatus(context.Background(), "flow"); err == nil {
		t.Fatal("device-status HTTP error succeeded")
	}
	if _, err := p.postForm(context.Background(), "https://example.test", url.Values{}); err == nil {
		t.Fatal("form HTTP status error succeeded")
	}
	if _, err := p.doGet(context.Background(), "https://example.test"); err == nil {
		t.Fatal("GET HTTP status error succeeded")
	}
	dfPrintBox(io.Discard, []string{"short"})
}

func TestCrossPlatformCoverageMultiAccountSelectorAndIdentityLoadEdges(t *testing.T) {
	cfg := &ProfilesConfig{
		Profiles: []Profile{
			{Name: "local-a", CorpID: "corp-a", CorpName: "Shared", UserID: "u1", UserName: "Alice"},
			{Name: "duplicate", CorpID: "corp-a", CorpName: "Shared", UserID: "u2", UserName: "Alice"},
			{Name: "duplicate", CorpID: "corp-b", CorpName: "Shared", UserID: "u3", UserName: "Bob"},
			{Name: "solo", CorpID: "corp-c", CorpName: "Solo", UserID: "u4", UserName: "Carol"},
		},
	}

	for _, tc := range []struct {
		selector string
		wantErr  bool
	}{
		{"", true},
		{"missing:u1", true},
		{"Shared:u1", true},
		{"corp-a:u1", false},
		{"corp-a:Alice", true},
		{"corp-a:missing", true},
		{"corp-a", true},
		{"corp-c", false},
		{"Solo", false},
		{"local-a", false},
		{"duplicate", true},
		{"missing", true},
	} {
		_, _, err := resolveProfileSelection("", cfg, tc.selector)
		if (err != nil) != tc.wantErr {
			t.Fatalf("resolveProfileSelection(%q) error = %v", tc.selector, err)
		}
	}
	if _, _, err := resolveProfileSelection("", nil, "x"); err == nil {
		t.Fatal("nil profile config selection succeeded")
	}
	cfg.OrgCurrentProfiles = map[string]string{"corp-a": "corp-a:u2"}
	if got, _, err := resolveProfileSelection("", cfg, "corp-a"); err != nil || got.UserID != "u2" {
		t.Fatalf("organization current selection = %#v %v", got, err)
	}

	for _, tc := range []struct {
		selector string
		wantErr  bool
		exact    bool
	}{
		{"", true, false},
		{"corp-a:u1", false, true},
		{"corp-a", false, false},
		{"Solo", false, false},
		{"local-a", false, true},
		{"duplicate", true, true},
		{"missing", true, false},
	} {
		_, exact, err := resolveProfileDeletionSelection(cfg, tc.selector)
		if (err != nil) != tc.wantErr || (!tc.wantErr && exact != tc.exact) {
			t.Fatalf("resolveProfileDeletionSelection(%q) = exact %v, %v", tc.selector, exact, err)
		}
	}
	if _, _, err := resolveProfileDeletionSelection(nil, "x"); err == nil {
		t.Fatal("nil deletion config succeeded")
	}

	for _, selector := range []string{"", "corp-a", "missing", "Solo", "Shared"} {
		_, _ = resolveOrganizationCorpID(cfg, selector)
	}
	if got, err := resolveOrganizationCorpID(nil, "x"); err != nil || got != "" {
		t.Fatalf("nil organization resolution = %q %v", got, err)
	}
	if _, _, err := resolveOrganizationDefault(cfg, "missing", "missing", nil); err == nil {
		t.Fatal("empty organization default succeeded")
	}
	if got, _, err := resolveOrganizationDefault(cfg, "corp-c", "Solo", profilesForCorpID(cfg, "corp-c")); err != nil || got.UserID != "u4" {
		t.Fatalf("single account organization default = %#v %v", got, err)
	}
	if _, _, err := resolveOrganizationDefault(&ProfilesConfig{}, "corp-a", "corp-a", profilesForCorpID(cfg, "corp-a")); err == nil {
		t.Fatal("ambiguous organization default succeeded")
	}
	if got := profileSelectorCandidates([]*Profile{nil, &cfg.Profiles[1], &cfg.Profiles[0]}); len(got) != 2 || got[0] != "corp-a:u1" {
		t.Fatalf("selector candidates = %#v", got)
	}
	for selector, want := range map[string]bool{
		"":                   false,
		"corp-a:u1":          true,
		"Shared:Bob":         true,
		"corp-c":             true,
		"local-a":            true,
		"definitely-missing": false,
	} {
		if got := profileSelectorReferenceExists(cfg, selector); got != want {
			t.Fatalf("profileSelectorReferenceExists(%q) = %v", selector, got)
		}
	}
	if profileSelectorReferenceExists(nil, "corp-a") {
		t.Fatal("nil profile selector reference exists")
	}

	oldAcquire := profilesAcquireDualLock
	oldEnsure := profilesEnsureMigration
	oldLoad := profilesLoad
	t.Cleanup(func() {
		profilesAcquireDualLock = oldAcquire
		profilesEnsureMigration = oldEnsure
		profilesLoad = oldLoad
	})
	profilesAcquireDualLock = func(context.Context, string) (*DualLock, error) { return &DualLock{}, nil }
	profilesEnsureMigration = func(string) error { return nil }
	profilesLoad = func(string) (*ProfilesConfig, error) { return cfg, nil }
	if got, exact, err := ResolveProfileDeletionScope("cfg", "corp-a:u1"); err != nil || !exact || got.UserID != "u1" {
		t.Fatalf("deletion scope = %#v %v %v", got, exact, err)
	}
	profilesEnsureMigration = func(string) error { return errors.New("migration") }
	if _, _, err := ResolveProfileDeletionScope("cfg", "corp-a"); err == nil {
		t.Fatal("deletion scope migration failure succeeded")
	}
	profilesEnsureMigration = func(string) error { return nil }
	profilesLoad = func(string) (*ProfilesConfig, error) { return nil, errors.New("load") }
	if _, _, err := ResolveProfileDeletionScope("cfg", "corp-a"); err == nil {
		t.Fatal("deletion scope load failure succeeded")
	}

	oldLoadIdentity := profilesLoadIdentity
	oldLoadCorp := profilesLoadCorp
	oldSaveIdentity := profilesSaveIdentity
	t.Cleanup(func() {
		profilesLoadIdentity = oldLoadIdentity
		profilesLoadCorp = oldLoadCorp
		profilesSaveIdentity = oldSaveIdentity
	})
	profile := Profile{CorpID: "corp-a", UserID: "u1"}
	profilesLoadIdentity = func(string, string) (*TokenData, error) { return &TokenData{AccessToken: "identity"}, nil }
	if got, err := loadTokenForProfileIdentity(profile); err != nil || got.AccessToken != "identity" {
		t.Fatalf("identity token load = %#v %v", got, err)
	}
	fail := errors.New("fail")
	profilesLoadIdentity = func(string, string) (*TokenData, error) { return nil, fail }
	if _, err := loadTokenForProfileIdentity(profile); !errors.Is(err, fail) {
		t.Fatalf("identity load failure = %v", err)
	}
	profilesLoadIdentity = func(string, string) (*TokenData, error) { return nil, ErrTokenDataNotFound }
	profilesLoadCorp = func(string) (*TokenData, error) { return nil, ErrTokenDataNotFound }
	if _, err := loadTokenForProfileIdentity(profile); !errors.Is(err, ErrTokenDataNotFound) {
		t.Fatalf("missing organization mirror = %v", err)
	}
	profilesLoadCorp = func(string) (*TokenData, error) { return nil, fail }
	if _, err := loadTokenForProfileIdentity(profile); !errors.Is(err, fail) {
		t.Fatalf("organization mirror failure = %v", err)
	}
	for _, data := range []*TokenData{
		{CorpID: "corp-a"},
		{CorpID: "corp-a", UserID: "other"},
	} {
		profilesLoadCorp = func(string) (*TokenData, error) { return data, nil }
		if _, err := loadTokenForProfileIdentity(profile); err == nil {
			t.Fatalf("invalid organization mirror %#v succeeded", data)
		}
	}
	profilesLoadCorp = func(string) (*TokenData, error) {
		return &TokenData{CorpID: "corp-a", UserID: "u1", AccessToken: "mirror"}, nil
	}
	profilesSaveIdentity = func(string, string, *TokenData) error { return fail }
	if _, err := loadTokenForProfileIdentity(profile); !errors.Is(err, fail) {
		t.Fatalf("identity repair save failure = %v", err)
	}
	profilesSaveIdentity = func(string, string, *TokenData) error { return nil }
	if got, err := loadTokenForProfileIdentity(profile); err != nil || got.AccessToken != "mirror" {
		t.Fatalf("identity repair = %#v %v", got, err)
	}
	if _, err := loadTokenForProfileIdentity(Profile{CorpID: "corp-a"}); err != nil {
		t.Fatalf("organization-only token load = %v", err)
	}
}
