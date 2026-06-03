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
	"os/exec"
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
