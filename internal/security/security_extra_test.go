package security

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// ─── storage.go ────────────────────────────────────────────────────────

func TestSecureTokenStorage_SaveAndLoad(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	storage := NewSecureTokenStorage(dir, "", "aa:bb:cc:dd:ee:ff")

	plaintext := []byte(`{"access_token":"access-token-xyz","corp_id":"corp-123"}`)
	if err := storage.SaveEncryptedBytes(plaintext); err != nil {
		t.Fatalf("SaveEncryptedBytes error: %v", err)
	}

	loaded, err := storage.LoadEncryptedBytes()
	if err != nil {
		t.Fatalf("LoadEncryptedBytes error: %v", err)
	}
	if !bytes.Equal(loaded, plaintext) {
		t.Fatalf("round-trip mismatch: got %q want %q", loaded, plaintext)
	}
}

func TestSecureTokenStorage_WrongMAC(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	storage := NewSecureTokenStorage(dir, "", "aa:bb:cc:dd:ee:ff")
	if err := storage.SaveEncryptedBytes([]byte("secret")); err != nil {
		t.Fatalf("SaveEncryptedBytes error: %v", err)
	}

	wrongStorage := NewSecureTokenStorage(dir, "", "11:22:33:44:55:66")
	_, err := wrongStorage.LoadEncryptedBytes()
	if err == nil {
		t.Fatal("expected error with wrong MAC")
	}
}

func TestSecureTokenStorage_LoadMissing(t *testing.T) {
	t.Parallel()
	storage := NewSecureTokenStorage(t.TempDir(), "", "aa:bb:cc:dd:ee:ff")
	_, err := storage.LoadEncryptedBytes()
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestSecureTokenStorage_Exists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	storage := NewSecureTokenStorage(dir, "", "aa:bb:cc:dd:ee:ff")
	if storage.Exists() {
		t.Fatal("should not exist yet")
	}
	if err := storage.SaveEncryptedBytes([]byte("t")); err != nil {
		t.Fatalf("SaveEncryptedBytes error: %v", err)
	}
	if !storage.Exists() {
		t.Fatal("should exist after save")
	}
}

func TestSecureTokenStorage_FallbackDir(t *testing.T) {
	t.Parallel()
	primary := t.TempDir()
	fallback := t.TempDir()

	// Save in fallback
	fbStorage := NewSecureTokenStorage(fallback, "", "aa:bb:cc:dd:ee:ff")
	if err := fbStorage.SaveEncryptedBytes([]byte("fb-token")); err != nil {
		t.Fatalf("SaveEncryptedBytes error: %v", err)
	}

	// Load from primary with fallback
	storage := NewSecureTokenStorage(primary, fallback, "aa:bb:cc:dd:ee:ff")
	loaded, err := storage.LoadEncryptedBytes()
	if err != nil {
		t.Fatalf("LoadEncryptedBytes error: %v", err)
	}
	if !bytes.Equal(loaded, []byte("fb-token")) {
		t.Fatalf("expected fb-token, got %s", loaded)
	}
}

func TestSecureTokenStorage_DataDirs(t *testing.T) {
	t.Parallel()
	s := NewSecureTokenStorage("/a", "/b", "")
	dirs := s.DataDirs()
	if len(dirs) != 2 || dirs[0] != "/a" || dirs[1] != "/b" {
		t.Fatalf("expected [/a /b], got %v", dirs)
	}

	s2 := NewSecureTokenStorage("/a", "", "")
	if len(s2.DataDirs()) != 1 {
		t.Fatal("expected 1 dir when fallback empty")
	}
}

func TestDataFileExistsInAny(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if DataFileExistsInAny(dir) {
		t.Fatal("should not exist")
	}
	os.WriteFile(filepath.Join(dir, DataFileName), []byte("data"), 0o600)
	if !DataFileExistsInAny(dir) {
		t.Fatal("should exist")
	}
	if DataFileExistsInAny("", "/nonexistent") {
		t.Fatal("empty and nonexistent should not match")
	}
}

func TestSecureTokenStorage_DeleteToken(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	storage := NewSecureTokenStorage(dir, "", "aa:bb:cc:dd:ee:ff")
	if err := storage.SaveEncryptedBytes([]byte("t")); err != nil {
		t.Fatalf("SaveEncryptedBytes error: %v", err)
	}
	if !storage.Exists() {
		t.Fatal("should exist")
	}
	if err := storage.DeleteToken(); err != nil {
		t.Fatalf("DeleteToken error: %v", err)
	}
	if storage.Exists() {
		t.Fatal("should not exist after delete")
	}
}
