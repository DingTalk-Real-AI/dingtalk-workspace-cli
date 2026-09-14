// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	"strconv"
	"strings"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/runtimecred"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

const personalSubscriptionAttemptOperation = "event.consume.personal.subscribe"

type personalSubscriptionAttemptStore interface {
	Claim([]personal.AttemptSpec, time.Duration) (*personal.AttemptClaim, error)
	CompleteSuccess(*personal.AttemptClaim) error
	CompleteFailure(*personal.AttemptClaim, []string, personal.AttemptFailure) (personal.AttemptHold, error)
	Release(*personal.AttemptClaim) error
}

var (
	personalNewSubscriptionAttemptStore = func(workDir string) personalSubscriptionAttemptStore {
		return personal.NewAttemptStore(workDir)
	}
	personalSubscriptionAttemptNow = time.Now
)

type personalSubscriptionAttemptItem struct {
	eventKey    string
	fingerprint string
}

type personalSubscriptionAttemptReservation struct {
	store personalSubscriptionAttemptStore
	claim *personal.AttemptClaim
	items []personalSubscriptionAttemptItem
}

type personalSubscriptionFailureClass struct {
	retryability personal.Retryability
	retryAfter   time.Duration
	code         string
	traceID      string
	reason       string
	auth         bool
}

func reservePersonalSubscriptionAttempts(
	workDir string,
	client *personal.Client,
	identity personal.Identity,
	profileSelector string,
	plans []personalConsumeOptions,
) (*personalSubscriptionAttemptReservation, error) {
	if len(plans) == 0 {
		return nil, personalSubscriptionGuardError(
			errors.New("personal event: no subscription attempts to reserve"),
		)
	}
	if client == nil {
		return nil, personalSubscriptionGuardError(
			errors.New("personal event: nil subscription control client"),
		)
	}
	if err := validatePersonalSubscriptionEndpoint(client.BaseURL); err != nil {
		return nil, personalSubscriptionValidationError(err)
	}

	items := make([]personalSubscriptionAttemptItem, 0, len(plans))
	specs := make([]personal.AttemptSpec, 0, len(plans))
	for _, plan := range plans {
		prepared, err := preparePersonalSubscription(identity, plan)
		if err != nil {
			return nil, personalSubscriptionValidationError(err)
		}
		fingerprint := personal.Fingerprint(
			client.BaseURL,
			prepared.Request.IdempotencyKey,
			profileSelector,
		)
		items = append(items, personalSubscriptionAttemptItem{
			eventKey:    prepared.EventKey,
			fingerprint: fingerprint,
		})
		specs = append(specs, personal.AttemptSpec{
			Fingerprint: fingerprint,
			EventKey:    prepared.EventKey,
		})
	}

	store := personalNewSubscriptionAttemptStore(workDir)
	if store == nil {
		return nil, personalSubscriptionGuardError(
			errors.New("personal event: subscription attempt store is unavailable"),
		)
	}
	claim, err := store.Claim(specs, personalSubscriptionAttemptLease(client, len(specs)))
	if err != nil {
		var blocked *personal.AttemptBlockedError
		if errors.As(err, &blocked) {
			return nil, personalSubscriptionBlockedError(blocked)
		}
		return nil, personalSubscriptionGuardError(err)
	}
	return &personalSubscriptionAttemptReservation{
		store: store,
		claim: claim,
		items: items,
	}, nil
}

func validatePersonalSubscriptionEndpoint(raw string) error {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" ||
		(!strings.EqualFold(parsed.Scheme, "http") &&
			!strings.EqualFold(parsed.Scheme, "https")) {
		if err == nil {
			err = errors.New("an absolute http(s) URL is required")
		}
		return fmt.Errorf("personal event: invalid subscription control endpoint %q: %w", raw, err)
	}
	return nil
}

func personalSubscriptionAttemptLease(client *personal.Client, batchSize int) time.Duration {
	const (
		leaseOverhead = 30 * time.Second
		minLease      = time.Minute
		maxLease      = 10 * time.Minute
	)
	if batchSize < 1 {
		batchSize = 1
	}
	timeout := config.HTTPTimeout
	if client != nil && client.HTTPClient != nil && client.HTTPClient.Timeout > 0 {
		timeout = client.HTTPClient.Timeout
	}
	maxRequestBudget := maxLease - leaseOverhead
	if timeout <= 0 || timeout > maxRequestBudget/time.Duration(batchSize) {
		return maxLease
	}
	lease := timeout*time.Duration(batchSize) + leaseOverhead
	if lease < minLease {
		return minLease
	}
	return lease
}

func (r *personalSubscriptionAttemptReservation) completeSuccess() error {
	if r == nil {
		return nil
	}
	if r.store == nil || r.claim == nil {
		return personalSubscriptionGuardError(
			errors.New("personal event: subscription attempt reservation is incomplete"),
		)
	}
	if err := r.store.CompleteSuccess(r.claim); err != nil {
		return personalSubscriptionGuardError(err)
	}
	return nil
}

// releaseRuntimeTokenFailure releases the in-flight claim without recording a
// cross-invocation hold. A host may supply a fresh token on the very next
// command, which must be allowed to retry immediately.
func (r *personalSubscriptionAttemptReservation) releaseRuntimeTokenFailure() error {
	if r == nil {
		return runtimecred.ErrRuntimeTokenRejected
	}
	if r.store == nil || r.claim == nil {
		return personalSubscriptionGuardError(errors.Join(
			runtimecred.ErrRuntimeTokenRejected,
			errors.New("personal event: subscription attempt reservation is incomplete"),
		))
	}
	if err := r.store.Release(r.claim); err != nil {
		return personalSubscriptionGuardError(errors.Join(runtimecred.ErrRuntimeTokenRejected, err))
	}
	return runtimecred.ErrRuntimeTokenRejected
}

func (r *personalSubscriptionAttemptReservation) completeFailure(
	ctx context.Context,
	failedIndex int,
	succeededCount int,
	cause error,
	override *personalSubscriptionFailureClass,
) error {
	if r == nil {
		return cause
	}
	if r.store == nil || r.claim == nil {
		return personalSubscriptionGuardError(errors.Join(
			cause,
			errors.New("personal event: subscription attempt reservation is incomplete"),
		))
	}
	if failedIndex < 0 || failedIndex >= len(r.items) ||
		succeededCount < 0 || succeededCount > failedIndex {
		return personalSubscriptionGuardError(errors.Join(
			cause,
			errors.New("personal event: invalid subscription attempt completion indexes"),
		))
	}
	if personalSubscriptionCanceled(ctx, cause) {
		// Cancellation is not a failed attempt. Restoring the claim normally
		// completes immediately; if the lock cannot be acquired, leaving the
		// finite lease behind is still safer than recording a false failure.
		_ = r.store.Release(r.claim)
		return cause
	}

	classification := classifyPersonalSubscriptionFailure(cause, personalSubscriptionAttemptNow())
	if override != nil {
		classification = *override
	}
	succeeded := make([]string, 0, succeededCount)
	for i := 0; i < succeededCount; i++ {
		succeeded = append(succeeded, r.items[i].fingerprint)
	}
	hold, err := r.store.CompleteFailure(r.claim, succeeded, personal.AttemptFailure{
		Fingerprint:  r.items[failedIndex].fingerprint,
		Retryability: classification.retryability,
		RetryAfter:   classification.retryAfter,
		ErrorCode:    classification.code,
		TraceID:      classification.traceID,
	})
	if err != nil {
		return personalSubscriptionGuardError(errors.Join(cause, err))
	}
	return personalSubscriptionFailureError(cause, classification, hold)
}

func personalSubscriptionCanceled(ctx context.Context, err error) bool {
	if errors.Is(err, context.Canceled) {
		return true
	}
	return ctx != nil && errors.Is(ctx.Err(), context.Canceled)
}

func classifyPersonalSubscriptionFailure(err error, now time.Time) personalSubscriptionFailureClass {
	classification := personalSubscriptionFailureClass{
		retryability: personal.RetryabilityUnknown,
		reason:       "personal_subscription_unknown",
	}

	var apiErr *personal.APIError
	if errors.As(err, &apiErr) {
		classification.code = strings.TrimSpace(apiErr.Code)
		classification.traceID = strings.TrimSpace(apiErr.TraceID)
		classification.retryAfter = personalAPIRetryDelay(apiErr, now)
		classification.auth = personalSubscriptionAuthFailure(apiErr.HTTPStatus, apiErr.Code)
		switch {
		case apiErr.Retryable != nil && *apiErr.Retryable:
			classification.retryability = personal.RetryabilityRetryable
			classification.reason = "personal_subscription_server_retryable"
		case apiErr.Retryable != nil:
			classification.retryability = personal.RetryabilityNonRetryable
			classification.reason = "personal_subscription_server_non_retryable"
		case apiErr.HTTPStatus == http.StatusRequestTimeout ||
			apiErr.HTTPStatus == http.StatusTooEarly ||
			apiErr.HTTPStatus == http.StatusTooManyRequests ||
			apiErr.HTTPStatus >= http.StatusInternalServerError:
			classification.retryability = personal.RetryabilityRetryable
			classification.reason = "personal_subscription_transient_http"
		case apiErr.HTTPStatus == http.StatusUnauthorized ||
			apiErr.HTTPStatus == http.StatusForbidden:
			classification.retryability = personal.RetryabilityNonRetryable
			classification.reason = "personal_subscription_auth"
		case personalSubscriptionTerminalBusinessCode(apiErr.Code):
			classification.retryability = personal.RetryabilityNonRetryable
			classification.reason = "personal_subscription_business_rejected"
		case personalSubscriptionErrorHasSubscribeID(apiErr):
			// A few legacy/proxy error shapes include an existing subscription
			// ID without a stable server contract. Keep the response as an
			// error, but do not turn that unverified shape into a one-hour hold.
			classification.reason = "personal_subscription_unverified_existing_id"
		case apiErr.HTTPStatus >= http.StatusBadRequest:
			classification.retryability = personal.RetryabilityNonRetryable
			classification.reason = "personal_subscription_http_rejected"
		}
		return classification
	}

	if errors.Is(err, context.DeadlineExceeded) {
		classification.retryability = personal.RetryabilityRetryable
		classification.reason = "personal_subscription_timeout"
		return classification
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if strings.EqualFold(strings.TrimSpace(urlErr.Op), "parse") {
			classification.retryability = personal.RetryabilityNonRetryable
			classification.reason = "personal_subscription_invalid"
			return classification
		}
		classification.retryability = personal.RetryabilityRetryable
		classification.reason = "personal_subscription_network"
		return classification
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		classification.retryability = personal.RetryabilityRetryable
		classification.reason = "personal_subscription_network"
		return classification
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		classification.retryability = personal.RetryabilityRetryable
		classification.reason = "personal_subscription_network"
		return classification
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "access token") || strings.Contains(lower, "oauth") {
		classification.retryability = personal.RetryabilityNonRetryable
		classification.reason = "personal_subscription_auth"
		classification.auth = true
	}
	return classification
}

func personalSubscriptionErrorHasSubscribeID(apiErr *personal.APIError) bool {
	if apiErr == nil {
		return false
	}
	subscribeID, ok := apiErr.Details["subscribe_id"].(string)
	return ok && strings.TrimSpace(subscribeID) != ""
}

func personalAPIRetryDelay(apiErr *personal.APIError, now time.Time) time.Duration {
	if apiErr == nil {
		return 0
	}
	var delay time.Duration
	if apiErr.RetryAfterSeconds != nil {
		delay = maxPersonalRetryDelay(delay, personalRetrySeconds(*apiErr.RetryAfterSeconds))
	}
	if apiErr.NextRetryAt != nil {
		delay = maxPersonalRetryDelay(delay, apiErr.NextRetryAt.Sub(now))
	}
	if raw, ok := apiErr.Details["retry_after"].(string); ok {
		raw = strings.TrimSpace(raw)
		if seconds, err := strconv.ParseInt(raw, 10, 64); err == nil {
			delay = maxPersonalRetryDelay(delay, personalRetrySeconds(seconds))
		} else if next, err := http.ParseTime(raw); err == nil {
			delay = maxPersonalRetryDelay(delay, next.Sub(now))
		}
	}
	return delay
}

func personalRetrySeconds(seconds int64) time.Duration {
	if seconds <= 0 {
		return 0
	}
	if seconds > math.MaxInt64/int64(time.Second) {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(seconds) * time.Second
}

func maxPersonalRetryDelay(left, right time.Duration) time.Duration {
	if right > left {
		return right
	}
	return left
}

func personalSubscriptionTerminalBusinessCode(raw string) bool {
	code := strings.ToUpper(strings.TrimSpace(raw))
	replacer := strings.NewReplacer("-", "_", ".", "_", " ", "_")
	code = replacer.Replace(code)

	// Keep this list deliberately conservative. Unknown server codes must stay
	// unknown so a newly introduced transient condition cannot accidentally be
	// converted into a one-hour terminal hold.
	switch code {
	case "INVALID_PARAM", "INVALID_PARAMS", "INVALID_PARAMETER", "INVALID_PARAMETERS",
		"ILLEGAL_PARAM", "ILLEGAL_PARAMS", "ILLEGAL_PARAMETER", "ILLEGAL_PARAMETERS",
		"PARAM_ERROR", "PARAMETER_ERROR",
		"CLIENT_ID_REQUIRED", "SOURCE_ID_REQUIRED", "EVENT_KEY_REQUIRED", "RULE_TYPE_REQUIRED",
		"NO_AUTH", "NO_PERMISSION", "PERMISSION_DENIED", "ACCESS_DENIED",
		"FORBIDDEN", "UNAUTHORIZED",
		"NOT_FOUND", "NOT_EXIST", "NOT_SUPPORTED", "UNSUPPORTED",
		"UNIFIED_APP_ID_NOT_FOUND":
		return true
	}

	// Resource-qualified variants are stable business-rejection shapes. Avoid
	// broad substring matching (for example, RETRY_REQUIRED must remain
	// unknown).
	for _, suffix := range []string{
		"_NOT_BELONG_TO_ORG",
		"_DOES_NOT_BELONG_TO_ORG",
		"_NOT_FOUND",
		"_NOT_EXIST",
		"_NOT_SUPPORTED",
		"_UNSUPPORTED",
		"_NO_PERMISSION",
		"_PERMISSION_DENIED",
		"_ACCESS_DENIED",
	} {
		if strings.HasSuffix(code, suffix) {
			return true
		}
	}
	return false
}

func personalSubscriptionAuthFailure(status int, rawCode string) bool {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return true
	}
	code := strings.ToUpper(strings.TrimSpace(rawCode))
	for _, marker := range []string{
		"NO_AUTH", "UNAUTHORIZED", "FORBIDDEN", "PERMISSION", "ACCESS_DENIED",
	} {
		if strings.Contains(code, marker) {
			return true
		}
	}
	return false
}

func personalSubscriptionFailureError(
	cause error,
	classification personalSubscriptionFailureClass,
	hold personal.AttemptHold,
) error {
	options := personalSubscriptionErrorOptions(
		classification.retryability,
		hold.RetryAfter,
		hold.NextAllowedAt,
		classification.code,
		classification.traceID,
		classification.reason,
		cause,
	)
	message := cause.Error()
	if classification.retryability == personal.RetryabilityNonRetryable {
		if classification.auth {
			return apperrors.NewAuth(message, options...)
		}
		return apperrors.NewValidation(message, options...)
	}
	return apperrors.NewAPI(message, options...)
}

func personalSubscriptionBlockedError(blocked *personal.AttemptBlockedError) error {
	if blocked == nil {
		return personalSubscriptionGuardError(
			errors.New("personal event: nil blocked subscription attempt"),
		)
	}
	reason := "personal_subscription_" + string(blocked.State)
	options := personalSubscriptionErrorOptions(
		blocked.Retryability,
		blocked.RetryAfter,
		blocked.NextAllowedAt,
		blocked.ErrorCode,
		blocked.TraceID,
		reason,
		blocked,
	)
	if blocked.Retryability == personal.RetryabilityNonRetryable {
		if personalSubscriptionAuthFailure(0, blocked.ErrorCode) {
			return apperrors.NewAuth(blocked.Error(), options...)
		}
		return apperrors.NewValidation(blocked.Error(), options...)
	}
	return apperrors.NewAPI(blocked.Error(), options...)
}

func personalSubscriptionErrorOptions(
	retryability personal.Retryability,
	retryAfter time.Duration,
	nextRetryAt time.Time,
	code string,
	traceID string,
	reason string,
	cause error,
) []apperrors.Option {
	options := []apperrors.Option{
		apperrors.WithOperation(personalSubscriptionAttemptOperation),
		apperrors.WithReason(reason),
		apperrors.WithCause(cause),
	}
	if retryable, known := retryability.Value(); known {
		options = append(options, apperrors.WithRetryable(retryable))
	}
	if retryAfter > 0 {
		options = append(options, apperrors.WithRetryAfterSeconds(ceilPersonalRetrySeconds(retryAfter)))
	}
	if !nextRetryAt.IsZero() {
		options = append(options, apperrors.WithNextRetryAt(nextRetryAt))
	}
	if code != "" || traceID != "" {
		options = append(options, apperrors.WithServerDiag(apperrors.ServerDiagnostics{
			TraceID:         strings.TrimSpace(traceID),
			ServerErrorCode: strings.TrimSpace(code),
		}))
	}
	return options
}

func ceilPersonalRetrySeconds(delay time.Duration) int64 {
	if delay <= 0 {
		return 0
	}
	seconds := int64(delay / time.Second)
	if delay%time.Second != 0 {
		seconds++
	}
	return seconds
}

func personalSubscriptionGuardError(cause error) error {
	if cause == nil {
		cause = errors.New("personal event: subscription attempt guard failed")
	}
	return apperrors.NewInternal(
		fmt.Sprintf("personal subscription attempt guard failed: %v", cause),
		apperrors.WithOperation(personalSubscriptionAttemptOperation),
		apperrors.WithReason("personal_subscription_guard_failed"),
		apperrors.WithRetryable(false),
		apperrors.WithCause(cause),
	)
}

func personalSubscriptionValidationError(cause error) error {
	if cause == nil {
		cause = errors.New("personal event: invalid subscription parameters")
	}
	return apperrors.NewValidation(
		cause.Error(),
		apperrors.WithOperation(personalSubscriptionAttemptOperation),
		apperrors.WithReason("personal_subscription_invalid"),
		apperrors.WithRetryable(false),
		apperrors.WithCause(cause),
	)
}

func personalSubscriptionLocalFailure() personalSubscriptionFailureClass {
	return personalSubscriptionFailureClass{
		retryability: personal.RetryabilityUnknown,
		reason:       "personal_subscription_local_failure",
	}
}
