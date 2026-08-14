// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComputeSHA256(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "testfile")
	content := []byte("hello world")
	os.WriteFile(path, content, 0644)

	hash, err := ComputeSHA256(path)
	if err != nil {
		t.Fatalf("ComputeSHA256() error = %v", err)
	}

	expected := sha256.Sum256(content)
	want := hex.EncodeToString(expected[:])
	if hash != want {
		t.Errorf("ComputeSHA256() = %q, want %q", hash, want)
	}
}

func TestVerifySHA256(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "testfile")
	content := []byte("hello world")
	os.WriteFile(path, content, 0644)

	expected := sha256.Sum256(content)
	hash := hex.EncodeToString(expected[:])

	if err := VerifySHA256(path, hash); err != nil {
		t.Errorf("VerifySHA256() with correct hash: error = %v", err)
	}

	if err := VerifySHA256(path, "0000000000000000000000000000000000000000000000000000000000000000"); err == nil {
		t.Error("VerifySHA256() with wrong hash: expected error")
	}
}

func TestCrossPlatformCoverageVerifySHA256RejectsMalformedDigests(t *testing.T) {
	originalOpen, originalCopy := verifyOpenFile, verifyCopy
	t.Cleanup(func() { verifyOpenFile, verifyCopy = originalOpen, originalCopy })
	path := filepath.Join(t.TempDir(), "asset")
	content := []byte("streamed asset")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(content)
	upper := strings.ToUpper(hex.EncodeToString(want[:]))
	if err := VerifySHA256(path, "  "+upper+"  "); err != nil {
		t.Fatalf("uppercase digest rejected: %v", err)
	}
	for _, digest := range []string{"", "deadbeef", strings.Repeat("z", 64)} {
		if err := VerifySHA256(path, digest); err == nil {
			t.Fatalf("malformed digest %q accepted", digest)
		}
	}
	if err := VerifySHA256(path, strings.Repeat("0", 64)); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("wrong digest error = %v", err)
	}
	if err := VerifySHA256(filepath.Join(t.TempDir(), "missing"), strings.Repeat("0", 64)); err == nil {
		t.Fatal("missing file accepted")
	}
	verifyCopy = func(io.Writer, io.Reader) (int64, error) { return 0, errors.New("read failure") }
	if _, err := ComputeSHA256(path); err == nil || !strings.Contains(err.Error(), "计算 SHA256") {
		t.Fatalf("stream read error = %v", err)
	}
}

func TestParseChecksumFile(t *testing.T) {
	content := `abc123def456  dws-darwin-arm64.tar.gz
fedcba987654  dws-linux-amd64.tar.gz
112233445566  dws-skills.zip
`

	result := ParseChecksumFile(content)
	if len(result) != 3 {
		t.Fatalf("len(result) = %d, want 3", len(result))
	}
	if result["dws-darwin-arm64.tar.gz"] != "abc123def456" {
		t.Errorf("darwin hash = %q", result["dws-darwin-arm64.tar.gz"])
	}
	if result["dws-skills.zip"] != "112233445566" {
		t.Errorf("skills hash = %q", result["dws-skills.zip"])
	}
}

func TestVerifyFileFromChecksums(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "myfile.bin")
	content := []byte("test content")
	os.WriteFile(path, content, 0644)

	expected := sha256.Sum256(content)
	hash := hex.EncodeToString(expected[:])
	checksums := hash + "  myfile.bin\nother  otherfile.bin\n"

	if err := VerifyFileFromChecksums(path, "myfile.bin", checksums); err != nil {
		t.Errorf("VerifyFileFromChecksums() with correct hash: error = %v", err)
	}

	if err := VerifyFileFromChecksums(path, "missing.bin", checksums); err == nil {
		t.Error("VerifyFileFromChecksums() with missing file: expected error")
	}
}
