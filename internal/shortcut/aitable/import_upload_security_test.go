// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

const importUploadTestObjectPath = "/notable/mcp_import_temp/0123456789abcdef0123456789abcdef/data.xlsx"

func TestCrossPlatformCoverageImportUploadRejectsOtherBucketsBeforePUT(t *testing.T) {
	file := filepath.Join(t.TempDir(), "data.xlsx")
	if err := os.WriteFile(file, []byte("private workbook bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	putCalls := 0
	testseam.Swap(t, &importHTTPDo, func(*http.Request) (*http.Response, error) {
		putCalls++
		return nil, errors.New("unexpected PUT")
	})
	for _, target := range []string{
		"attacker.example" + importUploadTestObjectPath,
		"attacker.oss-cn-zhangjiakou.aliyuncs.com" + importUploadTestObjectPath,
		"attacker.cn-zhangjiakou.oss.aliyuncs.com" + importUploadTestObjectPath,
		"attacker.oss-ap-southeast-1.aliyuncs.com" + importUploadTestObjectPath,
		"attacker.ap-southeast-1.oss.aliyuncs.com" + importUploadTestObjectPath,
		"attacker.alidocs-notable.cn-zhangjiakou.oss.aliyuncs.com" + importUploadTestObjectPath,
		"ALİDOCS-NOTABLE.cn-zhangjiakou.oss.aliyuncs.com" + importUploadTestObjectPath,
		"alidocs-notable.cn-zhangjiakou.oss.aliyuncs.com.attacker.example" + importUploadTestObjectPath,
		"alidocs-notable.ap-southeast-1.oss.aliyuncs.com" + importUploadTestObjectPath,
		"cn-zhangjiakou.oss.aliyuncs.com/attacker" + importUploadTestObjectPath,
		"oss-cn-zhangjiakou.aliyuncs.com/attacker" + importUploadTestObjectPath,
		"ap-southeast-1.oss.aliyuncs.com/attacker" + importUploadTestObjectPath,
		"oss-ap-southeast-1.aliyuncs.com/attacker" + importUploadTestObjectPath,
		"cn-zhangjiakou.oss.aliyuncs.com/alidocs-notable" + importUploadTestObjectPath,
		"oss-cn-zhangjiakou.aliyuncs.com/alidocs-notable" + importUploadTestObjectPath,
		"ap-southeast-1.oss.aliyuncs.com/alidocs-notable-sg" + importUploadTestObjectPath,
		"cn-zhangjiakou.oss.aliyuncs.com/alidocs-notable/../attacker" + importUploadTestObjectPath,
		"cn-zhangjiakou.oss.aliyuncs.com/alidocs-notable%2f..%2fattacker" + importUploadTestObjectPath,
		"alidocs-notable.cn-zhangjiakou.oss.aliyuncs.com/attacker" + importUploadTestObjectPath,
		"alidocs-notable.cn-zhangjiakou.oss.aliyuncs.com/",
	} {
		t.Run(target, func(t *testing.T) {
			putCalls = 0
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: mustJSONText(t, map[string]any{
				"uploadUrl": "https://" + target + "?Signature=synthetic-secret", "importId": "imp",
			})}}}
			out, err := runAITableCompositeCLI(t, caller, "+import-file", "--base-id", "base", "--file", file, "--yes")
			if err == nil || out != "" || strings.Contains(err.Error(), "synthetic-secret") || strings.Contains(err.Error(), "private workbook bytes") {
				t.Fatalf("unsafe upload result: output=%q err=%v", out, err)
			}
			if putCalls != 0 || len(caller.calls) != 1 || caller.calls[0].tool != "prepare_import_upload" {
				t.Fatalf("unsafe upload reached PUT/import: PUT=%d MCP=%#v", putCalls, caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageImportUploadPreservesServiceSignedURL(t *testing.T) {
	file := filepath.Join(t.TempDir(), "data.xlsx")
	if err := os.WriteFile(file, []byte("workbook"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{
		"alidocs-notable.cn-zhangjiakou.oss.aliyuncs.com",
		"alidocs-notable-sg.ap-southeast-1.oss.aliyuncs.com",
		"alidocs-notable-test.cn-zhangjiakou.oss.aliyuncs.com",
	} {
		t.Run(host, func(t *testing.T) {
			// Synthetic credentials only. Escaped filename and signature must survive
			// validation byte-for-byte; Content-Length is signed by the service.
			raw := "https://" + host + "/notable/mcp_import_temp/0123456789abcdef0123456789abcdef/data%20%E8%A1%A8%2B%23%3F.xlsx?Expires=123&OSSAccessKeyId=synthetic&Signature=a%2Bb%2Fc%3D"
			putCalls := 0
			testseam.Swap(t, &importHTTPDo, func(req *http.Request) (*http.Response, error) {
				putCalls++
				data, err := io.ReadAll(req.Body)
				if err != nil || string(data) != "workbook" || req.Method != http.MethodPut || req.URL.String() != raw || req.ContentLength != 8 || req.Header.Get("Content-Type") != "" {
					t.Fatalf("signed upload request changed: req=%#v body=%q err=%v", req, data, err)
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(""))}, nil
			})
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
				{text: mustJSONText(t, map[string]any{"uploadUrl": raw, "importId": "imp"})},
				{text: `{"success":true}`},
			}}
			_, err := runAITableCompositeCLI(t, caller, "+import-file", "--base-id", "base", "--file", file, "--yes")
			if err != nil || putCalls != 1 || len(caller.calls) != 2 || caller.calls[1].tool != "import_data" {
				t.Fatalf("signed upload failed: err=%v PUT=%d MCP=%#v", err, putCalls, caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageImportUploadObjectPathValidation(t *testing.T) {
	for _, path := range []string{
		"", "/", "/upload", "/notable/mcp_import_temp/short/data.csv",
		"/notable/mcp_import_temp/0123456789abcdef0123456789abcdef/",
		"/notable/mcp_import_temp/0123456789abcdef0123456789abcdef/.",
		"/notable/mcp_import_temp/0123456789abcdef0123456789abcdef/..",
		"/notable/mcp_import_temp/0123456789abcdef0123456789abcdef/%2e%2e",
		"/notable/mcp_import_temp/0123456789abcdef0123456789abcdef/../data.csv",
		"/notable/mcp_import_temp/0123456789abcdef0123456789abcdef/data%2ffile.csv",
		importUploadTestObjectPath + "#fragment",
	} {
		if err := validateImportUploadURL("https://alidocs-notable.cn-zhangjiakou.oss.aliyuncs.com" + path); err == nil {
			t.Errorf("accepted invalid object path %q", path)
		}
	}
}

func TestCrossPlatformCoverageImportUploadHostAndDialPolicy(t *testing.T) {
	for _, host := range []string{
		"", "127.0.0.1", "[::ffff:127.0.0.1]", "attacker.oss-cn-zhangjiakou.aliyuncs.com",
		"cn-zhangjiakou.oss.aliyuncs.com", "oss-cn-zhangjiakou.aliyuncs.com",
		"ap-southeast-1.oss.aliyuncs.com", "oss-ap-southeast-1.aliyuncs.com",
		"alidocs-notable.cn-zhangjiakou.oss.aliyuncs.com.",
		"alidocs-notable-sg.cn-zhangjiakou.oss.aliyuncs.com",
		"alidocs-notable-test.ap-southeast-1.oss.aliyuncs.com",
		"ALİDOCS-NOTABLE.cn-zhangjiakou.oss.aliyuncs.com",
	} {
		t.Run(host, func(t *testing.T) {
			if isTrustedImportUploadHost(host) || validateImportUploadURL("https://"+host+importUploadTestObjectPath) == nil {
				t.Fatal("accepted untrusted host")
			}
			lookedUp := false
			testseam.Swap(t, &importUploadLookupIPAddr, func(context.Context, string) ([]net.IPAddr, error) {
				lookedUp = true
				return nil, errors.New("unexpected lookup")
			})
			if _, err := dialTrustedImportUpload(context.Background(), "tcp", net.JoinHostPort(host, "443")); err == nil || lookedUp {
				t.Fatalf("untrusted host reached resolver: err=%v lookedUp=%v", err, lookedUp)
			}
		})
	}
	if err := validateImportUploadURL("https://ALIDOCS-NOTABLE.CN-ZHANGJIAKOU.OSS.ALIYUNCS.COM:443" + importUploadTestObjectPath); err != nil {
		t.Fatalf("rejected case-insensitive DNS hostname: %v", err)
	}
}

func TestCrossPlatformCoverageImportUploadRejectsSpecialDNSAddresses(t *testing.T) {
	// Keep fixtures independent of the production prefix table. Both boundaries
	// of CGNAT/benchmark ranges and mapped IPv4 must be blocked before dialing.
	for _, raw := range []string{
		"invalid", "0.1.2.3", "10.0.0.1", "100.64.0.0", "100.127.255.255", "127.0.0.1",
		"169.254.169.254", "172.16.0.1", "192.0.0.1", "192.0.2.1", "192.168.0.1",
		"198.18.0.0", "198.19.255.255", "198.51.100.1", "203.0.113.1", "224.0.0.1", "240.0.0.1",
		"255.255.255.255", "::", "::1", "::192.0.2.1", "64:ff9b::a00:1", "64:ff9b:1::1",
		"100::1", "100:0:0:1::1", "2001::1", "2001:db8::1", "2002:7f00:1::", "3fff::1",
		"5f00::1", "fc00::1", "fe80::1", "fec0::1", "ff02::1", "4000::1",
	} {
		t.Run(raw, func(t *testing.T) {
			ip := net.ParseIP(raw)
			variants := []net.IP{ip}
			if v4 := ip.To4(); v4 != nil {
				variants = append(variants, v4, net.ParseIP("::ffff:"+raw))
			}
			for _, variant := range variants {
				if isPublicImportUploadIP(variant) {
					t.Errorf("accepted special IP %v", variant)
				}
				testseam.Swap(t, &importUploadLookupIPAddr, func(context.Context, string) ([]net.IPAddr, error) {
					// A public answer first must not hide a subsequent unsafe answer.
					return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}, {IP: variant}}, nil
				})
				dialed := false
				testseam.Swap(t, &importUploadDial, func(context.Context, string, string) (net.Conn, error) {
					dialed = true
					return nil, errors.New("unexpected dial")
				})
				_, err := dialTrustedImportUpload(context.Background(), "tcp", "alidocs-notable.cn-zhangjiakou.oss.aliyuncs.com:443")
				if err == nil || dialed {
					t.Errorf("unsafe DNS answer reached network: err=%v dialed=%v IP=%v", err, dialed, variant)
				}
			}
		})
	}
	for _, raw := range []string{"8.8.8.8", "1.1.1.1", "100.63.255.255", "100.128.0.0", "::ffff:8.8.8.8", "2001:4860:4860::8888"} {
		if !isPublicImportUploadIP(net.ParseIP(raw)) {
			t.Errorf("rejected ordinary public IP %s", raw)
		}
	}
}
