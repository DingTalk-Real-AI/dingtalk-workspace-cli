// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package source

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/gorilla/websocket"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/payload"
)

func TestCrossPlatformCoveragePersonalSystemFrames(t *testing.T) {
	for _, topic := range []string{"ping", "disconnect", "unsupported"} {
		t.Run(topic, func(t *testing.T) {
			s := &PersonalSource{machine: NewMachine()}
			type result struct {
				err   error
				emits int
			}
			done := make(chan result, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upgrader := websocket.Upgrader{}
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					done <- result{err: err}
					return
				}
				defer conn.Close()
				count := 0
				err = s.handleFrame(conn, []byte(`{"type":"SYSTEM","headers":{"messageId":"system-fixture","topic":"`+topic+`"},"data":"{\"opaque\":1}"}`), func(*dwsevent.RawEvent) { count++ })
				done <- result{err: err, emits: count}
			}))
			defer srv.Close()
			conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			var ack payload.DataFrameResponse
			if err := conn.ReadJSON(&ack); err != nil {
				t.Fatal(err)
			}
			r := <-done
			if r.emits != 0 || !s.State().LastEventAt.IsZero() {
				t.Fatalf("系统帧进入业务事件链路: emits=%d", r.emits)
			}
			if ack.GetHeader(payload.DataFrameHeaderKMessageId) != "system-fixture" {
				t.Fatal("ACK 未关联原系统帧")
			}
			if topic == "ping" && (r.err != nil || ack.Data != `{"opaque":1}`) {
				t.Fatalf("ping 必须原样回显 data: %+v err=%v", ack, r.err)
			}
			if topic == "disconnect" && !isRetryablePersonalError(r.err) {
				t.Fatalf("disconnect 必须请求重连: %v", r.err)
			}
			if topic == "unsupported" && (r.err != nil || ack.Code != 404) {
				t.Fatalf("未知系统帧应回报未处理: %+v %v", ack, r.err)
			}
		})
	}
}
