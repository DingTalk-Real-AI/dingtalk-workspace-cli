package security

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ─── crypto.go ─────────────────────────────────────────────────────────

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	t.Parallel()
	plain := []byte("sensitive data here")
	password := []byte("test-password-123")
	enc, err := Encrypt(plain, password)
	if err != nil {
		t.Fatalf("Encrypt error: %v", err)
	}
	dec, err := Decrypt(enc, password)
	if err != nil {
		t.Fatalf("Decrypt error: %v", err)
	}
	if string(dec) != string(plain) {
		t.Fatalf("round-trip mismatch: %q vs %q", dec, plain)
	}
}

func TestEncryptDecrypt_EmptyPlaintext(t *testing.T) {
	t.Parallel()
	enc, err := Encrypt([]byte{}, []byte("pass"))
	if err != nil {
		t.Fatalf("Encrypt empty error: %v", err)
	}
	dec, err := Decrypt(enc, []byte("pass"))
	if err != nil {
		t.Fatalf("Decrypt empty error: %v", err)
	}
	if len(dec) != 0 {
		t.Fatalf("expected empty, got %d bytes", len(dec))
	}
}

func TestDecrypt_TooShort(t *testing.T) {
	t.Parallel()
	_, err := Decrypt([]byte("short"), []byte("pass"))
	if err == nil {
		t.Fatal("expected error for short ciphertext")
	}
}

func TestDecrypt_Corrupted(t *testing.T) {
	t.Parallel()
	enc, _ := Encrypt([]byte("data"), []byte("pass"))
	// Corrupt the last byte (GCM tag)
	enc[len(enc)-1] ^= 0xff
	_, err := Decrypt(enc, []byte("pass"))
	if err == nil {
		t.Fatal("expected error for corrupted ciphertext")
	}
}

func TestDecrypt_WrongPassword(t *testing.T) {
	t.Parallel()
	enc, _ := Encrypt([]byte("secret"), []byte("correct"))
	_, err := Decrypt(enc, []byte("wrong"))
	if err == nil {
		t.Fatal("expected error for wrong password")
	}
}

func TestDeriveKey_Deterministic(t *testing.T) {
	t.Parallel()
	salt := []byte("fixed-salt-value-32-bytes-long!!")
	k1 := DeriveKey([]byte("pass"), salt)
	k2 := DeriveKey([]byte("pass"), salt)
	if len(k1) != KeySize {
		t.Fatalf("expected %d bytes, got %d", KeySize, len(k1))
	}
	for i := range k1 {
		if k1[i] != k2[i] {
			t.Fatal("same inputs should produce same key")
		}
	}
}

// ─── fingerprint.go ────────────────────────────────────────────────────

func TestIsVirtualMAC(t *testing.T) {
	t.Parallel()
	tests := []struct {
		mac  string
		want bool
	}{
		{"02:42:ac:11:00:02", true},  // Docker
		{"00:50:56:c0:00:01", true},  // VMware
		{"00:0c:29:ab:cd:ef", true},  // VMware
		{"08:00:27:12:34:56", true},  // VirtualBox
		{"52:54:00:ab:cd:ef", true},  // KVM/QEMU
		{"aa:bb:cc:dd:ee:ff", false}, // physical
		{"14:98:77:ab:cd:ef", false}, // physical
	}
	for _, tt := range tests {
		if got := isVirtualMAC(tt.mac); got != tt.want {
			t.Errorf("isVirtualMAC(%s) = %v, want %v", tt.mac, got, tt.want)
		}
	}
}

func TestGetMACAddress_ReturnsNonEmpty(t *testing.T) {
	t.Parallel()
	mac, err := GetMACAddress()
	if err != nil {
		t.Skipf("no physical NIC available: %v", err)
	}
	if mac == "" {
		t.Fatal("expected non-empty MAC address")
	}
	// Should look like a MAC
	if len(mac) < 11 {
		t.Fatalf("MAC too short: %s", mac)
	}
}

// ─── storage.go ────────────────────────────────────────────────────────

func TestDeleteEncryptedData(t *testing.T) {
	t.Parallel()
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	// Create .data files
	os.WriteFile(filepath.Join(dir1, DataFileName), []byte("enc1"), 0o600)
	os.WriteFile(filepath.Join(dir2, DataFileName), []byte("enc2"), 0o600)

	err := DeleteEncryptedData(dir1, dir2)
	if err != nil {
		t.Fatalf("DeleteEncryptedData error: %v", err)
	}
	if DataFileExistsInAny(dir1, dir2) {
		t.Fatal("files should be deleted")
	}
}

func TestDeleteEncryptedData_EmptyDirs(t *testing.T) {
	t.Parallel()
	err := DeleteEncryptedData("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteEncryptedData_NonExistent(t *testing.T) {
	t.Parallel()
	err := DeleteEncryptedData("/nonexistent/dir")
	if err != nil {
		t.Fatalf("unexpected error for nonexistent: %v", err)
	}
}

func TestSecureTokenStorage_ExistsInFallback(t *testing.T) {
	t.Parallel()
	primary := t.TempDir()
	fallback := t.TempDir()
	os.WriteFile(filepath.Join(fallback, DataFileName), []byte("data"), 0o600)

	s := NewSecureTokenStorage(primary, fallback, "")
	if !s.Exists() {
		t.Fatal("should find .data in fallback dir")
	}
}

func TestSecureTokenStorage_DeleteTokenBothDirs(t *testing.T) {
	t.Parallel()
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	s1 := NewSecureTokenStorage(dir1, "", "aa:bb:cc:dd:ee:ff")
	s1.SaveToken(&TokenData{AccessToken: "t", ExpiresAt: time.Now().Add(time.Hour)})
	os.WriteFile(filepath.Join(dir2, DataFileName), []byte("enc"), 0o600)

	s := NewSecureTokenStorage(dir1, dir2, "aa:bb:cc:dd:ee:ff")
	if err := s.DeleteToken(); err != nil {
		t.Fatalf("error: %v", err)
	}
	if DataFileExistsInAny(dir1, dir2) {
		t.Fatal("both should be deleted")
	}
}
