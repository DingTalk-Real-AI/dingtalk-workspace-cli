package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/runtimetoken"
	"github.com/gorilla/websocket"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/event"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/payload"
)

func main() {
	waitSeconds := envInt("PROBE_WAIT_SECONDS", 300)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(waitSeconds+30)*time.Second)
	defer cancel()

	ticketURL := firstNonEmptyEnv("DWS_STREAM_TICKET_URL", "STREAM_TICKET_URL")
	if ticketURL == "" {
		ticketURL = strings.TrimRight(config.GetMCPBaseURL(), "/") + "/stream/connections/ticket"
	}
	sourceID := firstNonEmptyEnv("DWS_STREAM_SOURCE_ID", "STREAM_SOURCE_ID", "DWS_STREAM_CHANNEL_TYPE", "STREAM_CHANNEL_TYPE")
	if sourceID == "" {
		sourceID = "pre_open_source"
	}
	mode := firstNonEmptyEnv("DWS_STREAM_TICKET_MODE", "STREAM_TICKET_MODE")
	clientID := firstNonEmptyEnv("DWS_STREAM_CLIENT_ID", "STREAM_CLIENT_ID", "DWS_CLIENT_ID")
	clientSecret := firstNonEmptyEnv("DWS_STREAM_CLIENT_SECRET", "STREAM_CLIENT_SECRET", "DWS_CLIENT_SECRET")

	token, err := runtimetoken.ResolveAccessToken(ctx, config.DefaultConfigDir(), "")
	if err != nil {
		exitf("resolve_token_error: %v", err)
	}
	if strings.TrimSpace(token) == "" {
		exitf("resolve_token_error: empty token")
	}

	httpClient := &http.Client{Timeout: 20 * time.Second}
	ticket, err := requestPortalTicket(ctx, httpClient, ticketURL, token, sourceID, mode, clientID, clientSecret)
	if err != nil {
		exitf("portal_ticket_error: %v", err)
	}

	fmt.Printf("portal_ticket_url: %s\n", ticketURL)
	fmt.Printf("source_id: %q\n", sourceID)
	fmt.Printf("ticket_mode: %s\n", displayTicketMode(mode))
	if strings.EqualFold(strings.TrimSpace(mode), "custom") {
		fmt.Printf("custom_client_id: <redacted:%d chars>\n", len(clientID))
		fmt.Printf("custom_client_secret: <redacted:%d chars>\n", len(clientSecret))
	}
	fmt.Printf("endpoint_host: %s\n", endpointHost(ticket.Endpoint))
	fmt.Printf("endpoint: <redacted:%d chars>\n", len(ticket.Endpoint))
	fmt.Printf("ticket: <redacted:%d chars>\n", len(ticket.Ticket))
	fmt.Printf("probe_wait_seconds: %d\n", waitSeconds)
	if envBool("PROBE_FETCH_TICKET_ONLY") {
		return
	}

	wsURL, err := websocketURL(ticket)
	if err != nil {
		exitf("websocket_url_error: %v", err)
	}
	conn, resp, err := (&websocket.Dialer{HandshakeTimeout: 20 * time.Second}).DialContext(ctx, wsURL, http.Header{
		"User-Agent": []string{"dws-stream-ticket-http-probe"},
	})
	if err != nil {
		if resp != nil {
			defer resp.Body.Close()
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			exitf("stream_connect_error: status=%d body=%s err=%v", resp.StatusCode, truncateForLog(string(raw), 300), err)
		}
		exitf("stream_connect_error: %v", err)
	}
	defer conn.Close()
	fmt.Println("stream_connect: ok")

	received := make(chan string, 1)
	readErr := make(chan error, 1)
	go readFrames(ctx, conn, received, readErr)

	if waitSeconds <= 0 {
		return
	}
	select {
	case kind := <-received:
		fmt.Printf("stream_receive: ok kind=%s\n", kind)
	case err := <-readErr:
		exitf("stream_read_error: %v", err)
	case <-time.After(time.Duration(waitSeconds) * time.Second):
		fmt.Println("stream_receive: timeout")
	case <-ctx.Done():
		fmt.Printf("stream_receive: context_done err=%v\n", ctx.Err())
	}
}

type streamTicket struct {
	Endpoint string `json:"endpoint"`
	Ticket   string `json:"ticket"`
}

func requestPortalTicket(ctx context.Context, httpClient *http.Client, ticketURL, token, sourceID, mode, clientID, clientSecret string) (streamTicket, error) {
	requestBody := map[string]string{
		"sourceId":    sourceID,
		"channelType": sourceID,
	}
	if strings.TrimSpace(mode) != "" {
		requestBody["mode"] = strings.TrimSpace(mode)
	}
	if strings.TrimSpace(clientID) != "" {
		requestBody["clientId"] = strings.TrimSpace(clientID)
	}
	if strings.TrimSpace(clientSecret) != "" {
		requestBody["clientSecret"] = strings.TrimSpace(clientSecret)
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		return streamTicket{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ticketURL, bytes.NewReader(body))
	if err != nil {
		return streamTicket{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "dws-stream-ticket-http-probe")
	req.Header.Set("x-user-access-token", token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return streamTicket{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return streamTicket{}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateForLog(string(raw), 300))
	}

	var direct streamTicket
	if err := json.Unmarshal(raw, &direct); err == nil && direct.Endpoint != "" && direct.Ticket != "" {
		return direct, nil
	}

	var envelope struct {
		Success   bool         `json:"success"`
		Result    streamTicket `json:"result"`
		ErrorCode string       `json:"errorCode"`
		ErrorMsg  string       `json:"errorMsg"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return streamTicket{}, fmt.Errorf("parse failed: %w", err)
	}
	if !envelope.Success {
		return streamTicket{}, fmt.Errorf("business failed: %s %s", envelope.ErrorCode, envelope.ErrorMsg)
	}
	if envelope.Result.Endpoint == "" || envelope.Result.Ticket == "" {
		return streamTicket{}, fmt.Errorf("missing endpoint/ticket")
	}
	return envelope.Result, nil
}

func websocketURL(ticket streamTicket) (string, error) {
	u, err := url.Parse(ticket.Endpoint)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("ticket", ticket.Ticket)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func endpointHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<parse-error>"
	}
	if u.Scheme == "" || u.Host == "" {
		return "<missing>"
	}
	return u.Scheme + "://" + u.Host
}

func displayTicketMode(mode string) string {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return "normal(default)"
	}
	return mode
}

func readFrames(ctx context.Context, conn *websocket.Conn, received chan<- string, readErr chan<- error) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			readErr <- err
			return
		}
		fmt.Printf("stream_frame_received: messageType=%d bytes=%d raw=%q\n",
			messageType,
			len(message),
			truncateForLog(string(message), 300),
		)
		if messageType == websocket.TextMessage {
			if acked := handleDataFrame(conn, message); acked {
				notifyReceived(received, "data_frame")
			}
		}
	}
}

func handleDataFrame(conn *websocket.Conn, message []byte) bool {
	df, err := payload.DecodeDataFrame(message)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stream_frame_decode_error: %v\n", err)
		return false
	}
	fmt.Printf("event_frame_received: type=%s topic=%s messageId=%s data=%q\n",
		df.Type,
		df.GetTopic(),
		df.GetMessageId(),
		truncateForLog(df.Data, 300),
	)

	resp := payload.NewSuccessDataFrameResponse()
	if err := resp.SetJson(event.NewEventProcessResultSuccess()); err != nil {
		fmt.Fprintf(os.Stderr, "stream_ack_build_error: %v\n", err)
		return true
	}
	if resp.GetHeader(payload.DataFrameHeaderKMessageId) == "" {
		resp.SetHeader(payload.DataFrameHeaderKMessageId, df.GetMessageId())
	}
	if resp.GetHeader(payload.DataFrameHeaderKContentType) == "" {
		resp.SetHeader(payload.DataFrameHeaderKContentType, payload.DataFrameContentTypeKJson)
	}
	if err := conn.WriteMessage(websocket.TextMessage, resp.Encode()); err != nil {
		fmt.Fprintf(os.Stderr, "stream_ack_send_error: %v\n", err)
		return true
	}
	fmt.Printf("stream_ack_sent: messageId=%s code=%d\n", df.GetMessageId(), resp.Code)
	return true
}

func truncateForLog(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func envInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return fallback
	}
	return n
}

func envBool(name string) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return value == "1" || value == "true" || value == "yes"
}

func firstNonEmptyEnv(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func notifyReceived(ch chan<- string, kind string) {
	select {
	case ch <- kind:
	default:
	}
}

func exitf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
