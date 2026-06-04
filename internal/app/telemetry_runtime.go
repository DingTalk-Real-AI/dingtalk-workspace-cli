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

package app

import (
	"os"
	"runtime"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/telemetry"
)

// emitTelemetry ships one anonymous operational metric for a finished
// invocation. It is cheap to skip: telemetry.NewForwarderFromEnv returns nil
// (after a single env read) when telemetry is disabled, so the hot path pays
// nothing and never loads the token or touches request content.
//
// Only coarse dimensions are collected here — there is intentionally no path
// that reads param values, object names, or natural-language input.
func emitTelemetry(execID string, inv executor.Invocation, ok bool, errClass string, dur time.Duration) {
	fwd := telemetry.NewForwarderFromEnv()
	if fwd == nil {
		return
	}

	ev := telemetry.New(time.Now(), execID)
	ev.CLIVersion = version
	ev.Channel = os.Getenv(envDWSChannel)
	ev.OS = runtime.GOOS
	ev.Module = inv.CanonicalProduct
	ev.Command = inv.CanonicalProduct
	ev.Subcommand = inv.Tool
	ev.DurationMS = dur.Milliseconds()

	// corp_id is the only identity-adjacent dimension, kept for per-tenant
	// health. Best-effort: a missing/locked token simply omits it.
	if td, err := authpkg.LoadTokenData(defaultConfigDir()); err == nil && td != nil {
		ev.CorpID = td.CorpID
	}

	if ok {
		ev.Outcome = "ok"
	} else {
		ev.Outcome = "error"
		ev.ErrClass = errClass
		ev.ExitCode = 1
	}

	_ = fwd.Emit(ev)
}
