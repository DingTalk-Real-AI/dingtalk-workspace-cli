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
	"bytes"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	t.Parallel()

	password := []byte("aa:bb:cc:dd:ee:ff")
	plain := []byte(`{"access_token":"x"}`)

	encrypted, err := Encrypt(plain, password)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if len(encrypted) < SaltSize+NonceSize+16 {
		t.Fatalf("ciphertext too short: %d bytes", len(encrypted))
	}

	decrypted, err := Decrypt(encrypted, password)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if !bytes.Equal(decrypted, plain) {
		t.Fatalf("round-trip: got %q want %q", decrypted, plain)
	}
}

func TestDecryptWithWrongPassword(t *testing.T) {
	t.Parallel()

	password := []byte("aa:bb:cc:dd:ee:ff")
	plain := []byte(`{"access_token":"x"}`)

	encrypted, err := Encrypt(plain, password)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	_, err = Decrypt(encrypted, []byte("wrong-password"))
	if err == nil {
		t.Fatal("Decrypt with wrong password should fail")
	}
}

func TestDecryptRejectsShortData(t *testing.T) {
	t.Parallel()

	_, err := Decrypt([]byte{0, 1, 2, 3}, []byte("password"))
	if err == nil {
		t.Fatal("Decrypt should fail with data shorter than salt+nonce+tag")
	}
}

func TestDecryptRejectsTamperedCiphertext(t *testing.T) {
	t.Parallel()

	password := []byte("test-password-123456")
	encrypted, err := Encrypt([]byte("secret data"), password)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	tampered := make([]byte, len(encrypted))
	copy(tampered, encrypted)
	tampered[len(tampered)-1] ^= 0xFF

	_, err = Decrypt(tampered, password)
	if err == nil {
		t.Fatal("Decrypt should fail with tampered ciphertext")
	}
}

func TestEncryptProducesDifferentCiphertexts(t *testing.T) {
	t.Parallel()

	password := []byte("deterministic-password")
	plaintext := []byte("same plaintext")

	enc1, err := Encrypt(plaintext, password)
	if err != nil {
		t.Fatalf("first Encrypt() error = %v", err)
	}
	enc2, err := Encrypt(plaintext, password)
	if err != nil {
		t.Fatalf("second Encrypt() error = %v", err)
	}
	if bytes.Equal(enc1, enc2) {
		t.Fatal("two encryptions of same plaintext should differ (random salt/nonce)")
	}

	// Both should decrypt to the same plaintext.
	dec1, _ := Decrypt(enc1, password)
	dec2, _ := Decrypt(enc2, password)
	if !bytes.Equal(dec1, dec2) || !bytes.Equal(dec1, plaintext) {
		t.Fatal("both ciphertexts should decrypt to the same plaintext")
	}
}

func TestGetMACAddress(t *testing.T) {
	mac, err := GetMACAddress()
	if err != nil {
		t.Skipf("no suitable MAC in this environment: %v", err)
	}
	// MAC should be at least "xx:xx:xx:xx:xx:xx" (17 chars) or fallback format.
	if len(mac) < 17 {
		t.Fatalf("unexpected MAC: %q", mac)
	}
}

func TestSecureTokenStorageRoundTrip(t *testing.T) {
	configDir := t.TempDir()
	mac := "aa:bb:cc:dd:ee:ff"
	storage := NewSecureTokenStorage(configDir, "", mac)

	if storage.Exists() {
		t.Fatal("storage should not exist yet")
	}

	plaintext := []byte(`{"access_token":"at_test","refresh_token":"rt_test","corp_id":"dingcorp"}`)
	if err := storage.SaveEncryptedBytes(plaintext); err != nil {
		t.Fatalf("SaveEncryptedBytes() error = %v", err)
	}

	if !storage.Exists() {
		t.Fatal("storage should exist after save")
	}

	loaded, err := storage.LoadEncryptedBytes()
	if err != nil {
		t.Fatalf("LoadEncryptedBytes() error = %v", err)
	}
	if !bytes.Equal(loaded, plaintext) {
		t.Fatalf("round-trip mismatch: got %q want %q", loaded, plaintext)
	}

	// Wrong MAC should fail.
	wrongStorage := NewSecureTokenStorage(configDir, "", "11:22:33:44:55:66")
	if _, err := wrongStorage.LoadEncryptedBytes(); err == nil {
		t.Fatal("LoadEncryptedBytes with wrong MAC should fail")
	}

	if err := storage.DeleteToken(); err != nil {
		t.Fatalf("DeleteToken() error = %v", err)
	}
	if storage.Exists() {
		t.Fatal("storage should not exist after delete")
	}
}
