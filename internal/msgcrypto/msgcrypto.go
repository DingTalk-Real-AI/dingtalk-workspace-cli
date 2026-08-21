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

// Package msgcrypto wraps the third-party SafeChat SDK so DWS can decrypt
// DingTalk messages that an organization has encrypted with its own key
// material, and encrypt outbound ones.
//
// The SafeChat backend links a prebuilt C static library and therefore needs
// CGO. Because DWS ships CGO-free cross-compiled release binaries, the backend
// is compiled only under the "safechat" build tag:
//
//	CGO_ENABLED=1 go build -tags safechat ./cmd
//
// Every other build gets a stub whose constructor fails with ErrUnavailable,
// so callers must always handle that error rather than assume the capability
// exists. Use Available to branch before offering the feature to a user.
//
// Key material is fetched from the vendor key server on demand, which requires
// a DingTalk 免登 authCode supplied through AuthCodeProvider. DWS does not mint
// that code itself; the caller injects a provider.
package msgcrypto

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

// Errors reported by this package. Callers are expected to test for
// ErrUnavailable explicitly, because it is the normal outcome on every build
// that does not enable the safechat tag.
var (
	// ErrUnavailable means this binary was built without the SafeChat
	// backend, or for a platform the vendor does not ship a static library
	// for (notably windows/arm64).
	ErrUnavailable = errors.New("msgcrypto: SafeChat backend not built into this binary")

	// ErrAlreadyOpen means a Cipher is already open. The underlying C
	// library keeps global state, so only one may exist per process.
	ErrAlreadyOpen = errors.New("msgcrypto: a cipher is already open in this process")

	// ErrClosed is returned by a Cipher whose Close has already run.
	ErrClosed = errors.New("msgcrypto: cipher is closed")

	// ErrNoAuthCodeProvider means Config.AuthCode was nil. Without it the
	// backend cannot fetch or rotate key material.
	ErrNoAuthCodeProvider = errors.New("msgcrypto: config.AuthCode is required")

	// ErrEmptyPayload means an encrypt or decrypt call got no bytes. The
	// vendor SDK rejects empty input, so we reject it earlier with a
	// clearer message.
	ErrEmptyPayload = errors.New("msgcrypto: payload is empty")

	// ErrNoCorpID means the caller omitted the organization id, which
	// selects the key and therefore cannot be defaulted.
	ErrNoCorpID = errors.New("msgcrypto: corpID is required")

	// ErrNoKeyServer means Config.KeyServer was empty. The vendor C
	// library would otherwise pick the key-request destination.
	ErrNoKeyServer = errors.New("msgcrypto: config.KeyServer is required")

	// ErrInvalidKeyServer means Config.KeyServer is not a usable URL.
	ErrInvalidKeyServer = errors.New("msgcrypto: config.KeyServer is not a valid URL")

	// ErrKeyServerNotHTTPS means Config.KeyServer is not https.
	ErrKeyServerNotHTTPS = errors.New("msgcrypto: config.KeyServer must be an https URL")

	// ErrRedirectHostMismatch means the domain goProxy received does not
	// match Config.AllowedRedirectHost. The domain is never sent to portal.
	ErrRedirectHostMismatch = errors.New("msgcrypto: goProxy domain host does not match AllowedRedirectHost")
)

// keystoreDirPerm keeps the key cache owner-only. The directory holds
// organization key material, so it must not be group- or world-readable.
const keystoreDirPerm fs.FileMode = 0o700

// DefaultKeystoreDir returns the default key cache directory,
// ~/.dws/safechat/keystore, honouring DWS_CONFIG_DIR like the rest of DWS.
func DefaultKeystoreDir() string {
	return filepath.Join(config.DefaultConfigDir(), "safechat", "keystore")
}

// Cipher encrypts and decrypts message payloads for one organization at a
// time. Implementations are safe for concurrent use.
type Cipher interface {
	// EncryptMessage encrypts plaintext for corpID/staffID and returns the
	// vendor ciphertext envelope.
	EncryptMessage(ctx context.Context, corpID, staffID string, plaintext []byte) ([]byte, error)

	// DecryptMessage decrypts a vendor ciphertext envelope.
	DecryptMessage(ctx context.Context, corpID, staffID string, ciphertext []byte) ([]byte, error)

	// Close releases the backend and frees the process-wide slot so a later
	// Open can succeed. Calling it twice is safe.
	Close() error
}

// Config parameterises Open.
type Config struct {
	// KeystoreDir is where fetched keys are cached. Defaults to
	// DefaultKeystoreDir. It is created with 0700 if missing.
	KeystoreDir string

	// UserID is an opaque local identifier. The vendor SDK stores it but
	// does not use it for key selection; leave empty to let the SDK
	// generate one.
	UserID string

	// AuthCode supplies the DingTalk 免登 authCode used to authenticate key
	// requests. Required. The backend calls it only from the vendor goProxy
	// callback (cold keystore or key-version rotation), never on every
	// encrypt/decrypt.
	AuthCode AuthCodeProvider

	// KeyServer is the HTTPS URL of the vendor key service. Required: it
	// replaces the host the closed-source C library would otherwise pick.
	KeyServer string

	// AllowedRedirectHost, when set, is compared to the domain the C
	// library passes into goProxy. A mismatch fails that key fetch. It is
	// a local check only; the domain is never sent to portal.
	AllowedRedirectHost string

	// MaxRetry bounds retries while a key is still being fetched. Zero
	// selects the vendor default.
	MaxRetry int

	// HTTPTimeout bounds each key request. Zero selects the vendor default.
	HTTPTimeout time.Duration

	// Debug enables backend logging through a redacting logger. Off by
	// default because the vendor SDK logs the authCode and raw key-server
	// responses at debug level.
	Debug bool

	// Logf receives already-redacted backend log lines when Debug is set.
	// Nil discards them.
	Logf func(format string, args ...any)
}

// withDefaults returns cfg with empty optional fields filled in.
func (cfg Config) withDefaults() Config {
	if cfg.KeystoreDir == "" {
		cfg.KeystoreDir = DefaultKeystoreDir()
	}
	return cfg
}

// validate reports whether cfg carries everything the backend needs.
func (cfg Config) validate() error {
	if cfg.AuthCode == nil {
		return ErrNoAuthCodeProvider
	}
	if cfg.KeystoreDir == "" {
		return errors.New("msgcrypto: config.KeystoreDir resolved to an empty path")
	}
	if err := validateKeyServer(cfg.KeyServer); err != nil {
		return err
	}
	return nil
}

// prepareKeystore creates dir if needed and makes sure it is owner-only.
// An existing directory with looser bits is tightened, because it caches key
// material.
func prepareKeystore(dir string) error {
	if err := os.MkdirAll(dir, keystoreDirPerm); err != nil {
		return fmt.Errorf("msgcrypto: create keystore dir: %w", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("msgcrypto: stat keystore dir: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("msgcrypto: keystore path %s is not a directory", dir)
	}
	// Windows does not model POSIX bits, so only tighten where they apply.
	if runtimeSupportsPOSIXPerm && info.Mode().Perm() != keystoreDirPerm {
		if err := os.Chmod(dir, keystoreDirPerm); err != nil {
			return fmt.Errorf("msgcrypto: restrict keystore dir permissions: %w", err)
		}
	}
	return nil
}

// process holds the single-instance guard. The vendor C library keeps global
// state, so a second concurrent Cipher would corrupt it.
var process struct {
	mu   sync.Mutex
	open bool
}

// Open validates cfg, prepares the keystore and starts the backend.
//
// It returns ErrUnavailable when the backend was not compiled in, so callers
// can degrade gracefully. Only one Cipher may be open per process; Close frees
// the slot.
func Open(ctx context.Context, cfg Config) (Cipher, error) {
	cfg = cfg.withDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if !Available() {
		return nil, ErrUnavailable
	}
	if err := prepareKeystore(cfg.KeystoreDir); err != nil {
		return nil, err
	}

	process.mu.Lock()
	defer process.mu.Unlock()
	if process.open {
		return nil, ErrAlreadyOpen
	}
	backend, err := newBackend(ctx, cfg)
	if err != nil {
		return nil, err
	}
	process.open = true
	return &trackedCipher{backend: backend}, nil
}

// trackedCipher releases the process-wide slot when the wrapped backend closes.
type trackedCipher struct {
	mu      sync.Mutex
	backend Cipher
}

// EncryptMessage validates the payload and delegates to the backend.
func (c *trackedCipher) EncryptMessage(ctx context.Context, corpID, staffID string, plaintext []byte) ([]byte, error) {
	backend, err := c.live(corpID, plaintext)
	if err != nil {
		return nil, err
	}
	return backend.EncryptMessage(ctx, corpID, staffID, plaintext)
}

// DecryptMessage validates the payload and delegates to the backend.
func (c *trackedCipher) DecryptMessage(ctx context.Context, corpID, staffID string, ciphertext []byte) ([]byte, error) {
	backend, err := c.live(corpID, ciphertext)
	if err != nil {
		return nil, err
	}
	return backend.DecryptMessage(ctx, corpID, staffID, ciphertext)
}

// live returns the backend after checking the cipher is open and the arguments
// are usable.
func (c *trackedCipher) live(corpID string, payload []byte) (Cipher, error) {
	if corpID == "" {
		return nil, ErrNoCorpID
	}
	if len(payload) == 0 {
		return nil, ErrEmptyPayload
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.backend == nil {
		return nil, ErrClosed
	}
	return c.backend, nil
}

// Close closes the backend once and releases the process-wide slot.
func (c *trackedCipher) Close() error {
	c.mu.Lock()
	backend := c.backend
	c.backend = nil
	c.mu.Unlock()
	if backend == nil {
		return nil
	}

	err := backend.Close()

	process.mu.Lock()
	process.open = false
	process.mu.Unlock()
	return err
}
