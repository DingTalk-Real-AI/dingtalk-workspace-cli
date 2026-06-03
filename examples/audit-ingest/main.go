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

// Command audit-ingest is a minimal reference receiver for dws audit forwarding
// (the DWS_AUDIT_FORWARD_URL target). It implements the exact contract the dws
// HTTP forwarder speaks, so you can validate the whole chain end-to-end before
// wiring a real sink.
//
// Contract (see internal/audit/forward.go):
//
//	POST /
//	Content-Type: application/json
//	Authorization: Bearer <token>     (optional; enforced here if -token set)
//	X-Dws-Audit-Schema: <n>
//	Body: one audit Event JSON
//	-> respond 2xx
//
// Local-validation build: it appends each accepted event to a JSONL file. To
// ship audit to Aliyun SLS instead, replace store.write() with an SLS PutLogs
// call (one function — see README.md in this directory).
//
// Run:
//
//	go run ./examples/audit-ingest -addr :8088 -token secret -out ingest.jsonl
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
)

func main() {
	addr := flag.String("addr", ":8088", "listen address")
	token := flag.String("token", "", "expected Bearer token (empty = no auth check)")
	out := flag.String("out", "ingest.jsonl", "output JSONL file (local-validation sink)")
	flag.Parse()

	sink, err := newFileSink(*out)
	if err != nil {
		log.Fatalf("open sink: %v", err)
	}
	defer sink.Close()

	h := &handler{token: *token, sink: sink}
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	http.Handle("/", h)

	log.Printf("audit-ingest listening on %s (auth=%v, out=%s)", *addr, *token != "", *out)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

type handler struct {
	token string
	sink  *fileSink
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.token != "" && r.Header.Get("Authorization") != "Bearer "+h.token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	// Validate it is well-formed JSON before accepting (reject garbage early).
	var probe map[string]any
	if err := json.Unmarshal(body, &probe); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := h.sink.write(body); err != nil {
		http.Error(w, "sink error", http.StatusInternalServerError)
		return
	}
	log.Printf("accepted event: trace_id=%v schema=%s command=%v/%v outcome=%v",
		probe["trace_id"], r.Header.Get("X-Dws-Audit-Schema"), probe["command"], probe["subcommand"], probe["outcome"])
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintln(w, `{"ok":true}`)
}

// fileSink appends one JSON object per line. Swap this for SLS PutLogs to ship
// to Aliyun Log Service instead — see README.md.
type fileSink struct {
	mu sync.Mutex
	f  *os.File
}

func newFileSink(path string) (*fileSink, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	return &fileSink{f: f}, nil
}

func (s *fileSink) write(line []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.f.Write(append(line, '\n')); err != nil {
		return err
	}
	return s.f.Sync()
}

func (s *fileSink) Close() error { return s.f.Close() }
