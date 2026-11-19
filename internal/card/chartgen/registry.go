// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.

// Package chartgen builds A2UI chart cards from validated business facts.
package chartgen

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

//go:embed presets.json
var presetBytes []byte

type Preset struct {
	ID              string           `json:"id"`
	Alias           string           `json:"alias"`
	Family          string           `json:"family"`
	SourceSHA256    string           `json:"sourceSha256"`
	SeriesTypes     []string         `json:"seriesTypes"`
	SeriesStyles    []map[string]any `json:"seriesStyles"`
	CategoryAxis    string           `json:"categoryAxis"`
	Legend          bool             `json:"legend"`
	RadarStyle      map[string]any   `json:"radarStyle,omitempty"`
	RadarGroupCount int              `json:"radarGroupCount,omitempty"`
	VisualMapStyle  map[string]any   `json:"visualMapStyle,omitempty"`
}

type registry struct {
	Version             string   `json:"registryVersion"`
	SourceArchiveSHA256 string   `json:"sourceArchiveSha256"`
	Presets             []Preset `json:"presets"`
}

var (
	loadOnce sync.Once
	loaded   registry
	loadErr  error
)

func decodeStrict(data []byte, target any) error {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if err := exactJSONFields(raw, reflect.TypeOf(target)); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	// The initial Unmarshal already rejects trailing or multiple JSON values.
	return nil
}

// encoding/json accepts case-insensitive field aliases; ChartSpec does not.
func exactJSONFields(value any, typ reflect.Type) error {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == reflect.TypeOf(json.RawMessage{}) {
		return nil
	}
	switch typ.Kind() {
	case reflect.Struct:
		fields, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		known := map[string]reflect.Type{}
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "" {
				name = field.Name
			}
			if name != "-" {
				known[name] = field.Type
			}
		}
		for name, child := range fields {
			field, ok := known[name]
			if !ok {
				return fmt.Errorf("unknown field %q (field names are case-sensitive)", name)
			}
			if err := exactJSONFields(child, field); err != nil {
				return err
			}
		}
	case reflect.Array, reflect.Slice:
		if items, ok := value.([]any); ok {
			for _, item := range items {
				if err := exactJSONFields(item, typ.Elem()); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func load() (registry, error) {
	loadOnce.Do(func() {
		loadErr = decodeStrict(presetBytes, &loaded)
		if loadErr != nil {
			return
		}
		if loaded.Version != "1.0" || len(loaded.Presets) != 43 {
			loadErr = fmt.Errorf("unsupported or incomplete chart preset registry")
			return
		}
		seen := map[string]bool{}
		for _, preset := range loaded.Presets {
			if preset.ID == "G16" || preset.ID == "G25" || preset.ID == "" || preset.Alias == "" || preset.Family == "" || len(preset.SeriesTypes) == 0 || len(preset.SeriesTypes) != len(preset.SeriesStyles) {
				loadErr = fmt.Errorf("invalid chart preset %q", preset.ID)
				return
			}
			for _, name := range []string{strings.ToLower(preset.ID), strings.ToLower(preset.Alias)} {
				if seen[name] {
					loadErr = fmt.Errorf("duplicate chart preset name %q", name)
					return
				}
				seen[name] = true
			}
		}
	})
	return loaded, loadErr
}

func clonePreset(value Preset) Preset {
	result := value
	result.SeriesTypes = append([]string(nil), value.SeriesTypes...)
	result.SeriesStyles = make([]map[string]any, len(value.SeriesStyles))
	for i, style := range value.SeriesStyles {
		result.SeriesStyles[i] = make(map[string]any, len(style))
		for key, item := range style {
			result.SeriesStyles[i][key] = item
		}
	}
	result.RadarStyle = make(map[string]any, len(value.RadarStyle))
	for key, item := range value.RadarStyle {
		result.RadarStyle[key] = item
	}
	result.VisualMapStyle = make(map[string]any, len(value.VisualMapStyle))
	for key, item := range value.VisualMapStyle {
		result.VisualMapStyle[key] = item
	}
	return result
}

// List returns a detached copy of the 43 source-traceable visual presets.
func List() ([]Preset, error) {
	value, err := load()
	if err != nil {
		return nil, err
	}
	result := make([]Preset, len(value.Presets))
	for i, preset := range value.Presets {
		result[i] = clonePreset(preset)
	}
	return result, nil
}

// Lookup accepts a stable G ID or a semantic alias.
func Lookup(name string) (Preset, error) {
	value, err := load()
	if err != nil {
		return Preset{}, err
	}
	for _, preset := range value.Presets {
		if strings.EqualFold(name, preset.ID) || strings.EqualFold(name, preset.Alias) {
			return clonePreset(preset), nil
		}
	}
	return Preset{}, fmt.Errorf("unknown chart preset %q", name)
}
