// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/bus"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/consume"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/runtimecred"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/source"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageEventCommandsWireTrimmedRootRuntimeToken(t *testing.T) {
	oldConsume := eventRunPersonalConsume
	oldStatus := eventRunPersonalStatus
	oldStop := eventRunPersonalStop
	t.Cleanup(func() {
		eventRunPersonalConsume = oldConsume
		eventRunPersonalStatus = oldStatus
		eventRunPersonalStop = oldStop
	})

	flags := &GlobalFlags{Token: "  runtime-canary  ", ClientID: " root-client "}
	assertIdentity := func(token, clientID string) {
		t.Helper()
		if token != "runtime-canary" || clientID != "root-client" {
			t.Fatalf("runtime identity = token %q client %q", token, clientID)
		}
	}

	eventRunPersonalConsume = func(_ *cobra.Command, opts personalConsumeOptions) error {
		assertIdentity(opts.ExplicitToken, opts.ClientIDOverride)
		return nil
	}
	consumeCmd := newEventConsumeCommand(flags)
	if err := consumeCmd.RunE(consumeCmd, []string{personal.EventMention}); err != nil {
		t.Fatalf("consume RunE() error = %v", err)
	}

	eventRunPersonalStatus = func(_ *cobra.Command, opts personalStatusOptions) error {
		assertIdentity(opts.ExplicitToken, opts.ClientIDOverride)
		return nil
	}
	statusCmd := newEventStatusCommandWithFlags(flags)
	if err := statusCmd.RunE(statusCmd, nil); err != nil {
		t.Fatalf("status RunE() error = %v", err)
	}

	eventRunPersonalStop = func(_ *cobra.Command, opts personalStopOptions) error {
		assertIdentity(opts.ExplicitToken, opts.ClientIDOverride)
		return nil
	}
	stopCmd := newEventStopCommandWithFlags(flags)
	stopRoot := &cobra.Command{Use: "dws"}
	stopRoot.PersistentFlags().Bool("yes", true, "")
	stopRoot.AddCommand(stopCmd)
	if err := stopCmd.RunE(stopCmd, []string{"sub-runtime"}); err != nil {
		t.Fatalf("stop RunE() error = %v", err)
	}

	listenCmd := newEventListenIMCommand(flags)
	if err := listenCmd.RunE(listenCmd, nil); err != nil {
		t.Fatalf("listen-im RunE() error = %v", err)
	}
}

func TestCrossPlatformCoverageEventConsumeParsesRootRuntimeTokenBeforeAndAfterSubcommand(t *testing.T) {
	oldConsume := eventRunPersonalConsume
	t.Cleanup(func() { eventRunPersonalConsume = oldConsume })
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "before", args: []string{"--token", "runtime-before", "event", "consume", personal.EventMention}},
		{name: "after", args: []string{"event", "consume", personal.EventMention, "--token", "runtime-after"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flags := &GlobalFlags{}
			root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
			bindPersistentFlags(root, flags)
			root.AddCommand(newEventCommand(flags))
			var got string
			eventRunPersonalConsume = func(_ *cobra.Command, opts personalConsumeOptions) error {
				got = opts.ExplicitToken
				return nil
			}
			root.SetArgs(tc.args)
			if err := root.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			want := "runtime-" + tc.name
			if got != want {
				t.Fatalf("ExplicitToken = %q, want %q", got, want)
			}
		})
	}
}

func TestCrossPlatformCoverageResolvePersonalEventIdentityWithTokenUsesMetadataOnly(t *testing.T) {
	oldEdition := edition.Get()
	oldLoadProfiles := personalLoadProfiles
	oldLoadToken := personalLoadTokenData
	oldAux := personalResolveAuxiliaryAccessToken
	oldClientID := personalClientID
	oldCredentials := personalResolveAppCredentialsStrict
	previousProfile := authpkg.RuntimeProfile()
	t.Cleanup(func() {
		edition.Override(oldEdition)
		personalLoadProfiles = oldLoadProfiles
		personalLoadTokenData = oldLoadToken
		personalResolveAuxiliaryAccessToken = oldAux
		personalClientID = oldClientID
		personalResolveAppCredentialsStrict = oldCredentials
		authpkg.SetRuntimeProfile(previousProfile)
	})

	personalLoadTokenData = func(string) (*authpkg.TokenData, error) {
		t.Fatal("explicit token identity read sensitive TokenData")
		return nil, nil
	}
	personalResolveAuxiliaryAccessToken = func(context.Context, string, string) (string, error) {
		t.Fatal("explicit token identity resolved local OAuth")
		return "", nil
	}
	personalClientID = func() string {
		t.Fatal("explicit root client ID was not preferred")
		return ""
	}
	personalResolveAppCredentialsStrict = func(string) (string, string, authpkg.CredentialSource, authpkg.CredentialSource, error) {
		t.Fatal("explicit root client ID unexpectedly fell back to app credentials")
		return "", "", "", "", nil
	}
	personalLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) {
		return &authpkg.ProfilesConfig{
			Version: 2,
			Profiles: []authpkg.Profile{{
				Name:     "Runtime profile",
				CorpID:   "profile-corp",
				CorpName: "Runtime Org",
				UserID:   "profile-user",
				UserName: "Runtime User",
				ClientID: "profile-client",
			}},
		}, nil
	}
	authpkg.SetRuntimeProfile("Runtime Org:Runtime User")
	edition.Override(&edition.Hooks{RuntimeDefaults: func() map[string]edition.RuntimeDefaultFn {
		return map[string]edition.RuntimeDefaultFn{
			"$corpId":        func(context.Context) (string, bool) { return "runtime-corp", true },
			"$currentUserId": func(context.Context) (string, bool) { return "", false },
		}
	}})

	identity, err := resolvePersonalEventIdentityWithToken(
		context.Background(), "unused", "runtime-source", " runtime-canary ", "root-client",
	)
	if err != nil {
		t.Fatalf("resolvePersonalEventIdentityWithToken() error = %v", err)
	}
	if identity.CorpID != "runtime-corp" || identity.UserID != "profile-user" || identity.ClientID != "root-client" {
		t.Fatalf("identity metadata = %#v", identity)
	}
	if identity.AccessToken != "" {
		t.Fatalf("identity retained raw runtime token: %q", identity.AccessToken)
	}
	if identity.LocalSubject != "" {
		t.Fatalf("complete identity LocalSubject = %q, want empty", identity.LocalSubject)
	}
}

func TestCrossPlatformCoverageResolvePersonalEventIdentityWithCompleteRuntimeMetadataSkipsProfiles(t *testing.T) {
	oldEdition := edition.Get()
	oldLoadProfiles := personalLoadProfiles
	oldRuntimeClientID := personalRuntimeEventClientID
	t.Cleanup(func() {
		edition.Override(oldEdition)
		personalLoadProfiles = oldLoadProfiles
		personalRuntimeEventClientID = oldRuntimeClientID
	})
	personalLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) {
		t.Fatal("complete host metadata unexpectedly read profiles.json")
		return nil, nil
	}
	edition.Override(&edition.Hooks{RuntimeDefaults: func() map[string]edition.RuntimeDefaultFn {
		return map[string]edition.RuntimeDefaultFn{
			"$corpId":        func(context.Context) (string, bool) { return "runtime-corp", true },
			"$currentUserId": func(context.Context) (string, bool) { return "runtime-user", true },
		}
	}})
	personalRuntimeEventClientID = func() string { return "edition-client" }
	identity, err := resolvePersonalEventIdentityWithToken(context.Background(), "unused", "source", "canary", "root-client")
	if err != nil {
		t.Fatalf("resolvePersonalEventIdentityWithToken() error = %v", err)
	}
	if identity.CorpID != "runtime-corp" || identity.UserID != "runtime-user" || identity.ClientID != "root-client" {
		t.Fatalf("identity = %#v", identity)
	}
}

func TestCrossPlatformCoverageRuntimeEventClientIDPrefersEditionBeforeEnvironment(t *testing.T) {
	oldEdition := edition.Get()
	t.Cleanup(func() { edition.Override(oldEdition) })
	t.Setenv("DWS_CLIENT_ID", "environment-client")

	edition.Override(&edition.Hooks{AuthClientID: "edition-client"})
	if got := runtimePersonalEventClientID(); got != "edition-client" {
		t.Fatalf("runtime client ID = %q, want edition hook", got)
	}
	edition.Override(&edition.Hooks{})
	if got := runtimePersonalEventClientID(); got != "environment-client" {
		t.Fatalf("runtime client ID = %q, want environment fallback", got)
	}
}

func TestCrossPlatformCoverageCompleteRuntimeIdentityUsesEditionClientBeforeProfiles(t *testing.T) {
	oldEdition := edition.Get()
	oldLoadProfiles := personalLoadProfiles
	oldRuntimeClientID := personalRuntimeEventClientID
	t.Cleanup(func() {
		edition.Override(oldEdition)
		personalLoadProfiles = oldLoadProfiles
		personalRuntimeEventClientID = oldRuntimeClientID
	})
	personalLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) {
		t.Fatal("complete host metadata unexpectedly read profiles.json")
		return nil, errors.New("unreachable")
	}
	personalRuntimeEventClientID = func() string { return "edition-client" }
	edition.Override(&edition.Hooks{RuntimeDefaults: func() map[string]edition.RuntimeDefaultFn {
		return map[string]edition.RuntimeDefaultFn{
			"$corpId":        func(context.Context) (string, bool) { return "runtime-corp", true },
			"$currentUserId": func(context.Context) (string, bool) { return "runtime-user", true },
		}
	}})
	identity, err := resolvePersonalEventIdentityWithToken(context.Background(), "unused", "source", "canary")
	if err != nil {
		t.Fatalf("resolvePersonalEventIdentityWithToken() error = %v", err)
	}
	if identity.CorpID != "runtime-corp" || identity.UserID != "runtime-user" || identity.ClientID != "edition-client" {
		t.Fatalf("identity = %#v", identity)
	}
}

func TestCrossPlatformCoverageSelectedProfileClientPrecedesPersistedGlobalClient(t *testing.T) {
	oldEdition := edition.Get()
	oldLoadProfiles := personalLoadProfiles
	oldRuntimeClientID := personalRuntimeEventClientID
	oldClientID := personalClientID
	previousProfile := authpkg.RuntimeProfile()
	t.Cleanup(func() {
		edition.Override(oldEdition)
		personalLoadProfiles = oldLoadProfiles
		personalRuntimeEventClientID = oldRuntimeClientID
		personalClientID = oldClientID
		authpkg.SetRuntimeProfile(previousProfile)
	})
	edition.Override(&edition.Hooks{})
	personalRuntimeEventClientID = func() string { return "" }
	personalClientID = func() string { return "stale-global-client" }
	personalLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) {
		return &authpkg.ProfilesConfig{
			Version:        3,
			CurrentProfile: "corp:user",
			Profiles: []authpkg.Profile{{
				Name: "Selected", CorpID: "corp", UserID: "user", ClientID: "profile-client",
			}},
		}, nil
	}
	authpkg.SetRuntimeProfile("corp:user")
	identity, err := resolvePersonalEventIdentityWithToken(context.Background(), "unused", "source", "canary")
	if err != nil {
		t.Fatalf("resolvePersonalEventIdentityWithToken() error = %v", err)
	}
	if identity.ClientID != "profile-client" {
		t.Fatalf("ClientID = %q, want selected profile client", identity.ClientID)
	}
}

func TestCrossPlatformCoverageMalformedPersistedProfilesDoNotBlockRuntimeDefaultsAndGlobalClient(t *testing.T) {
	oldEdition := edition.Get()
	oldLoadProfiles := personalLoadProfiles
	oldRuntimeClientID := personalRuntimeEventClientID
	oldClientID := personalClientID
	previousProfile := authpkg.RuntimeProfile()
	t.Cleanup(func() {
		edition.Override(oldEdition)
		personalLoadProfiles = oldLoadProfiles
		personalRuntimeEventClientID = oldRuntimeClientID
		personalClientID = oldClientID
		authpkg.SetRuntimeProfile(previousProfile)
	})
	authpkg.SetRuntimeProfile("")
	personalRuntimeEventClientID = func() string { return "" }
	personalClientID = func() string { return "global-client" }
	personalLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) {
		return nil, errors.New("malformed persisted profiles")
	}
	edition.Override(&edition.Hooks{RuntimeDefaults: func() map[string]edition.RuntimeDefaultFn {
		return map[string]edition.RuntimeDefaultFn{
			"$corpId":        func(context.Context) (string, bool) { return "runtime-corp", true },
			"$currentUserId": func(context.Context) (string, bool) { return "runtime-user", true },
		}
	}})

	identity, err := resolvePersonalEventIdentityWithToken(context.Background(), "unused", "source", "canary")
	if err != nil {
		t.Fatalf("resolvePersonalEventIdentityWithToken() error = %v", err)
	}
	if identity.CorpID != "runtime-corp" || identity.UserID != "runtime-user" || identity.ClientID != "global-client" {
		t.Fatalf("identity = %#v", identity)
	}
}

func TestCrossPlatformCoverageResolvePersonalEventIdentityWithTokenRejectsMultipleProfilesBeforeMetadata(t *testing.T) {
	oldEdition := edition.Get()
	oldLoadProfiles := personalLoadProfiles
	previousProfile := authpkg.RuntimeProfile()
	t.Cleanup(func() {
		edition.Override(oldEdition)
		personalLoadProfiles = oldLoadProfiles
		authpkg.SetRuntimeProfile(previousProfile)
	})
	personalLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) {
		t.Fatal("multiple runtime profiles unexpectedly reached metadata loading")
		return nil, nil
	}
	authpkg.SetRuntimeProfile("corp-a:user-a,corp-b:user-b")
	edition.Override(&edition.Hooks{RuntimeDefaults: func() map[string]edition.RuntimeDefaultFn {
		return map[string]edition.RuntimeDefaultFn{
			"$corpId":        func(context.Context) (string, bool) { return "runtime-corp", true },
			"$currentUserId": func(context.Context) (string, bool) { return "runtime-user", true },
		}
	}})
	_, err := resolvePersonalEventIdentityWithToken(context.Background(), "unused", "source", "canary", "root-client")
	if err == nil || !strings.Contains(err.Error(), "exactly one --profile") {
		t.Fatalf("multiple-profile error = %v", err)
	}
}

func TestCrossPlatformCoverageExplicitProfileRequiresMetadataRegistry(t *testing.T) {
	oldLoadProfiles := personalLoadProfiles
	defer func() { personalLoadProfiles = oldLoadProfiles }()
	oldProfile := authpkg.RuntimeProfile()
	authpkg.SetRuntimeProfile("missing-profile")
	defer authpkg.SetRuntimeProfile(oldProfile)

	personalLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) {
		return &authpkg.ProfilesConfig{}, nil
	}
	_, err := personalEventProfileMetadata(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), `profile "missing-profile" not found`) {
		t.Fatalf("personalEventProfileMetadata() error = %v", err)
	}
}

func TestCrossPlatformCoverageCompleteRuntimeIdentityStillValidatesExplicitProfile(t *testing.T) {
	oldEdition := edition.Get()
	oldLoadProfiles := personalLoadProfiles
	oldRuntimeClientID := personalRuntimeEventClientID
	oldProfile := authpkg.RuntimeProfile()
	t.Cleanup(func() {
		edition.Override(oldEdition)
		personalLoadProfiles = oldLoadProfiles
		personalRuntimeEventClientID = oldRuntimeClientID
		authpkg.SetRuntimeProfile(oldProfile)
	})

	authpkg.SetRuntimeProfile("missing-profile")
	personalLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) {
		return &authpkg.ProfilesConfig{}, nil
	}
	personalRuntimeEventClientID = func() string { return "runtime-client" }
	edition.Override(&edition.Hooks{RuntimeDefaults: func() map[string]edition.RuntimeDefaultFn {
		return map[string]edition.RuntimeDefaultFn{
			"$corpId":        func(context.Context) (string, bool) { return "runtime-corp", true },
			"$currentUserId": func(context.Context) (string, bool) { return "runtime-user", true },
		}
	}})

	_, err := resolvePersonalEventIdentityWithToken(context.Background(), t.TempDir(), "source", "canary")
	if err == nil || !strings.Contains(err.Error(), `profile "missing-profile" not found`) {
		t.Fatalf("resolvePersonalEventIdentityWithToken() error = %v", err)
	}
}

func TestCrossPlatformCoveragePersonalProfileMetadataOrganizationCurrentBeatsUnresolved(t *testing.T) {
	cfg := &authpkg.ProfilesConfig{
		Version: 3,
		Profiles: []authpkg.Profile{
			{Name: "Historical", CorpID: "corp-1"},
			{Name: "Exact", CorpID: "corp-1", UserID: "user-1"},
		},
		OrgCurrentProfiles: map[string]string{"corp-1": "corp-1:user-1"},
	}
	profile, err := selectPersonalEventProfileMetadata(cfg, "corp-1", make(map[string]struct{}))
	if err != nil {
		t.Fatalf("selectPersonalEventProfileMetadata() error = %v", err)
	}
	if profile == nil || profile.UserID != "user-1" {
		t.Fatalf("selected profile = %#v, want organization current account", profile)
	}
}

func TestCrossPlatformCoverageExplicitTokenControlClientRedactsReflected401(t *testing.T) {
	const token = "runtime-control-canary"
	oldLogger := slog.Default()
	var logs bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(oldLogger) })

	client := newPersonalEventControlClient("unused", "https://control.invalid", personal.Identity{
		ClientID: "client", SourceID: "source",
	}, token)
	wrapped, ok := client.HTTPClient.Transport.(runtimeTokenControlTransport)
	if !ok {
		t.Fatalf("control transport = %T, want runtimeTokenControlTransport", client.HTTPClient.Transport)
	}
	var authorization string
	wrapped.base = eventRuntimeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		authorization = req.Header.Get("Authorization")
		body := `{"code":"UNAUTHORIZED","message":"rejected ` + token + `"}`
		header := make(http.Header)
		header.Set("X-Request-Id", "request-"+token)
		header.Set("X-Trace-Id", "trace-"+token)
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Header:     header,
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})
	client.HTTPClient.Transport = wrapped

	_, err := client.ListSubscriptions(context.Background(), personal.ListOptions{})
	if err == nil {
		t.Fatal("ListSubscriptions() unexpectedly succeeded")
	}
	if authorization != "Bearer "+token {
		t.Fatalf("Authorization = %q", authorization)
	}
	if strings.Contains(err.Error(), token) || strings.Contains(logs.String(), token) {
		t.Fatalf("runtime token leaked: error=%q logs=%q", err, logs.String())
	}
	if !strings.Contains(err.Error(), "RUNTIME_TOKEN_REJECTED") {
		t.Fatalf("error = %q, want fixed runtime token rejection", err)
	}
}

func TestCrossPlatformCoverageRuntimeTokenRedirectGuardDoesNotForwardCustomHeader(t *testing.T) {
	const token = "runtime-redirect-canary"
	var controlTargetHits, ticketTargetHits atomic.Int32
	controlTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		controlTargetHits.Add(1)
		if r.Header.Get("x-user-access-token") == token {
			t.Error("control redirect forwarded runtime token")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer controlTarget.Close()
	controlOrigin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-user-access-token") != token {
			t.Error("control origin did not receive runtime token")
		}
		http.Redirect(w, r, controlTarget.URL, http.StatusFound)
	}))
	defer controlOrigin.Close()

	client := newPersonalEventControlClient("unused", controlOrigin.URL, personal.Identity{
		ClientID: "client", SourceID: "source",
	}, token)
	_, controlErr := client.ListSubscriptions(context.Background(), personal.ListOptions{})
	if controlErr == nil {
		t.Fatal("cross-host control redirect unexpectedly succeeded")
	}
	if controlTargetHits.Load() != 0 || strings.Contains(controlErr.Error(), token) {
		t.Fatalf("control redirect hits=%d error=%q", controlTargetHits.Load(), controlErr)
	}

	ticketTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ticketTargetHits.Add(1)
		if r.Header.Get("x-user-access-token") == token {
			t.Error("ticket redirect forwarded runtime token")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ticketTarget.Close()
	ticketOrigin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-user-access-token") != token {
			t.Error("ticket origin did not receive runtime token")
		}
		http.Redirect(w, r, ticketTarget.URL, http.StatusFound)
	}))
	defer ticketOrigin.Close()

	broker := runtimecred.New(runtimecred.Config{RequireSeed: true})
	if _, err := broker.Update(0, token); err != nil {
		t.Fatal(err)
	}
	src, err := newPersonalStreamSource(context.Background(), personalStreamSourceOptions{
		ConfigDir:        "unused",
		Identity:         personal.Identity{ClientID: "client", SourceID: "source"},
		TicketURL:        ticketOrigin.URL,
		CredentialBroker: broker,
		RuntimeTokenMode: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = src.Start(context.Background(), func(*dwsevent.RawEvent) {})
	if err == nil {
		t.Fatal("cross-host ticket redirect unexpectedly succeeded")
	}
	if ticketTargetHits.Load() != 0 || strings.Contains(err.Error(), token) {
		t.Fatalf("ticket redirect hits=%d error=%q", ticketTargetHits.Load(), err)
	}
}

func TestCrossPlatformCoverageRuntimeTokenRedirectPolicyBranches(t *testing.T) {
	origin, _ := http.NewRequest(http.MethodGet, "https://control.example/start", nil)
	sameHost, _ := http.NewRequest(http.MethodGet, "https://control.example/next", nil)
	if err := runtimeTokenRedirectPolicy(sameHost, []*http.Request{origin}); err != nil {
		t.Fatalf("same-host HTTPS redirect rejected: %v", err)
	}
	for name, request := range map[string]*http.Request{
		"cross-host": func() *http.Request {
			r, _ := http.NewRequest(http.MethodGet, "https://other.example/next", nil)
			return r
		}(),
		"downgrade": func() *http.Request {
			r, _ := http.NewRequest(http.MethodGet, "http://control.example/next", nil)
			return r
		}(),
	} {
		if err := runtimeTokenRedirectPolicy(request, []*http.Request{origin}); !errors.Is(err, http.ErrUseLastResponse) {
			t.Fatalf("%s redirect policy error = %v", name, err)
		}
	}
	if err := runtimeTokenRedirectPolicy(nil, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("empty redirect chain error = %v", err)
	}
}

func TestCrossPlatformCoverageExplicitTokenControlClientRedactsEveryErrorEnvelope(t *testing.T) {
	const token = "runtime-control-all-status-canary"
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "bad-request", status: http.StatusBadRequest, body: `{"code":"BAD_REQUEST","message":"` + token + `"}`},
		{name: "server-error", status: http.StatusInternalServerError, body: `{"code":"INTERNAL","message":"` + token + `"}`},
		{name: "success-false", status: http.StatusOK, body: `{"success":false,"errorCode":"DENIED","errorMsg":"` + token + `"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			oldLogger := slog.Default()
			var logs bytes.Buffer
			slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
			t.Cleanup(func() { slog.SetDefault(oldLogger) })

			client := newPersonalEventControlClient("unused", "https://control.invalid", personal.Identity{
				ClientID: "client", SourceID: "source",
			}, token)
			wrapped := client.HTTPClient.Transport.(runtimeTokenControlTransport)
			wrapped.base = eventRuntimeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				header := make(http.Header)
				header.Set("X-Request-Id", "request-"+token)
				header.Set("X-Trace-Id", "trace-"+token)
				return &http.Response{
					StatusCode: tc.status,
					Header:     header,
					Body:       io.NopCloser(strings.NewReader(tc.body)),
					Request:    req,
				}, nil
			})
			client.HTTPClient.Transport = wrapped

			_, err := client.ListSubscriptions(context.Background(), personal.ListOptions{})
			if err == nil {
				t.Fatal("ListSubscriptions() unexpectedly succeeded")
			}
			if strings.Contains(err.Error(), token) || strings.Contains(logs.String(), token) {
				t.Fatalf("runtime token leaked: error=%q logs=%q", err, logs.String())
			}
		})
	}
}

func TestCrossPlatformCoverageExplicitTokenControlTransportPreservesSuccessfulResponse(t *testing.T) {
	client := newPersonalEventControlClient("unused", "https://control.invalid", personal.Identity{
		ClientID: "client", SourceID: "source",
	}, "runtime-success-canary")
	wrapped := client.HTTPClient.Transport.(runtimeTokenControlTransport)
	wrapped.base = eventRuntimeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"success":true,"result":{"items":[],"total":0}}`)),
			Request:    req,
		}, nil
	})
	client.HTTPClient.Transport = wrapped
	if _, err := client.ListSubscriptions(context.Background(), personal.ListOptions{}); err != nil {
		t.Fatalf("ListSubscriptions() successful response error = %v", err)
	}
}

func TestCrossPlatformCoverageExplicitTokenControlClientRedactsJSONEscapedToken(t *testing.T) {
	const token = "runtime<escaped>&canary"
	body, err := json.Marshal(map[string]any{"code": "BAD_REQUEST", "message": "rejected " + token})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte(token)) {
		t.Fatalf("fixture was not JSON-escaped: %s", body)
	}
	oldLogger := slog.Default()
	var logs bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(oldLogger) })

	client := newPersonalEventControlClient("unused", "https://control.invalid", personal.Identity{
		ClientID: "client", SourceID: "source",
	}, token)
	wrapped := client.HTTPClient.Transport.(runtimeTokenControlTransport)
	wrapped.base = eventRuntimeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(body)),
			Request:    req,
		}, nil
	})
	client.HTTPClient.Transport = wrapped
	_, err = client.ListSubscriptions(context.Background(), personal.ListOptions{})
	if err == nil {
		t.Fatal("ListSubscriptions() unexpectedly succeeded")
	}
	if strings.Contains(err.Error(), token) || strings.Contains(logs.String(), token) {
		t.Fatalf("escaped runtime token leaked: error=%q logs=%q", err, logs.String())
	}
}

func TestCrossPlatformCoverageRuntimeTokenBusModeSkipsLocalOAuthIdentity(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	oldResolve := eventResolvePersonal
	oldSource := eventNewPersonalSource
	oldRun := eventBusRun
	t.Cleanup(func() {
		eventResolvePersonal = oldResolve
		eventNewPersonalSource = oldSource
		eventBusRun = oldRun
	})

	resolvedLocal := false
	eventResolvePersonal = func(context.Context, string, string) (personal.Identity, error) {
		resolvedLocal = true
		return personal.Identity{}, nil
	}
	var sourceOpts personalStreamSourceOptions
	eventNewPersonalSource = func(_ context.Context, opts personalStreamSourceOptions) (*source.PersonalSource, error) {
		sourceOpts = opts
		return nil, nil
	}
	var busCfg bus.Config
	eventBusRun = func(_ context.Context, cfg bus.Config) error {
		busCfg = cfg
		return nil
	}

	cmd := newEventBusCommand()
	cmd.SetArgs([]string{
		"--source-kind", "personal_stream",
		"--runtime-token-mode",
		"--identity-hash", "0123456789abcdef",
		"--client-id", "runtime-client",
		"--stream-source-id", "runtime-source",
		"--idle-timeout", "0",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("event _bus runtime mode error = %v", err)
	}
	if resolvedLocal {
		t.Fatal("runtime token bus resolved local OAuth identity")
	}
	if sourceOpts.CredentialBroker == nil || busCfg.CredentialBroker != sourceOpts.CredentialBroker {
		t.Fatal("personal source and bus did not share one credential broker")
	}
	if busCfg.IdentityHash != "0123456789abcdef" || busCfg.ClientID != "runtime-client" || busCfg.SourceID != "runtime-source" {
		t.Fatalf("bus identity = %#v", busCfg)
	}
	generation, err := sourceOpts.CredentialBroker.Update(0, "detached-activation-canary")
	if err != nil {
		t.Fatalf("seed detached broker: %v", err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := sourceOpts.CredentialBroker.Resolve(waitCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("detached broker resolved before consumer activation: %v", err)
	}
	if _, err := sourceOpts.CredentialBroker.Activate(generation); err != nil {
		t.Fatalf("activate detached broker: %v", err)
	}
	if resolved, err := sourceOpts.CredentialBroker.Resolve(context.Background()); err != nil || resolved == "" {
		t.Fatalf("detached broker did not resolve after activation: %v", err)
	}
}

func TestCrossPlatformCoverageForegroundRuntimeBrokerDoesNotRequireActivation(t *testing.T) {
	broker := newPersonalCredentialBroker(t.TempDir(), true, false)
	if _, err := broker.Update(0, "foreground-activation-canary"); err != nil {
		t.Fatalf("seed foreground broker: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if resolved, err := broker.Resolve(ctx); err != nil || resolved == "" {
		t.Fatalf("foreground broker unexpectedly waited for activation: %v", err)
	}
}

func TestCrossPlatformCoveragePersonalRuntimeBusSpawnArgsContainNoSecretOrProfile(t *testing.T) {
	const token = "runtime-spawn-canary"
	args := personalBusSpawnArgsForToken(personal.Identity{
		ClientID: "client", SourceID: "source", CorpID: "corp", UserID: "user",
	}, "identity-hash", "normal", "https://ticket.invalid", "corp:user", token)
	joined := strings.Join(args, " ")
	for _, forbidden := range []string{token, "--profile", "corp:user"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("spawn args leaked %q: %q", forbidden, joined)
		}
	}
	for _, required := range []string{"--runtime-token-mode", "--identity-hash", "identity-hash", "--stream-source-id", "source"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("spawn args %q missing %q", joined, required)
		}
	}
}

func TestCrossPlatformCoverageUnsupportedOldBusDoesNotDeleteReusedSubscription(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	oldEdition := edition.Get()
	oldEnsure := personalEnsureSubscription
	oldUpsert := personalUpsertRunState
	oldDelete := personalDeleteSubscription
	oldRemove := personalRemoveRunStates
	oldConsume := personalConsumeRun
	oldValidate := personalValidateConsumeConfig
	oldConflict := personalValidateNoOutputConflict
	t.Cleanup(func() {
		edition.Override(oldEdition)
		personalEnsureSubscription = oldEnsure
		personalUpsertRunState = oldUpsert
		personalDeleteSubscription = oldDelete
		personalRemoveRunStates = oldRemove
		personalConsumeRun = oldConsume
		personalValidateConsumeConfig = oldValidate
		personalValidateNoOutputConflict = oldConflict
	})
	edition.Override(&edition.Hooks{RuntimeDefaults: func() map[string]edition.RuntimeDefaultFn {
		return map[string]edition.RuntimeDefaultFn{
			"$corpId":        func(context.Context) (string, bool) { return "runtime-corp", true },
			"$currentUserId": func(context.Context) (string, bool) { return "runtime-user", true },
		}
	}})
	personalEnsureSubscription = func(context.Context, *personal.Client, personal.Identity, personalConsumeOptions) (*personal.Subscription, string, string, error) {
		return &personal.Subscription{SubscribeID: "sub-existing"}, personal.EventMention, "at", nil
	}
	personalUpsertRunState = func(string, personal.RunState) error { return nil }
	deleteCalls := 0
	personalDeleteSubscription = func(*personal.Client, context.Context, string) error {
		deleteCalls++
		return nil
	}
	var removed []string
	personalRemoveRunStates = func(_ string, ids []string) error {
		removed = append(removed, ids...)
		return nil
	}
	personalValidateConsumeConfig = func(consume.Config) error { return nil }
	personalValidateNoOutputConflict = func(consume.Config, string) error { return nil }
	personalConsumeRun = func(_ context.Context, cfg consume.Config) error {
		if strings.TrimSpace(cfg.RuntimeToken) == "" {
			t.Fatal("runtime token was not wired to consume")
		}
		return &consume.RuntimeTokenUnsupportedError{BusPID: 72}
	}

	err := runPersonalEventConsumeSingle(newPersonalCoverageCommand(), personalConsumeOptions{
		SubscribeID:      "sub-existing",
		ExplicitToken:    "old-bus-cleanup-canary",
		ClientIDOverride: "runtime-client",
	})
	if !errors.Is(err, consume.ErrRuntimeTokenUnsupported) {
		t.Fatalf("consume error = %v", err)
	}
	if deleteCalls != 0 {
		t.Fatalf("reused remote subscription was deleted %d time(s)", deleteCalls)
	}
	if len(removed) != 0 {
		t.Fatalf("reused local run-state was removed: %#v", removed)
	}
}

func TestCrossPlatformCoverageRuntimeTokenReusedDryRunUsesExplicitControlCredential(t *testing.T) {
	const token = "runtime-dry-run-control-canary"
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	oldEdition := edition.Get()
	oldEnsure := personalEnsureSubscription
	oldUpsert := personalUpsertRunState
	oldConsume := personalConsumeRun
	oldBusRun := personalBusRun
	t.Cleanup(func() {
		edition.Override(oldEdition)
		personalEnsureSubscription = oldEnsure
		personalUpsertRunState = oldUpsert
		personalConsumeRun = oldConsume
		personalBusRun = oldBusRun
	})
	edition.Override(&edition.Hooks{RuntimeDefaults: func() map[string]edition.RuntimeDefaultFn {
		return map[string]edition.RuntimeDefaultFn{
			"$corpId":        func(context.Context) (string, bool) { return "runtime-corp", true },
			"$currentUserId": func(context.Context) (string, bool) { return "runtime-user", true },
		}
	}})

	personalEnsureSubscription = func(ctx context.Context, client *personal.Client, _ personal.Identity, _ personalConsumeOptions) (*personal.Subscription, string, string, error) {
		if _, ok := client.HTTPClient.Transport.(runtimeTokenControlTransport); !ok {
			t.Fatalf("control transport = %T, want runtimeTokenControlTransport", client.HTTPClient.Transport)
		}
		got, err := client.AccessTokenProvider(ctx)
		if err != nil || got != token {
			t.Fatalf("control token = %q, %v", got, err)
		}
		return &personal.Subscription{SubscribeID: "sub-existing"}, personal.EventMention, "at", nil
	}
	personalUpsertRunState = func(string, personal.RunState) error {
		t.Fatal("dry-run unexpectedly persisted run state")
		return nil
	}
	consumeCalls := 0
	personalConsumeRun = func(_ context.Context, cfg consume.Config) error {
		consumeCalls++
		if !cfg.DryRun {
			t.Fatal("consume config is not dry-run")
		}
		if strings.Contains(strings.Join(cfg.SpawnExtraArgs, " "), token) {
			t.Fatal("dry-run spawn args leaked runtime token")
		}
		return nil
	}
	personalBusRun = func(context.Context, bus.Config) error {
		t.Fatal("dry-run unexpectedly started a bus")
		return nil
	}

	err := runPersonalEventConsumeSingle(newPersonalCoverageCommand(), personalConsumeOptions{
		SubscribeID:      "sub-existing",
		ExplicitToken:    token,
		ClientIDOverride: "runtime-client",
		Common:           commonConsumeOptions{DryRun: true},
	})
	if err != nil {
		t.Fatalf("dry-run consume error = %v", err)
	}
	if consumeCalls != 1 {
		t.Fatalf("dry-run consume calls = %d, want 1", consumeCalls)
	}
}

func TestCrossPlatformCoverageRuntimeTokenControlRejectionReleasesSubscriptionClaim(t *testing.T) {
	store := &personalRecordingAttemptStore{}
	reservation := &personalSubscriptionAttemptReservation{
		store: store,
		claim: &personal.AttemptClaim{AttemptID: "runtime-token-attempt"},
		items: []personalSubscriptionAttemptItem{{eventKey: personal.EventMention, fingerprint: strings.Repeat("a", 64)}},
	}
	cause := &personal.APIError{
		Code:       "RUNTIME_TOKEN_REJECTED",
		Message:    "event runtime token was rejected; retry with a fresh host credential",
		HTTPStatus: http.StatusUnauthorized,
	}
	if !personalRuntimeTokenControlRejection(cause) {
		t.Fatal("runtime token control rejection was not classified")
	}
	err := reservation.releaseRuntimeTokenFailure()
	if err == nil || !strings.Contains(err.Error(), "runtime token was rejected") {
		t.Fatalf("releaseRuntimeTokenFailure() error = %v", err)
	}
	if store.releaseCalls != 1 || store.failureCalls != 0 {
		t.Fatalf("attempt store release=%d failure=%d, want release only", store.releaseCalls, store.failureCalls)
	}
}

type eventRuntimeRoundTripFunc func(*http.Request) (*http.Response, error)

func (f eventRuntimeRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
