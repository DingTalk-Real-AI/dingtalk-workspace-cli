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

// Package validate provides input sanitization and validation functions
// to prevent injection attacks (control characters, CRLF, Unicode spoofing,
// ANSI escape sequences) at the CLI boundary.
package validate

import (
	"fmt"
	"strings"
)

// RejectControlChars rejects C0 control characters (except \t and \n) and
// dangerous Unicode characters from user input.
//
// Control characters cause subtle security issues: null bytes truncate strings
// at the C layer, \r enables HTTP header injection via CRLF.
// Dangerous Unicode characters allow visual spoofing (e.g. making "admin"
// appear as a different string via Bidi overrides).
func RejectControlChars(value, flagName string) error {
	for _, r := range value {
		if r != '\t' && r != '\n' && (r < 0x20 || r == 0x7f) {
			return fmt.Errorf("%s contains invalid control characters", flagName)
		}
		if isDangerousUnicode(r) {
			return fmt.Errorf("%s contains dangerous Unicode characters", flagName)
		}
	}
	return nil
}

// RejectCRLF rejects strings containing carriage return (\r) or line feed (\n).
// These characters enable MIME/HTTP header injection and must never appear in
// header values, filenames, or single-line parameters.
func RejectCRLF(value, fieldName string) error {
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%s contains invalid line break characters", fieldName)
	}
	return nil
}

// StripQueryFragment removes any ?query or #fragment suffix from a URL path.
// API parameters must go through structured flags, not embedded in the path,
// to prevent parameter injection.
func StripQueryFragment(path string) string {
	for i := 0; i < len(path); i++ {
		if path[i] == '?' || path[i] == '#' {
			return path[:i]
		}
	}
	return path
}

// isDangerousUnicode identifies Unicode code points used for visual spoofing.
// These characters are invisible or alter text direction.
func isDangerousUnicode(r rune) bool {
	switch {
	case r >= 0x200B && r <= 0x200D: // zero-width space/non-joiner/joiner
		return true
	case r == 0xFEFF: // BOM / ZWNBSP
		return true
	case r >= 0x202A && r <= 0x202E: // Bidi: LRE/RLE/PDF/LRO/RLO
		return true
	case r >= 0x2028 && r <= 0x2029: // line/paragraph separator
		return true
	case r >= 0x2066 && r <= 0x2069: // Bidi isolates: LRI/RLI/FSI/PDI
		return true
	}
	return false
}
