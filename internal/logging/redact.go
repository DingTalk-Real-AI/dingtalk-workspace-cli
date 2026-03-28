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

import "strings"

// sensitiveKeys are header/field names whose values must be redacted in logs.
var sensitiveKeys = map[string]bool{
	"authorization":       true,
	"x-user-access-token": true,
	"client_secret":       true,
	"client-secret":       true,
	"token":               true,
	"secret":              true,
	"password":            true,
	"cookie":              true,
}

// IsSensitiveKey returns true if the key (case-insensitive) refers to a
// credential or secret that must not appear in log files.
func IsSensitiveKey(key string) bool {
	return sensitiveKeys[strings.ToLower(key)]
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
