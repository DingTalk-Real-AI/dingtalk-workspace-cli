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

package main

import (
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/telemetry"
)

var (
	appExecute                = app.ExecuteWithTelemetry
	resolveTelemetryIdentity  = app.ResolveTelemetryIdentity
	submitTelemetry           = telemetry.Submit
	snapshotTelemetryIdentity = startTelemetryIdentity
	exitProcess               = os.Exit
)

func main() {
	if code := run(); code != 0 {
		exitProcess(code)
	}
}

func run() int {
	if telemetry.RunWorker(os.Args[1:], os.Stdin) {
		return 0
	}
	optedOut := telemetry.OptedOut()
	var identity <-chan app.TelemetryIdentity
	if !optedOut {
		identity = snapshotTelemetryIdentity(os.Args[1:])
	}
	command := filepath.Base(os.Args[0])
	start := time.Now()
	code, path, message := appExecute()
	finished := time.Now()
	if !optedOut {
		event := telemetry.Event{
			Version: app.RawVersion(), Command: command, Path: path, ExitCode: code,
			DurationMillis: finished.Sub(start).Milliseconds(), CompletedAtMillis: finished.UnixMilli(),
			ErrorSummary: message,
		}
		select {
		case value := <-identity:
			event.Identity = telemetry.Identity{UserID: value.UserID, UserName: value.UserName, CorpID: value.CorpID}
		default:
		}
		submitTelemetry(event)
	}
	return code
}

func startTelemetryIdentity(args []string) <-chan app.TelemetryIdentity {
	result := make(chan app.TelemetryIdentity, 1)
	args = slices.Clone(args)
	resolve := resolveTelemetryIdentity
	go func() {
		var identity app.TelemetryIdentity
		defer func() { _ = recover(); result <- identity }()
		identity = resolve(args)
	}()
	return result
}
