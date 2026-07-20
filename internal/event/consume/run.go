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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/busctl"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

// Config holds everything Run needs. Built by the cobra command handler
// (P5) from flag values + strict resolver output.
type Config struct {
	// WorkDir is the bus working directory:
	// <ConfigDir>/events/<edition>/<source_kind>/<identity_hash>/
	WorkDir string
	// IPCEndpoint is the Unix socket path / Windows pipe name. Caller
	// computes from WorkDir on Unix, from edition+hash on Windows.
	IPCEndpoint string
	// ClientID is forwarded to busctl.Spawn so it can pass --client-id
	// when forking _bus.
	ClientID string
	// SpawnExtraArgs are forwarded to the hidden _bus process when consume.Run
	// needs to start a daemon. Used for source-mode options that must be
	// reproduced in the child process, including portal ticket mode and
	// personal_stream.
	SpawnExtraArgs []string

	// EventTypes / Filter / Compact are forwarded to the bus via Hello
	// for server-side pushdown filtering.
	EventTypes  []string
	Filter      string
	SubscribeID string
	Compact     bool

	// MaxEvents: stop after receiving this many events. 0 = no limit.
	MaxEvents int

	// EventKey is the single event key being consumed. Used only for the
	// AI-subprocess contract stderr lines (`[event] ready event_key=...`).
	// Empty → the key is omitted from the marker.
	EventKey string

	// Duration: wall-clock budget for the consume run. After this elapses,
	// Run returns nil (clean exit, exit code 0). Zero = no limit.
	//
	// Note: this is event-consume specific and intentionally NOT named
	// "Timeout" — global dws --timeout is HTTP request timeout (int
	// seconds) which would collide if reused. See plan §1 决策
	// "事件运行时长 flag 不复用全局 --timeout".
	Duration time.Duration

	// DryRun, when true, prints the resolved configuration to Stderr and
	// returns nil without dialing the bus. Used by the cobra layer to
	// preview configuration with `--dry-run` (plan §3.1).
	DryRun bool

	// Foreground hint, passed through to status output but otherwise has
	// no behavioural effect inside consume.Run — the cobra layer decides
	// whether to call this Run or to bus.Run directly when --foreground
	// is set.
	Foreground bool
	// Force, like Foreground, is informational at this layer. The cobra
	// layer enforces the "--force requires --foreground" rule before
	// calling Run.
	Force bool

	// --- Output / Sink config (P4) ---
	// Format controls the per-event output shape (ndjson/json/pretty/raw/
	// compact). The cobra layer maps --format string → Format via
	// NormalizeFormat; an empty Format here defaults to NDJSON inside
	// BuildPipeline.
	Format Format
	// OutputDir, if non-empty, switches the fallback sink from stdout to
	// "file per event" under this directory.
	OutputDir string
	// Routes are pre-parsed --route specs. Empty = no routing.
	Routes []Route
	// Projector, when set, maps transport envelopes to the public value used
	// by all structured formats. Raw format always bypasses it.
	Projector Projector

	// ReadySubscribeID identifies the personal subscription in the stable
	// ready marker emitted after HelloAck. It is separate from SubscribeID
	// because debug raw mode deliberately clears the latter's local filter.
	ReadySubscribeID string

	// Stdin, when non-nil, is watched for EOF: closing stdin triggers a
	// graceful shutdown (reason: signal). This wires the AI-subprocess
	// contract — a parent closes stdin to stop the consumer. The cobra
	// layer passes os.Stdin; tests inject a controllable reader. nil →
	// stdin is not watched (backward-compatible default for callers that
	// do not opt in).
	//
	// Note: `< /dev/null` EOFs immediately and exits at once. To stay
	// resident feed a never-EOF stdin (`< <(tail -f /dev/null)`) or run
	// bounded (--max-events / --duration).
	Stdin io.Reader

	// Stdout sink; nil → os.Stdout. Injected for tests.
	Stdout io.Writer
	// Stderr sink for status lines (HelloAck info, bye reason); nil → os.Stderr.
	// Set to io.Discard when --quiet is in effect.
	Stderr io.Writer

	// Quiet suppresses stderr status writes (the HelloAck / bye banners).
	Quiet bool
}

var discoverBus = busctl.Discover

// Run dials the bus (forking one if necessary), sends Hello, and writes
// each received Event frame as one NDJSON line to stdout. Blocks until
// ctx is cancelled, MaxEvents is reached, the bus sends Bye, or the
// stream is interrupted.
//
// Returns nil on graceful exits (ctx done, max-events reached, bye
// received, stdout pipe closed). Returns a non-nil error only for
// connection / protocol failures.
func Run(ctx context.Context, cfg Config) error {
	if cfg.WorkDir == "" || cfg.IPCEndpoint == "" || cfg.ClientID == "" {
		return errors.New("consume: WorkDir, IPCEndpoint, and ClientID are required")
	}
	if cfg.Stdout == nil {
		cfg.Stdout = os.Stdout
	}
	if cfg.Stderr == nil {
		cfg.Stderr = os.Stderr
	}
	if cfg.Quiet {
		cfg.Stderr = io.Discard
	}
	if cfg.Format == "" {
		cfg.Format = FormatNDJSON
	}

	// --dry-run: print resolved config, return without dialing.
	if cfg.DryRun {
		PrintDryRun(cfg.Stderr, cfg)
		return nil
	}

	// Distinguish the exit cause for the contract's `exited` line:
	//   duration deadline    → timeout
	//   parentCtx cancelled  → signal (SIGTERM/SIGINT)
	//   runCtx-only cancelled → signal (stdin EOF)
	parentCtx := ctx

	// --duration: layer a deadline on top of caller-provided ctx. Run
	// returns nil on deadline (clean exit) rather than surfacing the
	// context.DeadlineExceeded as an error to the user.
	var timeoutCtx context.Context
	if cfg.Duration > 0 {
		var cancel context.CancelFunc
		timeoutCtx, cancel = context.WithTimeout(ctx, cfg.Duration)
		defer cancel()
		ctx = timeoutCtx
	}

	// runCtx lets the stdin watcher and the read loop share one cancel
	// without disturbing the timeout/parent contexts (so we can still tell
	// stdin-EOF from a real signal from a duration deadline).
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	ctx = runCtx

	pipeline, err := BuildPipeline(
		cfg.Format,
		cfg.OutputDir,
		cfg.Routes,
		cfg.Stdout,
		WithProjector(cfg.Projector),
		WithProjectionWarnings(cfg.Stderr),
	)
	if err != nil {
		return fmt.Errorf("consume: build pipeline: %w", err)
	}
	defer pipeline.Close()

	conn, err := discoverBus(busctl.DiscoverConfig{
		WorkDir:        cfg.WorkDir,
		IPCEndpoint:    cfg.IPCEndpoint,
		ClientID:       cfg.ClientID,
		SpawnExtraArgs: cfg.SpawnExtraArgs,
	})
	if err != nil {
		return fmt.Errorf("consume: discover bus: %w", err)
	}
	defer conn.Close()

	// Ensure the conn closes when ctx cancels so blocked Read returns.
	closeOnContext(ctx, conn)

	w := transport.NewWriter(conn)
	r := transport.NewReader(conn)

	hello := transport.Hello{
		Type:        transport.FrameTypeHello,
		ConsumerPID: os.Getpid(),
		EventTypes:  cfg.EventTypes,
		Filter:      cfg.Filter,
		SubscribeID: cfg.SubscribeID,
		Compact:     cfg.Compact,
	}
	if err := w.WriteJSON(hello); err != nil {
		return fmt.Errorf("consume: write hello: %w", err)
	}

	var ack transport.HelloAck
	if err := r.ReadJSON(&ack); err != nil {
		return fmt.Errorf("consume: read hello_ack: %w", err)
	}
	if ack.Type != transport.FrameTypeHelloAck {
		return fmt.Errorf("consume: unexpected first frame type %q", ack.Type)
	}
	if !cfg.Quiet {
		// Contract: a fixed ready line on stderr BEFORE any stdout event.
		// Parents block on stderr until this appears, then read stdout.
		if cfg.EventKey != "" {
			fmt.Fprintf(cfg.Stderr, "[event] ready event_key=%s bus_pid=%d", cfg.EventKey, ack.BusPID)
			if cfg.ReadySubscribeID != "" {
				fmt.Fprintf(cfg.Stderr, " subscribe_id=%s", cfg.ReadySubscribeID)
			}
			fmt.Fprintln(cfg.Stderr)
		} else {
			fmt.Fprintf(cfg.Stderr, "[event] ready bus_pid=%d\n", ack.BusPID)
		}
		// Secondary diagnostic line (source/state/idle); not part of the
		// ready contract.
		fmt.Fprintf(cfg.Stderr, "[event] bus source=%s state=%s idle_timeout=%ds\n",
			ack.StateSource, ack.SourceState, ack.IdleTimeoutSecs)
	}

	// Watch stdin for EOF → graceful shutdown (AI-subprocess contract).
	// The cobra layer only sets Stdin when the watcher should arm (a
	// pipe-style, unbounded run); tests inject it directly.
	if cfg.Stdin != nil {
		go watchStdinEOF(runCtx, cfg.Stdin, cfg.Stderr, cancelRun)
	}

	// Exit-reason contract: on a graceful exit emit a final stderr line
	//   [event] exited — received N event(s) in Xs (reason: <r>)
	// Error returns leave reason empty → no `exited` line (an `Error:` line
	// is printed by the cobra layer instead).
	received := 0
	start := time.Now()
	reason := ""
	defer func() {
		if !cfg.Quiet && reason != "" {
			fmt.Fprintf(cfg.Stderr, "[event] exited — received %d event(s) in %s (reason: %s)\n",
				received, time.Since(start).Round(time.Millisecond), reason)
		}
	}()

	// classifyCancel maps a context-cancelled exit to a contract reason.
	classifyCancel := func() string {
		if timeoutCtx != nil && errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) && parentCtx.Err() == nil {
			return "timeout"
		}
		return "signal"
	}

	for {
		raw, err := r.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				reason = "bus_shutdown" // peer closed cleanly
				return nil
			}
			if isCtxCancelled(ctx) {
				reason = classifyCancel()
				return nil
			}
			return fmt.Errorf("consume: read frame: %w", err)
		}
		typ, err := transport.PeekType(raw)
		if err != nil {
			// Malformed frame; skip and continue.
			continue
		}
		switch typ {
		case transport.FrameTypeEvent:
			var ev transport.Event
			if err := json.Unmarshal(raw, &ev); err != nil {
				continue
			}
			if err := pipeline.Deliver(ev); err != nil {
				if errors.Is(err, ErrPipeClosed) {
					// Downstream stdout consumer closed; exit cleanly.
					_ = w.WriteJSON(transport.Bye{
						Type:   transport.FrameTypeBye,
						Reason: "client_done",
					})
					reason = "signal"
					return nil
				}
				return fmt.Errorf("consume: deliver event: %w", err)
			}
			received++
			if cfg.MaxEvents > 0 && received >= cfg.MaxEvents {
				_ = w.WriteJSON(transport.Bye{
					Type:   transport.FrameTypeBye,
					Reason: "client_done",
				})
				reason = "limit"
				return nil
			}
		case transport.FrameTypeBye:
			var bye transport.Bye
			_ = json.Unmarshal(raw, &bye)
			if !cfg.Quiet {
				fmt.Fprintf(cfg.Stderr, "[event] bus closing: %s\n", bye.Reason)
			}
			reason = "bus_shutdown"
			return nil
		case transport.FrameTypeSourceState:
			if !cfg.Quiet {
				var s transport.SourceState
				_ = json.Unmarshal(raw, &s)
				fmt.Fprintf(cfg.Stderr, "source state: %s (source=%s, attempt=%d)\n", s.State, s.StateSource, s.Attempt)
			}
		case transport.FrameTypeHeartbeat:
			// silent
		default:
			// future frame types: ignored for forward compat
		}
	}
}

// closeOnContext spawns a goroutine that closes conn when ctx is done.
// This unblocks any pending Read on conn so the main loop can return.
func closeOnContext(ctx context.Context, conn net.Conn) {
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()
}

func isCtxCancelled(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

// watchStdinEOF reads and discards stdin until EOF (or any read error),
// then prints a self-explaining diagnostic and calls onEOF to trigger a
// graceful shutdown. This implements the AI-subprocess contract: a parent
// closes the child's stdin to stop it. It returns early if ctx is
// cancelled first (the run ended for another reason), so it does not fire
// a spurious shutdown. errOut is io.Discard under --quiet.
func watchStdinEOF(ctx context.Context, r io.Reader, errOut io.Writer, onEOF func()) {
	buf := make([]byte, 512)
	for {
		if _, err := r.Read(buf); err != nil {
			select {
			case <-ctx.Done():
				// Run already ending; do not attribute this to stdin.
			default:
				fmt.Fprintln(errOut, "[event] stdin closed — shutting down. "+
					"consume treats stdin EOF as an exit signal (wired for AI subprocess callers). "+
					"To keep running: pass --max-events/--duration for a bounded run, "+
					"keep stdin open (`< <(tail -f /dev/null)` in a script), "+
					"or stop via SIGTERM instead of closing stdin.")
				onEOF()
			}
			return
		}
		// Discard any data and keep reading until EOF.
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}
