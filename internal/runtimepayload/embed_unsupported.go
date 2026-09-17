//go:build (!darwin && !linux && !windows) || (!amd64 && !arm64)

package runtimepayload

var embeddedPayload []byte
