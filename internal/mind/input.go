// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.

// Package mind validates the public mind-v1 input. Internal checkpoints and
// collaboration protocols remain owned by Solution, State and the mind model.
package mind

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/jsonutil"
)

const MaxSourceBytes = 2 * 1024 * 1024
const MaxNodeJSONBytes = 128 * 1024

var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

type Topic struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Text     string `json:"text"`
	ParentID string `json:"parentId,omitempty"`
}

type Page struct {
	ID    string  `json:"id"`
	Title string  `json:"title"`
	Nodes []Topic `json:"nodes"`
}

type Source struct {
	SchemaVersion  string `json:"schemaVersion"`
	CatalogVersion string `json:"catalogVersion"`
	Pages          []Page `json:"pages"`
}

func ValidateID(value string) error {
	if !idPattern.MatchString(value) {
		return fmt.Errorf("ID must match %s", idPattern)
	}
	return nil
}

func ValidateText(value string) error {
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > 10000 {
		return fmt.Errorf("text must be valid UTF-8 with at most 10000 characters")
	}
	return nil
}

// Display names use Java String.length; OpenNodes page titles use code points.
func ValidateName(value string) error {
	if strings.TrimSpace(value) == "" || !utf8.ValidString(value) || len(utf16.Encode([]rune(value))) > 128 {
		return fmt.Errorf("name must be nonblank valid UTF-8 within 128 UTF-16 units")
	}
	return nil
}

func validatePageTitle(value string) error {
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) < 1 || utf8.RuneCountInString(value) > 255 {
		return fmt.Errorf("page title must contain 1-255 Unicode characters")
	}
	return nil
}

var cursorPattern = regexp.MustCompile(`^mindc1_[A-Za-z0-9_-]{43}$`)

func ValidateCursor(value string) error {
	if value != "" && !cursorPattern.MatchString(value) {
		return fmt.Errorf("next-token must be the mindc1_ token returned by the previous query (50 characters)")
	}
	return nil
}

func ValidateVersion(value string) error {
	if value == "" || (len(value) > 1 && value[0] == '0') || strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return fmt.Errorf("version must be a decimal string")
	}
	if _, err := strconv.ParseUint(value, 10, 31); err != nil {
		return fmt.Errorf("version must be within 0..2147483647: %w", err)
	}
	return nil
}

// object checks exact property spelling as encoding/json accepts case aliases.
func object(data []byte, required []string, optional ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, fmt.Errorf("expected a JSON object")
	}
	allowed := map[string]bool{}
	for _, k := range append(append([]string{}, required...), optional...) {
		allowed[k] = true
	}
	for k := range fields {
		if !allowed[k] {
			return nil, fmt.Errorf("unsupported field %q", k)
		}
	}
	for _, k := range required {
		if v, ok := fields[k]; !ok || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return nil, fmt.Errorf("missing or null field %q", k)
		}
	}
	return fields, nil
}

func parseTopic(data []byte, parentAllowed bool) (Topic, error) {
	var topic Topic
	var optional []string
	if parentAllowed {
		optional = []string{"parentId"}
	}
	fields, err := object(data, []string{"id", "type", "text"}, optional...)
	if err != nil {
		return topic, err
	}
	if raw, ok := fields["parentId"]; ok && (bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || bytes.Equal(bytes.TrimSpace(raw), []byte(`""`))) {
		return topic, fmt.Errorf("parentId must be omitted for a root")
	}
	if err := json.Unmarshal(data, &topic); err != nil {
		return topic, err
	}
	if err := ValidateID(topic.ID); err != nil {
		return topic, err
	}
	if topic.Type != "topic" {
		return topic, fmt.Errorf("type must be topic")
	}
	if err := ValidateText(topic.Text); err != nil {
		return topic, err
	}
	if topic.ParentID != "" {
		if err := ValidateID(topic.ParentID); err != nil {
			return topic, err
		}
	}
	return topic, nil
}

func ParseTopic(data []byte) (Topic, error) {
	if len(data) > MaxNodeJSONBytes || !utf8.Valid(data) {
		return Topic{}, fmt.Errorf("topic must be valid UTF-8 within 128 KiB")
	}
	if err := jsonutil.RejectDuplicateObjectKeys(data); err != nil {
		return Topic{}, err
	}
	return parseTopic(data, false)
}

func ParseSource(data []byte) (*Source, error) {
	if len(data) > MaxSourceBytes || !utf8.Valid(data) {
		return nil, fmt.Errorf("source must be valid UTF-8 within 2 MiB")
	}
	if err := jsonutil.RejectDuplicateObjectKeys(data); err != nil {
		return nil, err
	}
	fields, err := object(data, []string{"schemaVersion", "catalogVersion", "pages"})
	if err != nil {
		return nil, err
	}
	var header struct {
		SchemaVersion  string `json:"schemaVersion"`
		CatalogVersion string `json:"catalogVersion"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return nil, err
	}
	if header.SchemaVersion != "1.0" || header.CatalogVersion != "mind-v1" {
		return nil, fmt.Errorf("source requires schemaVersion=1.0 and catalogVersion=mind-v1")
	}
	var pages []json.RawMessage
	if err := json.Unmarshal(fields["pages"], &pages); err != nil {
		return nil, err
	}
	if len(pages) < 1 || len(pages) > 20 {
		return nil, fmt.Errorf("source must contain 1-20 pages")
	}
	source := Source{SchemaVersion: header.SchemaVersion, CatalogVersion: header.CatalogVersion}
	pageIDs, topicIDs := map[string]bool{}, map[string]bool{}
	for pi, raw := range pages {
		pf, err := object(raw, []string{"id", "title", "nodes"})
		if err != nil {
			return nil, fmt.Errorf("pages[%d]: %w", pi, err)
		}
		var pageHeader struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}
		if err := json.Unmarshal(raw, &pageHeader); err != nil {
			return nil, fmt.Errorf("pages[%d]: %w", pi, err)
		}
		page := Page{ID: pageHeader.ID, Title: pageHeader.Title}
		if err := ValidateID(page.ID); err != nil {
			return nil, fmt.Errorf("pages[%d].id: %w", pi, err)
		}
		if pageIDs[page.ID] {
			return nil, fmt.Errorf("duplicate page ID %q", page.ID)
		}
		pageIDs[page.ID] = true
		if err := validatePageTitle(page.Title); err != nil {
			return nil, fmt.Errorf("pages[%d].title: %w", pi, err)
		}
		var nodes []json.RawMessage
		if err := json.Unmarshal(pf["nodes"], &nodes); err != nil {
			return nil, err
		}
		parents := map[string]string{}
		roots := 0
		for ni, node := range nodes {
			topic, err := parseTopic(node, true)
			if err != nil {
				return nil, fmt.Errorf("pages[%d].nodes[%d]: %w", pi, ni, err)
			}
			if topicIDs[topic.ID] {
				return nil, fmt.Errorf("duplicate topic ID %q", topic.ID)
			}
			topicIDs[topic.ID] = true
			if len(topicIDs) > 10000 {
				return nil, fmt.Errorf("source exceeds 10000 topics")
			}
			page.Nodes = append(page.Nodes, topic)
			parents[topic.ID] = topic.ParentID
			if topic.ParentID == "" {
				roots++
			}
		}
		source.Pages = append(source.Pages, page)
		if roots != 1 {
			return nil, fmt.Errorf("page %q must have exactly one root", page.ID)
		}
		for id := range parents {
			seen := map[string]bool{}
			for current, depth := id, 1; current != ""; depth++ {
				if depth > 64 || seen[current] {
					return nil, fmt.Errorf("page %q topic %q has a cycle or depth greater than 64", page.ID, id)
				}
				seen[current] = true
				parent, ok := parents[current]
				if !ok {
					return nil, fmt.Errorf("page %q topic %q references missing parent %q", page.ID, id, current)
				}
				current = parent
			}
		}
	}
	return &source, nil
}
