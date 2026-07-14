package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type Sink interface {
	Emit(event *Event) error
	Close() error
}

type NopSink struct{}

func (NopSink) Emit(*Event) error { return nil }
func (NopSink) Close() error      { return nil }

type FileSink struct {
	writer    *DateRotatingWriter
	chain     *Chain
	forwarder *HTTPForwarder
}

func NewFileSink(writer *DateRotatingWriter, chain *Chain, forwarder *HTTPForwarder) *FileSink {
	return &FileSink{
		writer:    writer,
		chain:     chain,
		forwarder: forwarder,
	}
}

func (s *FileSink) Emit(evt *Event) error {
	body, err := marshalWithoutHash(evt)
	if err != nil {
		return fmt.Errorf("audit: marshal event: %w", err)
	}

	f, release, err := s.writer.beginAppend()
	if err != nil {
		return fmt.Errorf("audit: acquire writer: %w", err)
	}

	// Derive prev_hash from the file tail, seal, and append — all under the
	// writer's process + inter-process lock so the chain cannot fork.
	prevHash, hash, err := s.chain.SealFromFile(f, body)
	if err != nil {
		release()
		return fmt.Errorf("audit: seal event: %w", err)
	}
	evt.PrevHash = prevHash
	evt.Hash = hash

	line, err := json.Marshal(evt)
	if err != nil {
		release()
		return fmt.Errorf("audit: marshal final event: %w", err)
	}
	line = append(line, '\n')
	if _, err := f.Write(line); err != nil {
		release()
		return fmt.Errorf("audit: write event: %w", err)
	}
	release()

	if s.forwarder != nil {
		s.forwarder.Forward(*evt)
	}

	return nil
}

// Close flushes in-flight remote forwards (bounded) before closing the writer,
// so events are not silently dropped when the CLI process exits right after
// emitting.
func (s *FileSink) Close() error {
	var forwardErr error
	if s.forwarder != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		forwardErr = s.forwarder.Close(ctx)
		cancel()
	}
	if err := s.writer.Close(); err != nil {
		return err
	}
	return forwardErr
}

func marshalWithoutHash(evt *Event) ([]byte, error) {
	saved := Event{
		Timestamp:     evt.Timestamp,
		ExecutionID:   evt.ExecutionID,
		AgentID:       evt.AgentID,
		Actor:         evt.Actor,
		Product:       evt.Product,
		Command:       evt.Command,
		Endpoint:      evt.Endpoint,
		ParamsSummary: evt.ParamsSummary,
		Result:        evt.Result,
		ErrCategory:   evt.ErrCategory,
		ErrReason:     evt.ErrReason,
		DurationMs:    evt.DurationMs,
		CLIVersion:    evt.CLIVersion,
		OS:            evt.OS,
		Arch:          evt.Arch,
	}
	return json.Marshal(saved)
}
