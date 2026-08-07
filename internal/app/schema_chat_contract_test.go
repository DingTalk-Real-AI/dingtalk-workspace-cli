// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import "testing"

func TestCrossPlatformCoverageChatPinAndTopKeepTypedPrimaryPaths(t *testing.T) {
	typedPaths := map[string]string{
		"chat.set_pin_message":   "chat message set-pin-msg",
		"chat.unset_pin_message": "chat message unset-pin-msg",
		"chat.set_top_message":   "chat message set-top-msg",
		"chat.unset_top_message": "chat message unset-top-msg",
	}
	canonicals := make([]string, 0, len(typedPaths))
	for canonical := range typedPaths {
		canonicals = append(canonicals, canonical)
	}
	payload := schemaContractPayloadForBoundCanonicals(t, NewRootCommand(), canonicals...)
	for canonical, wantPath := range typedPaths {
		if got := schemaContractString(payload.Tools[canonical]["primary_cli_path"]); got != wantPath {
			t.Errorf("%s primary_cli_path = %q, want %q", canonical, got, wantPath)
		}
	}
}
