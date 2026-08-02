// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package smart

import "testing"

func TestPreferExactGroupMatches(t *testing.T) {
	groups := []sendGroupMatch{
		{id: "exact", name: "DWS-SHORTCUT-AUDIT"},
		{id: "prefix", name: "DWS-SHORTCUT-AUDIT-THREAD"},
		{id: "exact", name: "DWS-SHORTCUT-AUDIT"},
	}
	got := preferExactGroupMatches(groups, "  dws-shortcut-audit ")
	if len(got) != 1 || got[0].id != "exact" {
		t.Fatalf("exact group resolution = %#v", got)
	}

	got = preferExactGroupMatches(groups, "SHORTCUT")
	if len(got) != 2 {
		t.Fatalf("ambiguous substring resolution = %#v, want 2 unique groups", got)
	}
}

func TestResolveMemberTypes(t *testing.T) {
	users, bots, err := resolveMemberTypes(nil)
	if err != nil || !users || !bots {
		t.Fatalf("default member types = users:%v bots:%v err:%v", users, bots, err)
	}
	users, bots, err = resolveMemberTypes([]string{"BOT", "user", "bot"})
	if err != nil || !users || !bots {
		t.Fatalf("normalized member types = users:%v bots:%v err:%v", users, bots, err)
	}
	if _, _, err := resolveMemberTypes([]string{"service-account"}); err == nil {
		t.Fatal("invalid member type was accepted")
	}
}

func TestGroupBotProject(t *testing.T) {
	got := groupBotProject(map[string]any{
		"result": map[string]any{
			"bots": []any{
				map[string]any{
					"name":                   "日报机器人",
					"botOpenDingTalkId":      "DINGBOT",
					"openBotId":              "open-bot",
					"robotCode":              "robot-code",
					"credentialsNotForAgent": "must-not-leak",
				},
			},
		},
	})
	if len(got) != 1 || got[0]["name"] != "日报机器人" || got[0]["openBotId"] != "open-bot" {
		t.Fatalf("bot projection = %#v", got)
	}
	if _, leaked := got[0]["credentialsNotForAgent"]; leaked {
		t.Fatalf("bot projection leaked unrelated field: %#v", got[0])
	}
}
