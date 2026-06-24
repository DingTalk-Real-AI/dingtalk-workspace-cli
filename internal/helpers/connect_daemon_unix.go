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

package helpers

import (
	"os/exec"
	"syscall"
)

// daemonDetachSupported reports whether the current OS can detach a daemon.
const daemonDetachSupported = true

// detachSysProcAttr returns the SysProcAttr that detaches a child from the
// controlling terminal: Setsid starts a new session so the child survives the
// parent shell closing (no SIGHUP) and has no controlling tty. Used for the
// `connect --daemon` re-exec. Unix-only; Windows uses the stub.
func detachSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

// applyDetach configures cmd so it runs in its own session, detached from the
// terminal.
func applyDetach(cmd *exec.Cmd) {
	cmd.SysProcAttr = detachSysProcAttr()
}
