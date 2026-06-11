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

package audit

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDateRotatingWriter_RollsAndPrunes(t *testing.T) {
	dir := t.TempDir()
	w := NewDateRotatingWriter(dir, "audit", 7, 0o600, 0o700)

	// Drive a fake clock so the test is deterministic.
	day := time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC)
	w.now = func() time.Time { return day }

	if _, err := w.Write([]byte("d1-a\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("d1-b\n")); err != nil {
		t.Fatal(err)
	}

	// Next calendar day -> a new file.
	day = day.AddDate(0, 0, 1)
	if _, err := w.Write([]byte("d2-a\n")); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()

	f1 := filepath.Join(dir, "audit-2026-06-04.jsonl")
	f2 := filepath.Join(dir, "audit-2026-06-05.jsonl")
	if got := readFile(t, f1); got != "d1-a\nd1-b\n" {
		t.Errorf("day1 file = %q", got)
	}
	if got := readFile(t, f2); got != "d2-a\n" {
		t.Errorf("day2 file = %q", got)
	}
}

func TestDateRotatingWriter_PrunesOldFiles(t *testing.T) {
	dir := t.TempDir()

	// Seed an old file (well beyond retention) and an in-window file.
	old := filepath.Join(dir, "audit-2026-01-01.jsonl")
	recent := filepath.Join(dir, "audit-2026-06-03.jsonl")
	unrelated := filepath.Join(dir, "audit-notes.txt") // must be left alone
	for _, f := range []string{old, recent, unrelated} {
		if err := os.WriteFile(f, []byte("x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	w := NewDateRotatingWriter(dir, "audit", 7, 0o600, 0o700)
	w.now = func() time.Time { return time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC) }

	// Writing triggers openDay -> prune.
	if _, err := w.Write([]byte("today\n")); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("old file should have been pruned, stat err=%v", err)
	}
	if _, err := os.Stat(recent); err != nil {
		t.Errorf("in-window file must be kept: %v", err)
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Errorf("non-dated file must not be touched: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "audit-2026-06-04.jsonl")); err != nil {
		t.Errorf("today's file must exist: %v", err)
	}
}

func TestDateRotatingWriter_MaxAgeZeroKeepsAll(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "audit-2020-01-01.jsonl")
	if err := os.WriteFile(old, []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	w := NewDateRotatingWriter(dir, "audit", 0, 0o600, 0o700) // 0 = keep forever
	w.now = func() time.Time { return time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC) }
	if _, err := w.Write([]byte("today\n")); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	if _, err := os.Stat(old); err != nil {
		t.Errorf("with maxAge=0 nothing should be pruned: %v", err)
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}
