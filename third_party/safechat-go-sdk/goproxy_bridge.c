#include "goproxy_bridge.h"
#include "csrc/safechat.h"
#include <stdlib.h>

/*
 * goproxy_bridge.c - CGO callback bridge implementation
 *
 * This file MUST reside in the Go package root directory so that
 * CGO compiles it together with the Go code. It bridges C library
 * callback invocations to Go exported functions.
 *
 * Go exported functions are declared as extern here.
 * The bridge functions simply forward calls from C library
 * to the Go runtime via CGO mechanism.
 */

/* Go exported function declarations (defined in callback.go) */
extern int goProxy(char *corpid, char *uid, char *domain,
                   char *url, char *param, char *seq_id);
extern int goBlock(char *corpid);
extern int goCancelBlock(char *corpid);

/*
 * init - Wrapper for safechatInit
 * Initializes the SafeChat library.
 */
int init(char *path, char *my_id, void *reserved) {
    (void)reserved; /* unused */
    return safechatInit(path, my_id);
}

/*
 * clearCache - Clears the key cache for a specific enterprise
 * Returns 0 on success, non-zero on error.
 * Note: This is a placeholder - actual implementation may vary.
 */
int clearCache(char *corpid) {
    (void)corpid; /* unused for now */
    /* TODO: Implement actual cache clearing if needed */
    return 0;
}

/*
 * freeCryptoBuf - Frees a buffer allocated by the C library
 * The C library allocates buffers with malloc, so we free with free.
 */
void freeCryptoBuf(void *buf) {
    if (buf != NULL) {
        free(buf);
    }
}

/*
 * goProxyBridge - Bridge for call_proxy_func typedef
 * Called by C library when a key request needs to be sent.
 * Forwards to Go's goProxy which performs HTTP request
 * and calls setResponse to feed back the key data.
 */
int goProxyBridge(char *corpid, char *uid, char *domain,
                  char *url, char *param, char *seq_id) {
    return goProxy(corpid, uid, domain, url, param, seq_id);
}

/*
 * goBlockBridge - Bridge for block_crypto_func typedef
 * Called by C library when enterprise key is restricted.
 * Notifies Go layer of the restriction.
 */
int goBlockBridge(char *corpid) {
    return goBlock(corpid);
}

/*
 * goCancelBlockBridge - Bridge for cancel_block_crypto_func typedef
 * Called by C library when enterprise key restriction is lifted.
 * Notifies Go layer to remove the restriction.
 */
int goCancelBlockBridge(char *corpid) {
    return goCancelBlock(corpid);
}
