//go:build darwin && amd64

package runtimepayload

import _ "embed"

//go:embed assets/darwin-amd64.payload
var embeddedPayload []byte
