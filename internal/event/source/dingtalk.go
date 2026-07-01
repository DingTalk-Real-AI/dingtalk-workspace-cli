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

// Package source wraps the open-dingtalk Stream SDK and exposes a small
// blocking Start interface plus a connection state machine. The SDK is the
// only place in the bus that talks to the cloud; the rest of the bus stays
// vendor-agnostic and would be drop-in replaceable for a different Stream
// provider.
package source

import (
	"context"
	"errors"
	"time"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/client"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/event"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/payload"
)

// Config carries the credentials and behavioural knobs needed to construct a
// DingtalkSource. ClientID/ClientSecret are required; everything else has a
// safe default.
type Config struct {
	ClientID     string
	ClientSecret string
	// PortalTicket switches the source to the portal-managed user Stream
	// ticket flow. When set, Start fetches endpoint+ticket over HTTP and
	// dials the returned WebSocket directly instead of asking the SDK to
	// open an app-credential connection.
	PortalTicket *PortalTicketConfig
	// Now is injected for tests. Defaults to time.Now when nil.
	Now func() time.Time
}

// SourceOptions is the functional-option type reserved for future overlay
// extensions (e.g. inject trace IDs into emit, swap the underlying client
// for a fake). v1 takes no options; the type exists so callers can write
// `New(cfg)` today and `New(cfg, WithFoo())` tomorrow without an API break.
// See plan §7 Edition 扩展点.
type SourceOption func(*sourceConfig)

type sourceConfig struct {
	// reserved for v2 hooks (pre-emit interceptor, etc.)
}

// DingtalkSource is the cloud-side adapter. It owns one StreamClient and one
// state Machine; lifecycle is bounded by the context passed to Start.
type DingtalkSource struct {
	cfg     Config
	machine *Machine
	cli     *client.StreamClient
}

// New constructs a DingtalkSource. Returns an error if required Config
// fields are missing — keep the boundary tight so misconfiguration fails
// loudly rather than at first-event time.
func New(cfg Config, _ ...SourceOption) (*DingtalkSource, error) {
	if cfg.ClientID == "" {
		return nil, errors.New("source: ClientID is required")
	}
	if cfg.PortalTicket == nil && cfg.ClientSecret == "" {
		return nil, errors.New("source: ClientSecret is required")
	}
	if cfg.PortalTicket != nil {
		if err := cfg.PortalTicket.Valid(); err != nil {
			return nil, err
		}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	m := NewMachine()
	m.now = cfg.Now
	return &DingtalkSource{cfg: cfg, machine: m}, nil
}

// State returns the current Snapshot of the connection state machine.
func (s *DingtalkSource) State() Snapshot { return s.machine.Snapshot() }

// Start opens the Stream WebSocket and blocks until ctx is cancelled or a
// fatal SDK error occurs. Events are delivered to emit synchronously from
// the SDK callback goroutine — emit MUST return immediately (the SDK's
// processLoop is single-goroutine; see plan invariant #1 and P0 §1 row 2).
//
// Blocking semantics: the underlying StreamClient.Start returns as soon as
// dial succeeds (it spawns its own processLoop goroutine), so we wait on
// ctx.Done() before returning. cli.Close() is always called on exit; ctx.Err
// is returned for context cancellation, otherwise the underlying error.
func (s *DingtalkSource) Start(ctx context.Context, emit dwsevent.EmitFn) error {
	if emit == nil {
		return errors.New("source: emit is required")
	}
	if s.cli != nil {
		return errors.New("source: Start called twice")
	}
	if s.cfg.PortalTicket != nil {
		return s.startPortalTicket(ctx, emit)
	}

	options := []client.ClientOption{
		client.WithAppCredential(client.NewAppCredentialConfig(s.cfg.ClientID, s.cfg.ClientSecret)),
	}
	s.cli = client.NewStreamClient(options...)
	s.cli.RegisterAllEventRouter(s.makeHandler(emit))

	s.machine.OnConnecting()
	if err := s.cli.Start(ctx); err != nil {
		s.machine.OnStopped()
		return err
	}
	s.machine.OnConnected()

	<-ctx.Done()
	s.cli.Close()
	s.machine.OnStopped()
	return ctx.Err()
}

// makeHandler returns the IFrameHandler closure the SDK invokes for every
// inbound event. The closure:
//  1. parses the EventHeader from the raw DataFrame,
//  2. builds a RawEvent with all 5 SDK header fields + payload + receive
//     time + full header map (passthrough),
//  3. calls emit (non-blocking by contract),
//  4. updates the state machine,
//  5. returns SUCCESS (LATER is reserved for future explicit retry policy;
//     v1 always Success and relies on dedup, see P0 §1 row 6).
func (s *DingtalkSource) makeHandler(emit dwsevent.EmitFn) func(context.Context, *payload.DataFrame) (*payload.DataFrameResponse, error) {
	return func(_ context.Context, df *payload.DataFrame) (*payload.DataFrameResponse, error) {
		hdr := event.NewEventHeaderFromDataFrame(df)
		raw := &dwsevent.RawEvent{
			EventID:           hdr.EventId,
			EventBornTime:     hdr.EventBornTime,
			EventCorpID:       hdr.EventCorpId,
			EventType:         hdr.EventType,
			EventUnifiedAppID: hdr.EventUnifiedAppId,
			Data:              df.Data,
			Headers:           copyHeaders(df.Headers),
			ReceivedAt:        s.cfg.Now().UTC(),
		}
		emit(raw)
		s.machine.OnEvent()

		resp := payload.NewSuccessDataFrameResponse()
		if err := resp.SetJson(event.NewEventProcessResultSuccess()); err != nil {
			return nil, err
		}
		return resp, nil
	}
}

func copyHeaders(h payload.DataFrameHeader) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		out[k] = v
	}
	return out
}
