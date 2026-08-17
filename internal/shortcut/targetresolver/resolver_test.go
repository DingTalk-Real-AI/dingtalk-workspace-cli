// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package targetresolver

import (
	stderrors "errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/profilectx"
)

type resolverReaderFunc func(product, tool string, params map[string]any) (map[string]any, error)

func (f resolverReaderFunc) CallMCPData(product, tool string, params map[string]any) (map[string]any, error) {
	return f(product, tool, params)
}

type chatResolutionReader struct {
	responses []map[string]any
	calls     []map[string]any
}

func (r *chatResolutionReader) CallMCPData(product, tool string, params map[string]any) (map[string]any, error) {
	r.calls = append(r.calls, params)
	if product != "im" || tool != "search_groups" {
		return nil, stderrors.New("unexpected resolver tool")
	}
	if len(r.calls) > len(r.responses) {
		return nil, stderrors.New("unexpected resolver page")
	}
	return r.responses[len(r.calls)-1], nil
}

func TestCrossPlatformCoverageExtractUsersKeepsUsableExternalContacts(t *testing.T) {
	users := ExtractUsers(map[string]any{
		"result": []any{
			map[string]any{"userId": "u1", "openDingTalkId": "D1", "name": "张三"},
			map[string]any{"openDingtalkId": "D2", "nick": "外部张三"},
			map[string]any{"name": "无 ID"},
			"garbage",
		},
	})
	if len(users) != 2 {
		t.Fatalf("users = %#v", users)
	}
	if users[1].OpenDingTalkID != "D2" || users[1].Name != "外部张三" {
		t.Fatalf("external user = %#v", users[1])
	}
}

func TestCrossPlatformCoverageExtractUsersAcceptsEnterprisePersonMetadata(t *testing.T) {
	users := ExtractUsers(map[string]any{
		"result": []any{
			map[string]any{
				"meta": map[string]any{
					"staffId":        "u1",
					"openDingTalkId": "D1",
					"name":           "柏荣",
				},
			},
			map[string]any{
				"userId":         "u2",
				"openDingTalkId": "D2",
				"title":          "展示名",
			},
		},
	})
	if !reflect.DeepEqual(users, []User{
		{UserID: "u1", OpenDingTalkID: "D1", Name: "柏荣"},
		{UserID: "u2", OpenDingTalkID: "D2", Name: "展示名"},
	}) {
		t.Fatalf("enterprise users = %#v", users)
	}
}

func TestCrossPlatformCoverageResolveEnterpriseUserUsesCalibratedNameSearch(t *testing.T) {
	reader := resolverReaderFunc(func(product, tool string, params map[string]any) (map[string]any, error) {
		if product != "aisearch" || tool != "enterprise_person_search" {
			t.Fatalf("tool = %s/%s", product, tool)
		}
		if params["keyword"] != "柏荣" || !reflect.DeepEqual(params["dimension"], []string{"name"}) {
			t.Fatalf("params = %#v", params)
		}
		return map[string]any{"result": []any{map[string]any{
			"userId":         "u1",
			"openDingTalkId": "D1",
			"meta":           map[string]any{"name": "柏荣"},
		}}}, nil
	})
	resolved, err := ResolveEnterpriseUser(reader, " 柏荣 ", IdentityAny)
	if err != nil || resolved.MatchType != "exact" || resolved.Selected.UserID != "u1" {
		t.Fatalf("resolved = %#v, err = %v", resolved, err)
	}
}

func TestCrossPlatformCoverageUserResolutionFailsClosedOnUnpageableContinuation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		resolve func(Reader) (UserResolution, error)
		data    map[string]any
	}{
		{
			name: "contact search",
			resolve: func(reader Reader) (UserResolution, error) {
				return ResolveUser(reader, "张三", IdentityAny)
			},
			data: map[string]any{
				"result":     []any{map[string]any{"userId": "u1", "name": "张三"}},
				"hasMore":    true,
				"nextCursor": "page-2",
			},
		},
		{
			name: "enterprise search",
			resolve: func(reader Reader) (UserResolution, error) {
				return ResolveEnterpriseUser(reader, "张三", IdentityAny)
			},
			data: map[string]any{
				"result":     []any{map[string]any{"userId": "u1", "name": "张三"}},
				"nextCursor": "page-2",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.resolve(resolverReaderFunc(func(string, string, map[string]any) (map[string]any, error) {
				return tc.data, nil
			}))
			var typed *apperrors.Error
			if !stderrors.As(err, &typed) || typed.Reason != "resolution_incomplete" || !typed.Retryable {
				t.Fatalf("error = %#v", err)
			}
			if typed.Details["entityType"] != "user" || typed.Details["subtype"] != StatusIncomplete {
				t.Fatalf("details = %#v", typed.Details)
			}
		})
	}

	fullPage := make([]any, userSearchDefaultResultLimit)
	fullPage[0] = map[string]any{
		"openDingTalkId": "D1",
		"name":           "张三",
	}
	for i := 1; i < len(fullPage); i++ {
		// These first-page contacts do not satisfy the downstream open-ID
		// requirement. A second-page namesake still could, so the one usable
		// first-page candidate must not be treated as globally unique.
		fullPage[i] = map[string]any{
			"userId": fmt.Sprintf("u%d", i),
			"name":   fmt.Sprintf("张三候选%d", i),
		}
	}
	_, err := ResolveUser(resolverReaderFunc(func(string, string, map[string]any) (map[string]any, error) {
		return map[string]any{"result": fullPage}, nil
	}), "张三", IdentityOpenDingTalkID)
	var fullPageError *apperrors.Error
	if !stderrors.As(err, &fullPageError) || fullPageError.Reason != "resolution_incomplete" {
		t.Fatalf("full-page error = %#v", err)
	}
	if cause := fmt.Sprint(fullPageError.Details["cause"]); !strings.Contains(cause, "满额 20 条") {
		t.Fatalf("full-page cause = %q", cause)
	}

	terminalCursor := map[string]any{
		"result":     []any{map[string]any{"userId": "u1", "name": "张三"}},
		"hasMore":    false,
		"nextCursor": "terminal-token",
	}
	resolved, err := ResolveUser(resolverReaderFunc(func(string, string, map[string]any) (map[string]any, error) {
		return terminalCursor, nil
	}), "张三", IdentityAny)
	if err != nil || resolved.Selected.UserID != "u1" {
		t.Fatalf("terminal cursor resolved = %#v, err = %v", resolved, err)
	}
}

func TestCrossPlatformCoverageOpenConversationIDIsNeverSearchedAsAGroupName(t *testing.T) {
	for _, value := range []string{"cid-fixture-chat-0001", " CIDO123456789 "} {
		if !LooksLikeOpenConversationID(value) {
			t.Fatalf("LooksLikeOpenConversationID(%q) = false", value)
		}
	}
	for _, value := range []string{"cid", "项目cid群", "conversation-1"} {
		if LooksLikeOpenConversationID(value) {
			t.Fatalf("LooksLikeOpenConversationID(%q) = true", value)
		}
	}

	reader := &chatResolutionReader{}
	_, err := ResolveChat(reader, "cid-fixture-chat-0001")
	var typed *apperrors.Error
	if err == nil || !stderrors.As(err, &typed) || typed.Reason != "target_type_mismatch" {
		t.Fatalf("ResolveChat(stable id) error = %v", err)
	}
	if strings.Contains(typed.Message, "看起来是") || !strings.Contains(typed.Message, "群目标参数类型不匹配") {
		t.Fatalf("ResolveChat(stable id) message = %q", typed.Message)
	}
	if typed.Details["providedType"] != "openConversationId" || typed.Details["expectedType"] != "chatName" {
		t.Fatalf("ResolveChat(stable id) details = %#v", typed.Details)
	}
	if len(reader.calls) != 0 {
		t.Fatalf("stable id unexpectedly reached search: %#v", reader.calls)
	}
}

func TestCrossPlatformCoverageResolveChatTargetStableIDBypassesSearch(t *testing.T) {
	reader := &chatResolutionReader{}
	resolved, err := ResolveChatTarget(reader, " cid-fixture-chat-0001 ", "")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Selected.OpenConversationID != "cid-fixture-chat-0001" || resolved.MatchType != "stable_id" {
		t.Fatalf("resolved = %#v", resolved)
	}
	if len(reader.calls) != 0 {
		t.Fatalf("stable id unexpectedly reached search: %#v", reader.calls)
	}
}

func TestCrossPlatformCoverageResolveChatTargetNaturalDirectValueUsesResolver(t *testing.T) {
	reader := &chatResolutionReader{responses: []map[string]any{{
		"result":  []any{map[string]any{"openConversationId": "cid-project-1", "title": "项目群"}},
		"hasMore": false,
	}}}
	resolved, err := ResolveChatTarget(reader, "项目群", "")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Selected.OpenConversationID != "cid-project-1" || len(reader.calls) != 1 {
		t.Fatalf("resolved = %#v calls = %#v", resolved, reader.calls)
	}
}

func TestCrossPlatformCoverageResolveChatTargetQueryStableIDAlsoBypassesSearch(t *testing.T) {
	reader := &chatResolutionReader{}
	resolved, err := ResolveChatTarget(reader, "", "cid-query-123456")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Selected.OpenConversationID != "cid-query-123456" || len(reader.calls) != 0 {
		t.Fatalf("resolved = %#v calls = %#v", resolved, reader.calls)
	}
}

func TestCrossPlatformCoverageUserSelectionDedupesButDoesNotHideNamesakes(t *testing.T) {
	users := dedupeUsers([]User{
		{UserID: "u1", OpenDingTalkID: "D1", Name: "张三"},
		{UserID: "u1", OpenDingTalkID: "D1", Name: "duplicate"},
		{UserID: "u2", OpenDingTalkID: "D2", Name: "张三丰"},
	})
	selected, matchType := selectUsers(users, "  张三 ")
	if len(selected) != 2 || matchType != "ambiguous" {
		t.Fatalf("selected = %#v, matchType = %q", selected, matchType)
	}
}

func TestCrossPlatformCoverageChatSelectionKeepsMultipleExactMatchesAmbiguous(t *testing.T) {
	chats := dedupeChats([]Chat{
		{OpenConversationID: "c1", Name: "项目群"},
		{OpenConversationID: "c1", Name: "项目群"},
		{OpenConversationID: "c2", Name: "项目群"},
		{OpenConversationID: "c3", Name: "项目群-归档"},
	})
	selected, matchType := preferExactChats(chats, "项目群")
	if len(selected) != 2 || matchType != "exact" {
		t.Fatalf("selected = %#v, matchType = %q", selected, matchType)
	}
}

func TestCrossPlatformCoverageResolveChatPagesBeforeApplyingExactPreference(t *testing.T) {
	reader := &chatResolutionReader{responses: []map[string]any{
		{
			"result": []any{
				map[string]any{"openConversationId": "archive", "title": "项目群-归档"},
			},
			"hasMore":    true,
			"nextCursor": "page-2",
		},
		{
			"result": []any{
				map[string]any{"openConversationId": "active", "title": "项目群"},
			},
			"hasMore": false,
		},
	}}
	resolved, err := ResolveChat(reader, "项目群")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Selected.OpenConversationID != "active" || resolved.MatchType != "exact" {
		t.Fatalf("resolved = %#v", resolved)
	}
	wantCursors := []any{"0", "page-2"}
	gotCursors := []any{reader.calls[0]["cursor"], reader.calls[1]["cursor"]}
	if !reflect.DeepEqual(gotCursors, wantCursors) {
		t.Fatalf("cursors = %#v, want %#v", gotCursors, wantCursors)
	}
}

func TestCrossPlatformCoverageResolveChatKeepsExactNamesakesAcrossPagesAmbiguous(t *testing.T) {
	reader := &chatResolutionReader{responses: []map[string]any{
		{
			"result": []any{
				map[string]any{"openConversationId": "c1", "title": "项目群"},
			},
			"hasMore":    true,
			"nextCursor": "page-2",
		},
		{
			"result": map[string]any{
				"items": []any{
					map[string]any{"openConversationId": "c2", "title": "项目群"},
				},
				"hasMore": false,
			},
		},
	}}
	_, err := ResolveChat(reader, "项目群")
	var typed *apperrors.Error
	if !stderrors.As(err, &typed) || typed.Reason != "resolution_ambiguous" {
		t.Fatalf("error = %#v", err)
	}
	candidates, ok := typed.Details["candidates"].([]Chat)
	if !ok || len(candidates) != 2 {
		t.Fatalf("details = %#v", typed.Details)
	}
}

func TestCrossPlatformCoverageResolveChatFailsClosedWhenPaginationCannotAdvance(t *testing.T) {
	for _, tc := range []struct {
		name         string
		nextCursor   any
		wantFragment string
	}{
		{name: "missing cursor", wantFragment: "没有返回可继续"},
		{name: "stalled cursor", nextCursor: "0", wantFragment: "游标停滞"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := map[string]any{
				"result": []any{
					map[string]any{"openConversationId": "c1", "title": "项目群"},
				},
				"hasMore": true,
			}
			if tc.nextCursor != nil {
				response["nextCursor"] = tc.nextCursor
			}
			reader := &chatResolutionReader{responses: []map[string]any{response}}
			_, err := ResolveChat(reader, "项目群")
			var typed *apperrors.Error
			if !stderrors.As(err, &typed) || typed.Reason != "resolution_incomplete" || !typed.Retryable {
				t.Fatalf("error = %#v", err)
			}
			if typed.Details["subtype"] != StatusIncomplete {
				t.Fatalf("details = %#v", typed.Details)
			}
			if typed.Origin != "mcp_gateway" || typed.FailureStage != "target_resolution" || typed.ExecutionStarted == nil || *typed.ExecutionStarted {
				t.Fatalf("failure semantics = origin %q stage %q execution_started %v", typed.Origin, typed.FailureStage, typed.ExecutionStarted)
			}
			cause, _ := typed.Details["cause"].(string)
			if cause == "" || !strings.Contains(cause, tc.wantFragment) {
				t.Fatalf("cause = %q, want fragment %q", cause, tc.wantFragment)
			}
		})
	}
}

func TestCrossPlatformCoverageResolveChatRejectsFullMaximumProbeWithoutCursor(t *testing.T) {
	page := func(size int, prefix string) []any {
		rows := make([]any, size)
		for i := range rows {
			rows[i] = map[string]any{
				"openConversationId": fmt.Sprintf("%s-%d", prefix, i),
				"title":              fmt.Sprintf("项目群候选-%s-%d", prefix, i),
			}
		}
		return rows
	}
	reader := &chatResolutionReader{responses: []map[string]any{
		{"result": page(chatResolutionPageSize, "first"), "hasMore": false},
		{"result": page(chatResolutionMaxWindowSize, "probe"), "hasMore": false},
	}}
	_, err := ResolveChat(reader, "项目群")
	var typed *apperrors.Error
	if !stderrors.As(err, &typed) || typed.Reason != "resolution_incomplete" {
		t.Fatalf("error = %#v", err)
	}
	if len(reader.calls) != 2 || reader.calls[1]["limit"] != chatResolutionMaxWindowSize {
		t.Fatalf("calls = %#v", reader.calls)
	}
}

func TestCrossPlatformCoverageResolveChatAcceptsShortLegacyPageWithoutPaginationMetadata(t *testing.T) {
	reader := &chatResolutionReader{responses: []map[string]any{{
		"result": []any{
			map[string]any{"openConversationId": "c1", "title": "项目群"},
		},
	}}}
	resolved, err := ResolveChat(reader, "项目群")
	if err != nil || resolved.Selected.OpenConversationID != "c1" {
		t.Fatalf("resolved = %#v, err = %v", resolved, err)
	}
}

func TestCrossPlatformCoverageResolveChatProbesMaximumWindowBeforeSelecting(t *testing.T) {
	firstPage := make([]any, chatResolutionPageSize)
	for i := range firstPage {
		firstPage[i] = map[string]any{
			"openConversationId": fmt.Sprintf("archive-%d", i),
			"title":              fmt.Sprintf("项目群-归档-%d", i),
		}
	}
	reader := &chatResolutionReader{responses: []map[string]any{
		{"result": map[string]any{"groups": firstPage, "hasMore": false}},
		{"result": map[string]any{"groups": []any{
			map[string]any{"openConversationId": "active", "title": "项目群"},
		}, "hasMore": false}},
	}}
	resolved, err := ResolveChat(reader, "项目群")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Selected.OpenConversationID != "active" || resolved.MatchType != "exact" {
		t.Fatalf("resolved = %#v", resolved)
	}
	if len(reader.calls) != 2 || reader.calls[0]["limit"] != chatResolutionPageSize ||
		reader.calls[1]["limit"] != chatResolutionMaxWindowSize || reader.calls[1]["cursor"] != "0" {
		t.Fatalf("calls = %#v", reader.calls)
	}
}

func TestCrossPlatformCoverageResolutionErrorCarriesStructuredCandidates(t *testing.T) {
	err := newResolutionError(StatusAmbiguous, "chat", "项目群", []Chat{
		{OpenConversationID: "c1", Name: "项目群"},
		{OpenConversationID: "c2", Name: "项目群"},
	})
	var typed *apperrors.Error
	if !stderrors.As(err, &typed) {
		t.Fatalf("error type = %T", err)
	}
	if typed.Reason != "resolution_ambiguous" {
		t.Fatalf("reason = %q", typed.Reason)
	}
	if typed.Origin != "client" || typed.FailureStage != "target_resolution" || typed.ExecutionStarted == nil || *typed.ExecutionStarted {
		t.Fatalf("failure semantics = origin %q stage %q execution_started %v", typed.Origin, typed.FailureStage, typed.ExecutionStarted)
	}
	if typed.Details["type"] != "resolution" || typed.Details["subtype"] != StatusAmbiguous {
		t.Fatalf("details = %#v", typed.Details)
	}
	candidates, ok := typed.Details["candidates"].([]Chat)
	if !ok || len(candidates) != 2 {
		t.Fatalf("candidates = %#v", typed.Details["candidates"])
	}
}

func TestCrossPlatformCoverageResolverCompletionBranches(t *testing.T) {
	t.Run("user transport and selection", func(t *testing.T) {
		upstream := stderrors.New("upstream")
		if _, err := ResolveUser(resolverReaderFunc(func(string, string, map[string]any) (map[string]any, error) {
			return nil, upstream
		}), "张三", IdentityAny); !stderrors.Is(err, upstream) {
			t.Fatalf("transport error = %v", err)
		}

		reader := resolverReaderFunc(func(_ string, _ string, params map[string]any) (map[string]any, error) {
			switch params["keyword"] {
			case "missing":
				return map[string]any{"result": []any{}}, nil
			case "external":
				return map[string]any{"result": []any{map[string]any{"openDingTalkId": "D1", "name": "外部"}}}, nil
			default:
				return map[string]any{"result": []any{map[string]any{"userId": "u1", "openDingTalkId": "D1", "name": "张三"}}}, nil
			}
		})
		if _, err := ResolveUser(reader, "missing", IdentityAny); err == nil {
			t.Fatal("missing user unexpectedly resolved")
		}
		if resolved, err := ResolveUser(reader, "张三", IdentityUserID); err != nil || resolved.MatchType != "exact" {
			t.Fatalf("exact user = %#v, %v", resolved, err)
		}
		if resolved, err := ResolveUser(reader, "external", IdentityOpenDingTalkID); err != nil || resolved.Selected.OpenDingTalkID != "D1" {
			t.Fatalf("external user = %#v, %v", resolved, err)
		}
		ambiguous := resolverReaderFunc(func(string, string, map[string]any) (map[string]any, error) {
			return map[string]any{"result": []any{
				map[string]any{"userId": "u1", "name": "甲"},
				map[string]any{"userId": "u2", "name": "乙"},
			}}, nil
		})
		if _, err := ResolveUser(ambiguous, "用户", IdentityAny); err == nil {
			t.Fatal("ambiguous user unexpectedly resolved")
		}
		if _, err := ResolveEnterpriseUser(resolverReaderFunc(func(string, string, map[string]any) (map[string]any, error) {
			return nil, upstream
		}), "张三", IdentityAny); !stderrors.Is(err, upstream) {
			t.Fatalf("enterprise transport error = %v", err)
		}
	})

	t.Run("chat transport metadata and page limit", func(t *testing.T) {
		upstream := stderrors.New("upstream")
		if _, err := ResolveChat(resolverReaderFunc(func(string, string, map[string]any) (map[string]any, error) {
			return nil, upstream
		}), "群"); !stderrors.Is(err, upstream) {
			t.Fatalf("transport error = %v", err)
		}

		fullPage := make([]any, chatResolutionPageSize)
		for i := range fullPage {
			fullPage[i] = map[string]any{"openConversationId": fmt.Sprintf("c%d", i), "title": fmt.Sprintf("群%d", i)}
		}
		if _, err := ResolveChat(&chatResolutionReader{responses: []map[string]any{{"result": fullPage}}}, "群"); err == nil {
			t.Fatal("full page without continuation unexpectedly resolved")
		}

		pages := 0
		limitReader := resolverReaderFunc(func(_ string, _ string, _ map[string]any) (map[string]any, error) {
			pages++
			return map[string]any{
				"result":     []any{map[string]any{"openConversationId": fmt.Sprintf("c%d", pages), "title": "群"}},
				"hasMore":    true,
				"nextCursor": fmt.Sprintf("page-%d", pages),
			}, nil
		})
		if _, err := ResolveChat(limitReader, "群"); err == nil || pages != chatResolutionPageLimit {
			t.Fatalf("page limit error = %v, pages=%d", err, pages)
		}
	})

	t.Run("target and pagination validation", func(t *testing.T) {
		reader := &chatResolutionReader{}
		if _, err := ResolveChatTarget(reader, "cid-123456789", "群"); err == nil {
			t.Fatal("conflicting targets unexpectedly resolved")
		}
		if _, err := ResolveChatTarget(reader, "", ""); err == nil {
			t.Fatal("missing target unexpectedly resolved")
		}
		if page := extractChatPagination(nil); page.hasMoreKnown || page.nextCursor != "" {
			t.Fatalf("nil pagination = %#v", page)
		}
		page := extractChatPagination(map[string]any{"data": map[string]any{"has_more": true, "next_token": 42}})
		if !page.hasMoreKnown || !page.hasMore || page.nextCursor != "42" {
			t.Fatalf("nested pagination = %#v", page)
		}
		for _, value := range []any{nil, "", "<nil>"} {
			if got := paginationString(value); got != "" {
				t.Fatalf("paginationString(%#v) = %q", value, got)
			}
		}
	})

	t.Run("batch resolution", func(t *testing.T) {
		reader := resolverReaderFunc(func(product, _ string, params map[string]any) (map[string]any, error) {
			query := fmt.Sprint(params["keyword"])
			if query == "upstream" {
				return nil, stderrors.New("upstream")
			}
			if product == "contact" {
				if query == "missing" {
					return map[string]any{"result": []any{}}, nil
				}
				return map[string]any{"result": []any{map[string]any{"userId": "u1", "name": query}}}, nil
			}
			if query == "missing" {
				return map[string]any{"result": []any{}, "hasMore": false}, nil
			}
			return map[string]any{"result": []any{map[string]any{"openConversationId": "cid-shared-123", "title": query}}, "hasMore": false}, nil
		})
		if users, err := ResolveUsers(reader, []string{"张三", " 张三 "}, IdentityUserID); err != nil || len(users) != 1 {
			t.Fatalf("deduped users = %#v, %v", users, err)
		}
		if _, err := ResolveUsers(reader, []string{"missing"}, IdentityUserID); err == nil {
			t.Fatal("batch missing user unexpectedly resolved")
		}
		if _, err := ResolveUsers(reader, []string{"upstream"}, IdentityUserID); err == nil {
			t.Fatal("batch user transport error missing")
		}
		if chats, err := ResolveChats(reader, []string{"群一", "群二"}); err != nil || len(chats) != 1 {
			t.Fatalf("deduped chats = %#v, %v", chats, err)
		}
		if _, err := ResolveChats(reader, []string{"missing"}); err == nil {
			t.Fatal("batch missing chat unexpectedly resolved")
		}
		if _, err := ResolveChats(reader, []string{"upstream"}); err == nil {
			t.Fatal("batch chat transport error missing")
		}

		assertIncomplete := func(label string, err error) {
			t.Helper()
			var typed *apperrors.Error
			if !stderrors.As(err, &typed) || typed.Category != apperrors.CategoryAPI ||
				typed.Reason != "resolution_incomplete" || !typed.Retryable {
				t.Fatalf("%s error = %#v", label, err)
			}
		}
		fullUsers := make([]any, userSearchDefaultResultLimit)
		for i := range fullUsers {
			fullUsers[i] = map[string]any{"userId": fmt.Sprintf("u%d", i), "name": "张三"}
		}
		_, err := ResolveUsers(resolverReaderFunc(func(string, string, map[string]any) (map[string]any, error) {
			return map[string]any{"result": fullUsers}, nil
		}), []string{"张三", "李四"}, IdentityUserID)
		assertIncomplete("batch user", err)

		fullChats := make([]any, chatResolutionPageSize)
		for i := range fullChats {
			fullChats[i] = map[string]any{
				"openConversationId": fmt.Sprintf("cid-%d", i),
				"title":              "项目群",
			}
		}
		_, err = ResolveChats(resolverReaderFunc(func(string, string, map[string]any) (map[string]any, error) {
			return map[string]any{"result": fullChats}, nil
		}), []string{"项目群", "研发群"})
		assertIncomplete("batch chat", err)
	})

	t.Run("public projections and internal helpers", func(t *testing.T) {
		users := ExtractUsers(map[string]any{"items": []any{"bad", map[string]any{"name": "missing"}, map[string]any{"orgUserId": "u1", "staffName": "张三"}}})
		if got := UsersWithUserID(users); len(got) != 1 {
			t.Fatalf("users = %#v", got)
		}
		chats := ExtractChats(map[string]any{"groups": []any{"bad", map[string]any{"title": "missing"}, map[string]any{"id": "c1"}}})
		if len(chats) != 1 || len(PreferExactChats(chats, "none")) != 1 {
			t.Fatalf("chats = %#v", chats)
		}
		if labels := UserLabels([]User{{OpenDingTalkID: "D1", Name: "外部"}}); !reflect.DeepEqual(labels, []string{"外部(D1)"}) {
			t.Fatalf("user labels = %#v", labels)
		}
		if labels := ChatLabels([]Chat{{OpenConversationID: "c1"}}); !strings.Contains(labels[0], "未命名") {
			t.Fatalf("chat labels = %#v", labels)
		}
		if _, ok := resolutionDetails(stderrors.New("plain")); ok {
			t.Fatal("plain error reported resolution details")
		}
		if label := candidateLabels(1); label != "候选" {
			t.Fatalf("candidate label = %q", label)
		}
		if label := candidateLabels([]User{{UserID: "u1", Name: "甲"}}); label != "甲(u1)" {
			t.Fatalf("user candidate label = %q", label)
		}
		if firstList(nil, "result") != nil {
			t.Fatal("nil firstList should return nil")
		}
		if firstList(map[string]any{"result": "not-a-list"}, "result") != nil {
			t.Fatal("missing firstList should return nil")
		}
		if got := filterUsersByIdentity([]User{{OpenDingTalkID: "D1"}}, IdentityUserID); len(got) != 0 {
			t.Fatalf("user-id filter = %#v", got)
		}
		if got := filterUsersByIdentity([]User{{UserID: "u1"}}, IdentityOpenDingTalkID); len(got) != 0 {
			t.Fatalf("open-id filter = %#v", got)
		}

		profilectx.Set("fixture")
		t.Cleanup(func() { profilectx.Set("") })
		for _, err := range []error{
			newResolutionError(StatusNotFound, "user", "missing", nil),
			newIncompleteChatResolutionError("群", nil, "fixture"),
			newIncompleteUserResolutionError("用户", nil, "fixture"),
		} {
			var typed *apperrors.Error
			if !stderrors.As(err, &typed) || typed.Details["profile"] != "fixture" {
				t.Fatalf("profile details = %#v", err)
			}
		}
	})

	t.Run("batch deduplicates shared open ids", func(t *testing.T) {
		reader := resolverReaderFunc(func(_ string, _ string, params map[string]any) (map[string]any, error) {
			query := fmt.Sprint(params["keyword"])
			return map[string]any{"result": []any{map[string]any{
				"userId":         "user-" + query,
				"openDingTalkId": "D-shared",
				"name":           query,
			}}}, nil
		})
		resolved, err := ResolveUsers(reader, []string{"甲", "乙"}, IdentityAny)
		if err != nil || len(resolved) != 1 {
			t.Fatalf("shared open-id result = %#v, %v", resolved, err)
		}
	})
}

func TestCrossPlatformCoverageLooksLikeCurrentDOpenDingTalkID(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "canonical current D ciphertext", value: "DAAAAAAAAAAAiE", want: true},
		{name: "synthetic current D ciphertext with escaped plus", value: "DiPiPiPiPiPiPiPiPiPiP8iE", want: true},
		{name: "synthetic current D ciphertext with escaped literal i", value: "DYmJiiYmJiiYmIiE", want: true},
		{name: "surrounding whitespace", value: "  DAAAAAAAAAAAiE  ", want: true},
		{name: "empty", value: "", want: false},
		{name: "prefix only", value: "D", want: false},
		{name: "lowercase prefix", value: "dAAAAAAAAAAAiE", want: false},
		{name: "natural uppercase D fixture", value: "D-prefix-fixture-user", want: false},
		{name: "natural lowercase d fixture", value: "d-prefix-fixture-user", want: false},
		{name: "invalid escape", value: "DAAAAAiDAAAAAiE", want: false},
		{name: "non alphanumeric body", value: "DAAAAA/AAAAAiE", want: false},
		{name: "invalid base64", value: "Dabc", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := LooksLikeCurrentDOpenDingTalkID(tc.value); got != tc.want {
				t.Fatalf("LooksLikeCurrentDOpenDingTalkID(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageResolveUserTargetUsesFormatClassificationWithoutIDPreflight(t *testing.T) {
	const openID = "DAAAAAAAAAAAiE"
	calls := 0
	resolved, err := ResolveUserTarget(resolverReaderFunc(func(string, string, map[string]any) (map[string]any, error) {
		calls++
		return nil, stderrors.New("unexpected remote call")
	}), openID, IdentityAny)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 || resolved.MatchType != "stable_id" || resolved.Selected.OpenDingTalkID != openID {
		t.Fatalf("calls=%d resolved=%#v", calls, resolved)
	}
}

func TestCrossPlatformCoverageResolveSenderTargetScopesStableIDPreferenceToSenderInputs(t *testing.T) {
	calls := 0
	reader := resolverReaderFunc(func(product, tool string, params map[string]any) (map[string]any, error) {
		calls++
		if product != "contact" || tool != "search_contact_by_key_word" {
			t.Fatalf("unexpected tool %s/%s", product, tool)
		}
		query := fmt.Sprint(params["keyword"])
		switch query {
		case "D-prefix-fixture-user":
			return map[string]any{"result": []any{map[string]any{
				"userId": "fixture-user-d-upper", "openDingTalkId": "D-directory-value", "name": "D-prefix-fixture-user",
			}}}, nil
		case "d-prefix-fixture-user":
			return map[string]any{"result": []any{map[string]any{
				"userId": "fixture-user-d-lower", "openDingTalkId": "D-directory-value-2", "name": "d-prefix-fixture-user",
			}}}, nil
		case "fixture-user-id":
			return map[string]any{"result": []any{
				map[string]any{"userId": "other", "name": "相似候选"},
				map[string]any{"userId": "fixture-user-id", "openDingTalkId": "D-directory-value-3", "name": "测试目标用户"},
			}}, nil
		default:
			t.Fatalf("unexpected query %q", query)
			return nil, nil
		}
	})

	for _, query := range []string{"D-prefix-fixture-user", "d-prefix-fixture-user"} {
		_, err := ResolveSenderTarget(reader, query, IdentityAny)
		if err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}

	_, err := ResolveUser(reader, "fixture-user-id", IdentityAny)
	var ambiguous *apperrors.Error
	if !stderrors.As(err, &ambiguous) || ambiguous.Reason != "resolution_ambiguous" {
		t.Fatalf("shared name resolver error = %#v, want resolution_ambiguous", err)
	}

	resolved, err := ResolveSenderTarget(reader, "fixture-user-id", IdentityAny)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.MatchType != "stable_id" || resolved.Selected.UserID != "fixture-user-id" {
		t.Fatalf("exact sender userId resolution = %#v", resolved)
	}
	if calls != 4 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestCrossPlatformCoverageResolveSenderTargetRoutesCurrentDWithoutDirectoryLookup(t *testing.T) {
	const openID = "DAAAAAAAAAAAiE"
	calls := 0
	resolved, err := ResolveSenderTarget(resolverReaderFunc(func(string, string, map[string]any) (map[string]any, error) {
		calls++
		return nil, stderrors.New("unexpected directory call")
	}), openID, IdentityAny)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 || resolved.MatchType != "stable_id" || resolved.Selected.OpenDingTalkID != openID {
		t.Fatalf("calls=%d resolved=%#v", calls, resolved)
	}
}

func TestCrossPlatformCoverageResolveSenderTargetNeverSubstitutesUnrelatedUniqueDirectoryCandidate(t *testing.T) {
	resolved, err := ResolveSenderTarget(resolverReaderFunc(func(product, tool string, params map[string]any) (map[string]any, error) {
		if product != "contact" || tool != "search_contact_by_key_word" || params["keyword"] != "fixture-user-id" {
			t.Fatalf("unexpected call %s/%s %#v", product, tool, params)
		}
		return map[string]any{
			"result": []any{map[string]any{
				"userId": "other-user",
				"name":   "其他用户",
			}},
			"hasMore": false,
		}, nil
	}), "fixture-user-id", IdentityAny)
	if err != nil {
		t.Fatal(err)
	}
	if !IsUnverifiedUserIDResolution(resolved) || resolved.Selected.UserID != "fixture-user-id" {
		t.Fatalf("resolution=%#v", resolved)
	}
}

func TestCrossPlatformCoverageResolveSenderTargetOpenIDFailureEdges(t *testing.T) {
	t.Run("directory failure", func(t *testing.T) {
		wantErr := stderrors.New("directory unavailable")
		_, err := ResolveSenderTarget(resolverReaderFunc(func(string, string, map[string]any) (map[string]any, error) {
			return nil, wantErr
		}), "fixture-sender", IdentityOpenDingTalkID)
		if !stderrors.Is(err, wantErr) {
			t.Fatalf("error=%#v, want directory error", err)
		}
	})

	t.Run("incomplete directory result", func(t *testing.T) {
		_, err := ResolveSenderTarget(resolverReaderFunc(func(string, string, map[string]any) (map[string]any, error) {
			return map[string]any{
				"result": []any{map[string]any{
					"openDingTalkId": "fixture-open-id-1",
					"name":           "测试候选用户",
				}},
				"hasMore": true,
			}, nil
		}), "fixture-sender", IdentityOpenDingTalkID)
		var typed *apperrors.Error
		if !stderrors.As(err, &typed) || typed.Reason != "resolution_incomplete" {
			t.Fatalf("error=%#v, want resolution_incomplete", err)
		}
	})

	t.Run("multiple exact names remain ambiguous", func(t *testing.T) {
		_, err := ResolveSenderTarget(resolverReaderFunc(func(string, string, map[string]any) (map[string]any, error) {
			return map[string]any{
				"result": []any{
					map[string]any{"openDingTalkId": "fixture-open-id-1", "name": "测试同名用户"},
					map[string]any{"openDingTalkId": "fixture-open-id-2", "name": "测试同名用户"},
				},
				"hasMore": false,
			}, nil
		}), "测试同名用户", IdentityOpenDingTalkID)
		var typed *apperrors.Error
		if !stderrors.As(err, &typed) || typed.Reason != "resolution_ambiguous" {
			t.Fatalf("error=%#v, want resolution_ambiguous", err)
		}
	})

	t.Run("no exact open id or name match", func(t *testing.T) {
		_, err := ResolveSenderTarget(resolverReaderFunc(func(string, string, map[string]any) (map[string]any, error) {
			return map[string]any{
				"result": []any{map[string]any{
					"openDingTalkId": "fixture-open-id-1",
					"name":           "其他测试用户",
				}},
				"hasMore": false,
			}, nil
		}), "fixture-sender", IdentityOpenDingTalkID)
		var typed *apperrors.Error
		if !stderrors.As(err, &typed) || typed.Reason != "resolution_not_found" {
			t.Fatalf("error=%#v, want resolution_not_found", err)
		}
	})
}

func TestCrossPlatformCoverageValidateExplicitOpenDingTalkIDNeverFallsBack(t *testing.T) {
	if err := ValidateExplicitOpenDingTalkID("--open-dingtalk-id", "DAAAAAAAAAAAiE"); err != nil {
		t.Fatalf("valid format: %v", err)
	}
	err := ValidateExplicitOpenDingTalkID("--open-dingtalk-id", "D-prefix-fixture-user")
	var typed *apperrors.Error
	if !stderrors.As(err, &typed) || typed.Reason != "target_type_mismatch" {
		t.Fatalf("error=%#v", err)
	}
	err = ValidateExplicitOpenDingTalkID("--open-dingtalk-id", "  DAAAAAAAAAAAiE  ")
	if !stderrors.As(err, &typed) || typed.Reason != "target_type_mismatch" {
		t.Fatalf("whitespace-wrapped explicit ID error=%#v", err)
	}
}

func TestCrossPlatformCoverageResolveStableUserTargetPassesExplicitUserIDWithoutDirectory(t *testing.T) {
	calls := 0
	reader := resolverReaderFunc(func(string, string, map[string]any) (map[string]any, error) {
		calls++
		return nil, stderrors.New("directory unavailable")
	})

	resolved, err := ResolveStableUserTarget(reader, "fixture-user-id", IdentityAny)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 || resolved.MatchType != "stable_id" || resolved.Selected.UserID != "fixture-user-id" {
		t.Fatalf("exact userId resolution = %#v", resolved)
	}
}

func TestCrossPlatformCoverageResolveStableUserTargetRoutesCurrentDWithoutDirectoryLookup(t *testing.T) {
	const openID = "DAAAAAAAAAAAiE"
	calls := 0
	resolved, err := ResolveStableUserTarget(resolverReaderFunc(func(string, string, map[string]any) (map[string]any, error) {
		calls++
		return nil, stderrors.New("unexpected remote call")
	}), openID, IdentityAny)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 || resolved.Selected.OpenDingTalkID != openID {
		t.Fatalf("calls=%d resolved=%#v", calls, resolved)
	}
}

func TestStableIdentityExactMatchWinsEvenWhenDirectoryPageIsIncomplete(t *testing.T) {
	reader := resolverReaderFunc(func(string, string, map[string]any) (map[string]any, error) {
		return map[string]any{
			"result": []any{
				map[string]any{"userId": "fixture-user-id", "openDingTalkId": "DAAAAAAAAAAAiE", "name": "测试目标用户"},
				map[string]any{"userId": "other", "name": "其他候选"},
			},
			"hasMore": true,
		}, nil
	})
	for _, resolve := range []struct {
		name string
		fn   func(Reader, string, IdentityRequirement) (UserResolution, error)
	}{
		{name: "sender", fn: ResolveSenderTarget},
		{name: "stable-only", fn: ResolveStableUserTarget},
	} {
		t.Run(resolve.name, func(t *testing.T) {
			resolved, err := resolve.fn(reader, "fixture-user-id", IdentityAny)
			if err != nil {
				t.Fatal(err)
			}
			if resolved.MatchType != "stable_id" || resolved.Selected.UserID != "fixture-user-id" {
				t.Fatalf("resolved=%#v", resolved)
			}
		})
	}
}

func TestCrossPlatformCoverageMixedIdentityResolutionFailureEdges(t *testing.T) {
	unexpectedReader := resolverReaderFunc(func(string, string, map[string]any) (map[string]any, error) {
		return nil, stderrors.New("directory unavailable")
	})

	for _, tc := range []struct {
		name        string
		resolve     func(Reader, string, IdentityRequirement) (UserResolution, error)
		value       string
		requirement IdentityRequirement
		wantReason  string
	}{
		{name: "mixed empty", resolve: ResolveUserTarget, wantReason: "missing_target"},
		{name: "mixed open id requires user id", resolve: ResolveUserTarget, value: "DAAAAAAAAAAAiE", requirement: IdentityUserID, wantReason: "target_type_mismatch"},
		{name: "stable empty", resolve: ResolveStableUserTarget, wantReason: "missing_target"},
		{name: "stable open id requires user id", resolve: ResolveStableUserTarget, value: "DAAAAAAAAAAAiE", requirement: IdentityUserID, wantReason: "target_type_mismatch"},
		{name: "stable user id requires open id", resolve: ResolveStableUserTarget, value: "ordinary-user", requirement: IdentityOpenDingTalkID, wantReason: "target_type_mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.resolve(unexpectedReader, tc.value, tc.requirement)
			var typed *apperrors.Error
			if !stderrors.As(err, &typed) || typed.Reason != tc.wantReason {
				t.Fatalf("error=%#v, want reason %q", err, tc.wantReason)
			}
		})
	}

	if _, err := ResolveUserTarget(unexpectedReader, "ordinary-user", IdentityAny); err == nil || err.Error() != "directory unavailable" {
		t.Fatalf("mixed directory error=%v", err)
	}
	resolved, err := ResolveSenderTarget(unexpectedReader, "ordinary-user", IdentityAny)
	if err != nil || !IsUnverifiedUserIDResolution(resolved) || resolved.Selected.UserID != "ordinary-user" {
		t.Fatalf("sender fallback resolution=%#v error=%v", resolved, err)
	}
	resolved, err = ResolveStableUserTarget(unexpectedReader, "ordinary-user", IdentityAny)
	if err != nil || resolved.MatchType != "stable_id" || resolved.Selected.UserID != "ordinary-user" {
		t.Fatalf("stable passthrough resolution=%#v error=%v", resolved, err)
	}
}

func TestCrossPlatformCoverageStableIdentityAmbiguousAndIncompleteEdges(t *testing.T) {
	duplicateReader := resolverReaderFunc(func(string, string, map[string]any) (map[string]any, error) {
		return map[string]any{"result": []any{
			map[string]any{"userId": "duplicate", "openDingTalkId": "D-one", "name": "甲"},
			map[string]any{"userId": "other", "openDingTalkId": "duplicate", "name": "乙"},
		}}, nil
	})
	_, err := ResolveSenderTarget(duplicateReader, "duplicate", IdentityAny)
	var typed *apperrors.Error
	if !stderrors.As(err, &typed) || typed.Reason != "resolution_ambiguous" {
		t.Fatalf("ambiguous sender error=%#v", err)
	}

	incompleteReader := resolverReaderFunc(func(string, string, map[string]any) (map[string]any, error) {
		return map[string]any{
			"result":  []any{map[string]any{"userId": "other", "name": "其他"}},
			"hasMore": true,
		}, nil
	})
	resolved, err := ResolveSenderTarget(incompleteReader, "missing", IdentityAny)
	if err != nil || !IsUnverifiedUserIDResolution(resolved) || resolved.Selected.UserID != "missing" {
		t.Fatalf("incomplete sender fallback=%#v error=%v", resolved, err)
	}
}

func TestCrossPlatformCoverageCurrentDCodecEdges(t *testing.T) {
	if _, ok := unescapeCurrentDOpenDingTalkID("abcdi"); ok {
		t.Fatal("dangling escape unexpectedly accepted")
	}
	if decoded, ok := unescapeCurrentDOpenDingTalkID("iS"); !ok || decoded != "/" {
		t.Fatalf("slash escape decoded=%q ok=%v", decoded, ok)
	}
	if got := escapeCurrentDOpenDingTalkID("i+/="); got != "iiiPiSiE" {
		t.Fatalf("escaped=%q", got)
	}
}
