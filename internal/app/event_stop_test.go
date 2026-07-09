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
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
)

func TestEventStopHelpDescribesPersonalSubscription(t *testing.T) {
	cmd := newEventStopCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"stop [subscribe_id]",
		"取消个人事件订阅并停止本地消费",
		"取消个人事件订阅并停止本地消费，清理对应本地消费状态",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("help missing %q:\n%s", want, got)
		}
	}
	for _, stale := range []string{"优雅停止 bus 守护进程", strings.Join([]string{"--as", "app"}, " "), "应用事件"} {
		if strings.Contains(got, stale) {
			t.Fatalf("help still contains stale public app wording %q:\n%s", stale, got)
		}
	}
}

func TestEventStopRequiresSubscribeIDOrAll(t *testing.T) {
	cmd := newEventStopCommand()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "subscribe_id is required unless --all is set") {
		t.Fatalf("Execute() error = %v, want subscribe_id requirement", err)
	}
}

func TestEventStopSubscribeIDAndAllAreMutuallyExclusive(t *testing.T) {
	cmd := newEventStopCommand()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{"subId-1", "--all"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "subscribe_id and --all are mutually exclusive") {
		t.Fatalf("Execute() error = %v, want mutual exclusion", err)
	}
}

func TestEventStopAsAppRejectsSubscribeID(t *testing.T) {
	cmd := newEventStopCommand()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{"--as", "app", "subId-1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "app event is not publicly available yet") {
		t.Fatalf("Execute() error = %v, want public availability guard", err)
	}
}

func TestPersonalStopTargets(t *testing.T) {
	workDir := t.TempDir()
	if err := personal.UpsertRunState(workDir, personal.RunState{SubscribeID: "sub-b"}); err != nil {
		t.Fatalf("UpsertRunState() error = %v", err)
	}
	if err := personal.UpsertRunState(workDir, personal.RunState{SubscribeID: "sub-a"}); err != nil {
		t.Fatalf("UpsertRunState() error = %v", err)
	}

	got, err := personalStopTargets(workDir, "sub-explicit", false)
	if err != nil {
		t.Fatalf("personalStopTargets(explicit) error = %v", err)
	}
	if want := []string{"sub-explicit"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("explicit targets = %#v, want %#v", got, want)
	}

	got, err = personalStopTargets(workDir, "", true)
	if err != nil {
		t.Fatalf("personalStopTargets(all) error = %v", err)
	}
	if want := []string{"sub-a", "sub-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("all targets = %#v, want %#v", got, want)
	}

	if _, err := personalStopTargets(workDir, "", false); err == nil || !strings.Contains(err.Error(), "subscribe_id is required unless --all is set") {
		t.Fatalf("personalStopTargets(no target) error = %v, want required error", err)
	}
	if _, err := personalStopTargets(workDir, "sub-explicit", true); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("personalStopTargets(explicit+all) error = %v, want mutual exclusion", err)
	}
}

func TestPrintPersonalStopResult(t *testing.T) {
	var out bytes.Buffer
	printPersonalStopResult(&out, []string{"sub-1"}, true, "personal bus stopped")
	if got := out.String(); got != "cancelled personal subscription sub-1; personal bus stopped\n" {
		t.Fatalf("single output = %q", got)
	}

	out.Reset()
	printPersonalStopResult(&out, []string{"sub-1", "sub-2"}, false, "personal bus still running")
	if got := out.String(); got != "cancelled 2 personal subscription(s); personal bus still running\n" {
		t.Fatalf("multi output = %q", got)
	}
}
