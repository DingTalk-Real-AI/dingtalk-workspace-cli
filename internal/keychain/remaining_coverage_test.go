// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

//go:build darwin

package keychain

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCrossPlatformCoverageAuthTokenCiphertextPathFailureEdges(t *testing.T) {
	origReadDir, origInfo := authEntriesReadDir, authEntryInfo
	t.Cleanup(func() {
		authEntriesReadDir = origReadDir
		authEntryInfo = origInfo
	})

	t.Setenv(StorageDirEnv, t.TempDir())
	if paths, err := authTokenCiphertextPaths(Service); err != nil || len(paths) != 0 {
		t.Fatalf("missing storage = %v, %v", paths, err)
	}

	fail := errors.New("read dir failed")
	authEntriesReadDir = func(string) ([]os.DirEntry, error) { return nil, fail }
	if _, err := authTokenCiphertextPaths(Service); !errors.Is(err, fail) {
		t.Fatalf("read dir error = %v", err)
	}

	dir := StorageDir(Service)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	entryPath := filepath.Join(dir, safeFileName(AccountToken))
	if err := os.WriteFile(entryPath, []byte("ciphertext"), 0o600); err != nil {
		t.Fatal(err)
	}
	authEntriesReadDir = os.ReadDir
	authEntryInfo = func(os.DirEntry) (os.FileInfo, error) { return nil, fail }
	if _, err := authTokenCiphertextPaths(Service); !errors.Is(err, fail) {
		t.Fatalf("entry info error = %v", err)
	}

	authEntryInfo = func(entry os.DirEntry) (os.FileInfo, error) { return entry.Info() }
	if err := os.Remove(entryPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(entryPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := authTokenCiphertextPaths(Service); err == nil {
		t.Fatal("non-regular auth entry should fail")
	}
}

func TestCrossPlatformCoverageDarwinRemainingValidationAndReadEdges(t *testing.T) {
	origRead := keychainEntryReadFile
	origPaths := keychainAuthTokenCiphertextPaths
	origGet := keyringGet
	origDefault := readDefaultKeychain
	t.Cleanup(func() {
		keychainEntryReadFile = origRead
		keychainAuthTokenCiphertextPaths = origPaths
		keyringGet = origGet
		readDefaultKeychain = origDefault
	})

	t.Setenv(StorageDirEnv, t.TempDir())
	t.Setenv(DisableKeychainEnv, "")
	readDefaultKeychain = func() ([]byte, error) { return nil, nil }
	fail := errors.New("injected failure")
	keyringGet = func(string, string) (string, error) { return "", fail }
	if _, _, err := decryptWithAvailableDEK(Service, []byte("ciphertext")); !IsUnavailable(err) {
		t.Fatalf("unavailable system key error = %v", err)
	}

	keychainEntryReadFile = func(string) ([]byte, error) { return nil, fail }
	if _, err := keyForNewEntry(Service, AccountToken+":corp"); !errors.Is(err, fail) {
		t.Fatalf("anchor read error = %v", err)
	}
	if err := platformSet(Service, AccountToken, "token"); !errors.Is(err, fail) {
		t.Fatalf("existing entry read error = %v", err)
	}

	keychainAuthTokenCiphertextPaths = func(string) ([]string, error) { return nil, fail }
	if err := platformValidateAuthTokenEntries(Service); !errors.Is(err, fail) {
		t.Fatalf("path discovery error = %v", err)
	}
	keychainAuthTokenCiphertextPaths = func(string) ([]string, error) { return []string{"entry"}, nil }
	if err := platformValidateAuthTokenEntries(Service); !errors.Is(err, fail) {
		t.Fatalf("entry read error = %v", err)
	}

	t.Setenv(DisableKeychainEnv, "1")
	key, err := fileDEK(Service)
	if err != nil {
		t.Fatal(err)
	}
	keychainEntryReadFile = func(path string) ([]byte, error) {
		if path == "entry" {
			return []byte("invalid"), nil
		}
		return origRead(path)
	}
	if err := platformValidateAuthTokenEntries(Service); err == nil {
		t.Fatal("invalid ciphertext should fail validation")
	}
	ciphertext, err := encryptData("token", key)
	if err != nil {
		t.Fatal(err)
	}
	keychainEntryReadFile = func(path string) ([]byte, error) {
		if path == "entry" {
			return ciphertext, nil
		}
		return origRead(path)
	}
	if err := ValidateAuthTokenEntries(Service); err != nil {
		t.Fatalf("valid ciphertext = %v", err)
	}
}

func TestCrossPlatformCoverageDarwinFileDEKMigrationFailureEdges(t *testing.T) {
	origPaths := migrationAuthTokenCiphertextPaths
	origRead := migrationReadFile
	origFileDEK := migrationFileDEK
	origEncrypt := migrationEncryptData
	origDecrypt := migrationDecryptData
	origWrite := migrationWriteFile
	origRename := migrationRename
	origGet := keyringGet
	origDefault := readDefaultKeychain
	t.Cleanup(func() {
		migrationAuthTokenCiphertextPaths = origPaths
		migrationReadFile = origRead
		migrationFileDEK = origFileDEK
		migrationEncryptData = origEncrypt
		migrationDecryptData = origDecrypt
		migrationWriteFile = origWrite
		migrationRename = origRename
		keyringGet = origGet
		readDefaultKeychain = origDefault
	})

	t.Setenv(StorageDirEnv, t.TempDir())
	t.Setenv(DisableKeychainEnv, "")
	readDefaultKeychain = func() ([]byte, error) { return nil, nil }
	systemKey := make([]byte, dekBytes)
	for i := range systemKey {
		systemKey[i] = byte(i + 1)
	}
	encodedSystemKey := "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="
	keyringGet = func(string, string) (string, error) { return encodedSystemKey, nil }
	ciphertext, err := encryptData("token", systemKey)
	if err != nil {
		t.Fatal(err)
	}
	fail := errors.New("injected failure")
	entryPath := filepath.Join(StorageDir(Service), safeFileName(AccountToken))

	reset := func() {
		migrationAuthTokenCiphertextPaths = func(string) ([]string, error) { return []string{entryPath}, nil }
		migrationReadFile = func(string) ([]byte, error) { return ciphertext, nil }
		migrationFileDEK = func(string) ([]byte, error) { return make([]byte, dekBytes), nil }
		migrationEncryptData = encryptData
		migrationDecryptData = decryptData
		migrationWriteFile = func(string, []byte, os.FileMode) error { return nil }
		migrationRename = func(string, string) error { return nil }
	}

	reset()
	migrationAuthTokenCiphertextPaths = func(string) ([]string, error) { return nil, fail }
	if _, err := platformMigrateToFileDEK(Service, false); !errors.Is(err, fail) {
		t.Fatalf("path error = %v", err)
	}
	reset()
	migrationReadFile = func(string) ([]byte, error) { return nil, fail }
	if _, err := platformMigrateToFileDEK(Service, false); !errors.Is(err, fail) {
		t.Fatalf("read error = %v", err)
	}
	reset()
	migrationFileDEK = func(string) ([]byte, error) { return nil, fail }
	if _, err := platformMigrateToFileDEK(Service, false); !errors.Is(err, fail) {
		t.Fatalf("file DEK error = %v", err)
	}
	reset()
	migrationEncryptData = func(string, []byte) ([]byte, error) { return nil, fail }
	if _, err := platformMigrateToFileDEK(Service, false); !errors.Is(err, fail) {
		t.Fatalf("encrypt error = %v", err)
	}
	reset()
	migrationDecryptData = func([]byte, []byte) (string, error) { return "", fail }
	if _, err := platformMigrateToFileDEK(Service, false); !errors.Is(err, fail) {
		t.Fatalf("verify error = %v", err)
	}
	reset()
	migrationWriteFile = func(string, []byte, os.FileMode) error { return fail }
	if _, err := platformMigrateToFileDEK(Service, false); !errors.Is(err, fail) {
		t.Fatalf("stage error = %v", err)
	}
	reset()
	migrationRename = func(string, string) error { return fail }
	if _, err := platformMigrateToFileDEK(Service, false); !errors.Is(err, fail) {
		t.Fatalf("rename error = %v", err)
	}
}
