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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

type unixListener struct {
	l    net.Listener
	path string
}

func (u *unixListener) Accept() (net.Conn, error) { return u.l.Accept() }
func (u *unixListener) Endpoint() string          { return u.path }
func (u *unixListener) Close() error {
	err := u.l.Close()
	// Best-effort unlink. Ignored errors here because the bus may have
	// already been replaced by a competing bus that unlinked first.
	_ = os.Remove(u.path)
	return err
}

func listen(path string) (Listener, error) {
	// Stale socket cleanup. Caller holds bus.lock so this is race-safe.
	if _, err := os.Stat(path); err == nil {
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("transport: remove stale socket %s: %w", path, err)
		}
	}
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("transport: listen %s: %w", path, err)
	}
	if err := os.Chmod(path, config.FilePerm); err != nil {
		_ = l.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("transport: chmod %s: %w", path, err)
	}
	return &unixListener{l: l, path: path}, nil
}

func dial(path string) (net.Conn, error) {
	return net.Dial("unix", path)
}
