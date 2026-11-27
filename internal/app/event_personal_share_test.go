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
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/consume"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/lock"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoveragePersonalSubscriptionLocalFailures(t *testing.T) {
	for _, failure := range []string{"guard", "retain", "guard-close"} {
		t.Run(failure, func(t *testing.T) {
			t.Setenv("DWS_CONFIG_DIR", t.TempDir())
			recording := &personalRecordingAttemptStore{}
			testseam.Swap(t, &personalNewSubscriptionAttemptStore, func(string) personalSubscriptionAttemptStore { return recording })
			testseam.Swap(t, &personalResolveEventIdentity, func(context.Context, string, string, ...personalIdentityOptions) (personal.Identity, error) {
				return personal.Identity{AccessToken: "fixture", ClientID: "fixture", SourceID: "open", LocalSubject: "fixture"}, nil
			})
			creates := 0
			testseam.Swap(t, &personalEnsureSubscription, func(context.Context, *personal.Client, personal.Identity, personalConsumeOptions) (*personal.Subscription, string, string, error) {
				creates++
				return &personal.Subscription{SubscribeID: "fixture"}, personal.EventMention, "at", nil
			})
			testseam.Swap(t, &personalValidateConsumeConfig, func(consume.Config) error { return nil })
			boom := errors.New("fixture local failure")
			switch failure {
			case "guard":
				testseam.Swap(t, &personalAcquireSubscriptionGuard, func(context.Context, string) (*lock.File, error) { return nil, boom })
			case "retain":
				testseam.Swap(t, &personalRetainSubscription, func(string, string, bool) (*personalSubscriptionShare, error) { return nil, boom })
			case "guard-close":
				testseam.Swap(t, &personalAcquireSubscriptionGuard, func(ctx context.Context, work string) (*lock.File, error) {
					guard, err := personalSubscriptionGuard(ctx, work)
					if err == nil {
						err = guard.File().Close()
					}
					return guard, err
				})
			}
			err := runPersonalEventConsumeSingle(newPersonalCoverageCommand(), personalConsumeOptions{EventKey: personal.EventMention, ControlBaseURL: "https://mcp.example.test/dws"})
			if err == nil {
				t.Fatal("local failure was discarded")
			}
			if failure == "guard" {
				if creates != 0 || recording.claimCalls != 0 {
					t.Fatal("failed guard left a create reservation")
				}
			} else if creates != 1 || recording.failureCalls != 1 {
				t.Fatalf("create=%d recorded failure=%d", creates, recording.failureCalls)
			}
		})
	}
}

func TestCrossPlatformCoveragePersonalSubscriptionGuardFailures(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := personalSubscriptionGuard(context.Background(), file); err == nil {
		t.Fatal("non-directory accepted")
	}
	work := t.TempDir()
	guard, err := personalSubscriptionGuard(context.Background(), work)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Millisecond)
	defer cancel()
	if _, err := personalSubscriptionGuard(ctx, work); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait error=%v", err)
	}
}

func TestCrossPlatformCoveragePersonalSubscriptionLeaseFailures(t *testing.T) {
	for _, failure := range []string{"retain-lock", "marker-write", "release-guard", "lease-close", "final-lock", "marker-stat"} {
		t.Run(failure, func(t *testing.T) {
			work := t.TempDir()
			share, err := retainPersonalSubscription(work, "sub", true)
			if err != nil {
				t.Fatal(err)
			}
			defer share.lease.Close()
			switch failure {
			case "retain-lock", "marker-write":
				share.lease.Close()
				path := share.path + ".lock"
				if failure == "marker-write" {
					path = share.path + ".owned"
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
				if _, err := retainPersonalSubscription(work, "sub", true); err == nil {
					t.Fatal("retain discarded filesystem failure")
				}
				return
			case "release-guard":
				if err := os.Mkdir(filepath.Join(work, "subscription-lifecycle.lock"), 0700); err != nil {
					t.Fatal(err)
				}
			case "lease-close":
				if err := share.lease.File().Close(); err != nil {
					t.Fatal(err)
				}
			case "final-lock":
				// Release the OS handle first: Windows disallows unlinking open files.
				if err := share.lease.Close(); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(share.path + ".lock"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(share.path+".lock", 0700); err != nil {
					t.Fatal(err)
				}
			case "marker-stat":
				testseam.Swap(t, &personalSubscriptionStat, func(string) (os.FileInfo, error) { return nil, os.ErrPermission })
			}
			deletes := 0
			if err := share.release(context.Background(), func() error { deletes++; return nil }); err == nil {
				t.Fatal("release discarded filesystem failure")
			}
			if deletes != 0 {
				t.Fatal("deleted subscription despite failed ownership check")
			}
		})
	}
}

func TestCrossPlatformCoveragePersonalSubscriptionCommandSharing(t *testing.T) {
	for _, first := range []int{0, 1} {
		t.Run(fmt.Sprintf("exit-%d-first", first), func(t *testing.T) {
			t.Setenv("DWS_CONFIG_DIR", t.TempDir())
			testseam.Swap(t, &personalNewSubscriptionAttemptStore, func(string) personalSubscriptionAttemptStore { return personalNoopAttemptStore{} })
			testseam.Swap(t, &personalResolveEventIdentity, func(context.Context, string, string, ...personalIdentityOptions) (personal.Identity, error) {
				return personal.Identity{AccessToken: "fixture", ClientID: "fixture", SourceID: "open", LocalSubject: "fixture"}, nil
			})
			testseam.Swap(t, &personalEnsureSubscription, func(context.Context, *personal.Client, personal.Identity, personalConsumeOptions) (*personal.Subscription, string, string, error) {
				return &personal.Subscription{SubscribeID: "shared-fixture"}, personal.EventMention, "at", nil
			})
			testseam.Swap(t, &personalValidateConsumeConfig, func(consume.Config) error { return nil })
			testseam.Swap(t, &personalUpsertRunState, func(string, personal.RunState) error { return nil })
			testseam.Swap(t, &personalRemoveRunStates, func(string, []string) error { return nil })
			var deletes, runs atomic.Int32
			var deleteErr error
			if first == 1 {
				deleteErr = errors.New("fixture cleanup unavailable")
			}
			testseam.Swap(t, &personalDeleteSubscription, func(*personal.Client, context.Context, string) error { deletes.Add(1); return deleteErr })
			ready := make(chan int, 2)
			stops := []chan struct{}{make(chan struct{}), make(chan struct{})}
			testseam.Swap(t, &personalConsumeRun, func(context.Context, consume.Config) error {
				index := int(runs.Add(1)) - 1
				ready <- index
				<-stops[index]
				return nil
			})
			results := []chan error{make(chan error, 1), make(chan error, 1)}
			stderr := [2]bytes.Buffer{}
			for i := range results {
				go func(i int) {
					cmd := newPersonalCoverageCommand()
					cmd.SetErr(&stderr[i])
					results[i] <- runPersonalEventConsumeSingle(cmd, personalConsumeOptions{EventKey: personal.EventMention, ControlBaseURL: "https://mcp.example.test/dws"})
				}(i)
				if got := <-ready; got != i {
					t.Fatalf("ready=%d want=%d", got, i)
				}
			}
			close(stops[first])
			if err := <-results[first]; err != nil {
				t.Fatal(err)
			}
			if deletes.Load() != 0 {
				t.Fatal("first exit deleted active shared subscription")
			}
			close(stops[1-first])
			if err := <-results[1-first]; !errors.Is(err, deleteErr) {
				t.Fatalf("cleanup error=%v want=%v", err, deleteErr)
			}
			if deleteErr != nil && !strings.Contains(stderr[1-first].String(), deleteErr.Error()) {
				t.Fatal("cleanup failure missing from stderr")
			}
			if deletes.Load() != 1 {
				t.Fatalf("final exit deletes=%d", deletes.Load())
			}
		})
	}
}

func TestCrossPlatformCoveragePersonalSubscriptionExitOrders(t *testing.T) {
	for _, owned := range [][2]bool{{true, true}, {true, false}, {false, true}, {false, false}} {
		for _, first := range []int{0, 1} {
			work := t.TempDir()
			guard, err := personalSubscriptionGuard(context.Background(), work)
			if err != nil {
				t.Fatal(err)
			}
			var shares [2]*personalSubscriptionShare
			for i := range shares {
				shares[i], err = retainPersonalSubscription(work, "same-sub", owned[i])
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := guard.Close(); err != nil {
				t.Fatal(err)
			}
			deletes := 0
			remove := func() error { deletes++; return nil }
			if err := shares[first].release(context.Background(), remove); err != nil {
				t.Fatal(err)
			}
			if deletes != 0 {
				t.Fatal("subscription deleted while another consumer is using it")
			}
			if err := shares[1-first].release(context.Background(), remove); err != nil {
				t.Fatal(err)
			}
			want := 0
			if owned[0] || owned[1] {
				want = 1
			}
			if deletes != want {
				t.Fatalf("ownership=%v, first=%d, deletes=%d want=%d", owned, first, deletes, want)
			}
		}
	}
}

func TestCrossPlatformCoveragePersonalSubscriptionStartDuringDelete(t *testing.T) {
	work := t.TempDir()
	guard, err := personalSubscriptionGuard(context.Background(), work)
	if err != nil {
		t.Fatal(err)
	}
	share, err := retainPersonalSubscription(work, "sub", true)
	if err != nil {
		t.Fatal(err)
	}
	guard.Close()
	deleting, finishDelete, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		finished <- share.release(context.Background(), func() error {
			close(deleting)
			<-finishDelete
			return nil
		})
	}()
	<-deleting
	// A new ensure/create cannot enter its critical section during final deletion.
	probe, err := lock.TryAcquire(filepath.Join(work, "subscription-lifecycle.lock"))
	if probe != nil {
		probe.Close()
	}
	if !errors.Is(err, lock.ErrBusy) {
		t.Fatalf("creation was not serialized: %v", err)
	}
	close(finishDelete)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	guard, err = personalSubscriptionGuard(context.Background(), work)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	if _, err := os.Stat(share.path + ".owned"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ownership marker after final deletion: %v", err)
	}
}

func TestCrossPlatformCoveragePersonalSubscriptionCleanupFailure(t *testing.T) {
	work := t.TempDir()
	guard, err := personalSubscriptionGuard(context.Background(), work)
	if err != nil {
		t.Fatal(err)
	}
	share, err := retainPersonalSubscription(work, "sub", true)
	if err != nil {
		t.Fatal(err)
	}
	guard.Close()
	want := errors.New("control-plane deletion failed")
	calls := 0
	err = share.release(context.Background(), func() error { calls++; return want })
	if !errors.Is(err, want) || calls != 1 {
		t.Fatalf("got %v, calls=%d", err, calls)
	}
	if _, err := os.Stat(share.path + ".owned"); err != nil {
		t.Fatalf("lost cleanup failure state: %v", err)
	}
}
