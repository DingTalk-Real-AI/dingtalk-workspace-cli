package helpers

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestEnrichAisearchResponseExposesSourceAndCandidateEvidence(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	data := map[string]any{
		"success": true,
		"result": []any{
			map[string]any{
				"sourceType": "document",
				"title":      "客户续约作战手册",
				"snippet":    "客户续约流程",
				"nodeId":     "doc-1",
			},
			map[string]any{
				"sourceType": "todo",
				"title":      "项目发版确认",
				"snippet":    "状态: 未完成",
				"url":        "https://example.invalid/?taskId=todo-1",
			},
		},
	}
	args := map[string]any{
		"queries":     []string{"客户续约"},
		"searchTypes": []string{"document", "im", "todo"},
	}

	out := enrichAisearchResponse(data, args, now).(map[string]any)
	items := out["result"].([]any)
	docEvidence := items[0].(map[string]any)["_searchEvidence"].(map[string]any)
	if docEvidence["queryMatch"] != "text_evidence_present" {
		t.Fatalf("document queryMatch = %v", docEvidence["queryMatch"])
	}
	todoEvidence := items[1].(map[string]any)["_searchEvidence"].(map[string]any)
	if todoEvidence["queryMatch"] != "no_text_evidence" {
		t.Fatalf("todo queryMatch = %v", todoEvidence["queryMatch"])
	}
	refs := todoEvidence["stableRefs"].([]map[string]string)
	if len(refs) != 1 || refs[0]["domain"] != "todo.taskId" || refs[0]["value"] != "todo-1" {
		t.Fatalf("todo stableRefs = %#v", refs)
	}

	searchEvidence := out["searchEvidence"].(map[string]any)
	sources := searchEvidence["sources"].([]map[string]any)
	if len(sources) != 3 || sources[1]["sourceType"] != "im" || sources[1]["status"] != "no_returned_items" {
		t.Fatalf("sources = %#v", sources)
	}
	coverage := searchEvidence["coverage"].(map[string]any)
	if coverage["status"] != "unknown" || coverage["complete"] != false {
		t.Fatalf("coverage = %#v", coverage)
	}
}

func TestResolveAisearchExactTodoCandidateReadsDetailOnce(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"success":true,"result":{"todoDetailModel":{"taskId":"todo-1","subject":"项目发版确认"}}}`}}}
	installScriptedCaller(t, caller)
	data := map[string]any{"result": []any{map[string]any{
		"sourceType": "todo", "title": "项目发版确认",
		"url": "https://example.invalid/?taskId=todo-1",
	}}}
	out := resolveAisearchExactCandidates(context.Background(), data, map[string]any{
		"queries": []string{"项目发版确认"}, "searchTypes": []string{"todo", "im"},
	}, time.Now()).(map[string]any)
	item := out["result"].([]any)[0].(map[string]any)
	detail := item["resolvedDetail"].(map[string]any)
	if detail["status"] != "resolved" || detail["source"] != "todo/get_todo_detail" {
		t.Fatalf("resolvedDetail = %#v", detail)
	}
	if caller.calls != 1 || caller.server != "todo" || caller.tool != "get_todo_detail" || caller.args["taskId"] != "todo-1" {
		t.Fatalf("call = %d %s/%s %#v", caller.calls, caller.server, caller.tool, caller.args)
	}
}

func TestResolveAisearchCalendarFallbackKeepsOriginalRange(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"success":true,"result":{"events":[{"id":"event-1","summary":"项目启动评审","description":"确认目标和风险","start":{"dateTime":"2026-09-22T14:00:00+08:00"},"end":{"dateTime":"2026-09-22T15:00:00+08:00"}}]}}`}}}
	installScriptedCaller(t, caller)
	data := map[string]any{"result": []any{}}
	out := resolveAisearchExactCandidates(context.Background(), data, map[string]any{
		"queries": []string{"项目启动评审"}, "searchTypes": []string{"calendar", "im"}, "timeRange": "未来七天",
	}, now).(map[string]any)
	items := out["result"].([]any)
	if len(items) != 1 {
		t.Fatalf("items = %#v", items)
	}
	item := items[0].(map[string]any)
	if item["sourceType"] != "calendar" || nestedValue(item, "meta", "nativeFallback") != true {
		t.Fatalf("calendar item = %#v", item)
	}
	detail := item["resolvedDetail"].(map[string]any)
	if detail["status"] != "resolved" || caller.tool != "list_calendar_events" {
		t.Fatalf("detail/call = %#v %s", detail, caller.tool)
	}
	wantStart := now.UnixMilli()
	wantEnd := time.Date(2026, 9, 27, 23, 59, 59, 999999999, now.Location()).UnixMilli()
	if caller.args["startTime"] != wantStart || caller.args["endTime"] != wantEnd {
		t.Fatalf("calendar args = %#v", caller.args)
	}
}

func TestResolveAisearchBehaviorMessagesUsesReturnedAliasesAndOriginalDay(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"result":[{"title":"瑞达","meta":{"name":"符咏畅","nick":"瑞达"}}]}`},
		{text: `{"result":{"conversationMessagesList":[{"messages":[{"messageId":"m1","sender":"瑞达","text":"今天项目同步","time":"2026-09-20 10:00:00"},{"messageId":"m2","sender":"其他人","text":"无关","time":"2026-09-20 10:01:00"}]}]}}`},
	}}
	installScriptedCaller(t, caller)
	out := resolveAisearchBehaviorMessages(context.Background(), map[string]any{"result": []any{}}, map[string]any{
		"queries": []string{}, "searchTypes": []string{"im"}, "direction": "瑞达->我", "timeRange": "今天",
	}, now).(map[string]any)
	resolved := out["resolvedMessageEvidence"].(map[string]any)
	if resolved["status"] != "resolved" || resolved["messageCount"] != 1 {
		t.Fatalf("resolvedMessageEvidence = %#v", resolved)
	}
	if caller.calls != 2 || caller.toolLog[0] != "enterprise_person_search" || caller.toolLog[1] != "search_messages_by_time_range" {
		t.Fatalf("calls = %#v/%#v", caller.serverLog, caller.toolLog)
	}
	if caller.argsLog[1]["startTime"] != time.Date(2026, 9, 20, 0, 0, 0, 0, now.Location()).UnixMilli() {
		t.Fatalf("chat args = %#v", caller.argsLog[1])
	}
}

func TestResolveAisearchBehaviorMessagesSupportsUnknownSenderDiscovery(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":{"messages":[{"messageId":"m1","sender":"瑞达","text":"DWS 评测文件","time":"2026-09-19 10:00:00"},{"messageId":"m2","sender":"南润","text":"其他主题","time":"2026-09-19 11:00:00"}]}}`}}}
	installScriptedCaller(t, caller)
	out := resolveAisearchBehaviorMessages(context.Background(), map[string]any{"result": []any{}}, map[string]any{
		"queries": []string{"DWS"}, "searchTypes": []string{"im"}, "direction": "某人->我",
	}, now).(map[string]any)
	resolved := out["resolvedMessageEvidence"].(map[string]any)
	if resolved["messageCount"] != 1 || nestedValue(resolved, "identity", "match") != "any_returned_sender" {
		t.Fatalf("resolvedMessageEvidence = %#v", resolved)
	}
	if caller.calls != 1 || caller.tool != "search_messages_by_keyword" || caller.args["keyword"] != "DWS" {
		t.Fatalf("calls = %d %s", caller.calls, caller.tool)
	}
}

func TestCollectAisearchMessagesFindsWordAttachmentByPersonAlias(t *testing.T) {
	data := map[string]any{"result": map[string]any{"messages": []any{
		map[string]any{"messageId": "m1", "sender": "南润DingTalk:)", "text": "[文件] literature.docx", "resourceRefs": []any{map[string]any{"name": "literature.docx"}}},
		map[string]any{"messageId": "m2", "sender": "其他人", "text": "[文件] other.docx"},
	}}}
	got := collectAisearchMessages(data, []string{"陈邦杰", "南润"}, []string{"Word 文件"})
	if len(got) != 1 || got[0]["messageId"] != "m1" {
		t.Fatalf("messages = %#v", got)
	}
}

func TestEnrichAisearchResponseFlagsOutOfRangeIMAndIdentityDomains(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	data := map[string]any{
		"success": true,
		"result": []any{map[string]any{
			"sourceType": "im",
			"date":       "2026-06-29 11:59:06",
			"actor":      "2149537653",
			"snippet":    "[发送者:符咏畅] 项目同步",
			"meta": map[string]any{
				"creatorId":          float64(2149537653),
				"openConversationId": "cid-1",
			},
		}},
	}
	args := map[string]any{
		"queries":      []string{"项目"},
		"searchTypes":  []string{"im"},
		"timeRange":    "最近七天",
		"behaviorType": "receive",
	}

	out := enrichAisearchResponse(data, args, now).(map[string]any)
	if items := out["result"].([]any); len(items) != 0 {
		t.Fatalf("out-of-range result was not compacted: %#v", items)
	}
	delivery := out["searchEvidence"].(map[string]any)["delivery"].(map[string]any)
	if delivery["omittedOutOfRangeCount"] != 1 || delivery["doNotRetryOrExpand"] != true {
		t.Fatalf("delivery = %#v", delivery)
	}
}

func TestEnrichAisearchResponseCompactsUnverifiedLongTailPerSource(t *testing.T) {
	items := []any{map[string]any{"sourceType": "document", "title": "客户续约手册", "snippet": "客户续约流程"}}
	for i := 0; i < 8; i++ {
		items = append(items, map[string]any{"sourceType": "todo", "title": fmt.Sprintf("无关待办%d", i)})
	}
	out := enrichAisearchResponse(map[string]any{"result": items}, map[string]any{
		"queries": []string{"客户续约"}, "searchTypes": []string{"document", "todo"},
	}, time.Now()).(map[string]any)
	if got := len(out["result"].([]any)); got != 4 {
		t.Fatalf("retained result count = %d, want 4", got)
	}
	delivery := out["searchEvidence"].(map[string]any)["delivery"].(map[string]any)
	if delivery["omittedUnverifiedCandidateCount"] != 5 {
		t.Fatalf("delivery = %#v", delivery)
	}
}

func TestEnrichAisearchResponsePrioritizesResolvedMessages(t *testing.T) {
	data := map[string]any{
		"result": []any{
			map[string]any{"sourceType": "im", "title": "会话一", "snippet": "混合会话片段"},
			map[string]any{"sourceType": "im", "title": "会话二", "snippet": "另一个混合会话片段"},
		},
		"resolvedMessageEvidence": map[string]any{
			"status": "resolved", "messages": []any{map[string]any{"sender": "瑞达", "text": "已核验消息"}},
		},
	}
	out := enrichAisearchResponse(data, map[string]any{
		"queries": []string{}, "searchTypes": []string{"im"}, "direction": "瑞达->我",
	}, time.Now()).(map[string]any)
	if got := len(out["result"].([]any)); got != 0 {
		t.Fatalf("semantic candidates retained = %d", got)
	}
	delivery := out["searchEvidence"].(map[string]any)["delivery"].(map[string]any)
	if delivery["action"] != "answer_from_resolved_messages" || delivery["omittedSearchCandidateCount"] != 2 {
		t.Fatalf("delivery = %#v", delivery)
	}
}

func TestEnrichAisearchResponseTruncatesVerboseSnippetAfterEvidenceExtraction(t *testing.T) {
	long := strings.Repeat("安全培训", 500)
	out := enrichAisearchResponse(map[string]any{"result": []any{map[string]any{
		"sourceType": "document", "title": "培训资料", "snippet": long,
	}}}, map[string]any{"queries": []string{"安全培训"}, "searchTypes": []string{"document"}}, time.Now()).(map[string]any)
	item := out["result"].([]any)[0].(map[string]any)
	if len([]rune(item["snippet"].(string))) != 1601 {
		t.Fatalf("snippet length = %d", len([]rune(item["snippet"].(string))))
	}
	evidence := item["_searchEvidence"].(map[string]any)
	if evidence["queryMatch"] != "text_evidence_present" || evidence["snippetTruncated"] != true {
		t.Fatalf("evidence = %#v", evidence)
	}
}

func TestSplitAisearchQueryTermsKeepsCompoundIntentPhrase(t *testing.T) {
	terms := splitAisearchQueryTerms("新员工安全培训")
	found := false
	for _, term := range terms {
		if term == "安全培训" {
			found = true
		}
	}
	if !found {
		t.Fatalf("terms = %#v", terms)
	}
}

func TestEnrichAisearchPersonResponseExposesAliasesAndIdentityDomain(t *testing.T) {
	data := map[string]any{
		"success": true,
		"result": []any{map[string]any{
			"actor":  "2149537653",
			"author": "陈邦杰",
			"title":  "南润",
			"userId": "489149",
			"meta": map[string]any{
				"name": "陈邦杰",
				"nick": "南润",
			},
		}},
	}
	out := enrichAisearchPersonResponse(data, map[string]any{
		"keyword": "陈邦杰", "dimension": []string{"name"},
	}).(map[string]any)
	evidence := out["result"].([]any)[0].(map[string]any)["_searchEvidence"].(map[string]any)
	aliases := evidence["aliases"].([]string)
	if len(aliases) != 2 || aliases[0] != "陈邦杰" || aliases[1] != "南润" {
		t.Fatalf("aliases = %#v", aliases)
	}
	refs := evidence["identityRefs"].([]map[string]string)
	foundContact := false
	for _, ref := range refs {
		if ref["domain"] == "contact.userId" && ref["value"] == "489149" {
			foundContact = true
		}
	}
	if !foundContact {
		t.Fatalf("identityRefs = %#v", refs)
	}
	search := out["searchEvidence"].(map[string]any)
	if search["returnedCandidateCount"] != 1 {
		t.Fatalf("searchEvidence = %#v", search)
	}
}

func TestEnrichAisearchPersonResponseMarksExactAliasContainment(t *testing.T) {
	out := enrichAisearchPersonResponse(map[string]any{"result": []any{
		map[string]any{"title": "成都-刘柏良小助手"},
		map[string]any{"title": "小柏同学"},
	}}, map[string]any{"keyword": "小柏", "dimension": []string{"name"}}).(map[string]any)
	items := out["result"].([]any)
	first := items[0].(map[string]any)
	second := items[1].(map[string]any)
	if nestedValue(first, "_searchEvidence", "exactAliasContainsQuery") != false || nestedValue(second, "_searchEvidence", "exactAliasContainsQuery") != true {
		t.Fatalf("items = %#v", items)
	}
	if out["searchEvidence"].(map[string]any)["exactAliasContainsCount"] != 1 {
		t.Fatalf("searchEvidence = %#v", out["searchEvidence"])
	}
}

func TestResolveAisearchPersonRelationReadsDirectSupervisorInSameCommand(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":[{"title":"瑞达","meta":{"name":"符咏畅","supervisor":"李主管"}}]}`}}}
	installScriptedCaller(t, caller)
	out := resolveAisearchPersonRelation(context.Background(), map[string]any{"result": []any{}}, map[string]any{
		"keyword": "符咏畅的直属上级", "dimension": []string{"supervisor"},
	}).(map[string]any)
	resolved := out["resolvedRelation"].(map[string]any)
	if resolved["relation"] != "direct_supervisor" || resolved["supervisor"] != "李主管" {
		t.Fatalf("resolvedRelation = %#v", resolved)
	}
}

func TestAisearchNaturalRangeCommonChineseRanges(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	tests := []struct {
		raw       string
		wantStart string
		wantEnd   string
	}{
		{"最近一周", "2026-09-14 00:00:00", "2026-09-20 12:00:00"},
		{"这个月", "2026-09-01 00:00:00", "2026-09-20 12:00:00"},
		{"未来七天", "2026-09-20 12:00:00", "2026-09-27 23:59:59"},
	}
	for _, tc := range tests {
		start, end, ok := aisearchNaturalRange(tc.raw, now)
		if !ok || start.Format("2006-01-02 15:04:05") != tc.wantStart || end.Format("2006-01-02 15:04:05") != tc.wantEnd {
			t.Fatalf("%s = %v %s..%s", tc.raw, ok, start, end)
		}
	}
}
