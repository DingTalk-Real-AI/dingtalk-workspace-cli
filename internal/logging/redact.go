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

package logging

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/requestmeta"
)

// sensitiveKeys are header/field names whose values must be redacted in logs.
var sensitiveKeys = map[string]bool{
	"authorization":               true,
	"x-user-access-token":         true,
	"dws_agent_ext":               true,
	"x-dws-agent-ext":             true,
	requestmeta.DingTalkExtHeader: true,
	"umid":                        true,
	"client_secret":               true,
	"client-secret":               true,
	"token":                       true,
	"secret":                      true,
	"password":                    true,
	"cookie":                      true,
	"api_key":                     true,
	"api-key":                     true,
	"access_token":                true,
	"credential":                  true,
}

// sensitiveSubstrings are substrings that mark a key as sensitive.
var sensitiveSubstrings = []string{
	"password", "secret", "token", "credential",
}

// IsSensitiveKey returns true if the key (case-insensitive) refers to a
// credential or secret that must not appear in log files.
func IsSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	if sensitiveKeys[lower] {
		return true
	}
	for _, sub := range sensitiveSubstrings {
		if strings.Contains(lower, sub) {
			return true
		}
	}
	return false
}

// RedactValue replaces a sensitive value with a safe placeholder.
// It preserves the first 4 characters for identification if the value is
// long enough, otherwise fully redacts.
func RedactValue(value string) string {
	if len(value) <= 8 {
		return "***"
	}
	return value[:4] + "***"
}

// TruncateBody returns the body truncated to maxBytes with a UTF-8 safe
// boundary. If truncated, appends a marker showing the original size.
func TruncateBody(body []byte, maxBytes int) string {
	if len(body) <= maxBytes {
		return string(body)
	}
	safe := body[:maxBytes]
	// Walk back to a valid UTF-8 boundary.
	for len(safe) > 0 && !utf8.Valid(safe) {
		safe = safe[:len(safe)-1]
	}
	return fmt.Sprintf("%s...(truncated, total=%d bytes)", string(safe), len(body))
}

// SanitizeArguments returns a JSON string of the arguments map with
// sensitive-looking values replaced by "***". Truncates to maxBytes.
func SanitizeArguments(args map[string]any, maxBytes int) string {
	if len(args) == 0 {
		return "{}"
	}
	sanitized := make(map[string]any, len(args))
	for k, v := range args {
		sanitized[k] = v
	}
	redactMapValues(sanitized)
	data, err := json.Marshal(sanitized)
	if err != nil {
		return "{}"
	}
	return TruncateBody(data, maxBytes)
}

// redactMapValues replaces values of sensitive keys with "***" in-place.
func redactMapValues(m map[string]any) {
	for k, v := range m {
		if IsSensitiveKey(k) {
			m[k] = "***"
			continue
		}
		if nested, ok := v.(map[string]any); ok {
			redactMapValues(nested)
		}
	}
}

// RedactHeaders returns slog attributes for HTTP headers with sensitive
// values redacted.
func RedactHeaders(headers http.Header) []slog.Attr {
	if len(headers) == 0 {
		return nil
	}
	attrs := make([]slog.Attr, 0, len(headers))
	for key := range headers {
		value := headers.Get(key)
		if IsSensitiveKey(key) {
			value = RedactValue(value)
		}
		attrs = append(attrs, slog.String("header."+strings.ToLower(key), value))
	}
	return attrs
}
