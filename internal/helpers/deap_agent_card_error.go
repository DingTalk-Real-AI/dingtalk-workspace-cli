// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"errors"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type employeeCardDiagnostic struct {
	Code             string `json:"code,omitempty"`
	Category         string `json:"category,omitempty"`
	Reason           string `json:"reason,omitempty"`
	Stage            string `json:"stage,omitempty"`
	ExecutionStarted *bool  `json:"execution_started,omitempty"`
	Retryable        *bool  `json:"retryable,omitempty"`
	ExecutionID      string `json:"execution_id,omitempty"`
	TraceID          string `json:"trace_id,omitempty"`
	Message          string `json:"message,omitempty"`
	Cause            string `json:"cause,omitempty"`
}

var employeeCardDiagnosticURL = regexp.MustCompile(`https?://[^\s"'<>]+`)
var employeeCardDiagnosticSecret = regexp.MustCompile(`(?i)(bearer\s+[^\s"'<>]+|(?:[a-z0-9_-]*(?:token|secret|password|credential|api[_-]?key)|authorization|cookie)\s*["']?\s*[:=]\s*["']?[^\s"',;]+)`)
var employeeCardDiagnosticHeader = regexp.MustCompile(`(?i)(authorization|cookie)\s*["']?\s*[:=]\s*[^;\r\n]*`)
var employeeCardDiagnosticScalar = regexp.MustCompile(`^[a-zA-Z0-9_./:-]{1,128}$`)

func employeeCardSafeDiagnosticText(s string) string {
	// 删除完整 URL 的身份、路径及查询参数，再过滤错误字符串中的凭据赋值。
	s = employeeCardDiagnosticURL.ReplaceAllStringFunc(s, func(raw string) string {
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			return "[redacted-url]"
		}
		return u.Scheme + "://" + u.Hostname() + "/[redacted]"
	})
	s = employeeCardDiagnosticHeader.ReplaceAllString(s, "[redacted]")
	s = employeeCardDiagnosticSecret.ReplaceAllString(s, "[redacted]")
	s = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return ' '
		}
		return r
	}, s)
	if len(s) > 1024 {
		s = s[:1024]
		for !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
	}
	return s
}

func employeeCardSafeScalar(s string) string {
	if employeeCardDiagnosticScalar.MatchString(s) {
		return s
	}
	return ""
}

func employeeCardTransportReason(reason string) bool {
	switch reason {
	case "tls_certificate_untrusted", "tls_certificate_invalid", "dns_resolution_failed", "connection_refused", "request_failed", "request_timeout", "request_cancelled", "http_client_timeout", "tls_timeout", "io_timeout":
		return true
	}
	return false
}

func employeeCardCanRecover(err error) bool {
	var classified *employeeRunError
	if !errors.As(err, &classified) || classified.Code != "card_request_not_sent" || classified.CardDiagnostic == nil {
		return false
	}
	d := classified.CardDiagnostic
	return d.ExecutionStarted != nil && !*d.ExecutionStarted && d.Retryable != nil && *d.Retryable
}

func (r *employeeRuntime) preserveCardResult(e employeeEvent, record *employeeTaskRecord, answer string) error {
	if strings.TrimSpace(answer) == "" {
		return nil
	}
	relative := filepath.Join("card-results", filepath.Base(r.recordPath(e)))
	// 正文不进入任务账本或运行日志，沿用原子写入和 0700/0600 的访问权限。
	if err := writeEmployeeJSON(filepath.Join(r.dir, relative), map[string]any{
		"bizId": record.BizID, "surfaceId": record.SurfaceID, "answer": answer, "updatedAt": time.Now(),
	}); err != nil {
		return employeeTerminal("ledger_unavailable")
	}
	record.CardResultFile = relative
	return nil
}

type employeeCardErrorEnvelope struct {
	Success *bool `json:"success"`
	Error   struct {
		Category         string `json:"category"`
		Type             string `json:"type"`
		ExecutionStarted *bool  `json:"execution_started"`
		Retryable        *bool  `json:"retryable"`
		Reason           string `json:"reason"`
		Stage            string `json:"stage"`
		Message          string `json:"message"`
		Cause            string `json:"cause"`
		ExecutionID      string `json:"execution_id"`
		TraceID          string `json:"trace_id"`
		Details          struct {
			ExecutionID string `json:"execution_id"`
		} `json:"details"`
	} `json:"error"`
}
