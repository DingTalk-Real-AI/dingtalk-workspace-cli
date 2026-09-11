package audit

import (
	"context"
	"testing"
)

func TestCrossPlatformCoverageLocalReasonIsOptionalAndContextScoped(t *testing.T) {
	for _, ctx := range []context.Context{nil, context.Background()} {
		if got := LocalReason(ctx); got != "" {
			t.Fatalf("absent audit context has reason %q", got)
		}
	}
	parent := context.Background()
	if got := LocalReason(WithLocalReason(parent, "清理重复图表")); got != "清理重复图表" {
		t.Fatalf("local reason = %q", got)
	}
	if got := LocalReason(parent); got != "" {
		t.Fatalf("child reason leaked to parent: %q", got)
	}
}
