// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package busctl

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

// ErrConsumerStopUnsupported lets callers fall back to the legacy process
// signal when an already-running bus predates the targeted stop protocol.
var ErrConsumerStopUnsupported = errors.New("busctl: targeted consumer stop is unsupported")

var consumerStopDial = func(endpoint string) (net.Conn, error) { return transport.Dial(endpoint) }

// StopConsumers asks a running bus to close only consumers matching the
// supplied personal subscription IDs.
func StopConsumers(endpoint string, subscribeIDs []string) (transport.ConsumerStopResp, error) {
	var empty transport.ConsumerStopResp
	if strings.TrimSpace(endpoint) == "" {
		return empty, errors.New("busctl: consumer stop endpoint is required")
	}
	ids := make([]string, 0, len(subscribeIDs))
	seen := map[string]struct{}{}
	for _, id := range subscribeIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return empty, errors.New("busctl: at least one subscribe_id is required")
	}

	conn, err := consumerStopDial(endpoint)
	if err != nil {
		return empty, fmt.Errorf("busctl: dial bus for consumer stop: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(DefaultStatusRPCTimeout))
	w := transport.NewWriter(conn)
	r := transport.NewReader(conn)
	if err := w.WriteJSON(transport.Hello{
		Type:        transport.FrameTypeHello,
		ConsumerPID: os.Getpid(),
		Role:        transport.HelloRoleConsumerStop,
	}); err != nil {
		return empty, fmt.Errorf("busctl: write consumer stop hello: %w", err)
	}
	if err := w.WriteJSON(transport.ConsumerStopReq{
		Type:         transport.FrameTypeConsumerStopReq,
		SubscribeIDs: ids,
	}); err != nil {
		return empty, fmt.Errorf("busctl: write consumer stop request: %w", err)
	}
	var resp transport.ConsumerStopResp
	if err := r.ReadJSON(&resp); err != nil {
		return empty, fmt.Errorf("busctl: read consumer stop response: %w", err)
	}
	if resp.Type != transport.FrameTypeConsumerStopResp {
		return empty, fmt.Errorf("%w: unexpected response type %q", ErrConsumerStopUnsupported, resp.Type)
	}
	return resp, nil
}
