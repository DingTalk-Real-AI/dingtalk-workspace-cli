// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package outboundcall

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageOutboundCallInput(t *testing.T) {
	for _, infos := range [][]CalleeInfo{
		nil, {}, {nil},
		{{"callee": "u", "prompt": "p"}},
		{{"callee": 1, "prompt": "p", "greeting": "g"}},
		{{"callee": "u", "prompt": " ", "greeting": "g"}},
		{{"Callee": "u", "prompt": "p", "greeting": "g"}},
		{{"callee": "u", "prompt": "p", "greeting": "g"}, {"callee": "v", "prompt": "p", "greeting": "g"}},
		{{"callee": "u", "prompt": "p", "greeting": "g", "extra": make(chan int)}},
	} {
		if err := (CreateRequest{CalleeInfos: infos}).Validate(); err == nil {
			t.Errorf("invalid request accepted: %#v", infos)
		}
	}
	r, err := Single("u", " p\n", " g ")
	if err != nil || len(r.CalleeInfos) != 1 || r.CalleeInfos[0]["prompt"] != " p\n" || r.CalleeInfos[0]["greeting"] != " g " {
		t.Fatalf("payload lost or changed: %#v err=%v", r, err)
	}
	raw, err := json.Marshal(r.Arguments())
	if err != nil || string(raw) != `{"calleeInfos":[{"callee":"u","greeting":" g ","prompt":" p\n"}]}` {
		t.Fatalf("single-callee payload: %s err=%v", raw, err)
	}
	for _, values := range [][3]string{{"", "p", "g"}, {"u", "", "g"}, {"u", "p", " "}} {
		if _, err := Single(values[0], values[1], values[2]); err == nil {
			t.Errorf("empty field accepted: %#v", values)
		}
	}
	if _, err := Single("u", strings.Repeat("p", MaxInputBytes), "g"); err == nil {
		t.Fatal("oversize single call accepted")
	}
}
