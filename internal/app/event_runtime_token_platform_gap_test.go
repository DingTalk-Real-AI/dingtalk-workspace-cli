// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/consume"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/runtimecred"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCrossPlatformCoverageRuntimeTokenBusRejectsIncompleteIdentity(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "invalid identity hash",
			args: []string{"--source-kind", "personal_stream", "--runtime-token-mode", "--identity-hash", "not-a-hash", "--client-id", "client"},
			want: "16-character hexadecimal identity hash",
		},
		{
			name: "missing client id",
			args: []string{"--source-kind", "personal_stream", "--runtime-token-mode", "--identity-hash", "0123456789abcdef"},
			want: "--client-id is required",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newEventBusCommand()
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Execute() error = %v, want %q", err, tc.want)
			}
		})
	}
}

type eventRuntimeTokenReleaseErrorStore struct {
	err error
}

func (*eventRuntimeTokenReleaseErrorStore) Claim([]personal.AttemptSpec, time.Duration) (*personal.AttemptClaim, error) {
	return nil, nil
}

func (*eventRuntimeTokenReleaseErrorStore) CompleteSuccess(*personal.AttemptClaim) error {
	return nil
}

func (*eventRuntimeTokenReleaseErrorStore) CompleteFailure(*personal.AttemptClaim, []string, personal.AttemptFailure) (personal.AttemptHold, error) {
	return personal.AttemptHold{}, nil
}

func (s *eventRuntimeTokenReleaseErrorStore) Release(*personal.AttemptClaim) error {
	return s.err
}

func TestCrossPlatformCoverageRuntimeTokenAttemptReleaseGuardEdges(t *testing.T) {
	var nilReservation *personalSubscriptionAttemptReservation
	if err := nilReservation.releaseRuntimeTokenFailure(); !errors.Is(err, runtimecred.ErrRuntimeTokenRejected) {
		t.Fatalf("nil reservation error = %v", err)
	}

	incomplete := &personalSubscriptionAttemptReservation{}
	if err := incomplete.releaseRuntimeTokenFailure(); !errors.Is(err, runtimecred.ErrRuntimeTokenRejected) ||
		!strings.Contains(err.Error(), "reservation is incomplete") {
		t.Fatalf("incomplete reservation error = %v", err)
	}

	wantErr := errors.New("release failed")
	reservation := &personalSubscriptionAttemptReservation{
		store: &eventRuntimeTokenReleaseErrorStore{err: wantErr},
		claim: &personal.AttemptClaim{AttemptID: "attempt"},
	}
	if err := reservation.releaseRuntimeTokenFailure(); !errors.Is(err, runtimecred.ErrRuntimeTokenRejected) ||
		!errors.Is(err, wantErr) {
		t.Fatalf("release failure error = %v", err)
	}
}

func TestCrossPlatformCoverageRuntimeTokenConsumeRejectionAndOversizeEdges(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	oldEdition := edition.Get()
	oldProfile := authpkg.RuntimeProfile()
	oldLoadProfiles := personalLoadProfiles
	oldValidate := personalValidateConsumeConfig
	oldConflict := personalValidateNoOutputConflict
	oldAttemptStore := personalNewSubscriptionAttemptStore
	oldEnsure := personalEnsureSubscription
	t.Cleanup(func() {
		edition.Override(oldEdition)
		authpkg.SetRuntimeProfile(oldProfile)
		personalLoadProfiles = oldLoadProfiles
		personalValidateConsumeConfig = oldValidate
		personalValidateNoOutputConflict = oldConflict
		personalNewSubscriptionAttemptStore = oldAttemptStore
		personalEnsureSubscription = oldEnsure
	})
	edition.Override(&edition.Hooks{})
	authpkg.SetRuntimeProfile("")
	personalLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) { return nil, nil }
	personalValidateConsumeConfig = func(consume.Config) error { return nil }
	personalValidateNoOutputConflict = func(consume.Config, string) error { return nil }

	oversized := strings.Repeat("x", runtimecred.DefaultMaxTokenBytes+1)
	err := runPersonalEventConsumeSingle(newPersonalCoverageCommand(), personalConsumeOptions{
		EventKey:         personal.EventMention,
		ExplicitToken:    oversized,
		ClientIDOverride: "runtime-client",
		Common:           commonConsumeOptions{Foreground: true},
	})
	if !errors.Is(err, runtimecred.ErrTokenTooLarge) {
		t.Fatalf("oversized foreground token error = %v", err)
	}

	rejection := &personal.APIError{
		Code:       "RUNTIME_TOKEN_REJECTED",
		HTTPStatus: http.StatusUnauthorized,
	}
	if personalRuntimeTokenControlRejection(errors.New("ordinary failure")) {
		t.Fatal("ordinary error classified as runtime-token rejection")
	}

	singleStore := &personalRecordingAttemptStore{}
	personalNewSubscriptionAttemptStore = func(string) personalSubscriptionAttemptStore { return singleStore }
	personalEnsureSubscription = func(context.Context, *personal.Client, personal.Identity, personalConsumeOptions) (*personal.Subscription, string, string, error) {
		return nil, "", "", rejection
	}
	err = runPersonalEventConsumeSingle(newPersonalCoverageCommand(), personalConsumeOptions{
		EventKey:         personal.EventMention,
		ExplicitToken:    "runtime-token-single",
		ClientIDOverride: "runtime-client",
		ControlBaseURL:   "https://control.example.test",
	})
	if !errors.Is(err, runtimecred.ErrRuntimeTokenRejected) || singleStore.releaseCalls != 1 || singleStore.failureCalls != 0 {
		t.Fatalf("single rejection = %v, release=%d failure=%d", err, singleStore.releaseCalls, singleStore.failureCalls)
	}

	manyStore := &personalRecordingAttemptStore{}
	personalNewSubscriptionAttemptStore = func(string) personalSubscriptionAttemptStore { return manyStore }
	err = runPersonalEventConsumeMany(newPersonalCoverageCommand(), personalConsumeOptions{
		EventKeys:        []string{personal.EventMention, personal.EventAllSingleChat},
		ExplicitToken:    "runtime-token-many",
		ClientIDOverride: "runtime-client",
		ControlBaseURL:   "https://control.example.test",
	})
	if !errors.Is(err, runtimecred.ErrRuntimeTokenRejected) || manyStore.releaseCalls != 1 || manyStore.failureCalls != 0 {
		t.Fatalf("multi rejection = %v, release=%d failure=%d", err, manyStore.releaseCalls, manyStore.failureCalls)
	}
}

func TestCrossPlatformCoverageRuntimeTokenIdentityFallbackEdges(t *testing.T) {
	configDir := t.TempDir()
	oldEdition := edition.Get()
	oldProfile := authpkg.RuntimeProfile()
	oldResolveIdentity := personalResolveEventIdentity
	oldResolveAuxiliary := personalResolveAuxiliaryAccessToken
	oldLoadTokenData := personalLoadTokenData
	oldLoadProfiles := personalLoadProfiles
	oldRuntimeClientID := personalRuntimeEventClientID
	oldClientID := personalClientID
	oldResolveCredentials := personalResolveAppCredentialsStrict
	t.Cleanup(func() {
		edition.Override(oldEdition)
		authpkg.SetRuntimeProfile(oldProfile)
		personalResolveEventIdentity = oldResolveIdentity
		personalResolveAuxiliaryAccessToken = oldResolveAuxiliary
		personalLoadTokenData = oldLoadTokenData
		personalLoadProfiles = oldLoadProfiles
		personalRuntimeEventClientID = oldRuntimeClientID
		personalClientID = oldClientID
		personalResolveAppCredentialsStrict = oldResolveCredentials
	})

	legacy := personal.Identity{ClientID: "legacy-client", SourceID: "legacy-source"}
	personalResolveEventIdentity = func(context.Context, string, string) (personal.Identity, error) { return legacy, nil }
	identity, err := resolvePersonalEventIdentityForToken(context.Background(), configDir, "", "  ")
	if err != nil || identity.ClientID != legacy.ClientID {
		t.Fatalf("wrapper empty-token fallback = %#v, %v", identity, err)
	}
	personalResolveAuxiliaryAccessToken = func(context.Context, string, string) (string, error) {
		return "legacy-access", nil
	}
	personalLoadTokenData = func(string) (*authpkg.TokenData, error) {
		return &authpkg.TokenData{
			CorpID: "legacy-corp", UserID: "legacy-user", ClientID: "direct-client",
		}, nil
	}
	identity, err = resolvePersonalEventIdentityWithToken(context.Background(), configDir, "", "  ")
	if err != nil || identity.ClientID != "direct-client" {
		t.Fatalf("direct empty-token fallback = %#v, %v", identity, err)
	}

	edition.Override(&edition.Hooks{})
	personalRuntimeEventClientID = func() string { return "" }
	personalClientID = func() string { return "" }
	wantMetadataErr := errors.New("profiles unreadable")
	personalLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) { return nil, wantMetadataErr }
	authpkg.SetRuntimeProfile("corp:user")
	if _, err := resolvePersonalEventIdentityWithToken(context.Background(), configDir, "", "token", "runtime-client"); !errors.Is(err, wantMetadataErr) {
		t.Fatalf("explicit profile metadata error = %v", err)
	}

	authpkg.SetRuntimeProfile("")
	personalLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) { return nil, nil }
	personalResolveAppCredentialsStrict = func(string) (string, string, authpkg.CredentialSource, authpkg.CredentialSource, error) {
		return "app-client", "", "", "", nil
	}
	identity, err = resolvePersonalEventIdentityWithToken(context.Background(), configDir, "", "runtime-token")
	if err != nil || identity.ClientID != "app-client" || !strings.HasPrefix(identity.LocalSubject, "access:") {
		t.Fatalf("app-credential fallback identity = %#v, %v", identity, err)
	}

	personalResolveAppCredentialsStrict = func(string) (string, string, authpkg.CredentialSource, authpkg.CredentialSource, error) {
		return "", "", "", "", errors.New("missing app credentials")
	}
	if _, err := resolvePersonalEventIdentityWithToken(context.Background(), configDir, "", "runtime-token"); err == nil || !strings.Contains(err.Error(), "cannot resolve OAuth client_id") {
		t.Fatalf("missing client ID error = %v", err)
	}

	personalLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) {
		return &authpkg.ProfilesConfig{
			CurrentProfile: "stale",
			Profiles:       []authpkg.Profile{{Name: "other", CorpID: "corp", UserID: "user"}},
		}, nil
	}
	profile, err := personalEventProfileMetadata(configDir)
	if err != nil || profile != nil {
		t.Fatalf("stale implicit current profile = %#v, %v", profile, err)
	}

	personalLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) {
		return &authpkg.ProfilesConfig{Profiles: []authpkg.Profile{{Name: "other"}}}, nil
	}
	profile, err = personalEventProfileMetadata(configDir)
	if err != nil || profile != nil {
		t.Fatalf("empty implicit selector = %#v, %v", profile, err)
	}

	authpkg.SetRuntimeProfile("corp-a:user-a,corp-b:user-b")
	personalLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) { return nil, nil }
	if _, err := personalEventProfileMetadata(configDir); err == nil || !strings.Contains(err.Error(), "exactly one --profile") {
		t.Fatalf("multi-profile metadata error = %v", err)
	}
}

type eventRuntimeTokenReadErrorBody struct{}

func (eventRuntimeTokenReadErrorBody) Read([]byte) (int, error) {
	return 0, errors.New("body read failed")
}

func (eventRuntimeTokenReadErrorBody) Close() error { return nil }

func TestCrossPlatformCoverageRuntimeTokenControlTransportErrorEdges(t *testing.T) {
	const token = "runtime-control-edge-canary"

	unsupported, err := http.NewRequest(http.MethodGet, "unsupported://control.example.test/path", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (runtimeTokenControlTransport{}).RoundTrip(unsupported); err == nil {
		t.Fatal("nil base unexpectedly accepted an unsupported protocol")
	}

	wantTransportErr := errors.New("ordinary transport failure")
	ordinary := runtimeTokenControlTransport{base: eventRuntimeRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, wantTransportErr
	})}
	if _, err := ordinary.RoundTrip(unsupported); !errors.Is(err, wantTransportErr) {
		t.Fatalf("ordinary transport error = %v", err)
	}

	leaking := runtimeTokenControlTransport{
		token: token,
		base: eventRuntimeRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("reflected " + token)
		}),
	}
	if _, err := leaking.RoundTrip(unsupported); err == nil || strings.Contains(err.Error(), token) ||
		err.Error() != "personal event: runtime-token control request failed" {
		t.Fatalf("redacted transport error = %v", err)
	}

	nilResponse := runtimeTokenControlTransport{base: eventRuntimeRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, nil
	})}
	if resp, err := nilResponse.RoundTrip(unsupported); resp != nil || err != nil {
		t.Fatalf("nil response = %#v, %v", resp, err)
	}

	readFailure := runtimeTokenControlTransport{base: eventRuntimeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Header:     make(http.Header),
			Body:       eventRuntimeTokenReadErrorBody{},
			Request:    req,
		}, nil
	})}
	if _, err := readFailure.RoundTrip(unsupported); err == nil || !strings.Contains(err.Error(), "read runtime-token control response") {
		t.Fatalf("body read error = %v", err)
	}

	nilHeader := runtimeTokenControlTransport{base: eventRuntimeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})}
	resp, err := nilHeader.RoundTrip(unsupported)
	if err != nil || resp == nil || resp.Header == nil || resp.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("nil-header response = %#v, %v", resp, err)
	}

	if got := redactRuntimeTokenResponseBody(nil, token); len(got) != 0 {
		t.Fatalf("empty response redaction = %q", got)
	}
	value, changed := redactRuntimeTokenJSONValue([]any{"plain", "prefix-" + token}, token)
	items, ok := value.([]any)
	if !ok || !changed || len(items) != 2 || strings.Contains(items[1].(string), token) {
		t.Fatalf("array redaction = %#v changed=%t", value, changed)
	}
}
