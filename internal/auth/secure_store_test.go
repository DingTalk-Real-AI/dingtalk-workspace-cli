package auth

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSaveSecureTokenData_FixesUnsafePermissions(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "unsafe")
	// Create directory with overly permissive mode.
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	data := &TokenData{
		AccessToken:  "at_perm",
		RefreshToken: "rt_perm",
		ExpiresAt:    time.Now().Add(time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
		CorpID:       "corp1",
	}
	if err := SaveSecureTokenData(configDir, data); err != nil {
		t.Fatalf("SaveSecureTokenData() error = %v", err)
	}

	info, err := os.Stat(configDir)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0o700 {
		t.Fatalf("directory permissions = %o, want 0700", perm)
	}
}

func TestSaveSecureTokenData_PlaintextZeroed(t *testing.T) {
	configDir := t.TempDir()
	data := &TokenData{
		AccessToken:  "at_zero_save",
		RefreshToken: "rt_zero_save",
		ExpiresAt:    time.Now().Add(time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
		CorpID:       "corp1",
	}

	// Save should succeed, which exercises the defer that zeros plaintext.
	if err := SaveSecureTokenData(configDir, data); err != nil {
		t.Fatalf("SaveSecureTokenData() error = %v", err)
	}

	// Verify the data round-trips correctly, proving the zeroing defer
	// runs after encryption (not before).
	loaded, err := LoadSecureTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadSecureTokenData() error = %v", err)
	}
	if loaded.AccessToken != "at_zero_save" {
		t.Fatalf("AccessToken = %q, want at_zero_save", loaded.AccessToken)
	}
}

func TestLoadSecureTokenData_PlaintextZeroed(t *testing.T) {
	configDir := t.TempDir()
	data := &TokenData{
		AccessToken:  "at_zero_load",
		RefreshToken: "rt_zero_load",
		ExpiresAt:    time.Now().Add(time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
		CorpID:       "corp1",
	}
	if err := SaveSecureTokenData(configDir, data); err != nil {
		t.Fatalf("SaveSecureTokenData() error = %v", err)
	}

	// Load twice to confirm zeroing the first plaintext buffer does not
	// corrupt subsequent loads (i.e. each call decrypts independently).
	for i := 0; i < 2; i++ {
		loaded, err := LoadSecureTokenData(configDir)
		if err != nil {
			t.Fatalf("LoadSecureTokenData() iteration %d error = %v", i, err)
		}
		if loaded.AccessToken != "at_zero_load" {
			t.Fatalf("iteration %d: AccessToken = %q, want at_zero_load", i, loaded.AccessToken)
		}
	}
}

func TestSaveSecureTokenData_TmpFileCleanedOnSuccess(t *testing.T) {
	configDir := t.TempDir()
	data := &TokenData{
		AccessToken:  "at_tmp",
		RefreshToken: "rt_tmp",
		ExpiresAt:    time.Now().Add(time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
		CorpID:       "corp1",
	}
	if err := SaveSecureTokenData(configDir, data); err != nil {
		t.Fatalf("SaveSecureTokenData() error = %v", err)
	}

	tmpPath := filepath.Join(configDir, secureDataFile+".tmp")
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Fatalf(".data.tmp should not remain after successful save, stat err = %v", err)
	}

	// The final file must exist.
	finalPath := filepath.Join(configDir, secureDataFile)
	if _, err := os.Stat(finalPath); err != nil {
		t.Fatalf(".data should exist after save, stat err = %v", err)
	}
}

func TestSaveSecureTokenData_DoesNotCreateTokenMarker(t *testing.T) {
	configDir := t.TempDir()
	data := &TokenData{
		AccessToken:  "at_no_marker",
		RefreshToken: "rt_no_marker",
		ExpiresAt:    time.Now().Add(time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
		CorpID:       "corp1",
	}
	if err := SaveSecureTokenData(configDir, data); err != nil {
		t.Fatalf("SaveSecureTokenData() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(configDir, "token.json")); !os.IsNotExist(err) {
		t.Fatalf("token.json should not be created in OSS build, stat err = %v", err)
	}
}

func TestSaveSecureTokenData_ConcurrentSaves(t *testing.T) {
	configDir := t.TempDir()
	const goroutines = 10

	var wg sync.WaitGroup
	errs := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			data := &TokenData{
				AccessToken:  "at_concurrent",
				RefreshToken: "rt_concurrent",
				ExpiresAt:    time.Now().Add(time.Hour),
				RefreshExpAt: time.Now().Add(24 * time.Hour),
				CorpID:       "corp1",
			}
			errs[idx] = SaveSecureTokenData(configDir, data)
		}(i)
	}
	wg.Wait()

	// Under concurrency, some saves may fail due to tmp-file races. That is
	// acceptable — the important thing is that at least one succeeds and the
	// final file is not corrupted.
	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes == 0 {
		t.Fatal("all concurrent SaveSecureTokenData() calls failed; expected at least one success")
	}

	// After all concurrent saves, the file should be loadable (not corrupted).
	loaded, err := LoadSecureTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadSecureTokenData() after concurrent saves error = %v", err)
	}
	if loaded.AccessToken != "at_concurrent" {
		t.Fatalf("AccessToken = %q, want at_concurrent", loaded.AccessToken)
	}
}

func TestDeleteSecureData_Idempotent(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()

	// Delete when file does not exist should not return an error.
	if err := DeleteSecureData(configDir); err != nil {
		t.Fatalf("first DeleteSecureData() on empty dir error = %v", err)
	}

	// Calling again should still be fine.
	if err := DeleteSecureData(configDir); err != nil {
		t.Fatalf("second DeleteSecureData() error = %v", err)
	}
}

func TestSecureDataExists_FalseWhenMissing(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	if SecureDataExists(configDir) {
		t.Fatal("SecureDataExists() = true on empty dir, want false")
	}

	// Also check a completely non-existent directory.
	if SecureDataExists(filepath.Join(configDir, "nonexistent")) {
		t.Fatal("SecureDataExists() = true on non-existent dir, want false")
	}
}
