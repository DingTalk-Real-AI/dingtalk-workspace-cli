//go:build linux && amd64

package runtimepayload

import _ "embed"

//go:embed assets/linux-amd64.payload
var embeddedPayload []byte
