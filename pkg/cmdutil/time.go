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

package cmdutil

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// FlexTimeLayouts is the ordered list of time formats tried by ParseISOTimeToMillis.
var FlexTimeLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05Z",
	"2006-01-02T15:04:05-07:00",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04",
	"2006-01-02 15:04",
	"2006-01-02",
	"2006/01/02 15:04:05",
	"2006/01/02",
	"20060102",
}

// ParseISOTimeToMillis parses a time string into a millisecond Unix timestamp.
// Supports RFC3339, UTC Z, timezone-less, space-separated, date-only, and more.
// When the input lacks an explicit timezone, Asia/Shanghai is assumed.
func ParseISOTimeToMillis(flagName, value string) (int64, error) {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"'")
	if value == "" {
		return 0, fmt.Errorf("flag --%s is required\n  hint: example: 2026-03-10T14:00:00+08:00", flagName)
	}
	if isDecimalDigits(value) {
		switch len(value) {
		case 13:
			ms, err := strconv.ParseInt(value, 10, 64)
			if err == nil {
				return ms, nil
			}
		case 10:
			sec, err := strconv.ParseInt(value, 10, 64)
			if err == nil {
				return sec * 1000, nil
			}
		}
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	if loc == nil {
		loc = time.Local
	}
	for _, layout := range FlexTimeLayouts {
		t, err := time.ParseInLocation(layout, value, loc)
		if err == nil {
			return t.UnixMilli(), nil
		}
	}
	return 0, fmt.Errorf(
		"cannot parse time for --%s (input: %q)\n  hint: supported formats: 2026-03-23T14:00:00+08:00, 2026-03-23 14:00:00, 2026-03-23",
		flagName, value)
}

func isDecimalDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ValidateTimeRange checks that endMs is strictly after startMs.
func ValidateTimeRange(startMs, endMs int64) error {
	if endMs <= startMs {
		return fmt.Errorf("--end must be after --start\n  hint: swap the values or adjust the time range")
	}
	return nil
}
