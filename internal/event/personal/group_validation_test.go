// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package personal

import (
	"strings"
	"testing"
)

func TestCrossPlatformCoverageGroupRuleRejectsMultipleGroups(t *testing.T) {
	for _, def := range Catalog("", true, false) {
		if def.RuleType != "group" {
			continue
		}
		t.Run(def.EventKey, func(t *testing.T) {
			for _, group := range []string{"cid-a,cid-b", "cid-a, cid-b", ",cid-a", "cid-a,", "cid-a,cid-a"} {
				_, params, err := BuildRuleParam(def.EventKey, RuleOptions{GroupID: group})
				if err == nil || !strings.Contains(err.Error(), "--group accepts exactly one openConversationId") || params != nil {
					t.Errorf("group %q: params=%#v error=%v; want rejection", group, params, err)
				}
			}
			rule, params, err := BuildRuleParam(def.EventKey, RuleOptions{GroupID: "  cid-single+/==  "})
			if err != nil || rule != "group" || params["openConversationId"] != "cid-single+/==" {
				t.Fatalf("single group changed: rule=%q params=%#v error=%v", rule, params, err)
			}
		})
	}
}
