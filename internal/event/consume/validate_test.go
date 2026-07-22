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

package consume

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func base() Config {
	return Config{
		WorkDir:     "/tmp/x",
		IPCEndpoint: "/tmp/x/bus.sock",
		ClientID:    "ding_abc",
	}
}

func TestValidate_Happy(t *testing.T) {
	if err := ValidateConfig(base()); err != nil {
		t.Fatalf("baseline should be valid, got %v", err)
	}
}

func TestValidate_RequiredFields(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Config)
	}{
		{"empty WorkDir", func(c *Config) { c.WorkDir = "" }},
		{"whitespace WorkDir", func(c *Config) { c.WorkDir = "   " }},
		{"empty IPCEndpoint", func(c *Config) { c.IPCEndpoint = "" }},
		{"empty ClientID", func(c *Config) { c.ClientID = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base()
			tc.mut(&c)
			err := ValidateConfig(c)
			if !IsValidationError(err) {
				t.Fatalf("expected ValidationError, got %v", err)
			}
		})
	}
}

func TestValidate_ForceRequiresForeground(t *testing.T) {
	c := base()
	c.Force = true
	c.Foreground = false
	err := ValidateConfig(c)
	if !errors.Is(err, ErrForceRequiresForeground) {
		t.Fatalf("err = %v, want ErrForceRequiresForeground", err)
	}
	for _, recoveryStep := range []string{"event stop --all --dry-run", "event stop --all --yes", "event consume"} {
		if strings.Contains(err.Error(), recoveryStep) {
			continue
		}
		t.Errorf("error message must include the recovery hint, got: %s", err.Error())
	}

	// With --foreground it's fine.
	c.Foreground = true
	if err := ValidateConfig(c); err != nil {
		t.Fatalf("--force + --foreground should be valid, got %v", err)
	}
}

func TestValidate_FormatJSONRequiresBounded(t *testing.T) {
	c := base()
	c.Format = FormatJSON
	c.MaxEvents = 0
	c.Duration = 0
	err := ValidateConfig(c)
	if !errors.Is(err, ErrJSONFormatRequiresBounded) {
		t.Fatalf("err = %v, want ErrJSONFormatRequiresBounded", err)
	}

	// With --max-events it passes.
	c.MaxEvents = 10
	if err := ValidateConfig(c); err != nil {
		t.Fatalf("--format json + --max-events should be valid: %v", err)
	}
	c.MaxEvents = 0
	c.Duration = 30 * time.Second
	if err := ValidateConfig(c); err != nil {
		t.Fatalf("--format json + --duration should be valid: %v", err)
	}

	// NDJSON has no such requirement.
	c.Format = FormatNDJSON
	c.MaxEvents = 0
	c.Duration = 0
	if err := ValidateConfig(c); err != nil {
		t.Fatalf("ndjson unbounded should be valid: %v", err)
	}
}

func TestValidateNoOutputConflict(t *testing.T) {
	c := base()
	c.OutputDir = "/tmp/events"
	if err := ValidateNoOutputConflict(c, ""); err != nil {
		t.Fatalf("no global -o → ok, got %v", err)
	}
	if err := ValidateNoOutputConflict(c, "/tmp/out.json"); !IsValidationError(err) {
		t.Fatalf("--output-dir + global -o should be ValidationError, got %v", err)
	}

	c2 := base()
	c2.Routes, _ = ParseRoutes([]string{`^im=dir:./im/`})
	if err := ValidateNoOutputConflict(c2, "/tmp/out.json"); !IsValidationError(err) {
		t.Fatalf("--route + global -o should be ValidationError, got %v", err)
	}
}

func TestPrintDryRun_NilWriterSafe(t *testing.T) {
	// Must not panic.
	PrintDryRun(nil, base())
}

func TestPrintDryRun_RendersAllSetFields(t *testing.T) {
	var buf bytes.Buffer
	c := base()
	c.EventTypes = []string{"im.*", "approval.*"}
	c.Filter = "^im\\."
	c.Format = FormatCompact
	c.Flatten = true
	c.OutputDir = "/tmp/events"
	c.Routes, _ = ParseRoutes([]string{`^im\.=dir:/tmp/im/`})
	c.MaxEvents = 5
	c.Duration = 30 * time.Second
	c.Compact = true
	c.Quiet = true
	c.Foreground = true
	c.Force = true

	PrintDryRun(&buf, c)
	out := buf.String()
	wants := []string{
		"client_id", "workdir", "ipc_endpoint", "im.*,approval.*",
		"^im\\.", "compact", "/tmp/events", "route[0]", "max_events       : 5",
		"duration", "flatten          : true", "true",
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("dry-run missing %q in output:\n%s", w, out)
		}
	}
}

func TestPrintDryRun_CatchAllWhenEventTypesEmpty(t *testing.T) {
	var buf bytes.Buffer
	PrintDryRun(&buf, base())
	if !strings.Contains(buf.String(), "(catch-all)") {
		t.Errorf("expected '(catch-all)' for empty event_types:\n%s", buf.String())
	}
}
