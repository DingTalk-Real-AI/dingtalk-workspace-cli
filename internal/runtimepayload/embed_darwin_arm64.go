//go:build darwin && arm64

package runtimepayload

import _ "embed"

//go:embed assets/darwin-arm64.payload
var embeddedPayload []byte
