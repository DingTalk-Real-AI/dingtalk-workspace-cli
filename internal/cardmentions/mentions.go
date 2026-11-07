// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package cardmentions

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// ReservedMentionAll is the atId reserved for mentioning everyone; it is
// expressed in the Markdown body and must not appear in the declared
// mention list.
const ReservedMentionAll = "all"

// ReservedMentionAllWire is the highlight alias key the server expects for
// the @everyone mention. Bodies keep the readable reserved ID "all" while
// authoring and validating; send paths rewrite the tags to this value on the
// wire together with atAll=true.
const ReservedMentionAllWire = "10"

// MaxMentionDeclarations is the server-side raw declaration budget: the
// number of declared mention ids, duplicates included, must not exceed it.
const MaxMentionDeclarations = 100

var (
	atTagPattern         = regexp.MustCompile(`<a\s+atId\s*=\s*"([^"]+)"\s*>`)
	atAllTagPattern      = regexp.MustCompile(`<a\s+atId\s*=\s*"all"\s*>`)
	imPlaceholderPattern = regexp.MustCompile(`<@[^<>]*>`)
)

// MentionOccurrence is one @-mention token found in a literal Markdown body.
type MentionOccurrence struct {
	Token   string
	Pointer string
}

// MentionUsage summarizes the @-mention usage of A2UI message JSON strings.
// Literal Markdown and same-batch data paths are statically visible. Future
// bound values and expressions remain a runtime concern.
type MentionUsage struct {
	AtTags         []MentionOccurrence
	IMPlaceholders []MentionOccurrence
	HasBindings    bool
}

// AtIDs returns the distinct non-reserved atIds in first-seen order.
func (u MentionUsage) AtIDs() []string {
	out := make([]string, 0, len(u.AtTags))
	seen := map[string]struct{}{}
	for _, o := range u.AtTags {
		if o.Token == ReservedMentionAll {
			continue
		}
		if _, ok := seen[o.Token]; ok {
			continue
		}
		seen[o.Token] = struct{}{}
		out = append(out, o.Token)
	}
	return out
}

// HasAtAll reports whether the reserved <a atId="all"> tag is used.
func (u MentionUsage) HasAtAll() bool {
	for _, o := range u.AtTags {
		if o.Token == ReservedMentionAll {
			return true
		}
	}
	return false
}

// ExtractMentions scans literal Markdown and same-batch absolute data paths
// referenced by Markdown components. Code examples are not notifications.
func ExtractMentions(messages []string) MentionUsage {
	bodies, bound := mentionBodies(messages)
	usage := MentionUsage{HasBindings: bound}
	for _, body := range bodies {
		ptr := fmt.Sprintf("/%d%s", body.message, body.pointer)
		for _, match := range atTagPattern.FindAllStringSubmatch(mentionMarkdown(body.content), -1) {
			usage.AtTags = append(usage.AtTags, MentionOccurrence{match[1], ptr})
		}
		for _, token := range imPlaceholderPattern.FindAllString(mentionMarkdown(body.content), -1) {
			usage.IMPlaceholders = append(usage.IMPlaceholders, MentionOccurrence{token, ptr})
		}
	}

	return usage
}

// MentionIMPlaceholderDiagnostics reports IM-style <@...> placeholders, which
// the card flow renders literally instead of mentioning anyone.
func MentionIMPlaceholderDiagnostics(content, ptr string) []Diagnostic {
	tokens := imPlaceholderPattern.FindAllString(mentionMarkdown(content), -1)
	if len(tokens) == 0 {
		return nil
	}
	return []Diagnostic{{
		Code:     "mention.im_placeholder",
		Severity: "error",
		Pointer:  ptr,
		Message:  fmt.Sprintf("IM-style %s placeholders do not work in cards and are displayed literally; use <a atId=\"openDingTalkId\">nickname</a> and list the id via --at-open-dingtalk-ids", strings.Join(tokens, " ")),
	}}
}

// ValidateMentionConsistency enforces the bidirectional contract between the
// declared mention ids and the <a atId="..."> tags in the card Markdown
// bodies.
func ValidateMentionConsistency(messages []string, declared []string, atAll bool) error {
	if mentionsDisabled() && (len(declared) > 0 || atAll) {
		return fmt.Errorf("A2UI @ notifications disabled by DWS_A2UI_MENTIONS_DISABLED")
	}

	// With explicit notification intent, malformed JSON must fail before the
	// best-effort extractor can turn it into a misleading unused-id error.
	if len(declared) > 0 || atAll {
		for i, raw := range messages {
			if _, err := decode([]byte(raw)); err != nil {
				return fmt.Errorf("A2UI 消息[%d] JSON 无效: %w", i, err)
			}
		}
	}
	usage := ExtractMentions(messages)
	used := map[string]struct{}{}
	for _, id := range usage.AtIDs() {
		used[id] = struct{}{}
	}
	declaredSet := map[string]struct{}{}
	unused := make([]string, 0)
	for _, id := range declared {
		declaredSet[id] = struct{}{}
		if _, ok := used[id]; !ok {
			unused = append(unused, id)
		}
	}
	if len(unused) > 0 && !usage.HasBindings {
		return fmt.Errorf("--at-open-dingtalk-ids 声明的成员必须在卡片 Markdown 中使用 <a atId=\"openDingTalkId\">；未使用: %s", strings.Join(unused, ","))
	}
	undeclared := make([]string, 0)
	for _, id := range usage.AtIDs() {
		if _, ok := declaredSet[id]; !ok {
			undeclared = append(undeclared, id)
		}
	}
	if len(undeclared) > 0 {
		return fmt.Errorf("卡片 Markdown 中的 <a atId=\"openDingTalkId\"> 必须同时通过 --at-open-dingtalk-ids 声明；未声明: %s", strings.Join(undeclared, ","))
	}
	if atAll && !usage.HasAtAll() && !usage.HasBindings {
		return fmt.Errorf("--at-all 需要卡片 Markdown 中同时包含 <a atId=\"all\">everyone</a>")
	}
	if !atAll && usage.HasAtAll() {
		return fmt.Errorf("卡片 Markdown 中的 <a atId=\"all\"> 必须同时指定 --at-all")
	}
	return nil
}

// MentionDeclarationDiagnostics is the offline lint counterpart of
// ValidateMentionConsistency: the declared list is supplied explicitly.
func MentionDeclarationDiagnostics(messages []string, declared []string) []Diagnostic {
	usage := ExtractMentions(messages)
	declaredSet := map[string]struct{}{}
	for _, id := range declared {
		declaredSet[id] = struct{}{}
	}
	diagnostics := []Diagnostic{}
	for _, o := range usage.AtTags {
		if o.Token == ReservedMentionAll {
			continue
		}
		if _, ok := declaredSet[o.Token]; !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "mention.undeclared",
				Severity: "error",
				Pointer:  o.Pointer,
				Message:  fmt.Sprintf("<a atId=%q> is not listed in --at-open-dingtalk-ids; list it at send time or it gets no highlight or notification, because the server only notifies declared ids", o.Token),
			})
		}
	}
	used := map[string]struct{}{}
	for _, id := range usage.AtIDs() {
		used[id] = struct{}{}
	}
	for _, id := range declared {
		if _, ok := used[id]; !ok && !usage.HasBindings {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "mention.declared_unused",
				Severity: "error",
				Pointer:  "",
				Message:  fmt.Sprintf("--at-open-dingtalk-ids lists %q but no literal Markdown body uses <a atId=%q>", id, id),
			})
		}
	}
	return diagnostics
}

// RewriteMentionAllTags rewrites literal Markdown <a atId="all"> tags to the
// wire alias ReservedMentionAllWire so the server highlights the @everyone
// mention; creation pairs the rewritten body with atAll=true. Literal content
// and resolved same-batch binding sources are patched in place. Other JSON
// bytes, unresolved bindings and malformed messages remain unchanged.
func RewriteMentionAllTags(messages []string) []string {
	if mentionsDisabled() {
		return append([]string(nil), messages...)
	}

	bodies, _ := mentionBodies(messages)
	replacements := make([]map[string]string, len(messages))
	for _, body := range bodies {
		next := rewriteMarkdownAll(body.content)
		if next == body.content {
			continue
		}
		if replacements[body.message] == nil {
			replacements[body.message] = map[string]string{}
		}
		replacements[body.message][body.pointer] = next
	}
	out := make([]string, len(messages))
	for i, raw := range messages {
		out[i] = rewriteMentionJSON(raw, replacements[i])
	}
	return out
}

func rewriteMarkdownAll(content string) string {
	matches := atAllTagPattern.FindAllStringIndex(mentionMarkdown(content), -1)
	for i := len(matches) - 1; i >= 0; i-- {
		span := matches[i]
		content = content[:span[0]] + `<a atId="` + ReservedMentionAllWire + `">` + content[span[1]:]
	}
	return content
}

func mentionsDisabled() bool {
	value := os.Getenv("DWS_A2UI_MENTIONS_DISABLED")
	return value == "1" || strings.EqualFold(value, "true")
}
