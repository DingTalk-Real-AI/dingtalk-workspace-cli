package recovery

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	recoveryDirName = "recovery"
	lastErrorFile   = "last_error.json"
	eventsFile      = "recovery_events.jsonl"
	recoveryMaxAge  = 30 * 24 * time.Hour
)

type Store struct {
	baseDir string
}

var runtimeState struct {
	mu          sync.Mutex
	lastCapture *LastError
}

func NewStore(configDir string) *Store {
	return &Store{baseDir: configDir}
}

func (s *Store) Enabled() bool {
	return s != nil && s.baseDir != ""
}

func (s *Store) Capture(ctx RecoveryContext, replay ...Replay) (*LastError, error) {
	if !s.Enabled() {
		return nil, nil
	}

	last := &LastError{
		EventID:    newEventID(),
		RecordedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Context:    ctx,
	}
	if len(replay) > 0 {
		last.Replay = cloneReplay(replay[0])
	}
	if err := s.writeJSON(s.lastErrorPath(), last); err != nil {
		return nil, err
	}
	var replayPtr *Replay
	if !isEmptyReplay(last.Replay) {
		copied := cloneReplay(last.Replay)
		replayPtr = &copied
	}
	if err := s.appendEvent(RecoveryEvent{
		EventID:    last.EventID,
		Phase:      "captured",
		RecordedAt: last.RecordedAt,
		Context:    &last.Context,
		Replay:     replayPtr,
	}); err != nil {
		return nil, err
	}
	setLatestCapture(last)
	return last, nil
}

func (s *Store) LoadLastError() (*LastError, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("recovery store disabled")
	}
	if err := s.pruneExpiredArtifacts(time.Now().UTC()); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(s.lastErrorPath())
	if err != nil {
		return nil, err
	}
	var last LastError
	if err := json.Unmarshal(data, &last); err != nil {
		return nil, err
	}
	return &last, nil
}

func (s *Store) LoadErrorByEvent(eventID string) (*LastError, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("recovery store disabled")
	}
	if eventID == "" {
		return nil, fmt.Errorf("event id is required")
	}
	if err := s.pruneExpiredArtifacts(time.Now().UTC()); err != nil {
		return nil, err
	}

	file, err := os.Open(s.eventsPath())
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		file = nil
	}
	if file != nil {
		defer func() {
			_ = file.Close()
		}()
		reader := bufio.NewReader(file)
		for {
			line, err := reader.ReadBytes('\n')
			if len(line) > 0 {
				var event RecoveryEvent
				if json.Unmarshal(line, &event) == nil && event.EventID == eventID && event.Phase == "captured" && event.Context != nil {
					last := &LastError{
						EventID:    event.EventID,
						RecordedAt: event.RecordedAt,
						Context:    cloneRecoveryContext(*event.Context),
					}
					if event.Replay != nil {
						last.Replay = cloneReplay(*event.Replay)
					}
					return last, nil
				}
			}
			if err != nil {
				if err == io.EOF {
					break
				}
				return nil, err
			}
		}
	}

	last, err := s.LoadLastError()
	if err == nil && last != nil && last.EventID == eventID {
		return last, nil
	}
	return nil, fmt.Errorf("未找到 event_id=%s 对应的失败快照", eventID)
}

func (s *Store) SavePlan(eventID string, plan RecoveryPlan) error {
	if !s.Enabled() {
		return nil
	}
	return s.appendEvent(RecoveryEvent{
		EventID:    eventID,
		Phase:      "planned",
		RecordedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Plan:       &plan,
	})
}

func (s *Store) SaveAnalysis(eventID string, plan RecoveryPlan, bundle RecoveryBundle) error {
	if !s.Enabled() {
		return nil
	}
	return s.appendEvent(RecoveryEvent{
		EventID:    eventID,
		Phase:      "analyzed",
		RecordedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Plan:       &plan,
		Bundle:     &bundle,
	})
}

func (s *Store) Finalize(eventID, outcome string, exec *RecoveryExecution) error {
	if !s.Enabled() {
		return nil
	}
	return s.appendEvent(RecoveryEvent{
		EventID:    eventID,
		Phase:      "finalized",
		RecordedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Execution:  exec,
		Outcome:    outcome,
	})
}

func (s *Store) writeJSON(path string, payload any) error {
	if err := s.ensureDir(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func (s *Store) appendEvent(event RecoveryEvent) error {
	if err := s.ensureDir(); err != nil {
		return err
	}
	file, err := os.OpenFile(s.eventsPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
	}()

	writer := bufio.NewWriter(file)
	defer func() {
		_ = writer.Flush()
	}()

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := writer.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func (s *Store) ensureDir() error {
	if !s.Enabled() {
		return fmt.Errorf("recovery store disabled")
	}
	if err := os.MkdirAll(s.dir(), 0o700); err != nil {
		return err
	}
	return s.pruneExpiredArtifacts(time.Now().UTC())
}

func (s *Store) dir() string {
	return filepath.Join(s.baseDir, recoveryDirName)
}

func (s *Store) lastErrorPath() string {
	return filepath.Join(s.dir(), lastErrorFile)
}

func (s *Store) eventsPath() string {
	return filepath.Join(s.dir(), eventsFile)
}

func newEventID() string {
	return fmt.Sprintf("evt_%d", time.Now().UTC().UnixNano())
}

func LatestCapture() *LastError {
	runtimeState.mu.Lock()
	defer runtimeState.mu.Unlock()
	if runtimeState.lastCapture == nil {
		return nil
	}
	copied := *runtimeState.lastCapture
	copied.Context = cloneRecoveryContext(copied.Context)
	copied.Replay = cloneReplay(copied.Replay)
	return &copied
}

func ResetRuntimeState() {
	runtimeState.mu.Lock()
	defer runtimeState.mu.Unlock()
	runtimeState.lastCapture = nil
}

func setLatestCapture(last *LastError) {
	runtimeState.mu.Lock()
	defer runtimeState.mu.Unlock()
	if last == nil {
		runtimeState.lastCapture = nil
		return
	}
	copied := *last
	copied.Context = cloneRecoveryContext(copied.Context)
	copied.Replay = cloneReplay(copied.Replay)
	runtimeState.lastCapture = &copied
}

func cloneRecoveryContext(ctx RecoveryContext) RecoveryContext {
	cloned := ctx
	if len(ctx.CommandPath) > 0 {
		cloned.CommandPath = append([]string(nil), ctx.CommandPath...)
	}
	if len(ctx.ArgsSummary) > 0 {
		cloned.ArgsSummary = cloneMap(ctx.ArgsSummary)
	}
	return cloned
}

func cloneReplay(replay Replay) Replay {
	cloned := replay
	if len(replay.ToolArgs) > 0 {
		cloned.ToolArgs = cloneMap(replay.ToolArgs)
	}
	if len(replay.RedactedArgv) > 0 {
		cloned.RedactedArgv = append([]string(nil), replay.RedactedArgv...)
	}
	return cloned
}

func isEmptyReplay(replay Replay) bool {
	return replay.ServerID == "" &&
		replay.ToolName == "" &&
		replay.OperationKind == "" &&
		len(replay.ToolArgs) == 0 &&
		len(replay.RedactedArgv) == 0 &&
		replay.RedactedCommand == ""
}

func cloneMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = cloneValue(value)
	}
	return dst
}

func cloneSlice(src []any) []any {
	if len(src) == 0 {
		return nil
	}
	dst := make([]any, len(src))
	for i, value := range src {
		dst[i] = cloneValue(value)
	}
	return dst
}

func cloneValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return cloneMap(v)
	case []any:
		return cloneSlice(v)
	case []string:
		out := make([]string, len(v))
		copy(out, v)
		return out
	default:
		return v
	}
}

func (s *Store) pruneExpiredArtifacts(now time.Time) error {
	if !s.Enabled() {
		return nil
	}
	cutoff := now.Add(-recoveryMaxAge)
	if err := s.pruneLastError(cutoff); err != nil {
		return err
	}
	if err := s.pruneEvents(cutoff); err != nil {
		return err
	}
	return s.pruneOtherArtifacts(cutoff)
}

func (s *Store) pruneLastError(cutoff time.Time) error {
	path := s.lastErrorPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var last LastError
	if err := json.Unmarshal(data, &last); err == nil {
		if recordedAt, ok := parseRecordedAt(last.RecordedAt); ok && recordedAt.Before(cutoff) {
			return os.Remove(path)
		}
		return nil
	}

	info, statErr := os.Stat(path)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return nil
		}
		return statErr
	}
	if info.ModTime().Before(cutoff) {
		return os.Remove(path)
	}
	return nil
}

func (s *Store) pruneEvents(cutoff time.Time) error {
	path := s.eventsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event RecoveryEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			kept = append(kept, line)
			continue
		}
		recordedAt, ok := parseRecordedAt(event.RecordedAt)
		if ok && recordedAt.Before(cutoff) {
			continue
		}
		kept = append(kept, line)
	}

	if len(kept) == 0 {
		return os.Remove(path)
	}
	content := strings.Join(kept, "\n") + "\n"
	return os.WriteFile(path, []byte(content), 0o600)
}

func (s *Store) pruneOtherArtifacts(cutoff time.Time) error {
	entries, err := os.ReadDir(s.dir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		name := entry.Name()
		if name == lastErrorFile || name == eventsFile {
			continue
		}
		path := filepath.Join(s.dir(), name)
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if info.ModTime().Before(cutoff) {
			if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}

func parseRecordedAt(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}
