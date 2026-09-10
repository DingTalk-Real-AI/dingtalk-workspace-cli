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

import "fmt"

// redactingLogger satisfies the vendor SDK's logger interface while making it
// impossible for the SDK to leak secrets into DWS output.
//
// This matters because the vendor SDK logs, at debug level, both the authCode
// it sends to the key server and the raw key-server response body, which
// carries key material. Rather than trying to enumerate and pattern-match
// every sensitive field, we drop every string argument and keep only its
// length. Numeric and boolean arguments pass through, so operators still get
// the useful diagnostics: HTTP status, payload sizes, timings and return
// codes. The format string is preserved so it stays clear which field was
// elided.
type redactingLogger struct {
	logf func(format string, args ...any)
}

// newRedactingLogger returns a logger that forwards to logf, or nil when logf
// is nil so the SDK skips logging entirely.
func newRedactingLogger(logf func(format string, args ...any)) *redactingLogger {
	if logf == nil {
		return nil
	}
	return &redactingLogger{logf: logf}
}

// Debug forwards a redacted debug line.
func (l *redactingLogger) Debug(msg string, args ...interface{}) { l.emit("debug", msg, args) }

// Info forwards a redacted info line.
func (l *redactingLogger) Info(msg string, args ...interface{}) { l.emit("info", msg, args) }

// Error forwards a redacted error line.
func (l *redactingLogger) Error(msg string, args ...interface{}) { l.emit("error", msg, args) }

// emit rewrites args so no string value survives, then forwards the line.
func (l *redactingLogger) emit(level, msg string, args []interface{}) {
	if l == nil || l.logf == nil {
		return
	}
	l.logf("safechat[%s] "+msg, append([]any{level}, redactArgs(args)...)...)
}

// redactArgs replaces every string-like argument with a length marker and
// leaves other kinds intact.
func redactArgs(args []interface{}) []any {
	out := make([]any, 0, len(args))
	for _, arg := range args {
		out = append(out, redactArg(arg))
	}
	return out
}

// redactArg elides a single argument's contents when it could carry a secret.
// Strings and byte slices are reduced to their length; errors are reduced to
// their type so a wrapped body preview cannot slip through; everything else
// (numbers, booleans, durations) is kept because it cannot carry key material.
func redactArg(arg any) any {
	switch v := arg.(type) {
	case string:
		return redactedValue(len(v))
	case []byte:
		return redactedValue(len(v))
	case fmt.Stringer:
		// time.Duration and friends are Stringers, but so are opaque types
		// that may embed a payload. Keep durations, elide the rest.
		if _, ok := arg.(interface{ Nanoseconds() int64 }); ok {
			return v.String()
		}
		return redactedValue(len(v.String()))
	case error:
		return fmt.Sprintf("<%T redacted>", v)
	default:
		return v
	}
}

// redactedValue renders the placeholder used in place of elided content.
func redactedValue(n int) string {
	return fmt.Sprintf("<redacted len=%d>", n)
}
