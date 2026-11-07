//go:build windows && amd64

package runtimepayload

import _ "embed"

//go:embed assets/windows-amd64.payload
var embeddedPayload []byte
