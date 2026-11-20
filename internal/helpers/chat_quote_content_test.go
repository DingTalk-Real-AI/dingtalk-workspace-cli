// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageChatQuoteContentPreservesMentionMarkers(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		markers    []string
	}{
		{"member", "<@" + helperCurrentDOpenID + "> 请确认", []string{"<@" + helperCurrentDOpenID + ">"}},
		{"all", "<@all> 请确认", []string{"<@all>"}},
		{"combined", "<@all> <@" + helperCurrentDOpenID + ">", []string{"<@all>", "<@" + helperCurrentDOpenID + ">"}},
		{"plain text", "Literal <tag> & \"quoted\" text\nC:\\work", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire := ChatQuoteContent("message", helperCurrentDOpenID, tc.body)
			for _, marker := range tc.markers {
				if !strings.Contains(wire, marker) {
					t.Fatalf("nested JSON must preserve literal mention marker %q: %s", marker, wire)
				}
			}
			var got map[string]string
			if err := json.Unmarshal([]byte(wire), &got); err != nil {
				t.Fatal(err)
			}
			want := map[string]string{
				"referenceOpenMessageId": "message", "srcMsgSendOpenDingTalkId": helperCurrentDOpenID,
				"replyMsgType": "text", "content": tc.body,
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("quote content = %#v; want %#v", got, want)
			}
		})
	}
}
