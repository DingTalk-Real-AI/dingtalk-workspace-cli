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

package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"

	"golang.org/x/crypto/pbkdf2"
)

const (
	// SaltSize is the byte length of the random salt prepended to ciphertext.
	SaltSize = 32

	// NonceSize is the byte length of the GCM nonce.
	NonceSize = 12

	// KeySize is the AES-256 key length in bytes.
	KeySize = 32

	// Iterations is the PBKDF2 iteration count.
	Iterations = 600_000
)

// DeriveKey derives a KeySize-byte key from password and salt using PBKDF2-SHA256.
func DeriveKey(password, salt []byte) []byte {
	return pbkdf2.Key(password, salt, Iterations, KeySize, sha256.New)
}

// Encrypt encrypts plaintext using PBKDF2-derived AES-256-GCM.
// Output format: salt(32) || nonce(12) || ciphertext+tag.
func Encrypt(plaintext, password []byte) ([]byte, error) {
	salt := make([]byte, SaltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("generating salt: %w", err)
	}
	key := DeriveKey(password, salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}
	gcm, err := cipher.NewGCMWithNonceSize(block, NonceSize)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}
	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	out := make([]byte, 0, SaltSize+NonceSize+len(ciphertext))
	out = append(out, salt...)
	out = append(out, nonce...)
	out = append(out, ciphertext...)
	return out, nil
}

// Decrypt decrypts data produced by Encrypt.
// Expects salt(32) || nonce(12) || ciphertext+tag.
func Decrypt(data, password []byte) ([]byte, error) {
	const minSealed = 16 // GCM tag
	if len(data) < SaltSize+NonceSize+minSealed {
		return nil, fmt.Errorf("ciphertext too short (%d bytes)", len(data))
	}
	salt := data[:SaltSize]
	nonce := data[SaltSize : SaltSize+NonceSize]
	sealed := data[SaltSize+NonceSize:]
	key := DeriveKey(password, salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}
	gcm, err := cipher.NewGCMWithNonceSize(block, NonceSize)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption or integrity check failed: %w", err)
	}
	return plain, nil
}
