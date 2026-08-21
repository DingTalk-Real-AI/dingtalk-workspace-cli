package safechat

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// httpLogBodyLimit is the maximum body length (bytes) included in debug logs.
// Beyond this, the body is truncated with a "..." marker so the log line stays
// readable. Full bodies are still passed to the C library unchanged.
const httpLogBodyLimit = 4096

// keyClient handles HTTP communication with the key server.
// It is responsible for sending key requests (triggered by C library's goProxy callback)
// and returning the JSON response to be fed back via setResponse.
//
// The code field holds a DingTalk authCode  used for server authentication.
// It is required when:
//   - Fetching keys for the first time (empty keystore)
//   - Server-side key version rotation (C library detects version mismatch)
type keyClient struct {
	httpClient *http.Client
	code       string // DingTalk authCode
	keyServer  string // Optional override for key server URL
	logger     Logger
}

// newKeyClient creates a new key client with the given configuration.
func newKeyClient(cfg Config) *keyClient {
	// Read InsecureSkipVerify from environment variable.
	// Default is false (secure). Set SAFECHAT_INSECURE_SKIP_VERIFY=true to disable verification
	// for testing with self-signed certificates.
	insecureSkipVerify := false
	if envVal := os.Getenv("SAFECHAT_INSECURE_SKIP_VERIFY"); envVal != "" {
		if parsed, err := strconv.ParseBool(envVal); err == nil {
			insecureSkipVerify = parsed
		}
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: insecureSkipVerify,
		},
		MaxIdleConns:       10,
		IdleConnTimeout:    30 * time.Second,
		DisableCompression: true,
	}

	return &keyClient{
		httpClient: &http.Client{
			Timeout:   cfg.HTTPTimeout,
			Transport: transport,
		},
		code:      cfg.Code,
		keyServer: cfg.KeyServer,
		logger:    cfg.Logger,
	}
}

func (kc *keyClient) doKeyRequest(fullURL, param, code string) (string, error) {
	// Use override key server if configured
	targetURL := fullURL
	if kc.keyServer != "" {
		targetURL = kc.keyServer
	}

	var body string
	if strings.HasPrefix(param, "param=") {
		// V1 format: C library output already has "param=" prefix and all fields.
		body = fmt.Sprintf("%s&code=%s", param, url.QueryEscape(code))
	} else {
		body = fmt.Sprintf("%s&code=%s&appAlgVersion=1", param, url.QueryEscape(code))
	}

	// === Request logging (debug) ===
	// Log full request line, headers and body (truncated) so we can verify
	// the URL, the URL-encoded payload and the auth code are shaped as expected.
	if kc.logger != nil {
		kc.logger.Debug("=== HTTP key request ===")
		kc.logger.Debug("URL:    POST %s", targetURL)
		kc.logger.Debug("C-URL:  %s (from C library)", fullURL)
		kc.logger.Debug("Server: %s%s",
			func() string {
				if kc.keyServer != "" {
					return kc.keyServer + " (override)"
				}
				return "(C-provided)"
			}(),
			"")
		kc.logger.Debug("Headers: Content-Type=application/x-www-form-urlencoded, User-Agent=SafeChat-Go-SDK/1.0")
		kc.logger.Debug("Code (auth_token, length=%d): %s", len(code), previewString(code, 64))
		kc.logger.Debug("Body (length=%d): %s", len(body), previewString(body, httpLogBodyLimit))
	}

	req, err := http.NewRequest("POST", targetURL, strings.NewReader(body))
	if err != nil {
		if kc.logger != nil {
			kc.logger.Error("create request failed: %v", err)
		}
		return "", fmt.Errorf("create request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "SafeChat-Go-SDK/1.0")

	// Capture timing so we can see if the server is slow / timing out.
	start := time.Now()
	resp, err := kc.httpClient.Do(req)
	if err != nil {
		if kc.logger != nil {
			kc.logger.Error("HTTP request failed after %s: %v", time.Since(start), err)
		}
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		if kc.logger != nil {
			kc.logger.Error("read response body failed after %s: %v", time.Since(start), err)
		}
		return "", fmt.Errorf("read response body failed: %w", err)
	}
	duration := time.Since(start)

	// === Response logging (debug on success, info on non-2xx) ===
	if kc.logger != nil {
		kc.logger.Debug("=== HTTP key response ===")
		kc.logger.Debug("Status: %d %s (took %s)", resp.StatusCode, http.StatusText(resp.StatusCode), duration)
		kc.logger.Debug("Headers: Content-Type=%s, Content-Length=%d", resp.Header.Get("Content-Type"), len(respBody))
		kc.logger.Debug("Body (length=%d): %s", len(respBody), previewString(string(respBody), httpLogBodyLimit))
	}

	if resp.StatusCode != http.StatusOK {
		if kc.logger != nil {
			kc.logger.Error("key server returned HTTP %d %s (took %s, body length=%d): %s",
				resp.StatusCode, http.StatusText(resp.StatusCode), duration, len(respBody),
				previewString(string(respBody), httpLogBodyLimit))
		}
		return "", fmt.Errorf("key server returned HTTP %d", resp.StatusCode)
	}

	return string(respBody), nil
}

// previewString returns s unchanged when it fits in max bytes; otherwise it
// returns the first max bytes followed by "...[truncated, total=N]". This is
// used to keep HTTP request/response log lines readable for very large bodies
// while still preserving the head of the payload for debugging.
func previewString(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("...[truncated, total=%d]", len(s))
}

// updateCode updates the authentication code (may change during runtime).
func (kc *keyClient) updateCode(code string) {
	kc.code = code
}
