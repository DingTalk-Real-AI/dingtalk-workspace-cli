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
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"gitlab.alibaba-inc.com/aes/aem-go-sdk/clitrack"
)

var (
	appExecute               = app.ExecuteWithTelemetry
	resolveTelemetryIdentity = app.ResolveTelemetryIdentity
	trackRun                 = func(cfg clitrack.Config, execute func() error, exitCode func(error) int) {
		clitrack.New(cfg).Run(execute, exitCode)
	}
)

// trackedExitError tells clitrack that the command failed without asking it to
// print the error a second time. The already-rendered message is published via
// ExtraFields c5, while app.Execute remains the sole owner of presentation.
type trackedExitError struct{}

func (trackedExitError) Error() string { return "" }

func trackerConfig(identity app.TelemetryIdentity, commandPath, errorMessage *string) clitrack.Config {
	return clitrack.Config{
		PID:                   "wcCRwZ",
		App:                   "dws",
		Version:               app.RawVersion(),
		UID:                   identity.UserID,
		Username:              identity.UserName,
		NoCommandLine:         true,
		NoCwd:                 true,
		NoAutomaticDimensions: true,
		ExtraFields: func() map[string]string {
			fields := map[string]string{"c9": *commandPath}
			if identity.CorpID != "" {
				fields["c10"] = identity.CorpID
			}
			if *errorMessage != "" {
				fields["c5"] = *errorMessage
			}
			return fields
		},
	}
}

func telemetryOptedOut() bool {
	return strings.TrimSpace(os.Getenv("DO_NOT_TRACK")) != ""
}

func main() {
	optedOut := telemetryOptedOut()
	identity := app.TelemetryIdentity{}
	if !optedOut {
		identity = resolveTelemetryIdentity(os.Args[1:])
	}
	exitCode := 0
	commandPath := "dws"
	errorMessage := ""
	cfg := trackerConfig(identity, &commandPath, &errorMessage)
	if optedOut {
		cfg.PID = ""
	}
	trackRun(
		cfg,
		func() error {
			exitCode, commandPath, errorMessage = appExecute()
			if exitCode != 0 {
				return trackedExitError{}
			}
			return nil
		},
		func(error) int { return exitCode },
	)
}
