// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package contact

import (
	"encoding/json"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/spf13/cobra"
)

func TestStrictFriendListProjectsCleanList(t *testing.T) {
	const raw = `{
		"success": true,
		"result": {
			"friendList": [
				{"dingtalkId":"alice","alias":"Alice","remark":"colleague","status":1,"gmtCreate":1758422400000},
				{"dingtalkId":"bob","alias":"Bob","status":1}
			],
			"cursor": 100,
			"hasMore": true
		}
	}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	friends, cursor, hasMore, err := strictFriendList(data, friendOperationList)
	if err != nil {
		t.Fatalf("projection error: %v", err)
	}
	if len(friends) != 2 {
		t.Fatalf("want 2 friends, got %d (%v)", len(friends), friends)
	}
	if friends[0]["dingtalkId"] != "alice" || friends[0]["remark"] != "colleague" {
		t.Fatalf("first friend projection mismatch: %v", friends[0])
	}
	if _, exists := friends[1]["remark"]; exists {
		t.Fatalf("second friend should not have empty remark field")
	}
	if cursor != 100 || !hasMore {
		t.Fatalf("pagination mismatch: cursor=%d hasMore=%v", cursor, hasMore)
	}
}

func TestStrictFriendListEmpty(t *testing.T) {
	friends, cursor, hasMore, err := strictFriendList(map[string]any{
		"success": true,
		"result":  map[string]any{"friendList": []any{}, "cursor": 0, "hasMore": false},
	}, friendOperationList)
	if err != nil {
		t.Fatalf("projection error: %v", err)
	}
	if len(friends) != 0 || cursor != 0 || hasMore {
		t.Fatalf("empty list mismatch: %d, cursor=%d hasMore=%v", len(friends), cursor, hasMore)
	}
}

func TestStrictFriendListRejectsMissingDingtalkId(t *testing.T) {
	_, _, _, err := strictFriendList(map[string]any{
		"success": true,
		"result":  map[string]any{"friendList": []any{map[string]any{"alias": "Alice"}}},
	}, friendOperationList)
	if err == nil {
		t.Fatal("expected error for missing dingtalkId")
	}
}

func TestStrictFriendListRejectsDuplicateDingtalkId(t *testing.T) {
	_, _, _, err := strictFriendList(map[string]any{
		"success": true,
		"result":  map[string]any{"friendList": []any{map[string]any{"dingtalkId": "alice"}, map[string]any{"dingtalkId": "alice"}}},
	}, friendOperationList)
	if err == nil {
		t.Fatal("expected error for duplicate dingtalkId")
	}
}

func TestStrictFriendRequestListProjectsCleanList(t *testing.T) {
	const raw = `{
		"success": true,
		"result": {
			"friendList": [
				{"dingtalkId":"alice","status":0,"remark":"hi","modifyAt":1758422400000,"isRead":false},
				{"dingtalkId":"bob","status":1,"modifyAt":1758422500000,"isRead":true}
			],
			"cursor": 50,
			"hasMore": false,
			"pendingCount": 1
		}
	}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	requests, cursor, hasMore, pendingCount, err := strictFriendRequestList(data, friendOperationRequestList)
	if err != nil {
		t.Fatalf("projection error: %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("want 2 requests, got %d (%v)", len(requests), requests)
	}
	if requests[0]["dingtalkId"] != "alice" || requests[0]["isRead"] != false {
		t.Fatalf("first request projection mismatch: %v", requests[0])
	}
	if cursor != 50 || hasMore || pendingCount != 1 {
		t.Fatalf("pagination mismatch: cursor=%d hasMore=%v pendingCount=%d", cursor, hasMore, pendingCount)
	}
}

func TestStrictFriendRequestListEmpty(t *testing.T) {
	requests, cursor, hasMore, pendingCount, err := strictFriendRequestList(map[string]any{
		"success": true,
		"result":  map[string]any{"friendList": []any{}, "cursor": 0, "hasMore": false, "pendingCount": 0},
	}, friendOperationRequestList)
	if err != nil {
		t.Fatalf("projection error: %v", err)
	}
	if len(requests) != 0 || cursor != 0 || hasMore || pendingCount != 0 {
		t.Fatalf("empty list mismatch: %d, cursor=%d hasMore=%v pendingCount=%d", len(requests), cursor, hasMore, pendingCount)
	}
}

func TestStrictFriendRequestListRejectsMissingDingtalkId(t *testing.T) {
	_, _, _, _, err := strictFriendRequestList(map[string]any{
		"success": true,
		"result":  map[string]any{"friendList": []any{map[string]any{"status": 0}}},
	}, friendOperationRequestList)
	if err == nil {
		t.Fatal("expected error for missing dingtalkId")
	}
}

// mountForTest registers flags onto a cobra command the same way the shortcut
// framework does.
func mountForTest(s shortcut.Shortcut) *cobra.Command {
	cmd := &cobra.Command{Use: s.Command}
	for _, f := range s.Flags {
		switch f.Type {
		case shortcut.FlagInt:
			cmd.Flags().Int(f.Name, 0, f.Desc)
		default:
			cmd.Flags().String(f.Name, "", f.Desc)
		}
	}
	return cmd
}

func TestFriendValidationRejectsBlankRequiredFlags(t *testing.T) {
	cases := []struct {
		name     string
		shortcut shortcut.Shortcut
		flag     string
		value    string
	}{
		{"send-to", SendFriendRequest, "to", "   "},
		{"accept-from", AcceptFriendRequest, "from", "   "},
		{"reject-from", RejectFriendRequest, "from", "   "},
		{"remove-friend", RemoveFriend, "friend", "   "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := mountForTest(tc.shortcut)
			_ = cmd.Flags().Set(tc.flag, tc.value)
			rt := shortcut.RuntimeContextForTest(cmd, tc.shortcut)
			if err := tc.shortcut.Validate(rt); err == nil {
				t.Fatal("expected validation error for blank flag")
			}
		})
	}
}

func TestFriendValidationRejectsInvalidSize(t *testing.T) {
	for _, s := range []shortcut.Shortcut{ListFriends, ListFriendRequests} {
		cmd := mountForTest(s)
		_ = cmd.Flags().Set("size", "0")
		rt := shortcut.RuntimeContextForTest(cmd, s)
		if err := s.Validate(rt); err == nil {
			t.Fatalf("%s: expected validation error for size=0", s.Command)
		}
	}
}

func TestFriendValidationRejectsNegativeCursor(t *testing.T) {
	for _, s := range []shortcut.Shortcut{ListFriends, ListFriendRequests} {
		cmd := mountForTest(s)
		_ = cmd.Flags().Set("cursor", "-1")
		rt := shortcut.RuntimeContextForTest(cmd, s)
		if err := s.Validate(rt); err == nil {
			t.Fatalf("%s: expected validation error for negative cursor", s.Command)
		}
	}
}

func TestFriendCommandsRegistered(t *testing.T) {
	all := shortcut.All()
	for _, name := range []string{"+friend-list", "+friend-request-list", "+friend-request-send", "+friend-request-accept", "+friend-request-reject", "+friend-remove"} {
		found := false
		for _, s := range all {
			if s.Service == "contact" && s.Command == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("friend shortcut %s not registered", name)
		}
	}
}
