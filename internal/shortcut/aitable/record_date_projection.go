// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"strings"
	"time"
)

var recordDateZone = time.FixedZone("UTC+8", 8*60*60)

// recordDateValueEqual compares the stored instant, including the server's
// minute precision for timezone-qualified strings and integer milliseconds.
// Only the requested value is truncated: a different read-back second is not
// evidence that the requested normalization was stored.
func recordDateValueEqual(got, want any) bool {
	actual, ok := got.(string)
	if !ok {
		return false
	}
	readTime, err := time.Parse(time.RFC3339Nano, actual)
	writeTime, valid := recordDateWriteTime(want)
	return err == nil && valid && readTime.Equal(writeTime)
}

// recordDateWriteTime mirrors the standard MCP date write projection. Numeric
// strings remain a field-scoped compatibility path, never a guessed seconds
// unit. Bounded exact arithmetic rejects fractional and overflowing millis.
func recordDateWriteTime(value any) (time.Time, bool) {
	if number, ok := recordNumericValue(value); ok {
		if !number.IsInt() || !number.Num().IsInt64() {
			return time.Time{}, false
		}
		date := time.UnixMilli(number.Num().Int64()).In(recordDateZone)
		if date.Year() < 1 || date.Year() > 9999 {
			return time.Time{}, false
		}
		return date.Truncate(time.Minute), true
	}
	text, ok := value.(string)
	if !ok {
		return time.Time{}, false
	}
	text = strings.TrimSpace(text)
	for _, layout := range []string{"2006-01-02", "2006-01-02 15:04"} {
		if date, err := time.ParseInLocation(layout, text, recordDateZone); err == nil && date.Year() >= 1 {
			return date, true
		}
	}
	// Older snapshot examples omitted seconds. Continue accepting that input
	// alongside full RFC3339, as the MCP OffsetDateTime parser does.
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04Z07:00"} {
		if date, err := time.Parse(layout, text); err == nil {
			date = date.In(recordDateZone)
			if date.Year() >= 1 && date.Year() <= 9999 {
				return date.Truncate(time.Minute), true
			}
		}
	}
	return time.Time{}, false
}
