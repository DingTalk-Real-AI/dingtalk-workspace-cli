package helpers

import (
	"slices"
	"testing"
)

func TestCrossPlatformCoverageCrossProductAliasPeers(t *testing.T) {
	// 组内任一成员都能查到其余成员，且结果不含自身。
	for _, group := range crossProductAliases {
		for _, name := range group.names {
			peers := crossProductAliasPeers(name)
			if len(peers) != len(group.names)-1 {
				t.Fatalf("peers(%q) = %#v, want %d entries", name, peers, len(group.names)-1)
			}
			if slices.Contains(peers, name) {
				t.Fatalf("peers(%q) contains itself: %#v", name, peers)
			}
			for _, peer := range peers {
				if !slices.Contains(group.names, peer) {
					t.Fatalf("peers(%q) leaked %q from another group", name, peer)
				}
			}
		}
	}

	// drive list 的分页互斥（driveListLimitConflict / driveListCursorConflict）依赖这两组：
	// 改动注册表若挪走这些别名，会在此失败而不是静默丢失互斥覆盖。
	if peers := crossProductAliasPeers("limit"); !slices.Contains(peers, "page-size") {
		t.Fatalf("limit peers = %#v, want page-size", peers)
	}
	for _, want := range []string{"page-token", "next-token"} {
		if peers := crossProductAliasPeers("cursor"); !slices.Contains(peers, want) {
			t.Fatalf("cursor peers = %#v, want %s", peers, want)
		}
	}

	// 不属于任何语义组的 flag 返回 nil。
	if peers := crossProductAliasPeers("latest"); peers != nil {
		t.Fatalf("peers(latest) = %#v, want nil", peers)
	}
}
