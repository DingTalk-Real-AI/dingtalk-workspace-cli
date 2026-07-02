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

package audit

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
)

// HTTPForwarder must POST a valid event with schema header and bearer to the
// organization's endpoint.
func TestHTTPForwarder_PostsToOrgSink(t *testing.T) {
	var gotBody []byte
	var gotAuth, gotSchema string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = bytesReadAll(r)
		gotAuth = r.Header.Get("Authorization")
		gotSchema = r.Header.Get("X-Dws-Audit-Schema")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	f := NewHTTPForwarder(srv.URL, "org-token")
	if err := f.Emit(sampleEvent()); err != nil {
		t.Fatalf("forward failed: %v", err)
	}
	if gotAuth != "Bearer org-token" {
		t.Errorf("missing/short bearer: %q", gotAuth)
	}
	if gotSchema != SchemaVersion {
		t.Errorf("missing schema header: %q", gotSchema)
	}
	var back Event
	if err := json.Unmarshal(gotBody, &back); err != nil {
		t.Fatalf("server got invalid JSON: %v", err)
	}
	if back.TraceID != "trace-abc" {
		t.Errorf("event lost in transit: %+v", back)
	}
}

// Non-2xx from the sink surfaces as an error (so MultiSink can log it) but the
// local file already holds the record.
func TestHTTPForwarder_Non2xxErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	if err := NewHTTPForwarder(srv.URL, "").Emit(sampleEvent()); err == nil {
		t.Fatal("expected error on 500")
	}
}

// BuildSink: disabled => NopSink, no emission anywhere.
func TestBuildSink_DisabledIsNop(t *testing.T) {
	t.Setenv(EnvEnabled, "")
	if _, ok := BuildSink(&bytes.Buffer{}).(NopSink); !ok {
		t.Fatal("disabled audit must yield NopSink")
	}
}

// BuildSink: enabled + forward URL => file AND forwarder, forwarder redacted.
func TestBuildSink_FileAndForward(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := bytesReadAll(r)
		if bytes.Contains(body, []byte("C02SN12345")) {
			t.Error("minimal-redacted forward leaked raw serial")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv(EnvEnabled, "true")
	t.Setenv(EnvForwardURL, srv.URL)
	t.Setenv(EnvForwardRedact, "minimal")

	var file bytes.Buffer
	sink := BuildSink(&file)
	if _, ok := sink.(*MultiSink); !ok {
		t.Fatalf("expected MultiSink, got %T", sink)
	}
	if err := sink.Emit(sampleEvent()); err != nil {
		t.Fatal(err)
	}
	// Local file keeps the FULL record (serial present) — trust boundary is local.
	if !bytes.Contains(file.Bytes(), []byte("C02SN12345")) {
		t.Error("local file should keep full record verbatim")
	}
}

// Device fingerprint is gated: off => no id/serial; OS is always present.
func TestCollectDevice_OptInGate(t *testing.T) {
	off := CollectDevice(false)
	if off.DeviceID != "" || off.SerialNo != "" {
		t.Errorf("fingerprint off must not collect device id/serial: %+v", off)
	}
	if off.OS == "" {
		t.Error("OS should always be set")
	}
	// On darwin (the dev/customer platform) opt-in must actually yield a
	// machine UUID — proves the ioreg path works, not just compiles.
	if runtime.GOOS == "darwin" {
		on := CollectDevice(true)
		if on.DeviceID == "" {
			t.Error("darwin opt-in should return IOPlatformUUID")
		}
	}
}

// helpers (avoid importing io just for ReadAll in this file).
func bytesReadAll(r *http.Request) ([]byte, error) {
	var b bytes.Buffer
	_, err := b.ReadFrom(r.Body)
	return b.Bytes(), err
}
