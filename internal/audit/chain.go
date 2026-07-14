package audit

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Chain computes the L1 sha256 tamper-evidence chain. It is deliberately
// stateless: every seal derives prev_hash from the last record already present
// in the current day's file rather than from an in-memory or sidecar cursor.
//
// This makes the chain correct in two cases the previous global-sidecar design
// broke:
//   - cross-date: each day rotates to a fresh file whose first record chains
//     from "", so every file is independently verifiable by VerifyFile.
//   - cross-process: because the caller holds an exclusive inter-process lock on
//     the file while sealing, the tail read reflects records written by any
//     other dws process, so concurrent writers cannot fork the chain.
type Chain struct{}

// NewChain keeps the historical constructor signature. The directory argument
// is no longer needed because prev_hash is derived from the target file.
func NewChain(string) *Chain { return &Chain{} }

// SealFromFile reads the hash of the last record in f (the current day's audit
// file) and returns the prev_hash / hash pair for the event whose hash-free
// body is provided. The caller must hold the file lock.
func (c *Chain) SealFromFile(f *os.File, body []byte) (prevHash, hash string, err error) {
	prevHash, err = lastRecordHash(f)
	if err != nil {
		return "", "", err
	}
	return prevHash, ComputeHash(prevHash, body), nil
}

// lastRecordHash returns the "hash" field of the final non-empty JSONL record in
// f, or "" when the file is empty. It reads only the tail of the file so cost
// does not grow with file size.
func lastRecordHash(f *os.File) (string, error) {
	fi, err := f.Stat()
	if err != nil {
		return "", err
	}
	size := fi.Size()
	if size == 0 {
		return "", nil
	}

	const tailWindow = 64 * 1024
	start := size - tailWindow
	if start < 0 {
		start = 0
	}
	buf := make([]byte, size-start)
	if _, err := f.ReadAt(buf, start); err != nil && err != io.EOF {
		return "", err
	}

	// Trim trailing newlines, then isolate the last line within the window.
	end := len(buf)
	for end > 0 && (buf[end-1] == '\n' || buf[end-1] == '\r') {
		end--
	}
	if end == 0 {
		return "", nil
	}
	lineStart := end
	for lineStart > 0 && buf[lineStart-1] != '\n' {
		lineStart--
	}
	last := buf[lineStart:end]

	var rec struct {
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal(last, &rec); err != nil {
		// The last record spilled past our tail window (pathologically large
		// line). Fall back to a full scan for correctness.
		if lineStart == 0 && start > 0 {
			return lastRecordHashFullScan(f)
		}
		return "", fmt.Errorf("audit: parse last record: %w", err)
	}
	return rec.Hash, nil
}

func lastRecordHashFullScan(f *os.File) (string, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 8*1024*1024)
	var last []byte
	for scanner.Scan() {
		if b := scanner.Bytes(); len(b) > 0 {
			last = append(last[:0], b...)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	if len(last) == 0 {
		return "", nil
	}
	var rec struct {
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal(last, &rec); err != nil {
		return "", fmt.Errorf("audit: parse last record: %w", err)
	}
	return rec.Hash, nil
}

func VerifyFile(path string) (valid bool, brokenAt int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return false, 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 8*1024*1024)

	prevHash := ""
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()

		var evt struct {
			PrevHash string `json:"prev_hash"`
			Hash     string `json:"hash"`
		}
		if err := json.Unmarshal(line, &evt); err != nil {
			return false, lineNum, fmt.Errorf("line %d: invalid JSON: %w", lineNum, err)
		}

		if evt.PrevHash != prevHash {
			return false, lineNum, fmt.Errorf("line %d: prev_hash mismatch", lineNum)
		}

		body := stripHashFields(line)
		h := sha256.New()
		h.Write([]byte(prevHash))
		h.Write(body)
		expected := hex.EncodeToString(h.Sum(nil))

		if evt.Hash != expected {
			return false, lineNum, fmt.Errorf("line %d: hash mismatch", lineNum)
		}

		prevHash = evt.Hash
	}
	if err := scanner.Err(); err != nil {
		return false, lineNum, err
	}
	return true, 0, nil
}

func stripHashFields(line []byte) []byte {
	var evt Event
	if err := json.Unmarshal(line, &evt); err != nil {
		return line
	}
	evt.PrevHash = ""
	evt.Hash = ""
	out, err := json.Marshal(evt)
	if err != nil {
		return line
	}
	return out
}

func ComputeHash(prevHash string, eventJSON []byte) string {
	h := sha256.New()
	h.Write([]byte(prevHash))
	h.Write(eventJSON)
	return hex.EncodeToString(h.Sum(nil))
}
