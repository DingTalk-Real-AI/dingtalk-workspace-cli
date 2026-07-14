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
	"errors"
	"fmt"
	"io"
	"strings"
)

// ValidationError represents a flag-level user error. It wraps a clear,
// actionable message — the cobra command layer surfaces it to the user
// with exit code 2 (validation error).
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// validation sentinels (cobra layer uses errors.Is to set the exit code).
var (
	// ErrForceRequiresForeground is the plan §3.1 contract: --force only
	// makes sense in foreground mode where the bus runs in the current
	// process (then --force skips the single-instance lock so a second
	// foreground bus can co-exist for a brief debug window). Outside of
	// --foreground, --force would silently produce two daemons writing
	// to the same socket — refuse upfront.
	ErrForceRequiresForeground = &ValidationError{
		Msg: "--force is only meaningful with --foreground (in daemon mode it would produce multiple bus instances; cloud events would be randomly split across connections). To restart the bus: preview dws event stop --all --dry-run, confirm with dws event stop --all --yes, then run dws event consume",
	}

	// ErrJSONFormatRequiresBounded is the plan §3.1 contract: --format
	// json renders each event as a multi-line JSON object suitable for
	// human inspection. With an unbounded stream the output mixes events
	// without delimiters. Force the user to bound the run.
	ErrJSONFormatRequiresBounded = &ValidationError{
		Msg: "--format json requires --max-events or --duration (an unbounded JSON stream is not parseable). Use --format ndjson for unbounded streams.",
	}
)

// ValidateConfig performs all pre-flight validation that does not require
// disk / network I/O. Returns a *ValidationError for any rule violation;
// returns nil if the cfg is launchable. The cobra layer calls this BEFORE
// calling Run so the user gets clear errors at parse time.
//
// Rules implemented:
//  1. WorkDir / IPCEndpoint / ClientID non-empty
//  2. --force requires --foreground (plan §3.1)
//  3. --format json requires --max-events OR --duration (bounded)
//  4. Routes already pre-parsed (any parse error is reported by ParseRoutes)
//  5. --output-dir conflict with global --output (caller-supplied flag —
//     we expose ValidateNoOutputConflict separately because global -o is
//     a cobra-layer concern)
//
// Rules NOT enforced here (deferred to caller / Run):
//   - Credentials presence (auth.ResolveAppCredentialsStrict already
//     reports a typed error)
//   - bus availability (busctl.Discover handles)
func ValidateConfig(cfg Config) error {
	if strings.TrimSpace(cfg.WorkDir) == "" {
		return &ValidationError{Msg: "consume: WorkDir is required"}
	}
	if strings.TrimSpace(cfg.IPCEndpoint) == "" {
		return &ValidationError{Msg: "consume: IPCEndpoint is required"}
	}
	if strings.TrimSpace(cfg.ClientID) == "" {
		return &ValidationError{Msg: "consume: ClientID is required"}
	}
	if cfg.Force && !cfg.Foreground {
		return ErrForceRequiresForeground
	}
	if cfg.Format == FormatJSON && cfg.MaxEvents <= 0 && cfg.Duration <= 0 {
		return ErrJSONFormatRequiresBounded
	}
	return nil
}

// ValidateNoOutputConflict ensures --output-dir / --route (event-stream
// sinks) are not combined with the dws global hidden -o/--output flag
// (request-output to file). The cobra layer reads the global output flag
// from inherited flags and passes its value here; an empty globalOutput
// means the flag was unset.
func ValidateNoOutputConflict(cfg Config, globalOutput string) error {
	if globalOutput == "" {
		return nil
	}
	if cfg.OutputDir != "" || len(cfg.Routes) > 0 {
		return &ValidationError{
			Msg: fmt.Sprintf("--output-dir/--route cannot be combined with global -o/--output=%q (event stream sinks are mutually exclusive with single-file output capture)", globalOutput),
		}
	}
	return nil
}

// IsValidationError reports whether err is a flag-level user error.
// Cobra command handlers use this to map validation errors to exit code 2.
func IsValidationError(err error) bool {
	var v *ValidationError
	return errors.As(err, &v)
}

// PrintDryRun writes the resolved configuration to w in a single
// human-readable block. Called by Run when cfg.DryRun is true. Format
// avoids JSON so users can `dws event consume --dry-run | head` cleanly.
//
// Secret-bearing fields are never present in Config (credentials never
// reach this layer), so no redaction is required here.
func PrintDryRun(w io.Writer, cfg Config) {
	if w == nil {
		return
	}
	fmt.Fprintln(w, "dws event consume — dry run (no bus connection will be made)")
	fmt.Fprintf(w, "  client_id        : %s\n", cfg.ClientID)
	fmt.Fprintf(w, "  workdir          : %s\n", cfg.WorkDir)
	fmt.Fprintf(w, "  ipc_endpoint     : %s\n", cfg.IPCEndpoint)
	if len(cfg.EventTypes) > 0 {
		fmt.Fprintf(w, "  event_types      : %s\n", strings.Join(cfg.EventTypes, ","))
	} else {
		fmt.Fprintln(w, "  event_types      : (catch-all)")
	}
	if cfg.Filter != "" {
		fmt.Fprintf(w, "  filter           : %s\n", cfg.Filter)
	}
	fmt.Fprintf(w, "  format           : %s\n", cfg.Format)
	if cfg.OutputDir != "" {
		fmt.Fprintf(w, "  output_dir       : %s\n", cfg.OutputDir)
	}
	for i, r := range cfg.Routes {
		fmt.Fprintf(w, "  route[%d]         : %s\n", i, r.Raw)
	}
	if cfg.MaxEvents > 0 {
		fmt.Fprintf(w, "  max_events       : %d\n", cfg.MaxEvents)
	}
	if cfg.Duration > 0 {
		fmt.Fprintf(w, "  duration         : %s\n", cfg.Duration)
	}
	fmt.Fprintf(w, "  compact          : %v\n", cfg.Compact)
	fmt.Fprintf(w, "  quiet            : %v\n", cfg.Quiet)
	fmt.Fprintf(w, "  foreground       : %v\n", cfg.Foreground)
	fmt.Fprintf(w, "  force            : %v\n", cfg.Force)
}
