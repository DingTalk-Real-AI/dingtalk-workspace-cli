// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package wiki

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageWikiMemberRemoveConfirmation(t *testing.T) {
	for _, users := range []string{"u1", "u1,u2"} {
		t.Run(users, func(t *testing.T) {
			args := []string{"+member-remove", "--workspace", "w", "--users", users}
			denied := &wikiCoverageCaller{}
			_, err := runWikiCoverageCLI(t, denied, args...)
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Reason != "confirmation_required" || len(denied.calls) != 0 {
				t.Fatalf("unconfirmed remove err=%#v calls=%#v", err, denied.calls)
			}
			accepted := &wikiCoverageCaller{responses: map[string][]string{"wiki/remove_member": {`{"success":true}`}}}
			out, err := runWikiCoverageCLI(t, accepted, append(args, "--yes")...)
			if err != nil || out["success"] != true || len(accepted.calls) != 1 {
				t.Fatalf("confirmed remove out=%#v err=%v calls=%#v", out, err, accepted.calls)
			}
			call := accepted.calls[0]
			if call.product != "wiki" || call.tool != "remove_member" || call.args["workspaceId"] != "w" ||
				!reflect.DeepEqual(call.args["userIds"], strings.Split(users, ",")) {
				t.Fatalf("remove changed the authorized target: %#v", call)
			}
			verification, _ := out["verification"].(map[string]any)
			if verification["status"] != "terminal_response_only" {
				t.Fatalf("confirmation must not invent readback: %#v", verification)
			}
			preview := &wikiCoverageCaller{}
			out, err = runWikiCoverageCLI(t, preview, append(args, "--dry-run")...)
			if err != nil || out["executed"] != false || len(preview.calls) != 0 {
				t.Fatalf("preview out=%#v err=%v calls=%#v", out, err, preview.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageWikiMemberRemoveKeepsOwnerFailure(t *testing.T) {
	caller := &wikiCoverageCaller{responses: map[string][]string{
		"wiki/remove_member": {`{"success":false,"errorMsg":"OWNER cannot be removed"}`},
	}}
	_, err := runWikiCoverageCLI(t, caller, "+member-remove", "--workspace", "w", "--users", "owner", "--yes")
	if err == nil || len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0].args["userIds"], []string{"owner"}) {
		t.Fatalf("owner rejection was swallowed, retried, or retargeted: err=%v calls=%#v", err, caller.calls)
	}
}
