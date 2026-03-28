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
	"io"
	"os"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

const (
	// maxStdinSize limits the amount of data read from stdin or @file
	// to prevent memory exhaustion from accidental large pipes.
	maxStdinSize = 10 * 1024 * 1024 // 10 MB
)

// ReadStdinIfPiped reads all data from stdin if it is a pipe (not a terminal).
// Returns empty string if stdin is a terminal or has no data.
func ReadStdinIfPiped() (string, error) {
	info, err := os.Stdin.Stat()
	if err != nil {
		return "", nil
	}
	// Check if stdin is a pipe (not a character device / terminal).
	if info.Mode()&os.ModeCharDevice != 0 {
		return "", nil
	}
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
func ReadFileArg(value string) (string, bool, error) {
	if !strings.HasPrefix(value, "@") {
		return value, false, nil
	}
	path := value[1:]
	if path == "" {
		return "", false, apperrors.NewValidation("@file: filename must not be empty")
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", false, apperrors.NewValidation("@file: " + err.Error())
	}
	if info.Size() > maxStdinSize {
		return "", false, apperrors.NewValidation("@file: file exceeds 10 MB limit")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", false, apperrors.NewValidation("@file: " + err.Error())
	}
	return string(data), true, nil
}
