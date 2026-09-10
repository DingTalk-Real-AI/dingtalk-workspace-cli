#ifndef GOPROXY_BRIDGE_H
#define GOPROXY_BRIDGE_H

/*
 * goproxy_bridge.h - CGO callback bridge declarations
 *
 * These bridge functions are called by the C library (safechat.c)
 * and forward to Go exported functions via CGO.
 * This indirection is required because CGO cannot directly pass
 * Go function pointers to C code.
 */

/* Proxy callback bridge - forwards key requests to Go HTTP client */
int goProxyBridge(char *corpid, char *uid, char *domain,
                  char *url, char *param, char *seq_id);

/* Block crypto callback bridge - notifies Go of key restriction */
int goBlockBridge(char *corpid);

/* Cancel block crypto callback bridge - notifies Go of restriction lift */
int goCancelBlockBridge(char *corpid);

/* Wrapper functions for CGO */
int init(char *path, char *my_id, void *reserved);
int clearCache(char *corpid);
void freeCryptoBuf(void *buf);

#endif /* GOPROXY_BRIDGE_H */
