// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build windows

package keychain

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sys/windows/registry"
)

var registryReadValueNames = registry.Key.ReadValueNames

// Windows stores token entries in the DPAPI-protected user registry. Enumerate
// every auth-token account so orphaned slots that are not present in
// profiles.json are still validated before a login can overwrite credentials.
func platformValidateAuthTokenEntries(service string) error {
	keyPath := registryPathForService(service)
	k, err := registryOpenReadKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("registry open for auth token validation failed: %w", err)
	}
	defer k.Close()

	names, err := registryReadValueNames(k, -1)
	if err != nil {
		return fmt.Errorf("registry list values for auth token validation failed: %w", err)
	}
	for _, name := range names {
		accountBytes, decodeErr := base64.RawURLEncoding.DecodeString(name)
		if decodeErr != nil {
			continue
		}
		account := string(accountBytes)
		if account != AccountToken && !strings.HasPrefix(account, AccountToken+":") {
			continue
		}
		if _, found, err := registryGetFromKey(k, service, account); err != nil {
			return fmt.Errorf("validate registry auth token account %q: %w", account, err)
		} else if !found {
			// The value was returned by ReadValueNames but disappeared before it
			// could be read. Fail closed so a concurrent mutation cannot make the
			// preflight silently skip an existing credential slot.
			return fmt.Errorf("validate registry auth token account %q: value disappeared during validation", account)
		}
	}
	return nil
}

func platformRemoveAuthTokenEntries(service string) error {
	return registryRemoveAuthTokenEntries(service)
}
