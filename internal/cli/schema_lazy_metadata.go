// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

// SchemaMetadataLoadCounts exposes read-only diagnostics for startup tests and
// profiling. Counts are incremented only when a lazy delivery snapshot is
// assembled / projected for the first time.
type SchemaMetadataLoadCounts struct {
	Catalog          uint64
	MetaIndex        uint64
	AgentMetadata    uint64
	ParameterBinding uint64
}

// RuntimeSchemaMetadataLoadCounts reads the concurrency-safe lazy loader
// counters without triggering any loader.
func RuntimeSchemaMetadataLoadCounts() SchemaMetadataLoadCounts {
	return SchemaMetadataLoadCounts{
		Catalog:          runtimeDeliverySchemaCatalogLazyCount.Load(),
		MetaIndex:        runtimeDeliverySchemaMetaIndexLazyCount.Load(),
		AgentMetadata:    runtimeAgentMetadataLazyLoadCount.Load(),
		ParameterBinding: runtimeSchemaParameterBindingsLazyLoadCount.Load(),
	}
}
