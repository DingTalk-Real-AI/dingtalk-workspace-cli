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

package busctl

import (
	"os"
	"os/exec"
	"syscall"
)

// applyDetach configures the child to live past parent death and not share
// the parent's controlling terminal. Setsid puts the child in a new
// session, so SIGHUP on the parent's controlling tty (e.g. SSH disconnect)
// does not propagate. Setpgid is implied by Setsid.
func applyDetach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
}

// attachReadyPipe hands pw to the child as fd 3 via ExtraFiles and returns
// the ReadyFDEnv value the child passes to os.NewFile.
func attachReadyPipe(cmd *exec.Cmd, pw *os.File) (string, error) {
	cmd.ExtraFiles = []*os.File{pw} // child sees fd 3 = pw
	return "3", nil
}
