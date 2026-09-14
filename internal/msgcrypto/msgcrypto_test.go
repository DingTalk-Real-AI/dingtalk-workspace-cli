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

package msgcrypto

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeCipher stands in for a backend so the wrapper logic can be tested on
// every platform, with or without the safechat tag.
type fakeCipher struct {
	mu         sync.Mutex
	encrypted  [][]byte
	decrypted  [][]byte
	closeCount int
	closeErr   error
}

type fakeFileInfo struct {
	mode  fs.FileMode
	isDir bool
}

func (f fakeFileInfo) Name() string       { return "fake" }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() fs.FileMode  { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.isDir }
func (f fakeFileInfo) Sys() any           { return nil }

func (f *fakeCipher) EncryptMessage(_ context.Context, _, _ string, plaintext []byte) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.encrypted = append(f.encrypted, plaintext)
	return []byte("cipher:" + string(plaintext)), nil
}

func (f *fakeCipher) DecryptMessage(_ context.Context, _, _ string, ciphertext []byte) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.decrypted = append(f.decrypted, ciphertext)
	return []byte(strings.TrimPrefix(string(ciphertext), "cipher:")), nil
}

func (f *fakeCipher) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closeCount++
	return f.closeErr
}

// newTrackedForTest wraps backend and claims the process slot the same way
// Open does, so slot release can be asserted.
func newTrackedForTest(t *testing.T, backend Cipher) *trackedCipher {
	t.Helper()
	process.mu.Lock()
	process.open = true
	process.mu.Unlock()
	t.Cleanup(func() {
		process.mu.Lock()
		process.open = false
		process.mu.Unlock()
	})
	return &trackedCipher{backend: backend}
}

func TestCrossPlatformCoverageConfigWithDefaultsFillsKeystoreDir(t *testing.T) {
	cfg := Config{}.withDefaults()
	if cfg.KeystoreDir == "" {
		t.Fatal("withDefaults left KeystoreDir empty")
	}
	if want := DefaultKeystoreDir(); cfg.KeystoreDir != want {
		t.Fatalf("KeystoreDir = %q, want %q", cfg.KeystoreDir, want)
	}
}

func TestCrossPlatformCoverageConfigWithDefaultsKeepsExplicitKeystoreDir(t *testing.T) {
	cfg := Config{KeystoreDir: "/custom/keys"}.withDefaults()
	if cfg.KeystoreDir != "/custom/keys" {
		t.Fatalf("KeystoreDir = %q, want /custom/keys", cfg.KeystoreDir)
	}
}

func TestCrossPlatformCoverageDefaultKeystoreDirHonoursConfigDirOverride(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	got := DefaultKeystoreDir()
	if !strings.HasSuffix(got, filepath.Join("safechat", "keystore")) {
		t.Fatalf("DefaultKeystoreDir() = %q, want it to end with safechat/keystore", got)
	}
	if !strings.HasPrefix(got, os.Getenv("DWS_CONFIG_DIR")) {
		t.Fatalf("DefaultKeystoreDir() = %q, want it under DWS_CONFIG_DIR", got)
	}
}

func TestCrossPlatformCoverageConfigValidateRequiresAuthCodeProvider(t *testing.T) {
	err := Config{KeystoreDir: "/tmp/keys"}.validate()
	if !errors.Is(err, ErrNoAuthCodeProvider) {
		t.Fatalf("validate() = %v, want ErrNoAuthCodeProvider", err)
	}
}

func TestCrossPlatformCoverageConfigValidateRequiresResolvedKeystoreDir(t *testing.T) {
	err := Config{KeystoreDir: "", AuthCode: StaticAuthCode("code"), KeyServer: "https://key.example.test"}.validate()
	if err == nil || !strings.Contains(err.Error(), "KeystoreDir") {
		t.Fatalf("validate() = %v, want empty KeystoreDir error", err)
	}
}

func TestCrossPlatformCoverageConfigValidateAcceptsCompleteConfig(t *testing.T) {
	cfg := Config{
		KeystoreDir: "/tmp/keys",
		AuthCode:    StaticAuthCode("code"),
		KeyServer:   "https://key.example.test",
	}
	if err := cfg.validate(); err != nil {
		t.Fatalf("validate() = %v, want nil", err)
	}
}

func TestCrossPlatformCoverageConfigValidateRequiresKeyServer(t *testing.T) {
	err := Config{KeystoreDir: "/tmp/keys", AuthCode: StaticAuthCode("code")}.validate()
	if !errors.Is(err, ErrNoKeyServer) {
		t.Fatalf("validate() = %v, want ErrNoKeyServer", err)
	}
}

func TestCrossPlatformCoverageConfigValidateRejectsHTTPKeyServer(t *testing.T) {
	err := Config{
		KeystoreDir: "/tmp/keys",
		AuthCode:    StaticAuthCode("code"),
		KeyServer:   "http://key.example.test",
	}.validate()
	if !errors.Is(err, ErrKeyServerNotHTTPS) {
		t.Fatalf("validate() = %v, want ErrKeyServerNotHTTPS", err)
	}
}

func TestCrossPlatformCoveragePrepareKeystoreCreatesOwnerOnlyDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "keystore")
	if err := prepareKeystore(dir); err != nil {
		t.Fatalf("prepareKeystore() = %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("prepareKeystore did not create a directory")
	}
	if runtime.GOOS == "windows" {
		return
	}
	if got := info.Mode().Perm(); got != keystoreDirPerm {
		t.Fatalf("perm = %#o, want %#o", got, keystoreDirPerm)
	}
}

func TestCrossPlatformCoveragePrepareKeystoreTightensLoosePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not modelled on Windows")
	}
	dir := filepath.Join(t.TempDir(), "keystore")
	if err := os.MkdirAll(dir, fs.FileMode(0o755)); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := prepareKeystore(dir); err != nil {
		t.Fatalf("prepareKeystore() = %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != keystoreDirPerm {
		t.Fatalf("perm = %#o, want %#o (key material must stay owner-only)", got, keystoreDirPerm)
	}
}

func TestCrossPlatformCoveragePrepareKeystoreRejectsFilePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keystore")
	if err := os.WriteFile(path, []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	err := prepareKeystore(path)
	if err == nil {
		t.Fatal("prepareKeystore() = nil, want an error for a non-directory path")
	}
}

func TestCrossPlatformCoveragePrepareKeystoreReportsStatFailure(t *testing.T) {
	oldStat := statKeystore
	t.Cleanup(func() { statKeystore = oldStat })
	wantErr := errors.New("stat failed")
	statKeystore = func(string) (os.FileInfo, error) {
		return nil, wantErr
	}

	err := prepareKeystore(filepath.Join(t.TempDir(), "keystore"))
	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "stat keystore dir") {
		t.Fatalf("prepareKeystore() = %v, want stat failure", err)
	}
}

func TestCrossPlatformCoveragePrepareKeystoreRejectsNonDirectoryStatResult(t *testing.T) {
	oldStat := statKeystore
	t.Cleanup(func() { statKeystore = oldStat })
	statKeystore = func(string) (os.FileInfo, error) {
		return fakeFileInfo{mode: 0o600, isDir: false}, nil
	}

	err := prepareKeystore(filepath.Join(t.TempDir(), "keystore"))
	if err == nil || !strings.Contains(err.Error(), "is not a directory") {
		t.Fatalf("prepareKeystore() = %v, want non-directory error", err)
	}
}

func TestCrossPlatformCoverageOpenRejectsMissingAuthCodeProviderBeforeTouchingDisk(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "keystore")
	_, err := Open(context.Background(), Config{KeystoreDir: dir})
	if !errors.Is(err, ErrNoAuthCodeProvider) {
		t.Fatalf("Open() = %v, want ErrNoAuthCodeProvider", err)
	}
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Fatal("Open created the keystore dir despite an invalid config")
	}
}

func TestCrossPlatformCoverageOpenRejectsMissingKeyServerBeforeTouchingDisk(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "keystore")
	_, err := Open(context.Background(), Config{
		KeystoreDir: dir,
		AuthCode:    StaticAuthCode("code"),
	})
	if !errors.Is(err, ErrNoKeyServer) {
		t.Fatalf("Open() = %v, want ErrNoKeyServer", err)
	}
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Fatal("Open created the keystore dir despite a missing KeyServer")
	}
}

func TestCrossPlatformCoverageOpenWithoutBackendReportsUnavailable(t *testing.T) {
	if Available() {
		t.Skip("this binary has the safechat backend compiled in")
	}
	_, err := Open(context.Background(), Config{
		KeystoreDir: filepath.Join(t.TempDir(), "keystore"),
		AuthCode:    StaticAuthCode("code"),
		KeyServer:   "https://key.example.test",
	})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Open() = %v, want ErrUnavailable", err)
	}
}

func TestCrossPlatformCoverageOpenBackendSeamEdges(t *testing.T) {
	oldAvailable := backendAvailable
	oldBackend := openBackend
	t.Cleanup(func() {
		backendAvailable = oldAvailable
		openBackend = oldBackend
		process.mu.Lock()
		process.open = false
		process.mu.Unlock()
	})

	backendAvailable = func() bool { return true }
	wantErr := errors.New("backend failed")
	openBackend = func(context.Context, Config) (Cipher, error) {
		return nil, wantErr
	}
	_, err := Open(context.Background(), Config{
		KeystoreDir: filepath.Join(t.TempDir(), "keystore"),
		AuthCode:    StaticAuthCode("code"),
		KeyServer:   "https://key.example.test",
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Open() backend error = %v, want %v", err, wantErr)
	}

	backend := &fakeCipher{}
	openBackend = func(context.Context, Config) (Cipher, error) {
		return backend, nil
	}
	cipher, err := Open(context.Background(), Config{
		KeystoreDir: filepath.Join(t.TempDir(), "keystore"),
		AuthCode:    StaticAuthCode("code"),
		KeyServer:   "https://key.example.test",
	})
	if err != nil {
		t.Fatalf("Open() success = %v", err)
	}
	if _, err := Open(context.Background(), Config{
		KeystoreDir: filepath.Join(t.TempDir(), "keystore"),
		AuthCode:    StaticAuthCode("code"),
		KeyServer:   "https://key.example.test",
	}); !errors.Is(err, ErrAlreadyOpen) {
		t.Fatalf("second Open() = %v, want ErrAlreadyOpen", err)
	}
	if err := cipher.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
}

func TestCrossPlatformCoverageOpenReportsKeystorePrepareFailure(t *testing.T) {
	oldAvailable := backendAvailable
	oldBackend := openBackend
	t.Cleanup(func() {
		backendAvailable = oldAvailable
		openBackend = oldBackend
	})

	backendAvailable = func() bool { return true }
	openBackend = func(context.Context, Config) (Cipher, error) {
		t.Fatal("backend should not be opened after keystore preparation fails")
		return nil, nil
	}

	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := Open(context.Background(), Config{
		KeystoreDir: filepath.Join(file, "child"),
		AuthCode:    StaticAuthCode("code"),
		KeyServer:   "https://key.example.test",
	})
	if err == nil || !strings.Contains(err.Error(), "create keystore dir") {
		t.Fatalf("Open() = %v, want keystore preparation error", err)
	}
}

func TestCrossPlatformCoverageAvailableAgreesWithBackendVersion(t *testing.T) {
	if Available() != (BackendVersion != "") {
		t.Fatalf("Available() = %v but BackendVersion = %q; they must agree", Available(), BackendVersion)
	}
}

func TestCrossPlatformCoverageTrackedCipherRoundTripsThroughBackend(t *testing.T) {
	backend := &fakeCipher{}
	cipher := newTrackedForTest(t, backend)

	ciphertext, err := cipher.EncryptMessage(context.Background(), "corp", "staff", []byte("hello"))
	if err != nil {
		t.Fatalf("EncryptMessage() = %v", err)
	}
	plaintext, err := cipher.DecryptMessage(context.Background(), "corp", "staff", ciphertext)
	if err != nil {
		t.Fatalf("DecryptMessage() = %v", err)
	}
	if string(plaintext) != "hello" {
		t.Fatalf("round trip = %q, want hello", plaintext)
	}
}

func TestCrossPlatformCoverageTrackedCipherRejectsEmptyCorpID(t *testing.T) {
	cipher := newTrackedForTest(t, &fakeCipher{})
	if _, err := cipher.EncryptMessage(context.Background(), "", "staff", []byte("x")); !errors.Is(err, ErrNoCorpID) {
		t.Fatalf("EncryptMessage() = %v, want ErrNoCorpID", err)
	}
	if _, err := cipher.DecryptMessage(context.Background(), "", "staff", []byte("x")); !errors.Is(err, ErrNoCorpID) {
		t.Fatalf("DecryptMessage() = %v, want ErrNoCorpID", err)
	}
}

func TestCrossPlatformCoverageTrackedCipherRejectsEmptyPayload(t *testing.T) {
	cipher := newTrackedForTest(t, &fakeCipher{})
	if _, err := cipher.EncryptMessage(context.Background(), "corp", "staff", nil); !errors.Is(err, ErrEmptyPayload) {
		t.Fatalf("EncryptMessage() = %v, want ErrEmptyPayload", err)
	}
	if _, err := cipher.DecryptMessage(context.Background(), "corp", "staff", []byte{}); !errors.Is(err, ErrEmptyPayload) {
		t.Fatalf("DecryptMessage() = %v, want ErrEmptyPayload", err)
	}
}

func TestCrossPlatformCoverageTrackedCipherRejectsUseAfterClose(t *testing.T) {
	backend := &fakeCipher{}
	cipher := newTrackedForTest(t, backend)
	if err := cipher.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	if _, err := cipher.EncryptMessage(context.Background(), "corp", "staff", []byte("x")); !errors.Is(err, ErrClosed) {
		t.Fatalf("EncryptMessage() after Close = %v, want ErrClosed", err)
	}
}

func TestCrossPlatformCoverageTrackedCipherCloseIsIdempotentAndClosesBackendOnce(t *testing.T) {
	backend := &fakeCipher{}
	cipher := newTrackedForTest(t, backend)
	for i := 0; i < 3; i++ {
		if err := cipher.Close(); err != nil {
			t.Fatalf("Close() #%d = %v", i+1, err)
		}
	}
	if backend.closeCount != 1 {
		t.Fatalf("backend Close called %d times, want exactly 1", backend.closeCount)
	}
}

func TestCrossPlatformCoverageTrackedCipherCloseReleasesProcessSlot(t *testing.T) {
	cipher := newTrackedForTest(t, &fakeCipher{})
	if err := cipher.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	process.mu.Lock()
	open := process.open
	process.mu.Unlock()
	if open {
		t.Fatal("Close did not release the process slot, so a later Open would fail")
	}
}

func TestCrossPlatformCoverageTrackedCipherClosePropagatesBackendError(t *testing.T) {
	wantErr := errors.New("backend close failed")
	cipher := newTrackedForTest(t, &fakeCipher{closeErr: wantErr})
	if err := cipher.Close(); !errors.Is(err, wantErr) {
		t.Fatalf("Close() = %v, want %v", err, wantErr)
	}
	process.mu.Lock()
	open := process.open
	process.mu.Unlock()
	if open {
		t.Fatal("a failing backend Close must still release the process slot")
	}
}

func TestCrossPlatformCoverageOpenRefusesSecondCipherWhileOneIsOpen(t *testing.T) {
	oldAvailable := backendAvailable
	oldBackend := openBackend
	t.Cleanup(func() {
		backendAvailable = oldAvailable
		openBackend = oldBackend
	})
	backendAvailable = func() bool { return true }
	openBackend = func(context.Context, Config) (Cipher, error) { return &fakeCipher{}, nil }

	process.mu.Lock()
	process.open = true
	process.mu.Unlock()
	t.Cleanup(func() {
		process.mu.Lock()
		process.open = false
		process.mu.Unlock()
	})
	_, err := Open(context.Background(), Config{
		KeystoreDir: filepath.Join(t.TempDir(), "keystore"),
		AuthCode:    StaticAuthCode("code"),
		KeyServer:   "https://key.example.test",
	})
	if !errors.Is(err, ErrAlreadyOpen) {
		t.Fatalf("Open() = %v, want ErrAlreadyOpen", err)
	}
}
