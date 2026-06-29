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

package helpers

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// connectLockDir is a var so tests can isolate lock files.
var connectLockDir = os.TempDir()

// acquireConnectLock guards against two local connectors holding Stream
// connections for the same robot. DingTalk load-balances pushes across
// connections sharing one clientId, so a duplicate connector makes the bot
// answer intermittently — the hardest-to-diagnose failure mode this command
// has (verified live). The lock is a pid file; a stale file whose owning
// process is gone is taken over silently.
func acquireConnectLock(clientID string) (release func(), err error) {
	path := filepath.Join(connectLockDir, "dws-connect-"+sanitizeLockID(clientID)+".pid")
	for attempt := 0; attempt < 2; attempt++ {
		f, openErr := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if openErr == nil {
			fmt.Fprintf(f, "%d", os.Getpid())
			f.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !os.IsExist(openErr) {
			return nil, openErr
		}
		raw, readErr := os.ReadFile(path)
		if readErr == nil {
			if pid, perr := strconv.Atoi(strings.TrimSpace(string(raw))); perr == nil && pid > 0 && processAlive(pid) {
				return nil, fmt.Errorf("机器人 %s 已有连接器在本机运行 (pid %d)。同一机器人的多个 Stream 连接会随机分流消息，表现为机器人时灵时不灵；请先停掉旧进程，或确认后删除 %s", clientID, pid, path)
			}
		}
		// Stale lock (owner gone or file unreadable): take over.
		_ = os.Remove(path)
	}
	return nil, fmt.Errorf("无法获取连接器锁 %s", path)
}

// sanitizeLockID keeps the lock filename filesystem-safe.
func sanitizeLockID(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		}
		return '_'
	}, s)
}

// processAlive reports whether pid refers to a live process (unix signal-0
// probe; a permission error still means "alive").
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	return errors.Is(err, syscall.EPERM)
}
