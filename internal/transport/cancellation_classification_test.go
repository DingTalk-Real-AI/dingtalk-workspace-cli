package transport

import (
	"context"
	"testing"
)

func TestCancellationClassificationKeepsDeadlineDistinct(t *testing.T) {
	if reason, _ := classifyRequestFailure(context.Canceled); reason != "request_cancelled" {
		t.Fatalf("cancellation reason=%q", reason)
	}
	if reason, _ := classifyRequestFailure(context.DeadlineExceeded); reason != "deadline_exceeded" {
		t.Fatalf("deadline reason=%q", reason)
	}
}
