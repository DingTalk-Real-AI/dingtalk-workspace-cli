// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package consume

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/busctl"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

// ConsumerSpec describes one logical subscription within a multi-event
// foreground process. Each spec gets an independent IPC connection so the bus
// can continue enforcing the exact event_type + subscribe_id pair.
type ConsumerSpec struct {
	EventKey         string
	EventTypes       []string
	Filter           string
	SubscribeID      string
	ReadySubscribeID string
}

type manySession struct {
	spec ConsumerSpec
	conn net.Conn
	w    *transport.Writer
	r    *transport.Reader
	ack  transport.HelloAck
}

type manyFrame struct {
	index int
	raw   []byte
	err   error
}

// RunMany consumes multiple independently isolated subscriptions in one
// process. It shares one formatter/sink pipeline and one command lifecycle,
// while retaining one bus IPC connection per subscription.
func RunMany(ctx context.Context, cfg Config, specs []ConsumerSpec) error {
	if cfg.WorkDir == "" || cfg.IPCEndpoint == "" || cfg.ClientID == "" {
		return errors.New("consume: WorkDir, IPCEndpoint, and ClientID are required")
	}
	if len(specs) < 2 {
		return errors.New("consume: RunMany requires at least two consumers")
	}
	if err := validateConsumerSpecs(specs); err != nil {
		return err
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
	if cfg.DryRun {
		PrintDryRunMany(cfg.Stderr, cfg, specs)
		return nil
	}

	parentCtx := ctx
	var timeoutCtx context.Context
	if cfg.Duration > 0 {
		var cancel context.CancelFunc
		timeoutCtx, cancel = context.WithTimeout(ctx, cfg.Duration)
		defer cancel()
		ctx = timeoutCtx
	}
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

	sessions := make([]*manySession, 0, len(specs))
	closeSessions := func() {
		for _, session := range sessions {
			_ = session.conn.Close()
		}
	}
	defer closeSessions()

	for _, spec := range specs {
		conn, err := discoverBus(busctl.DiscoverConfig{
			WorkDir:        cfg.WorkDir,
			IPCEndpoint:    cfg.IPCEndpoint,
			ClientID:       cfg.ClientID,
			SpawnExtraArgs: cfg.SpawnExtraArgs,
		})
		if err != nil {
			return fmt.Errorf("consume: discover bus for %s: %w", spec.EventKey, err)
		}
		session := &manySession{
			spec: spec,
			conn: conn,
			w:    transport.NewWriter(conn),
			r:    transport.NewReader(conn),
		}
		sessions = append(sessions, session)
		closeOnContext(ctx, session.conn)
		if err := session.w.WriteJSON(transport.Hello{
			Type:        transport.FrameTypeHello,
			ConsumerPID: os.Getpid(),
			EventTypes:  spec.EventTypes,
			Filter:      spec.Filter,
			SubscribeID: spec.SubscribeID,
			Compact:     cfg.Compact,
		}); err != nil {
			return fmt.Errorf("consume: write hello for %s: %w", spec.EventKey, err)
		}
		if err := session.r.ReadJSON(&session.ack); err != nil {
			return fmt.Errorf("consume: read hello_ack for %s: %w", spec.EventKey, err)
		}
		if session.ack.Type != transport.FrameTypeHelloAck {
			return fmt.Errorf("consume: unexpected first frame type %q for %s", session.ack.Type, spec.EventKey)
		}
		if len(sessions) > 1 && session.ack.BusPID != sessions[0].ack.BusPID {
			return fmt.Errorf("consume: consumers connected to different bus processes (%d and %d)", sessions[0].ack.BusPID, session.ack.BusPID)
		}
	}

	if !cfg.Quiet {
		for _, session := range sessions {
			fmt.Fprintf(cfg.Stderr, "[event] subscription event_key=%s subscribe_id=%s\n",
				session.spec.EventKey, session.spec.ReadySubscribeID)
		}
		fmt.Fprintf(cfg.Stderr, "[event] ready event_count=%d bus_pid=%d\n", len(sessions), sessions[0].ack.BusPID)
		fmt.Fprintf(cfg.Stderr, "[event] bus source=%s state=%s idle_timeout=%ds\n",
			sessions[0].ack.StateSource, sessions[0].ack.SourceState, sessions[0].ack.IdleTimeoutSecs)
	}

	if cfg.Stdin != nil {
		go watchStdinEOF(runCtx, cfg.Stdin, cfg.Stderr, cancelRun)
	}

	frames := make(chan manyFrame, len(sessions)*2)
	active := make(map[int]struct{}, len(sessions))
	for index, session := range sessions {
		active[index] = struct{}{}
		go readManySession(ctx, index, session, frames)
	}

	received := 0
	start := time.Now()
	reason := ""
	defer func() {
		if !cfg.Quiet && reason != "" {
			fmt.Fprintf(cfg.Stderr, "[event] exited — received %d event(s) in %s (reason: %s)\n",
				received, time.Since(start).Round(time.Millisecond), reason)
		}
	}()
	classifyCancel := func() string {
		if timeoutCtx != nil && errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) && parentCtx.Err() == nil {
			return "timeout"
		}
		return "signal"
	}

	for len(active) > 0 {
		select {
		case <-ctx.Done():
			reason = classifyCancel()
			return nil
		case frame := <-frames:
			if _, ok := active[frame.index]; !ok {
				continue
			}
			if frame.err != nil {
				if isCtxCancelled(ctx) {
					reason = classifyCancel()
					return nil
				}
				return fmt.Errorf("consume: read frame for %s: %w", sessions[frame.index].spec.EventKey, frame.err)
			}
			typ, err := transport.PeekType(frame.raw)
			if err != nil {
				continue
			}
			switch typ {
			case transport.FrameTypeEvent:
				var ev transport.Event
				if err := json.Unmarshal(frame.raw, &ev); err != nil {
					continue
				}
				if err := pipeline.Deliver(ev); err != nil {
					if errors.Is(err, ErrPipeClosed) {
						sendManyBye(sessions, active, "client_done")
						reason = "signal"
						return nil
					}
					return fmt.Errorf("consume: deliver event: %w", err)
				}
				received++
				if cfg.MaxEvents > 0 && received >= cfg.MaxEvents {
					sendManyBye(sessions, active, "client_done")
					reason = "limit"
					return nil
				}
			case transport.FrameTypeBye:
				var bye transport.Bye
				_ = json.Unmarshal(frame.raw, &bye)
				if bye.Reason == transport.ByeReasonSubscriptionStopped {
					delete(active, frame.index)
					_ = sessions[frame.index].conn.Close()
					if !cfg.Quiet {
						fmt.Fprintf(cfg.Stderr, "[event] subscription stopped event_key=%s subscribe_id=%s remaining=%d\n",
							sessions[frame.index].spec.EventKey,
							sessions[frame.index].spec.ReadySubscribeID,
							len(active))
					}
					continue
				}
				if !cfg.Quiet {
					fmt.Fprintf(cfg.Stderr, "[event] bus closing: %s\n", bye.Reason)
				}
				reason = "bus_shutdown"
				return nil
			case transport.FrameTypeSourceState:
				if !cfg.Quiet && frame.index == firstActiveSession(active) {
					var state transport.SourceState
					_ = json.Unmarshal(frame.raw, &state)
					fmt.Fprintf(cfg.Stderr, "source state: %s (source=%s, attempt=%d)\n", state.State, state.StateSource, state.Attempt)
				}
			case transport.FrameTypeHeartbeat:
				// silent
			}
		}
	}
	reason = "subscriptions_stopped"
	return nil
}

func validateConsumerSpecs(specs []ConsumerSpec) error {
	seen := map[string]struct{}{}
	for _, spec := range specs {
		if strings.TrimSpace(spec.EventKey) == "" {
			return errors.New("consume: consumer event_key is required")
		}
		id := strings.TrimSpace(spec.SubscribeID)
		if id == "" {
			return fmt.Errorf("consume: subscribe_id is required for %s", spec.EventKey)
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("consume: duplicate subscribe_id %q", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func readManySession(ctx context.Context, index int, session *manySession, out chan<- manyFrame) {
	for {
		raw, err := session.r.Read()
		frame := manyFrame{index: index, raw: raw, err: err}
		select {
		case out <- frame:
		case <-ctx.Done():
			return
		}
		if err != nil {
			return
		}
		if typ, _ := transport.PeekType(raw); typ == transport.FrameTypeBye {
			return
		}
	}
}

func sendManyBye(sessions []*manySession, active map[int]struct{}, reason string) {
	indices := make([]int, 0, len(active))
	for index := range active {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	for _, index := range indices {
		_ = sessions[index].w.WriteJSON(transport.Bye{Type: transport.FrameTypeBye, Reason: reason})
	}
}

func firstActiveSession(active map[int]struct{}) int {
	first := -1
	for index := range active {
		if first == -1 || index < first {
			first = index
		}
	}
	return first
}
