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

package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

const (
	// maxStdinSize limits the amount of data read from stdin or @file
	// to prevent memory exhaustion from accidental large pipes.
	maxStdinSize = 10 * 1024 * 1024 // 10 MB
)

// StdinGuard ensures stdin is consumed at most once per command invocation.
// Multiple flags using @- or implicit stdin fallback would race on the same
// reader; StdinGuard detects and rejects the second claim with a clear error.
type StdinGuard struct {
	mu      sync.Mutex
	claimed bool
	claimBy string
}

// NewStdinGuard creates a fresh guard for one command invocation.
func NewStdinGuard() *StdinGuard {
	return &StdinGuard{}
}

// Claim marks stdin as consumed by the named source (e.g. "--text @-").
// Returns an error if stdin was already claimed.
func (g *StdinGuard) Claim(source string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.claimed {
		return apperrors.NewValidation(fmt.Sprintf(
			"stdin already consumed by %s; cannot also read stdin for %s",
			g.claimBy, source,
		))
	}
	g.claimed = true
	g.claimBy = source
	return nil
}

// Claimed reports whether stdin has been consumed.
func (g *StdinGuard) Claimed() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.claimed
}

// StdinIsPipe reports whether stdin is a pipe (not a terminal).
// This is a non-consuming check — it only inspects file mode via stat.
func StdinIsPipe() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice == 0
}

// ReadStdinIfPiped reads all data from stdin if it is a pipe (not a terminal).
// Returns empty string if stdin is a terminal or has no data.
func ReadStdinIfPiped() (string, error) {
	if !StdinIsPipe() {
		return "", nil
	}
	return readStdinBounded()
}

// ReadStdin reads all data from stdin unconditionally (up to maxStdinSize).
// Use this when the caller has explicitly requested stdin via @-.
func ReadStdin() (string, error) {
	return readStdinBounded()
}

// readFileBounded opens a file and reads up to maxStdinSize bytes.
// Uses io.LimitReader to avoid TOCTOU between stat and read.
func readFileBounded(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, apperrors.NewValidation("@file: " + err.Error())
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxStdinSize+1))
	if err != nil {
		return nil, apperrors.NewValidation("@file: " + err.Error())
	}
	if int64(len(data)) > maxStdinSize {
		return nil, apperrors.NewValidation("@file: file exceeds 10 MB limit")
	}
	return data, nil
}

// readStdinBounded reads from os.Stdin up to maxStdinSize bytes.
func readStdinBounded() (string, error) {
	data, err := io.ReadAll(io.LimitReader(os.Stdin, maxStdinSize+1))
	if err != nil {
		return "", apperrors.NewValidation("failed to read stdin: " + err.Error())
	}
	if int64(len(data)) > maxStdinSize {
		return "", apperrors.NewValidation("stdin input exceeds 10 MB limit")
	}
	return string(data), nil
}

// ReadFileArg reads the contents of a file referenced by the @filename syntax.
// Returns the original value unchanged if it does not start with "@".
// Returns an error if the file cannot be read or exceeds the size limit.
//
// Note: @- (stdin) is NOT handled here; use ResolveInputSource instead.
func ReadFileArg(value string) (string, bool, error) {
	if !strings.HasPrefix(value, "@") {
		return value, false, nil
	}
	path := value[1:]
	if path == "" {
		return "", false, apperrors.NewValidation("@file: filename must not be empty")
	}
	// @- is stdin, not a file — callers should use ResolveInputSource.
	if path == "-" {
		return value, false, nil
	}

	data, err := readFileBounded(path)
	if err != nil {
		return "", false, err
	}
	return string(data), true, nil
}

// ResolveInputSource resolves a flag value that may reference an external
// input source. It supports three forms:
//
//   - "@-"          reads from stdin (requires StdinGuard claim)
//   - "@<path>"     reads from the named file
//   - anything else returned unchanged
//
// The flagName parameter is used only for error messages and StdinGuard tracking.
func ResolveInputSource(value string, flagName string, guard *StdinGuard) (string, error) {
	if !strings.HasPrefix(value, "@") {
		return value, nil
	}

	path := value[1:]
	if path == "" {
		return "", apperrors.NewValidation(fmt.Sprintf("--%s: @file filename must not be empty", flagName))
	}

	// @- reads from stdin.
	if path == "-" {
		if guard == nil {
			return "", apperrors.NewValidation(fmt.Sprintf("--%s: stdin (@-) not available in this context", flagName))
		}
		if err := guard.Claim(fmt.Sprintf("--%s @-", flagName)); err != nil {
			return "", err
		}
		return ReadStdin()
	}

	// @<path> reads from file.
	data, err := readFileBounded(path)
	if err != nil {
		return "", fmt.Errorf("--%s: %w", flagName, err)
	}
	return string(data), nil
}
