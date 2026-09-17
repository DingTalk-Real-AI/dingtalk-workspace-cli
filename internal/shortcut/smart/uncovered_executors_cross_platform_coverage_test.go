// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package smart

import (
	"io"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/spf13/cobra"
)

func executeUncoveredShortcut(t *testing.T, declaration shortcut.Shortcut, caller *smartCoverageCaller, setup func(*cobra.Command)) error {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	declaration.OutputRollout = output.RolloutLegacyOnly
	cmd := &cobra.Command{Use: declaration.Command}
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if setup != nil {
		setup(cmd)
	}
	return declaration.Execute(shortcut.RuntimeContextForTest(cmd, declaration))
}

func TestCrossPlatformCoverageUncoveredSmartExecutors(t *testing.T) {
	t.Run("find-room", func(t *testing.T) {
		caller := &smartCoverageCaller{responses: map[string][]string{
			"calendar/query_available_meeting_room": {`{"result":[{"roomId":"r1","roomName":"A","capacity":8}]}`},
		}}
		if err := executeUncoveredShortcut(t, FindRoom, caller, func(cmd *cobra.Command) {
			cmd.Flags().String("start", "", "")
			cmd.Flags().String("end", "", "")
			_ = cmd.Flags().Set("start", "2026-03-10T14:00:00+08:00")
			_ = cmd.Flags().Set("end", "2026-03-10T15:00:00+08:00")
		}); err != nil {
			t.Fatal(err)
		}
		if err := executeUncoveredShortcut(t, FindRoom, caller, func(cmd *cobra.Command) {
			cmd.Flags().String("start", "", "")
			cmd.Flags().String("end", "", "")
			_ = cmd.Flags().Set("start", "not-a-time")
			_ = cmd.Flags().Set("end", "2026-03-10T15:00:00+08:00")
		}); err == nil {
			t.Fatal("invalid start accepted")
		}
		if err := executeUncoveredShortcut(t, FindRoom, caller, func(cmd *cobra.Command) {
			cmd.Flags().String("start", "", "")
			cmd.Flags().String("end", "", "")
			_ = cmd.Flags().Set("start", "2026-03-10T15:00:00+08:00")
			_ = cmd.Flags().Set("end", "2026-03-10T14:00:00+08:00")
		}); err == nil {
			t.Fatal("reversed range accepted")
		}
		_ = findRoomExtractRooms(nil)
		_ = findRoomExtractRooms(map[string]any{"result": []any{"x", map[string]any{"id": "r"}}})
		_ = findRoomExtractRooms(map[string]any{"data": map[string]any{"rooms": []any{map[string]any{"name": "B", "capacity": "12"}}}})
		_ = findRoomCapacity(map[string]any{"seatCount": int64(3)})
		_ = findRoomCapacity(map[string]any{"seats": 4})
		_ = findRoomFirstString(map[string]any{"title": "T"}, "missing", "title")
	})

	t.Run("list-tables", func(t *testing.T) {
		caller := &smartCoverageCaller{responses: map[string][]string{
			"aitable/get_tables": {`{"tables":[{"tableId":"t1","tableName":"N"}]}`},
		}}
		if err := executeUncoveredShortcut(t, ListTables, caller, func(cmd *cobra.Command) {
			cmd.Flags().String("base", "", "")
			_ = cmd.Flags().Set("base", "B")
		}); err != nil {
			t.Fatal(err)
		}
		caller.responses["aitable/get_tables"] = []string{`{"ok":true}`}
		if err := executeUncoveredShortcut(t, ListTables, caller, func(cmd *cobra.Command) {
			cmd.Flags().String("base", "", "")
			_ = cmd.Flags().Set("base", "B")
		}); err != nil {
			t.Fatal(err)
		}
		_ = listTablesItems(nil)
		_ = listTablesItems(map[string]any{"result": map[string]any{"tables": []any{map[string]any{"id": 7, "name": "n"}}}})
		_ = listTablesString(int(1))
		_ = listTablesString(int64(2))
		_ = listTablesString(3.0)
		_ = listTablesID(map[string]any{"table_id": "x"})
		_ = listTablesName(map[string]any{"title": "y"})
	})

	t.Run("find-file", func(t *testing.T) {
		caller := &smartCoverageCaller{responses: map[string][]string{
			"drive/search_files": {`{"result":{"items":[{"name":"f","type":"file","dentryId":"d","fileSize":12}]}}`},
		}}
		if err := executeUncoveredShortcut(t, FindFile, caller, func(cmd *cobra.Command) {
			cmd.Flags().String("query", "", "")
			_ = cmd.Flags().Set("query", "季度")
		}); err != nil {
			t.Fatal(err)
		}
		_ = shortcutFindFileItems(map[string]any{"nodes": []any{map[string]any{"title": "a", "size": int64(2)}}})
		_ = shortcutFindFileItems(map[string]any{"other": []any{map[string]any{"id": "x"}}})
		_ = shortcutFindFileSize(map[string]any{"size": "3"})
		_ = shortcutFindFileStr(map[string]any{"name": "  "}, "name", "title")
	})

	t.Run("find-doc", func(t *testing.T) {
		caller := &smartCoverageCaller{responses: map[string][]string{
			"doc/search_documents": {`{"result":{"documents":[{"title":"T","url":"u","docType":"doc","token":"tok"}]}}`},
		}}
		if err := executeUncoveredShortcut(t, FindDoc, caller, func(cmd *cobra.Command) {
			cmd.Flags().String("query", "", "")
			cmd.Flags().Int("limit", 0, "")
			_ = cmd.Flags().Set("query", "合同")
			_ = cmd.Flags().Set("limit", "10")
		}); err != nil {
			t.Fatal(err)
		}
		caller.responses["doc/search_documents"] = []string{`{"result":{}}`}
		if err := executeUncoveredShortcut(t, FindDoc, caller, func(cmd *cobra.Command) {
			cmd.Flags().String("query", "", "")
			cmd.Flags().Int("limit", 0, "")
			_ = cmd.Flags().Set("query", "none")
		}); err == nil {
			t.Fatal("empty doc search accepted")
		}
		_ = shortcutFindDocItems(map[string]any{"hits": []any{map[string]any{"name": "n"}}})
	})

	t.Run("record-share-links", func(t *testing.T) {
		if err := executeUncoveredShortcut(t, RecordShareLinks, &smartCoverageCaller{}, func(cmd *cobra.Command) {
			cmd.Flags().String("base", "", "")
			cmd.Flags().String("table", "", "")
			cmd.Flags().StringSlice("record-ids", nil, "")
			cmd.Flags().String("view-id", "", "")
			_ = cmd.Flags().Set("base", "B")
			_ = cmd.Flags().Set("table", "T")
			_ = cmd.Flags().Set("record-ids", "")
		}); err == nil {
			t.Fatal("empty record ids accepted")
		}
		caller := &smartCoverageCaller{
			responses: map[string][]string{
				"aitable-helper/get_record_share_url": {
					`{"items":[{"recordId":"r1","shareUrl":"u1"}]}`,
					`{"data":{"items":[{"id":"r21","url":"u21"}]}}`,
				},
			},
			failAt: map[string]int{"aitable-helper/get_record_share_url": 2},
		}
		ids := make([]string, 0, 21)
		for i := 0; i < 21; i++ {
			ids = append(ids, "r"+string(rune('a'+i%26))+string(rune('0'+i%10)))
		}
		if err := executeUncoveredShortcut(t, RecordShareLinks, caller, func(cmd *cobra.Command) {
			cmd.Flags().String("base", "", "")
			cmd.Flags().String("table", "", "")
			cmd.Flags().StringSlice("record-ids", nil, "")
			cmd.Flags().String("view-id", "", "")
			_ = cmd.Flags().Set("base", "B")
			_ = cmd.Flags().Set("table", "T")
			_ = cmd.Flags().Set("view-id", "viw")
			for _, id := range ids {
				_ = cmd.Flags().Set("record-ids", id)
			}
		}); err != nil {
			t.Fatal(err)
		}
		_ = dedupStrings([]string{"", "a", "a", "b"})
		_ = recordShareItems(map[string]any{"items": []any{"x", map[string]any{"record_id": "r", "share_url": nil}}})
		_ = firstNonEmpty(map[string]any{"id": "x"}, "missing", "id")
		_ = firstAny(map[string]any{"url": nil}, "missing", "url")
	})

	t.Run("overdue", func(t *testing.T) {
		caller := &smartCoverageCaller{responses: map[string][]string{
			"todo/get_user_todos_in_current_org": {`{"success":true,"result":{"todoCards":[{"taskId":"t1","subject":"old","dueTime":1},{"taskId":"t2","subject":"done","dueTime":1,"isDone":true},{"taskId":"t3","subject":"future","dueTime":9999999999999}],"hasMore":false}}`},
		}}
		if err := executeUncoveredShortcut(t, Overdue, caller, nil); err != nil {
			t.Fatal(err)
		}
		_ = shortcutOverdueIsDone(map[string]any{"isDone": "true"})
		_ = shortcutOverdueIsDone(map[string]any{"done": true})
		_ = shortcutOverdueIsDone(map[string]any{"status": "FINISHED"})
		_, _ = shortcutOverdueDueTime(map[string]any{"dueTime": int64(9)})
		_, _ = shortcutOverdueDueTime(map[string]any{"gmtDue": "12"})
		_, _ = shortcutOverdueDueTime(map[string]any{"dueTime": "-3"})
	})

	t.Run("my-free", func(t *testing.T) {
		caller := &smartCoverageCaller{responses: map[string][]string{
			"contact/get_current_user_profile": {`{"result":[{"orgEmployeeModel":{"userId":"u1"}}]}`},
			"calendar/query_busy_status":       {`{"success":true,"result":[{"scheduleItems":[{"start":{"dateTime":"2026-07-10T09:00:00+08:00"},"end":{"dateTime":"2026-07-10T10:00:00+08:00"}}]}]}`},
		}}
		if err := executeUncoveredShortcut(t, MyFree, caller, func(cmd *cobra.Command) {
			cmd.Flags().String("start", "", "")
			cmd.Flags().String("end", "", "")
			_ = cmd.Flags().Set("start", "2026-07-10T09:00:00+08:00")
			_ = cmd.Flags().Set("end", "2026-07-10T18:00:00+08:00")
		}); err != nil {
			t.Fatal(err)
		}
		if err := executeUncoveredShortcut(t, MyFree, caller, func(cmd *cobra.Command) {
			cmd.Flags().String("start", "", "")
			cmd.Flags().String("end", "", "")
			_ = cmd.Flags().Set("start", "2026-07-10T18:00:00+08:00")
			_ = cmd.Flags().Set("end", "2026-07-10T09:00:00+08:00")
		}); err == nil {
			t.Fatal("reversed my-free range accepted")
		}
	})

	t.Run("unread-chats", func(t *testing.T) {
		caller := &smartCoverageCaller{responses: map[string][]string{
			"chat/unread_message_conversation_list": {`{"result":[{"name":"c","unreadCount":2,"openConversationId":"cid"}]}`},
		}}
		if err := executeUncoveredShortcut(t, UnreadChats, caller, func(cmd *cobra.Command) {
			cmd.Flags().Int("count", 0, "")
			cmd.Flags().Bool("exclude-muted", false, "")
			_ = cmd.Flags().Set("count", "20")
			_ = cmd.Flags().Set("exclude-muted", "true")
		}); err != nil {
			t.Fatal(err)
		}
		caller.responses["chat/unread_message_conversation_list"] = []string{`{"ok":true}`}
		if err := executeUncoveredShortcut(t, UnreadChats, caller, nil); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("related-tasks", func(t *testing.T) {
		caller := &smartCoverageCaller{responses: map[string][]string{
			"todo/get_user_todos_in_current_org": {`{"success":true,"result":{"todoCards":[{"taskId":"t1","subject":"a"},{"taskId":"t1","subject":"dup"},{"taskId":"t2","subject":"b"}],"hasMore":false}}`},
		}}
		if err := executeUncoveredShortcut(t, RelatedTasks, caller, func(cmd *cobra.Command) {
			cmd.Flags().String("role-types", "", "")
			cmd.Flags().String("status", "", "")
			_ = cmd.Flags().Set("role-types", "creator,executor")
			_ = cmd.Flags().Set("status", "TODO")
		}); err != nil {
			t.Fatal(err)
		}
		if err := executeUncoveredShortcut(t, RelatedTasks, caller, func(cmd *cobra.Command) {
			cmd.Flags().String("role-types", "", "")
			cmd.Flags().String("status", "", "")
			_ = cmd.Flags().Set("role-types", "nope")
		}); err == nil {
			t.Fatal("invalid role-type accepted")
		}
	})

	t.Run("whoami-helpers", func(t *testing.T) {
		_ = whoamiProject(map[string]any{"result": []any{map[string]any{"orgEmployeeModel": map[string]any{
			"orgUserName": "n", "userId": "u", "orgUserMobile": "m", "orgAuthEmail": "e", "orgName": "o",
			"depts": []any{map[string]any{"deptName": "d"}},
		}}}})
		_ = whoamiProject(map[string]any{"orgEmployeeModel": map[string]any{"name": "n"}})
		_ = whoamiProject(map[string]any{"result": map[string]any{"orgEmployeeModel": map[string]any{"nick": "n"}}})
		_ = whoamiProject(map[string]any{"result": map[string]any{"userName": "n"}})
		_ = whoamiProject(map[string]any{"x": 1})
		_ = whoamiEmployeeModel(nil)
		_ = whoamiEmployeeModel(map[string]any{"result": []any{"bad"}})
		_ = whoamiFirstDept(map[string]any{"depts": []any{"bad", map[string]any{}}})
		_ = whoamiStr(map[string]any{"name": "n"}, "missing", "name")
	})
}
