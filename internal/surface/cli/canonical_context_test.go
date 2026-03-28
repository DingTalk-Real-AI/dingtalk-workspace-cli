package cli

import (
	"context"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/ir"
	"github.com/spf13/cobra"
)

type contextCapturingLoader struct {
	ctx context.Context
}

func (l *contextCapturingLoader) Load(ctx context.Context) (ir.Catalog, error) {
	l.ctx = ctx
	return ir.Catalog{}, nil
}

type canonicalContextKey struct{}

func TestAddCanonicalProductsUsesRootContext(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	ctx := context.WithValue(context.Background(), canonicalContextKey{}, "ctx-value")
	root.SetContext(ctx)

	loader := &contextCapturingLoader{}
	if err := AddCanonicalProducts(root, loader, nil); err != nil {
		t.Fatalf("AddCanonicalProducts() error = %v", err)
	}
	if got := loader.ctx.Value(canonicalContextKey{}); got != "ctx-value" {
		t.Fatalf("loader context value = %#v, want ctx-value", got)
	}
}
