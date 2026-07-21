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

//go:build windows

package keychain

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var registryDeleteValue = func(key registry.Key, name string) error {
	return key.DeleteValue(name)
}

var registryOpenDeleteKey = func(path string, access uint32) (registry.Key, error) {
	return registry.OpenKey(registry.CURRENT_USER, path, access)
}

// ---------------------------------------------------------------------------
// Windows backend: DPAPI + HKCU registry
// ---------------------------------------------------------------------------

const regRootPath = `Software\DwsCli\keychain`

// StorageDir returns the storage directory for a given service name on Windows.
// The Windows keychain backend keeps secrets in DPAPI-protected HKCU registry
// values rather than on disk, so this path is used only by the portable
// auth-bundle export/import (internal/auth) to colocate config. When the
// DWS_KEYCHAIN_DIR environment variable is set, the storage root is taken from
// that env var; otherwise it defaults to %LocalAppData%\<service>.
func StorageDir(service string) string {
	if override := os.Getenv(StorageDirEnv); override != "" {
		return filepath.Join(override, service)
	}
	if local := os.Getenv("LocalAppData"); local != "" {
		return filepath.Join(local, service)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		fmt.Fprintf(os.Stderr, "warning: unable to determine home directory: %v\n", err)
		return filepath.Join(".dws", "keychain", service)
	}
	return filepath.Join(home, "AppData", "Local", service)
}

func registryPathForService(service string) string {
	path := regRootPath + `\` + safeRegistryComponent(service)
	namespace := strings.TrimSpace(os.Getenv(TestNamespaceEnv))
	if namespace == "" {
		return path
	}

	// Windows stores credentials in HKCU instead of DWS_KEYCHAIN_DIR. Tests set
	// an explicit process namespace so concurrent package binaries cannot
	// delete each other's credentials. Hash it to avoid leaking temp paths or
	// introducing registry separators.
	namespace = filepath.Clean(namespace)
	if absolute, err := filepath.Abs(namespace); err == nil {
		namespace = absolute
	}
	sum := sha256.Sum256([]byte(strings.ToLower(namespace)))
	return fmt.Sprintf(`%s\test-%x`, path, sum[:16])
}

var safeRegRe = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

func safeRegistryComponent(s string) string {
	// Registry key path uses '\\' separators; avoid accidental nesting and odd chars.
	s = strings.ReplaceAll(s, "\\", "_")
	return safeRegRe.ReplaceAllString(s, "_")
}

func valueNameForAccount(account string) string {
	// Avoid any special characters; keep deterministic.
	return base64.RawURLEncoding.EncodeToString([]byte(account))
}

func dpapiEntropy(service, account string) *windows.DataBlob {
	// Bind ciphertext to (service, account) to reduce swap/replay risks.
	data := []byte(service + "\x00" + account)
	if len(data) == 0 {
		return nil
	}
	return &windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
}

func dpapiProtect(plaintext []byte, entropy *windows.DataBlob) ([]byte, error) {
	var in windows.DataBlob
	if len(plaintext) > 0 {
		in = windows.DataBlob{Size: uint32(len(plaintext)), Data: &plaintext[0]}
	}
	var out windows.DataBlob
	err := windows.CryptProtectData(&in, nil, entropy, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	if err != nil {
		return nil, err
	}
	defer freeDataBlob(&out)

	if out.Data == nil || out.Size == 0 {
		return []byte{}, nil
	}
	buf := unsafe.Slice(out.Data, int(out.Size))
	res := make([]byte, len(buf))
	copy(res, buf)
	return res, nil
}

func dpapiUnprotect(ciphertext []byte, entropy *windows.DataBlob) ([]byte, error) {
	var in windows.DataBlob
	if len(ciphertext) > 0 {
		in = windows.DataBlob{Size: uint32(len(ciphertext)), Data: &ciphertext[0]}
	}
	var out windows.DataBlob
	err := windows.CryptUnprotectData(&in, nil, entropy, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	if err != nil {
		return nil, err
	}
	defer freeDataBlob(&out)

	if out.Data == nil || out.Size == 0 {
		return []byte{}, nil
	}
	buf := unsafe.Slice(out.Data, int(out.Size))
	res := make([]byte, len(buf))
	copy(res, buf)
	return res, nil
}

func freeDataBlob(b *windows.DataBlob) {
	if b == nil || b.Data == nil {
		return
	}
	// Per DPAPI contract, output buffers must be freed with LocalFree.
	_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(b.Data)))
	b.Data = nil
	b.Size = 0
}

func platformGet(service, account string) (string, error) {
	v, ok := registryGet(service, account)
	if !ok {
		return "", nil // Not found is not an error
	}
	return v, nil
}

func platformSet(service, account, data string) error {
	entropy := dpapiEntropy(service, account)
	protected, err := dpapiProtect([]byte(data), entropy)
	if err != nil {
		return fmt.Errorf("dpapi protect failed: %w", err)
	}
	return registrySet(service, account, protected)
}

func platformRemove(service, account string) error {
	return registryRemove(service, account)
}

func registryGet(service, account string) (string, bool) {
	keyPath := registryPathForService(service)
	k, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE)
	if err != nil {
		return "", false
	}
	defer k.Close()

	b64, _, err := k.GetStringValue(valueNameForAccount(account))
	if err != nil || b64 == "" {
		return "", false
	}
	blob, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", false
	}
	entropy := dpapiEntropy(service, account)
	plain, err := dpapiUnprotect(blob, entropy)
	if err != nil {
		return "", false
	}
	return string(plain), true
}

func registrySet(service, account string, protected []byte) error {
	keyPath := registryPathForService(service)
	k, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("registry create/open failed: %w", err)
	}
	defer k.Close()

	b64 := base64.StdEncoding.EncodeToString(protected)
	if err := k.SetStringValue(valueNameForAccount(account), b64); err != nil {
		return fmt.Errorf("registry set failed: %w", err)
	}
	return nil
}

func registryRemove(service, account string) error {
	keyPath := registryPathForService(service)
	k, err := registryOpenDeleteKey(keyPath, registry.SET_VALUE)
	if err != nil {
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
			return nil
		}
		return fmt.Errorf("registry open for delete failed: %w", err)
	}
	defer k.Close()
	return deleteRegistryValue(k, valueNameForAccount(account))
}

func deleteRegistryValue(key registry.Key, name string) error {
	if err := registryDeleteValue(key, name); err != nil {
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
			return nil
		}
		return fmt.Errorf("registry delete failed: %w", err)
	}
	return nil
}

func registryRemoveAuthTokenEntries(service string) error {
	keyPath := registryPathForService(service)
	k, err := registryOpenDeleteKey(keyPath, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
			return nil
		}
		return fmt.Errorf("registry open for auth token cleanup failed: %w", err)
	}
	defer k.Close()

	names, err := k.ReadValueNames(-1)
	if err != nil {
		return fmt.Errorf("registry list values failed: %w", err)
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
		if err := k.DeleteValue(name); err != nil {
			return fmt.Errorf("registry delete auth token value failed: %w", err)
		}
	}
	return nil
}
