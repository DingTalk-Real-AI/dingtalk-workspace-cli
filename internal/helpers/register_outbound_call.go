// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

// Register the long-tail product through the same helper path as sport.
func init() {
	RegisterPublicNamed("outbound-call", func() Handler {
		return wukongHandler{name: "outbound-call", buildFn: newOutboundCallCommand}
	})
}
