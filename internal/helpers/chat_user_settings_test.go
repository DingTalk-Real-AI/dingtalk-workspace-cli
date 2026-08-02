package helpers

import (
	"strings"
	"testing"
)

func TestCrossPlatformCoverageChatGroupUserSettingsQueryValidation(t *testing.T) {
	caller := &guardedMutationCaller{}
	if err := executeGuardedMutationCommand(t, caller, newChatCommand,
		"group", "user-settings", "query", "--groups", ","); err == nil {
		t.Fatal("empty groups returned nil")
	}

	tooMany := make([]string, 101)
	for i := range tooMany {
		tooMany[i] = "cid"
	}
	if err := executeGuardedMutationCommand(t, caller, newChatCommand,
		"group", "user-settings", "query", "--groups", strings.Join(tooMany, ",")); err == nil {
		t.Fatal("101 groups returned nil")
	}
}

func TestCrossPlatformCoverageChatGroupUserSettingsSetValidation(t *testing.T) {
	caller := &guardedMutationCaller{}
	if err := executeGuardedMutationCommand(t, caller, newChatCommand,
		"group", "user-settings", "set", "--items", "[]"); err == nil {
		t.Fatal("empty items returned nil")
	}

	var sb strings.Builder
	sb.WriteString("[")
	for i := 0; i < 101; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"openConversationId":"cid"}`)
	}
	sb.WriteString("]")
	if err := executeGuardedMutationCommand(t, caller, newChatCommand,
		"group", "user-settings", "set", "--items", sb.String()); err == nil {
		t.Fatal("101 items returned nil")
	}
}

func TestCrossPlatformCoverageChatGroupUserSettingsSetItemValidation(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newChatCommand,
		"group", "user-settings", "set", "--items", `[{"top":true}]`)
	if err == nil || !strings.Contains(err.Error(), "openConversationId") || !strings.Contains(err.Error(), "--items[0]") {
		t.Fatalf("err = %v, want missing openConversationId error", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("calls = %#v, want no MCP call on invalid item", caller.calls)
	}

	// 非首项缺失时错误需带正确下标；空白字符串同样视为缺失。
	caller = &guardedMutationCaller{}
	err = executeGuardedMutationCommand(t, caller, newChatCommand,
		"group", "user-settings", "set", "--items", `[{"openConversationId":"cid1"},{"openConversationId":"  "}]`)
	if err == nil || !strings.Contains(err.Error(), "--items[1]") {
		t.Fatalf("err = %v, want --items[1] error", err)
	}

	// 合法条目仍然成功下发。
	caller = &guardedMutationCaller{}
	if err := executeGuardedMutationCommand(t, caller, newChatCommand,
		"group", "user-settings", "set", "--items", `[{"openConversationId":"cid1","top":true,"mute":false}]`); err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 || caller.calls[0].toolName != "batch_update_group_chat_settings" {
		t.Fatalf("calls = %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageChatGroupUserSettingsSetHappyPath(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newChatCommand,
		"group", "user-settings", "set", "--items", `[{"openConversationId":"cid1","top":true}]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v", caller.calls)
	}
	call := caller.calls[0]
	if call.productID != "im" || call.toolName != "batch_update_group_chat_settings" {
		t.Fatalf("call = %#v", call)
	}
}

func TestCrossPlatformCoverageChatMessageSendLocation(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newChatCommand,
		"message", "send", "--group", "cid1", "--msg-type", "location", "--latitude", "39.9")
	if err == nil || !strings.Contains(err.Error(), "are all required for msgType=location") {
		t.Fatalf("err = %v, want missing location params", err)
	}

	caller = &guardedMutationCaller{}
	err = executeGuardedMutationCommand(t, caller, newChatCommand,
		"message", "send", "--group", "cid1", "--msg-type", "location",
		"--latitude", "39.9", "--longitude", "116.4", "--location-name", "国贸", "--map-thumbnail-url", "@media1")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v", caller.calls)
	}
	call := caller.calls[0]
	if call.args["msgType"] != "location" {
		t.Fatalf("msgType = %#v", call.args["msgType"])
	}
	content, _ := call.args["content"].(string)
	if !strings.Contains(content, `"latitude":"39.9"`) || !strings.Contains(content, `"mapThumbnailUrl":"@media1"`) {
		t.Fatalf("content = %s", content)
	}
}

func TestCrossPlatformCoverageChatMessageSendProfile(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newChatCommand,
		"message", "send", "--group", "cid1", "--msg-type", "profile")
	if err == nil || !strings.Contains(err.Error(), "--contact-id is required") {
		t.Fatalf("err = %v, want contact-id required", err)
	}

	caller = &guardedMutationCaller{}
	err = executeGuardedMutationCommand(t, caller, newChatCommand,
		"message", "send", "--group", "cid1", "--msg-type", "profile", "--contact-id", "od123")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v", caller.calls)
	}
	content, _ := caller.calls[0].args["content"].(string)
	if !strings.Contains(content, `"openDingTalkId":"od123"`) {
		t.Fatalf("content = %s", content)
	}
}

func TestCrossPlatformCoverageChatSearchAdvancedWukongAliases(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newChatCommand,
		"message", "search-advanced", "--query", "通知",
		"--only-robot-messages", "--search-conv-type", "group_chat")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v", caller.calls)
	}
	args := caller.calls[0].args
	if args["onlyRobotMessages"] != true {
		t.Fatalf("onlyRobotMessages = %#v", args["onlyRobotMessages"])
	}
	if args["searchConvType"] != "group_chat" {
		t.Fatalf("searchConvType = %#v", args["searchConvType"])
	}
}
