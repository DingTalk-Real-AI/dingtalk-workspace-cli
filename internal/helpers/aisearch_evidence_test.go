package helpers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

type aisearchTestStringer struct{ value string }

func (s aisearchTestStringer) String() string { return s.value }

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

func TestAisearchEvidenceEdgeBranches(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))

	if got := resolveAisearchBehaviorMessages(context.Background(), "raw", map[string]any{"searchTypes": []string{"im"}}, now); got != "raw" {
		t.Fatalf("non-map behavior result = %#v", got)
	}
	if got := resolveAisearchBehaviorMessages(context.Background(), map[string]any{}, map[string]any{"searchTypes": []string{"document"}, "direction": "甲->我"}, now); got == nil {
		t.Fatal("non-IM behavior result is nil")
	}
	if got := resolveAisearchBehaviorMessages(context.Background(), map[string]any{}, map[string]any{"searchTypes": []string{"im"}, "direction": "我->甲"}, now); got == nil {
		t.Fatal("outbound behavior result is nil")
	}

	t.Run("person lookup error and ambiguity", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{err: errors.New("person failed")}}}
		installScriptedCaller(t, caller)
		root := map[string]any{}
		if got := resolveAisearchBehaviorMessages(context.Background(), root, map[string]any{"searchTypes": []string{"im"}, "direction": "甲->我"}, now); got == nil {
			t.Fatal("lookup error result is nil")
		}
	})
	t.Run("ambiguous person", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":[]}`}}}
		installScriptedCaller(t, caller)
		root := map[string]any{}
		resolveAisearchBehaviorMessages(context.Background(), root, map[string]any{"searchTypes": []string{"im"}, "direction": "甲->我"}, now)
	})
	t.Run("message error", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{err: errors.New("chat failed")}}}
		installScriptedCaller(t, caller)
		out := resolveAisearchBehaviorMessages(context.Background(), map[string]any{}, map[string]any{"searchTypes": []string{"im"}, "direction": "某人->我"}, now).(map[string]any)
		if nestedValue(out, "resolvedMessageEvidence", "status") != "failed" {
			t.Fatalf("out=%#v", out)
		}
	})
	t.Run("word dedupe and truncation", func(t *testing.T) {
		rows := make([]string, 0, 24)
		for i := 0; i < 22; i++ {
			rows = append(rows, fmt.Sprintf(`{"sender":"甲","text":"f%d.docx","time":"2026-09-20 10:00:00"}`, i))
		}
		rows = append(rows, `{"sender":"甲","text":"f0.docx","time":"2026-09-20 10:00:00"}`, `{"sender":"甲","text":"","time":"2026-09-20 10:00:00"}`)
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":{"messages":[` + strings.Join(rows, ",") + `]}}`}}}
		installScriptedCaller(t, caller)
		out := resolveAisearchBehaviorMessages(context.Background(), map[string]any{}, map[string]any{"queries": []string{"Word 文件"}, "searchTypes": []string{"im"}, "direction": "某人->我", "timeRange": "今天"}, now).(map[string]any)
		if nestedValue(out, "resolvedMessageEvidence", "messageCount") != 20 || nestedValue(out, "resolvedMessageEvidence", "truncatedTo20") != true {
			t.Fatalf("out=%#v", out)
		}
	})

	filtered := filterAisearchMessagesByTime([]map[string]any{{"time": "2026-09-19 10:00:00"}, {"time": "bad"}}, now.Add(-time.Hour), now)
	if len(filtered) != 1 {
		t.Fatalf("filtered=%#v", filtered)
	}
	if got := dedupeAisearchMessagesByText([]map[string]any{{"text": "A"}, {"text": "a"}, {"text": ""}}); len(got) != 1 {
		t.Fatalf("dedupe=%#v", got)
	}
	if aliases, ok := uniquePersonAliases(map[string]any{"result": []any{map[string]any{"title": "甲"}, map[string]any{"title": "乙"}}}); ok || aliases != nil {
		t.Fatalf("aliases=%#v ok=%v", aliases, ok)
	}

	messageTree := map[string]any{"messages": []any{map[string]any{"sender": "甲", "text": "主题", "children": []any{map[string]any{"sender": "甲", "text": "主题"}}}}}
	if got := collectAisearchMessages(messageTree, nil, []string{"主题"}); len(got) != 1 {
		t.Fatalf("messages=%#v", got)
	}
}

func TestAisearchExactCandidateEdgeBranches(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	if got := resolveAisearchExactCandidates(context.Background(), "raw", nil, now); got != "raw" {
		t.Fatalf("got=%#v", got)
	}

	tests := []struct {
		name, source string
		item         map[string]any
		response     scriptedToolStep
	}{
		{"document adoc", "document", map[string]any{"sourceType": "document", "title": "标题", "nodeId": "n1", "meta": map[string]any{"doc_type": "adoc"}}, scriptedToolStep{text: `{"result":{"body":"正文"}}`}},
		{"document file", "document", map[string]any{"sourceType": "document", "title": "标题", "nodeId": "n2"}, scriptedToolStep{text: `{"result":{"name":"标题"}}`}},
		{"detail failure", "todo", map[string]any{"sourceType": "todo", "title": "标题", "taskId": "t1"}, scriptedToolStep{err: errors.New("read failed")}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{steps: []scriptedToolStep{tc.response}}
			installScriptedCaller(t, caller)
			out := resolveAisearchExactCandidates(context.Background(), map[string]any{"result": []any{tc.item}}, map[string]any{"queries": []string{"标题"}, "searchTypes": []string{tc.source}}, now).(map[string]any)
			if out["result"] == nil {
				t.Fatalf("out=%#v", out)
			}
		})
	}

	minute := map[string]any{"sourceType": "minute", "title": "标题", "meta": map[string]any{"summary": "摘要"}}
	out := resolveAisearchExactCandidates(context.Background(), map[string]any{"result": []any{minute}}, map[string]any{"queries": []string{"标题"}, "searchTypes": []string{"minute"}}, now).(map[string]any)
	if nestedValue(out["result"].([]any)[0].(map[string]any), "resolvedDetail", "status") != "resolved_from_search_payload" {
		t.Fatalf("out=%#v", out)
	}

	for _, item := range []map[string]any{
		{"sourceType": "document", "title": "标题"}, {"sourceType": "todo", "title": "标题"}, {"sourceType": "calendar", "title": "标题"}, {"sourceType": "unknown", "title": "标题"},
	} {
		resolveAisearchExactCandidates(context.Background(), map[string]any{"result": []any{item}}, map[string]any{"queries": []string{"标题"}, "searchTypes": []string{item["sourceType"].(string)}}, now)
	}
	resolveAisearchExactCandidates(context.Background(), map[string]any{"result": []any{map[string]any{"sourceType": "todo", "title": "标题", "taskId": "1"}, map[string]any{"sourceType": "todo", "title": "标题", "taskId": "2"}}}, map[string]any{"queries": []string{"标题"}, "searchTypes": []string{"todo"}}, now)

	t.Run("calendar existing and fallback error", func(t *testing.T) {
		items := []any{map[string]any{"sourceType": "calendar", "title": "标题"}}
		if got := appendExactCalendarCandidate(context.Background(), items, map[string]any{"queries": []string{"标题"}, "timeRange": "今天"}, now); len(got) != 1 {
			t.Fatalf("got=%#v", got)
		}
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{err: errors.New("calendar failed")}}}
		installScriptedCaller(t, caller)
		if got := appendExactCalendarCandidate(context.Background(), nil, map[string]any{"queries": []string{"标题"}, "searchTypes": []string{"calendar"}, "timeRange": "今天"}, now); len(got) != 0 {
			t.Fatalf("got=%#v", got)
		}
	})
}

func TestAisearchPersonAndValueEdgeBranches(t *testing.T) {
	base := map[string]any{"keyword": "甲的直属上级", "dimension": []string{"supervisor"}}
	for _, step := range []scriptedToolStep{{err: errors.New("person failed")}, {text: `{"result":[]}`}, {text: `{"result":[{"title":"甲"}]}`}} {
		t.Run(fmt.Sprint(step.err, step.text), func(t *testing.T) {
			caller := &scriptedToolCaller{steps: []scriptedToolStep{step}}
			installScriptedCaller(t, caller)
			resolveAisearchPersonRelation(context.Background(), map[string]any{}, base)
		})
	}
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":[{"title":"甲","meta":{"supervisor":"乙"}}]}`}}}
	installScriptedCaller(t, caller)
	if got := resolveAisearchPersonRelation(context.Background(), "raw", base); got != "raw" {
		t.Fatalf("got=%#v", got)
	}

	if got := enrichAisearchPersonResponse("raw", nil).(map[string]any)["result"]; got != "raw" {
		t.Fatalf("got=%#v", got)
	}
	enrichAisearchPersonResponse(map[string]any{"result": []any{"raw"}}, nil)
	enrichAisearchResponse("raw", nil, time.Now())
	enrichAisearchResponse(map[string]any{"result": []any{"raw"}}, nil, time.Now())

	items := []any{"raw", map[string]any{"sourceType": "doc", "_searchEvidence": map[string]any{"queryMatch": "text_evidence_present"}}, map[string]any{"sourceType": "doc", "_searchEvidence": map[string]any{"queryMatch": "no_text_evidence"}}}
	compactAisearchItems(items)

	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	for _, tc := range []struct{ date, rng, status string }{{"x", "", "not_requested"}, {"x", "今天", "unparseable_result_time"}, {"2026-09-20 10:00:00", "以后", "range_not_machine_verifiable"}} {
		_, status := aisearchTimeRangeStatus(tc.date, tc.rng, now)
		if status != tc.status {
			t.Fatalf("%v status=%s", tc, status)
		}
	}
	for _, raw := range []string{"今天", "昨天", "本周", "本月", "最近七天", "未来7天", "近3天", "近三天", "近零天"} {
		aisearchNaturalRange(raw, now)
	}
	if got := splitAisearchQueryTerms("甲 乙"); len(got) != 2 {
		t.Fatalf("got=%#v", got)
	}
	for _, raw := range []string{"3", "三", "零"} {
		chineseOrArabicNumber(raw)
	}
	for _, raw := range []string{"", "bad%zz?taskId=fallback", "https://x.invalid/?taskId=ok", "https://x.invalid/?x=1&taskId=regex%zz", "https://x.invalid/?x=1"} {
		taskIDFromURL(raw)
	}
	for _, value := range []any{[]string{"a"}, []any{"a", 1, nil}, "", "a", true} {
		stringSlice(value)
	}
	for _, value := range []any{"a", float64(1.5), 1, int64(2), aisearchTestStringer{"s"}, true} {
		aisearchStringValue(value)
	}
	if matchesExactAisearchTitle("", []string{"a"}) {
		t.Fatal("empty title matched")
	}
	if aisearchSourceType(map[string]any{"sourceType": "doc"}) != "document" {
		t.Fatal("doc source not normalized")
	}
	buildAisearchCandidateEvidence(map[string]any{"sourceType": "im", "title": "风险复盘", "snippet": "风险", "resolvedDetail": map[string]any{"status": "resolved"}, "date": "bad"}, map[string]any{"queries": []string{"发版风险"}, "timeRange": "今天"}, now)
}
