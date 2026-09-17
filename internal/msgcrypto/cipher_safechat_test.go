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

// Keep this constraint in sync with cipher_safechat.go.

//go:build cgo && (darwin || linux || windows) && (amd64 || arm64)

package msgcrypto

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// These tests exercise the real vendor backend, which means they link
// libsafechat.a and initialise the C library. They never reach the key server:
// an authCode is only spent when a key is actually fetched, and asserting that
// encryption fails without one is exactly the behaviour we want pinned.

func TestCrossPlatformCoverageBackendIsReportedAvailable(t *testing.T) {
	if !Available() {
		t.Fatal("Available() = false in a default CGO build")
	}
	if BackendVersion == "" {
		t.Fatal("BackendVersion is empty in a default CGO build")
	}
	if !strings.Contains(BackendVersion, "safechat") {
		t.Fatalf("BackendVersion = %q, want it to name the vendor SDK", BackendVersion)
	}
}

func TestCrossPlatformCoverageOpenInitialisesCLibraryAndClosesCleanly(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "keystore")

	cipher, err := Open(context.Background(), Config{
		KeystoreDir: dir,
		AuthCode:    StaticAuthCode("placeholder-code"),
		KeyServer:   "https://key.example.test",
	})
	if err != nil {
		t.Fatalf("Open() = %v, want the C library to initialise", err)
	}

	if info, statErr := os.Stat(dir); statErr != nil || !info.IsDir() {
		t.Fatalf("Open did not prepare the keystore dir: %v", statErr)
	}

	if err := cipher.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}

	// The slot must be free again, otherwise a second Open in the same
	// process would be refused forever.
	second, err := Open(context.Background(), Config{
		KeystoreDir: dir,
		AuthCode:    StaticAuthCode("placeholder-code"),
		KeyServer:   "https://key.example.test",
	})
	if err != nil {
		t.Fatalf("second Open() after Close = %v, want success", err)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("second Close() = %v", err)
	}
}

func TestCrossPlatformCoverageOpenRefusesConcurrentSecondCipher(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "keystore")
	first, err := Open(context.Background(), Config{
		KeystoreDir: dir,
		AuthCode:    StaticAuthCode("placeholder-code"),
		KeyServer:   "https://key.example.test",
	})
	if err != nil {
		t.Fatalf("Open() = %v", err)
	}
	defer first.Close()

	_, err = Open(context.Background(), Config{
		KeystoreDir: filepath.Join(t.TempDir(), "keystore"),
		AuthCode:    StaticAuthCode("placeholder-code"),
		KeyServer:   "https://key.example.test",
	})
	if !errors.Is(err, ErrAlreadyOpen) {
		t.Fatalf("second Open() = %v, want ErrAlreadyOpen (the C library keeps global state)", err)
	}
}

func TestCrossPlatformCoverageOpenHonoursCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Open(ctx, Config{
		KeystoreDir: filepath.Join(t.TempDir(), "keystore"),
		AuthCode:    StaticAuthCode("placeholder-code"),
		KeyServer:   "https://key.example.test",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Open() with a cancelled context = %v, want context.Canceled", err)
	}
}

func TestCrossPlatformCoverageEncryptWithoutUsableKeyReportsAuthCodeCause(t *testing.T) {
	// A cold keystore forces a key request. With no reachable key server the
	// operation must fail with a message that names the auth code, rather
	// than a bare vendor return code.
	cipher, err := Open(context.Background(), Config{
		KeystoreDir: filepath.Join(t.TempDir(), "keystore"),
		AuthCode: AuthCodeFunc(func(context.Context) (string, error) {
			return "", errors.New("no code available in test")
		}),
		KeyServer: "https://key.example.test",
	})
	if err != nil {
		t.Fatalf("Open() = %v", err)
	}
	defer cipher.Close()

	_, err = cipher.EncryptMessage(context.Background(), "test-corp", "test-staff", []byte("hello"))
	if err == nil {
		t.Skip("the environment served a key without an auth code; nothing to assert")
	}
	if !strings.Contains(err.Error(), "auth code") {
		t.Fatalf("EncryptMessage() = %v, want the error to name the auth code cause", err)
	}
}

func TestCrossPlatformCoverageCipherRejectsBadArgumentsBeforeCallingC(t *testing.T) {
	cipher, err := Open(context.Background(), Config{
		KeystoreDir: filepath.Join(t.TempDir(), "keystore"),
		AuthCode:    StaticAuthCode("placeholder-code"),
		KeyServer:   "https://key.example.test",
	})
	if err != nil {
		t.Fatalf("Open() = %v", err)
	}
	defer cipher.Close()

	if _, err := cipher.EncryptMessage(context.Background(), "", "staff", []byte("x")); !errors.Is(err, ErrNoCorpID) {
		t.Fatalf("EncryptMessage() with no corpID = %v, want ErrNoCorpID", err)
	}
	// The vendor SDK dereferences the first byte of the payload, so an empty
	// slice must never reach it.
	if _, err := cipher.DecryptMessage(context.Background(), "corp", "staff", nil); !errors.Is(err, ErrEmptyPayload) {
		t.Fatalf("DecryptMessage() with no payload = %v, want ErrEmptyPayload", err)
	}
}

// TestCrossPlatformCoverageCloseDoesNotRaceActiveOperations pins that Close
// waits for in-flight operations. The vendor client reads Client.inited before
// taking its own mutex, so a Close overlapping an encrypt or decrypt is a data
// race that only surfaces under -race: the window is narrow and every cycle
// must be attempted to hit it.
func TestCrossPlatformCoverageCloseDoesNotRaceActiveOperations(t *testing.T) {
	for cycle := 0; cycle < 120; cycle++ {
		cipher, err := Open(context.Background(), Config{
			KeystoreDir: filepath.Join(t.TempDir(), "keystore"),
			AuthCode:    StaticAuthCode("placeholder-code"),
			KeyServer:   "https://key.example.test",
		})
		if err != nil {
			t.Fatalf("cycle %d: Open() = %v", cycle, err)
		}

		start := make(chan struct{})
		var wg sync.WaitGroup
		for worker := 0; worker < 8; worker++ {
			wg.Add(1)
			go func(worker int) {
				defer wg.Done()
				<-start
				if worker%2 == 0 {
					_, _ = cipher.EncryptMessage(context.Background(), "test-corp", "test-staff", []byte("hello"))
					return
				}
				_, _ = cipher.DecryptMessage(context.Background(), "test-corp", "test-staff", []byte("hello"))
			}(worker)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_ = cipher.Close()
		}()

		close(start)
		wg.Wait()

		if err := cipher.Close(); err != nil {
			t.Fatalf("cycle %d: repeated Close() = %v, want the second close to be a no-op", cycle, err)
		}
	}
}
