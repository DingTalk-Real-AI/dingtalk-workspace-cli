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

// Handler is the core abstraction for pipeline extensions. Each
// handler declares the phase it belongs to and provides a Handle
// method that receives a mutable context. Handlers are executed in
// registration order within a phase; each handler's output becomes
// the next handler's input.
//
// A handler that returns a non-nil error aborts the chain — no
// further handlers in the same phase (or subsequent phases) will
// run.
type Handler interface {
	// Name returns a short, unique identifier for the handler
	// (e.g. "sticky", "alias", "date-normalise"). Used in
	// correction records and log output.
	Name() string

	// Phase returns the pipeline phase this handler belongs to.
	Phase() Phase

	// Handle processes the context and returns an error to abort
	// the chain, or nil to continue.
	Handle(ctx *Context) error
}
