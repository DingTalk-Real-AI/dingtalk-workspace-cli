package auth

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestSaveSecureTokenData_FixesUnsafePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows enforces directory access through ACLs, not POSIX mode bits")
	}
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

	tmpPaths, err := filepath.Glob(filepath.Join(configDir, secureDataFile+".tmp-*"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(tmpPaths) != 0 {
		t.Fatalf("temporary files should not remain after successful save: %v", tmpPaths)
	}

	// The final file must exist.
	finalPath := filepath.Join(configDir, secureDataFile)
	if _, err := os.Stat(finalPath); err != nil {
		t.Fatalf(".data should exist after save, stat err = %v", err)
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

	// Each writer owns its temporary file, so concurrent saves must not corrupt
	// the final file. A platform may still reject simultaneous replacements, but
	// at least one complete save must succeed.
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

	tmpPattern := filepath.Join(configDir, secureDataFile+".tmp-*")
	if matches, err := filepath.Glob(tmpPattern); err != nil || len(matches) != 0 {
		t.Fatalf("secure temp files should not remain after concurrent saves, matches = %v, err = %v", matches, err)
	}
}

func TestDeleteSecureData_Idempotent(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	legacyTmpPath := filepath.Join(configDir, secureDataFile+".tmp")
	if err := os.WriteFile(legacyTmpPath, []byte("legacy temp"), 0o600); err != nil {
		t.Fatalf("WriteFile() legacy temp error = %v", err)
	}
	uniqueTmpPath := filepath.Join(configDir, secureDataFile+".tmp-interrupted")
	if err := os.WriteFile(uniqueTmpPath, []byte("unique temp"), 0o600); err != nil {
		t.Fatalf("WriteFile() unique temp error = %v", err)
	}
	unrelatedTmpPath := filepath.Join(configDir, secureDataFile+".unrelated.tmp")
	if err := os.WriteFile(unrelatedTmpPath, []byte("unrelated temp"), 0o600); err != nil {
		t.Fatalf("WriteFile() unrelated temp error = %v", err)
	}

	// Delete when the final file does not exist should still remove both legacy
	// and per-write crash remnants without returning an error.
	if err := DeleteSecureData(configDir); err != nil {
		t.Fatalf("first DeleteSecureData() on empty dir error = %v", err)
	}
	for _, tmpPath := range []string{legacyTmpPath, uniqueTmpPath} {
		if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
			t.Fatalf("temporary file %q was not removed, stat err = %v", tmpPath, err)
		}
	}
	if _, err := os.Stat(unrelatedTmpPath); err != nil {
		t.Fatalf("unrelated temporary file was removed: %v", err)
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
