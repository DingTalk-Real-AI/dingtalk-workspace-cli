// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pipeline

import "fmt"

// Engine manages handler registration and executes the pipeline
// chain. Handlers are grouped by phase and executed in registration
// order within each phase. The engine is safe to use concurrently
// for reads after all handlers have been registered; registration
// itself is not concurrent-safe and should be done at startup.
type Engine struct {
	handlers                  map[Phase][]Handler
	commandPathFallbackLookup CommandPathFallbackLookup
}

// CommandPathFallback is the pipeline-local projection of one generated
// recovery record. The app injects the cli lookup so pipeline stays independent
// from the authored/generator package and can be tested with small fixtures.
type CommandPathFallback struct {
	From       string
	Mode       string
	To         string
	Candidates []string
}

// CommandPathFallbackLookup resolves an exact normalized path.
type CommandPathFallbackLookup func(path string) (CommandPathFallback, bool)

// HandlerError preserves the pipeline location of a handler failure for logs
// and diagnostics while keeping the underlying domain error available to
// user-facing adapters through Unwrap.
type HandlerError struct {
	Phase   Phase
	Handler string
	Cause   error
}

func (e *HandlerError) Error() string {
	return fmt.Sprintf("pipeline %s handler %q: %v", e.Phase, e.Handler, e.Cause)
}

func (e *HandlerError) Unwrap() error {
	return e.Cause
}

// NewEngine creates a pipeline engine with no registered handlers.
func NewEngine() *Engine {
	return &Engine{
		handlers: make(map[Phase][]Handler),
	}
}

// Register adds a handler to its declared phase. Handlers within
// the same phase execute in registration order.
func (e *Engine) Register(h Handler) {
	phase := h.Phase()
	e.handlers[phase] = append(e.handlers[phase], h)
}

// RegisterAll registers multiple handlers at once.
func (e *Engine) RegisterAll(handlers ...Handler) {
	for _, h := range handlers {
		e.Register(h)
	}
}

// SetCommandPathFallbackLookup installs the build-time generated command-path
// recovery lookup. It must be configured before the engine is used.
func (e *Engine) SetCommandPathFallbackLookup(lookup CommandPathFallbackLookup) {
	if e == nil {
		return
	}
	e.commandPathFallbackLookup = lookup
}

func (e *Engine) lookupCommandPathFallback(path string) (CommandPathFallback, bool) {
	if e == nil || e.commandPathFallbackLookup == nil {
		return CommandPathFallback{}, false
	}
	entry, ok := e.commandPathFallbackLookup(path)
	entry.Candidates = append([]string(nil), entry.Candidates...)
	return entry, ok
}

// Handlers returns the registered handlers for a given phase, in
// registration order. The returned slice must not be modified.
func (e *Engine) Handlers(phase Phase) []Handler {
	return e.handlers[phase]
}

// HasHandlers reports whether at least one handler is registered
// for the given phase.
func (e *Engine) HasHandlers(phase Phase) bool {
	return len(e.handlers[phase]) > 0
}

// RunPhase executes all handlers registered for the given phase in
// chain order. If any handler returns an error, execution stops
// immediately and the error is returned with the handler name as
// context.
func (e *Engine) RunPhase(phase Phase, ctx *Context) error {
	for _, h := range e.handlers[phase] {
		if err := h.Handle(ctx); err != nil {
			return &HandlerError{Phase: phase, Handler: h.Name(), Cause: err}
		}
	}
	return nil
}

// Run executes all phases in order: Register → PreParse → PostParse
// → PreRequest → PostResponse. Each phase runs its handler chain
// completely before the next phase begins.
//
// Callers typically do not use Run directly — the CLI integration
// calls RunPhase at each stage of the execution flow. Run is
// provided for testing and for cases where the full pipeline must
// be exercised in one shot.
func (e *Engine) Run(ctx *Context) error {
	for _, phase := range phases() {
		if err := e.RunPhase(phase, ctx); err != nil {
			return err
		}
	}
	return nil
}

// HandlerCount returns the total number of registered handlers
// across all phases.
func (e *Engine) HandlerCount() int {
	total := 0
	for _, handlers := range e.handlers {
		total += len(handlers)
	}
	return total
}
