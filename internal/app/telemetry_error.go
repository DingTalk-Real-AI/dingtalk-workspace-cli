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
	stderrors "errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

const maxTelemetryErrorRunes = 200

var (
	telemetryUnknownFlagPattern  = regexp.MustCompile(`(?i)unknown flag:\s*(--[a-z0-9][a-z0-9-]*)`)
	telemetryAuthPattern         = regexp.MustCompile(`(?i)\b(?:bearer|basic)\s+[a-z0-9._~+/=-]+`)
	telemetrySensitiveFlag       = regexp.MustCompile(`(?i)(--(?:access-token|refresh-token|token|client-secret|client-id|password|api-key|authorization|cookie|credential|secret))(?:=|\s+)\S+`)
	telemetrySensitiveValue      = regexp.MustCompile(`(?i)\b(authorization|client[-_]?secret|client[-_]?id|access[-_]?token|refresh[-_]?token|api[-_]?key|password|cookie|credential|secret|token)\b\s*[:=]\s*[^\s,;]+`)
	telemetryURLPattern          = regexp.MustCompile(`(?i)\b(?:https?|wss?)://[^\s]+`)
	telemetryJSONPattern         = regexp.MustCompile(`(?s)[\[{].*[\]}]`)
	telemetryUnixPathPattern     = regexp.MustCompile(`(^|[\s=:])(?:~/|/)[^\s]+`)
	telemetryWindowsPathPattern  = regexp.MustCompile(`(?i)(^|[\s=])[a-z]:[\\/][^\s]+`)
	telemetryRelativePathPattern = regexp.MustCompile(`(^|[\s=:])\.\.?/[^\s]+`)
	telemetryEmailPattern        = regexp.MustCompile(`(?i)\b[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}\b`)
	telemetryPhonePattern        = regexp.MustCompile(`\b\+?\d[\d -]{7,}\d\b`)
	telemetryOpaqueTokenPattern  = regexp.MustCompile(`\b[a-zA-Z0-9_-]{16,}\b`)
)

func telemetryErrorSummary(err error) string {
	if err == nil {
		return ""
	}
	var patError *apperrors.PATError
	if stderrors.As(err, &patError) {
		return "permission error"
	}
	var rawError apperrors.RawStderrError
	if stderrors.As(err, &rawError) {
		return "raw stderr error"
	}
	if isUnknownCommandError(err) {
		return "unknown command"
	}
	if match := telemetryUnknownFlagPattern.FindStringSubmatch(err.Error()); len(match) == 2 {
		return "unknown flag: " + match[1]
	}
	return sanitizeTelemetryErrorText(err.Error())
}

func telemetryPanicMessages(value any) (display, summary string) {
	return fmt.Sprintf("internal panic: %v", value), "internal panic"
}

func sanitizeTelemetryErrorText(message string) string {
	message = output.SanitizeForTerminal(message)
	message = telemetryAuthPattern.ReplaceAllString(message, "<credential>")
	message = telemetrySensitiveFlag.ReplaceAllString(message, "$1=<redacted>")
	message = telemetrySensitiveValue.ReplaceAllString(message, "$1=<redacted>")
	message = telemetryURLPattern.ReplaceAllString(message, "<url>")
	message = telemetryJSONPattern.ReplaceAllString(message, "<payload>")
	message = redactTelemetryQuotedText(message)
	message = telemetryUnixPathPattern.ReplaceAllString(message, "$1<path>")
	message = telemetryWindowsPathPattern.ReplaceAllString(message, "$1<path>")
	message = telemetryRelativePathPattern.ReplaceAllString(message, "$1<path>")
	message = telemetryEmailPattern.ReplaceAllString(message, "<email>")
	message = telemetryPhonePattern.ReplaceAllString(message, "<phone>")
	message = telemetryOpaqueTokenPattern.ReplaceAllStringFunc(message, func(value string) string {
		var hasLetter, hasDigit bool
		for _, r := range value {
			hasLetter = hasLetter || unicode.IsLetter(r)
			hasDigit = hasDigit || unicode.IsDigit(r)
		}
		if hasLetter && hasDigit {
			return "<id>"
		}
		return value
	})
	message = strings.Join(strings.Fields(message), " ")
	return truncateTelemetryText(message, maxTelemetryErrorRunes)
}

func redactTelemetryQuotedText(message string) string {
	var result strings.Builder
	runes := []rune(message)
	for index := 0; index < len(runes); {
		quote := runes[index]
		if quote != '\'' && quote != '"' && quote != '`' {
			result.WriteRune(quote)
			index++
			continue
		}
		result.WriteRune(quote)
		result.WriteString("<redacted>")
		index++
		escaped := false
		for index < len(runes) {
			current := runes[index]
			index++
			if escaped {
				escaped = false
				continue
			}
			if current == '\\' && quote != '`' {
				escaped = true
				continue
			}
			if current == quote {
				result.WriteRune(quote)
				break
			}
		}
	}
	return result.String()
}

func truncateTelemetryText(message string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(message)
	if len(runes) <= maxRunes {
		return message
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-3]) + "..."
}
