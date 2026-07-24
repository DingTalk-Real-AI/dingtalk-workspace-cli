// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package busctl

import (
	"errors"
	"net"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

func TestStopConsumersRoundTrip(t *testing.T) {
	client, server := net.Pipe()
	oldDial := consumerStopDial
	consumerStopDial = func(string) (net.Conn, error) { return client, nil }
	t.Cleanup(func() {
		consumerStopDial = oldDial
		_ = server.Close()
	})

	requestCh := make(chan transport.ConsumerStopReq, 1)
	errCh := make(chan error, 1)
	go func() {
		r := transport.NewReader(server)
		w := transport.NewWriter(server)
		var hello transport.Hello
		if err := r.ReadJSON(&hello); err != nil {
			errCh <- err
			return
		}
		if hello.Role != transport.HelloRoleConsumerStop {
			errCh <- errors.New("unexpected hello role")
			return
		}
		var req transport.ConsumerStopReq
		if err := r.ReadJSON(&req); err != nil {
			errCh <- err
			return
		}
		requestCh <- req
		errCh <- w.WriteJSON(transport.ConsumerStopResp{
			Type:     transport.FrameTypeConsumerStopResp,
			Stopped:  []string{"sub-a"},
			NotFound: []string{"sub-b"},
		})
	}()

	resp, err := StopConsumers("pipe", []string{" sub-a ", "sub-a", "", "sub-b"})
	if err != nil {
		t.Fatalf("StopConsumers() error = %v", err)
	}
	if !reflect.DeepEqual(resp.Stopped, []string{"sub-a"}) || !reflect.DeepEqual(resp.NotFound, []string{"sub-b"}) {
		t.Fatalf("response = %#v", resp)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("server error = %v", err)
	}
	req := <-requestCh
	if req.Type != transport.FrameTypeConsumerStopReq || !reflect.DeepEqual(req.SubscribeIDs, []string{"sub-a", "sub-b"}) {
		t.Fatalf("request = %#v", req)
	}
}

func TestStopConsumersDetectsLegacyBus(t *testing.T) {
	client, server := net.Pipe()
	oldDial := consumerStopDial
	consumerStopDial = func(string) (net.Conn, error) { return client, nil }
	t.Cleanup(func() {
		consumerStopDial = oldDial
		_ = server.Close()
	})

	go func() {
		r := transport.NewReader(server)
		w := transport.NewWriter(server)
		var hello transport.Hello
		_ = r.ReadJSON(&hello)
		var req transport.ConsumerStopReq
		_ = r.ReadJSON(&req)
		_ = w.WriteJSON(transport.HelloAck{Type: transport.FrameTypeHelloAck, BusPID: 1})
	}()

	_, err := StopConsumers("pipe", []string{"sub-a"})
	if !errors.Is(err, ErrConsumerStopUnsupported) {
		t.Fatalf("StopConsumers() error = %v, want ErrConsumerStopUnsupported", err)
	}
}

func TestStopConsumersValidatesInputAndDialErrors(t *testing.T) {
	if _, err := StopConsumers("", []string{"sub-a"}); err == nil {
		t.Fatal("empty endpoint succeeded")
	}
	if _, err := StopConsumers("pipe", []string{"", "  "}); err == nil {
		t.Fatal("empty subscriptions succeeded")
	}

	wantErr := errors.New("dial")
	oldDial := consumerStopDial
	consumerStopDial = func(string) (net.Conn, error) { return nil, wantErr }
	t.Cleanup(func() { consumerStopDial = oldDial })
	if _, err := StopConsumers("pipe", []string{"sub-a"}); !errors.Is(err, wantErr) {
		t.Fatalf("dial error = %v", err)
	}
}
