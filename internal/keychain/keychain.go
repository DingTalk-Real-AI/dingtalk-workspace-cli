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
)

// KeychainAccess abstracts keychain Get/Set/Remove for dependency injection.
type KeychainAccess interface {
	Get(service, account string) (string, error)
	Set(service, account, value string) error
	Remove(service, account string) error
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
