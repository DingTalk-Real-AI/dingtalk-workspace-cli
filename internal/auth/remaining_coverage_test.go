// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package auth

import (
	"archive/tar"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCrossPlatformCoverageMigrateKeychainToFileDEKCoverage(t *testing.T) {
	orig := authMigrateToFileDEK
	t.Cleanup(func() { authMigrateToFileDEK = orig })

	authMigrateToFileDEK = func(string, bool) (int, error) { return 7, nil }
	if got, err := MigrateKeychainToFileDEK(t.TempDir(), true); err != nil || got != 7 {
		t.Fatalf("migration = %d, %v", got, err)
	}
	fail := errors.New("migration failed")
	authMigrateToFileDEK = func(string, bool) (int, error) { return 0, fail }
	if _, err := MigrateKeychainToFileDEK(t.TempDir(), false); !errors.Is(err, fail) {
		t.Fatalf("migration error = %v", err)
	}
}

func TestCrossPlatformCoverageTokenPersistencePreflightRemainingEdges(t *testing.T) {
	origHooks := edition.Get()
	origGet := authKeychainGet
	origValidate := authValidateEntries
	origProfilesReadFile := profilesReadFile
	t.Cleanup(func() {
		edition.Override(origHooks)
		authKeychainGet = origGet
		authValidateEntries = origValidate
		profilesReadFile = origProfilesReadFile
	})

	edition.Override(&edition.Hooks{SaveToken: func(string, []byte) error { return nil }})
	if err := preflightTokenPersistence(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := preflightTokenRefreshPersistence(t.TempDir(), &TokenData{CorpID: "corp"}); err != nil {
		t.Fatal(err)
	}

	edition.Override(&edition.Hooks{})
	authKeychainGet = func(string, string) (string, error) { return "", nil }
	authValidateEntries = func(string) error { return nil }
	profileFail := errors.New("profile load failed")
	profilesReadFile = func(string) ([]byte, error) { return nil, profileFail }
	if err := preflightTokenPersistence(t.TempDir()); !errors.Is(err, profileFail) {
		t.Fatalf("profile load error = %v", err)
	}
	profilesReadFile = origProfilesReadFile

	dir := t.TempDir()
	rawProfiles := []byte(`{"version":1,"profiles":[{"name":"blank","corpId":"   "},{"name":"first","corpId":"corp"},{"name":"duplicate","corpId":"corp"}]}`)
	if err := os.WriteFile(ProfilesPath(dir), rawProfiles, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := preflightTokenPersistence(dir); err != nil {
		t.Fatalf("blank and duplicate profiles = %v", err)
	}

	fail := errors.New("unreadable")
	authKeychainGet = func(_ string, account string) (string, error) {
		if account == TokenAccountForCorpID("corp") {
			return "", fail
		}
		return "", nil
	}
	if err := preflightTokenPersistence(dir); !errors.Is(err, fail) {
		t.Fatalf("profile slot error = %v", err)
	}
	if err := preflightTokenRefreshPersistence(t.TempDir(), &TokenData{CorpID: "corp"}); !errors.Is(err, fail) {
		t.Fatalf("refresh profile slot error = %v", err)
	}

	authKeychainGet = func(string, string) (string, error) { return "", nil }
	authValidateEntries = func(string) error { return fail }
	if err := preflightTokenPersistence(dir); !errors.Is(err, fail) {
		t.Fatalf("inventory error = %v", err)
	}
}

func TestCrossPlatformCoveragePortableExportRemainingEdges(t *testing.T) {
	origGOOS := portableRuntimeGOOS
	origStat := portableStat
	origConfigFiles := portableConfigFilesForExport
	origManifest := portableWriteManifest
	origAddDir := portableAddDir
	origAddFile := portableAddFile
	origKeychainGet := authKeychainGet
	origKeychainExists := authKeychainExists
	t.Cleanup(func() {
		portableRuntimeGOOS = origGOOS
		portableStat = origStat
		portableConfigFilesForExport = origConfigFiles
		portableWriteManifest = origManifest
		portableAddDir = origAddDir
		portableAddFile = origAddFile
		authKeychainGet = origKeychainGet
		authKeychainExists = origKeychainExists
	})
	portableRuntimeGOOS = func() string { return "linux" }

	t.Setenv(keychain.StorageDirEnv, t.TempDir())
	t.Setenv(keychain.DisableKeychainEnv, "1")
	authKeychainGet = func(string, string) (string, error) { return "", nil }
	authKeychainExists = func(string, string) bool { return false }
	configDir := t.TempDir()
	encPath := filepath.Join(keychain.StorageDir(keychain.Service), keychain.AccountToken+".enc")
	portableStat = func(path string) (os.FileInfo, error) {
		if path == encPath {
			return os.Stat(configDir)
		}
		return nil, os.ErrNotExist
	}
	if !PortableAuthTargetPopulated(configDir) {
		t.Fatal("encrypted target should count as populated")
	}
	portableStat = origStat

	if err := os.MkdirAll(filepath.Dir(encPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(encPath, []byte("encrypted"), 0o600); err != nil {
		t.Fatal(err)
	}
	authKeychainGet = func(string, string) (string, error) {
		return `{"corp_id":"corp","access_token":"token"}`, nil
	}
	if err := os.WriteFile(filepath.Join(configDir, "app.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	fail := errors.New("export failed")
	reset := func() {
		portableConfigFilesForExport = origConfigFiles
		portableWriteManifest = origManifest
		portableAddDir = origAddDir
		portableAddFile = origAddFile
	}

	reset()
	portableConfigFilesForExport = func(string) ([]string, error) { return nil, fail }
	if err := ExportPortableAuthBundle(configDir, &bytes.Buffer{}); !errors.Is(err, fail) {
		t.Fatalf("config scan error = %v", err)
	}
	reset()
	portableWriteManifest = func(*tar.Writer, portableAuthBundleManifest) error { return fail }
	if err := ExportPortableAuthBundle(configDir, &bytes.Buffer{}); !errors.Is(err, fail) {
		t.Fatalf("manifest error = %v", err)
	}
	reset()
	portableAddDir = func(*tar.Writer, string, string) error { return fail }
	if err := ExportPortableAuthBundle(configDir, &bytes.Buffer{}); !errors.Is(err, fail) {
		t.Fatalf("directory error = %v", err)
	}
	reset()
	portableAddFile = func(*tar.Writer, string, string) error { return fail }
	if err := ExportPortableAuthBundle(configDir, &bytes.Buffer{}); !errors.Is(err, fail) {
		t.Fatalf("config file error = %v", err)
	}
}

func TestCrossPlatformCoverageLoadTokenDataForProfileLegacyFailure(t *testing.T) {
	origResolve := tokenResolveProfile
	origLoadCorp := tokenLoadKeychainForCorpID
	origLoadLegacy := tokenLoadKeychain
	t.Cleanup(func() {
		tokenResolveProfile = origResolve
		tokenLoadKeychainForCorpID = origLoadCorp
		tokenLoadKeychain = origLoadLegacy
	})

	tokenResolveProfile = func(string, string) (*Profile, error) {
		return &Profile{Name: "current", CorpID: "corp"}, nil
	}
	tokenLoadKeychainForCorpID = func(string) (*TokenData, error) { return nil, ErrTokenDataNotFound }
	fail := errors.New("legacy read failed")
	tokenLoadKeychain = func() (*TokenData, error) { return nil, fail }
	if _, err := LoadTokenDataForProfile(t.TempDir(), ""); !errors.Is(err, fail) {
		t.Fatalf("legacy error = %v", err)
	}
}
