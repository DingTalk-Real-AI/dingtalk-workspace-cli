// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemaruntime

import (
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemacachepb"
	"google.golang.org/protobuf/proto"
)

// Even authenticated bytes must reject malformed JSON, including provenance
// fields that are not projected into a known runtime field. Diagnostics retain
// the exact location while valid values avoid constructing it.
func TestSchemaCacheInvalidJSONLocations(t *testing.T) {
	built, meta := buildFixtureCache(t, allFieldsRegistry())
	for _, test := range []struct {
		name     string
		edit     func(*schemacachepb.ToolSpec)
		location string
	}{
		{"default", func(tool *schemacachepb.ToolSpec) {
			tool.Parameters.Items[0].DefaultValue = &schemacachepb.BytesValue{Value: []byte("{")}
		}, " parameter "},
		{"interface_default", func(tool *schemacachepb.ToolSpec) {
			tool.Parameters.Items[0].InterfaceDefault = &schemacachepb.BytesValue{Value: []byte("{")}
		}, "interface_default is invalid JSON"},
		{"example", func(tool *schemacachepb.ToolSpec) {
			tool.Parameters.Items[0].Example = &schemacachepb.BytesValue{Value: []byte("{")}
		}, "example is invalid JSON"},
		{"result", func(tool *schemacachepb.ToolSpec) { tool.Result.DataSchema.Value = []byte("{") }, "result data_schema is invalid JSON"},
		{"winner", func(tool *schemacachepb.ToolSpec) { tool.FieldProvenance.Items[0].Value.Value.Value = []byte("{") }, ".value is invalid JSON"},
		{"unknown-provenance", func(tool *schemacachepb.ToolSpec) {
			tool.FieldProvenance.Items = append(tool.FieldProvenance.Items, &schemacachepb.ProvenanceEntry{
				Key: "zz_future", Value: &schemacachepb.FieldProvenance{Value: &schemacachepb.BytesValue{Value: []byte("{")}},
			})
		}, ".zz_future.value is invalid JSON"},
		{"candidate", func(tool *schemacachepb.ToolSpec) {
			tool.FieldProvenance.Items[0].Value.Candidates.Items[0].Value.Value = []byte("{")
		}, "candidate 0 value is invalid JSON"},
		{"overridden", func(tool *schemacachepb.ToolSpec) {
			tool.FieldProvenance.Items[0].Value.OverriddenCandidates = &schemacachepb.CandidateList{Items: []*schemacachepb.FieldCandidate{{Value: &schemacachepb.BytesValue{Value: []byte("{")}}}}
		}, "candidate 0 value is invalid JSON"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var message schemacachepb.SchemaProductCache
			if err := proto.Unmarshal(built.ProductShards, &message); err != nil {
				t.Fatal(err)
			}
			test.edit(message.Product.Tools.Items[0])
			payload, err := MarshalSchemaCacheDeterministic(&message)
			if err != nil {
				t.Fatal(err)
			}
			descriptor := meta.ProductDescriptors[0]
			descriptor.Length, descriptor.SHA256 = uint64(len(payload)), sha256.Sum256(payload)
			changed := meta
			changed.ProductDescriptors = []ProductDescriptor{descriptor}
			_, err = DecodeSchemaProductCache(payload, descriptor, changed)
			if err == nil || !strings.Contains(err.Error(), test.location) || !strings.Contains(err.Error(), "is invalid JSON") {
				t.Fatalf("error = %v, want JSON failure at %q", err, test.location)
			}
		})
	}
}
