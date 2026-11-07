// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package transport

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
)

// 不根据错误文案推断请求是否提交；这些类型证明失败发生在 HTTP 发送之前。
func requestFailureBeforeSend(err error) bool {
	if certificateFailureReason(err) != "" {
		return true
	}
	var dns *net.DNSError
	var op *net.OpError
	return errors.As(err, &dns) || errors.As(err, &op) && op.Op == "dial"
}

func certificateFailureReason(err error) string {
	var unknown x509.UnknownAuthorityError
	if errors.As(err, &unknown) {
		return "tls_certificate_untrusted"
	}
	var verification *tls.CertificateVerificationError
	var hostname x509.HostnameError
	var invalid x509.CertificateInvalidError
	if errors.As(err, &verification) || errors.As(err, &hostname) || errors.As(err, &invalid) {
		return "tls_certificate_invalid"
	}
	return ""
}
