package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/runtimetoken"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/client"
	sdklogger "github.com/open-dingtalk/dingtalk-stream-sdk-go/logger"
)

type redactingSDKLogger struct{}

func (redactingSDKLogger) Debugf(string, ...interface{}) {}
func (redactingSDKLogger) Infof(format string, args ...interface{}) {
	fmt.Fprintln(os.Stderr, "[stream] "+redactLog(fmt.Sprintf(format, args...)))
}
func (redactingSDKLogger) Warningf(format string, args ...interface{}) {
	fmt.Fprintln(os.Stderr, "[stream][warn] "+redactLog(fmt.Sprintf(format, args...)))
}
func (redactingSDKLogger) Errorf(format string, args ...interface{}) {
	fmt.Fprintln(os.Stderr, "[stream][error] "+redactLog(fmt.Sprintf(format, args...)))
}
func (redactingSDKLogger) Fatalf(format string, args ...interface{}) {
	fmt.Fprintln(os.Stderr, "[stream][fatal] "+redactLog(fmt.Sprintf(format, args...)))
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	ticketURL := strings.TrimSpace(os.Getenv("DWS_STREAM_TICKET_URL"))
	if ticketURL == "" {
		ticketURL = strings.TrimRight(config.GetMCPBaseURL(), "/") + "/stream/connections/ticket"
	}
	channelType := strings.TrimSpace(os.Getenv("STREAM_CHANNEL_TYPE"))
	if channelType == "" {
		channelType = "pre_open_source"
	}

	token, err := runtimetoken.ResolveAccessToken(ctx, config.DefaultConfigDir(), "")
	if err != nil {
		exitf("resolve_token_error: %v", err)
	}
	if strings.TrimSpace(token) == "" {
		exitf("resolve_token_error: empty token")
	}

	httpClient := &http.Client{Timeout: 20 * time.Second}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.0/gateway/connections/open" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		var sdkReq struct {
			Subscriptions []struct {
				Type  string `json:"type"`
				Topic string `json:"topic"`
			} `json:"subscriptions"`
		}
		_ = json.Unmarshal(body, &sdkReq)

		ticket, err := requestPortalTicket(r.Context(), httpClient, ticketURL, token, channelType)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		fmt.Fprintf(os.Stderr, "[probe] injected portal ticket: endpoint=<redacted:%d chars> ticket=<redacted:%d chars> sdkSubscriptions=%d\n",
			len(ticket.Endpoint), len(ticket.Ticket), len(sdkReq.Subscriptions))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ticket)
	}))
	defer proxy.Close()

	sdklogger.SetLogger(redactingSDKLogger{})
	cli := client.NewStreamClient(
		client.WithAppCredential(client.NewAppCredentialConfig("dws-user-token-ticket-proxy", "unused-secret")),
		client.WithOpenApiHost(proxy.URL),
		client.WithAutoReconnect(false),
	)
	cli.RegisterChatBotCallbackRouter(func(_ context.Context, _ *chatbot.BotCallbackDataModel) ([]byte, error) {
		return []byte(""), nil
	})

	fmt.Printf("portal_ticket_url: %s\n", ticketURL)
	fmt.Printf("channel_type: %q\n", channelType)
	fmt.Printf("sdk_openapi_proxy: %s\n", proxy.URL)
	if err := cli.Start(ctx); err != nil {
		exitf("stream_start_error: %v", err)
	}
	fmt.Println("stream_start: ok")
	cli.Close()
}

type streamTicket struct {
	Endpoint string `json:"endpoint"`
	Ticket   string `json:"ticket"`
}

func requestPortalTicket(ctx context.Context, httpClient *http.Client, ticketURL, token, channelType string) (streamTicket, error) {
	body, err := json.Marshal(map[string]string{"channelType": channelType})
	if err != nil {
		return streamTicket{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ticketURL, bytes.NewReader(body))
	if err != nil {
		return streamTicket{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "dws-stream-ticket-injection-probe")
	req.Header.Set("x-user-access-token", token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return streamTicket{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return streamTicket{}, fmt.Errorf("portal ticket HTTP %d: %s", resp.StatusCode, truncateForLog(string(raw), 200))
	}

	var envelope struct {
		Success   bool         `json:"success"`
		Result    streamTicket `json:"result"`
		ErrorCode string       `json:"errorCode"`
		ErrorMsg  string       `json:"errorMsg"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return streamTicket{}, fmt.Errorf("portal ticket parse failed: %w", err)
	}
	if !envelope.Success {
		return streamTicket{}, fmt.Errorf("portal ticket failed: %s %s", envelope.ErrorCode, envelope.ErrorMsg)
	}
	if strings.TrimSpace(envelope.Result.Endpoint) == "" || strings.TrimSpace(envelope.Result.Ticket) == "" {
		return streamTicket{}, fmt.Errorf("portal ticket result missing endpoint/ticket")
	}
	return envelope.Result, nil
}

func redactLog(s string) string {
	if idx := strings.Index(s, "sessionId=["); idx >= 0 {
		end := strings.Index(s[idx:], "]")
		if end >= 0 {
			return s[:idx] + "sessionId=[<redacted>]" + s[idx+end+1:]
		}
	}
	return s
}

func truncateForLog(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func exitf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
