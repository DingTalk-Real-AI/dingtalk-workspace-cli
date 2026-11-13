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

package lock

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCrossPlatformCoverageSharedLeaseProcess(t *testing.T) {
	if path := os.Getenv("DWS_SHARED_LOCK_TEST_PATH"); path != "" {
		lease, err := TryAcquireShared(path)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Println("ready")
		bufio.NewReader(os.Stdin).ReadString('\n')
		if err := lease.Close(); err != nil {
			t.Fatal(err)
		}
		return
	}
	path := filepath.Join(t.TempDir(), "shared.lock")
	lease, err := TryAcquireShared(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestCrossPlatformCoverageSharedLeaseProcess$")
	cmd.Env = append(os.Environ(), "DWS_SHARED_LOCK_TEST_PATH="+path)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stdin.Close(); cmd.Process.Kill() })
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != "ready\n" {
		t.Fatalf("child: %q %v", line, err)
	}
	lease.Close()
	probe, err := TryAcquire(path)
	if probe != nil {
		probe.Close()
	}
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("exclusive lock ignored another process: %v", err)
	}
	stdin.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	probe, err = TryAcquire(path)
	if err != nil {
		t.Fatalf("final process did not release lease: %v", err)
	}
	probe.Close()
}
