package audit

import "context"

type localReasonKey struct{}

// WithLocalReason attaches a CLI-only audit remark to this invocation. It must
// never be copied into the tool arguments or request headers.
func WithLocalReason(ctx context.Context, reason string) context.Context {
	return context.WithValue(ctx, localReasonKey{}, reason)
}

// LocalReason returns the CLI-only remark, if the caller supplied one.
func LocalReason(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	reason, _ := ctx.Value(localReasonKey{}).(string)
	return reason
}
