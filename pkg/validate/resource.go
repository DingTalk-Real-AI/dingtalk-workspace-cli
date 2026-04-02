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

package validate

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// unsafeResourceChars matches URL-special characters, control characters,
// and percent signs (to prevent %2e%2e encoding bypass).
var unsafeResourceChars = regexp.MustCompile(`[?#%\x00-\x1f\x7f]`)

// ResourceName validates an API resource identifier (userId, taskId, etc.)
// before it is interpolated into a URL path. It rejects path traversal (..),
// URL metacharacters (?#%), percent-encoded bypasses, control characters,
// and dangerous Unicode.
//
// Without this check, an input like "../admin" or "?evil=true" in a resource ID
// would alter the API endpoint.
func ResourceName(name, flagName string) error {
	if name == "" {
		return fmt.Errorf("%s must not be empty", flagName)
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == ".." {
			return fmt.Errorf("%s must not contain '..' path traversal", flagName)
		}
	}
	if unsafeResourceChars.MatchString(name) {
		return fmt.Errorf("%s contains invalid characters", flagName)
	}
	for _, r := range name {
		if isDangerousUnicode(r) {
			return fmt.Errorf("%s contains dangerous Unicode characters", flagName)
		}
	}
	return nil
}

// EncodePathSegment percent-encodes user input for safe use as a single URL
// path segment (e.g. / → %2F, ? → %3F), ensuring the value cannot alter URL
// routing when interpolated into an API path.
func EncodePathSegment(s string) string {
	return url.PathEscape(s)
}
