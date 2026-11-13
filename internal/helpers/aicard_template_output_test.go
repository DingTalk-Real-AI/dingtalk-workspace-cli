// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package helpers

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCrossPlatformCoverageAicardTemplateOutputFileIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "card.json")
	if err := writeAicardTemplateFile(path, []byte("not-published"), path); !errors.Is(err, errAicardTemplateOutputConflict) {
		t.Fatalf("created-file alias was accepted: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("conflicting new file was retained: %v", err)
	}
	if err := writeAicardTemplateFile(path, []byte("preserved"), filepath.Join(filepath.Dir(path), "result.json")); err != nil {
		t.Fatal(err)
	}
	if err := writeAicardTemplateFile(path, []byte("replacement"), ""); err == nil {
		t.Fatal("existing file was overwritten")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "preserved" {
		t.Fatalf("existing bytes changed: %q %v", data, err)
	}
}
