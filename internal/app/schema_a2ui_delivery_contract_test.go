// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
package app

import "testing"

func TestCrossPlatformCoverageA2UIDeliveryOptionsReachFinalSchema(t *testing.T) {
	root := NewRootCommand()
	payload := schemaContractPayloadForBoundCanonicals(t, root,
		"chat.create_and_send_a2ui_card", "chat.shortcut_messages_send", "aicard.preview",
	)
	for _, tc := range []struct {
		canonical string
		summary   string
	}{
		{"chat.create_and_send_a2ui_card", "summary"},
		{"chat.shortcut_messages_send", "card-summary"},
		{"aicard.preview", "summary"},
	} {
		parameters := schemaContractMap(payload.Tools[tc.canonical]["parameters"])
		if parameters[tc.summary]["type"] != "string" {
			t.Errorf("%s summary parameter=%v", tc.canonical, parameters[tc.summary])
		}
		if parameters["support-forward"]["type"] != "boolean" {
			t.Errorf("%s forwarding parameter=%v", tc.canonical, parameters["support-forward"])
		}
	}
	atomic := schemaContractMap(payload.Tools["chat.create_and_send_a2ui_card"]["parameters"])
	if atomic["summary"]["property"] != "summary" || atomic["support-forward"]["property"] != "supportForward" {
		t.Errorf("MCP field mapping=%v", atomic)
	}
}
