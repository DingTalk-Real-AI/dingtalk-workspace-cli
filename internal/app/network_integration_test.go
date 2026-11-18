// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/apiclient"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageNetworkCLIAPIThroughRealCONNECT(t *testing.T) {
	clearNetworkEnv(t)
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Cleanup(CloseFileLogger)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "proxy-test"}, DNSNames: []string{"api.dingtalk.com"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DWS_CA_BUNDLE", ca)
	var origins atomic.Int32
	origin := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origins.Add(1)
		if r.Host != "api.dingtalk.com" || r.Header.Get("x-acs-dingtalk-access-token") != "fixture-token" {
			t.Error("origin or token changed")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"probe":"through-selected-proxy"}`)
	}))
	origin.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	origin.StartTLS()
	defer origin.Close()
	var connects, direct atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" || r.Host != "api.dingtalk.com:443" {
			t.Error("unexpected proxy request")
			w.WriteHeader(400)
			return
		}
		connects.Add(1)
		up, err := net.Dial("tcp", origin.Listener.Addr().String())
		if err != nil {
			t.Error(err)
			return
		}
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			up.Close()
			return
		}
		defer conn.Close()
		defer up.Close()
		rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		rw.Flush()
		done := make(chan struct{})
		go func() { io.Copy(up, rw); up.Close(); close(done) }()
		io.Copy(conn, up)
		conn.Close()
		<-done
	}))
	defer proxy.Close()
	purl, _ := url.Parse(proxy.URL)
	testseam.Swap(t, &newRawAPIClient, func(token, baseURL string) *apiclient.APIClient {
		c := apiclient.NewClient(token, baseURL)
		c.HTTPClient.Transport.(*http.Transport).DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			if address != purl.Host {
				direct.Add(1)
				return nil, fmt.Errorf("direct origin forbidden")
			}
			return (&net.Dialer{}).DialContext(ctx, network, address)
		}
		return c
	})
	root := NewRootCommand()
	root.SetArgs([]string{"--proxy", proxy.URL, "--token", "fixture-token", "api", "GET", "/v1.0/probe"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatal(err, out.String())
	}
	if connects.Load() != 1 || origins.Load() != 1 || direct.Load() != 0 || !strings.Contains(out.String(), "through-selected-proxy") {
		t.Fatalf("CONNECT=%d origin=%d direct=%d output=%s", connects.Load(), origins.Load(), direct.Load(), out.String())
	}
}
