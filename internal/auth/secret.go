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

package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
)

const (
	// secretKeyPrefix is the keychain account prefix for app secrets.
	secretKeyPrefix = "appsecret:"
)

// SecretRef references a secret stored externally.
type SecretRef struct {
	Source string `json:"source"` // "keychain" | "file"
	ID     string `json:"id"`     // keychain key or file path
}

// SecretInput represents a secret value: either a plain string or a SecretRef object.
type SecretInput struct {
	Plain string     // non-empty for plain string values
	Ref   *SecretRef // non-nil for SecretRef values
}

// PlainSecret creates a SecretInput from a plain string.
func PlainSecret(s string) SecretInput {
	return SecretInput{Plain: s}
}

// IsZero returns true if the SecretInput has no value.
func (s SecretInput) IsZero() bool {
	return s.Plain == "" && s.Ref == nil
}

// IsSecretRef returns true if this is a SecretRef object.
func (s SecretInput) IsSecretRef() bool {
	return s.Ref != nil
}

// IsPlain returns true if this is a plain text string (not a SecretRef).
func (s SecretInput) IsPlain() bool {
	return s.Ref == nil && s.Plain != ""
}

// MarshalJSON serializes SecretInput: plain string → JSON string, SecretRef → JSON object.
func (s SecretInput) MarshalJSON() ([]byte, error) {
	if s.Ref != nil {
		return json.Marshal(s.Ref)
	}
	return json.Marshal(s.Plain)
}

// UnmarshalJSON deserializes SecretInput from either a JSON string or a SecretRef object.
func (s *SecretInput) UnmarshalJSON(data []byte) error {
	// Try string first
	var plain string
	if err := json.Unmarshal(data, &plain); err == nil {
		s.Plain = plain
		s.Ref = nil
		return nil
	}
	// Try SecretRef object
	var ref SecretRef
	if err := json.Unmarshal(data, &ref); err == nil && isValidSource(ref.Source) && ref.ID != "" {
		s.Ref = &ref
		s.Plain = ""
		return nil
	}
	return fmt.Errorf("clientSecret must be a string or {source, id} object")
}

// ValidSecretSources is the set of recognized SecretRef sources.
var ValidSecretSources = map[string]bool{
	"file": true, "keychain": true,
}

func isValidSource(source string) bool {
	return ValidSecretSources[source]
}

// secretAccountKey generates the keychain account key for an app's secret.
func secretAccountKey(clientID string) string {
	return secretKeyPrefix + clientID
}

// ResolveSecret resolves a SecretInput to a plain string.
// SecretRef objects are resolved by source (file / keychain).
func ResolveSecret(input SecretInput) (string, error) {
	if input.Ref == nil {
		return input.Plain, nil
	}
	switch input.Ref.Source {
	case "file":
		data, err := os.ReadFile(input.Ref.ID)
		if err != nil {
			return "", fmt.Errorf("failed to read secret file %s: %w", input.Ref.ID, err)
		}
		return strings.TrimSpace(string(data)), nil
	case "keychain":
		val, err := keychain.Get(keychain.Service, input.Ref.ID)
		if err != nil {
			return "", fmt.Errorf("failed to get secret from keychain: %w", err)
		}
		return val, nil
	default:
		return "", fmt.Errorf("unknown secret source: %s", input.Ref.Source)
	}
}

// StoreSecret stores a plain text secret in keychain and returns a SecretRef.
// If the input is already a SecretRef, it is returned as-is.
// Returns error if keychain is unavailable.
func StoreSecret(clientID string, input SecretInput) (SecretInput, error) {
	if !input.IsPlain() {
		return input, nil // SecretRef → keep as-is
	}
	key := secretAccountKey(clientID)
	if err := keychain.Set(keychain.Service, key, input.Plain); err != nil {
		return SecretInput{}, fmt.Errorf("keychain unavailable: %w\nhint: use file reference in config to bypass keychain", err)
	}
	return SecretInput{Ref: &SecretRef{Source: "keychain", ID: key}}, nil
}

// RemoveSecretStore cleans up keychain entries when an app is removed.
// Errors are intentionally ignored — cleanup is best-effort.
func RemoveSecretStore(input SecretInput) {
	if input.IsSecretRef() && input.Ref.Source == "keychain" {
		_ = keychain.Remove(keychain.Service, input.Ref.ID)
	}
}
