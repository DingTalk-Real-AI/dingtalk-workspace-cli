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
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/lock"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

var (
	personalAcquireSubscriptionGuard = personalSubscriptionGuard
	personalRetainSubscription       = retainPersonalSubscription
	personalSubscriptionStat         = os.Stat
)

// Creation and final deletion share this identity-scoped mutex. Hold it until
// the new consumer has acquired its subscription lease, before starting IPC.
// Waiting for a local mutex does not retry any control-plane operation.
func personalSubscriptionGuard(ctx context.Context, workDir string) (*lock.File, error) {
	if err := os.MkdirAll(workDir, config.DirPerm); err != nil {
		return nil, err
	}
	for {
		guard, err := lock.TryAcquire(filepath.Join(workDir, "subscription-lifecycle.lock"))
		if !errors.Is(err, lock.ErrBusy) {
			return guard, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

type personalSubscriptionShare struct {
	workDir string
	path    string
	lease   *lock.File
}

// Caller must hold personalSubscriptionGuard. All participating consumers,
// including explicit --subscribe-id borrowers, keep a shared kernel lease.
// Ownership is a marker rather than a PID, so an owner may exit first without
// losing the final consumer's responsibility to remove an owned subscription.
func retainPersonalSubscription(workDir, subscribeID string, owned bool) (*personalSubscriptionShare, error) {
	path := filepath.Join(workDir, fmt.Sprintf("subscription-%x", sha256.Sum256([]byte(subscribeID))))
	lease, err := lock.TryAcquireShared(path + ".lock")
	if err != nil {
		return nil, err
	}
	if owned {
		if err := os.WriteFile(path+".owned", nil, config.FilePerm); err != nil {
			_ = lease.Close()
			return nil, err
		}
	}
	return &personalSubscriptionShare{workDir: workDir, path: path, lease: lease}, nil
}

func (s *personalSubscriptionShare) release(ctx context.Context, remove func() error) error {
	defer s.lease.Close()
	guard, err := personalSubscriptionGuard(ctx, s.workDir)
	if err != nil {
		return err
	}
	defer guard.Close()
	if err := s.lease.Close(); err != nil {
		return err
	}
	last, err := lock.TryAcquire(s.path + ".lock")
	if errors.Is(err, lock.ErrBusy) {
		return nil // Other consumers still own a live lease; retain the subscription.
	}
	if err != nil {
		return err
	}
	defer last.Close()
	if _, err := personalSubscriptionStat(s.path + ".owned"); errors.Is(err, os.ErrNotExist) {
		return nil // Only explicit borrowers: external owner retains its subscription.
	} else if err != nil {
		return err
	}
	if err := remove(); err != nil {
		return err // Preserve ownership marker and report the actual cleanup failure.
	}
	return os.Remove(s.path + ".owned")
}
