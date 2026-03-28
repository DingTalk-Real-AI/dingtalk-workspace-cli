package testutil

import (
	"net"
	"net/http"
	"net/http/httptest"
)

func NewIPv4Server(handler http.Handler) (*httptest.Server, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	server := httptest.NewUnstartedServer(handler)
	server.Listener = listener
	server.Start()
	return server, nil
}
