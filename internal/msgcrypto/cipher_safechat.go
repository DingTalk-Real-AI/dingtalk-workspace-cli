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

// The constraint below must stay in sync with cipher_stub.go, which negates it
// verbatim. It encodes the platforms the vendor ships a libsafechat.a for:
// darwin and linux on amd64/arm64, plus windows/amd64. windows/arm64 is
// deliberately excluded because the vendor has not delivered that static
// library, and DWS does release that target.

//go:build safechat && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package msgcrypto

import (
	"context"
	"errors"
	"fmt"
	"sync"

	safechat "safechat-go-sdk"
)

// BackendVersion identifies the compiled-in vendor SDK.
const BackendVersion = "safechat " + safechat.Version

// Available reports that this binary carries the SafeChat backend.
func Available() bool { return true }

// safechatCipher adapts the vendor client to Cipher.
//
// The vendor client serialises its own C calls internally, so this type adds no
// further locking. Auth codes are minted only from AuthCodeHook, which the
// vendor SDK calls inside goProxy when a key is actually missing.
type safechatCipher struct {
	client      *safechat.Client
	codes       AuthCodeProvider
	allowedHost string

	mu          sync.Mutex
	lastCodeErr error
}

// newBackend starts the vendor client against cfg's keystore.
//
// A warm keystore serves encrypt and decrypt without a key request, so no
// authCode is fetched at open time. The hook runs only if goProxy fires.
func newBackend(ctx context.Context, cfg Config) (Cipher, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var logf func(string, ...any)
	if cfg.Debug {
		logf = cfg.Logf
	}

	c := &safechatCipher{codes: cfg.AuthCode, allowedHost: cfg.AllowedRedirectHost}
	client, err := safechat.New(safechat.Config{
		DataPath:     cfg.KeystoreDir,
		UserID:       cfg.UserID,
		KeyServer:    cfg.KeyServer,
		MaxRetry:     cfg.MaxRetry,
		HTTPTimeout:  cfg.HTTPTimeout,
		Logger:       newRedactingLogger(logf),
		AuthCodeHook: c.authCodeHook,
	})
	if err != nil {
		if errors.Is(err, safechat.ErrAlreadyInitialized) {
			return nil, ErrAlreadyOpen
		}
		return nil, fmt.Errorf("msgcrypto: start safechat backend: %w", err)
	}
	c.client = client
	return c, nil
}

// EncryptMessage encrypts plaintext and returns the vendor ciphertext.
func (c *safechatCipher) EncryptMessage(ctx context.Context, corpID, staffID string, plaintext []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.setLastCodeErr(nil)
	out, err := c.client.EncryptMsg(corpID, staffID, plaintext)
	if err != nil {
		return nil, c.explain("encrypt", corpID, err)
	}
	return out, nil
}

// DecryptMessage decrypts a vendor ciphertext.
func (c *safechatCipher) DecryptMessage(ctx context.Context, corpID, staffID string, ciphertext []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.setLastCodeErr(nil)
	out, err := c.client.DecryptMsg(corpID, staffID, ciphertext)
	if err != nil {
		return nil, c.explain("decrypt", corpID, err)
	}
	return out, nil
}

// Close releases the vendor client.
func (c *safechatCipher) Close() error {
	c.client.Close()
	return nil
}

// authCodeHook is invoked from goProxy immediately before the key request.
// domain is compared locally and never forwarded to portal. The returned
// code is used once by the SDK and is not stored on the client.
func (c *safechatCipher) authCodeHook(corpID, domain string) (string, error) {
	code, err := mintAuthCodeForProxy(c.codes, c.allowedHost, corpID, domain)
	c.setLastCodeErr(err)
	return code, err
}

func (c *safechatCipher) setLastCodeErr(err error) {
	c.mu.Lock()
	c.lastCodeErr = err
	c.mu.Unlock()
}

func (c *safechatCipher) lastAuthCodeErr() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastCodeErr
}

// explain turns a vendor error into an actionable one, folding in a failed
// goProxy authCode mint and the admin-restricted case.
func (c *safechatCipher) explain(op, corpID string, opErr error) error {
	if c.client.IsBlocked(corpID) {
		return fmt.Errorf("msgcrypto: %s blocked: the organization's key is restricted by its administrator: %w", op, opErr)
	}

	// A key fetch was needed but we had no usable code: that is the real
	// cause, so report both.
	if codeErr := c.lastAuthCodeErr(); codeErr != nil {
		return fmt.Errorf("msgcrypto: %s failed and no usable auth code was available: %w (auth code error: %v)", op, opErr, codeErr)
	}

	if errors.Is(opErr, safechat.ErrMaxRetryExceeded) {
		invalidateAuthCode(c.codes)
		return fmt.Errorf("msgcrypto: %s failed: key material never became available: %w", op, opErr)
	}

	return fmt.Errorf("msgcrypto: %s failed: %w", op, opErr)
}
