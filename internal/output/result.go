// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package output

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/spf13/cobra"
)

// CommandResult is the immutable framework result consumed by CLI and MCP
// adapters. The unexported envelope method prevents product packages from
// implementing ad-hoc result shapes.
type CommandResult interface {
	Outcome() Outcome
	ExitCode() int
	envelope() *Envelope
}

type commandResult struct {
	env              Envelope
	exitCode         int
	exitCodeOverride bool
}

func (r *commandResult) Outcome() Outcome { return r.env.Outcome }
func (r *commandResult) ExitCode() int    { return r.exitCode }
func (r *commandResult) envelope() *Envelope {
	copy := cloneEnvelope(r.env)
	return &copy
}

// ResultOption enriches a result without exposing mutable framework fields.
type ResultOption struct{ apply func(*Envelope) }

func WithIdentity(identity string) ResultOption {
	return ResultOption{apply: func(env *Envelope) { env.Identity = identity }}
}

func WithDryRun() ResultOption {
	return ResultOption{apply: func(env *Envelope) { env.DryRun = true }}
}

func WithMeta(meta *Meta) ResultOption {
	return ResultOption{apply: func(env *Envelope) { env.Meta = cloneMeta(meta) }}
}

// Success constructs an immutable success result.
func Success(data any, opts ...ResultOption) CommandResult {
	return newCommandResult(OutcomeSuccess, data, nil, opts...)
}

// Pending constructs an immutable accepted-but-not-terminal result.
func Pending(data any, operation *OperationInfo, opts ...ResultOption) CommandResult {
	opts = append(opts, ResultOption{apply: func(env *Envelope) {
		if env.Meta == nil {
			env.Meta = &Meta{}
		}
		env.Meta.Operation = operation
	}})
	return newCommandResult(OutcomePending, data, nil, opts...)
}

// Partial constructs an immutable multi-status result with exit code 7.
func Partial(data *PartialData, opts ...ResultOption) CommandResult {
	return newCommandResult(OutcomePartialFailure, data, nil, opts...)
}

// Failure constructs an immutable typed failure result.
func Failure(info *ErrorInfo, opts ...ResultOption) CommandResult {
	if info != nil {
		info = cloneErrorInfo(info)
		// Product code does not own process status. The framework derives it
		// from type/subtype so wire and process status cannot drift.
		info.ExitCode = 0
	}
	return newCommandResult(OutcomeFailure, nil, info, opts...)
}

// FailureWithExitCode adapts an existing repository error whose historical
// process status is part of a compatibility contract (for example SIGINT=130).
// Product handlers must use Failure; only the root legacy-error adapter should
// call this function.
func FailureWithExitCode(info *ErrorInfo, exitCode int, opts ...ResultOption) CommandResult {
	if info != nil {
		info = cloneErrorInfo(info)
		info.ExitCode = 0
	}
	return newCommandResultWithExitCode(OutcomeFailure, nil, info, exitCode, true, opts...)
}

func newCommandResult(outcome Outcome, data any, info *ErrorInfo, opts ...ResultOption) CommandResult {
	return newCommandResultWithExitCode(outcome, data, info, 0, false, opts...)
}

func newCommandResultWithExitCode(outcome Outcome, data any, info *ErrorInfo, override int, hasOverride bool, opts ...ResultOption) CommandResult {
	env := Envelope{Outcome: outcome, Data: data, Error: info}
	env.OK = outcome == OutcomeSuccess || outcome == OutcomePending
	for _, opt := range opts {
		if opt.apply != nil {
			opt.apply(&env)
		}
	}
	exitCode := ExitCodeForEnvelope(&env)
	if hasOverride && override > 0 {
		exitCode = override
	}
	if env.Error != nil {
		env.Error.ExitCode = exitCode
	}
	env = cloneEnvelope(env)
	return &commandResult{env: env, exitCode: exitCode, exitCodeOverride: hasOverride}
}

func cloneEnvelope(source Envelope) Envelope {
	out := source
	out.Meta = cloneMeta(source.Meta)
	out.Error = cloneErrorInfo(source.Error)
	out.Data = cloneResultData(source.Data)
	out.Notice = cloneResultData(source.Notice)
	if partial, ok := source.Data.(*PartialData); ok && partial != nil {
		copyPartial := &PartialData{
			Total:     partial.Total,
			Succeeded: make([]any, len(partial.Succeeded)),
			Failed:    make([]PartialFailedEntry, len(partial.Failed)),
			Unknown:   make([]PartialUnknownEntry, len(partial.Unknown)),
		}
		copy(copyPartial.Failed, partial.Failed)
		copy(copyPartial.Unknown, partial.Unknown)
		for i := range partial.Succeeded {
			copyPartial.Succeeded[i] = cloneResultData(partial.Succeeded[i])
		}
		for i := range copyPartial.Failed {
			copyPartial.Failed[i].Error = cloneErrorInfo(copyPartial.Failed[i].Error)
		}
		out.Data = copyPartial
	}
	return out
}

func cloneMeta(source *Meta) *Meta {
	if source == nil {
		return nil
	}
	meta := *source
	if source.Count != nil {
		count := *source.Count
		meta.Count = &count
	}
	if source.Operation != nil {
		op := *source.Operation
		meta.Operation = &op
	}
	if source.Pagination != nil {
		pagination := *source.Pagination
		meta.Pagination = &pagination
	}
	return &meta
}

func cloneResultData(value any) any {
	if value == nil {
		return nil
	}
	return cloneReflectValue(reflect.ValueOf(value), make(map[cloneVisit]reflect.Value)).Interface()
}

type cloneVisit struct {
	typ reflect.Type
	ptr uintptr
	len int
	cap int
}

func cloneReflectValue(value reflect.Value, seen map[cloneVisit]reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := cloneReflectValue(value.Elem(), seen)
		out := reflect.New(value.Type()).Elem()
		out.Set(cloned)
		return out
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		visit := cloneVisit{typ: value.Type(), ptr: value.Pointer()}
		if cloned, ok := seen[visit]; ok {
			return cloned
		}
		out := reflect.New(value.Type().Elem())
		seen[visit] = out
		out.Elem().Set(cloneReflectValue(value.Elem(), seen))
		return out
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		visit := cloneVisit{typ: value.Type(), ptr: value.Pointer()}
		if cloned, ok := seen[visit]; ok {
			return cloned
		}
		out := reflect.MakeMapWithSize(value.Type(), value.Len())
		seen[visit] = out
		iter := value.MapRange()
		for iter.Next() {
			out.SetMapIndex(cloneReflectValue(iter.Key(), seen), cloneReflectValue(iter.Value(), seen))
		}
		return out
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		visit := cloneVisit{typ: value.Type(), ptr: value.Pointer(), len: value.Len(), cap: value.Cap()}
		if cloned, ok := seen[visit]; ok {
			return cloned
		}
		out := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		seen[visit] = out
		for i := 0; i < value.Len(); i++ {
			out.Index(i).Set(cloneReflectValue(value.Index(i), seen))
		}
		return out
	case reflect.Array:
		out := reflect.New(value.Type()).Elem()
		for i := 0; i < value.Len(); i++ {
			out.Index(i).Set(cloneReflectValue(value.Index(i), seen))
		}
		return out
	case reflect.Struct:
		out := reflect.New(value.Type()).Elem()
		out.Set(value)
		for i := 0; i < value.NumField(); i++ {
			if value.Type().Field(i).PkgPath == "" && out.Field(i).CanSet() {
				out.Field(i).Set(cloneReflectValue(value.Field(i), seen))
			}
		}
		return out
	default:
		return value
	}
}

func cloneErrorInfo(source *ErrorInfo) *ErrorInfo {
	if source == nil {
		return nil
	}
	out := *source
	out.UpstreamCode = cloneResultData(source.UpstreamCode)
	out.Params = append([]string(nil), source.Params...)
	out.Actions = append([]string(nil), source.Actions...)
	out.AvailableFlags = append([]string(nil), source.AvailableFlags...)
	if source.ExecutionStarted != nil {
		started := *source.ExecutionStarted
		out.ExecutionStarted = &started
	}
	if source.Details != nil {
		out.Details = cloneResultData(source.Details).(map[string]any)
	}
	out.RPCData = cloneResultData(source.RPCData)
	if source.RetryAfterSeconds != nil {
		seconds := *source.RetryAfterSeconds
		out.RetryAfterSeconds = &seconds
	}
	return &out
}

// ValidateResult is the mandatory pre-emission policy boundary.
func ValidateResult(result CommandResult) error {
	if result == nil {
		return fmt.Errorf("output: nil command result")
	}
	env := result.envelope()
	if err := env.Validate(); err != nil {
		return err
	}
	if env.Outcome == OutcomePending {
		if env.Meta == nil || env.Meta.Operation == nil {
			return fmt.Errorf("output: pending result requires meta.operation")
		}
		op := env.Meta.Operation
		if strings.TrimSpace(op.ID) == "" {
			return fmt.Errorf("output: pending result requires operation.id")
		}
		if strings.TrimSpace(op.State) == "" {
			return fmt.Errorf("output: pending result requires operation.state")
		}
		if strings.TrimSpace(op.NextCommand) == "" {
			return fmt.Errorf("output: pending result requires operation.next_command")
		}
	}
	if env.Outcome == OutcomePartialFailure {
		if partial, ok := env.Data.(*PartialData); !ok || partial == nil {
			return fmt.Errorf("output: partial_failure result requires typed *PartialData")
		}
	}
	concrete, _ := result.(*commandResult)
	want := ExitCodeForEnvelope(env)
	if concrete == nil || !concrete.exitCodeOverride {
		if got := result.ExitCode(); got != want {
			return fmt.Errorf("output: result exit code=%d disagrees with outcome-derived code=%d", got, want)
		}
	} else if result.ExitCode() <= 0 {
		return fmt.Errorf("output: explicit compatibility exit code must be positive")
	}
	if env.Error != nil && env.Error.ExitCode != result.ExitCode() {
		return fmt.Errorf("output: error.exit_code=%d disagrees with process exit code=%d", env.Error.ExitCode, result.ExitCode())
	}
	return nil
}

// ResultStore transfers a successful in-memory result through Cobra without
// pretending it is an error. The root PersistentPostRunE emits the result
// before closing output sinks, so normal cleanup always runs.
type ResultStore struct {
	mu            sync.Mutex
	result        CommandResult
	emitAttempted bool
	bytesRisk     bool
	emitted       bool
	exitCode      int
}

type resultStoreContextKey struct{}

func WithResultStore(ctx context.Context) (context.Context, *ResultStore) {
	if ctx == nil {
		ctx = context.Background()
	}
	if store, ok := resultStoreFromContext(ctx); ok {
		return ctx, store
	}
	store := &ResultStore{}
	return context.WithValue(ctx, resultStoreContextKey{}, store), store
}

func resultStoreFromContext(ctx context.Context) (*ResultStore, bool) {
	if ctx == nil {
		return nil, false
	}
	store, ok := ctx.Value(resultStoreContextKey{}).(*ResultStore)
	return store, ok && store != nil
}

// StoreResult records exactly one terminal framework result for this command.
func StoreResult(ctx context.Context, result CommandResult) error {
	if err := ValidateResult(result); err != nil {
		return err
	}
	store, ok := resultStoreFromContext(ctx)
	if !ok {
		return fmt.Errorf("output: command context has no result store; install output.WithResultStore at the root execution boundary before calling StoreResult")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.result != nil {
		return fmt.Errorf("output: command produced more than one framework result")
	}
	store.result = result
	return nil
}

// EmitStoredResult is called from PersistentPostRunE before output cleanup.
func EmitStoredResult(cmd *cobra.Command) (int, bool, error) {
	if cmd == nil {
		return 0, false, nil
	}
	store, ok := resultStoreFromContext(cmd.Context())
	if !ok {
		return 0, false, nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.result == nil || store.emitAttempted {
		return store.exitCode, store.emitted, nil
	}
	if !UsesUnifiedResult(cmd) {
		return 0, false, fmt.Errorf("output: legacy command %q produced a unified framework result", cmd.CommandPath())
	}
	store.emitAttempted = true
	store.exitCode = store.result.ExitCode()
	code, bytesRisk, err := emitResult(cmd, store.result)
	store.bytesRisk = bytesRisk
	if err != nil {
		// Never report the original success code after an output failure. Do not
		// attempt a second envelope: an io.Writer may have consumed bytes even
		// when it reported n == 0 or returned a rendering-adjacent error.
		store.exitCode = exitCodeInternal
		return store.exitCode, false, err
	}
	store.exitCode = code
	store.emitted = true
	return code, true, nil
}

// StoredEmissionState reports whether result emission was attempted. Once an
// attempt starts, callers must not try another envelope because a writer may
// have accepted bytes even when it returned an error.
func StoredEmissionState(store *ResultStore) (exitCode int, attempted, emitted, bytesRisk bool) {
	if store == nil {
		return 0, false, false, false
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.exitCode, store.emitAttempted, store.emitted, store.bytesRisk
}

// StoredExitCode returns the code produced by PersistentPostRunE.
func StoredExitCode(store *ResultStore) (int, bool) {
	if store == nil {
		return 0, false
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.exitCode, store.emitted
}

// EnvelopeFromResult returns a detached envelope copy for adapters/tests.
func EnvelopeFromResult(result CommandResult) (*Envelope, error) {
	if err := ValidateResult(result); err != nil {
		return nil, err
	}
	return result.envelope(), nil
}
