//go:build darwin || linux

package keychain

import (
	"crypto/cipher"
	"errors"
	"testing"
)

func TestFileKeychainGCMConstructionErrors(t *testing.T) {
	wantErr := errors.New("GCM construction failed")
	failGCM := func(cipher.Block) (cipher.AEAD, error) {
		return nil, wantErr
	}
	key := make([]byte, dekBytes)

	if _, err := encryptDataWithGCM("secret", key, failGCM); !errors.Is(err, wantErr) {
		t.Fatalf("encrypt GCM error = %v", err)
	}
	ciphertext := make([]byte, ivBytes+tagBytes)
	if _, err := decryptDataWithGCM(ciphertext, key, failGCM); !errors.Is(err, wantErr) {
		t.Fatalf("decrypt GCM error = %v", err)
	}
}
