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

package telemetry

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/lock"
	"gitlab.alibaba-inc.com/aes/aem-go-sdk/clitrack"
)

type executionTracker interface {
	ReportExecution(clitrack.Execution) error
	Close() error
}

var (
	newTracker      = func(cfg clitrack.Config) executionTracker { return clitrack.New(cfg) }
	workerAfterFunc = time.AfterFunc
	workerExit      = os.Exit
	workerSend      = sendEvent
)

// RunWorker handles only the private sender invocation, before command assembly.
// Even malformed input or an SDK panic must not become a business command.
func RunWorker(args []string, input *os.File) bool {
	if len(args) != 1 || args[0] != workerArgument {
		return false
	}
	if OptedOut() {
		return true
	}
	info, err := input.Stat()
	if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
		return true
	}
	// This deadline also covers a stalled filesystem, incomplete input, or SDK
	// code that ignores cancellation. It runs only in the disposable worker.
	exit := workerExit
	timer := workerAfterFunc(workerLifetime, func() { exit(0) })
	defer timer.Stop()
	func() {
		defer func() { _ = recover() }()
		_ = runWorker(input, acquireSlot, workerSend)
	}()
	return true
}

func runWorker(input io.Reader, acquire func() (io.Closer, error), send func(Event) error) error {
	payload, err := io.ReadAll(io.LimitReader(input, maxEventBytes+1))
	if err != nil {
		return err
	}
	if len(payload) > maxEventBytes {
		return errors.New("oversized telemetry event")
	}
	var event Event
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return err
	}
	if !validEvent(event) {
		return errors.New("invalid telemetry event")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("trailing telemetry data")
	}
	slot, err := acquire()
	if err != nil {
		return err
	}
	defer slot.Close()
	return send(event)
}

func acquireSlot() (io.Closer, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	return acquireSlotIn(filepath.Join(cache, "dws", "clitrack", "slots"))
}

func acquireSlotIn(directory string) (io.Closer, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	for i := 0; i < maxSenders; i++ {
		slot, err := lock.TryAcquire(filepath.Join(directory, fmt.Sprintf("%d.lock", i)))
		if err == nil {
			return slot, nil
		}
		if !errors.Is(err, lock.ErrBusy) {
			return nil, err
		}
	}
	return nil, lock.ErrBusy
}

func sdkConfig(event Event) clitrack.Config {
	return clitrack.Config{
		PID: "wcCRwZ", App: "dws", Version: event.Version,
		UID: event.Identity.UserID, Username: event.Identity.UserName,
		NoCommandLine: true, NoCwd: true, NoAutomaticDimensions: true,
		NoAttribution: true,
		ExtraFields: func() map[string]string {
			return map[string]string{"c9": event.Path, "c10": event.Identity.CorpID}
		},
	}
}

func sendEvent(event Event) error {
	return sendWithConfig(event, sdkConfig(event))
}

func sendWithConfig(event Event, cfg clitrack.Config) (err error) {
	tracker := newTracker(cfg)
	defer func() {
		closeErr := tracker.Close()
		if err == nil {
			err = closeErr
		}
	}()
	return tracker.ReportExecution(clitrack.Execution{
		Command: event.Command, ExitCode: event.ExitCode,
		Duration:    time.Duration(event.DurationMillis) * time.Millisecond,
		CompletedAt: time.UnixMilli(event.CompletedAtMillis), ErrorSummary: event.ErrorSummary,
	})
}
