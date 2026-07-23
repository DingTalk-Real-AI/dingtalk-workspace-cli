// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import "testing"

func TestLoadRegistryInterfaceRefsUsesSplitRegistry(t *testing.T) {
	refs := loadRegistryInterfaceRefs()
	if len(refs) == 0 {
		t.Fatal("loadRegistryInterfaceRefs() returned no reviewed commands")
	}

	got, ok := refs["calendar.list_calendars"]
	if !ok {
		t.Fatal("calendar.list_calendars missing from reassembled split registry")
	}
	if got["product_id"] != "calendar" || got["rpc_name"] != "list_calendars" {
		t.Fatalf("calendar.list_calendars ref = %#v", got)
	}
}
