//go:build darwin

package keychain

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

var errKeychainInjected = errors.New("injected keychain failure")

func TestCrossPlatformCoverageUnavailableErrorEdges(t *testing.T) {
	var nilErr *UnavailableError
	if nilErr.Error() != "" || nilErr.Unwrap() != nil {
		t.Fatal("nil unavailable error should be empty")
	}
	for _, tc := range []struct {
		err  *UnavailableError
		want string
	}{
		{&UnavailableError{}, "keychain unavailable"},
		{&UnavailableError{Err: errKeychainInjected}, errKeychainInjected.Error()},
		{&UnavailableError{Op: "read"}, "read: keychain unavailable"},
		{&UnavailableError{Op: "read", Err: errKeychainInjected}, "read: injected keychain failure"},
	} {
		if got := tc.err.Error(); got != tc.want {
			t.Fatalf("Error() = %q, want %q", got, tc.want)
		}
		_ = tc.err.Unwrap()
	}
	_ = Diagnose()
}

func TestCrossPlatformCoverageFileDEKFailureEdges(t *testing.T) {
	origRead := keychainReadFile
	origMkdir := keychainMkdirAll
	origWrite := keychainWriteFile
	origRename := keychainRename
	origRemove := keychainRemove
	origRand := keychainRandRead
	t.Cleanup(func() {
		keychainReadFile = origRead
		keychainMkdirAll = origMkdir
		keychainWriteFile = origWrite
		keychainRename = origRename
		keychainRemove = origRemove
		keychainRandRead = origRand
	})
	t.Setenv(StorageDirEnv, t.TempDir())

	valid := bytesOf(7, dekBytes)
	keychainReadFile = func(string) ([]byte, error) { return valid, nil }
	if got, err := fileDEK("existing"); err != nil || len(got) != dekBytes {
		t.Fatalf("existing fileDEK = %d, %v", len(got), err)
	}

	keychainReadFile = func(string) ([]byte, error) { return nil, os.ErrNotExist }
	keychainMkdirAll = func(string, os.FileMode) error { return errKeychainInjected }
	if _, err := fileDEK("mkdir"); err == nil {
		t.Fatal("mkdir error expected")
	}
	keychainMkdirAll = func(string, os.FileMode) error { return nil }
	keychainRandRead = func([]byte) (int, error) { return 0, errKeychainInjected }
	if _, err := fileDEK("rand"); err == nil {
		t.Fatal("rand error expected")
	}
	keychainRandRead = origRand
	keychainWriteFile = func(string, []byte, os.FileMode) error { return errKeychainInjected }
	if _, err := fileDEK("write"); err == nil {
		t.Fatal("write error expected")
	}
	keychainWriteFile = func(string, []byte, os.FileMode) error { return nil }
	keychainRename = func(string, string) error { return errKeychainInjected }
	reads := 0
	keychainReadFile = func(string) ([]byte, error) {
		reads++
		if reads == 1 {
			return nil, os.ErrNotExist
		}
		return valid, nil
	}
	if got, err := fileDEK("rename-race"); err != nil || len(got) != dekBytes {
		t.Fatalf("rename race = %d, %v", len(got), err)
	}
	reads = 0
	keychainReadFile = func(string) ([]byte, error) { return nil, os.ErrNotExist }
	if _, err := fileDEK("rename"); err == nil {
		t.Fatal("rename error expected")
	}

	keychainReadFile = func(string) ([]byte, error) { return valid, nil }
	if got, err := fileDEKReadOnly("valid"); err != nil || len(got) != dekBytes {
		t.Fatalf("read-only valid = %d, %v", len(got), err)
	}
	keychainReadFile = func(string) ([]byte, error) { return nil, os.ErrNotExist }
	if _, err := fileDEKReadOnly("missing"); !IsDEKMissing(err) {
		t.Fatalf("missing error = %v", err)
	}
	keychainReadFile = func(string) ([]byte, error) { return nil, errKeychainInjected }
	if _, err := fileDEKReadOnly("read"); err == nil {
		t.Fatal("read error expected")
	}
	keychainReadFile = func(string) ([]byte, error) { return []byte("short"), nil }
	if _, err := fileDEKReadOnly("short"); !IsDEKMissing(err) {
		t.Fatalf("short error = %v", err)
	}
}

func TestCrossPlatformCoverageDarwinStorageAndDiagnosticEdges(t *testing.T) {
	t.Setenv(DisableKeychainEnv, "")
	origHome := keychainUserHomeDir
	origReadDefault := readDefaultKeychain
	origStat := keychainStat
	t.Cleanup(func() {
		keychainUserHomeDir = origHome
		readDefaultKeychain = origReadDefault
		keychainStat = origStat
	})
	t.Setenv(StorageDirEnv, "")
	_, _ = origReadDefault()
	keychainUserHomeDir = func() (string, error) { return "/home/test", nil }
	if got := StorageDir("svc"); !strings.Contains(got, "Application Support") {
		t.Fatalf("StorageDir = %q", got)
	}
	keychainUserHomeDir = func() (string, error) { return "", errKeychainInjected }
	if got := StorageDir("svc"); got != filepath.Join(".dws", "keychain", "svc") {
		t.Fatalf("fallback StorageDir = %q", got)
	}
	if safeFileName("a/b:c") != "a_b_c.enc" {
		t.Fatal("safe file name mismatch")
	}
	for input, want := range map[string]string{
		"":                 "",
		"/plain/path":      "/plain/path",
		`"/quoted/path"`:   "/quoted/path",
		`"invalid\xquote"`: `invalid\xquote`,
	} {
		if got := defaultKeychainPathFromSecurityOutput([]byte(input)); got != want {
			t.Fatalf("path(%q) = %q, want %q", input, got, want)
		}
	}

	readDefaultKeychain = func() ([]byte, error) { return nil, errKeychainInjected }
	if err := checkDefaultKeychainAvailable(); err != nil {
		t.Fatalf("security command failure is tolerated: %v", err)
	}
	d := platformDiagnose()
	if d.OK || d.Reason != "keychain_check_failed" {
		t.Fatalf("diagnostic = %#v", d)
	}
	readDefaultKeychain = func() ([]byte, error) { return nil, nil }
	if err := checkDefaultKeychainAvailable(); err != nil {
		t.Fatal(err)
	}
	d = platformDiagnose()
	if d.OK || d.Reason != "keychain_unavailable" {
		t.Fatalf("empty diagnostic = %#v", d)
	}
	readDefaultKeychain = func() ([]byte, error) { return []byte(`"/keychain"`), nil }
	keychainStat = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	if !IsUnavailable(checkDefaultKeychainAvailable()) {
		t.Fatal("missing keychain should be unavailable")
	}
	d = platformDiagnose()
	if d.OK || d.Reason != "keychain_unavailable" {
		t.Fatalf("missing diagnostic = %#v", d)
	}
	keychainStat = func(string) (os.FileInfo, error) { return nil, errKeychainInjected }
	d = platformDiagnose()
	if d.OK || d.Reason != "keychain_check_failed" {
		t.Fatalf("stat diagnostic = %#v", d)
	}
	keychainStat = func(string) (os.FileInfo, error) { return nil, nil }
	if err := checkDefaultKeychainAvailable(); err != nil {
		t.Fatal(err)
	}
	d = platformDiagnose()
	if !d.OK {
		t.Fatalf("healthy diagnostic = %#v", d)
	}
	t.Setenv(DisableKeychainEnv, "1")
	d = platformDiagnose()
	if !d.OK || d.Detail["mode"] != "file_dek" {
		t.Fatalf("file diagnostic = %#v", d)
	}
}

func TestCrossPlatformCoverageDarwinDEKKeyringEdges(t *testing.T) {
	origReadDefault := readDefaultKeychain
	origStat := keychainStat
	origGet := keyringGet
	origSet := keyringSet
	origRand := keychainRandRead
	t.Cleanup(func() {
		readDefaultKeychain = origReadDefault
		keychainStat = origStat
		keyringGet = origGet
		keyringSet = origSet
		keychainRandRead = origRand
	})
	t.Setenv(DisableKeychainEnv, "")
	readDefaultKeychain = func() ([]byte, error) { return []byte(`"/keychain"`), nil }
	keychainStat = func(string) (os.FileInfo, error) { return nil, nil }
	valid := bytesOf(3, dekBytes)
	encoded := base64.StdEncoding.EncodeToString(valid)

	keyringGet = func(string, string) (string, error) { return encoded, nil }
	if got, err := getSystemDEKReadOnly("valid"); err != nil || len(got) != dekBytes {
		t.Fatalf("read valid = %d, %v", len(got), err)
	}
	if got, err := getOrCreateDEK("valid"); err != nil || len(got) != dekBytes {
		t.Fatalf("create valid = %d, %v", len(got), err)
	}
	for _, value := range []string{"%%%", base64.StdEncoding.EncodeToString([]byte("short"))} {
		keyringGet = func(string, string) (string, error) { return value, nil }
		if _, err := getSystemDEKReadOnly("invalid"); !IsDEKMissing(err) {
			t.Fatalf("invalid read error = %v", err)
		}
	}
	keyringGet = func(string, string) (string, error) { return "", keyring.ErrNotFound }
	if _, err := getSystemDEKReadOnly("missing"); !IsDEKMissing(err) {
		t.Fatalf("missing read error = %v", err)
	}
	keyringGet = func(string, string) (string, error) { return "", errKeychainInjected }
	if _, err := getSystemDEKReadOnly("unavailable"); !IsUnavailable(err) {
		t.Fatalf("unavailable read error = %v", err)
	}
	if _, err := getOrCreateDEK("unavailable"); !IsUnavailable(err) {
		t.Fatalf("unavailable create error = %v", err)
	}

	setValue := ""
	keyringGet = func(string, string) (string, error) { return "invalid", nil }
	keyringSet = func(_, _, value string) error { setValue = value; return nil }
	if got, err := getOrCreateDEK("generate-invalid"); err != nil || len(got) != dekBytes || setValue == "" {
		t.Fatalf("generate invalid = %d, %v, %q", len(got), err, setValue)
	}
	keyringGet = func(string, string) (string, error) { return "", keyring.ErrNotFound }
	if got, err := getOrCreateDEK("generate-missing"); err != nil || len(got) != dekBytes {
		t.Fatalf("generate missing = %d, %v", len(got), err)
	}
	keychainRandRead = func([]byte) (int, error) { return 0, errKeychainInjected }
	if _, err := getOrCreateDEK("rand"); err == nil {
		t.Fatal("rand error expected")
	}
	keychainRandRead = origRand
	keyringSet = func(string, string, string) error { return errKeychainInjected }
	if _, err := getOrCreateDEK("set"); !IsUnavailable(err) {
		t.Fatalf("set error = %v", err)
	}

	timeoutRuntime := snapshotDarwinKeychainRuntime()
	timeoutRuntime.timeout = time.Millisecond
	release := make(chan struct{})
	timeoutRuntime.get = func(string, string) (string, error) {
		<-release
		return "", keyring.ErrNotFound
	}
	timeoutRuntime.randRead = func(data []byte) (int, error) {
		copy(data, bytesOf(4, len(data)))
		return len(data), nil
	}
	timeoutRuntime.set = func(string, string, string) error { return nil }
	_, err, readDone := getSystemDEKReadOnlyWithRuntime("timeout", timeoutRuntime)
	if !IsUnavailable(err) {
		t.Fatalf("read timeout = %v", err)
	}
	_, err, createDone := getOrCreateDEKWithRuntime("timeout", timeoutRuntime)
	if !IsUnavailable(err) {
		t.Fatalf("create timeout = %v", err)
	}
	close(release)
	waitDarwinKeychainWorkerDone(t, readDone)
	waitDarwinKeychainWorkerDone(t, createDone)

	panicRuntime := snapshotDarwinKeychainRuntime()
	panicRuntime.get = func(string, string) (string, error) { panic("keyring panic") }
	_, err, readDone = getSystemDEKReadOnlyWithRuntime("panic", panicRuntime)
	if !IsUnavailable(err) {
		t.Fatalf("read panic = %v", err)
	}
	waitDarwinKeychainWorkerDone(t, readDone)
	_, err, createDone = getOrCreateDEKWithRuntime("panic", panicRuntime)
	if !IsUnavailable(err) {
		t.Fatalf("create panic = %v", err)
	}
	waitDarwinKeychainWorkerDone(t, createDone)

	keychainStat = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	if _, err := getSystemDEKReadOnly("missing-keychain"); !IsUnavailable(err) {
		t.Fatalf("preflight read = %v", err)
	}
	if _, err := getOrCreateDEK("missing-keychain"); !IsUnavailable(err) {
		t.Fatalf("preflight create = %v", err)
	}
}

func waitDarwinKeychainWorkerDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for keychain worker shutdown")
	}
}

func TestCrossPlatformCoverageDarwinCryptoAndPlatformFailureEdges(t *testing.T) {
	origRead := keychainReadFile
	origMkdir := keychainMkdirAll
	origWrite := keychainWriteFile
	origRename := keychainRename
	origRemove := keychainRemove
	origRand := keychainRandRead
	t.Cleanup(func() {
		keychainReadFile = origRead
		keychainMkdirAll = origMkdir
		keychainWriteFile = origWrite
		keychainRename = origRename
		keychainRemove = origRemove
		keychainRandRead = origRand
	})
	t.Setenv(StorageDirEnv, t.TempDir())
	t.Setenv(DisableKeychainEnv, "1")
	if _, err := encryptData("x", []byte("short")); err == nil {
		t.Fatal("invalid encryption key expected")
	}
	keychainRandRead = func([]byte) (int, error) { return 0, errKeychainInjected }
	if _, err := encryptData("x", bytesOf(1, dekBytes)); err == nil {
		t.Fatal("encryption random error expected")
	}
	keychainRandRead = origRand
	key := bytesOf(2, dekBytes)
	ciphertext, err := encryptData("secret", key)
	if err != nil {
		t.Fatal(err)
	}
	if plaintext, err := decryptData(ciphertext, key); err != nil || plaintext != "secret" {
		t.Fatalf("decrypt = %q, %v", plaintext, err)
	}
	if _, err := decryptData([]byte("short"), key); err == nil {
		t.Fatal("short ciphertext error expected")
	}
	if _, err := decryptData(ciphertext, []byte("short")); err == nil {
		t.Fatal("invalid decryption key expected")
	}
	if _, err := decryptData(ciphertext, bytesOf(9, dekBytes)); err == nil {
		t.Fatal("authentication error expected")
	}

	keychainReadFile = func(string) ([]byte, error) { return nil, errKeychainInjected }
	if _, err := platformGet("svc", "account"); err == nil {
		t.Fatal("read error expected")
	}
	keychainReadFile = func(path string) ([]byte, error) {
		if strings.HasSuffix(path, ".enc") {
			return []byte("bad ciphertext"), nil
		}
		return nil, os.ErrNotExist
	}
	if _, err := platformGet("svc", "account"); !IsDEKMissing(err) {
		t.Fatalf("DEK read error = %v", err)
	}
	keychainReadFile = func(path string) ([]byte, error) {
		if strings.HasSuffix(path, ".enc") {
			return []byte("bad ciphertext"), nil
		}
		return bytesOf(1, dekBytes), nil
	}
	if _, err := platformGet("svc", "account"); err == nil {
		t.Fatal("decrypt error expected")
	}

	keychainReadFile = func(string) ([]byte, error) { return nil, os.ErrNotExist }
	keychainMkdirAll = func(string, os.FileMode) error { return errKeychainInjected }
	if err := platformSet("dek-error", "a", "v"); err == nil {
		t.Fatal("DEK error expected")
	}
	keychainMkdirAll = origMkdir
	if got, err := getOrCreateDEK("direct"); err != nil || len(got) != dekBytes {
		t.Fatalf("getOrCreateDEK = %d, %v", len(got), err)
	}
	keychainReadFile = func(string) ([]byte, error) { return bytesOf(1, dekBytes), nil }
	keychainMkdirAll = func(string, os.FileMode) error { return errKeychainInjected }
	if err := platformSet("mkdir", "a", "v"); err == nil {
		t.Fatal("mkdir error expected")
	}
	keychainMkdirAll = func(string, os.FileMode) error { return nil }
	keychainRandRead = func(p []byte) (int, error) {
		if len(p) == dekBytes {
			for i := range p {
				p[i] = 1
			}
			return len(p), nil
		}
		return 0, errKeychainInjected
	}
	if err := platformSet("encrypt", "a", "v"); err == nil {
		t.Fatal("encrypt error expected")
	}
	keychainRandRead = origRand
	keychainWriteFile = func(string, []byte, os.FileMode) error { return errKeychainInjected }
	if err := platformSet("write", "a", "v"); err == nil {
		t.Fatal("write error expected")
	}
	keychainWriteFile = func(string, []byte, os.FileMode) error { return nil }
	keychainRename = func(string, string) error { return errKeychainInjected }
	if err := platformSet("rename", "a", "v"); err == nil {
		t.Fatal("rename error expected")
	}
	keychainRemove = func(string) error { return errKeychainInjected }
	if err := platformRemove("remove", "a"); err == nil {
		t.Fatal("remove error expected")
	}
}

func TestCrossPlatformCoverageMigrationEdges(t *testing.T) {
	origMAC := migrateGetMACAddress
	origDecrypt := migrateDecrypt
	origExists := migrateExists
	origSet := migrateSet
	origRead := keychainReadFile
	origStat := keychainStat
	origRename := keychainRename
	origRemove := keychainRemove
	t.Cleanup(func() {
		migrateGetMACAddress = origMAC
		migrateDecrypt = origDecrypt
		migrateExists = origExists
		migrateSet = origSet
		keychainReadFile = origRead
		keychainStat = origStat
		keychainRename = origRename
		keychainRemove = origRemove
	})

	migrateExists = func(string, string) bool { return true }
	if got := MigrateFromLegacy(t.TempDir()); got.Migrated || got.Error != nil {
		t.Fatalf("already migrated = %#v", got)
	}
	migrateExists = func(string, string) bool { return false }
	keychainStat = func(string) (os.FileInfo, error) { return nil, errKeychainInjected }
	migrateGetMACAddress = func() (string, error) { return "", errKeychainInjected }
	got := MigrateFromLegacy(t.TempDir())
	if !got.NeedRelogin || got.Error == nil || got.FromPath == "" {
		t.Fatalf("MAC failure = %#v", got)
	}
	if _, err := loadLegacyData(t.TempDir()); err == nil {
		t.Fatal("MAC failure expected")
	}

	migrateGetMACAddress = func() (string, error) { return "mac", nil }
	keychainReadFile = func(string) ([]byte, error) { return nil, errKeychainInjected }
	if _, err := loadLegacyData(t.TempDir()); err == nil {
		t.Fatal("read failure expected")
	}
	keychainReadFile = func(string) ([]byte, error) { return []byte("cipher"), nil }
	migrateDecrypt = func([]byte, []byte) ([]byte, error) { return nil, errKeychainInjected }
	if _, err := loadLegacyData(t.TempDir()); err == nil {
		t.Fatal("decrypt failure expected")
	}
	migrateDecrypt = func([]byte, []byte) ([]byte, error) { return []byte("not-json"), nil }
	if _, err := loadLegacyData(t.TempDir()); err == nil {
		t.Fatal("JSON failure expected")
	}
	migrateDecrypt = func([]byte, []byte) ([]byte, error) { return []byte(`{"token":"ok"}`), nil }
	data, err := loadLegacyData(t.TempDir())
	if err != nil || data["token"] != "ok" {
		t.Fatalf("legacy data = %#v, %v", data, err)
	}

	keychainStat = func(string) (os.FileInfo, error) { return nil, nil }
	migrateSet = func(string, string, string) error { return errKeychainInjected }
	got = MigrateFromLegacy(t.TempDir())
	if got.Error == nil || got.NeedRelogin {
		t.Fatalf("set failure = %#v", got)
	}
	migrateSet = func(string, string, string) error { return nil }
	keychainRename = func(string, string) error { return nil }
	got = MigrateFromLegacy(t.TempDir())
	if !got.Migrated || got.BackupPath == "" {
		t.Fatalf("migration success = %#v", got)
	}
	keychainRename = func(string, string) error { return errKeychainInjected }
	keychainRemove = func(string) error { return nil }
	got = MigrateFromLegacy(t.TempDir())
	if !got.Migrated || got.BackupPath != "" {
		t.Fatalf("rename fallback = %#v", got)
	}

	keychainRemove = func(string) error { return os.ErrNotExist }
	if err := CleanupLegacyBackup(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	keychainRemove = func(string) error { return errKeychainInjected }
	if err := CleanupLegacyBackup(t.TempDir()); err == nil {
		t.Fatal("cleanup error expected")
	}
	keychainRemove = func(string) error { return nil }
	if err := CleanupLegacyBackup(t.TempDir()); err != nil {
		t.Fatal(err)
	}
}

func bytesOf(value byte, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = value
	}
	return b
}
