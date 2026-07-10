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

package personal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	eventlock "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/lock"
)

func TestRunStatesConcurrentUpsertPreservesEverySubscription(t *testing.T) {
	workDir := t.TempDir()
	const count = 64
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- UpsertRunState(workDir, RunState{SubscribeID: fmt.Sprintf("sub-%03d", i)})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("UpsertRunState() error = %v", err)
		}
	}

	states, err := LoadRunStates(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != count {
		t.Fatalf("run states = %d, want %d", len(states), count)
	}
	for i, st := range states {
		if want := fmt.Sprintf("sub-%03d", i); st.SubscribeID != want {
			t.Fatalf("state %d = %q, want %q", i, st.SubscribeID, want)
		}
	}
}

func TestRunStatesConcurrentUpsertAndRemoveRemainConsistent(t *testing.T) {
	workDir := t.TempDir()
	for i := 0; i < 50; i++ {
		if err := UpsertRunState(workDir, RunState{SubscribeID: fmt.Sprintf("base-%02d", i)}); err != nil {
			t.Fatal(err)
		}
	}

	var wg sync.WaitGroup
	errs := make(chan error, 75)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- UpsertRunState(workDir, RunState{SubscribeID: fmt.Sprintf("new-%02d", i)})
		}(i)
	}
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- RemoveRunStates(workDir, []string{fmt.Sprintf("base-%02d", i)})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("run-state mutation error = %v", err)
		}
	}

	raw, err := os.ReadFile(filepath.Join(workDir, StateFileName))
	if err != nil {
		t.Fatal(err)
	}
	var decoded []RunState
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("state file is invalid JSON: %v", err)
	}
	if len(decoded) != 75 {
		t.Fatalf("run states = %d, want 75", len(decoded))
	}
	present := make(map[string]bool, len(decoded))
	for _, st := range decoded {
		present[st.SubscribeID] = true
	}
	for i := 25; i < 50; i++ {
		if !present[fmt.Sprintf("base-%02d", i)] {
			t.Fatalf("unremoved base subscription %d was lost", i)
		}
	}
	for i := 0; i < 50; i++ {
		if !present[fmt.Sprintf("new-%02d", i)] {
			t.Fatalf("new subscription %d was lost", i)
		}
	}
}

func TestWithRunStateLockTimesOut(t *testing.T) {
	workDir := t.TempDir()
	held, err := eventlock.TryAcquire(filepath.Join(workDir, StateLockFileName))
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()

	called := false
	err = withRunStateLock(workDir, 20*time.Millisecond, func() error {
		called = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "timed out waiting for run-state lock") {
		t.Fatalf("withRunStateLock() error = %v, want timeout", err)
	}
	if called {
		t.Fatal("locked callback was called")
	}
}
