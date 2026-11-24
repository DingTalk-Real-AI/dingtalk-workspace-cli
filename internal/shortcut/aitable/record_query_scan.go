// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"context"
	"errors"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// recordQueryReadPolicy makes ordinary-read safeguards opt-in. Exact-ID reads,
// write verification and shared composite scans retain their existing policy.
type recordQueryReadPolicy struct {
	pageSize int
	scan     bool
	ctx      context.Context // Set only for a bounded timeout fallback; never mutates Cobra state.
}

// recordQueryReadState retains earlier count evidence when a terminal response
// drops its metadata. It is local to one attempt and is reset on fallback.
type recordQueryReadState struct {
	query    map[string]any
	fetched  int
	total    any
	hasTotal bool
	pageSize int
}

var errRecordQueryAmbiguousEmpty = errors.New("query_records legacy empty response requires a small-page verification")

// queryRecordWindowForScan retries a recognized size/legacy-shape problem or an
// eligible large-page RPC timeout once. It discards the failed attempt and restarts at the
// caller's original cursor, so results from separate attempts are never merged.
func queryRecordWindowForScan(rt *shortcut.RuntimeContext, params map[string]any, limit int) (recordQueryWindow, error) {
	window, err := queryRecordWindowWithPolicy(rt, params, limit, recordQueryReadPolicy{pageSize: recordQueryScanPageSize, scan: true})
	policy, cancel, retry := recordQueryFallbackPolicy(rt, err)
	if !retry {
		return window, err
	}
	defer cancel()
	return queryRecordWindowWithPolicy(rt, params, limit, policy)
}

// queryAllRecordsForScan applies the same bounded fallback to a full read. Its
// second attempt starts from the original query and retains max-records checks.
func queryAllRecordsForScan(rt *shortcut.RuntimeContext, params map[string]any, maxRecords int) ([]map[string]any, error) {
	records, err := queryAllRecordsWithPolicy(rt, params, maxRecords, recordQueryReadPolicy{pageSize: recordQueryScanPageSize, scan: true})
	policy, cancel, retry := recordQueryFallbackPolicy(rt, err)
	if !retry {
		return records, err
	}
	defer cancel()
	return queryAllRecordsWithPolicy(rt, params, maxRecords, policy)
}

// recordQueryNeedsSmallPage never guesses from error messages. A timeout must
// have been classified at an actual large-page RPC; cancellation and unrelated
// network, permission or cursor failures keep their original outcome.
func recordQueryNeedsSmallPage(err error) bool {
	if errors.Is(err, errRecordQueryAmbiguousEmpty) {
		return true
	}
	var timeout *recordQueryLargePageTimeout
	if errors.As(err, &timeout) {
		return true
	}
	var classified *apperrors.Error
	return errors.As(err, &classified) && classified.Category == apperrors.CategoryDiscovery &&
		classified.Operation == "tools/call" && classified.Reason == "tools_call_response_too_large"
}

// validateEmptyPage rejects contradictory terminal counts or a legacy empty
// object advertising more data. A large legacy empty object without sufficient
// evidence gets one fresh 20-row read, preserving the reviewed small-page zero
// result contract without accepting an ambiguous large response as zero rows.
func (policy recordQueryReadPolicy) validateEmptyPage(data map[string]any, records []map[string]any, found bool, state recordQueryReadState) error {
	if !policy.scan || len(records) != 0 {
		return nil
	}
	legacyEmpty := explicitEmptyRecordQuery(data)
	if !found && !legacyEmpty {
		return nil // The existing missing-records guard reports this shape.
	}
	total, hasTotal, err := recordQueryEmptyMetadata(data)
	if err != nil {
		return err
	}
	if !hasTotal && state.hasTotal && state.total != nil {
		total, err = recordQueryEmptyCount(state.total)
		if err != nil {
			return err
		}
		hasTotal = true
	}
	hasMore := responseHasMore(data)
	if legacyEmpty && (hasMore || responseCursor(data) != "") {
		return recordQueryInconsistentEmpty("legacy empty response advertises more data")
	}
	if !hasMore && responseCursor(data) != "" {
		return recordQueryInconsistentEmpty("empty response has unsupported continuation metadata")
	}
	initialCursor, _ := state.query["cursor"].(string)
	fromFirstPage := strings.TrimSpace(initialCursor) == ""
	verifiedZero := false
	if hasTotal {
		if !hasMore && fromFirstPage && total != int64(state.fetched) {
			return recordQueryInconsistentEmpty("empty terminal response contradicts totalCount")
		}
		verifiedZero = fromFirstPage && state.fetched == 0 && total == 0
	}
	if legacyEmpty && state.pageSize > recordQueryServicePageSize && !verifiedZero {
		return errRecordQueryAmbiguousEmpty
	}
	return nil
}

// recordQueryEmptyMetadata validates the same envelope containers used by the
// pager. Malformed pagination values or conflicting totals cannot prove that an
// empty response is a terminal page. Null optional cursors remain compatible.
func recordQueryEmptyMetadata(data map[string]any) (int64, bool, error) {
	for _, key := range []string{"hasMore", "nextCursor", "next_cursor", "cursor"} {
		if value, present := data[key]; present && value != nil {
			_, valid := value.(string)
			if key == "hasMore" {
				_, valid = value.(bool)
			}
			if !valid {
				return 0, false, recordQueryInconsistentEmpty("empty response has invalid pagination metadata")
			}
		}
	}
	var total int64
	raw, hasTotal := data["totalCount"]
	hasTotal = hasTotal && raw != nil
	if hasTotal {
		var err error
		total, err = recordQueryEmptyCount(raw)
		if err != nil {
			return 0, false, err
		}
	}
	for _, key := range []string{"data", "result", "pagination", "page"} {
		if nested, ok := data[key].(map[string]any); ok {
			count, present, err := recordQueryEmptyMetadata(nested)
			if err != nil {
				return 0, false, err
			}
			if present {
				if hasTotal && count != total {
					return 0, false, recordQueryInconsistentEmpty("empty response has conflicting totalCount values")
				}
				total, hasTotal = count, true
			}
		}
	}
	return total, hasTotal, nil
}

// recordQueryEmptyCount accepts only finite nonnegative integer counts, using
// the existing exact JSON-number parser for both current and earlier metadata.
func recordQueryEmptyCount(raw any) (int64, error) {
	count, valid := recordNumericValue(raw)
	if !valid || !count.IsInt() || count.Sign() < 0 || !count.Num().IsInt64() {
		return 0, recordQueryInconsistentEmpty("empty response has an invalid totalCount")
	}
	return count.Num().Int64(), nil
}

// recordQueryInconsistentEmpty reports an integrity failure without publishing
// any accumulated rows or manufacturing a continuation cursor for the caller.
func recordQueryInconsistentEmpty(message string) error {
	return apperrors.NewAPI("query_records: "+message,
		apperrors.WithOperation("aitable.query_records"),
		apperrors.WithReason("record_query_inconsistent_empty_response"),
		apperrors.WithRetryable(false),
		apperrors.WithDetails(map[string]any{"discard_previous_results": true}),
	)
}
