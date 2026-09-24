// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package delivery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/card/a2ui/state"
)

type Record struct {
	Handle               string        `json:"handle"`
	BizID                string        `json:"bizId"`
	CardInstanceID       string        `json:"cardInstanceId,omitempty"`
	CreateRequestID      string        `json:"createRequestId,omitempty"`
	IdempotencyKeySHA256 string        `json:"idempotencyKeySha256,omitempty"`
	Profile              ProfileScope  `json:"profile"`
	ConversationID       string        `json:"conversationId,omitempty"`
	ReceiverID           string        `json:"receiverOpenDingTalkId,omitempty"`
	Surface              state.Surface `json:"surface"`
	FlowStatus           string        `json:"flowStatus"`
	Revision             int           `json:"revision"`
	UpdatedAt            time.Time     `json:"updatedAt"`
}

// ProfileScope freezes the non-sensitive identity and transport environment
// used to create a remote card. Update and finish must resolve to the same
// scope instead of accepting any explicit --profile value.
type ProfileScope struct {
	Selector    string `json:"selector"`
	CorpID      string `json:"corpId,omitempty"`
	UserID      string `json:"userId,omitempty"`
	Environment string `json:"environment"`
}

func (p ProfileScope) Matches(current ProfileScope) bool {
	if strings.TrimSpace(p.Environment) == "" || p.Environment != current.Environment {
		return false
	}
	if strings.TrimSpace(p.CorpID) == "" || strings.TrimSpace(p.UserID) == "" || strings.TrimSpace(current.CorpID) == "" || strings.TrimSpace(current.UserID) == "" {
		return false
	}
	return p.CorpID == current.CorpID && p.UserID == current.UserID
}

func (r Record) ValidateUpdateScope(current ProfileScope) error {
	if !r.Profile.Matches(current) {
		return fmt.Errorf("card handle %s belongs to a different profile or environment; create a new card in the current scope", r.Handle)
	}
	conversation := strings.TrimSpace(r.ConversationID) != ""
	receiver := strings.TrimSpace(r.ReceiverID) != ""
	if conversation == receiver {
		return fmt.Errorf("card handle %s has an invalid frozen target scope", r.Handle)
	}
	return nil
}

type Store struct{ Dir string }

func DefaultDir() string {
	if value := strings.TrimSpace(os.Getenv("DWS_CARD_STATE_DIR")); value != "" {
		return value
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return ".dws-card-state"
	}
	return filepath.Join(config, "dws", "cards")
}

func (s Store) Save(record Record) error {
	if err := validateHandle(record.Handle); err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	record.UpdatedAt = time.Now().UTC()
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.Dir, ".card-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(s.Dir, record.Handle+".json"))
}

// SaveIfRevision atomically verifies the ledger revision while the caller
// holds the handle lock. It rejects a stale writer instead of silently
// replacing a newer local transition.
func (s Store) SaveIfRevision(record Record, expectedRevision int) error {
	current, err := s.Load(record.Handle)
	if err != nil {
		return err
	}
	if current.Revision != expectedRevision {
		return fmt.Errorf("card handle %s revision conflict: current=%d expected=%d", record.Handle, current.Revision, expectedRevision)
	}
	return s.Save(record)
}

// WithHandleLock serializes one card handle across processes for the complete
// load -> remote call -> compare-and-save transition. A concurrent or stale
// lock fails closed; DWS never starts a second remote write for that handle.
func (s Store) WithHandleLock(handle string, fn func() error) error {
	if err := validateHandle(handle); err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	lockPath := filepath.Join(s.Dir, handle+".lock")
	lock, err := acquireHandleLock(lockPath)
	if err != nil {
		return fmt.Errorf("card handle %s is busy; another update may be running (lock %s): %w", handle, lockPath, err)
	}
	record := handleLockRecord{PID: os.Getpid(), AcquiredAt: time.Now().UTC()}
	raw, err := json.Marshal(record)
	if err != nil {
		_ = lock.Close()
		_ = os.Remove(lockPath)
		return err
	}
	_, writeErr := lock.Write(append(raw, '\n'))
	closeErr := lock.Close()
	if writeErr != nil {
		_ = os.Remove(lockPath)
		return writeErr
	}
	if closeErr != nil {
		_ = os.Remove(lockPath)
		return closeErr
	}
	defer os.Remove(lockPath)
	return fn()
}

const staleHandleLockAge = 10 * time.Minute

type handleLockRecord struct {
	PID        int       `json:"pid"`
	AcquiredAt time.Time `json:"acquiredAt"`
}

var handleLockProcessAlive = platformProcessAlive

func acquireHandleLock(path string) (*os.File, error) {
	for attempt := 0; attempt < 3; attempt++ {
		lock, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return lock, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		stale, staleErr := staleHandleLock(path, time.Now().UTC())
		if staleErr != nil || !stale {
			if staleErr != nil {
				return nil, staleErr
			}
			return nil, err
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	return nil, os.ErrExist
}

func staleHandleLock(path string, now time.Time) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	var record handleLockRecord
	if err := json.Unmarshal(raw, &record); err != nil || record.PID <= 0 || record.AcquiredAt.IsZero() {
		return now.Sub(info.ModTime()) > staleHandleLockAge, nil
	}
	alive, err := handleLockProcessAlive(record.PID)
	if err == nil {
		return !alive, nil
	}
	return now.Sub(record.AcquiredAt) > staleHandleLockAge, nil
}

func (s Store) Load(handle string) (Record, error) {
	if err := validateHandle(handle); err != nil {
		return Record{}, err
	}
	raw, err := os.ReadFile(filepath.Join(s.Dir, handle+".json"))
	if err != nil {
		return Record{}, err
	}
	var record Record
	if err := json.Unmarshal(raw, &record); err != nil {
		return Record{}, err
	}
	// Ledgers created before snapshot round-trip support did not persist the
	// envelope version. All supported stored surfaces use the v1.0 snapshot
	// contract, so normalize them on read without rewriting user state.
	if record.Surface.Version == "" {
		record.Surface.Version = "v1.0"
	}
	return record, nil
}

func validateHandle(handle string) error {
	if strings.ContainsAny(handle, `/\\`) || strings.TrimSpace(handle) == "" || handle == "." || handle == ".." {
		return fmt.Errorf("invalid card handle")
	}
	return nil
}

func (s Store) List() ([]Record, error) {
	entries, err := os.ReadDir(s.Dir)
	if os.IsNotExist(err) {
		return []Record{}, nil
	}
	if err != nil {
		return nil, err
	}
	var records []Record
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		record, err := s.Load(strings.TrimSuffix(entry.Name(), ".json"))
		if err == nil {
			records = append(records, record)
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].UpdatedAt.After(records[j].UpdatedAt) })
	return records, nil
}

func ExtractBizID(response any) string {
	keys := map[string]bool{"bizId": true, "bizID": true, "bizCardId": true}
	var visit func(any) string
	visit = func(value any) string {
		switch typed := value.(type) {
		case map[string]any:
			for key, child := range typed {
				if keys[key] {
					if text, ok := child.(string); ok && text != "" {
						return text
					}
				}
			}
			for _, child := range typed {
				if found := visit(child); found != "" {
					return found
				}
			}
		case []any:
			for _, child := range typed {
				if found := visit(child); found != "" {
					return found
				}
			}
		}
		return ""
	}
	return visit(response)
}

// ExtractCardInstanceID is best-effort: some IM responses identify the card
// only by bizId. The ledger keeps an empty value instead of inventing one.
func ExtractCardInstanceID(response any) string {
	var visit func(any) string
	visit = func(value any) string {
		switch typed := value.(type) {
		case map[string]any:
			if raw, ok := typed["cardInstanceId"]; ok {
				switch id := raw.(type) {
				case string:
					return strings.TrimSpace(id)
				case json.Number:
					return id.String()
				case float64:
					if id > 0 && id == float64(int64(id)) {
						return strconv.FormatInt(int64(id), 10)
					}
				case int64:
					if id > 0 {
						return strconv.FormatInt(id, 10)
					}
				case int:
					if id > 0 {
						return strconv.Itoa(id)
					}
				}
			}
			for _, child := range typed {
				if found := visit(child); found != "" {
					return found
				}
			}
		case []any:
			for _, child := range typed {
				if found := visit(child); found != "" {
					return found
				}
			}
		}
		return ""
	}
	return visit(response)
}
