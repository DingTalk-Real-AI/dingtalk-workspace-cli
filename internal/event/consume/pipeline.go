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

package consume

import (
	"io"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

// Pipeline is the consumer-side delivery chain: format → route → sink.
// Each delivered event goes through formatting once; the routed sink then
// dispatches the formatted bytes to either a route-specific directory or
// the fallback (stdout/file).
//
// A Pipeline is bound to a single Config snapshot. Reconfiguring (changing
// format / routes mid-stream) is out of scope for v1.
type Pipeline struct {
	formatter Formatter
	sink      Sink
}

// NewPipeline builds a Pipeline for the given formatter and sink.
func NewPipeline(formatter Formatter, sink Sink) *Pipeline {
	return &Pipeline{formatter: formatter, sink: sink}
}

// Deliver renders ev with the configured formatter and hands the result
// to the sink. Returns ErrPipeClosed (re-raised from the sink) when the
// downstream stdout pipe closed; otherwise returns whatever formatting or
// sink error surfaced.
func (p *Pipeline) Deliver(ev transport.Event) error {
	body, err := p.formatter.Render(ev)
	if err != nil {
		return err
	}
	return p.sink.Write(ev, body)
}

// Close releases sink resources. Safe to call multiple times because
// underlying Sink Close methods are idempotent.
func (p *Pipeline) Close() error { return p.sink.Close() }

// BuildPipeline constructs a Pipeline from the cobra-side flag bundle. The
// cobra command first parses --format / --output-dir / --route into the
// derived inputs here so this function stays free of cobra dependencies.
//
// Sink selection rules (plan §3.1 输出约束):
//   - --route present → routed sink with per-rule dirs;
//     fallback is --output-dir if set, else stdout
//   - --output-dir only → file-per-event sink at the dir
//   - neither → stdout sink with stdoutW
//
// stdoutW is injected for tests (os.Stdout in production). When nil it
// defaults to io.Discard so a misconfigured pipeline never writes to
// the host process's actual stdout.
func BuildPipeline(format Format, outputDir string, routes []Route, stdoutW io.Writer) (*Pipeline, error) {
	fmter, err := NewFormatter(format)
	if err != nil {
		return nil, err
	}
	if stdoutW == nil {
		stdoutW = io.Discard
	}
	var fallback Sink
	if outputDir != "" {
		fallback = NewFileDirSink(outputDir)
	} else {
		fallback = NewStdoutSink(stdoutW)
	}
	if len(routes) > 0 {
		return NewPipeline(fmter, NewRoutedSink(NewRouter(routes), fallback)), nil
	}
	return NewPipeline(fmter, fallback), nil
}
