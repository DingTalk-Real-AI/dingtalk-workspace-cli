// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/busctl"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/gorilla/websocket"
	streamevent "github.com/open-dingtalk/dingtalk-stream-sdk-go/event"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/payload"
	"github.com/spf13/cobra"
)

const personalAppKeyChildEnv = "DWS_EVENT_APPKEY_E2E_CHILD"
const personalAppKeyCanary = "appkey-e2e-bearer-canary-a601e4"

// Only credential acquisition is synthetic. The child executes the production
// _bus command, ticket exchange, WebSocket source and IPC lifecycle.
func personalAppKeyTestHooks(explicit bool) *edition.Hooks {
	return &edition.Hooks{
		LoadToken: func(string) ([]byte, error) {
			if explicit {
				return nil, errors.New("explicit bearer must not read stored tokens")
			}
			return json.Marshal(&authpkg.TokenData{AccessToken: personalAppKeyCanary, ExpiresAt: time.Now().Add(time.Hour)})
		},
		SaveToken: func(string, []byte) error { return errors.New("event must not save token") },
	}
}

func runPersonalAppKeyE2EChild() (int, bool) {
	mode := os.Getenv(personalAppKeyChildEnv)
	if mode == "" {
		return 0, false
	}
	if len(os.Args) < 3 || os.Args[1] != "event" || os.Args[2] != "_bus" {
		return 90, true
	}
	if strings.Contains(strings.Join(os.Args, " "), personalAppKeyCanary) || strings.Contains(strings.Join(os.Environ(), " "), personalAppKeyCanary) {
		return 91, true
	}
	edition.Override(personalAppKeyTestHooks(mode == "explicit"))
	cmd := newEventBusCommand()
	cmd.SetArgs(append(os.Args[3:], "--idle-timeout", "1s"))
	err := cmd.ExecuteContext(context.Background())
	if err != nil && !errors.Is(err, context.Canceled) {
		return 92, true
	}
	return 0, true
}

func TestCrossPlatformCoveragePersonalEventAppKeyDetachedLifecycle(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		mode := map[bool]string{false: "stored", true: "explicit"}[explicit]
		t.Run(mode, func(t *testing.T) {
			base := ""
			if runtime.GOOS != "windows" {
				base = "/tmp"
			} // keep Unix socket paths below the platform limit
			dir, err := os.MkdirTemp(base, "dws-appkey-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(dir) })
			t.Setenv("DWS_CONFIG_DIR", dir)
			t.Setenv("DWS_CLIENT_ID", "")
			t.Setenv("DWS_CLIENT_SECRET", "")
			t.Setenv(personalAppKeyChildEnv, mode)
			prev := edition.Get()
			edition.Override(personalAppKeyTestHooks(explicit))
			t.Cleanup(func() { edition.Override(prev) })
			prevProfile := authpkg.RuntimeProfile()
			authpkg.SetRuntimeProfile("")
			t.Cleanup(func() { authpkg.SetRuntimeProfile(prevProfile) })
			var metadata, tickets, created, cancelled atomic.Int32
			var mu sync.Mutex
			subs := map[string]string{}
			var wsURL string
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			mux := http.NewServeMux()
			mux.HandleFunc("/cli/clientId", func(w http.ResponseWriter, r *http.Request) {
				metadata.Add(1)
				if r.Header.Get("x-user-access-token") != "" || r.Header.Get("Authorization") != "" {
					t.Error("metadata carried bearer")
				}
				_, _ = io.WriteString(w, `{"success":true,"result":"managed-e2e-client"}`)
			})
			mux.HandleFunc("/dws/subscription/user", func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					ClientID string `json:"clientId"`
					EventKey string `json:"eventKey"`
				}
				if json.NewDecoder(r.Body).Decode(&body) != nil || body.ClientID != "managed-e2e-client" || r.Header.Get("x-user-access-token") != personalAppKeyCanary {
					t.Error("subscription identity changed")
				}
				id := fmt.Sprintf("sub-%d", created.Add(1))
				mu.Lock()
				subs[id] = body.EventKey
				mu.Unlock()
				_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": map[string]any{"subId": id, "eventKey": body.EventKey, "sourceId": "open"}})
			})
			mux.HandleFunc("/dws/subscription/cancel", func(w http.ResponseWriter, r *http.Request) {
				cancelled.Add(1)
				if r.Header.Get("x-user-access-token") != personalAppKeyCanary {
					t.Error("cleanup bearer changed")
				}
				_, _ = io.WriteString(w, `{"success":true,"result":true}`)
			})
			mux.HandleFunc("/stream/connections/ticket", func(w http.ResponseWriter, r *http.Request) {
				tickets.Add(1)
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				if r.Header.Get("x-user-access-token") != personalAppKeyCanary || body["mode"] != "normal" || body["clientSecret"] != nil {
					t.Error("ticket bearer/mode changed or secret sent")
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": map[string]any{"endpoint": wsURL, "ticket": "test-ticket"}})
			})
			mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
				upgrader := websocket.Upgrader{}
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer conn.Close()
				for n := 0; ; n++ {
					select {
					case <-ctx.Done():
						return
					case <-time.After(30 * time.Millisecond):
					}
					mu.Lock()
					snapshot := make(map[string]string, len(subs))
					for id, key := range subs {
						snapshot[id] = key
					}
					mu.Unlock()
					for id, key := range snapshot {
						_ = conn.SetWriteDeadline(time.Now().Add(time.Second))
						if err := conn.WriteJSON(payload.DataFrame{Type: "event", Headers: payload.DataFrameHeader{
							payload.DataFrameHeaderKMessageId: fmt.Sprintf("msg-%s-%d", id, n), streamevent.DataFrameHeaderKEventId: fmt.Sprintf("evt-%s-%d", id, n),
							streamevent.DataFrameHeaderKEventType: key, "subscribeId": id, "sourceId": "open"}, Data: `{"message":{"text":"e2e-event"}}`}); err != nil {
							return
						}
						_ = conn.SetReadDeadline(time.Now().Add(time.Second))
						var ack payload.DataFrameResponse
						if err := conn.ReadJSON(&ack); err != nil {
							return
						}
					}
				}
			})
			srv := httptest.NewServer(mux)
			defer srv.Close()
			wsURL = "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
			if err := os.WriteFile(filepath.Join(dir, "mcp_url"), []byte(srv.URL), 0600); err != nil {
				t.Fatal(err)
			}
			identity := personal.Identity{LocalSubject: personalTokenSubject("access", personalAppKeyCanary), ClientID: "managed-e2e-client", SourceID: "open"}
			hash := dwsevent.IdentityHash(identity.Key())
			workDir := eventWorkDir(dir, "open", dwsevent.SourceKindPersonalStream, hash)
			endpoint := defaultIPCEndpoint(workDir, "open", dwsevent.SourceKindPersonalStream, hash)
			stop := func() {
				err := busctl.Stop(busctl.StopConfig{WorkDir: workDir, IPCEndpoint: endpoint, Timeout: 3 * time.Second})
				if err != nil && !errors.Is(err, busctl.ErrNotRunning) {
					t.Errorf("stop bus: %v", err)
				}
			}
			t.Cleanup(stop)
			var out, stderr bytes.Buffer
			cmd := &cobra.Command{}
			cmd.SetContext(ctx)
			cmd.SetOut(&out)
			cmd.SetErr(&stderr)
			opts := personalConsumeOptions{EventKey: personal.EventMention, EventKeys: []string{personal.EventMention, personal.EventAllSingleChat}, Common: commonConsumeOptions{MaxEvents: 2, Duration: 5 * time.Second}}
			if explicit {
				opts.ExplicitToken = personalAppKeyCanary
			}
			opts.Common.DryRun = true
			if err := runPersonalEventConsume(cmd, opts); err != nil {
				t.Fatalf("dry-run: %v", err)
			}
			if metadata.Load() != 1 || created.Load() != 0 || tickets.Load() != 0 {
				t.Fatal("dry-run must fetch metadata once without subscription or stream side effects")
			}
			metadata.Store(0)
			stderr.Reset()
			opts.Common.DryRun = false
			if err := runPersonalEventConsume(cmd, opts); err != nil {
				t.Fatalf("lifecycle: %v; stderr=%s", err, stderr.String())
			}
			if metadata.Load() != 1 || tickets.Load() < 1 || created.Load() != 2 || cancelled.Load() != 2 {
				t.Fatalf("metadata=%d tickets=%d created=%d cancelled=%d", metadata.Load(), tickets.Load(), created.Load(), cancelled.Load())
			}
			if !strings.Contains(stderr.String(), "[event] ready") || !strings.Contains(out.String(), "e2e-event") {
				t.Fatalf("missing ready/event: stderr=%s stdout=%s", stderr.String(), out.String())
			}
			stop()
			if _, err := os.Stat(filepath.Join(dir, "app.json")); !os.IsNotExist(err) {
				t.Fatal("event persisted app metadata")
			}
			if _, err := os.Stat(filepath.Join(dir, "profiles.json")); !os.IsNotExist(err) {
				t.Fatal("event persisted profile")
			}
			if err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !entry.Type().IsRegular() {
					return nil
				}
				body, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				if bytes.Contains(body, []byte(personalAppKeyCanary)) {
					t.Errorf("bearer persisted in %s", entry.Name())
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
