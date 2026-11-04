// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package transport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"net/http/httptrace"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageRequestFailureSubmissionEvidence(t *testing.T) {
	cert := &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}
	for _, scenario := range []string{"certificate", "invalid-certificate", "unknown", "unknown-then-certificate", "certificate-text-only", "written-then-certificate"} {
		t.Run(scenario, func(t *testing.T) {
			calls := 0
			client := edgeClient(func(req *http.Request) (*http.Response, error) {
				calls++
				if scenario == "written-then-certificate" {
					httptrace.ContextClientTrace(req.Context()).WroteHeaders()
				}
				if scenario == "unknown" || scenario == "unknown-then-certificate" && calls == 1 {
					return nil, io.ErrUnexpectedEOF
				}
				if scenario == "certificate-text-only" {
					return nil, errors.New("x509: certificate signed by unknown authority")
				}
				if scenario == "invalid-certificate" {
					return nil, &tls.CertificateVerificationError{Err: x509.HostnameError{Certificate: &x509.Certificate{DNSNames: []string{"expected.invalid"}}, Host: "other.invalid"}}
				}
				return nil, cert
			})
			if scenario == "unknown-then-certificate" {
				client.MaxRetries = 1
			}
			_, err := client.doWithRetry(context.Background(), "https://mcp-gw.dingtalk.com/server/test", nil, "tools/call")
			var typed *apperrors.Error
			if !errors.As(err, &typed) {
				t.Fatal("丢失结构化错误")
			}
			if scenario == "certificate" || scenario == "invalid-certificate" {
				wantReason := "tls_certificate_untrusted"
				if scenario == "invalid-certificate" {
					wantReason = "tls_certificate_invalid"
				}
				if typed.ExecutionStarted == nil || *typed.ExecutionStarted || typed.Reason != wantReason || typed.FailureStage != "request" {
					t.Fatalf("TLS 失败缺少未提交证据：%+v", typed)
				}
			} else if typed.ExecutionStarted != nil && !*typed.ExecutionStarted {
				t.Fatal("未知提交结果或错误文本不能授权重放")
			}
		})
	}
}
