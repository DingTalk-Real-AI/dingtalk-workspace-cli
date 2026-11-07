// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package localio

import (
	"context"
	"net"
)

// SetSecureDownloadDialTargetForTest reroutes every secure download dial to
// fixtureAddr. Subprocess e2e tests use it to serve platform download hosts
// from a loopback TLS fixture: TLS SNI and certificate verification still run
// against the real host name, and the production client keeps Proxy disabled.
// Production code must not call this.
func SetSecureDownloadDialTargetForTest(fixtureAddr string) {
	secureDialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, fixtureAddr)
	}
}
