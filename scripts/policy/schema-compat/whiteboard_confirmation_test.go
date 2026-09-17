// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package main

import "testing"

func TestCrossPlatformCoverageWhiteboardCreateConfirmationHardening(t *testing.T) {
	const path = "whiteboard/whiteboard.create_with_content"
	old := baselineContract().Products["doc"].Tools["doc.create"]
	old.PrimaryCLIPath = "whiteboard create-with-content"
	next := old
	next.Confirmation = "user_required"
	if failures := checkToolCompatibility(path, old, next); len(failures) != 0 {
		t.Fatalf("reviewed hardening rejected: %v", failures)
	}
	if failures := checkToolCompatibility(path, next, old); len(failures) == 0 {
		t.Fatal("confirmation weakening must remain incompatible")
	}
	for _, other := range []string{"whiteboard/whiteboard.update", "whiteboard/whiteboard.apply_personal_whiteboard_tpl", "doc/doc.create"} {
		if failures := checkToolCompatibility(other, old, next); len(failures) == 0 {
			t.Fatalf("unreviewed tool %s borrowed authorization", other)
		}
	}
	next.Risk = "high"
	if failures := checkToolCompatibility(path, old, next); len(failures) == 0 {
		t.Fatal("unreviewed risk change must not be bundled into authorization")
	}
}
