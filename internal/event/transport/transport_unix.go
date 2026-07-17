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

//go:build !windows

package transport

import (
	"fmt"
	"net"
	"os"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

type unixListener struct {
	l    net.Listener
	path string
}

var (
	statSocket   = os.Stat
	removeSocket = os.Remove
	listenUnix   = net.Listen
	chmodSocket  = os.Chmod
)

func (u *unixListener) Accept() (net.Conn, error) { return u.l.Accept() }
func (u *unixListener) Endpoint() string          { return u.path }
func (u *unixListener) Close() error {
	err := u.l.Close()
	// Best-effort unlink. Ignored errors here because the bus may have
	// already been replaced by a competing bus that unlinked first.
	_ = removeSocket(u.path)
	return err
}

// checkSocketPath rejects paths over the sun_path budget up front so the
// caller sees the actual problem instead of the syscall's bare EINVAL
// ("invalid argument").
func checkSocketPath(path string) error {
	if max := dwsevent.MaxUnixSocketPath(); len(path) > max {
		return fmt.Errorf("transport: unix socket path too long (%d > %d bytes): %s", len(path), max, path)
	}
	return nil
}

func listen(path string) (Listener, error) {
	if err := checkSocketPath(path); err != nil {
		return nil, err
	}
	// Stale socket cleanup. Caller holds bus.lock so this is race-safe.
	if _, err := statSocket(path); err == nil {
		if err := removeSocket(path); err != nil {
			return nil, fmt.Errorf("transport: remove stale socket %s: %w", path, err)
		}
	}
	l, err := listenUnix("unix", path)
	if err != nil {
		return nil, fmt.Errorf("transport: listen %s: %w", path, err)
	}
	if err := chmodSocket(path, config.FilePerm); err != nil {
		_ = l.Close()
		_ = removeSocket(path)
		return nil, fmt.Errorf("transport: chmod %s: %w", path, err)
	}
	return &unixListener{l: l, path: path}, nil
}

func dial(path string) (net.Conn, error) {
	if err := checkSocketPath(path); err != nil {
		return nil, err
	}
	return net.Dial("unix", path)
}
