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

//go:build windows

package transport

import (
	"fmt"
	"net"

	winio "github.com/Microsoft/go-winio"
)

type pipeListener struct {
	l    net.Listener
	name string
}

func (p *pipeListener) Accept() (net.Conn, error) { return p.l.Accept() }
func (p *pipeListener) Endpoint() string          { return p.name }
func (p *pipeListener) Close() error              { return p.l.Close() }

// listen binds a Windows Named Pipe at the given name. Stale-cleanup is
// unnecessary on Windows because pipe names don't outlive the server
// process (no inode in the FS); a fresh ListenPipe just takes the name.
//
// SecurityDescriptor pinned to "D:P(A;;GA;;;OW)" (Discretionary ACL,
// Protected, Generic All for OWNER) — only the current user can connect.
func listen(name string) (Listener, error) {
	cfg := &winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;GA;;;OW)",
		MessageMode:        false,
		InputBufferSize:    int32(MaxFrameBytes),
		OutputBufferSize:   int32(MaxFrameBytes),
	}
	l, err := winio.ListenPipe(name, cfg)
	if err != nil {
		return nil, fmt.Errorf("transport: ListenPipe %s: %w", name, err)
	}
	return &pipeListener{l: l, name: name}, nil
}

func dial(name string) (net.Conn, error) {
	c, err := winio.DialPipe(name, nil)
	if err != nil {
		return nil, fmt.Errorf("transport: DialPipe %s: %w", name, err)
	}
	return c, nil
}
