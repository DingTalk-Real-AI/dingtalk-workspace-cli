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

package process

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
)

func TestAlive_SelfPID(t *testing.T) {
	if !Alive(os.Getpid()) {
		t.Fatal("Alive(self pid) should be true")
	}
}

func TestAlive_NonPositivePID(t *testing.T) {
	for _, pid := range []int{0, -1, -42} {
		if Alive(pid) {
			t.Fatalf("Alive(%d) should be false (non-positive)", pid)
		}
	}
}

func TestAlive_DeadChild(t *testing.T) {
	// Spawn a short-lived child and wait for it to exit, then verify Alive
	// returns false. We use sleep 0 which exits immediately on both Unix
	// and Windows (Windows treats `sleep 0` as a built-in via cmd.exe but
	// to avoid Windows shell quirks we fall back to `cmd /c exit 0`).
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "exit", "0")
	} else {
		cmd = exec.Command("sh", "-c", "exit 0")
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wait child: %v", err)
	}

	// On Windows, even after Wait, the kernel may keep the handle table
	// entry briefly; GetExitCodeProcess should still report non-STILL_ACTIVE
	// so Alive returns false. On Unix, the parent has reaped via Wait so
	// signal 0 returns ESRCH.
	if Alive(pid) {
		t.Fatalf("Alive(%d) should be false after child exit", pid)
	}
}
