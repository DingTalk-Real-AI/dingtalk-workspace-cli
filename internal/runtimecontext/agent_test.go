package runtimecontext

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCrossPlatformCoverageAgentSnapshotOutcomes(t *testing.T) {
	matched := []byte(`{"agent":7,"match_tier":"high","os":"mac","execution_mode":"local","process":{"cmd":"private-command"},"future":{"signal":42}}`)
	for _, tc := range []struct {
		name     string
		raw      []byte
		code     int32
		err      error
		state    State
		id       int64
		retained bool
	}{
		{"matched", matched, 0, nil, StateReady, 7, true},
		{"unknown", []byte(`{"agent":0,"match_tier":"none","os":"linux","execution_mode":"sandbox"}`), 0, nil, AgentUnknown, 0, true},
		{"config open", matched, 2002, nil, AgentDegraded, 0, true},
		{"config read", matched, 2003, nil, AgentDegraded, 0, true},
		{"config format", matched, 2016, nil, AgentDegraded, 0, true},
		{"failure", nil, 1004, nil, StateError, 0, false},
		{"failure with object", matched, 1004, nil, StateError, 0, true},
		{"invalid", []byte(`{"agent":7.5}`), 0, nil, AgentInvalid, 0, false},
		{"missing ID", []byte(`{"match_tier":"none"}`), 0, nil, AgentInvalid, 0, false},
		{"native failure", nil, 0, errNativeLoad, StateError, 0, false},
		{"copy oversized", nil, 0, errAgentTooLarge, AgentOversized, 0, false},
		{"json oversized", bytes.Repeat([]byte("x"), MaxAgentJSONBytes+1), 0, nil, AgentOversized, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var snapshot AgentSnapshot
			if tc.err != nil {
				snapshot = parseAgentResult(tc.raw, tc.code, tc.err)
			} else {
				// 同包验证公开测试入口，避免依赖跨包插桩才计入覆盖率。
				snapshot = AgentSnapshotForTest(tc.raw, tc.code)
			}
			if snapshot.State != tc.state || snapshot.AgentID != tc.id || snapshot.ErrorCode != tc.code || (len(snapshot.JSON()) > 0) != tc.retained {
				t.Fatalf("snapshot metadata = %s id=%d", snapshot, snapshot.AgentID)
			}
			if strings.Contains(fmt.Sprintf("%#v", snapshot), "private-command") {
				t.Fatal("snapshot formatting leaked SDK object")
			}
			encoded, _ := json.Marshal(snapshot)
			if bytes.Contains(encoded, []byte("private-command")) {
				t.Fatal("snapshot serialization leaked SDK object")
			}
			if tc.retained {
				copy := snapshot.JSON()
				copy[0] = '!'
				if !json.Valid(snapshot.JSON()) {
					t.Fatal("caller mutated the cached object")
				}
			}
		})
	}
}

func TestCrossPlatformCoverageAgentOnceAndLateResult(t *testing.T) {
	var calls atomic.Int32
	release, completed := make(chan struct{}), make(chan struct{})
	p := &agentProvider{locate: successfulLocator, timeout: 5 * time.Millisecond, collect: func(string, string) ([]byte, int32, error) {
		calls.Add(1)
		<-release
		defer close(completed)
		return []byte(`{"agent":7,"match_tier":"high"}`), 0, nil
	}}
	var wait sync.WaitGroup
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if p.resolve().State != StateTimeout {
				t.Error("timeout snapshot changed")
			}
		}()
	}
	wait.Wait()
	close(release)
	<-completed
	if got := p.resolve(); got.State != StateTimeout || got.AgentID != 0 || len(got.JSON()) != 0 || calls.Load() != 1 {
		t.Fatal("late result replaced snapshot or collection repeated")
	}
	for _, locate := range []libraryLocator{
		func() (string, error) { return "", errors.New("missing") },
		func() (string, error) { panic("private error") },
	} {
		failed := &agentProvider{locate: locate, timeout: time.Second}
		if got := failed.resolve(); got.State != StateUnavailable && got.State != StateError {
			t.Fatal("locator failure escaped")
		}
	}
}

func TestCrossPlatformCoverageAgentAndUMIDIndependentHeaders(t *testing.T) {
	raw := []byte(`{"agent":7,"match_tier":"high","os":"mac","execution_mode":"local","process":{"pid":42}}`)
	for _, token := range []string{"device-value", ""} {
		result := ResultWithAgentForTest(token, raw, 0)
		header, ok := result.HeaderValue()
		if !ok {
			t.Fatal("header missing")
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal([]byte(header), &fields) != nil || string(fields["agent_id"]) != "7" || string(fields["os"]) != `"mac"` {
			t.Fatal("SDK integer or original dimensions missing")
		}
		if (len(fields["umid"]) > 0) != (token != "") {
			t.Fatal("UMID availability coupled to Agent")
		}
		link, _ := result.AttachToURL("https://example.com/auth?agentCode=old")
		if strings.Contains(link, "agent_id") || strings.Contains(link, "execution_mode") || strings.Contains(link, "os=") || !strings.Contains(link, "agentCode=old") {
			t.Fatal("SDK metadata entered authorization URL")
		}
	}
	result := ReadyResultForTest("device-value")
	result.agent = AgentSnapshot{State: StateTimeout}
	if header, _ := result.HeaderValue(); header != `{"umid":"device-value","agent_id":0}` {
		t.Fatal("Agent failure removed UMID")
	}
	result = ReadyResultForTest(strings.Repeat("x", maxHeaderBytes-len(`{"umid":""}`)))
	result.agent = parseAgentResult(raw, 0, nil)
	header, ok := result.HeaderValue()
	if !ok || len(header) > maxHeaderBytes || !strings.Contains(header, result.token) {
		t.Fatal("header limit discarded valid UMID")
	}
}

func TestCrossPlatformCoverageAgentAndUMIDShareLibraryHandle(t *testing.T) {
	var opens atomic.Int32
	open := func(string) (uintptr, error) { opens.Add(1); return 42, nil }
	var wait sync.WaitGroup
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			handle, err := sharedNativeLibrary(t.Name(), open)
			if err != nil || handle != 42 {
				t.Error("shared library handle changed")
			}
		}()
	}
	wait.Wait()
	if opens.Load() != 1 {
		t.Fatal("Agent and UMID opened separate copies of the library")
	}
}
