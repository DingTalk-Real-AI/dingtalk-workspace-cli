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
	"testing"
	"time"
)

func TestApplyEnvTuning_DefaultsWhenAllUnset(t *testing.T) {
	t.Setenv(EnvIdleTimeout, "")
	t.Setenv(EnvConsumerBuffer, "")
	t.Setenv(EnvDedupLRU, "")
	t.Setenv(EnvDropWarnPct, "")

	cfg := Config{}
	ApplyEnvTuning(&cfg)

	if cfg.IdleTimeout != 5*time.Minute {
		t.Errorf("IdleTimeout default = %s, want 5m", cfg.IdleTimeout)
	}
	if cfg.DropWarnPercent != DefaultDropWarnPercent {
		t.Errorf("DropWarnPercent default = %d, want %d",
			cfg.DropWarnPercent, DefaultDropWarnPercent)
	}
	// ConsumerBuffer / DedupCapacity intentionally left zero — the Hub
	// / dedup packages apply their own defaults from that signal.
	if cfg.ConsumerBuffer != 0 {
		t.Errorf("ConsumerBuffer should remain 0 (Hub picks default), got %d", cfg.ConsumerBuffer)
	}
	if cfg.DedupCapacity != 0 {
		t.Errorf("DedupCapacity should remain 0 (dedup picks default), got %d", cfg.DedupCapacity)
	}
}

func TestApplyEnvTuning_ReadsValidEnv(t *testing.T) {
	t.Setenv(EnvIdleTimeout, "10m")
	t.Setenv(EnvConsumerBuffer, "200")
	t.Setenv(EnvDedupLRU, "16384")
	t.Setenv(EnvDropWarnPct, "10")

	cfg := Config{}
	ApplyEnvTuning(&cfg)

	if cfg.IdleTimeout != 10*time.Minute {
		t.Errorf("IdleTimeout = %s, want 10m", cfg.IdleTimeout)
	}
	if cfg.ConsumerBuffer != 200 {
		t.Errorf("ConsumerBuffer = %d, want 200", cfg.ConsumerBuffer)
	}
	if cfg.DedupCapacity != 16384 {
		t.Errorf("DedupCapacity = %d, want 16384", cfg.DedupCapacity)
	}
	if cfg.DropWarnPercent != 10 {
		t.Errorf("DropWarnPercent = %d, want 10", cfg.DropWarnPercent)
	}
}

func TestApplyEnvTuning_ExplicitConfigWins(t *testing.T) {
	t.Setenv(EnvIdleTimeout, "10m")
	t.Setenv(EnvDropWarnPct, "20")

	cfg := Config{
		IdleTimeout:     3 * time.Minute,
		DropWarnPercent: 7,
	}
	ApplyEnvTuning(&cfg)

	if cfg.IdleTimeout != 3*time.Minute {
		t.Errorf("explicit IdleTimeout overridden by env: %s", cfg.IdleTimeout)
	}
	if cfg.DropWarnPercent != 7 {
		t.Errorf("explicit DropWarnPercent overridden by env: %d", cfg.DropWarnPercent)
	}
}

func TestApplyEnvTuning_IgnoresInvalidValues(t *testing.T) {
	t.Setenv(EnvIdleTimeout, "not-a-duration")
	t.Setenv(EnvConsumerBuffer, "-50")
	t.Setenv(EnvDedupLRU, "notanumber")
	t.Setenv(EnvDropWarnPct, "150") // out of 1..100

	cfg := Config{}
	ApplyEnvTuning(&cfg)

	if cfg.IdleTimeout != 5*time.Minute {
		t.Errorf("invalid duration → default; got %s", cfg.IdleTimeout)
	}
	if cfg.ConsumerBuffer != 0 {
		t.Errorf("negative buffer ignored → 0; got %d", cfg.ConsumerBuffer)
	}
	if cfg.DedupCapacity != 0 {
		t.Errorf("non-numeric LRU ignored → 0; got %d", cfg.DedupCapacity)
	}
	if cfg.DropWarnPercent != DefaultDropWarnPercent {
		t.Errorf("out-of-range pct → default; got %d", cfg.DropWarnPercent)
	}
}

func TestApplyEnvTuning_IdleTimeoutZeroEnv(t *testing.T) {
	t.Setenv(EnvIdleTimeout, "0")
	cfg := Config{}
	ApplyEnvTuning(&cfg)
	// "0" is parseable but <= 0 → use default 5m, not 0 (which would mean
	// "disabled" in the daemon — too dangerous as an env-driven default).
	if cfg.IdleTimeout != 5*time.Minute {
		t.Errorf("zero env should fall through to 5m default, got %s", cfg.IdleTimeout)
	}
}
