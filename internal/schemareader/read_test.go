// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemareader

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemaruntime"
)

func TestCrossPlatformCoverageDescriptorAndLocatorLookups(t *testing.T) {
	meta := schemaruntime.DecodedSchemaMeta{
		ProductDescriptors: []schemaruntime.ProductDescriptor{
			{ProductID: "calendar"},
			{ProductID: "drive"},
		},
		LocatorProductByPath: map[string]string{
			"calendar event create": "calendar",
			"calendar.event.create": "calendar",
		},
	}
	got, ok := Descriptor(meta, "drive")
	if !ok || got.ProductID != "drive" {
		t.Fatalf("Descriptor(drive) = %#v, %v", got, ok)
	}
	if _, ok := Descriptor(meta, "mail"); ok {
		t.Fatal("unknown product must miss")
	}

	index := schemaruntime.DecodedSchemaPayloadIndex{
		PayloadDescriptors: []schemaruntime.CommandPayloadDescriptor{
			{ProductID: "calendar", HeaderLength: 8},
			{ProductID: "drive", HeaderLength: 4},
		},
		LocatorProductByPath: map[string]string{
			"drive copy": "drive",
			"drive.copy": "drive",
		},
	}
	payload, ok := PayloadDescriptor(index, "calendar")
	if !ok || payload.HeaderLength != 8 {
		t.Fatalf("PayloadDescriptor(calendar) = %#v, %v", payload, ok)
	}
	if _, ok := PayloadDescriptor(index, "mail"); ok {
		t.Fatal("unknown payload product must miss")
	}

	if product, ok := Locator(meta, "calendar event create"); !ok || product != "calendar" {
		t.Fatalf("Locator = %q, %v", product, ok)
	}
	if product, ok := Locator(meta, "calendar.event.create"); !ok || product != "calendar" {
		t.Fatalf("Locator dotted = %q, %v", product, ok)
	}
	if _, ok := Locator(meta, "missing"); ok {
		t.Fatal("missing locator must miss")
	}
	if product, ok := IndexLocator(index, "drive copy"); !ok || product != "drive" {
		t.Fatalf("IndexLocator = %q, %v", product, ok)
	}
	if _, ok := IndexLocator(index, "missing"); ok {
		t.Fatal("missing index locator must miss")
	}
}
