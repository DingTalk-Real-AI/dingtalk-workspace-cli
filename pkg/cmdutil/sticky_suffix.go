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
	"strings"
	"unicode"
	"unicode/utf8"
)

// SuffixLooksLikeValue decides whether a candidate suffix from a glued
// "--flagsuffix" token plausibly represents a value for a flag's
// declared type/format/enum. Shared by StickyHandler (PreParse) and
// SuggestFlagFix (unknown-flag recovery).
//
// typ is a pflag value type string (e.g. "int", "bool", "string");
// format is JSON Schema "format" when present (e.g. "date-time");
// enum is the schema enum list when present.
func SuffixLooksLikeValue(suffix, typ, format string, enum []string) bool {
	if suffix == "" {
		return false
	}

	switch typ {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"integer", "number", "float", "float32", "float64",
		"count":
		return startsWithNumericSuffix(suffix)

	case "bool", "boolean":
		return isBoolLiteralSuffix(suffix)

	case "duration":
		return startsWithDigitSuffix(suffix) || startsWithSignSuffix(suffix)

	case "stringSlice", "stringArray", "intSlice", "boolSlice", "float32Slice",
		"float64Slice", "uintSlice", "durationSlice", "ipSlice", "array":
		return false

	case "object":
		return false
	}

	if len(enum) > 0 {
		return matchesEnumSuffix(suffix, enum)
	}

	switch strings.ToLower(format) {
	case "date", "date-time", "datetime", "time":
		return startsWithDigitSuffix(suffix)
	case "duration":
		return startsWithDigitSuffix(suffix) || startsWithSignSuffix(suffix)
	case "email":
		return strings.Contains(suffix, "@")
	case "uri", "url":
		lower := strings.ToLower(suffix)
		return strings.HasPrefix(lower, "http") ||
			strings.HasPrefix(lower, "ftp") ||
			strings.HasPrefix(lower, "mailto:")
	case "ipv4", "ipv6", "hostname":
		return startsWithDigitSuffix(suffix)
	case "uuid":
		first, _ := utf8.DecodeRuneInString(suffix)
		return isHexRuneSuffix(first)
	}

	first, _ := utf8.DecodeRuneInString(suffix)
	if first == utf8.RuneError || unicode.IsLetter(first) {
		return false
	}
	return true
}

func startsWithDigitSuffix(s string) bool {
	if s == "" {
		return false
	}
	c := s[0]
	return c >= '0' && c <= '9'
}

func startsWithSignSuffix(s string) bool {
	if s == "" {
		return false
	}
	c := s[0]
	return c == '+' || c == '-'
}

func startsWithNumericSuffix(s string) bool {
	if startsWithDigitSuffix(s) {
		return true
	}
	if startsWithSignSuffix(s) && len(s) > 1 {
		c := s[1]
		return c >= '0' && c <= '9'
	}
	return false
}

func isBoolLiteralSuffix(s string) bool {
	switch strings.ToLower(s) {
	case "true", "false", "1", "0", "t", "f", "yes", "no", "on", "off", "y", "n":
		return true
	}
	return false
}

func matchesEnumSuffix(s string, enum []string) bool {
	lower := strings.ToLower(s)
	for _, e := range enum {
		if strings.ToLower(e) == lower {
			return true
		}
	}
	return false
}

func isHexRuneSuffix(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}
