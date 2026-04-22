package auth

import (
	"net/http/httptest"
	"testing"
)

func TestCurrentChannelCode_ReturnsRawValue(t *testing.T) {
	t.Setenv(DWSChannelEnv, "Qoderwork;preview")

	if got := CurrentChannelCode(); got != "Qoderwork;preview" {
		t.Fatalf("CurrentChannelCode() = %q, want raw DWS_CHANNEL value", got)
	}
}

func TestApplyChannelHeader_ForwardsRawChannelCode(t *testing.T) {
	t.Setenv(DWSChannelEnv, "Qoderwork;preview")
	req := httptest.NewRequest("GET", "https://example.com", nil)

	ApplyChannelHeader(req)

	if got := req.Header.Get("x-dws-channel"); got != "Qoderwork;preview" {
		t.Fatalf("x-dws-channel = %q, want raw DWS_CHANNEL value", got)
	}
}