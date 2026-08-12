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
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

type unixListener struct {
	l    net.Listener
	path string
}

var (
	statSocket            = os.Stat
	removeSocket          = os.Remove
	listenUnix            = net.Listen
	chmodSocket           = os.Chmod
	lstatSocketPath       = os.Lstat
	statSocketRuntimeRoot = os.Stat
	mkdirSocketDir        = os.Mkdir
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

// ensureSocketDir makes the socket's immediate parent an owner-only
// directory and rejects unsafe pre-existing paths. The parent of that
// directory must itself either be private to the effective user (for
// XDG_RUNTIME_DIR and macOS temporary roots) or sticky (for Linux /tmp), so
// another user cannot rename the private directory out from under us.
func ensureSocketDir(path string, create bool) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("transport: unix socket path must be absolute: %s", path)
	}
	dir := filepath.Dir(path)
	root := filepath.Dir(dir)
	rootInfo, err := lstatSocketPath(root)
	if err != nil {
		return fmt.Errorf("transport: inspect socket runtime root %s: %w", root, err)
	}
	// macOS exposes the system /tmp as a root-owned symlink to /private/tmp.
	// Follow only that well-known alias, then apply the same ownership/sticky
	// validation to its target. Arbitrary runtime-root symlinks remain rejected.
	if rootInfo.Mode()&os.ModeSymlink != 0 && filepath.Clean(root) == "/tmp" {
		rootInfo, err = statSocketRuntimeRoot(root)
		if err != nil {
			return fmt.Errorf("transport: resolve socket runtime root %s: %w", root, err)
		}
	}
	if err := validateSocketRuntimeRoot(root, rootInfo, uint32(os.Geteuid())); err != nil {
		return err
	}
	if create {
		if err := mkdirSocketDir(dir, config.DirPerm); err != nil && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("transport: create socket directory %s: %w", dir, err)
		}
	}
	dirInfo, err := lstatSocketPath(dir)
	if err != nil {
		return fmt.Errorf("transport: inspect socket directory %s: %w", dir, err)
	}
	return validatePrivateSocketDir(dir, dirInfo, uint32(os.Geteuid()))
}

func validateSocketRuntimeRoot(path string, info os.FileInfo, effectiveUID uint32) error {
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("transport: socket runtime root is not a directory: %s", path)
	}
	owner, err := fileOwnerUID(info)
	if err != nil {
		return fmt.Errorf("transport: inspect socket runtime root owner %s: %w", path, err)
	}
	privateOwnerRoot := owner == effectiveUID && info.Mode().Perm()&0o022 == 0
	stickyRoot := info.Mode()&os.ModeSticky != 0
	if !privateOwnerRoot && !stickyRoot {
		return fmt.Errorf("transport: socket runtime root is neither private nor sticky: %s", path)
	}
	return nil
}

func validatePrivateSocketDir(path string, info os.FileInfo, effectiveUID uint32) error {
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("transport: socket directory is not a directory: %s", path)
	}
	owner, err := fileOwnerUID(info)
	if err != nil {
		return fmt.Errorf("transport: inspect socket directory owner %s: %w", path, err)
	}
	if owner != effectiveUID {
		return fmt.Errorf("transport: socket directory %s is owned by uid %d, want %d", path, owner, effectiveUID)
	}
	if perm := info.Mode().Perm(); perm != config.DirPerm {
		return fmt.Errorf("transport: socket directory %s has permissions %04o, want %04o", path, perm, config.DirPerm)
	}
	return nil
}

func fileOwnerUID(info os.FileInfo) (uint32, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, errors.New("stat result does not expose an owner uid")
	}
	return stat.Uid, nil
}

func listen(path string) (Listener, error) {
	if err := checkSocketPath(path); err != nil {
		return nil, err
	}
	if err := ensureSocketDir(path, true); err != nil {
		return nil, err
	}
	// The private per-user parent excludes other users. The caller's bus.lock
	// serializes stale-socket cleanup for processes using the same WorkDir.
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
	if err := ensureSocketDir(path, false); err != nil {
		return nil, err
	}
	return net.Dial("unix", path)
}
