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

package bus

import (
	"os"
	"strconv"
	"time"
)

// Tunable env vars (plan §15 已决项 — surfaced for operators without
// requiring a new CLI flag for each knob). Each lookup is read-once at
// bus startup; runtime changes require a bus restart.
const (
	EnvIdleTimeout    = "DWS_EVENT_BUS_IDLE_TIMEOUT" // Go duration, e.g. "10m"
	EnvConsumerBuffer = "DWS_EVENT_CONSUMER_BUFFER"  // integer
	EnvDedupLRU       = "DWS_EVENT_DEDUP_LRU"        // integer
	EnvDropWarnPct    = "DWS_EVENT_DROP_WARN_PCT"    // integer 1-100
)

// ApplyEnvTuning fills in Config defaults from the env vars listed above
// for any fields the caller left at zero. The cobra layer calls this after
// constructing Config so explicit flag values still win.
//
// Defaults (when env is absent or invalid):
//
//	IdleTimeout      → 5m
//	ConsumerBuffer   → DefaultSendBuffer
//	DedupCapacity    → 0 (let dedup package's DefaultCapacity apply)
//	DropWarnPercent  → DefaultDropWarnPercent
//
// Invalid env values (non-parseable, out of range) are silently ignored
// and the default is used. We deliberately do NOT fail the bus on bad
// env input — operators shouldn't lose a daemon over a typo'd env var.
func ApplyEnvTuning(cfg *Config) {
	if cfg.IdleTimeout == 0 {
		if v := os.Getenv(EnvIdleTimeout); v != "" {
			if d, err := time.ParseDuration(v); err == nil && d > 0 {
				cfg.IdleTimeout = d
			}
		}
		if cfg.IdleTimeout == 0 {
			cfg.IdleTimeout = 5 * time.Minute
		}
	}
	if cfg.ConsumerBuffer == 0 {
		if v := os.Getenv(EnvConsumerBuffer); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				cfg.ConsumerBuffer = n
			}
		}
	}
	if cfg.DedupCapacity == 0 {
		if v := os.Getenv(EnvDedupLRU); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				cfg.DedupCapacity = n
			}
		}
	}
	if cfg.DropWarnPercent == 0 {
		if v := os.Getenv(EnvDropWarnPct); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
				cfg.DropWarnPercent = n
			}
		}
		if cfg.DropWarnPercent == 0 {
			cfg.DropWarnPercent = DefaultDropWarnPercent
		}
	}
}
