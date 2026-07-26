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

package busctl

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

// CREATE_NEW_PROCESS_GROUP (0x00000200) prevents the child from receiving
// the parent's Ctrl+C signal, similar in spirit to Setsid on Unix.
const createNewProcessGroup = 0x00000200

func applyDetach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup,
		HideWindow:    true,
	}
}

// attachReadyPipe hands pw to the child on Windows, where cmd.ExtraFiles is
// not supported — os.StartProcess fails with EWINDOWS ("fork/exec ...: not
// supported by windows") on more than the 3 stdio files. Instead the pipe
// handle is marked inheritable and listed in AdditionalInheritedHandles.
// Inherited handles keep their value in the child, so ReadyFDEnv carries
// the handle value itself and the child's os.NewFile reconstructs the
// write end from it. Must run after applyDetach, which installs
// SysProcAttr (the nil check below is a safety net, not the normal path).
func attachReadyPipe(cmd *exec.Cmd, pw *os.File) (string, error) {
	h := syscall.Handle(pw.Fd())
	if err := syscall.SetHandleInformation(h, syscall.HANDLE_FLAG_INHERIT, syscall.HANDLE_FLAG_INHERIT); err != nil {
		return "", fmt.Errorf("mark ready pipe inheritable: %w", err)
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.AdditionalInheritedHandles = append(cmd.SysProcAttr.AdditionalInheritedHandles, h)
	return strconv.FormatUint(uint64(h), 10), nil
}
