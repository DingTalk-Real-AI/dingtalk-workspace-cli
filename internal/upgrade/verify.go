// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

// VerifySHA256 verifies a file against its expected SHA256 hash.
func VerifySHA256(filePath, expectedHash string) error {
	expectedHash = strings.ToLower(strings.TrimSpace(expectedHash))
	decoded, err := hex.DecodeString(expectedHash)
	if err != nil || len(decoded) != sha256.Size {
		return fmt.Errorf("无效的 SHA256 校验值")
	}
	actual, err := ComputeSHA256(filePath)
	if err != nil {
		return err
	}

	if actual != expectedHash {
		return fmt.Errorf("SHA256 mismatch: want %s, got %s", expectedHash[:16], actual[:16])
	}
	return nil
}

var verifyOpen = os.Open

// ComputeSHA256 computes the SHA256 hash of a file.
func ComputeSHA256(filePath string) (string, error) {
	file, err := verifyOpen(filePath)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("读取文件失败: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// ParseChecksumFile parses a SHA256SUMS-style file.
// Each line: "hash  filename" (two spaces or whitespace separated).
// Returns a map of filename -> lowercase hex hash.
func ParseChecksumFile(content string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			hash := strings.ToLower(parts[0])
			filename := parts[1]
			result[filename] = hash
		}
	}
	return result
}

// VerifyFileFromChecksums verifies a downloaded file against checksums.txt content.
func VerifyFileFromChecksums(filePath, filename, checksumsContent string) error {
	checksums := ParseChecksumFile(checksumsContent)
	expectedHash, ok := checksums[filename]
	if !ok {
		return fmt.Errorf("在校验和文件中未找到 %s", filename)
	}
	return VerifySHA256(filePath, expectedHash)
}
