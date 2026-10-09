// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageEmployeeDSHRegistrationProtocol(t *testing.T) {
	if _, err := runDigitalEmployeeDSHRegister(context.Background(), map[string]any{"invalid": make(chan int)}); err == nil {
		t.Fatal("unencodable registration accepted")
	}
	for _, tc := range []struct {
		name, body, want string
		ok               bool
	}{
		{"created", `printf '{"status":"created"}'`, "created", true},
		{"updated", `printf '{"status":"updated"}'`, "updated", true},
		{"unchanged", `printf '{"status":"unchanged"}'`, "unchanged", true},
		{"invalid status", `printf '{"status":"running"}'`, "", false},
		{"invalid json", `printf '{'`, "", false},
		{"failed process", `exit 7`, "", false},
		{"oversize", `printf '%070000d' 0`, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeShellExecutable(t, dir, "dsh-dingtalk", "cat >/dev/null\n"+tc.body+"\n")
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			got, err := runDigitalEmployeeDSHRegister(context.Background(), map[string]any{"agentUuid": "fixture-agent"})
			if (err == nil) != tc.ok || got != tc.want {
				t.Fatalf("status=%q err=%v", got, err)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeDSHControlIdentityAndCredentialBoundary(t *testing.T) {
	b := digitalEmployeeBinding{AgentUUID: "agent", DWSProfile: "corp:user", BindingRevision: 7}
	valid := `{"protocolVersion":1,"agentUuid":"agent","dwsProfile":"corp:user","bindingRevision":7,"released":true,"runtimeState":"stopped"}`
	for _, tc := range []struct {
		name, body string
		ok         bool
	}{
		{"valid", "printf '%s' '" + valid + "'", true},
		{"wrong identity", `printf '{"protocolVersion":1,"agentUuid":"other"}'`, false},
		{"malformed", `printf '{'`, false},
		{"failure", `exit 2`, false},
		{"oversize", `printf '%070000d' 0`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			body := "cat >/dev/null\n" + `test -z "$DWS_ACCESS_TOKEN$DWS_CLIENT_SECRET$DWS_CLIENT_ID$DWS_DUMP_RAW$EMPLOYEE_TEST_PASSWORD$EMPLOYEE_TEST_CREDENTIAL$EMPLOYEE_TEST_AUTH_CODE" || exit 9` + "\n" + tc.body + "\n"
			writeShellExecutable(t, dir, "dsh-dingtalk", body)
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			for _, key := range []string{"DWS_ACCESS_TOKEN", "DWS_CLIENT_SECRET", "DWS_CLIENT_ID", "DWS_DUMP_RAW", "EMPLOYEE_TEST_PASSWORD", "EMPLOYEE_TEST_CREDENTIAL", "EMPLOYEE_TEST_AUTH_CODE"} {
				t.Setenv(key, "test-secret")
			}
			got, err := runEmployeeDSHControl(nil, b, "stop")
			if (err == nil) != tc.ok {
				t.Fatalf("state=%+v err=%v", got, err)
			}
			if tc.ok && (!got.Released || got.RuntimeState != "stopped") {
				t.Fatalf("state=%+v", got)
			}
			if err != nil && strings.Contains(err.Error(), "test-secret") {
				t.Fatal("credentials leaked")
			}
		})
	}
}
