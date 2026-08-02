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

// Package agentproduct defines the caller-provided Agent product identity
// used for outbound observability and IM message display.
package agentproduct

import (
	"errors"
	"os"
	"regexp"
	"strings"
)

const (
	// EnvName is the runtime override for the Agent product identity.
	EnvName = "DWS_AGENT_PRODUCT"
	// HeaderName is the observability header used for the Agent product.
	// It is intentionally separate from the routing/PAT claw-type header.
	HeaderName = "x-dws-agent-product"
	// MaxValueBytes bounds the value because it is attached to every outbound
	// MCP request. Supported values are ASCII, so bytes and characters match.
	MaxValueBytes = 64
)

// ErrInvalid is returned when an Agent product value is unsafe or does not
// match the supported wire format. The error intentionally excludes the raw
// caller-controlled value.
var ErrInvalid = errors.New("invalid agent product")

var valuePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// Parse normalizes and validates a caller-provided Agent product. Only
// surrounding ASCII spaces and tabs are trimmed; other control or Unicode
// whitespace remains visible to validation and is rejected. An unset or
// ASCII-whitespace-only value means "use the edition default".
func Parse(raw string) (string, error) {
	if strings.ContainsAny(raw, "\r\n") {
		return "", ErrInvalid
	}

	value := strings.Trim(raw, " \t")
	if value == "" {
		return "", nil
	}
	if len(value) > MaxValueBytes {
		return "", ErrInvalid
	}
	if !valuePattern.MatchString(value) {
		return "", ErrInvalid
	}
	return value, nil
}

// ResolveFromEnv returns the validated DWS_AGENT_PRODUCT value, or fallback
// when the environment variable is unset or empty. Invalid caller input is
// returned as an error so command entrypoints can fail before network access.
func ResolveFromEnv(fallback string) (string, error) {
	value, err := Parse(os.Getenv(EnvName))
	if err != nil {
		return "", err
	}
	if value == "" {
		return fallback, nil
	}
	return value, nil
}
