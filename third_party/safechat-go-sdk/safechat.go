// Package safechat provides encryption/decryption capabilities for the

// Basic usage:
//
//	client, err := safechat.New(safechat.Config{
//	    DataPath: "/path/to/keystore",
//	    UserID:   "user123",
//	    Code:     "authCode ", // authCode
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Close()
//
//	ciphertext, err := client.EncryptMsg("corp_id", "staff_id", []byte("hello"))
//
// The Code field must be a valid DingTalk authCode. It is required
// when fetching keys from the server (first-time use or key version rotation).
package safechat

/*
#include "csrc/safechat.h"
#include "goproxy_bridge.h"
#include <stdlib.h>
#include <string.h>
*/
import "C"
import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/google/uuid"
)

// Client is the main SafeChat encryption client.
// It wraps the C library and provides a thread-safe Go API.
//
// IMPORTANT: Only one Client instance can exist per process because
// the underlying C library uses global state. Creating a second Client
// will return ErrAlreadyInitialized.
//
// All public methods are safe for concurrent use from multiple goroutines.
// Internal synchronization uses a two-lock design:
//   - mu:   serializes all C library calls (prevents C global state corruption)
//   - kcMu: protects keyClient state during HTTP requests (used in goProxy callback)
type Client struct {
	cfg          Config
	mu           sync.Mutex // Serializes all C library calls
	kcMu         sync.Mutex // Protects keyClient during HTTP key requests
	kc           *keyClient // HTTP client for key server communication
	inited       bool       // Whether C library init() has been called
	blockedCorps sync.Map   // map[string]bool - enterprises with restricted keys
}

// New creates a new SafeChat client and initializes the underlying C library.
//
// The Config.DataPath directory will be used to store encryption keys and
// related metadata. It must exist and be writable.
//
// Returns ErrAlreadyInitialized if called more than once per process.
func New(cfg Config) (*Client, error) {
	cfg = defaultConfig(cfg)
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	// Check if already initialized (C library is singleton)
	if getGlobalClient() != nil {
		return nil, ErrAlreadyInitialized
	}

	c := &Client{
		cfg: cfg,
		kc:  newKeyClient(cfg),
	}

	// Register globally for CGO callbacks before calling init
	registerGlobalClient(c)

	// Initialize C library
	if err := c.cInit(); err != nil {
		globalClient.Store((*Client)(nil))
		return nil, fmt.Errorf("safechat init failed: %w", err)
	}

	c.inited = true
	return c, nil
}

// Close releases resources held by the client.
// After Close is called, no other methods should be called.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.inited = false
	// Clear the global singleton so a new Client can be created after Close.
	// The C library has no explicit cleanup function; keys are persisted to
	// disk and process memory is reclaimed on exit.
	globalClient.Store((*Client)(nil))
}

// EncryptMsg encrypts a plaintext message for the given enterprise.
//
// Returns the ciphertext in the standard SafeChat format:
// base64(encrypted_data)||key_version||method_num||plain_length
//
// If the encryption key is not yet available, the SDK will automatically
// request it from the key server (via goProxy callback) and retry.
func (c *Client) EncryptMsg(corpID, staffID string, plaintext []byte) ([]byte, error) {
	if !c.inited {
		return nil, ErrNotInitialized
	}
	if len(plaintext) == 0 {
		return nil, fmt.Errorf("safechat: plaintext cannot be empty")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for i := 0; i <= c.cfg.MaxRetry; i++ {
		result, err := c.cEncryptData(corpID, staffID, plaintext)
		if err == nil {
			return result, nil
		}

		// Check if it's a "key requested" status - retry
		if cerr, ok := err.(*CError); ok && cerr.Code == cSendRequestParamOK {
			// goProxy was called and setResponse was invoked synchronously.
			// Next iteration should find the key in local cache.
			continue
		}

		// Check for key restriction
		if cerr, ok := err.(*CError); ok && cerr.Code == cEncryptDataKeyNegtive {
			return nil, ErrKeyRestricted
		}

		return nil, err
	}

	return nil, ErrMaxRetryExceeded
}

// DecryptMsg decrypts a ciphertext message.
//
// The ciphertext must be in the standard SafeChat format:
// base64(encrypted_data)||key_version||method_num||plain_length
func (c *Client) DecryptMsg(corpID, staffID string, ciphertext []byte) ([]byte, error) {
	if !c.inited {
		return nil, ErrNotInitialized
	}
	if len(ciphertext) == 0 {
		return nil, fmt.Errorf("safechat: ciphertext cannot be empty")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for i := 0; i <= c.cfg.MaxRetry; i++ {
		result, err := c.cDecryptData(corpID, staffID, ciphertext)
		if err == nil {
			return result, nil
		}

		if cerr, ok := err.(*CError); ok && cerr.Code == cSendRequestParamOK {
			continue
		}
		if cerr, ok := err.(*CError); ok && cerr.Code == cDecryptDataKeyNegtive {
			return nil, ErrKeyRestricted
		}

		return nil, err
	}

	return nil, ErrMaxRetryExceeded
}

// EncryptFile encrypts a file from srcPath to dstPath.
//
// The encrypted file uses a 12-byte header (msg_HandInfo_t) followed by
// SM4-ECB encrypted data in 8KiB blocks.
func (c *Client) EncryptFile(corpID, staffID, srcPath, dstPath string) error {
	if !c.inited {
		return ErrNotInitialized
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for i := 0; i <= c.cfg.MaxRetry; i++ {
		err := c.cEncryptFile(corpID, staffID, srcPath, dstPath)
		if err == nil {
			return nil
		}

		if cerr, ok := err.(*CError); ok && cerr.Code == cSendRequestParamOK {
			continue
		}

		return err
	}

	return ErrMaxRetryExceeded
}

// DecryptFile decrypts a file from srcPath to dstPath.
func (c *Client) DecryptFile(corpID, staffID, srcPath, dstPath string) error {
	if !c.inited {
		return ErrNotInitialized
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for i := 0; i <= c.cfg.MaxRetry; i++ {
		err := c.cDecryptFile(corpID, staffID, srcPath, dstPath)
		if err == nil {
			return nil
		}

		if cerr, ok := err.(*CError); ok && cerr.Code == cSendRequestParamOK {
			continue
		}

		return err
	}

	return ErrMaxRetryExceeded
}

// EncryptBuffer encrypts binary data in memory.
//
// The result includes a 12-byte header followed by SM4-ECB encrypted content.
func (c *Client) EncryptBuffer(corpID, staffID string, data []byte) ([]byte, error) {
	if !c.inited {
		return nil, ErrNotInitialized
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("safechat: data cannot be empty")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for i := 0; i <= c.cfg.MaxRetry; i++ {
		result, err := c.cEncryptBuffer(corpID, staffID, data)
		if err == nil {
			return result, nil
		}

		if cerr, ok := err.(*CError); ok && cerr.Code == cSendRequestParamOK {
			continue
		}

		return nil, err
	}

	return nil, ErrMaxRetryExceeded
}

// DecryptBuffer decrypts binary data in memory.
func (c *Client) DecryptBuffer(corpID, staffID string, data []byte) ([]byte, error) {
	if !c.inited {
		return nil, ErrNotInitialized
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("safechat: data cannot be empty")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for i := 0; i <= c.cfg.MaxRetry; i++ {
		result, err := c.cDecryptBuffer(corpID, staffID, data)
		if err == nil {
			return result, nil
		}

		if cerr, ok := err.(*CError); ok && cerr.Code == cSendRequestParamOK {
			continue
		}

		return nil, err
	}

	return nil, ErrMaxRetryExceeded
}

// SetResponse manually injects a key server response into the C library.
// This is an advanced API for cases where the caller handles HTTP
// communication externally.
func (c *Client) SetResponse(corpID, jsonStr string) error {
	if !c.inited {
		return ErrNotInitialized
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	return c.cSetResponse(corpID, jsonStr)
}

// SetPushData processes a server push notification (key update, etc).
func (c *Client) SetPushData(corpID, staffID, pushData string) error {
	if !c.inited {
		return ErrNotInitialized
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	return c.cSetPushData(corpID, staffID, pushData)
}

// ClearCache clears the local key cache for a specific enterprise.
func (c *Client) ClearCache(corpID string) error {
	if !c.inited {
		return ErrNotInitialized
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	cCorpID := C.CString(corpID)
	defer C.free(unsafe.Pointer(cCorpID))

	ret := C.clearCache(cCorpID)
	return mapCError(int(ret))
}

// IsBlocked returns true if the given enterprise's key is restricted.
func (c *Client) IsBlocked(corpID string) bool {
	_, ok := c.blockedCorps.Load(corpID)
	return ok
}

// UpdateCode updates the DingTalk authentication code at runtime.
func (c *Client) UpdateCode(code string) {
	c.kcMu.Lock()
	defer c.kcMu.Unlock()
	c.cfg.Code = code
	c.kc.updateCode(code)
}

// ============================================================
// CGO wrapper methods (called with c.mu held)
// ============================================================

func (c *Client) cInit() error {
	cPath := C.CString(c.cfg.DataPath)
	cUserID := C.CString(c.cfg.UserID)
	defer C.free(unsafe.Pointer(cPath))
	defer C.free(unsafe.Pointer(cUserID))

	ret := C.init(cPath, cUserID, nil)
	return mapCError(int(ret))
}

func (c *Client) cEncryptData(corpID, staffID string, plaintext []byte) ([]byte, error) {
	cCorpID := C.CString(corpID)
	cStaffID := C.CString(staffID)
	seqID := uuid.New().String()
	cSeqID := C.CString(seqID)
	defer C.free(unsafe.Pointer(cCorpID))
	defer C.free(unsafe.Pointer(cStaffID))
	defer C.free(unsafe.Pointer(cSeqID))

	var outBuf *C.uchar
	var outLen C.uint

	ret := C.encryptData(
		cCorpID,
		cStaffID,
		(*C.uchar)(unsafe.Pointer(&plaintext[0])),
		C.uint(len(plaintext)),
		nil, // id - NULL for group messages
		&outBuf,
		&outLen,
		cSeqID,
		(C.call_proxy_func)(C.goProxyBridge),
	)

	retCode := int(ret)
	if retCode != cFunctionOK {
		return nil, mapCError(retCode)
	}

	// Copy result to Go slice and free C buffer
	result := C.GoBytes(unsafe.Pointer(outBuf), C.int(outLen))
	C.freeCryptoBuf(unsafe.Pointer(outBuf))
	return result, nil
}

func (c *Client) cDecryptData(corpID, staffID string, ciphertext []byte) ([]byte, error) {
	cCorpID := C.CString(corpID)
	cStaffID := C.CString(staffID)
	seqID := uuid.New().String()
	cSeqID := C.CString(seqID)
	defer C.free(unsafe.Pointer(cCorpID))
	defer C.free(unsafe.Pointer(cStaffID))
	defer C.free(unsafe.Pointer(cSeqID))

	var outBuf *C.uchar
	var outLen C.uint

	// The C layer treats the ciphertext as a NUL-terminated C string
	// (sscanf/strtok). Go byte slices are not NUL-terminated and strtok would
	// mutate the caller's buffer, so pass a NUL-terminated private copy.
	cbuf := make([]byte, len(ciphertext)+1)
	copy(cbuf, ciphertext)

	ret := C.decryptData(
		cCorpID,
		cStaffID,
		(*C.uchar)(unsafe.Pointer(&cbuf[0])),
		C.uint(len(ciphertext)),
		nil, // id
		&outBuf,
		&outLen,
		cSeqID,
		(C.call_proxy_func)(C.goProxyBridge),
	)

	retCode := int(ret)
	if retCode != cFunctionOK {
		return nil, mapCError(retCode)
	}

	result := C.GoBytes(unsafe.Pointer(outBuf), C.int(outLen))
	C.freeCryptoBuf(unsafe.Pointer(outBuf))
	return result, nil
}

func (c *Client) cEncryptFile(corpID, staffID, srcPath, dstPath string) error {
	cCorpID := C.CString(corpID)
	cStaffID := C.CString(staffID)
	cID := C.CString("")
	cSrcPath := C.CString(srcPath)
	cDstPath := C.CString(dstPath)
	seqID := uuid.New().String()
	cSeqID := C.CString(seqID)
	defer C.free(unsafe.Pointer(cCorpID))
	defer C.free(unsafe.Pointer(cStaffID))
	defer C.free(unsafe.Pointer(cID))
	defer C.free(unsafe.Pointer(cSrcPath))
	defer C.free(unsafe.Pointer(cDstPath))
	defer C.free(unsafe.Pointer(cSeqID))

	ret := C.encryptFile(
		cCorpID,
		cStaffID,
		cID,
		cSrcPath,
		cDstPath,
		cSeqID,
		(C.call_proxy_func)(C.goProxyBridge),
	)

	retCode := int(ret)
	if retCode != cFunctionOK {
		return mapCError(retCode)
	}
	return nil
}

func (c *Client) cDecryptFile(corpID, staffID, srcPath, dstPath string) error {
	cCorpID := C.CString(corpID)
	cStaffID := C.CString(staffID)
	cID := C.CString("")
	cSrcPath := C.CString(srcPath)
	cDstPath := C.CString(dstPath)
	seqID := uuid.New().String()
	cSeqID := C.CString(seqID)
	defer C.free(unsafe.Pointer(cCorpID))
	defer C.free(unsafe.Pointer(cStaffID))
	defer C.free(unsafe.Pointer(cID))
	defer C.free(unsafe.Pointer(cSrcPath))
	defer C.free(unsafe.Pointer(cDstPath))
	defer C.free(unsafe.Pointer(cSeqID))

	ret := C.decryptFile(
		cCorpID,
		cStaffID,
		cID,
		cSrcPath,
		cDstPath,
		cSeqID,
		(C.call_proxy_func)(C.goProxyBridge),
	)

	retCode := int(ret)
	if retCode != cFunctionOK {
		return mapCError(retCode)
	}
	return nil
}

func (c *Client) cEncryptBuffer(corpID, staffID string, data []byte) ([]byte, error) {
	cCorpID := C.CString(corpID)
	cStaffID := C.CString(staffID)
	cID := C.CString("")
	seqID := uuid.New().String()
	cSeqID := C.CString(seqID)
	defer C.free(unsafe.Pointer(cCorpID))
	defer C.free(unsafe.Pointer(cStaffID))
	defer C.free(unsafe.Pointer(cID))
	defer C.free(unsafe.Pointer(cSeqID))

	var outBuf *C.uchar
	var outLen C.uint

	ret := C.encryptBuffer(
		cCorpID,
		cStaffID,
		(*C.uchar)(unsafe.Pointer(&data[0])),
		C.uint(len(data)),
		cID,
		&outBuf,
		&outLen,
		cSeqID,
		(C.call_proxy_func)(C.goProxyBridge),
	)

	retCode := int(ret)
	if retCode != cFunctionOK {
		return nil, mapCError(retCode)
	}

	result := C.GoBytes(unsafe.Pointer(outBuf), C.int(outLen))
	C.freeCryptoBuf(unsafe.Pointer(outBuf))
	return result, nil
}

func (c *Client) cDecryptBuffer(corpID, staffID string, data []byte) ([]byte, error) {
	cCorpID := C.CString(corpID)
	cStaffID := C.CString(staffID)
	cID := C.CString("")
	seqID := uuid.New().String()
	cSeqID := C.CString(seqID)
	defer C.free(unsafe.Pointer(cCorpID))
	defer C.free(unsafe.Pointer(cStaffID))
	defer C.free(unsafe.Pointer(cID))
	defer C.free(unsafe.Pointer(cSeqID))

	var outBuf *C.uchar
	var outLen C.uint

	ret := C.decryptBuffer(
		cCorpID,
		cStaffID,
		(*C.uchar)(unsafe.Pointer(&data[0])),
		C.uint(len(data)),
		cID,
		&outBuf,
		&outLen,
		cSeqID,
		(C.call_proxy_func)(C.goProxyBridge),
	)

	retCode := int(ret)
	if retCode != cFunctionOK {
		return nil, mapCError(retCode)
	}

	result := C.GoBytes(unsafe.Pointer(outBuf), C.int(outLen))
	C.freeCryptoBuf(unsafe.Pointer(outBuf))
	return result, nil
}

func (c *Client) cSetResponse(corpID, jsonStr string) error {
	cCorpID := C.CString(corpID)
	cJSON := C.CString(jsonStr)
	defer C.free(unsafe.Pointer(cCorpID))
	defer C.free(unsafe.Pointer(cJSON))

	ret := C.setResponse(
		cCorpID,
		cJSON,
		(C.block_crypto_func)(C.goBlockBridge),
	)

	retCode := int(ret)
	if retCode != cFunctionOK {
		return mapCError(retCode)
	}
	return nil
}

func (c *Client) cSetPushData(corpID, staffID, pushData string) error {
	cCorpID := C.CString(corpID)
	cStaffID := C.CString(staffID)
	cPushData := C.CString(pushData)
	seqID := uuid.New().String()
	cSeqID := C.CString(seqID)
	defer C.free(unsafe.Pointer(cCorpID))
	defer C.free(unsafe.Pointer(cStaffID))
	defer C.free(unsafe.Pointer(cPushData))
	defer C.free(unsafe.Pointer(cSeqID))

	ret := C.setPushData(
		cCorpID,
		cStaffID,
		cPushData,
		cSeqID,
		(C.call_proxy_func)(C.goProxyBridge),
		(C.cancel_block_crypto_func)(C.goCancelBlockBridge),
	)

	retCode := int(ret)
	if retCode != cFunctionOK && retCode != cSendRequestParamOK {
		return mapCError(retCode)
	}
	return nil
}
