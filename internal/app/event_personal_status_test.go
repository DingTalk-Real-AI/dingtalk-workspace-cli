// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/busctl"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

func TestRenderPersonalStatusTextShowsConsumersWithoutSubscriptions(t *testing.T) {
	var out bytes.Buffer
	renderPersonalStatusText(&out, personal.Identity{
		CorpID:   "corp-1",
		UserID:   "user-1",
		ClientID: "client-1",
		SourceID: "source-1",
	}, "identity-hash-1", nil, busctl.EntryStatus{
		Entry: busctl.BusEntry{
			WorkDir:   "wd",
			State:     busctl.BusStateRunning,
			HolderPID: 100,
		},
		Live: &transport.StatusResp{
			Consumers: []transport.StatusConsumer{
				{
					PID:         12345,
					EventTypes:  []string{"user_im_message_receive_o2o"},
					SubscribeID: "subId-1",
					Filter:      "content",
					Received:    3,
					Dropped:     1,
				},
				{
					PID:      12346,
					Received: 5,
				},
			},
		},
	})
	got := out.String()
	for _, want := range []string{
		"Subscriptions: none",
		"Consumers:",
		"PID",
		"EVENT_KEYS",
		"SUBSCRIBE_ID",
		"RECEIVED",
		"DROPPED",
		"12345",
		"user_im_message_receive_o2o",
		"subId-1",
		"content",
		"3",
		"1",
		"(catch-all)",
		"-",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("status output missing %q:\n%s", want, got)
		}
	}
}

func TestRenderPersonalStatusTextConsumersUnavailableWhenRPCFails(t *testing.T) {
	var out bytes.Buffer
	renderPersonalStatusText(&out, personal.Identity{ClientID: "client-1", SourceID: "source-1"}, "identity-hash-1", nil, busctl.EntryStatus{
		Entry: busctl.BusEntry{
			WorkDir:   "wd",
			State:     busctl.BusStateRunning,
			HolderPID: 100,
		},
	})
	if got := out.String(); !strings.Contains(got, "Consumers: unavailable (status RPC failed)") {
		t.Fatalf("status output = %q, want unavailable consumers", got)
	}
}

func TestRenderPersonalStatusTextConsumersNoneWhenBusNotRunning(t *testing.T) {
	var out bytes.Buffer
	renderPersonalStatusText(&out, personal.Identity{ClientID: "client-1", SourceID: "source-1"}, "identity-hash-1", nil, busctl.EntryStatus{
		Entry: busctl.BusEntry{
			WorkDir: "wd",
			State:   busctl.BusStateNotRunning,
		},
	})
	if got := out.String(); !strings.Contains(got, "Consumers: none") {
		t.Fatalf("status output = %q, want no consumers", got)
	}
}
