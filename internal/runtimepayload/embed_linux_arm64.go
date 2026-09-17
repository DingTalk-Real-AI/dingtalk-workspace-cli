//go:build linux && arm64

package runtimepayload

import _ "embed"

//go:embed assets/linux-arm64.payload
var embeddedPayload []byte
