package safechat

/*
#include "csrc/safechat.h"
#include "goproxy_bridge.h"
#include <stdlib.h>
*/
import "C"
import (
	"sync/atomic"
	"unsafe"
)

var globalClient atomic.Value // stores *Client

// registerGlobalClient stores the client reference for CGO callbacks.
func registerGlobalClient(c *Client) {
	globalClient.Store(c)
}

// getGlobalClient retrieves the active client from the atomic value.
// Returns nil if no client is registered.
func getGlobalClient() *Client {
	v := globalClient.Load()
	if v == nil {
		return nil
	}
	return v.(*Client)
}

// goProxy is the CGO callback invoked by the C library when a key request needs to be sent.
//
//export goProxy
func goProxy(corpid, uid, domain, url, param, seqID *C.char) C.int {
	client := getGlobalClient()
	if client == nil {
		return -1
	}

	// Copy C strings to Go values immediately to avoid dangling pointer issues.
	// The C layer may free these buffers after goProxy returns.
	goCorpID := C.GoString(corpid)
	goDomain := C.GoString(domain)
	goURL := C.GoString(url)
	goParam := C.GoString(param)

	if client.cfg.Logger != nil {
		client.cfg.Logger.Debug("goProxy called: corpID=%s, url=%s", goCorpID, goURL)
		client.cfg.Logger.Debug("goProxy input param (length=%d): %s", len(goParam), previewString(goParam, 1024))
	}

	code := client.cfg.Code
	if client.cfg.AuthCodeHook != nil {
		hookCode, hookErr := client.cfg.AuthCodeHook(goCorpID, goDomain)
		if hookErr != nil {
			if client.cfg.Logger != nil {
				client.cfg.Logger.Error("goProxy: authCode hook failed for corp %s: %v", goCorpID, hookErr)
			}
			return -1
		}
		if hookCode == "" {
			if client.cfg.Logger != nil {
				client.cfg.Logger.Error("goProxy: authCode hook returned an empty code for corp %s", goCorpID)
			}
			return -1
		}
		code = hookCode
	}

	// Use kcMu to protect the HTTP request - NOT c.mu!
	// c.mu is already held by the caller (EncryptMsg/DecryptMsg etc.)
	client.kcMu.Lock()
	resp, err := client.kc.doKeyRequest(goURL, goParam, code)
	client.kcMu.Unlock()

	if err != nil {
		if client.cfg.Logger != nil {
			client.cfg.Logger.Error("goProxy: key request failed for corp %s: %v", goCorpID, err)
		}
		return -1
	}

	if client.cfg.Logger != nil {
		client.cfg.Logger.Debug("goProxy: HTTP request succeeded, feeding response to C setResponse (corpID=%s, response length=%d): %s",
			goCorpID, len(resp), previewString(resp, 1024))
	}

	cCorpID := C.CString(goCorpID)
	cResp := C.CString(resp)
	defer C.free(unsafe.Pointer(cCorpID))
	defer C.free(unsafe.Pointer(cResp))

	ret := C.setResponse(
		cCorpID,
		cResp,
		(C.block_crypto_func)(C.goBlockBridge),
	)

	if client.cfg.Logger != nil {
		client.cfg.Logger.Debug("goProxy: C.setResponse returned %d for corpID=%s", ret, goCorpID)
	}

	if ret != 0 {
		if client.cfg.Logger != nil {
			client.cfg.Logger.Error("goProxy: C.setResponse FAILED for corpID=%s, ret=%d (key file may NOT have been generated)", goCorpID, ret)
		}
		return -1
	}
	return 0
}

// goBlock is called by C library when an enterprise key becomes restricted.
// This sets a flag that can be checked by the Go application layer.
//
//export goBlock
func goBlock(corpid *C.char) C.int {
	client := getGlobalClient()
	if client == nil {
		return -1
	}

	goCorpID := C.GoString(corpid)
	if client.cfg.Logger != nil {
		client.cfg.Logger.Info("goBlock: enterprise %s key restricted", goCorpID)
	}

	// Store blocked status - application can check via IsBlocked()
	client.blockedCorps.Store(goCorpID, true)
	return 0
}

// goCancelBlock is called when an enterprise key restriction is lifted.
//
//export goCancelBlock
func goCancelBlock(corpid *C.char) C.int {
	client := getGlobalClient()
	if client == nil {
		return -1
	}

	goCorpID := C.GoString(corpid)
	if client.cfg.Logger != nil {
		client.cfg.Logger.Info("goCancelBlock: enterprise %s key unblocked", goCorpID)
	}

	client.blockedCorps.Delete(goCorpID)
	return 0
}
