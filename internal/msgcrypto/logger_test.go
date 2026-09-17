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

package msgcrypto

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// captureLogf collects emitted lines for assertions.
func captureLogf(lines *[]string) func(string, ...any) {
	return func(format string, args ...any) {
		*lines = append(*lines, fmt.Sprintf(format, args...))
	}
}

func TestCrossPlatformCoverageNewRedactingLoggerReturnsNilWhenSinkIsNil(t *testing.T) {
	// A nil logger makes the vendor SDK skip logging entirely, which is the
	// safe default because it logs the authCode at debug level.
	if got := newRedactingLogger(nil); got != nil {
		t.Fatalf("newRedactingLogger(nil) = %v, want nil", got)
	}
}

func TestCrossPlatformCoverageRedactingLoggerHidesAuthCode(t *testing.T) {
	var lines []string
	logger := newRedactingLogger(captureLogf(&lines))

	// This mirrors the vendor SDK's own debug line, which prints the code.
	const secret = "abc123authcode"
	logger.Debug("Code (auth_token, length=%d): %s", len(secret), secret)

	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}
	if strings.Contains(lines[0], secret) {
		t.Fatalf("log line leaked the auth code: %q", lines[0])
	}
	if !strings.Contains(lines[0], "redacted") {
		t.Fatalf("log line = %q, want a redaction marker", lines[0])
	}
}

func TestCrossPlatformCoverageRedactingLoggerHidesKeyServerResponseBody(t *testing.T) {
	var lines []string
	logger := newRedactingLogger(captureLogf(&lines))

	body := `{"key":"BASE64KEYMATERIAL==","keyVersion":3}`
	logger.Debug("Body (length=%d): %s", len(body), body)

	if strings.Contains(lines[0], "BASE64KEYMATERIAL") {
		t.Fatalf("log line leaked key material: %q", lines[0])
	}
}

func TestCrossPlatformCoverageRedactingLoggerHidesByteSlices(t *testing.T) {
	var lines []string
	logger := newRedactingLogger(captureLogf(&lines))

	logger.Info("payload=%s", []byte("plaintext-message"))

	if strings.Contains(lines[0], "plaintext-message") {
		t.Fatalf("log line leaked a byte payload: %q", lines[0])
	}
}

func TestCrossPlatformCoverageRedactingLoggerHidesErrorText(t *testing.T) {
	var lines []string
	logger := newRedactingLogger(captureLogf(&lines))

	// Vendor error strings can embed a response body preview.
	logger.Error("request failed: %v", errors.New(`server said {"key":"LEAKED"}`))

	if strings.Contains(lines[0], "LEAKED") {
		t.Fatalf("log line leaked error contents: %q", lines[0])
	}
}

func TestCrossPlatformCoverageRedactingLoggerKeepsDiagnosticNumbers(t *testing.T) {
	var lines []string
	logger := newRedactingLogger(captureLogf(&lines))

	logger.Debug("Status: %d (took %s), size=%d", 503, 1500*time.Millisecond, 4096)

	line := lines[0]
	for _, want := range []string{"503", "1.5s", "4096"} {
		if !strings.Contains(line, want) {
			t.Fatalf("log line = %q, want it to keep %q for diagnostics", line, want)
		}
	}
}

func TestCrossPlatformCoverageRedactingLoggerLabelsLevel(t *testing.T) {
	var lines []string
	logger := newRedactingLogger(captureLogf(&lines))

	logger.Debug("d")
	logger.Info("i")
	logger.Error("e")

	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
	for i, want := range []string{"debug", "info", "error"} {
		if !strings.Contains(lines[i], want) {
			t.Fatalf("line %d = %q, want level %q", i, lines[i], want)
		}
	}
}

func TestCrossPlatformCoverageRedactingLoggerToleratesNilSinkAtEmit(t *testing.T) {
	// Guard against a partially constructed logger being used.
	var logger *redactingLogger
	logger.Debug("must not panic %s", "value")
}

func TestCrossPlatformCoverageRedactArgKeepsDurations(t *testing.T) {
	if got := redactArg(2 * time.Second); got != "2s" {
		t.Fatalf("redactArg(2s) = %v, want 2s", got)
	}
}

func TestCrossPlatformCoverageRedactArgElidesStrings(t *testing.T) {
	got, ok := redactArg("secret").(string)
	if !ok {
		t.Fatalf("redactArg returned %T, want string", got)
	}
	if strings.Contains(got, "secret") {
		t.Fatalf("redactArg = %q, want the value elided", got)
	}
	if !strings.Contains(got, "len=6") {
		t.Fatalf("redactArg = %q, want the length preserved", got)
	}
}

func TestCrossPlatformCoverageRedactArgPassesThroughNumbers(t *testing.T) {
	if got := redactArg(42); got != 42 {
		t.Fatalf("redactArg(42) = %v, want 42", got)
	}
	if got := redactArg(true); got != true {
		t.Fatalf("redactArg(true) = %v, want true", got)
	}
}

type secretStringer string

func (s secretStringer) String() string { return string(s) }

func TestCrossPlatformCoverageRedactArgElidesStringerPayloads(t *testing.T) {
	got, ok := redactArg(secretStringer("stringer-secret")).(string)
	if !ok {
		t.Fatalf("redactArg returned %T, want string", got)
	}
	if strings.Contains(got, "stringer-secret") || !strings.Contains(got, "len=15") {
		t.Fatalf("redactArg stringer = %q", got)
	}
}
