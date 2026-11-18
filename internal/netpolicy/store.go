// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
package netpolicy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// File is separate from authentication state so it can be configured before login.
type File struct {
	Version  int                 `json:"version"`
	Default  Settings            `json:"network"`
	Profiles map[string]Settings `json:"profiles,omitempty"`
}

func Load(dir string) (File, error) {
	f := File{Version: 1}
	path := filepath.Join(dir, "network.json")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return f, nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return f, fmt.Errorf("network: configuration must be a readable regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return f, fmt.Errorf("network: cannot read configuration")
	}
	if json.Unmarshal(data, &f) != nil || f.Version != 1 {
		return f, fmt.Errorf("network: invalid or unsupported configuration version")
	}
	return f, nil
}

func (f File) Settings(profile string) Settings {
	s := f.Default
	if v, ok := f.Profiles[profile]; ok && profile != "" {
		if v.Proxy != nil {
			s.Proxy = v.Proxy
		}
		if v.NoProxy != nil {
			s.NoProxy = v.NoProxy
		}
		if v.CABundle != nil {
			s.CABundle = v.CABundle
		}
	}
	return s
}

// Save publishes a validated non-secret snapshot atomically. Callers serialize
// read-modify-write with the existing config-directory lock.
func Save(dir string, f File) error {
	validate := func(s Settings) error {
		if s.Proxy != nil && *s.Proxy != "auto" && *s.Proxy != "direct" {
			u, err := parseProxy(*s.Proxy, false)
			if err != nil {
				return err
			}
			if u.User != nil {
				return fmt.Errorf("network: credentials must be supplied through DWS_PROXY, not persisted")
			}
		}
		p, err := Resolve(s, Overrides{}, Environment{})
		if p != nil {
			p.CloseIdleConnections()
		}
		return err
	}
	if err := validate(f.Default); err != nil {
		return err
	}
	for _, s := range f.Profiles {
		if err := validate(s); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".network-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(append(data, '\n')); err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, "network.json"))
}
