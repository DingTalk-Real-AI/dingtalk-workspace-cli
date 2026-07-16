package security

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"testing"
)

type alwaysErrorReader struct{ err error }

func (r alwaysErrorReader) Read([]byte) (int, error) { return 0, r.err }

func TestCrossPlatformCoverageEncryptRandomSourceErrors(t *testing.T) {
	old := cryptoRandReader
	t.Cleanup(func() { cryptoRandReader = old })
	wantErr := errors.New("entropy unavailable")
	cryptoRandReader = alwaysErrorReader{err: wantErr}
	if _, err := Encrypt([]byte("data"), []byte("password")); !errors.Is(err, wantErr) {
		t.Fatalf("salt error = %v", err)
	}
	cryptoRandReader = io.MultiReader(bytes.NewReader(make([]byte, SaltSize)), alwaysErrorReader{err: wantErr})
	if _, err := Encrypt([]byte("data"), []byte("password")); !errors.Is(err, wantErr) {
		t.Fatalf("nonce error = %v", err)
	}
}

func TestCrossPlatformCoverageCryptoConstructorErrors(t *testing.T) {
	wantErr := errors.New("cipher construction failed")
	failCipher := func([]byte) (cipher.Block, error) {
		return nil, wantErr
	}
	failGCM := func(cipher.Block, int) (cipher.AEAD, error) {
		return nil, wantErr
	}

	if _, err := encryptWithFactories([]byte("data"), []byte("password"), failCipher, cipher.NewGCMWithNonceSize); !errors.Is(err, wantErr) {
		t.Fatalf("Encrypt cipher error = %v", err)
	}
	if _, err := encryptWithFactories([]byte("data"), []byte("password"), aes.NewCipher, failGCM); !errors.Is(err, wantErr) {
		t.Fatalf("Encrypt GCM error = %v", err)
	}

	encrypted, err := Encrypt([]byte("data"), []byte("password"))
	if err != nil {
		t.Fatalf("Encrypt fixture error = %v", err)
	}
	if _, err := decryptWithFactories(encrypted, []byte("password"), failCipher, cipher.NewGCMWithNonceSize); !errors.Is(err, wantErr) {
		t.Fatalf("Decrypt cipher error = %v", err)
	}
	if _, err := decryptWithFactories(encrypted, []byte("password"), aes.NewCipher, failGCM); !errors.Is(err, wantErr) {
		t.Fatalf("Decrypt GCM error = %v", err)
	}
}

func TestCrossPlatformCoverageGetMACAddressEnumerationError(t *testing.T) {
	old := networkInterfaces
	t.Cleanup(func() { networkInterfaces = old })
	wantErr := errors.New("enumeration failed")
	networkInterfaces = func() ([]net.Interface, error) { return nil, wantErr }
	if _, err := GetMACAddress(); !errors.Is(err, wantErr) {
		t.Fatalf("GetMACAddress error = %v", err)
	}
}

type scriptedTokenTempFile struct {
	writeErr error
	syncErr  error
	closeErr error
}

func (f *scriptedTokenTempFile) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return len(p), nil
}
func (f *scriptedTokenTempFile) Sync() error  { return f.syncErr }
func (f *scriptedTokenTempFile) Close() error { return f.closeErr }

func TestCrossPlatformCoverageSecureTokenStorageFailureBoundaries(t *testing.T) {
	oldMkdir, oldMarshal, oldEncrypt := mkdirTokenDir, marshalTokenData, encryptTokenData
	oldOpen, oldRemove, oldRename := openTokenTempFile, removeTokenFile, renameTokenFile
	oldRead, oldDecrypt, oldUnmarshal := readTokenFile, decryptTokenData, unmarshalTokenData
	t.Cleanup(func() {
		mkdirTokenDir, marshalTokenData, encryptTokenData = oldMkdir, oldMarshal, oldEncrypt
		openTokenTempFile, removeTokenFile, renameTokenFile = oldOpen, oldRemove, oldRename
		readTokenFile, decryptTokenData, unmarshalTokenData = oldRead, oldDecrypt, oldUnmarshal
	})
	wantErr := errors.New("synthetic storage failure")
	storage := NewSecureTokenStorage("config", "fallback", "mac")
	data := &TokenData{AccessToken: "token"}

	resetSave := func() *scriptedTokenTempFile {
		file := &scriptedTokenTempFile{}
		mkdirTokenDir = func(string, os.FileMode) error { return nil }
		marshalTokenData = func(any) ([]byte, error) { return []byte(`{}`), nil }
		encryptTokenData = func([]byte, []byte) ([]byte, error) { return []byte("encrypted"), nil }
		openTokenTempFile = func(string, int, os.FileMode) (tokenTempFile, error) { return file, nil }
		removeTokenFile = func(string) error { return nil }
		renameTokenFile = func(string, string) error { return nil }
		return file
	}

	resetSave()
	mkdirTokenDir = func(string, os.FileMode) error { return wantErr }
	if err := storage.SaveToken(data); !errors.Is(err, wantErr) {
		t.Fatalf("mkdir error = %v", err)
	}
	resetSave()
	marshalTokenData = func(any) ([]byte, error) { return nil, wantErr }
	if err := storage.SaveToken(data); !errors.Is(err, wantErr) {
		t.Fatalf("marshal error = %v", err)
	}
	resetSave()
	encryptTokenData = func([]byte, []byte) ([]byte, error) { return nil, wantErr }
	if err := storage.SaveToken(data); !errors.Is(err, wantErr) {
		t.Fatalf("encrypt error = %v", err)
	}
	resetSave()
	openTokenTempFile = func(string, int, os.FileMode) (tokenTempFile, error) { return nil, wantErr }
	if err := storage.SaveToken(data); !errors.Is(err, wantErr) {
		t.Fatalf("open error = %v", err)
	}
	file := resetSave()
	file.writeErr = wantErr
	if err := storage.SaveToken(data); !errors.Is(err, wantErr) {
		t.Fatalf("write error = %v", err)
	}
	file = resetSave()
	file.syncErr = wantErr
	if err := storage.SaveToken(data); !errors.Is(err, wantErr) {
		t.Fatalf("sync error = %v", err)
	}
	file = resetSave()
	file.closeErr = wantErr
	if err := storage.SaveToken(data); !errors.Is(err, wantErr) {
		t.Fatalf("close error = %v", err)
	}
	resetSave()
	renameTokenFile = func(string, string) error { return wantErr }
	if err := storage.SaveToken(data); !errors.Is(err, wantErr) {
		t.Fatalf("rename error = %v", err)
	}

	readTokenFile = func(string) ([]byte, error) { return []byte("encrypted"), nil }
	decryptTokenData = func([]byte, []byte) ([]byte, error) { return nil, wantErr }
	if _, err := storage.LoadToken(); !errors.Is(err, wantErr) {
		t.Fatalf("decrypt error = %v", err)
	}
	decryptTokenData = func([]byte, []byte) ([]byte, error) { return []byte(`{}`), nil }
	unmarshalTokenData = func([]byte, any) error { return wantErr }
	if _, err := storage.LoadToken(); !errors.Is(err, wantErr) {
		t.Fatalf("unmarshal error = %v", err)
	}

	removeTokenFile = func(path string) error {
		if strings.HasSuffix(path, ".tmp") {
			return nil
		}
		return wantErr
	}
	if err := storage.DeleteToken(); !errors.Is(err, wantErr) {
		t.Fatalf("DeleteToken error = %v", err)
	}
	if err := DeleteEncryptedData("config", "fallback"); !errors.Is(err, wantErr) {
		t.Fatalf("DeleteEncryptedData error = %v", err)
	}
}
