// Package main demonstrates how to integrate the SafeChat Go SDK
// into the DingTalk Workspace CLI or any other Go application.
//
// Two usage modes:
//
//  1. Single-action mode (default)
//     Run one of: encrypt-msg / decrypt-msg / encrypt-file / decrypt-file /
//     encrypt-buf / decrypt-buf via -action flag.
//
//  2. Full-test mode (-test-all)
//     Exercise all 3 encryption APIs (Msg / File / Buffer) in one run,
//     verify round-trip (decrypt == original), and print a summary.
//
// Key cache files
//
//	The SDK persists negotiated keys under -data directory:
//	  ahflag_256.store   local key identifier
//	  ahkey_256.store    encrypted key material
//
// Build:
//
//	go build -o safechat-example ./example/
package main

import (
	"bytes"
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	safechat "safechat-go-sdk"
)

func main() {
	var (
		dataPath = flag.String("data", "./keystore", "Path to key storage directory (contains ahflag_256.store / ahkey_256.store)")
		userID   = flag.String("user", "", "Current login user ID")
		code     = flag.String("code", "", "DingTalk authCode (only needed when keys must be fetched from server)")
		corpID   = flag.String("corp", "", "Enterprise/corp ID (required)")
		staffID  = flag.String("staff", "", "Staff ID for encryption target (defaults to -user)")
		action   = flag.String("action", "", "Single action: encrypt-msg|decrypt-msg|encrypt-file|decrypt-file|encrypt-buf|decrypt-buf")
		input    = flag.String("input", "", "Input text / file path")
		output   = flag.String("output", "", "Output file path (for file operations)")
		server   = flag.String("server", "", "Key server URL override (optional)")
		testAll  = flag.Bool("test-all", false, "Run a full round-trip test of all 3 encryption APIs using cached keys")
		verbose  = flag.Bool("v", false, "Verbose logging")
	)
	flag.Parse()

	if *corpID == "" {
		fmt.Fprintln(os.Stderr, "Error: -corp flag is required")
		flag.Usage()
		os.Exit(1)
	}

	// Validate key cache files up-front so the user gets a clear message
	// instead of a cryptic C-library error later.
	checkKeyCache(*dataPath, *code)

	cfg := safechat.Config{
		DataPath:    *dataPath,
		UserID:      *userID,
		Code:        *code,
		MaxRetry:    5,
		HTTPTimeout: 15 * time.Second,
	}
	if !*verbose {
		cfg.Logger = &quietLogger{}
	} else {
		cfg.Logger = &stdLogger{}
	}
	if *server != "" {
		cfg.KeyServer = *server
	}

	client, err := safechat.New(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize SafeChat client: %v", err)
	}
	defer client.Close()

	targetStaff := *staffID
	if targetStaff == "" {
		targetStaff = *userID
	}

	if *testAll {
		runFullTest(client, *corpID, targetStaff)
		return
	}

	if *action == "" {
		fmt.Fprintln(os.Stderr, "Error: either -action <name> or -test-all must be provided")
		flag.Usage()
		os.Exit(1)
	}

	switch *action {
	case "encrypt-msg":
		encryptMessage(client, *corpID, targetStaff, *input)
	case "decrypt-msg":
		decryptMessage(client, *corpID, targetStaff, *input)
	case "encrypt-file":
		encryptFile(client, *corpID, targetStaff, *input, *output)
	case "decrypt-file":
		decryptFile(client, *corpID, targetStaff, *input, *output)
	case "encrypt-buf":
		encryptBuffer(client, *corpID, targetStaff, *input)
	case "decrypt-buf":
		decryptBuffer(client, *corpID, targetStaff, *input)
	default:
		fmt.Fprintf(os.Stderr, "Unknown action: %s\n", *action)
		os.Exit(1)
	}
}

// ---------- key-cache pre-check ----------

// checkKeyCache prints a friendly hint about which keys are available and
// whether a network round-trip to the key server should be expected.
func checkKeyCache(dataPath, code string) {
	flagFile := filepath.Join(dataPath, "ahflag_256.store")
	keyFile := filepath.Join(dataPath, "ahkey_256.store")

	_, err1 := os.Stat(flagFile)
	_, err2 := os.Stat(keyFile)

	hasFlag := err1 == nil
	hasKey := err2 == nil

	switch {
	case hasFlag && hasKey:
		fmt.Printf("[key-cache] found %s + %s (existence check only; C layer still validates hash/version)\n",
			filepath.Base(flagFile), filepath.Base(keyFile))
		fmt.Printf("[key-cache] NOTE: store files are NOT portable across CPU architectures — " +
			"calc_hash() depends on char signedness (signed on x86_64, unsigned on AArch64). " +
			"A foreign store fails the hash check, gets deleted and re-generated, which triggers a server call.\n")
	case hasFlag || hasKey:
		fmt.Fprintf(os.Stderr,
			"[key-cache] WARNING: only one of the pair exists (%s / %s); key negotiation will likely fail\n",
			filepath.Base(flagFile), filepath.Base(keyFile))
	default:
		if code == "" {
			fmt.Fprintf(os.Stderr,
				"[key-cache] no cached keys in %s and -code is empty; "+
					"either copy ahflag_256.store + ahkey_256.store into that dir, "+
					"or provide a valid DingTalk access_token via -code\n",
				dataPath)
		} else {
			fmt.Printf("[key-cache] no cached keys in %s; will try to fetch from server using -code\n", dataPath)
		}
	}
}

// ---------- full round-trip test of all 3 APIs ----------

// runFullTest exercises the 3 Go API pairs defined in safechat.go.
//
// The APIs under test are the PUBLIC Go methods on *safechat.Client:
//
//	API #1 — Msg API    : EncryptMsg    / DecryptMsg    (safechat.go L115, L154)
//	API #2 — File API   : EncryptFile   / DecryptFile   (safechat.go L188, L213)
//	API #3 — Buffer API : EncryptBuffer / DecryptBuffer (safechat.go L240, L268)
//
// These are pure Go entry points: parameter validation, mutex locking,
// MaxRetry loop, and Go error translation are all done in safechat.go.
// The underlying CGO bridge (cEncryptData / cDecryptData / ...) is a
// PRIVATE implementation detail and is NOT what this test targets.
func runFullTest(client *safechat.Client, corpID, staffID string) {
	fmt.Println("==========================================================")
	fmt.Printf(" SafeChat Go SDK — Go API round-trip test\n")
	fmt.Printf(" corpID=%s  staffID=%s\n", corpID, staffID)
	fmt.Println(" Target: the 3 public Go API pairs in safechat.go")
	fmt.Println("   #1 Msg API    : EncryptMsg    / DecryptMsg")
	fmt.Println("   #2 File API   : EncryptFile   / DecryptFile")
	fmt.Println("   #3 Buffer API : EncryptBuffer / DecryptBuffer")
	fmt.Println("==========================================================")

	passed, failed := 0, 0

	// Go API #1 — Msg
	fmt.Println("\n[1/3] Go API #1 : EncryptMsg / DecryptMsg  (safechat.go L115, L154)")
	if runCase("Msg API", func() error { return testMsgAPI(client, corpID, staffID) }) {
		passed++
	} else {
		failed++
	}

	// Go API #2 — File
	fmt.Println("\n[2/3] Go API #2 : EncryptFile / DecryptFile  (safechat.go L188, L213)")
	if runCase("File API", func() error { return testFileAPI(client, corpID, staffID) }) {
		passed++
	} else {
		failed++
	}

	// Go API #3 — Buffer
	fmt.Println("\n[3/3] Go API #3 : EncryptBuffer / DecryptBuffer  (safechat.go L240, L268)")
	if runCase("Buffer API", func() error { return testBufferAPI(client, corpID, staffID) }) {
		passed++
	} else {
		failed++
	}

	fmt.Println("\n==========================================================")
	fmt.Printf(" RESULT: %d passed, %d failed\n", passed, failed)
	fmt.Println("==========================================================")
	if failed > 0 {
		os.Exit(1)
	}
}

func runCase(name string, fn func() error) bool {
	if err := fn(); err != nil {
		fmt.Printf("  ✗ %s FAILED: %v\n", name, err)
		return false
	}
	fmt.Printf("  ✓ %s OK\n", name)
	return true
}

// testMsgAPI exercises Go API #1:
//
//	client.EncryptMsg(corpID, staffID, plain)  → ciphertext
//	client.DecryptMsg(corpID, staffID, ciphertext) → plaintext
//
// Both methods are defined in safechat.go (L115, L154). They perform
// parameter validation, mutex locking, and a MaxRetry loop before
// returning a Go []byte + error.
func testMsgAPI(c *safechat.Client, corpID, staffID string) error {
	plain := make([]byte, 128)
	if _, err := rand.Read(plain); err != nil {
		return err
	}

	// Go API call — safechat.go L115
	ct, err := c.EncryptMsg(corpID, staffID, plain)
	if err != nil {
		return fmt.Errorf("EncryptMsg (Go API): %w", err)
	}
	fmt.Printf("    [Go API] EncryptMsg    : plain=%d bytes -> ct=%d bytes\n", len(plain), len(ct))
	fmt.Printf("    [ciphertext str] %s\n", strings.ReplaceAll(string(ct), "\n", ""))

	// Go API call — safechat.go L154
	got, err := c.DecryptMsg(corpID, staffID, ct)
	if err != nil {
		return fmt.Errorf("DecryptMsg (Go API): %w", err)
	}
	if !bytes.Equal(got, plain) {
		return fmt.Errorf("round-trip mismatch: got %d bytes, want %d", len(got), len(plain))
	}
	fmt.Printf("    [Go API] DecryptMsg    : ct=%d bytes -> plain=%d bytes  OK\n", len(ct), len(got))
	return nil
}

// testFileAPI exercises Go API #2:
//
//	client.EncryptFile(corpID, staffID, src, enc)  error
//	client.DecryptFile(corpID, staffID, enc, dec)  error
//
// Both methods are defined in safechat.go (L188, L213). They operate on
// file paths and return only an error — no data crosses the Go/C boundary
// in the caller-visible API.
func testFileAPI(c *safechat.Client, corpID, staffID string) error {
	dir, err := os.MkdirTemp("", "safechat-file-test-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	src := filepath.Join(dir, "plain.bin")
	enc := filepath.Join(dir, "plain.bin.enc")
	dec := filepath.Join(dir, "plain.bin.dec")

	buf := make([]byte, 1024)
	if _, err := rand.Read(buf); err != nil {
		return err
	}
	if err := os.WriteFile(src, buf, 0644); err != nil {
		return err
	}

	// Go API call — safechat.go L188
	if err := c.EncryptFile(corpID, staffID, src, enc); err != nil {
		return fmt.Errorf("EncryptFile (Go API): %w", err)
	}
	encSize, _ := fileSize(enc)
	fmt.Printf("    [Go API] EncryptFile   : src=%d bytes -> enc=%d bytes\n", len(buf), encSize)

	// Go API call — safechat.go L213
	if err := c.DecryptFile(corpID, staffID, enc, dec); err != nil {
		return fmt.Errorf("DecryptFile (Go API): %w", err)
	}
	decBuf, err := os.ReadFile(dec)
	if err != nil {
		return err
	}
	if !bytes.Equal(decBuf, buf) {
		return fmt.Errorf("round-trip mismatch: dec=%d bytes, want=%d", len(decBuf), len(buf))
	}
	fmt.Printf("    [Go API] DecryptFile   : enc=%d bytes -> dec=%d bytes  OK\n", encSize, len(decBuf))
	return nil
}

// testBufferAPI exercises Go API #3:
//
//	client.EncryptBuffer(corpID, staffID, data)  → []byte, error
//	client.DecryptBuffer(corpID, staffID, data)  → []byte, error
//
// Both methods are defined in safechat.go (L240, L268). They operate on
// in-memory []byte buffers, suitable for binary protocols or DB blobs.
func testBufferAPI(c *safechat.Client, corpID, staffID string) error {
	plain := make([]byte, 512)
	if _, err := rand.Read(plain); err != nil {
		return err
	}

	// Go API call — safechat.go L240
	ct, err := c.EncryptBuffer(corpID, staffID, plain)
	if err != nil {
		return fmt.Errorf("EncryptBuffer (Go API): %w", err)
	}
	fmt.Printf("    [Go API] EncryptBuffer : plain=%d bytes -> ct=%d bytes\n", len(plain), len(ct))

	// Go API call — safechat.go L268
	got, err := c.DecryptBuffer(corpID, staffID, ct)
	if err != nil {
		return fmt.Errorf("DecryptBuffer (Go API): %w", err)
	}
	if !bytes.Equal(got, plain) {
		return fmt.Errorf("round-trip mismatch: got %d bytes, want %d", len(got), len(plain))
	}
	fmt.Printf("    [Go API] DecryptBuffer : ct=%d bytes -> plain=%d bytes  OK\n", len(ct), len(got))
	return nil
}

func fileSize(p string) (int64, error) {
	fi, err := os.Stat(p)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

// ---------- single-action helpers ----------

func encryptMessage(client *safechat.Client, corpID, staffID, plaintext string) {
	if plaintext == "" {
		plaintext = "Hello, this is a test message from SafeChat Go SDK!"
	}
	fmt.Printf("Encrypting message: %q\n", plaintext)
	ciphertext, err := client.EncryptMsg(corpID, staffID, []byte(plaintext))
	if err != nil {
		log.Fatalf("EncryptMsg failed: %v", err)
	}
	fmt.Printf("Encrypted (%d bytes): %s\n", len(ciphertext), string(ciphertext))
}

func decryptMessage(client *safechat.Client, corpID, staffID, ciphertext string) {
	if ciphertext == "" {
		log.Fatal("decrypt-msg requires -input with the ciphertext")
	}
	fmt.Printf("Decrypting message (%d bytes)...\n", len(ciphertext))
	plaintext, err := client.DecryptMsg(corpID, staffID, []byte(ciphertext))
	if err != nil {
		log.Fatalf("DecryptMsg failed: %v", err)
	}
	fmt.Printf("Decrypted: %s\n", string(plaintext))
}

func encryptFile(client *safechat.Client, corpID, staffID, srcPath, dstPath string) {
	if srcPath == "" {
		log.Fatal("encrypt-file requires -input with source file path")
	}
	if dstPath == "" {
		dstPath = srcPath + ".enc"
	}
	fmt.Printf("Encrypting file: %s -> %s\n", srcPath, dstPath)
	if err := client.EncryptFile(corpID, staffID, srcPath, dstPath); err != nil {
		log.Fatalf("EncryptFile failed: %v", err)
	}
	fmt.Println("File encrypted successfully")
}

func decryptFile(client *safechat.Client, corpID, staffID, srcPath, dstPath string) {
	if srcPath == "" {
		log.Fatal("decrypt-file requires -input with source file path")
	}
	if dstPath == "" {
		dstPath = srcPath + ".dec"
	}
	fmt.Printf("Decrypting file: %s -> %s\n", srcPath, dstPath)
	if err := client.DecryptFile(corpID, staffID, srcPath, dstPath); err != nil {
		log.Fatalf("DecryptFile failed: %v", err)
	}
	fmt.Println("File decrypted successfully")
}

func encryptBuffer(client *safechat.Client, corpID, staffID, inputPath string) {
	if inputPath == "" {
		log.Fatal("encrypt-buf requires -input with file path containing data to encrypt")
	}
	data, err := os.ReadFile(inputPath)
	if err != nil {
		log.Fatalf("Failed to read input file: %v", err)
	}
	fmt.Printf("Encrypting buffer (%d bytes)...\n", len(data))
	encrypted, err := client.EncryptBuffer(corpID, staffID, data)
	if err != nil {
		log.Fatalf("EncryptBuffer failed: %v", err)
	}
	outPath := inputPath + ".enc"
	if err := os.WriteFile(outPath, encrypted, 0644); err != nil {
		log.Fatalf("Failed to write output: %v", err)
	}
	fmt.Printf("Buffer encrypted successfully (%d bytes) -> %s\n", len(encrypted), outPath)
}

func decryptBuffer(client *safechat.Client, corpID, staffID, inputPath string) {
	if inputPath == "" {
		log.Fatal("decrypt-buf requires -input with file path containing data to decrypt")
	}
	data, err := os.ReadFile(inputPath)
	if err != nil {
		log.Fatalf("Failed to read input file: %v", err)
	}
	fmt.Printf("Decrypting buffer (%d bytes)...\n", len(data))
	decrypted, err := client.DecryptBuffer(corpID, staffID, data)
	if err != nil {
		log.Fatalf("DecryptBuffer failed: %v", err)
	}
	outPath := inputPath + ".dec"
	if err := os.WriteFile(outPath, decrypted, 0644); err != nil {
		log.Fatalf("Failed to write output: %v", err)
	}
	fmt.Printf("Buffer decrypted successfully (%d bytes) -> %s\n", len(decrypted), outPath)
}

// ---------- loggers ----------

type stdLogger struct{}

func (l *stdLogger) Debug(format string, args ...interface{}) {
	log.Printf("[DEBUG] "+format, args...)
}
func (l *stdLogger) Info(format string, args ...interface{}) {
	log.Printf("[INFO] "+format, args...)
}
func (l *stdLogger) Error(format string, args ...interface{}) {
	log.Printf("[ERROR] "+format, args...)
}

// quietLogger swallows SDK logs; only errors are surfaced via returned error values.
type quietLogger struct{}

func (l *quietLogger) Debug(format string, args ...interface{}) {}
func (l *quietLogger) Info(format string, args ...interface{})  {}
func (l *quietLogger) Error(format string, args ...interface{}) {}
