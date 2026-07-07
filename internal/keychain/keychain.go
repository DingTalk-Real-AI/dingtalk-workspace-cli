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

// Package keychain provides cross-platform secure storage for secrets.
// - macOS: System Keychain stores DEK (Data Encryption Key), data encrypted with AES-256-GCM
// - Linux: File-based DEK storage with AES-256-GCM encryption
// - Windows: DPAPI + Registry storage
package keychain

import "errors"

const (
	// Service is the unified keychain service name for all secrets.
	Service = "dws-cli"

	// AccountToken is the account key for storing auth token data.
	AccountToken = "auth-token"

	// StorageDirEnv overrides the on-disk keychain storage root on
	// platforms that use file-backed storage (macOS, Linux). It is
	// intended for tests that need to isolate keychain state from the
	// real user environment and from sibling test packages running in
	// parallel. When empty, the platform default applies.
	StorageDirEnv = "DWS_KEYCHAIN_DIR"

	// DisableKeychainEnv opts the macOS implementation out of system
	// Keychain access for the DEK, falling back to a file-based DEK
	// (same scheme as Linux). Intended for sandboxed runtimes where
	// Keychain APIs are blocked (e.g. Codex App). This weakens the
	// at-rest protection — DEK and ciphertext live in the same
	// directory — and is therefore opt-in.
	DisableKeychainEnv = "DWS_DISABLE_KEYCHAIN"
)

// ErrDEKMissing means encrypted local data may exist, but the Data Encryption
// Key needed to decrypt it is missing. Read paths must not create a new DEK,
// because a fresh key cannot decrypt existing ciphertext.
var ErrDEKMissing = errors.New("dek missing")

// KeychainAccess abstracts keychain Get/Set/Remove for dependency injection.
type KeychainAccess interface {
	Get(service, account string) (string, error)
	Set(service, account, value string) error
	Remove(service, account string) error
}

// Diagnostic is a read-only health report for the platform keychain backend.
// It never mutates credentials, DEKs, or OS keychain settings.
type Diagnostic struct {
	OK      bool              `json:"ok"`
	Reason  string            `json:"reason,omitempty"`
	Message string            `json:"message"`
	Hint    string            `json:"hint,omitempty"`
	Detail  map[string]string `json:"detail,omitempty"`
}

// UnavailableError marks failures where the platform keychain itself could
// not be reached, unlocked, or created. Callers can surface a diagnostic
// instead of treating the result as a normal missing credential.
type UnavailableError struct {
	Op  string
	Err error
}

func NewUnavailableError(op string, err error) error {
	return &UnavailableError{Op: op, Err: err}
}

func (e *UnavailableError) Error() string {
	if e == nil {
		return ""
	}
	if e.Op == "" {
		if e.Err != nil {
			return e.Err.Error()
		}
		return "keychain unavailable"
	}
	if e.Err == nil {
		return e.Op + ": keychain unavailable"
	}
	return e.Op + ": " + e.Err.Error()
}

func (e *UnavailableError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func IsUnavailable(err error) bool {
	var unavailable *UnavailableError
	return errors.As(err, &unavailable)
}

func IsDEKMissing(err error) bool {
	return errors.Is(err, ErrDEKMissing)
}

func Diagnose() Diagnostic {
	return platformDiagnose()
}

// Get retrieves a value from the keychain.
// Returns empty string and nil error if the entry does not exist.
func Get(service, account string) (string, error) {
	return platformGet(service, account)
}

// Set stores a value in the keychain, overwriting any existing entry.
func Set(service, account, data string) error {
	return platformSet(service, account, data)
}

// Remove deletes an entry from the keychain.
// Returns nil if the entry does not exist.
func Remove(service, account string) error {
	return platformRemove(service, account)
}

// Exists checks if an entry exists in the keychain.
func Exists(service, account string) bool {
	val, err := Get(service, account)
	return err == nil && val != ""
}
