// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemaruntime

import (
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemacachepb"
	"google.golang.org/protobuf/proto"
)

func TestCrossPlatformCoverageFlatMetaPreservesAllListPresenceCombinations(t *testing.T) {
	// Independently exercise nil, present-empty and nonempty for all six lists.
	// Encoding/decoding protobuf is essential: it collapses empty repeated fields.
	for combination := 0; combination < 729; combination++ {
		want := CommandMeta{
			Identity:  CommandIdentity{CLIPath: "sample group run", Canonical: "sample.run", ProductID: "sample", Title: "示例 <&>"},
			Safety:    CommandSafety{Effect: "read", Risk: "low", Confirmation: "none", Idempotency: "true"},
			Selection: CommandSelection{AgentSummary: "summary"},
		}
		fields := []*[]string{&want.Identity.Aliases, &want.Selection.UseWhen, &want.Selection.AvoidWhen, &want.Selection.Prerequisites, &want.Selection.Tips, &want.Selection.Examples}
		states := combination
		var wantMask uint32
		for bit, field := range fields {
			if states%3 != 0 {
				wantMask |= 1 << bit
			}
			switch states % 3 {
			case 1:
				*field = []string{}
			case 2:
				*field = []string{"one", "", "中文"}
			}
			states /= 3
		}
		encodedRow := commandMetaToProto(want)
		if encodedRow.ListsPresent != wantMask {
			t.Fatalf("combination %d changed presence bit ordering", combination)
		}
		encoded, err := proto.Marshal(encodedRow)
		if err != nil {
			t.Fatal(err)
		}
		var row schemacachepb.CommandMetaEntry
		if err := proto.Unmarshal(encoded, &row); err != nil {
			t.Fatal(err)
		}
		if err := validateCommandMetaListPresence(&row); err != nil {
			t.Fatalf("combination %d: %v", combination, err)
		}
		got := commandMetaFromProto(&row)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("combination %d lost metadata or list presence", combination)
		}
		for _, values := range commandMetaProtoLists(&row) {
			if len(values) > 0 {
				values[0] = "mutated protobuf"
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("combination %d retained protobuf backing slices", combination)
		}
	}
}

func TestCrossPlatformCoverageFlatMetaRejectsInconsistentListPresence(t *testing.T) {
	for bit := 0; bit < commandMetaListCount; bit++ {
		row := &schemacachepb.CommandMetaEntry{}
		fields := []*[]string{&row.Aliases, &row.UseWhen, &row.AvoidWhen, &row.Prerequisites, &row.Tips, &row.Examples}
		*fields[bit] = []string{"value"}
		if err := validateCommandMetaListPresence(row); err == nil {
			t.Fatalf("list %d without presence accepted", bit)
		}
	}
	for _, mask := range []uint32{1 << commandMetaListCount, 1 << 31, ^uint32(0)} {
		if err := validateCommandMetaListPresence(&schemacachepb.CommandMetaEntry{ListsPresent: mask}); err == nil {
			t.Fatalf("unknown presence mask %x accepted", mask)
		}
	}
}
