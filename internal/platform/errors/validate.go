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

package errors

import (
	stderrors "errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

var (
	ErrInvalidResourceName = stderrors.New("invalid resource name")
	ErrUnsafePath          = stderrors.New("unsafe path detected")
)

func ResourceName(name string) error {
	if name == "" {
		return stderrors.New("resource name cannot be empty")
	}
	if len([]rune(name)) > 128 {
		return stderrors.New("resource name too long (max 128 characters)")
	}
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			continue
		}
		return ErrInvalidResourceName
	}
	if unicode.IsDigit([]rune(name)[0]) {
		return ErrInvalidResourceName
	}
	return nil
}

// isDangerousUnicode identifies Unicode code points used for visual spoofing attacks.
// These characters are invisible or alter text direction, allowing attackers to make
// "report.exe" display as "report.txt" (Bidi override) or insert hidden content
// (zero-width characters).
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

// rejectControlChars rejects control characters in a string.
// Rejects C0 control characters (except \t and \n) and dangerous Unicode.
// Tab and newline are allowed as they may appear in legitimate multi-line input.
func rejectControlChars(s, fieldName string) error {
	for _, r := range s {
		// Allow tab (\t = 0x09) and newline (\n = 0x0A)
		if r != '\t' && r != '\n' && (r < 0x20 || r == 0x7f) {
			return fmt.Errorf("%s contains control characters", fieldName)
		}
		if isDangerousUnicode(r) {
			return fmt.Errorf("%s contains dangerous Unicode characters", fieldName)
		}
	}
	return nil
}

// RejectControlChars rejects C0 control characters (except \t and \n) and
// dangerous Unicode characters from user input.
//
// Control characters cause subtle security issues:
//   - Null bytes truncate strings at the C layer
//   - \r\n enables HTTP header injection
//   - Unicode Bidi characters allow visual spoofing (e.g. making "report.exe" display as "report.txt")
//
// Tab and newline are allowed as they may appear in legitimate multi-line input.
func RejectControlChars(value, flagName string) error {
	return rejectControlChars(value, flagName)
}

// SafePath performs basic path validation checking for dangerous patterns.
// For full security (symlink resolution, CWD containment), use SafeOutputPath or SafeInputPath.
func SafePath(path string) error {
	if path == "" {
		return stderrors.New("path cannot be empty")
	}

	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return ErrUnsafePath
	}
	if strings.ContainsRune(path, '\x00') {
		return ErrUnsafePath
	}

	// Check for dangerous Unicode
	for _, r := range path {
		if isDangerousUnicode(r) {
			return ErrUnsafePath
		}
	}

	lowerPath := strings.ToLower(path)
	for _, pattern := range []string{
		"..",
		"~",
		"$(",
		"`",
		"|",
		";",
		"&",
		"<",
		">",
		"\n",
		"\r",
	} {
		if strings.Contains(lowerPath, pattern) {
			return ErrUnsafePath
		}
	}
	return nil
}

// SafeOutputPath validates a download/export target path for --output flags.
// It rejects absolute paths, resolves symlinks to their real location, and
// verifies the canonical result is still under the current working directory.
// This prevents an AI Agent from being tricked into writing files outside the
// working directory (e.g. "../../.ssh/authorized_keys") or following symlinks
// to sensitive locations.
//
// The returned absolute path MUST be used for all subsequent I/O to prevent
// time-of-check-to-time-of-use (TOCTOU) race conditions.
func SafeOutputPath(path string) (string, error) {
	return safePath(path, "--output")
}

// SafeInputPath validates an upload/read source path for --file flags.
// It applies the same rules as SafeOutputPath — rejecting absolute paths,
// resolving symlinks, and enforcing working directory containment — to prevent
// an AI Agent from being tricked into reading sensitive files like /etc/passwd.
func SafeInputPath(path string) (string, error) {
	return safePath(path, "--file")
}

// safePath is the shared implementation for SafeOutputPath and SafeInputPath.
func safePath(raw, flagName string) (string, error) {
	if err := rejectControlChars(raw, flagName); err != nil {
		return "", err
	}

	path := filepath.Clean(raw)

	// Reject absolute paths - force relative paths within CWD
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("%s must be a relative path within the current directory, got %q (hint: cd to the target directory first, or use a relative path like ./filename)", flagName, raw)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot determine working directory: %w", err)
	}
	resolved := filepath.Join(cwd, path)

	// Resolve symlinks: for existing paths, follow to real location;
	// for non-existing paths, walk up to the nearest existing ancestor,
	// resolve its symlinks, and re-attach the remaining tail segments.
	// This prevents TOCTOU attacks where a non-existent intermediate
	// directory is replaced with a symlink between check and use.
	if _, err := os.Lstat(resolved); err == nil {
		resolved, err = filepath.EvalSymlinks(resolved)
		if err != nil {
			return "", fmt.Errorf("cannot resolve symlinks: %w", err)
		}
	} else {
		resolved, err = resolveNearestAncestor(resolved)
		if err != nil {
			return "", fmt.Errorf("cannot resolve symlinks: %w", err)
		}
	}

	canonicalCwd, _ := filepath.EvalSymlinks(cwd)
	if !isUnderDir(resolved, canonicalCwd) {
		return "", fmt.Errorf("%s %q resolves outside the current working directory (hint: the path must stay within the working directory after resolving .. and symlinks)", flagName, raw)
	}

	return resolved, nil
}

// resolveNearestAncestor walks up from path until it finds an existing
// ancestor, resolves that ancestor's symlinks, and re-joins the tail.
// This ensures even deeply nested non-existent paths are anchored to a
// real filesystem location, closing the TOCTOU symlink gap.
func resolveNearestAncestor(path string) (string, error) {
	var tail []string
	cur := path
	for {
		if _, err := os.Lstat(cur); err == nil {
			real, err := filepath.EvalSymlinks(cur)
			if err != nil {
				return "", err
			}
			parts := append([]string{real}, tail...)
			return filepath.Join(parts...), nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			// Reached filesystem root without finding an existing ancestor;
			// return path as-is and let the containment check reject it.
			parts := append([]string{cur}, tail...)
			return filepath.Join(parts...), nil
		}
		tail = append([]string{filepath.Base(cur)}, tail...)
		cur = parent
	}
}

// isUnderDir checks whether child is under parent directory.
func isUnderDir(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}

// SafeLocalFlagPath validates a flag value as a local file path.
// Empty values and http/https URLs are returned unchanged without validation,
// allowing the caller to handle non-path inputs (e.g. API keys, URLs) upstream.
// For all other values, SafeInputPath rules apply.
// The original relative path is returned unchanged (not resolved to absolute) so
// upload helpers can re-validate at the actual I/O point via SafeUploadPath.
func SafeLocalFlagPath(flagName, value string) (string, error) {
	if value == "" || strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value, nil
	}
	if _, err := SafeInputPath(value); err != nil {
		return "", fmt.Errorf("%s: %v", flagName, err)
	}
	return value, nil
}

// RejectCRLF rejects strings containing carriage return (\r) or line feed (\n).
// These characters enable MIME/HTTP header injection and must never appear in
// header field names, values, Content-ID, or filename parameters.
func RejectCRLF(value, fieldName string) error {
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%s contains invalid line break characters", fieldName)
	}
	return nil
}

// StripQueryFragment removes any ?query or #fragment suffix from a URL path.
// API parameters must go through structured --params flags, not embedded in
// the path, to prevent parameter injection and behaviour confusion.
func StripQueryFragment(path string) string {
	for i := 0; i < len(path); i++ {
		if path[i] == '?' || path[i] == '#' {
			return path[:i]
		}
	}
	return path
}
