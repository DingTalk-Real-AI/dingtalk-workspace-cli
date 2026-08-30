// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/busctl"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/consume"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	eventtransport "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

type personalRecordingAttemptStore struct {
	mu              sync.Mutex
	specs           []personal.AttemptSpec
	lease           time.Duration
	claimCalls      int
	completeCalls   int
	failureCalls    int
	releaseCalls    int
	claimErr        error
	completeErr     error
	completeFailure error
	failureHold     personal.AttemptHold
	lastFailure     personal.AttemptFailure
	lastSucceeded   []string
}

func (s *personalRecordingAttemptStore) Claim(
	specs []personal.AttemptSpec,
	lease time.Duration,
) (*personal.AttemptClaim, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claimCalls++
	s.specs = append([]personal.AttemptSpec(nil), specs...)
	s.lease = lease
	if s.claimErr != nil {
		return nil, s.claimErr
	}
	fingerprints := make([]string, 0, len(specs))
	for _, spec := range specs {
		fingerprints = append(fingerprints, spec.Fingerprint)
	}
	return &personal.AttemptClaim{
		AttemptID:    "recording-attempt",
		Fingerprints: fingerprints,
	}, nil
}

func (s *personalRecordingAttemptStore) CompleteSuccess(*personal.AttemptClaim) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completeCalls++
	return s.completeErr
}

func (s *personalRecordingAttemptStore) CompleteFailure(
	_ *personal.AttemptClaim,
	succeeded []string,
	failure personal.AttemptFailure,
) (personal.AttemptHold, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failureCalls++
	s.lastFailure = failure
	s.lastSucceeded = append([]string(nil), succeeded...)
	if s.completeFailure != nil {
		return personal.AttemptHold{}, s.completeFailure
	}
	hold := s.failureHold
	hold.Fingerprint = failure.Fingerprint
	hold.Retryability = failure.Retryability
	return hold, nil
}

func (s *personalRecordingAttemptStore) Release(*personal.AttemptClaim) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.releaseCalls++
	return nil
}

func TestCrossPlatformCoveragePersonalSubscriptionProtectionCoversAllPublicEvents(t *testing.T) {
	oldFactory := personalNewSubscriptionAttemptStore
	t.Cleanup(func() { personalNewSubscriptionAttemptStore = oldFactory })

	identity := personal.Identity{
		CorpID: "corp", UserID: "self", ClientID: "client", SourceID: "source",
	}
	client := personal.NewClient("https://mcp.example.test/dws", identity)
	publicCount := 0
	ruleTypes := make(map[string]bool)
	fingerprints := make(map[string]bool)

	for _, definition := range personal.Definitions() {
		if !definition.Public {
			continue
		}
		publicCount++
		ruleTypes[definition.RuleType] = true
		opts := personalConsumeOptions{EventKey: definition.EventKey}
		switch definition.RuleType {
		case "singleChat", "sender":
			opts.UserID = "target-user"
		case "group":
			opts.GroupID = "target-group"
		}

		recording := &personalRecordingAttemptStore{}
		personalNewSubscriptionAttemptStore = func(string) personalSubscriptionAttemptStore {
			return recording
		}
		reservation, err := reservePersonalSubscriptionAttempts(
			t.TempDir(),
			client,
			identity,
			"profile-a",
			[]personalConsumeOptions{opts},
		)
		if err != nil {
			t.Fatalf("%s reservation failed: %v", definition.EventKey, err)
		}
		if recording.claimCalls != 1 || len(recording.specs) != 1 {
			t.Fatalf("%s claim = %d, %#v", definition.EventKey, recording.claimCalls, recording.specs)
		}
		spec := recording.specs[0]
		if spec.EventKey != definition.EventKey || len(spec.Fingerprint) != 64 {
			t.Fatalf("%s spec = %#v", definition.EventKey, spec)
		}
		if fingerprints[spec.Fingerprint] {
			t.Fatalf("%s reused another event fingerprint", definition.EventKey)
		}
		fingerprints[spec.Fingerprint] = true
		if err := reservation.completeSuccess(); err != nil {
			t.Fatalf("%s completion failed: %v", definition.EventKey, err)
		}
	}

	if publicCount != 26 {
		t.Fatalf("public personal events = %d, want 26 (16 IM + 7 OA + 3 Todo)", publicCount)
	}
	for _, ruleType := range []string{"at", "all", "singleChat", "sender", "group"} {
		if !ruleTypes[ruleType] {
			t.Errorf("rule type %s was not protected", ruleType)
		}
	}
}

func TestCrossPlatformCoveragePersonalSubscriptionFingerprintChangesWithLogicalInputs(t *testing.T) {
	identity := personal.Identity{
		CorpID: "corp", UserID: "self", ClientID: "client", SourceID: "source",
	}
	fingerprint := func(endpoint, profile string, opts personalConsumeOptions) string {
		t.Helper()
		prepared, err := preparePersonalSubscription(identity, opts)
		if err != nil {
			t.Fatal(err)
		}
		return personal.Fingerprint(endpoint, prepared.Request.IdempotencyKey, profile)
	}
	baseOpts := personalConsumeOptions{
		EventKey: personal.EventSingleChat,
		UserID:   "target-a",
	}
	base := fingerprint("https://mcp.example.test/dws", "profile-a", baseOpts)
	candidates := map[string]string{
		"endpoint": fingerprint("https://other.example.test/dws", "profile-a", baseOpts),
		"profile":  fingerprint("https://mcp.example.test/dws", "profile-b", baseOpts),
		"event": fingerprint(
			"https://mcp.example.test/dws",
			"profile-a",
			personalConsumeOptions{EventKey: personal.EventFromUser, UserID: "target-a"},
		),
		"target": fingerprint(
			"https://mcp.example.test/dws",
			"profile-a",
			personalConsumeOptions{EventKey: personal.EventSingleChat, UserID: "target-b"},
		),
		"filter": fingerprint(
			"https://mcp.example.test/dws",
			"profile-a",
			personalConsumeOptions{
				EventKey: personal.EventSingleChat,
				UserID:   "target-a",
				QueryCSV: "urgent",
			},
		),
	}
	for dimension, candidate := range candidates {
		if candidate == base {
			t.Errorf("%s did not change subscription fingerprint", dimension)
		}
	}
}

func TestCrossPlatformCoveragePersonalSubscriptionRejectsMalformedEndpointBeforeClaim(t *testing.T) {
	oldFactory := personalNewSubscriptionAttemptStore
	t.Cleanup(func() { personalNewSubscriptionAttemptStore = oldFactory })
	factoryCalls := 0
	personalNewSubscriptionAttemptStore = func(string) personalSubscriptionAttemptStore {
		factoryCalls++
		return &personalRecordingAttemptStore{}
	}
	identity := personal.Identity{
		CorpID: "corp", UserID: "self", ClientID: "client", SourceID: "source",
	}
	client := personal.NewClient("://malformed", identity)
	_, err := reservePersonalSubscriptionAttempts(
		t.TempDir(),
		client,
		identity,
		"profile-a",
		[]personalConsumeOptions{{EventKey: personal.EventMention}},
	)
	if err == nil {
		t.Fatal("malformed subscription endpoint was accepted")
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation ||
		!typed.RetryableSet || typed.Retryable {
		t.Fatalf("malformed endpoint error = %#v, %v", typed, err)
	}
	if factoryCalls != 0 {
		t.Fatalf("attempt-store factory calls = %d, want 0", factoryCalls)
	}
}

type personalRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn personalRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestCrossPlatformCoveragePersonalSubscriptionProtectionConcurrentHundredAllowsOneHTTPRequest(t *testing.T) {
	oldFactory := personalNewSubscriptionAttemptStore
	t.Cleanup(func() { personalNewSubscriptionAttemptStore = oldFactory })
	personalNewSubscriptionAttemptStore = func(workDir string) personalSubscriptionAttemptStore {
		return personal.NewAttemptStore(workDir)
	}

	const concurrency = 100
	workDir := t.TempDir()
	identity := personal.Identity{
		AccessToken: "token",
		CorpID:      "corp",
		UserID:      "self",
		ClientID:    "client",
		SourceID:    "source",
	}
	opts := personalConsumeOptions{EventKey: personal.EventMention}
	var requests atomic.Int64
	requestStarted := make(chan struct{}, concurrency)
	releaseRequest := make(chan struct{})
	transport := personalRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/dws/subscription/user" {
			t.Errorf("request path = %q", request.URL.Path)
		}
		requests.Add(1)
		requestStarted <- struct{}{}
		<-releaseRequest
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"success":true,"result":["sub-one"]}`,
			)),
			Request: request,
		}, nil
	})

	start := make(chan struct{})
	results := make(chan error, concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			<-start
			client := personal.NewClient("https://mcp.example.test/dws", identity)
			client.HTTPClient = &http.Client{Transport: transport, Timeout: 30 * time.Second}
			reservation, err := reservePersonalSubscriptionAttempts(
				workDir,
				client,
				identity,
				"profile-a",
				[]personalConsumeOptions{opts},
			)
			if err != nil {
				results <- err
				return
			}
			_, _, _, err = ensurePersonalSubscription(
				context.Background(),
				client,
				identity,
				opts,
			)
			if err == nil {
				err = reservation.completeSuccess()
			}
			results <- err
		}()
	}
	close(start)

	select {
	case <-requestStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("no subscription HTTP request started")
	}

	for i := 0; i < concurrency-1; i++ {
		select {
		case err := <-results:
			var blocked *personal.AttemptBlockedError
			if !errors.As(err, &blocked) || blocked.State != personal.AttemptStateInFlight {
				t.Fatalf("concurrent loser error = %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("only %d concurrent losers completed", i)
		}
	}
	close(releaseRequest)
	select {
	case err := <-results:
		if err != nil {
			t.Fatalf("winning request failed: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("winning request did not complete")
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("subscription HTTP requests = %d, want 1", got)
	}
}

func TestCrossPlatformCoveragePersonalSubscriptionFailureClassification(t *testing.T) {
	retryable, nonRetryable := true, false
	now := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	networkErr := &url.Error{
		Op:  "Post",
		URL: "https://mcp.example.test",
		Err: &net.DNSError{IsTimeout: true},
	}
	tests := []struct {
		name          string
		err           error
		want          personal.Retryability
		wantReason    string
		wantAuth      bool
		wantRetryWait time.Duration
	}{
		{
			name: "explicit false",
			err: &personal.APIError{
				Code:      "GROUP_NOT_BELONG_TO_ORG",
				Retryable: &nonRetryable,
				Details:   map[string]any{"subscribe_id": "sub-existing"},
			},
			want:       personal.RetryabilityNonRetryable,
			wantReason: "personal_subscription_server_non_retryable",
		},
		{
			name: "explicit true",
			err: &personal.APIError{
				Code: "BUSY", Retryable: &retryable,
			},
			want:       personal.RetryabilityRetryable,
			wantReason: "personal_subscription_server_retryable",
		},
		{
			name: "terminal business code",
			err: &personal.APIError{
				Code:       "USER_NOT_FOUND",
				HTTPStatus: http.StatusOK,
				Details:    map[string]any{"subscribe_id": "sub-existing"},
			},
			want:       personal.RetryabilityNonRetryable,
			wantReason: "personal_subscription_business_rejected",
		},
		{
			name: "permission",
			err: &personal.APIError{
				Code: "PERMISSION_DENIED", HTTPStatus: http.StatusOK,
			},
			want:       personal.RetryabilityNonRetryable,
			wantReason: "personal_subscription_business_rejected",
			wantAuth:   true,
		},
		{
			name: "403 with existing id remains an auth rejection",
			err: &personal.APIError{
				Code:       "PROXY_AUTH_FAILURE",
				HTTPStatus: http.StatusForbidden,
				Details:    map[string]any{"subscribe_id": "sub-existing"},
			},
			want:       personal.RetryabilityNonRetryable,
			wantReason: "personal_subscription_auth",
			wantAuth:   true,
		},
		{
			name: "429",
			err: &personal.APIError{
				Code: "RATE_LIMIT", HTTPStatus: http.StatusTooManyRequests,
			},
			want:       personal.RetryabilityRetryable,
			wantReason: "personal_subscription_transient_http",
		},
		{
			name: "408",
			err: &personal.APIError{
				Code: "REQUEST_TIMEOUT", HTTPStatus: http.StatusRequestTimeout,
			},
			want:       personal.RetryabilityRetryable,
			wantReason: "personal_subscription_transient_http",
		},
		{
			name: "425",
			err: &personal.APIError{
				Code: "TOO_EARLY", HTTPStatus: http.StatusTooEarly,
			},
			want:       personal.RetryabilityRetryable,
			wantReason: "personal_subscription_transient_http",
		},
		{
			name: "429 overrides terminal-looking business marker",
			err: &personal.APIError{
				Code: "EVENT_KEY_NOT_SUPPORTED", HTTPStatus: http.StatusTooManyRequests,
			},
			want:       personal.RetryabilityRetryable,
			wantReason: "personal_subscription_transient_http",
		},
		{
			name: "5xx",
			err: &personal.APIError{
				Code: "SYSTEM_ERROR", HTTPStatus: http.StatusBadGateway,
			},
			want:       personal.RetryabilityRetryable,
			wantReason: "personal_subscription_transient_http",
		},
		{
			name: "http 200 system error remains unknown",
			err: &personal.APIError{
				Code: "SYSTEM_ERROR", HTTPStatus: http.StatusOK,
			},
			want:       personal.RetryabilityUnknown,
			wantReason: "personal_subscription_unknown",
		},
		{
			name: "unknown required code remains unknown",
			err: &personal.APIError{
				Code: "RETRY_REQUIRED", HTTPStatus: http.StatusOK,
			},
			want:       personal.RetryabilityUnknown,
			wantReason: "personal_subscription_unknown",
		},
		{
			name: "legacy DUP with existing id remains an unknown error",
			err: &personal.APIError{
				Code:       "DUP",
				HTTPStatus: http.StatusBadRequest,
				Details:    map[string]any{"subscribe_id": "sub-existing"},
			},
			want:       personal.RetryabilityUnknown,
			wantReason: "personal_subscription_unverified_existing_id",
		},
		{
			name: "legacy SUBSCRIPTION_ALREADY_EXIST with existing id remains an unknown error",
			err: &personal.APIError{
				Code:       "SUBSCRIPTION_ALREADY_EXIST",
				HTTPStatus: http.StatusBadRequest,
				Details:    map[string]any{"subscribe_id": "sub-existing"},
			},
			want:       personal.RetryabilityUnknown,
			wantReason: "personal_subscription_unverified_existing_id",
		},
		{
			name: "legacy ALREADY_SUBSCRIBED with existing id remains an unknown error",
			err: &personal.APIError{
				Code:       "ALREADY_SUBSCRIBED",
				HTTPStatus: http.StatusBadRequest,
				Details:    map[string]any{"subscribe_id": "sub-existing"},
			},
			want:       personal.RetryabilityUnknown,
			wantReason: "personal_subscription_unverified_existing_id",
		},
		{
			name: "legacy DUPLICATE with existing id remains an unknown error",
			err: &personal.APIError{
				Code:       "DUPLICATE",
				HTTPStatus: http.StatusBadRequest,
				Details:    map[string]any{"subscribe_id": "sub-existing"},
			},
			want:       personal.RetryabilityUnknown,
			wantReason: "personal_subscription_unverified_existing_id",
		},
		{
			name:       "network",
			err:        networkErr,
			want:       personal.RetryabilityRetryable,
			wantReason: "personal_subscription_network",
		},
		{
			name: "malformed url is local validation",
			err: &url.Error{
				Op: "parse", URL: "://malformed", Err: errors.New("missing protocol scheme"),
			},
			want:       personal.RetryabilityNonRetryable,
			wantReason: "personal_subscription_invalid",
		},
		{
			name:       "truncated response body",
			err:        fmt.Errorf("read response: %w", io.ErrUnexpectedEOF),
			want:       personal.RetryabilityRetryable,
			wantReason: "personal_subscription_network",
		},
		{
			name:       "deadline",
			err:        context.DeadlineExceeded,
			want:       personal.RetryabilityRetryable,
			wantReason: "personal_subscription_timeout",
		},
		{
			name: "longest retry after wins",
			err: &personal.APIError{
				Code:              "BUSY",
				RetryAfterSeconds: int64Pointer(10),
				NextRetryAt:       timePointer(now.Add(45 * time.Second)),
				Details:           map[string]any{"retry_after": "60"},
			},
			want:          personal.RetryabilityUnknown,
			wantReason:    "personal_subscription_unknown",
			wantRetryWait: time.Minute,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := classifyPersonalSubscriptionFailure(test.err, now)
			if got.retryability != test.want || got.reason != test.wantReason ||
				got.auth != test.wantAuth || got.retryAfter != test.wantRetryWait {
				t.Fatalf("classification = %#v", got)
			}
		})
	}
}

func TestCrossPlatformCoveragePersonalSubscriptionFailureErrorPreservesTriStateAndDiagnostics(t *testing.T) {
	now := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	hold := personal.AttemptHold{
		State:         personal.AttemptStateTerminalHold,
		Retryability:  personal.RetryabilityNonRetryable,
		RetryAfter:    time.Hour,
		NextAllowedAt: now.Add(time.Hour),
	}
	cause := &personal.APIError{
		Code:    "GROUP_NOT_BELONG_TO_ORG",
		Message: "group does not belong to organization",
		TraceID: "trace-1",
	}
	classification := personalSubscriptionFailureClass{
		retryability: personal.RetryabilityNonRetryable,
		code:         cause.Code,
		traceID:      cause.TraceID,
		reason:       "personal_subscription_business_rejected",
	}
	err := personalSubscriptionFailureError(cause, classification, hold)
	var typed *apperrors.Error
	if !errors.As(err, &typed) {
		t.Fatalf("error type = %T", err)
	}
	if !typed.RetryableSet || typed.Retryable ||
		typed.RetryAfterSeconds == nil || *typed.RetryAfterSeconds != 3600 ||
		typed.NextRetryAt == nil || !typed.NextRetryAt.Equal(now.Add(time.Hour)) ||
		typed.ServerDiag.ServerErrorCode != cause.Code ||
		typed.ServerDiag.TraceID != cause.TraceID {
		t.Fatalf("typed error = %#v", typed)
	}

	unknown := personalSubscriptionFailureError(
		errors.New("unknown"),
		personalSubscriptionFailureClass{
			retryability: personal.RetryabilityUnknown,
			reason:       "personal_subscription_unknown",
		},
		personal.AttemptHold{
			Retryability:  personal.RetryabilityUnknown,
			RetryAfter:    30 * time.Second,
			NextAllowedAt: now.Add(30 * time.Second),
		},
	)
	if !errors.As(unknown, &typed) || typed.RetryableSet {
		t.Fatalf("unknown retryability was not preserved: %#v", typed)
	}
}

func TestCrossPlatformCoveragePersonalSubscriptionBatchClaimFailureMakesZeroCreateCalls(t *testing.T) {
	restore := installPersonalManySeams(t)
	defer restore()
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())

	identity := personal.Identity{
		CorpID: "corp", UserID: "self", ClientID: "client", SourceID: "source",
	}
	personalResolveEventIdentity = func(context.Context, string, string) (personal.Identity, error) {
		return identity, nil
	}
	personalValidateConsumeConfig = func(consume.Config) error { return nil }
	createCalls := 0
	personalEnsureSubscription = func(
		context.Context,
		*personal.Client,
		personal.Identity,
		personalConsumeOptions,
	) (*personal.Subscription, string, string, error) {
		createCalls++
		return nil, "", "", errors.New("must not run")
	}

	next := time.Now().UTC().Add(time.Minute)
	recording := &personalRecordingAttemptStore{
		claimErr: &personal.AttemptBlockedError{
			State:         personal.AttemptStateCooldown,
			Retryability:  personal.RetryabilityUnknown,
			RetryAfter:    time.Minute,
			NextAllowedAt: next,
		},
	}
	personalNewSubscriptionAttemptStore = func(string) personalSubscriptionAttemptStore {
		return recording
	}

	err := runPersonalEventConsumeMany(newPersonalCoverageCommand(), personalConsumeOptions{
		EventKeys: []string{personal.EventMention, personal.EventAllSingleChat},
	})
	if err == nil {
		t.Fatal("blocked batch succeeded")
	}
	if createCalls != 0 {
		t.Fatalf("create calls = %d, want 0", createCalls)
	}
	if recording.claimCalls != 1 || len(recording.specs) != 2 {
		t.Fatalf("batch claim = %d, %#v", recording.claimCalls, recording.specs)
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.RetryableSet ||
		typed.NextRetryAt == nil || !typed.NextRetryAt.Equal(next) {
		t.Fatalf("blocked error = %#v, %v", typed, err)
	}
}

func TestCrossPlatformCoveragePersonalSubscriptionAttemptGuardBypassesNonCreatePaths(t *testing.T) {
	oldFactory := personalNewSubscriptionAttemptStore
	oldIdentity := personalResolveEventIdentity
	oldEnsure := personalEnsureSubscription
	oldGet := personalGetSubscription
	oldList := personalListSubscriptions
	oldUpsert := personalUpsertRunState
	oldDelete := personalDeleteSubscription
	oldRemove := personalRemoveRunStates
	oldLoad := personalLoadRunStates
	oldConsume := personalConsumeRun
	oldRunMany := personalConsumeRunMany
	oldValidate := personalValidateConsumeConfig
	oldConflict := personalValidateNoOutputConflict
	oldStopConsumers := personalStopConsumers
	oldStopBus := personalStopBus
	t.Cleanup(func() {
		personalNewSubscriptionAttemptStore = oldFactory
		personalResolveEventIdentity = oldIdentity
		personalEnsureSubscription = oldEnsure
		personalGetSubscription = oldGet
		personalListSubscriptions = oldList
		personalUpsertRunState = oldUpsert
		personalDeleteSubscription = oldDelete
		personalRemoveRunStates = oldRemove
		personalLoadRunStates = oldLoad
		personalConsumeRun = oldConsume
		personalConsumeRunMany = oldRunMany
		personalValidateConsumeConfig = oldValidate
		personalValidateNoOutputConflict = oldConflict
		personalStopConsumers = oldStopConsumers
		personalStopBus = oldStopBus
	})
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())

	recording := &personalRecordingAttemptStore{}
	factoryCalls := 0
	personalNewSubscriptionAttemptStore = func(string) personalSubscriptionAttemptStore {
		factoryCalls++
		return recording
	}
	identity := personal.Identity{
		CorpID: "corp", UserID: "self", ClientID: "client", SourceID: "source",
	}
	personalResolveEventIdentity = func(context.Context, string, string) (personal.Identity, error) {
		return identity, nil
	}
	personalEnsureSubscription = ensurePersonalSubscription
	personalGetSubscription = func(
		*personal.Client,
		context.Context,
		string,
	) (*personal.Subscription, error) {
		return &personal.Subscription{
			SubscribeID: "sub-existing",
			EventKey:    personal.EventMention,
			RuleType:    "at",
			Status:      "active",
		}, nil
	}
	personalListSubscriptions = func(
		*personal.Client,
		context.Context,
		personal.ListOptions,
	) ([]personal.Subscription, error) {
		return []personal.Subscription{{
			SubscribeID: "sub-existing",
			EventKey:    personal.EventMention,
			RuleType:    "at",
			Status:      "active",
		}}, nil
	}
	personalUpsertRunState = func(string, personal.RunState) error { return nil }
	personalDeleteSubscription = func(*personal.Client, context.Context, string) error { return nil }
	personalRemoveRunStates = func(string, []string) error { return nil }
	personalLoadRunStates = func(string) ([]personal.RunState, error) { return nil, nil }
	personalConsumeRun = func(context.Context, consume.Config) error { return nil }
	personalConsumeRunMany = func(context.Context, consume.Config, []consume.ConsumerSpec) error {
		return nil
	}
	personalValidateConsumeConfig = func(consume.Config) error { return nil }
	personalValidateNoOutputConflict = func(consume.Config, string) error { return nil }
	personalStopConsumers = func(string, []string) (eventtransport.ConsumerStopResp, error) {
		return eventtransport.ConsumerStopResp{Stopped: []string{"sub-existing"}}, nil
	}
	personalStopBus = func(busctl.StopConfig) error { return busctl.ErrNotRunning }

	assertNoAttemptClaim := func(stage string) {
		t.Helper()
		recording.mu.Lock()
		claimCalls := recording.claimCalls
		recording.mu.Unlock()
		if factoryCalls != 0 || claimCalls != 0 {
			t.Fatalf(
				"%s used subscription attempt guard: factory calls=%d claim calls=%d",
				stage,
				factoryCalls,
				claimCalls,
			)
		}
	}

	if err := runPersonalEventConsumeSingle(
		newPersonalCoverageCommand(),
		personalConsumeOptions{
			EventKey: personal.EventMention,
			Common:   commonConsumeOptions{DryRun: true},
		},
	); err != nil {
		t.Fatalf("single dry-run error = %v", err)
	}
	assertNoAttemptClaim("single dry-run")

	if err := runPersonalEventConsumeMany(
		newPersonalCoverageCommand(),
		personalConsumeOptions{
			EventKeys: []string{personal.EventMention, personal.EventAllSingleChat},
			Common:    commonConsumeOptions{DryRun: true},
		},
	); err != nil {
		t.Fatalf("multi dry-run error = %v", err)
	}
	assertNoAttemptClaim("multi dry-run")

	if err := runPersonalEventConsumeSingle(
		newPersonalCoverageCommand(),
		personalConsumeOptions{SubscribeID: "sub-existing"},
	); err != nil {
		t.Fatalf("subscribe-id reuse error = %v", err)
	}
	assertNoAttemptClaim("subscribe-id reuse")

	if err := runPersonalEventStatus(
		newPersonalCoverageCommand(),
		personalStatusOptions{Status: "all", Format: "json"},
	); err != nil {
		t.Fatalf("status error = %v", err)
	}
	assertNoAttemptClaim("status")

	if err := runPersonalEventStop(
		newPersonalCoverageCommand(),
		personalStopOptions{SubscribeID: "sub-existing"},
	); err != nil {
		t.Fatalf("stop error = %v", err)
	}
	assertNoAttemptClaim("stop")
}

func TestCrossPlatformCoveragePersonalSubscriptionLocalValidationRunsBeforeClaimAndCreate(t *testing.T) {
	restore := installPersonalManySeams(t)
	defer restore()
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())

	identity := personal.Identity{
		CorpID: "corp", UserID: "self", ClientID: "client", SourceID: "source",
	}
	personalResolveEventIdentity = func(context.Context, string, string) (personal.Identity, error) {
		return identity, nil
	}
	wantErr := errors.New("invalid local output")
	personalValidateConsumeConfig = func(consume.Config) error { return wantErr }
	recording := &personalRecordingAttemptStore{}
	personalNewSubscriptionAttemptStore = func(string) personalSubscriptionAttemptStore {
		return recording
	}
	createCalls := 0
	personalEnsureSubscription = func(
		context.Context,
		*personal.Client,
		personal.Identity,
		personalConsumeOptions,
	) (*personal.Subscription, string, string, error) {
		createCalls++
		return nil, "", "", errors.New("must not run")
	}

	err := runPersonalEventConsumeSingle(
		newPersonalCoverageCommand(),
		personalConsumeOptions{EventKey: personal.EventMention},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if recording.claimCalls != 0 || createCalls != 0 {
		t.Fatalf("claim calls = %d, create calls = %d", recording.claimCalls, createCalls)
	}

	err = runPersonalEventConsumeSingle(
		newPersonalCoverageCommand(),
		personalConsumeOptions{
			EventKey:       personal.EventMention,
			Flatten:        true,
			DebugRawEvents: true,
		},
	)
	if err == nil {
		t.Fatal("conflicting output modes succeeded")
	}
	if recording.claimCalls != 0 || createCalls != 0 {
		t.Fatalf("output validation reached claim/create: %d/%d", recording.claimCalls, createCalls)
	}

	err = runPersonalEventConsumeSingle(
		newPersonalCoverageCommand(),
		personalConsumeOptions{
			EventKey: personal.EventInChat,
			Common:   commonConsumeOptions{DryRun: true},
		},
	)
	if err == nil {
		t.Fatal("invalid dry-run subscription options succeeded")
	}
	if recording.claimCalls != 0 || createCalls != 0 {
		t.Fatalf("dry-run validation reached claim/create: %d/%d", recording.claimCalls, createCalls)
	}

	claimErr := errors.New("claim failed")
	recording.claimErr = claimErr
	personalValidateConsumeConfig = func(consume.Config) error { return nil }
	err = runPersonalEventConsumeSingle(
		newPersonalCoverageCommand(),
		personalConsumeOptions{EventKey: personal.EventMention},
	)
	if !errors.Is(err, claimErr) {
		t.Fatalf("claim error = %v, want %v", err, claimErr)
	}
	if recording.claimCalls != 1 || createCalls != 0 {
		t.Fatalf("failed claim calls = %d, create calls = %d", recording.claimCalls, createCalls)
	}
}

func TestCrossPlatformCoveragePersonalSubscriptionSingleAttemptCompletionErrors(t *testing.T) {
	tests := []struct {
		name           string
		cancelContext  bool
		nilSub         bool
		upsertErr      error
		completeErr    error
		wantRelease    int
		wantFailure    int
		wantComplete   int
		wantDelete     int
		wantCanceledGC bool
		wantUnknown    bool
	}{
		{
			name:        "nil subscription records failure",
			nilSub:      true,
			wantFailure: 1,
		},
		{
			name:         "completion failure cleans subscription",
			completeErr:  errors.New("complete success failed"),
			wantComplete: 1,
			wantDelete:   1,
		},
		{
			name:        "local state failure records unknown cooldown",
			upsertErr:   errors.New("save state failed"),
			wantFailure: 1,
			wantDelete:  1,
			wantUnknown: true,
		},
		{
			name:           "canceled local failure releases and uses canceled cleanup context",
			cancelContext:  true,
			upsertErr:      errors.New("save state failed"),
			wantRelease:    1,
			wantDelete:     1,
			wantCanceledGC: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			restore := installPersonalManySeams(t)
			defer restore()
			t.Setenv("DWS_CONFIG_DIR", t.TempDir())

			identity := personal.Identity{
				AccessToken:  "token",
				ClientID:     "client",
				SourceID:     "open",
				LocalSubject: "subject",
			}
			personalResolveEventIdentity = func(context.Context, string, string) (personal.Identity, error) {
				return identity, nil
			}
			recording := &personalRecordingAttemptStore{completeErr: test.completeErr}
			personalNewSubscriptionAttemptStore = func(string) personalSubscriptionAttemptStore {
				return recording
			}
			personalEnsureSubscription = func(
				context.Context,
				*personal.Client,
				personal.Identity,
				personalConsumeOptions,
			) (*personal.Subscription, string, string, error) {
				if test.nilSub {
					return nil, "", "", nil
				}
				return &personal.Subscription{
					SubscribeID: "sub-one",
				}, personal.EventMention, "at", nil
			}
			personalValidateConsumeConfig = func(consume.Config) error { return nil }
			personalUpsertRunState = func(string, personal.RunState) error {
				return test.upsertErr
			}
			deleteCalls := 0
			personalDeleteSubscription = func(
				_ *personal.Client,
				cleanupCtx context.Context,
				_ string,
			) error {
				deleteCalls++
				if got := cleanupCtx.Err() != nil; got != test.wantCanceledGC {
					t.Errorf("cleanup canceled = %t, want %t", got, test.wantCanceledGC)
				}
				return nil
			}
			personalRemoveRunStates = func(string, []string) error { return nil }

			cmd := newPersonalCoverageCommand()
			if test.cancelContext {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				cmd.SetContext(ctx)
			}
			err := runPersonalEventConsumeSingle(cmd, personalConsumeOptions{
				EventKey:       personal.EventMention,
				ControlBaseURL: "https://mcp.example.test/dws",
			})
			if err == nil {
				t.Fatal("single subscription error path succeeded")
			}
			if recording.releaseCalls != test.wantRelease ||
				recording.failureCalls != test.wantFailure ||
				recording.completeCalls != test.wantComplete ||
				deleteCalls != test.wantDelete {
				t.Fatalf(
					"release=%d failure=%d complete=%d delete=%d",
					recording.releaseCalls,
					recording.failureCalls,
					recording.completeCalls,
					deleteCalls,
				)
			}
			if test.wantUnknown {
				if recording.lastFailure.Retryability != personal.RetryabilityUnknown {
					t.Fatalf("local failure retryability = %q", recording.lastFailure.Retryability)
				}
				var typed *apperrors.Error
				if !errors.As(err, &typed) || typed.RetryableSet {
					t.Fatalf("local failure error = %#v, %v", typed, err)
				}
			}
		})
	}
}

func TestCrossPlatformCoveragePersonalSubscriptionManyCompletionFailureCleansBatch(t *testing.T) {
	restore := installPersonalManySeams(t)
	defer restore()
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())

	identity := personal.Identity{
		AccessToken:  "token",
		ClientID:     "client",
		SourceID:     "open",
		LocalSubject: "subject",
	}
	personalResolveEventIdentity = func(context.Context, string, string) (personal.Identity, error) {
		return identity, nil
	}
	recording := &personalRecordingAttemptStore{
		completeErr: errors.New("complete success failed"),
	}
	personalNewSubscriptionAttemptStore = func(string) personalSubscriptionAttemptStore {
		return recording
	}
	createCalls := 0
	personalEnsureSubscription = func(
		_ context.Context,
		_ *personal.Client,
		_ personal.Identity,
		opts personalConsumeOptions,
	) (*personal.Subscription, string, string, error) {
		createCalls++
		return &personal.Subscription{
			SubscribeID: fmt.Sprintf("sub-%d", createCalls),
		}, opts.EventKey, "all", nil
	}
	personalValidateConsumeConfig = func(consume.Config) error { return nil }
	personalUpsertRunState = func(string, personal.RunState) error { return nil }
	deleteCalls := 0
	personalDeleteSubscription = func(*personal.Client, context.Context, string) error {
		deleteCalls++
		return nil
	}
	personalRemoveRunStates = func(string, []string) error { return nil }
	personalConsumeRunMany = func(context.Context, consume.Config, []consume.ConsumerSpec) error {
		t.Fatal("consumer ran after attempt completion failure")
		return nil
	}

	err := runPersonalEventConsumeMany(newPersonalCoverageCommand(), personalConsumeOptions{
		EventKeys:      []string{personal.EventMention, personal.EventAllSingleChat},
		ControlBaseURL: "https://mcp.example.test/dws",
	})
	if err == nil {
		t.Fatal("multi subscription completion failure succeeded")
	}
	if recording.completeCalls != 1 || createCalls != 2 || deleteCalls != 2 {
		t.Fatalf(
			"complete=%d create=%d delete=%d",
			recording.completeCalls,
			createCalls,
			deleteCalls,
		)
	}
}

func TestCrossPlatformCoveragePersonalSubscriptionRejectsNonPublicEventsOnChangedPaths(t *testing.T) {
	const nonPublicEvent = personal.EventMention
	oldLookup := personalLookupDefinition
	oldGet := personalGetSubscription
	t.Cleanup(func() {
		personalLookupDefinition = oldLookup
		personalGetSubscription = oldGet
	})
	personalLookupDefinition = func(eventKey string) (personal.Definition, bool) {
		definition, ok := personal.Lookup(eventKey)
		if eventKey == nonPublicEvent {
			definition.Public = false
		}
		return definition, ok
	}

	err := runPersonalEventConsumeSingle(
		newPersonalCoverageCommand(),
		personalConsumeOptions{EventKey: nonPublicEvent},
	)
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation ||
		!typed.RetryableSet || typed.Retryable {
		t.Fatalf("single validation error = %#v, %v", typed, err)
	}

	if _, err := preparePersonalSubscription(
		personal.Identity{},
		personalConsumeOptions{EventKey: nonPublicEvent},
	); err == nil {
		t.Fatal("prepared a non-public personal event")
	}

	personalGetSubscription = func(
		*personal.Client,
		context.Context,
		string,
	) (*personal.Subscription, error) {
		return &personal.Subscription{
			SubscribeID: "sub-existing",
			EventKey:    nonPublicEvent,
		}, nil
	}
	if _, _, _, err := ensurePersonalSubscription(
		context.Background(),
		nil,
		personal.Identity{},
		personalConsumeOptions{SubscribeID: "sub-existing"},
	); err == nil {
		t.Fatal("reused a subscription for a non-public personal event")
	}
}

func TestCrossPlatformCoveragePersonalSubscriptionAttemptLeaseBounds(t *testing.T) {
	client := personal.NewClient("https://mcp.example.test/dws", personal.Identity{})
	tests := []struct {
		timeout time.Duration
		batch   int
		want    time.Duration
	}{
		{timeout: 10 * time.Second, batch: 1, want: time.Minute},
		{timeout: 10 * time.Second, batch: 0, want: time.Minute},
		{timeout: 30 * time.Second, batch: 2, want: 90 * time.Second},
		{timeout: 30 * time.Second, batch: 16, want: 510 * time.Second},
		{timeout: time.Hour, batch: 16, want: 10 * time.Minute},
	}
	for _, test := range tests {
		client.HTTPClient.Timeout = test.timeout
		if got := personalSubscriptionAttemptLease(client, test.batch); got != test.want {
			t.Errorf("lease(%s, %d) = %s, want %s", test.timeout, test.batch, got, test.want)
		}
	}
}

func TestCrossPlatformCoveragePersonalSubscriptionAttemptGuardHelperEdges(t *testing.T) {
	oldFactory := personalNewSubscriptionAttemptStore
	t.Cleanup(func() { personalNewSubscriptionAttemptStore = oldFactory })

	t.Run("default store factory", func(t *testing.T) {
		if store := oldFactory(t.TempDir()); store == nil {
			t.Fatal("default attempt-store factory returned nil")
		}
	})

	identity := personal.Identity{
		CorpID: "corp", UserID: "self", ClientID: "client", SourceID: "source",
	}
	validClient := personal.NewClient("https://mcp.example.test/dws", identity)
	validPlan := personalConsumeOptions{EventKey: personal.EventMention}

	t.Run("empty batch", func(t *testing.T) {
		if _, err := reservePersonalSubscriptionAttempts(
			t.TempDir(),
			validClient,
			identity,
			"profile-a",
			nil,
		); err == nil {
			t.Fatal("empty attempt batch was accepted")
		}
	})

	t.Run("nil client", func(t *testing.T) {
		if _, err := reservePersonalSubscriptionAttempts(
			t.TempDir(),
			nil,
			identity,
			"profile-a",
			[]personalConsumeOptions{validPlan},
		); err == nil {
			t.Fatal("nil subscription client was accepted")
		}
	})

	t.Run("relative endpoint", func(t *testing.T) {
		client := personal.NewClient("relative/control", identity)
		if _, err := reservePersonalSubscriptionAttempts(
			t.TempDir(),
			client,
			identity,
			"profile-a",
			[]personalConsumeOptions{validPlan},
		); err == nil {
			t.Fatal("relative subscription endpoint was accepted")
		}
	})

	t.Run("nil store", func(t *testing.T) {
		personalNewSubscriptionAttemptStore = func(string) personalSubscriptionAttemptStore {
			return nil
		}
		defer func() { personalNewSubscriptionAttemptStore = oldFactory }()
		if _, err := reservePersonalSubscriptionAttempts(
			t.TempDir(),
			validClient,
			identity,
			"profile-a",
			[]personalConsumeOptions{validPlan},
		); err == nil {
			t.Fatal("nil subscription attempt store was accepted")
		}
	})

	t.Run("claim failure", func(t *testing.T) {
		wantErr := errors.New("claim failed")
		personalNewSubscriptionAttemptStore = func(string) personalSubscriptionAttemptStore {
			return &personalRecordingAttemptStore{claimErr: wantErr}
		}
		defer func() { personalNewSubscriptionAttemptStore = oldFactory }()
		_, err := reservePersonalSubscriptionAttempts(
			t.TempDir(),
			validClient,
			identity,
			"profile-a",
			[]personalConsumeOptions{validPlan},
		)
		if !errors.Is(err, wantErr) {
			t.Fatalf("claim error = %v, want %v", err, wantErr)
		}
	})

	t.Run("invalid prepared subscription", func(t *testing.T) {
		if _, err := reservePersonalSubscriptionAttempts(
			t.TempDir(),
			validClient,
			identity,
			"profile-a",
			[]personalConsumeOptions{{EventKey: personal.EventInChat}},
		); err == nil {
			t.Fatal("invalid prepared subscription was accepted")
		}
	})

	t.Run("incomplete success reservation", func(t *testing.T) {
		if err := (&personalSubscriptionAttemptReservation{}).completeSuccess(); err == nil {
			t.Fatal("incomplete reservation succeeded")
		}
	})

	t.Run("success completion failure", func(t *testing.T) {
		wantErr := errors.New("complete success failed")
		reservation := &personalSubscriptionAttemptReservation{
			store: &personalRecordingAttemptStore{completeErr: wantErr},
			claim: &personal.AttemptClaim{AttemptID: "attempt"},
		}
		if err := reservation.completeSuccess(); !errors.Is(err, wantErr) {
			t.Fatalf("complete-success error = %v, want %v", err, wantErr)
		}
	})

	t.Run("nil failure reservation", func(t *testing.T) {
		wantErr := errors.New("create failed")
		var reservation *personalSubscriptionAttemptReservation
		if err := reservation.completeFailure(
			context.Background(),
			0,
			0,
			wantErr,
			nil,
		); !errors.Is(err, wantErr) {
			t.Fatalf("nil reservation error = %v, want %v", err, wantErr)
		}
	})

	t.Run("invalid failure indexes", func(t *testing.T) {
		wantErr := errors.New("create failed")
		reservation := &personalSubscriptionAttemptReservation{
			store: &personalRecordingAttemptStore{},
			claim: &personal.AttemptClaim{AttemptID: "attempt"},
			items: []personalSubscriptionAttemptItem{{fingerprint: "fingerprint"}},
		}
		if err := reservation.completeFailure(
			context.Background(),
			1,
			0,
			wantErr,
			nil,
		); !errors.Is(err, wantErr) {
			t.Fatalf("invalid-index error = %v, want joined cause %v", err, wantErr)
		}
	})

	t.Run("incomplete cancellation reservation", func(t *testing.T) {
		wantErr := context.Canceled
		err := (&personalSubscriptionAttemptReservation{}).completeFailure(
			context.Background(),
			0,
			0,
			wantErr,
			nil,
		)
		if !errors.Is(err, wantErr) {
			t.Fatalf("incomplete-cancellation error = %v, want joined cause %v", err, wantErr)
		}
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Reason != "personal_subscription_guard_failed" {
			t.Fatalf("incomplete-cancellation guard error = %#v, %v", typed, err)
		}
	})

	t.Run("failure completion failure", func(t *testing.T) {
		createErr := errors.New("create failed")
		completeErr := errors.New("complete failure failed")
		reservation := &personalSubscriptionAttemptReservation{
			store: &personalRecordingAttemptStore{completeFailure: completeErr},
			claim: &personal.AttemptClaim{AttemptID: "attempt"},
			items: []personalSubscriptionAttemptItem{{
				eventKey:    personal.EventMention,
				fingerprint: strings.Repeat("a", 64),
			}},
		}
		err := reservation.completeFailure(
			context.Background(),
			0,
			0,
			createErr,
			nil,
		)
		if !errors.Is(err, createErr) || !errors.Is(err, completeErr) {
			t.Fatalf("completion error = %v, want joined errors", err)
		}
	})
}

func TestCrossPlatformCoveragePersonalSubscriptionFailureClassificationEdges(t *testing.T) {
	now := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		err        error
		want       personal.Retryability
		wantReason string
		wantAuth   bool
	}{
		{
			name: "plain rejected http status",
			err: &personal.APIError{
				Code: "BUSINESS_REJECTED", HTTPStatus: http.StatusBadRequest,
			},
			want:       personal.RetryabilityNonRetryable,
			wantReason: "personal_subscription_http_rejected",
		},
		{
			name:       "direct network error",
			err:        &net.DNSError{Err: "temporary resolver failure", Name: "mcp.example.test"},
			want:       personal.RetryabilityRetryable,
			wantReason: "personal_subscription_network",
		},
		{
			name:       "token text",
			err:        errors.New("access token is missing"),
			want:       personal.RetryabilityNonRetryable,
			wantReason: "personal_subscription_auth",
			wantAuth:   true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := classifyPersonalSubscriptionFailure(test.err, now)
			if got.retryability != test.want || got.reason != test.wantReason ||
				got.auth != test.wantAuth {
				t.Fatalf("classification = %#v", got)
			}
		})
	}

	if delay := personalAPIRetryDelay(nil, now); delay != 0 {
		t.Fatalf("nil API retry delay = %s", delay)
	}
	if personalSubscriptionErrorHasSubscribeID(nil) {
		t.Fatal("nil API error reported a subscription ID")
	}
	if personalSubscriptionErrorHasSubscribeID(&personal.APIError{
		Details: map[string]any{"subscribe_id": 42},
	}) {
		t.Fatal("non-string subscription ID was accepted")
	}
	httpDate := now.Add(75 * time.Second).Format(http.TimeFormat)
	if delay := personalAPIRetryDelay(&personal.APIError{
		Details: map[string]any{"retry_after": httpDate},
	}, now); delay != 75*time.Second {
		t.Fatalf("HTTP-date retry delay = %s, want 75s", delay)
	}
	if delay := personalRetrySeconds(0); delay != 0 {
		t.Fatalf("zero retry seconds = %s", delay)
	}
	overflowSeconds := int64(math.MaxInt64/int64(time.Second)) + 1
	if delay := personalRetrySeconds(overflowSeconds); delay != time.Duration(math.MaxInt64) {
		t.Fatalf("overflow retry delay = %s", delay)
	}
	if delay := maxPersonalRetryDelay(time.Minute, time.Second); delay != time.Minute {
		t.Fatalf("max retry delay = %s, want 1m", delay)
	}
	if !personalSubscriptionAuthFailure(http.StatusUnauthorized, "") {
		t.Fatal("401 was not classified as an authentication failure")
	}
}

func TestCrossPlatformCoveragePersonalSubscriptionErrorConstructionEdges(t *testing.T) {
	nonRetryable := personalSubscriptionFailureClass{
		retryability: personal.RetryabilityNonRetryable,
		reason:       "personal_subscription_auth",
		auth:         true,
	}
	var typed *apperrors.Error
	err := personalSubscriptionFailureError(
		errors.New("authentication failed"),
		nonRetryable,
		personal.AttemptHold{},
	)
	if !errors.As(err, &typed) || typed.Category != apperrors.CategoryAuth {
		t.Fatalf("authentication failure error = %#v, %v", typed, err)
	}

	if err := personalSubscriptionBlockedError(nil); err == nil {
		t.Fatal("nil blocked error was accepted")
	}
	for _, test := range []struct {
		name     string
		code     string
		category apperrors.Category
	}{
		{name: "blocked auth", code: "NO_PERMISSION", category: apperrors.CategoryAuth},
		{name: "blocked validation", code: "INVALID_PARAM", category: apperrors.CategoryValidation},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := personalSubscriptionBlockedError(&personal.AttemptBlockedError{
				State:        personal.AttemptStateTerminalHold,
				Retryability: personal.RetryabilityNonRetryable,
				ErrorCode:    test.code,
			})
			typed = nil
			if !errors.As(err, &typed) || typed.Category != test.category {
				t.Fatalf("blocked error = %#v, %v", typed, err)
			}
		})
	}

	if seconds := ceilPersonalRetrySeconds(0); seconds != 0 {
		t.Fatalf("ceil zero retry seconds = %d", seconds)
	}
	if err := personalSubscriptionGuardError(nil); err == nil {
		t.Fatal("nil guard cause did not produce an error")
	}
	if err := personalSubscriptionValidationError(nil); err == nil {
		t.Fatal("nil validation cause did not produce an error")
	}
}

func TestCrossPlatformCoverageNewPersonalEventControlClientSetsVersionHeaders(t *testing.T) {
	oldVersion := version
	t.Cleanup(func() { version = oldVersion })
	version = "1.2.3-test"
	client := newPersonalEventControlClient(
		t.TempDir(),
		"https://mcp.example.test/dws",
		personal.Identity{ClientID: "client", SourceID: "source"},
	)
	if client.ClientVersion != "1.2.3-test" {
		t.Fatalf("ClientVersion = %q", client.ClientVersion)
	}
	if client.UserAgent != "dws-cli/1.2.3-test" {
		t.Fatalf("UserAgent = %q", client.UserAgent)
	}

	version = " "
	client = newPersonalEventControlClient(
		t.TempDir(),
		"https://mcp.example.test/dws",
		personal.Identity{ClientID: "client", SourceID: "source"},
	)
	if client.ClientVersion != "unknown" || client.UserAgent != "dws-cli/unknown" {
		t.Fatalf(
			"empty-version headers = %q, %q",
			client.ClientVersion,
			client.UserAgent,
		)
	}
}

func int64Pointer(value int64) *int64 {
	return &value
}

func timePointer(value time.Time) *time.Time {
	return &value
}
