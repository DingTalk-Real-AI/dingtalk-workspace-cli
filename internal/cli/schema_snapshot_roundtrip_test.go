// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

// Enable the Catalog↔typed content equality gate for package tests. Production
// binaries keep validateSchemaSnapshotTypedRoundTrip=false so cold start does
// not pay ~845 JSON equality comparisons; generation already proves delivery
// invariants, and tests below cover loader drift through this flag.
func init() {
	validateSchemaSnapshotTypedRoundTrip = true
}
