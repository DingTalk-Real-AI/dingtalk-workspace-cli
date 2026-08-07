// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import "testing"

func TestCrossPlatformCoverageChatTypedAndShortcutWriteSafetyParity(t *testing.T) {
	pairs := []struct {
		name             string
		typed            string
		shortcut         string
		wantConfirmation string
	}{
		{name: "send", typed: "chat.send_personal_message", shortcut: "chat.shortcut_messages_send", wantConfirmation: "not_required"},
		{name: "send by bot", typed: "chat.send_robot_message", shortcut: "chat.shortcut_messages_send_by_bot", wantConfirmation: "not_required"},
		{name: "send by webhook", typed: "chat.send_message_by_custom_robot", shortcut: "chat.shortcut_messages_send_by_webhook", wantConfirmation: "not_required"},
		{name: "reply", typed: "chat.reply_personal_message", shortcut: "chat.shortcut_messages_reply", wantConfirmation: "not_required"},
		{name: "recall", typed: "chat.recall_message", shortcut: "chat.shortcut_messages_recall", wantConfirmation: "not_required"},
		{name: "recall by bot", typed: "chat.recall_robot_message", shortcut: "chat.shortcut_messages_recall_by_bot", wantConfirmation: "not_required"},
		{name: "forward", typed: "chat.forward_message", shortcut: "chat.shortcut_messages_forward", wantConfirmation: "not_required"},
		{name: "combine forward", typed: "chat.combine_forward_messages", shortcut: "chat.shortcut_messages_combine_forward", wantConfirmation: "not_required"},
		{name: "forward topic", typed: "chat.forward_topic", shortcut: "chat.shortcut_messages_forward_topic", wantConfirmation: "not_required"},
		{name: "add emoji", typed: "chat.add_emoji_reaction", shortcut: "chat.shortcut_messages_add_emoji", wantConfirmation: "not_required"},
		{name: "remove emoji", typed: "chat.remove_emoji_reaction", shortcut: "chat.shortcut_messages_remove_emoji", wantConfirmation: "not_required"},
		{name: "add text emotion", typed: "chat.add_text_emotion", shortcut: "chat.shortcut_messages_add_text_emotion", wantConfirmation: "not_required"},
		{name: "remove text emotion", typed: "chat.remove_text_emotion", shortcut: "chat.shortcut_messages_remove_text_emotion", wantConfirmation: "not_required"},
		{name: "create text emotion", typed: "chat.create_text_emotion", shortcut: "chat.shortcut_messages_create_text_emotion", wantConfirmation: "not_required"},
		{name: "send card", typed: "chat.create_and_send_card", shortcut: "chat.shortcut_messages_send_card", wantConfirmation: "not_required"},
		{name: "update card", typed: "chat.update_streaming_card", shortcut: "chat.shortcut_messages_update_card", wantConfirmation: "not_required"},
		{name: "set pin", typed: "chat.set_pin_message", shortcut: "chat.shortcut_messages_set_pin", wantConfirmation: "not_required"},
		{name: "unset pin", typed: "chat.unset_pin_message", shortcut: "chat.shortcut_messages_unset_pin", wantConfirmation: "not_required"},
		{name: "set top", typed: "chat.set_top_message", shortcut: "chat.shortcut_messages_set_top", wantConfirmation: "not_required"},
		{name: "unset top", typed: "chat.unset_top_message", shortcut: "chat.shortcut_messages_unset_top", wantConfirmation: "not_required"},
	}

	canonicals := make([]string, 0, len(pairs)*2)
	for _, pair := range pairs {
		canonicals = append(canonicals, pair.typed, pair.shortcut)
	}
	payload := schemaContractPayloadForBoundCanonicals(t, NewRootCommand(), canonicals...)
	for _, pair := range pairs {
		pair := pair
		t.Run(pair.name, func(t *testing.T) {
			typed := payload.Tools[pair.typed]
			shortcut := payload.Tools[pair.shortcut]
			for _, field := range []string{"effect", "risk", "confirmation", "idempotency"} {
				if typed[field] != shortcut[field] {
					t.Errorf("%s: typed=%#v shortcut=%#v", field, typed[field], shortcut[field])
				}
			}
			if typed["confirmation"] != pair.wantConfirmation {
				t.Errorf("confirmation = %#v, want %s", typed["confirmation"], pair.wantConfirmation)
			}
		})
	}
}

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
