// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

// Package netpolicy separates outbound routing from each caller's target,
// credential, redirect and retry policies. Policies are invocation snapshots;
// no process-global transport or environment is mutated.
package netpolicy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Settings is the versioned, non-secret persisted network configuration.
// Pointers distinguish an absent setting from an explicit empty bypass list.
type Settings struct {
	Proxy    *string `json:"proxy,omitempty"`
	NoProxy  *string `json:"no_proxy,omitempty"`
	CABundle *string `json:"ca_bundle,omitempty"`
}

type Overrides struct{ Proxy, NoProxy *string }
type Environment map[string]string

var systemCertPool = x509.SystemCertPool

func SnapshotEnvironment() Environment {
	e := Environment{}
	for _, k := range []string{"DWS_PROXY", "DWS_NO_PROXY", "DWS_CA_BUNDLE", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy", "ALL_PROXY", "all_proxy", "REQUEST_METHOD"} {
		if v, ok := os.LookupEnv(k); ok {
			e[k] = v
		}
	}
	return e
}

type bypassRule struct {
	host, port string
	network    netip.Prefix
	ip         netip.Addr
	subdomains bool
}

// Policy is immutable after Resolve; only its owned connection-pool cache changes.
type Policy struct {
	mode, source, noProxy, noProxySource, caBundle string
	httpProxy, httpsProxy                          *url.URL
	rules                                          []bypassRule
	all, cgi, unsupportedAllProxy                  bool
	roots                                          *x509.CertPool
	mu                                             sync.Mutex
	transports                                     map[*http.Transport]*http.Transport
}

func value(e Environment, upper, lower string) string {
	if e[upper] != "" {
		return e[upper]
	}
	return e[lower]
}

// Resolve validates the selected configuration before any request can be sent.
func Resolve(s Settings, o Overrides, e Environment) (*Policy, error) {
	p := &Policy{mode: "auto", source: "default", noProxySource: "default", cgi: e["REQUEST_METHOD"] != "", transports: map[*http.Transport]*http.Transport{}}
	if s.Proxy != nil {
		p.mode, p.source = *s.Proxy, "profile"
	}
	if v, ok := e["DWS_PROXY"]; ok {
		p.mode, p.source = v, "DWS_PROXY"
	}
	if o.Proxy != nil {
		p.mode, p.source = *o.Proxy, "flag"
	}
	p.mode = strings.TrimSpace(p.mode)
	if p.mode == "" {
		return nil, fmt.Errorf("network: proxy must be auto, direct, or a supported proxy URL")
	}
	if s.NoProxy != nil {
		p.noProxy, p.noProxySource = *s.NoProxy, "profile"
	}
	if v := value(e, "NO_PROXY", "no_proxy"); v != "" {
		p.noProxy, p.noProxySource = v, "NO_PROXY"
	}
	if v, ok := e["DWS_NO_PROXY"]; ok {
		p.noProxy, p.noProxySource = v, "DWS_NO_PROXY"
	}
	if o.NoProxy != nil {
		p.noProxy, p.noProxySource = *o.NoProxy, "flag"
	}
	if p.mode != "direct" {
		var err error
		if p.rules, p.all, err = parseBypass(p.noProxy); err != nil {
			return nil, err
		}
		if p.mode == "auto" {
			p.httpProxy, err = parseProxy(value(e, "HTTP_PROXY", "http_proxy"), true)
			if err != nil {
				return nil, fmt.Errorf("network: invalid HTTP_PROXY (value omitted)")
			}
			p.httpsProxy, err = parseProxy(value(e, "HTTPS_PROXY", "https_proxy"), true)
			if err != nil {
				return nil, fmt.Errorf("network: invalid HTTPS_PROXY (value omitted)")
			}
			p.unsupportedAllProxy = value(e, "ALL_PROXY", "all_proxy") != ""
		} else {
			u, err := parseProxy(p.mode, false)
			if err != nil {
				return nil, err
			}
			p.httpProxy, p.httpsProxy, p.mode = u, u, "proxy"
		}
	}
	if s.CABundle != nil {
		p.caBundle = *s.CABundle
	}
	if v, ok := e["DWS_CA_BUNDLE"]; ok {
		p.caBundle = v
	}
	if p.caBundle != "" {
		if !filepath.IsAbs(p.caBundle) {
			return nil, fmt.Errorf("network: CA bundle path must be absolute")
		}
		pem, err := os.ReadFile(p.caBundle)
		if err != nil {
			return nil, fmt.Errorf("network: cannot read configured CA bundle")
		}
		roots, err := systemCertPool()
		if err != nil {
			return nil, fmt.Errorf("network: cannot load system CA roots")
		}
		if !roots.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("network: CA bundle contains no certificates")
		}
		p.roots = roots
	}
	return p, nil
}

func parseProxy(raw string, legacy bool) (*url.URL, error) {
	if raw == "" && legacy {
		return nil, nil
	}
	if legacy && !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.Opaque != "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("network: invalid proxy URL (value omitted)")
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, fmt.Errorf("network: unsupported proxy scheme; use http, https, socks5 or socks5h")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("network: invalid proxy port (value omitted)")
		}
	}
	return u, nil
}

func parseBypass(raw string) ([]bypassRule, bool, error) {
	var rules []bypassRule
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.ToLower(strings.TrimSpace(entry))
		if entry == "" {
			continue
		}
		if entry == "*" {
			return nil, true, nil
		}
		r := bypassRule{}
		if prefix, err := netip.ParsePrefix(entry); err == nil {
			r.network = prefix
			rules = append(rules, r)
			continue
		}
		if ip, err := netip.ParseAddr(entry); err == nil {
			r.ip = ip.Unmap()
			rules = append(rules, r)
			continue
		}
		host, port, err := net.SplitHostPort(entry)
		if err == nil {
			portNumber, portErr := strconv.Atoi(port)
			if portErr != nil || portNumber < 1 || portNumber > 65535 {
				return nil, false, fmt.Errorf("network: invalid NO_PROXY port (value omitted)")
			}
			r.port = port
			entry = host
		}
		if ip, err := netip.ParseAddr(entry); err == nil {
			r.ip = ip.Unmap()
		} else {
			r.host = strings.TrimPrefix(strings.TrimPrefix(entry, "*."), ".")
			r.subdomains = strings.HasPrefix(entry, ".") || strings.HasPrefix(entry, "*.")
			if r.host == "" || strings.ContainsAny(r.host, "/:*[] \t\r\n") {
				return nil, false, fmt.Errorf("network: invalid NO_PROXY entry (value omitted)")
			}
		}
		rules = append(rules, r)
	}
	return rules, false, nil
}

func (p *Policy) route(u *url.URL) (*url.URL, string, error) {
	if u == nil {
		return nil, "", fmt.Errorf("network: missing target URL")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	ip, _ := netip.ParseAddr(host)
	ip = ip.Unmap()
	if p.mode == "direct" {
		return nil, "direct mode", nil
	}
	if host == "localhost" || ip.IsLoopback() {
		return nil, "loopback target", nil
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" || u.Scheme == "wss" {
			port = "443"
		} else {
			port = "80"
		}
	}
	if p.all {
		return nil, "NO_PROXY *", nil
	}
	for _, r := range p.rules {
		if r.port != "" && r.port != port {
			continue
		}
		if (r.ip.IsValid() && r.ip == ip) || (r.network.IsValid() && r.network.Contains(ip)) || (r.host != "" && ((!r.subdomains && host == r.host) || strings.HasSuffix(host, "."+r.host))) {
			return nil, "NO_PROXY match", nil
		}
	}
	var proxy *url.URL
	switch u.Scheme {
	case "http", "ws":
		proxy = p.httpProxy
		if proxy != nil && p.cgi && p.mode == "auto" {
			return nil, "", fmt.Errorf("network: refusing HTTP_PROXY in CGI environment")
		}
	case "https", "wss":
		proxy = p.httpsProxy
	default:
		return nil, "", fmt.Errorf("network: unsupported target scheme")
	}
	if proxy == nil {
		return nil, "no proxy configured for target scheme", nil
	}
	copy := *proxy
	return &copy, "configured proxy", nil
}

func (p *Policy) Proxy(req *http.Request) (*url.URL, error) {
	u, _, err := p.route(req.URL)
	return u, err
}

// Explanation contains no URL paths, queries, userinfo or credentials.
type Explanation struct {
	Mode            string `json:"mode"`
	Source          string `json:"source"`
	NoProxySource   string `json:"no_proxy_source"`
	Target          string `json:"target,omitempty"`
	Route           string `json:"route,omitempty"`
	Reason          string `json:"reason,omitempty"`
	Proxy           string `json:"proxy,omitempty"`
	HTTPProxy       string `json:"http_proxy,omitempty"`
	HTTPSProxy      string `json:"https_proxy,omitempty"`
	CustomCA        bool   `json:"custom_ca"`
	ALLProxyIgnored bool   `json:"all_proxy_ignored"`
}

func redactedURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// Summary reports the effective settings without evaluating a target.
func (p *Policy) Summary() Explanation {
	return Explanation{Mode: p.mode, Source: p.source, NoProxySource: p.noProxySource, HTTPProxy: redactedURL(p.httpProxy), HTTPSProxy: redactedURL(p.httpsProxy), CustomCA: p.roots != nil, ALLProxyIgnored: p.unsupportedAllProxy}
}

func (p *Policy) Explain(u *url.URL) (Explanation, error) {
	e := p.Summary()
	if u != nil {
		proxy, reason, err := p.route(u)
		if err != nil {
			return e, err
		}
		e.Target = u.Scheme + "://" + u.Host
		e.Route = "direct"
		e.Reason = reason
		if proxy != nil {
			e.Route = "proxy"
			e.Proxy = redactedURL(proxy)
		}
	}
	return e, nil
}

type contextKey struct{}

func WithPolicy(ctx context.Context, p *Policy) context.Context {
	return context.WithValue(ctx, contextKey{}, p)
}
func FromContext(ctx context.Context) *Policy { p, _ := ctx.Value(contextKey{}).(*Policy); return p }

// Transport clones tuned caller settings and overrides only network routing and
// optional CA roots. Caller-owned and global transports are never modified.
func (p *Policy) Transport(base *http.Transport) *http.Transport {
	p.mu.Lock()
	defer p.mu.Unlock()
	if t := p.transports[base]; t != nil {
		return t
	}
	t := base.Clone()
	t.Proxy = p.Proxy
	if p.roots != nil {
		if t.TLSClientConfig == nil {
			t.TLSClientConfig = &tls.Config{}
		}
		t.TLSClientConfig.RootCAs = p.roots.Clone()
	}
	p.transports[base] = t
	return t
}

func (p *Policy) CloseIdleConnections() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, t := range p.transports {
		t.CloseIdleConnections()
	}
}

// Current returns the invocation policy, or a fresh environment policy for
// standalone callers.
func Current(ctx context.Context) (*Policy, error) {
	if p := FromContext(ctx); p != nil {
		return p, nil
	}
	return Resolve(Settings{}, Overrides{}, SnapshotEnvironment())
}

// bind keeps the caller's settings and custom round trippers.
func bind(p *Policy, client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{}
	}
	copy := *client
	if copy.Transport == nil {
		copy.Transport = http.DefaultTransport
	}
	if t, ok := copy.Transport.(*http.Transport); ok {
		copy.Transport = p.Transport(t)
	}
	return &copy
}

// Do preserves caller injection (including mocks), timeouts and redirects.
func Do(client *http.Client, req *http.Request) (*http.Response, error) {
	p, err := Current(req.Context())
	if err != nil {
		return nil, err
	}
	resp, err := bind(p, client).Do(req)
	// net/http includes the full request URL in url.Error. Signed resource
	// URLs and OAPI access_token queries must not escape through diagnostics.
	var requestError *url.Error
	if errors.As(err, &requestError) {
		copy := *requestError
		if target, parseErr := url.Parse(copy.URL); parseErr == nil {
			copy.URL = target.Scheme + "://" + target.Host
		} else {
			copy.URL = "<redacted target>"
		}
		err = &copy
	}
	return resp, err
}

// EnvironmentProxy is for the standalone download constructor; CLI requests
// additionally use their invocation snapshot via Do.
func EnvironmentProxy(req *http.Request) (*url.URL, error) {
	p, err := Resolve(Settings{}, Overrides{}, SnapshotEnvironment())
	if err != nil {
		return nil, err
	}
	return p.Proxy(req)
}
