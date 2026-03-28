package app

import (
	"context"
	"testing"
)

type rootContextKey struct{}

func TestNewRootCommandUsesProvidedContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), rootContextKey{}, "ctx-value")

	root := NewRootCommand(ctx)

	if got := root.Context().Value(rootContextKey{}); got != "ctx-value" {
		t.Fatalf("root.Context() value = %#v, want ctx-value", got)
	}
}
