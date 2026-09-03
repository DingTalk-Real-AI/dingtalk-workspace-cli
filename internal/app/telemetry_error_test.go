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
	"errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

type telemetryRawError string

func (e telemetryRawError) Error() string     { return string(e) }
func (e telemetryRawError) RawStderr() string { return string(e) }

func TestCrossPlatformCoverageTelemetryErrorSummaryFixedFamilies(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{name: "nil", want: ""},
		{name: "PAT", err: &apperrors.PATError{RawJSON: `{"token":"secret"}`}, want: "permission error"},
		{name: "raw stderr", err: telemetryRawError("raw secret"), want: "raw stderr error"},
		{name: "unknown command", err: errors.New(`unknown command "secret-value" for "dws"`), want: "unknown command"},
		{name: "unknown flag", err: errors.New("unknown flag: --token=secret-value"), want: "unknown flag: --token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := telemetryErrorSummary(tc.err); got != tc.want {
				t.Fatalf("telemetryErrorSummary() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageSanitizeTelemetryErrorText(t *testing.T) {
	message := "\x1b[31mfailed\x1b[0m " +
		"--client-secret very-secret " +
		"--access-token access-secret " +
		"Authorization: Bearer abcdefghijklmnop1234 " +
		"url=https://example.test/path?token=secret " +
		`body={"access_token":"secret"} ` +
		`user="Alice" email=alice@example.test phone=13800138000 ` +
		"path=/Users/alice/private.txt relative=./private/secrets.txt id=abcDEF1234567890XYZ"
	got := sanitizeTelemetryErrorText(message)
	for _, secret := range []string{
		"very-secret", "access-secret", "abcdefghijklmnop1234", "example.test", "access_token",
		"Alice", "alice@example.test", "13800138000", "/Users/alice", "abcDEF1234567890XYZ", "\x1b",
		"./private/secrets.txt",
	} {
		if strings.Contains(got, secret) {
			t.Fatalf("sanitized telemetry error leaked %q: %q", secret, got)
		}
	}
	for _, marker := range []string{"failed", "<redacted>", "<url>", "<payload>", "<path>", "<id>"} {
		if !strings.Contains(got, marker) {
			t.Fatalf("sanitized telemetry error missing %q: %q", marker, got)
		}
	}
}

func TestCrossPlatformCoverageTelemetryErrorTruncationAndPanic(t *testing.T) {
	message := strings.Repeat("错", maxTelemetryErrorRunes+1)
	got := sanitizeTelemetryErrorText(message)
	if len([]rune(got)) != maxTelemetryErrorRunes || !strings.HasSuffix(got, "...") {
		t.Fatalf("truncated telemetry error rune length = %d suffix = %q", len([]rune(got)), got[len(got)-3:])
	}
	display, summary := telemetryPanicMessages("token-secret")
	if display != "internal panic: token-secret" || summary != "internal panic" || strings.Contains(summary, "token-secret") {
		t.Fatalf("panic messages = display %q summary %q", display, summary)
	}
	if got := truncateTelemetryText("value", 0); got != "" {
		t.Fatalf("zero-limit truncation = %q", got)
	}
	if got := truncateTelemetryText("value", 3); got != "val" {
		t.Fatalf("short-limit truncation = %q", got)
	}
}

func TestCrossPlatformCoverageTelemetryErrorEscapesAndOpaqueWords(t *testing.T) {
	const opaqueWord = "abcdefghijklmnop"
	got := sanitizeTelemetryErrorText(`failed "quoted \"value" ` + opaqueWord)
	if strings.Contains(got, "value") || !strings.Contains(got, opaqueWord) {
		t.Fatalf("escaped quote sanitization = %q", got)
	}
}
