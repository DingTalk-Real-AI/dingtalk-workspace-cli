// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package contact

import (
	"errors"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// These tests replay the friend strict helpers and execute paths under a
// TestCrossPlatformCoverage name so the native platform coverage gate (which
// only runs TestAllShortcuts/TestCrossPlatformCoverage tests) exercises every
// friend.go statement introduced by this change.

func TestCrossPlatformCoverageContactFriendListsExecutePaths(t *testing.T) {
	caller := &contactCaller{payloads: map[string]string{
		"get_friend_list": `{"success":true,"result":{"cursor":7,"hasMore":true,"friendList":[` +
			`{"openDingTalkId":"open-1","nick":"Ann","alias":"Annie","remark":"school","status":1,"gmtCreate":1758422400000,"userProfileModel":{"nick":"Ann"}},` +
			`{"openDingTalkId":"open-2","status":0}]}}`,
	}}
	helpers.InitDepsForTest(t, caller)

	decl := ListFriends
	cmd := friendExecuteCmd(t, decl, map[string]string{"cursor": "7", "size": "50"})
	if err := decl.Validate(shortcut.RuntimeContextForTest(cmd, decl)); err != nil {
		t.Fatalf("valid flags rejected: %v", err)
	}
	if err := decl.Execute(shortcut.RuntimeContextForTest(cmd, decl)); err != nil {
		t.Fatalf("list execute: %v", err)
	}
	if caller.calls != 1 || caller.product != "contact" || caller.tool != "get_friend_list" || caller.args["cursor"] != 7 || caller.args["size"] != 50 {
		t.Fatalf("list mapping = calls:%d product:%q tool:%q args:%#v", caller.calls, caller.product, caller.tool, caller.args)
	}

	for _, tc := range []struct{ name, flag, value string }{
		{"negative-cursor", "cursor", "-1"},
		{"zero-size", "size", "0"},
		{"oversize", "size", "101"},
	} {
		bad := friendExecuteCmd(t, decl, map[string]string{tc.flag: tc.value})
		if err := decl.Validate(shortcut.RuntimeContextForTest(bad, decl)); err == nil {
			t.Errorf("%s: list accepted %s=%s", tc.name, tc.flag, tc.value)
		}
	}

	caller.calls = 0
	caller.errors = map[string]error{"get_friend_list": errors.New("transport down")}
	if err := decl.Execute(shortcut.RuntimeContextForTest(cmd, decl)); err == nil {
		t.Fatal("list swallowed the transport error")
	}
	caller.errors = nil

	for _, tc := range []struct{ name, payload string }{
		{"failure-envelope", `{"success":false,"errorCode":"E","errorMsg":"x"}`},
		{"missing-result", `{"success":true}`},
		{"missing-hasmore", `{"success":true,"result":{"cursor":0}}`},
		{"non-bool-hasmore", `{"success":true,"result":{"cursor":0,"hasMore":"yes"}}`},
		{"non-array-friendlist", `{"success":true,"result":{"cursor":0,"hasMore":false,"friendList":7}}`},
		{"non-object-item", `{"success":true,"result":{"cursor":0,"hasMore":false,"friendList":["bad"]}}`},
		{"missing-identity", `{"success":true,"result":{"cursor":0,"hasMore":false,"friendList":[{"nick":"no-id"}]}}`},
		{"duplicate-identity", `{"success":true,"result":{"cursor":0,"hasMore":false,"friendList":[{"openDingTalkId":"dup"},{"openDingTalkId":"dup"}]}}`},
	} {
		caller.calls = 0
		caller.payloads["get_friend_list"] = tc.payload
		if err := decl.Execute(shortcut.RuntimeContextForTest(cmd, decl)); err == nil {
			t.Errorf("%s: malformed list response accepted", tc.name)
		}
	}

	caller.payloads["get_friend_list"] = `{"success":true,"result":{"cursor":0,"hasMore":true,"friendList":[{"openDingTalkId":"open-1"}]}}`
	if err := decl.Execute(shortcut.RuntimeContextForTest(cmd, decl)); err == nil {
		t.Fatal("resumable page without a usable cursor accepted")
	}

	decl = ListFriendRequests
	caller.calls, caller.history = 0, nil
	caller.payloads["get_friend_request_list"] = `{"success":true,"result":{"cursor":3,"hasMore":true,"pendingCount":2,` +
		`"friendList":[{"openDingTalkId":"req-1","nick":"Ann","remark":"hi","status":2,"modifyAt":1758422500000,"isRead":false,"userProfileModel":{"nick":"Ann"}}]}}`
	cmd = friendExecuteCmd(t, decl, map[string]string{"cursor": "3", "size": "20"})
	if err := decl.Validate(shortcut.RuntimeContextForTest(cmd, decl)); err != nil {
		t.Fatalf("valid request-list flags rejected: %v", err)
	}
	if err := decl.Execute(shortcut.RuntimeContextForTest(cmd, decl)); err != nil {
		t.Fatalf("request-list execute: %v", err)
	}
	if caller.calls != 1 || caller.tool != "get_friend_request_list" || caller.args["cursor"] != 3 || caller.args["size"] != 20 {
		t.Fatalf("request-list mapping = calls:%d tool:%q args:%#v", caller.calls, caller.tool, caller.args)
	}

	caller.calls, caller.history = 0, nil
	caller.payloads["get_friend_request_list"] = `{"success":true,"result":{"cursor":0,"hasMore":false,"friendList":[]}}`
	firstPage := friendExecuteCmd(t, decl, map[string]string{"size": "20"})
	if err := decl.Execute(shortcut.RuntimeContextForTest(firstPage, decl)); err != nil {
		t.Fatalf("request-list first page: %v", err)
	}
	if _, present := caller.args["cursor"]; present {
		t.Fatalf("first page leaked cursor: %#v", caller.args)
	}

	for _, tc := range []struct{ name, flag, value string }{
		{"negative-cursor", "cursor", "-1"},
		{"zero-size", "size", "0"},
		{"oversize", "size", "101"},
	} {
		bad := friendExecuteCmd(t, decl, map[string]string{tc.flag: tc.value})
		if err := decl.Validate(shortcut.RuntimeContextForTest(bad, decl)); err == nil {
			t.Errorf("%s: request list accepted %s=%s", tc.name, tc.flag, tc.value)
		}
	}

	caller.calls = 0
	caller.errors = map[string]error{"get_friend_request_list": errors.New("transport down")}
	if err := decl.Execute(shortcut.RuntimeContextForTest(firstPage, decl)); err == nil {
		t.Fatal("request list swallowed the transport error")
	}
	caller.errors = nil

	for _, tc := range []struct{ name, payload string }{
		{"failure-envelope", `{"success":false,"errorCode":"E","errorMsg":"x"}`},
		{"missing-result", `{"success":true}`},
		{"missing-hasmore", `{"success":true,"result":{"cursor":0,"pendingCount":0}}`},
		{"non-bool-hasmore", `{"success":true,"result":{"cursor":0,"hasMore":1,"pendingCount":0}}`},
		{"non-array-friendlist", `{"success":true,"result":{"cursor":0,"hasMore":false,"pendingCount":0,"friendList":true}}`},
		{"non-object-item", `{"success":true,"result":{"cursor":0,"hasMore":false,"pendingCount":0,"friendList":[3]}}`},
		{"missing-identity", `{"success":true,"result":{"cursor":0,"hasMore":false,"pendingCount":0,"friendList":[{"status":1}]}}`},
		{"duplicate-identity", `{"success":true,"result":{"cursor":0,"hasMore":false,"pendingCount":0,"friendList":[{"openDingTalkId":"dup"},{"openDingTalkId":"dup"}]}}`},
	} {
		caller.calls = 0
		caller.payloads["get_friend_request_list"] = tc.payload
		if err := decl.Execute(shortcut.RuntimeContextForTest(firstPage, decl)); err == nil {
			t.Errorf("%s: malformed request-list response accepted", tc.name)
		}
	}

	caller.payloads["get_friend_request_list"] = `{"success":true,"result":{"cursor":0,"hasMore":true,"pendingCount":1,"friendList":[]}}`
	if err := decl.Execute(shortcut.RuntimeContextForTest(firstPage, decl)); err == nil {
		t.Fatal("resumable request page without a usable cursor accepted")
	}
}

func TestCrossPlatformCoverageContactFriendWriteCommandsExecute(t *testing.T) {
	caller := &contactCaller{}
	helpers.InitDepsForTest(t, caller)

	decl := SendFriendRequest
	if err := decl.Validate(shortcut.RuntimeContextForTest(friendExecuteCmd(t, decl, map[string]string{"to": "   "}), decl)); err == nil {
		t.Fatal("blank --to accepted")
	}
	cmd := friendExecuteCmd(t, decl, map[string]string{"to": " olz_target ", "remark": "见字如面"})
	if err := decl.Execute(shortcut.RuntimeContextForTest(cmd, decl)); err != nil {
		t.Fatalf("send execute: %v", err)
	}
	if caller.calls != 1 || caller.tool != "send_friend_request" || caller.args["destOpenDingtalkId"] != "olz_target" || caller.args["remark"] != "见字如面" {
		t.Fatalf("send args = %#v", caller.args)
	}

	caller.calls, caller.history = 0, nil
	noRemark := friendExecuteCmd(t, decl, map[string]string{"to": "olz_target"})
	if err := decl.Execute(shortcut.RuntimeContextForTest(noRemark, decl)); err != nil {
		t.Fatalf("plain send: %v", err)
	}
	if _, present := caller.args["remark"]; present {
		t.Fatalf("unset remark leaked: %#v", caller.args)
	}

	caller.err = errors.New("transport down")
	if err := decl.Execute(shortcut.RuntimeContextForTest(noRemark, decl)); err == nil {
		t.Fatal("send swallowed the transport error")
	}
	caller.err = nil

	decl = AcceptFriendRequest
	caller.calls, caller.history = 0, nil
	if err := decl.Validate(shortcut.RuntimeContextForTest(friendExecuteCmd(t, decl, map[string]string{"from": " "}), decl)); err == nil {
		t.Fatal("blank --from accepted")
	}
	cmd = friendExecuteCmd(t, decl, map[string]string{"from": " olz_from ", "alias": "Alice Li"})
	if err := decl.Execute(shortcut.RuntimeContextForTest(cmd, decl)); err != nil {
		t.Fatalf("accept execute: %v", err)
	}
	if caller.calls != 1 || caller.tool != "accept_friend_request" || caller.args["targetOpenDingtalkId"] != "olz_from" || caller.args["alias"] != "Alice Li" {
		t.Fatalf("accept args = %#v", caller.args)
	}

	caller.calls, caller.history = 0, nil
	noAlias := friendExecuteCmd(t, decl, map[string]string{"from": "olz_from"})
	if err := decl.Execute(shortcut.RuntimeContextForTest(noAlias, decl)); err != nil {
		t.Fatalf("plain accept: %v", err)
	}
	if _, present := caller.args["alias"]; present {
		t.Fatalf("unset alias leaked: %#v", caller.args)
	}

	decl = RejectFriendRequest
	caller.calls, caller.history = 0, nil
	if err := decl.Validate(shortcut.RuntimeContextForTest(friendExecuteCmd(t, decl, map[string]string{"from": ""}), decl)); err == nil {
		t.Fatal("empty --from accepted")
	}
	cmd = friendExecuteCmd(t, decl, map[string]string{"from": " olz_reject "})
	if err := decl.Execute(shortcut.RuntimeContextForTest(cmd, decl)); err != nil {
		t.Fatalf("reject execute: %v", err)
	}
	if caller.calls != 1 || caller.tool != "remove_friend_request" || caller.args["targetOpenDingtalkId"] != "olz_reject" {
		t.Fatalf("reject call = calls:%d tool:%q args:%#v", caller.calls, caller.tool, caller.args)
	}

	decl = RemoveFriend
	caller.calls, caller.history = 0, nil
	if err := decl.Validate(shortcut.RuntimeContextForTest(friendExecuteCmd(t, decl, map[string]string{"friend": "\t"}), decl)); err == nil {
		t.Fatal("blank --friend accepted")
	}
	cmd = friendExecuteCmd(t, decl, map[string]string{"friend": " olz_friend "})
	if err := decl.Execute(shortcut.RuntimeContextForTest(cmd, decl)); err != nil {
		t.Fatalf("remove execute: %v", err)
	}
	if caller.calls != 1 || caller.tool != "remove_friend" || caller.args["targetOpenDingtalkId"] != "olz_friend" {
		t.Fatalf("remove call = calls:%d tool:%q args:%#v", caller.calls, caller.tool, caller.args)
	}
}

func TestCrossPlatformCoverageContactFriendStrictHelpersAndPagination(t *testing.T) {
	meta, err := friendListPagination(friendOperationList, false, 0)
	if err != nil || meta == nil || meta.Pagination == nil {
		t.Fatalf("exhausted page rejected: meta=%#v err=%v", meta, err)
	}
	if meta, err = friendListPagination(friendOperationList, true, 12); err != nil || meta == nil || meta.Pagination == nil {
		t.Fatalf("resumable page rejected: meta=%#v err=%v", meta, err)
	}
	if _, err := friendListPagination(friendOperationList, true, 0); err == nil {
		t.Fatal("resumable page without a usable cursor accepted")
	}

	friends, cursor, hasMore, err := strictFriendList(map[string]any{
		"success": true,
		"result":  map[string]any{"cursor": float64(0), "hasMore": false},
	}, friendOperationList)
	if err != nil || len(friends) != 0 || cursor != 0 || hasMore {
		t.Fatalf("absent friendList = %#v cursor:%d hasMore:%v err:%v", friends, cursor, hasMore, err)
	}

	// Failure envelopes only reach the strict helpers through direct calls;
	// the execute path rejects them upstream before projection.
	if _, _, _, err := strictFriendList(map[string]any{
		"success": false, "errorCode": "FAILED", "errorMsg": "x",
	}, friendOperationList); err == nil {
		t.Fatal("failure envelope accepted by strict friend list")
	}
	if _, _, _, _, err := strictFriendRequestList(map[string]any{
		"success": false, "errorCode": "FAILED", "errorMsg": "x",
	}, friendOperationRequestList); err == nil {
		t.Fatal("failure envelope accepted by strict friend request list")
	}

	friends, _, _, err = strictFriendList(map[string]any{
		"success": true,
		"result": map[string]any{"cursor": float64(4), "hasMore": true, "friendList": []any{
			map[string]any{
				"openDingTalkId": "open-full", "alias": "Annie", "remark": "school", "status": float64(1), "gmtCreate": float64(1758422400000),
				"userProfileModel": map[string]any{"nick": "Ann"},
			},
			map[string]any{"openDingTalkId": "open-sparse", "userProfileModel": nil, "alias": "", "remark": "", "status": "x", "gmtCreate": "y"},
		}},
	}, friendOperationList)
	if err != nil || len(friends) != 2 {
		t.Fatalf("projection = %#v err:%v", friends, err)
	}
	if friends[0]["nick"] != "Ann" || friends[0]["alias"] != "Annie" || friends[0]["remark"] != "school" || friends[0]["status"] != int64(1) || friends[0]["gmtCreate"] != int64(1758422400000) {
		t.Fatalf("full row drift: %#v", friends[0])
	}
	if _, exists := friends[1]["nick"]; exists {
		t.Fatalf("nil profile leaked nick: %#v", friends[1])
	}
	for _, key := range []string{"alias", "remark", "status", "gmtCreate"} {
		if _, exists := friends[1][key]; exists {
			t.Fatalf("blank or invalid %s leaked: %#v", key, friends[1])
		}
	}

	requests, cursor, hasMore, pendingCount, err := strictFriendRequestList(map[string]any{
		"success": true,
		"result":  map[string]any{"cursor": float64(0), "hasMore": false},
	}, friendOperationRequestList)
	if err != nil || len(requests) != 0 || cursor != 0 || hasMore || pendingCount != 0 {
		t.Fatalf("absent request page = %#v cursor:%d hasMore:%v pending:%d err:%v", requests, cursor, hasMore, pendingCount, err)
	}

	requests, _, _, _, err = strictFriendRequestList(map[string]any{
		"success": true,
		"result": map[string]any{"cursor": float64(6), "hasMore": true, "pendingCount": "x", "friendList": []any{
			map[string]any{
				"openDingTalkId": "req-full", "remark": "hi", "status": float64(2), "modifyAt": float64(1758422500000), "isRead": true,
				"userProfileModel": map[string]any{"nick": "Ann"},
			},
			map[string]any{"openDingTalkId": "req-sparse", "userProfileModel": nil, "remark": "", "status": "x", "modifyAt": "y", "isRead": "x"},
		}},
	}, friendOperationRequestList)
	if err != nil || len(requests) != 2 {
		t.Fatalf("request projection = %#v err:%v", requests, err)
	}
	if requests[0]["nick"] != "Ann" || requests[0]["remark"] != "hi" || requests[0]["status"] != int64(2) || requests[0]["modifyAt"] != int64(1758422500000) || requests[0]["isRead"] != true {
		t.Fatalf("full request row drift: %#v", requests[0])
	}
	if _, exists := requests[1]["nick"]; exists {
		t.Fatalf("nil profile leaked nick: %#v", requests[1])
	}
	for _, key := range []string{"remark", "status", "modifyAt", "isRead"} {
		if _, exists := requests[1][key]; exists {
			t.Fatalf("blank or invalid %s leaked: %#v", key, requests[1])
		}
	}
}
