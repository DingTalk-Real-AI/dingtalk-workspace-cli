//go:build windows && arm64

package runtimepayload

import _ "embed"

//go:embed assets/windows-arm64.payload
var embeddedPayload []byte
