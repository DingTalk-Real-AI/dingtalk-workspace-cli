// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package contact

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/spf13/cobra"
)

func TestStrictFriendListProjectsCleanList(t *testing.T) {
	const raw = `{
		"success": true,
		"result": {
			"friendList": [
				{"openDingTalkId":"open-dt-alice","alias":"Alice","remark":"colleague","status":1,"gmtCreate":1758422400000,"userProfileModel":{"nick":"Alice Nick"}},
				{"openDingTalkId":"open-dt-bob","alias":"Bob","status":1,"userProfileModel":{"nick":"Bob Nick"}}
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
	if friends[0]["openDingTalkId"] != "open-dt-alice" || friends[0]["remark"] != "colleague" {
		t.Fatalf("first friend projection mismatch: %v", friends[0])
	}
	if friends[0]["nick"] != "Alice Nick" || friends[1]["nick"] != "Bob Nick" {
		t.Fatalf("nick projection mismatch: %v / %v", friends[0], friends[1])
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

func TestStrictFriendListOmitsMissingNick(t *testing.T) {
	friends, _, _, err := strictFriendList(map[string]any{
		"success": true,
		"result": map[string]any{"friendList": []any{
			map[string]any{"openDingTalkId": "open-dt-alice", "status": 1},
			map[string]any{"openDingTalkId": "open-dt-bob", "status": 1, "userProfileModel": map[string]any{"nick": ""}},
		}, "cursor": 0, "hasMore": false},
	}, friendOperationList)
	if err != nil {
		t.Fatalf("projection error: %v", err)
	}
	if _, exists := friends[0]["nick"]; exists {
		t.Fatalf("friend without userProfileModel should not have nick field")
	}
	if _, exists := friends[1]["nick"]; exists {
		t.Fatalf("friend with empty nick should not have nick field")
	}
}

func TestStrictFriendListRejectsMissingOpenDingTalkId(t *testing.T) {
	_, _, _, err := strictFriendList(map[string]any{
		"success": true,
		"result":  map[string]any{"friendList": []any{map[string]any{"alias": "Alice"}}, "hasMore": false},
	}, friendOperationList)
	if err == nil {
		t.Fatal("expected error for missing openDingTalkId")
	}
}

func TestStrictFriendListRejectsDuplicateOpenDingTalkId(t *testing.T) {
	_, _, _, err := strictFriendList(map[string]any{
		"success": true,
		"result":  map[string]any{"friendList": []any{map[string]any{"openDingTalkId": "open-dt-alice"}, map[string]any{"openDingTalkId": "open-dt-alice"}}, "hasMore": false},
	}, friendOperationList)
	if err == nil {
		t.Fatal("expected error for duplicate openDingTalkId")
	}
}

func TestStrictFriendRequestListProjectsCleanList(t *testing.T) {
	const raw = `{
		"success": true,
		"result": {
			"friendList": [
				{"openDingTalkId":"open-dt-alice","status":0,"remark":"hi","modifyAt":1758422400000,"isRead":false,"userProfileModel":{"nick":"Alice Nick"}},
				{"openDingTalkId":"open-dt-bob","status":1,"modifyAt":1758422500000,"isRead":true,"userProfileModel":{"nick":"Bob Nick"}}
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
	if requests[0]["openDingTalkId"] != "open-dt-alice" || requests[0]["isRead"] != false {
		t.Fatalf("first request projection mismatch: %v", requests[0])
	}
	if requests[0]["nick"] != "Alice Nick" || requests[1]["nick"] != "Bob Nick" {
		t.Fatalf("nick projection mismatch: %v / %v", requests[0], requests[1])
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

func TestStrictFriendRequestListOmitsMissingNick(t *testing.T) {
	requests, _, _, _, err := strictFriendRequestList(map[string]any{
		"success": true,
		"result": map[string]any{"friendList": []any{
			map[string]any{"openDingTalkId": "open-dt-alice", "status": 1},
		}, "cursor": 0, "hasMore": false, "pendingCount": 0},
	}, friendOperationRequestList)
	if err != nil {
		t.Fatalf("projection error: %v", err)
	}
	if _, exists := requests[0]["nick"]; exists {
		t.Fatalf("request without userProfileModel should not have nick field")
	}
}

func TestStrictFriendRequestListRejectsMissingOpenDingTalkId(t *testing.T) {
	_, _, _, _, err := strictFriendRequestList(map[string]any{
		"success": true,
		"result":  map[string]any{"friendList": []any{map[string]any{"status": 0}}, "hasMore": false},
	}, friendOperationRequestList)
	if err == nil {
		t.Fatal("expected error for missing openDingTalkId")
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

// Execute-path coverage: the strict-helper and validation tests above never
// reach the Execute closures, their transport error branches or
// parseFriendCursor. These tests pin the MCP mapping for every friend command.

func friendExecuteCmd(t *testing.T, decl shortcut.Shortcut, flags map[string]string) *cobra.Command {
	t.Helper()
	cmd := mountForTest(decl)
	for name, value := range flags {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s=%s: %v", name, value, err)
		}
	}
	return cmd
}

func TestFriendListExecuteSuccessTransportErrorAndMalformed(t *testing.T) {
	caller := &contactCaller{payloads: map[string]string{
		"get_friend_list": `{"success":true,"result":{"cursor":7,"hasMore":true,"friendList":[{"openDingTalkId":"friend-1","nick":"Ann"}]}}`,
	}}
	helpers.InitDepsForTest(t, caller)

	decl := ListFriends
	cmd := friendExecuteCmd(t, decl, map[string]string{"cursor": "7", "size": "50"})
	if err := decl.Validate(shortcut.RuntimeContextForTest(cmd, decl)); err != nil {
		t.Fatalf("valid flags rejected: %v", err)
	}
	if err := decl.Execute(shortcut.RuntimeContextForTest(cmd, decl)); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if caller.calls != 1 || caller.product != "contact" || caller.tool != "get_friend_list" {
		t.Fatalf("call = calls:%d product:%q tool:%q", caller.calls, caller.product, caller.tool)
	}
	if caller.args["cursor"] != 7 || caller.args["size"] != 50 {
		t.Fatalf("pagination args = %#v", caller.args)
	}

	caller.calls = 0
	caller.errors = map[string]error{"get_friend_list": errors.New("transport down")}
	if err := decl.Execute(shortcut.RuntimeContextForTest(cmd, decl)); err == nil {
		t.Fatal("transport error swallowed by execute")
	}

	caller.calls = 0
	caller.errors = nil
	caller.payloads["get_friend_list"] = `{"success":true}`
	if err := decl.Execute(shortcut.RuntimeContextForTest(cmd, decl)); err == nil {
		t.Fatal("envelope without result accepted")
	}
}

func TestFriendRequestListExecuteCursorPaginationAndErrors(t *testing.T) {
	caller := &contactCaller{payloads: map[string]string{
		"get_friend_request_list": `{"success":true,"result":{"cursor":3,"hasMore":false,"pendingCount":2,"friendList":[{"openDingTalkId":"req-1","status":1}]}}`,
	}}
	helpers.InitDepsForTest(t, caller)

	decl := ListFriendRequests
	cmd := friendExecuteCmd(t, decl, map[string]string{"cursor": "3", "size": "20"})
	if err := decl.Validate(shortcut.RuntimeContextForTest(cmd, decl)); err != nil {
		t.Fatalf("valid flags rejected: %v", err)
	}
	if err := decl.Execute(shortcut.RuntimeContextForTest(cmd, decl)); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if caller.calls != 1 || caller.tool != "get_friend_request_list" {
		t.Fatalf("call = calls:%d tool:%q", caller.calls, caller.tool)
	}
	if caller.args["cursor"] != 3 || caller.args["size"] != 20 {
		t.Fatalf("pagination args = %#v", caller.args)
	}

	// Cursor defaults to the first page and stays omitted.
	caller.calls, caller.history = 0, nil
	firstPage := friendExecuteCmd(t, decl, map[string]string{"size": "20"})
	if err := decl.Execute(shortcut.RuntimeContextForTest(firstPage, decl)); err != nil {
		t.Fatalf("first page execute: %v", err)
	}
	if _, present := caller.args["cursor"]; present {
		t.Fatalf("first page leaked cursor: %#v", caller.args)
	}

	caller.calls = 0
	caller.errors = map[string]error{"get_friend_request_list": errors.New("transport down")}
	if err := decl.Execute(shortcut.RuntimeContextForTest(firstPage, decl)); err == nil {
		t.Fatal("transport error swallowed by execute")
	}

	caller.calls = 0
	caller.errors = nil
	caller.payloads["get_friend_request_list"] = `{"success":true,"result":{"friendList":7}}`
	if err := decl.Execute(shortcut.RuntimeContextForTest(firstPage, decl)); err == nil {
		t.Fatal("non-array friendList accepted")
	}
}

func TestFriendRequestSendExecuteMapsRemark(t *testing.T) {
	caller := &contactCaller{}
	helpers.InitDepsForTest(t, caller)

	decl := SendFriendRequest
	cmd := friendExecuteCmd(t, decl, map[string]string{"to": "  olz_target  ", "remark": "见字如面"})
	rt := shortcut.RuntimeContextForTest(cmd, decl)
	if err := decl.Validate(rt); err != nil {
		t.Fatalf("valid flags rejected: %v", err)
	}
	if err := decl.Execute(rt); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if caller.calls != 1 || caller.tool != "send_friend_request" {
		t.Fatalf("call = calls:%d tool:%q", caller.calls, caller.tool)
	}
	if caller.args["destOpenDingtalkId"] != "olz_target" || caller.args["remark"] != "见字如面" {
		t.Fatalf("send args = %#v", caller.args)
	}

	caller.calls, caller.history = 0, nil
	noRemark := friendExecuteCmd(t, decl, map[string]string{"to": "olz_target"})
	if err := decl.Execute(shortcut.RuntimeContextForTest(noRemark, decl)); err != nil {
		t.Fatalf("plain execute: %v", err)
	}
	if _, present := caller.args["remark"]; present {
		t.Fatalf("unset remark leaked: %#v", caller.args)
	}

	caller.calls = 0
	caller.err = errors.New("transport down")
	if err := decl.Execute(shortcut.RuntimeContextForTest(noRemark, decl)); err == nil {
		t.Fatal("transport error swallowed by execute")
	}
}

func TestFriendRequestAcceptExecuteMapsAlias(t *testing.T) {
	caller := &contactCaller{}
	helpers.InitDepsForTest(t, caller)

	decl := AcceptFriendRequest
	cmd := friendExecuteCmd(t, decl, map[string]string{"from": " olz_from ", "alias": "Alice Li"})
	if err := decl.Execute(shortcut.RuntimeContextForTest(cmd, decl)); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if caller.calls != 1 || caller.tool != "accept_friend_request" {
		t.Fatalf("call = calls:%d tool:%q", caller.calls, caller.tool)
	}
	if caller.args["targetOpenDingtalkId"] != "olz_from" || caller.args["alias"] != "Alice Li" {
		t.Fatalf("accept args = %#v", caller.args)
	}

	caller.calls, caller.history = 0, nil
	noAlias := friendExecuteCmd(t, decl, map[string]string{"from": "olz_from"})
	if err := decl.Execute(shortcut.RuntimeContextForTest(noAlias, decl)); err != nil {
		t.Fatalf("plain execute: %v", err)
	}
	if _, present := caller.args["alias"]; present {
		t.Fatalf("unset alias leaked: %#v", caller.args)
	}
}

func TestFriendRequestRejectAndRemoveExecute(t *testing.T) {
	caller := &contactCaller{}
	helpers.InitDepsForTest(t, caller)

	reject := RejectFriendRequest
	cmd := friendExecuteCmd(t, reject, map[string]string{"from": " olz_reject "})
	if err := reject.Execute(shortcut.RuntimeContextForTest(cmd, reject)); err != nil {
		t.Fatalf("reject execute: %v", err)
	}
	if caller.calls != 1 || caller.tool != "remove_friend_request" || caller.args["targetOpenDingtalkId"] != "olz_reject" {
		t.Fatalf("reject call = calls:%d tool:%q args:%#v", caller.calls, caller.tool, caller.args)
	}

	caller.calls, caller.history = 0, nil
	remove := RemoveFriend
	cmd = friendExecuteCmd(t, remove, map[string]string{"friend": " olz_friend "})
	if err := remove.Execute(shortcut.RuntimeContextForTest(cmd, remove)); err != nil {
		t.Fatalf("remove execute: %v", err)
	}
	if caller.calls != 1 || caller.tool != "remove_friend" || caller.args["targetOpenDingtalkId"] != "olz_friend" {
		t.Fatalf("remove call = calls:%d tool:%q args:%#v", caller.calls, caller.tool, caller.args)
	}

	caller.calls = 0
	caller.err = errors.New("transport down")
	if err := remove.Execute(shortcut.RuntimeContextForTest(cmd, remove)); err == nil {
		t.Fatal("remove transport error swallowed")
	}
}

func TestStrictFriendListRejectsMalformedEnvelopes(t *testing.T) {
	cases := []struct{ name, payload string }{
		{"failure-envelope", `{"success":false,"errorCode":"E","errorMsg":"x"}`},
		{"missing-result", `{"success":true}`},
		{"non-array-friendList", `{"success":true,"result":{"hasMore":false,"friendList":"x"}}`},
		{"non-object-item", `{"success":true,"result":{"hasMore":false,"friendList":["bad"]}}`},
		{"missing-hasmore", `{"success":true,"result":{"cursor":7,"friendList":[]}}`},
		{"non-bool-hasmore", `{"success":true,"result":{"cursor":7,"hasMore":"true","friendList":[]}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var data map[string]any
			if err := json.Unmarshal([]byte(tc.payload), &data); err != nil {
				t.Fatal(err)
			}
			if _, _, _, err := strictFriendList(data, friendOperationList); err == nil {
				t.Fatalf("%s: malformed envelope accepted", tc.name)
			}
		})
	}

	var empty map[string]any
	if err := json.Unmarshal([]byte(`{"success":true,"result":{"cursor":2,"hasMore":true}}`), &empty); err != nil {
		t.Fatal(err)
	}
	friends, cursor, hasMore, err := strictFriendList(empty, friendOperationList)
	if err != nil || len(friends) != 0 || cursor != 2 || !hasMore {
		t.Fatalf("absent friendList = friends:%v cursor:%d hasMore:%v err:%v", friends, cursor, hasMore, err)
	}
}

func TestStrictFriendRequestListRejectsMalformedEnvelopes(t *testing.T) {
	cases := []struct{ name, payload string }{
		{"failure-envelope", `{"success":false,"errorCode":"E","errorMsg":"x"}`},
		{"missing-result", `{"success":true}`},
		{"non-array-friendList", `{"success":true,"result":{"hasMore":false,"pendingCount":0,"friendList":true}}`},
		{"non-object-item", `{"success":true,"result":{"hasMore":false,"pendingCount":0,"friendList":[3]}}`},
		{"duplicate-identity", `{"success":true,"result":{"hasMore":false,"pendingCount":0,"friendList":[{"openDingTalkId":"dup-1"},{"openDingTalkId":"dup-1"}]}}`},
		{"missing-hasmore", `{"success":true,"result":{"cursor":3,"pendingCount":0,"friendList":[]}}`},
		{"non-bool-hasmore", `{"success":true,"result":{"cursor":3,"hasMore":1,"pendingCount":0,"friendList":[]}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var data map[string]any
			if err := json.Unmarshal([]byte(tc.payload), &data); err != nil {
				t.Fatal(err)
			}
			if _, _, _, _, err := strictFriendRequestList(data, friendOperationRequestList); err == nil {
				t.Fatalf("%s: malformed envelope accepted", tc.name)
			}
		})
	}

	var empty map[string]any
	if err := json.Unmarshal([]byte(`{"success":true,"result":{"cursor":9,"hasMore":true,"pendingCount":4}}`), &empty); err != nil {
		t.Fatal(err)
	}
	requests, cursor, hasMore, pendingCount, err := strictFriendRequestList(empty, friendOperationRequestList)
	if err != nil || len(requests) != 0 || cursor != 9 || !hasMore || pendingCount != 4 {
		t.Fatalf("absent friendList = requests:%v cursor:%d hasMore:%v pending:%d err:%v", requests, cursor, hasMore, pendingCount, err)
	}
}

func TestParseFriendCursorVariants(t *testing.T) {
	if value, err := parseFriendCursor(""); err != nil || value != 0 {
		t.Fatalf("empty cursor = %d, %v", value, err)
	}
	if value, err := parseFriendCursor(" 42 "); err != nil || value != 42 {
		t.Fatalf("trimmed cursor = %d, %v", value, err)
	}
	if _, err := parseFriendCursor("abc"); err == nil {
		t.Fatal("non-numeric cursor accepted")
	}
	if _, err := parseFriendCursor("-1"); err == nil {
		t.Fatal("negative cursor accepted")
	}
}

// executeFriendShortcutForEnvelope mounts the shortcut through the real
// corecmd tree, runs Execute with a unified result store, and decodes the
// final JSON envelope so pagination contract assertions see what callers see.
func executeFriendShortcutForEnvelope(t *testing.T, decl shortcut.Shortcut, caller *contactCaller, flags map[string]string) (map[string]any, error) {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	cmd := corecmd.New(shortcut.FromShortcut(decl))
	for name, value := range flags {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s=%s: %v", name, value, err)
		}
	}
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	rt := shortcut.RuntimeContextForTest(cmd, decl)
	if err := decl.Validate(rt); err != nil {
		return nil, err
	}
	if err := decl.Execute(rt); err != nil {
		return nil, err
	}
	code, emitted, err := output.EmitStoredResult(cmd)
	if err != nil || !emitted {
		t.Fatalf("emit result: code=%d emitted=%t err=%v", code, emitted, err)
	}
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	return envelope, nil
}

func TestFriendListEnvelopePaginationTwoStates(t *testing.T) {
	caller := &contactCaller{payloads: map[string]string{
		"get_friend_list": `{"success":true,"result":{"cursor":7,"hasMore":true,"friendList":[{"openDingTalkId":"friend-1","nick":"Ann"}]}}`,
	}}
	envelope, err := executeFriendShortcutForEnvelope(t, ListFriends, caller, map[string]string{"cursor": "7", "size": "50"})
	if err != nil {
		t.Fatal(err)
	}
	data := envelope["data"].(map[string]any)
	pagination := envelope["meta"].(map[string]any)["pagination"].(map[string]any)
	if envelope["outcome"] != "success" || data["hasMore"] != true || data["cursor"] != float64(7) {
		t.Fatalf("resumable envelope = %#v", envelope)
	}
	if pagination["endpoint_exhausted"] != false || pagination["next_token"] != "7" {
		t.Fatalf("resumable pagination = %#v", pagination)
	}

	caller.calls = 0
	caller.payloads["get_friend_list"] = `{"success":true,"result":{"cursor":0,"hasMore":false,"friendList":[{"openDingTalkId":"friend-1"}]}}`
	envelope, err = executeFriendShortcutForEnvelope(t, ListFriends, caller, map[string]string{"size": "20"})
	if err != nil {
		t.Fatal(err)
	}
	pagination = envelope["meta"].(map[string]any)["pagination"].(map[string]any)
	if pagination["endpoint_exhausted"] != true || pagination["next_token"] != nil {
		t.Fatalf("exhausted pagination = %#v", pagination)
	}
}

func TestFriendRequestListEnvelopePaginationTwoStates(t *testing.T) {
	caller := &contactCaller{payloads: map[string]string{
		"get_friend_request_list": `{"success":true,"result":{"cursor":3,"hasMore":true,"pendingCount":2,"friendList":[{"openDingTalkId":"req-1","status":1}]}}`,
	}}
	envelope, err := executeFriendShortcutForEnvelope(t, ListFriendRequests, caller, map[string]string{"cursor": "3", "size": "20"})
	if err != nil {
		t.Fatal(err)
	}
	pagination := envelope["meta"].(map[string]any)["pagination"].(map[string]any)
	if pagination["endpoint_exhausted"] != false || pagination["next_token"] != "3" {
		t.Fatalf("resumable pagination = %#v", pagination)
	}

	caller.calls = 0
	caller.payloads["get_friend_request_list"] = `{"success":true,"result":{"cursor":0,"hasMore":false,"pendingCount":0,"friendList":[]}}`
	envelope, err = executeFriendShortcutForEnvelope(t, ListFriendRequests, caller, map[string]string{"size": "20"})
	if err != nil {
		t.Fatal(err)
	}
	data := envelope["data"].(map[string]any)
	pagination = envelope["meta"].(map[string]any)["pagination"].(map[string]any)
	if data["count"] != float64(0) || pagination["endpoint_exhausted"] != true || pagination["next_token"] != nil {
		t.Fatalf("exhausted envelope = %#v", envelope)
	}
}

func TestFriendListResumablePageWithoutCursorFailsClosed(t *testing.T) {
	caller := &contactCaller{payloads: map[string]string{
		"get_friend_list": `{"success":true,"result":{"cursor":0,"hasMore":true,"friendList":[{"openDingTalkId":"friend-1"}]}}`,
	}}
	if _, err := executeFriendShortcutForEnvelope(t, ListFriends, caller, map[string]string{"size": "20"}); err == nil {
		t.Fatal("resumable page without cursor accepted")
	}
}

func TestFriendRequestListResumablePageWithoutCursorFailsClosed(t *testing.T) {
	caller := &contactCaller{payloads: map[string]string{
		"get_friend_request_list": `{"success":true,"result":{"cursor":0,"hasMore":true,"pendingCount":1,"friendList":[{"openDingTalkId":"req-1","status":1}]}}`,
	}}
	if _, err := executeFriendShortcutForEnvelope(t, ListFriendRequests, caller, map[string]string{"size": "20"}); err == nil {
		t.Fatal("resumable request page without cursor accepted")
	}
}

// Execute-path proof that schema drift on hasMore fails closed with a
// response-validation error instead of publishing endpoint_exhausted.
func TestFriendListsExecuteRejectMalformedHasMore(t *testing.T) {
	caller := &contactCaller{payloads: map[string]string{
		"get_friend_list":         `{"success":true,"result":{"cursor":7,"friendList":[{"openDingTalkId":"friend-1"}]}}`,
		"get_friend_request_list": `{"success":true,"result":{"cursor":3,"pendingCount":0,"friendList":[]}}`,
	}}
	helpers.InitDepsForTest(t, caller)

	for _, tc := range []struct {
		name string
		decl shortcut.Shortcut
	}{
		{"list-missing-hasmore", ListFriends},
		{"request-list-missing-hasmore", ListFriendRequests},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.decl.Execute(shortcut.RuntimeContextForTest(friendExecuteCmd(t, tc.decl, map[string]string{"size": "20"}), tc.decl)); err == nil {
				t.Fatal("missing hasMore accepted")
			}
		})
	}

	caller.calls = 0
	caller.payloads["get_friend_list"] = `{"success":true,"result":{"cursor":7,"hasMore":"true","friendList":[]}}`
	caller.payloads["get_friend_request_list"] = `{"success":true,"result":{"cursor":3,"hasMore":1,"pendingCount":0,"friendList":[]}}`
	for _, tc := range []struct {
		name string
		decl shortcut.Shortcut
	}{
		{"list-non-bool-hasmore", ListFriends},
		{"request-list-non-bool-hasmore", ListFriendRequests},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.decl.Execute(shortcut.RuntimeContextForTest(friendExecuteCmd(t, tc.decl, map[string]string{"size": "20"}), tc.decl)); err == nil {
				t.Fatal("non-bool hasMore accepted")
			}
		})
	}
}
