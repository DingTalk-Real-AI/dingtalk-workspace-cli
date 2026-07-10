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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	eventlock "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/lock"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

const (
	StateFileName        = "personal_subscriptions.json"
	StateLockFileName    = "personal_subscriptions.lock"
	stateLockWaitTimeout = 5 * time.Second
	stateLockRetryDelay  = 25 * time.Millisecond
)

type RunState struct {
	SubscribeID  string    `json:"subscribe_id"`
	EventKey     string    `json:"event_key,omitempty"`
	RuleType     string    `json:"rule_type,omitempty"`
	ClientID     string    `json:"client_id,omitempty"`
	SourceID     string    `json:"source_id,omitempty"`
	IdentityHash string    `json:"identity_hash,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

func LoadRunStates(workDir string) ([]RunState, error) {
	path := filepath.Join(workDir, StateFileName)
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var states []RunState
	if err := json.Unmarshal(b, &states); err != nil {
		return nil, err
	}
	sort.Slice(states, func(i, j int) bool {
		return states[i].SubscribeID < states[j].SubscribeID
	})
	return states, nil
}

func UpsertRunState(workDir string, st RunState) error {
	if st.SubscribeID == "" {
		return nil
	}
	if st.CreatedAt.IsZero() {
		st.CreatedAt = time.Now().UTC()
	}
	return withRunStateLock(workDir, stateLockWaitTimeout, func() error {
		states, err := LoadRunStates(workDir)
		if err != nil {
			return err
		}
		replaced := false
		for i := range states {
			if states[i].SubscribeID == st.SubscribeID {
				states[i] = st
				replaced = true
				break
			}
		}
		if !replaced {
			states = append(states, st)
		}
		return writeRunStates(workDir, states)
	})
}

func RemoveRunStates(workDir string, subscribeIDs []string) error {
	if len(subscribeIDs) == 0 {
		return nil
	}
	remove := make(map[string]struct{}, len(subscribeIDs))
	for _, id := range subscribeIDs {
		if id != "" {
			remove[id] = struct{}{}
		}
	}
	return withRunStateLock(workDir, stateLockWaitTimeout, func() error {
		states, err := LoadRunStates(workDir)
		if err != nil {
			return err
		}
		filtered := states[:0]
		for _, st := range states {
			if _, ok := remove[st.SubscribeID]; !ok {
				filtered = append(filtered, st)
			}
		}
		return writeRunStates(workDir, filtered)
	})
}

func withRunStateLock(workDir string, wait time.Duration, fn func() error) error {
	if err := os.MkdirAll(workDir, config.DirPerm); err != nil {
		return err
	}
	lockPath := filepath.Join(workDir, StateLockFileName)
	deadline := time.Now().Add(wait)
	for {
		held, err := eventlock.TryAcquire(lockPath)
		if err == nil {
			defer held.Close()
			return fn()
		}
		if !errors.Is(err, eventlock.ErrBusy) {
			return err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("personal event: timed out waiting for run-state lock after %s", wait)
		}
		delay := stateLockRetryDelay
		if remaining < delay {
			delay = remaining
		}
		time.Sleep(delay)
	}
}

func writeRunStates(workDir string, states []RunState) error {
	if err := os.MkdirAll(workDir, config.DirPerm); err != nil {
		return err
	}
	sort.Slice(states, func(i, j int) bool {
		return states[i].SubscribeID < states[j].SubscribeID
	})
	path := filepath.Join(workDir, StateFileName)
	if len(states) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	b, err := json.MarshalIndent(states, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, config.FilePerm); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
