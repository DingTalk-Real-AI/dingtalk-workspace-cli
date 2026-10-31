package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtimecontext"
)

func TestCrossPlatformCoverageCompleteAgentObjectCrossesWorkerBoundary(t *testing.T) {
	raw := json.RawMessage(`{"agent":7,"match_tier":"high","os":"mac","execution_mode":"local","process":{"cmd":"` + strings.Repeat("private-process", 1000) + `"},"sandbox":{"detected":false},"collect_meta":{"arch":"arm64"},"unknown_future":{"value":[1,2,3]}}`)
	event := sampleEvent()
	event.AgentInfo = AgentInfoFromSnapshot(runtimecontext.AgentSnapshotForTest(raw, 0))
	var transferred []byte
	submit(event, time.Second, func(_ context.Context, payload []byte) error {
		transferred = append([]byte(nil), payload...)
		return nil
	})
	if len(transferred) <= maxEventBytes {
		t.Fatal("large object was dropped by the old 4 KiB limit")
	}
	err := runWorker(bytes.NewReader(transferred), func() (io.Closer, error) { return io.NopCloser(strings.NewReader("")), nil }, func(got Event) error {
		fields := sdkConfig(got).ExtraFields()
		if fields["c9"] != event.Path || fields["c10"] != event.Identity.CorpID {
			t.Fatal("existing attribution changed")
		}
		var ext struct {
			Agent json.RawMessage `json:"agentinfo"`
			State string          `json:"agentinfo_state"`
			Code  int32           `json:"agentinfo_error_code"`
		}
		if json.Unmarshal([]byte(fields["ext"]), &ext) != nil || ext.State != "ready" || ext.Code != 0 {
			t.Fatal("event-level collection status missing")
		}
		var before, after any
		if json.Unmarshal(raw, &before) != nil || json.Unmarshal(ext.Agent, &after) != nil {
			t.Fatal("invalid complete object")
		}
		b, _ := json.Marshal(before)
		a, _ := json.Marshal(after)
		if !bytes.Equal(a, b) {
			t.Fatal("SDK object was projected, truncated or double encoded")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageAgentIPCDegradesWithoutLosingBaseEvent(t *testing.T) {
	for _, tc := range []struct {
		name  string
		raw   json.RawMessage
		state runtimecontext.State
	}{
		{"oversized", json.RawMessage(`{"value":"` + strings.Repeat("x", runtimecontext.MaxAgentJSONBytes) + `"}`), runtimecontext.AgentOversized},
		{"invalid", json.RawMessage(`{"incomplete":`), runtimecontext.AgentInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := sampleEvent()
			event.AgentInfo = &AgentInfo{State: runtimecontext.StateReady, Result: tc.raw}
			launched := false
			submit(event, time.Second, func(_ context.Context, payload []byte) error {
				launched = true
				var got Event
				if json.Unmarshal(payload, &got) != nil || got.Path != event.Path || got.AgentInfo.State != tc.state || len(got.AgentInfo.Result) != 0 {
					t.Fatal("base event lost or invalid JSON retained")
				}
				return nil
			})
			if !launched {
				t.Fatal("SDK object failure dropped the base event")
			}
		})
	}
	event := sampleEvent()
	event.AgentInfo = AgentInfoFromSnapshot(runtimecontext.AgentSnapshotForTest([]byte(`{"agent":0,"match_tier":"none","os":"linux","execution_mode":"sandbox","process":{}}`), 2002))
	fields := sdkConfig(event).ExtraFields()
	if !strings.Contains(fields["ext"], `"agentinfo_error_code":2002`) || !strings.Contains(fields["ext"], `"agentinfo_state":"config_degraded"`) || !strings.Contains(fields["ext"], `"process":{}`) {
		t.Fatal("nonempty degraded result lost")
	}
	t.Setenv("DO_NOT_TRACK", "1")
	Submit(event)
	if err := runWorker(io.LimitReader(strings.NewReader(strings.Repeat("x", maxIPCBytes+1)), maxIPCBytes+1), func() (io.Closer, error) { t.Fatal("oversized IPC acquired slot"); return nil, nil }, func(Event) error { t.Fatal("oversized IPC sent"); return nil }); err == nil {
		t.Fatal("total IPC limit ignored")
	}
}

func TestCrossPlatformCoverageAgentHTMLCharactersDoNotInflateIPCLimit(t *testing.T) {
	raw := json.RawMessage(`{"agent":7,"match_tier":"high","process":{"cmd":"` + strings.Repeat("<", 240*1024) + `"}}`)
	event := sampleEvent()
	event.AgentInfo = AgentInfoFromSnapshot(runtimecontext.AgentSnapshotForTest(raw, 0))
	submit(event, time.Second, func(_ context.Context, payload []byte) error {
		sent := false
		err := runWorker(bytes.NewReader(payload), func() (io.Closer, error) { return io.NopCloser(strings.NewReader("")), nil }, func(got Event) error {
			sent = true
			if !bytes.Equal(got.AgentInfo.Result, raw) {
				t.Fatal("SDK string representation expanded in IPC")
			}
			return nil
		})
		if err != nil || !sent {
			t.Fatal("bounded complete SDK object rejected after IPC encoding", err)
		}
		return nil
	})
}

func TestCrossPlatformCoverageAgentTransportFailureBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  []byte
		err  error
	}{
		{"encoding failure", nil, errors.New("encoding failed")},
		{"IPC limit", make([]byte, maxIPCBytes+1), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testseam.Swap(t, &encodeHandoff, func(any) ([]byte, error) { return tc.raw, tc.err })
			submit(sampleEvent(), time.Second, func(context.Context, []byte) error { t.Error("failed payload launched a worker"); return nil })
		})
	}
	if _, err := encodeTelemetryJSON(make(chan int)); err == nil {
		t.Fatal("unencodable value accepted")
	}
	for _, info := range []*AgentInfo{
		{State: runtimecontext.StateReady, ErrorCategory: strings.Repeat("x", 129)},
		{State: runtimecontext.StateReady, Result: make([]byte, runtimecontext.MaxAgentJSONBytes+1)},
		{State: "unrecognized"},
	} {
		event := sampleEvent()
		event.AgentInfo = info
		if validEvent(event) {
			t.Fatal("invalid runtime metadata accepted by worker")
		}
	}
	t.Setenv("DO_NOT_TRACK", "")
	var attempts atomic.Int32
	testseam.Swap(t, &findExecutable, func() (string, error) { attempts.Add(1); return "", errors.New("no executable") })
	event := sampleEvent()
	event.AgentInfo = AgentInfoFromSnapshot(runtimecontext.AgentSnapshotForTest([]byte(`{"agent":7,"match_tier":"high","extra":"`+strings.Repeat("x", maxEventBytes)+`"}`), 0))
	Submit(event)
	if attempts.Load() != 1 {
		t.Fatal("large valid snapshot did not reach bounded handoff")
	}
	t.Setenv("DO_NOT_TRACK", "1")
	Submit(event)
	if attempts.Load() != 1 {
		t.Fatal("opt out launched a worker")
	}
}
