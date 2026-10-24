// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package chat

import (
	"encoding/json"
	"strings"
	"testing"
)

// The live gateway does not recognize HTML-escaped markers in this nested JSON.
// Decoding that string before asserting would hide HTML-escaped wire markers.
func TestCrossPlatformCoverageReplyMentionPreservesWirePlaceholders(t *testing.T) {
	for _, tc := range []struct {
		name, marker string
		flags        []string
	}{
		{"member", "<@" + fixtureCurrentDOpenID + ">", []string{"--at-open-dingtalk-ids", fixtureCurrentDOpenID}},
		{"all", "<@all>", []string{"--at-all"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &larkAlignmentCaller{responses: map[string]string{
				"im/list_messages_by_ids": `{"result":[{"openMessageId":"msg","openConversationId":"cid","senderOpenDingTalkId":"` + fixtureCurrentDOpenID + `"}]}`,
			}}
			args := []string{"+messages-reply", "--message-id", "msg", "--content", "Literal <tag> & \"quoted\" text", "--dry-run"}
			args = append(args, tc.flags...)
			out, err := runChatParity(t, f, args...)
			if err != nil {
				t.Fatal(err)
			}
			var result map[string]any
			if err := json.Unmarshal([]byte(out), &result); err != nil {
				t.Fatal(err)
			}
			wire := result["arguments"].(map[string]any)["content"].(string)
			if !strings.Contains(wire, tc.marker) || strings.Contains(wire, `\u003c`) || strings.Contains(wire, `\u003e`) {
				t.Fatalf("mention marker must survive in nested JSON wire string; content=%s", wire)
			}
			var content map[string]string
			if err := json.Unmarshal([]byte(wire), &content); err != nil {
				t.Fatal(err)
			}
			if content["replyMsgType"] != "text" || content["referenceOpenMessageId"] != "msg" || content["content"] != tc.marker+" Literal <tag> & \"quoted\" text" {
				t.Fatal(content)
			}
		})
	}
}
