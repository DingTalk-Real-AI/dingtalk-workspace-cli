package sender

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gitlab.alibaba-inc.com/aes/aem-go-sdk/internal/encoder"
)

const httpTimeout = 5 * time.Second

var httpClient = &http.Client{Timeout: httpTimeout}

// Send 单次 HTTP POST 上报 gokey 到 AES 后端，不重试。
//
// endpoint 不带 scheme 时默认 https://；以 http:// 或 https:// 开头时按原样使用。
// 路径固定为 /aes.1.1，请求体为 {"gokey": encodeURIComponent(gokey), "gmkey": "EXP"}。
func Send(endpoint string, gokey string, userAgent string) error {
	u := buildURL(endpoint)

	body, err := json.Marshal(map[string]string{
		"gokey": encoder.EncodeURIComponent(gokey),
		"gmkey": "EXP",
	})
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}

	req, err := http.NewRequest("POST", u, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

func buildURL(endpoint string) string {
	endpoint = strings.TrimRight(endpoint, "/")
	var u string
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		u = endpoint
	} else {
		u = fmt.Sprintf("https://%s", endpoint)
	}
	if strings.HasSuffix(u, "/aes.1.1") {
		return u
	}
	return u + "/aes.1.1"
}
