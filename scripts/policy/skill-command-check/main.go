// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

// Command skill-command-check verifies that executable `dws ...` references
// in published skill Markdown resolve against the current Cobra command tree.
package main

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/spf13/cobra"
)

var (
	inlineCommand = regexp.MustCompile("`(dws\\s+[^`]+)`")
	lineCommand   = regexp.MustCompile(`^\s*(?:[>$]\s*)?(dws\s+.+?)\s*$`)
	antiMarkers   = []string{
		"禁止", "不存在", "不要使用", "不要用", "错误写法", "错误命令",
		"反模式", "反例", "错例", "臆造", "虚构", "不支持", "unknown ",
		"❌", "×", "已下线", "废弃",
	}
	antiCommands = map[string]bool{
		"dws calendar list":         true,
		"dws minutes detail":        true,
		"dws minutes info":          true,
		"dws minutes summary":       true,
		"dws minutes transcribe":    true,
		"dws minutes transcription": true,
		"dws report inbox":          true,
	}
)

type commandRef struct {
	File string
	Line int
	Text string
}

type commandResolution uint8

const (
	resolutionValid commandResolution = iota
	resolutionInvalid
	resolutionSkip
)

func main() {
	rootPath, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(run(rootPath, app.NewRootCommand(), os.Stdout, os.Stderr))
}

func run(rootPath string, root *cobra.Command, stdout, stderr io.Writer) int {
	refs, err := extractReferences(filepath.Join(rootPath, "skills"))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	root.InitDefaultHelpCmd()
	var failures []string
	checked := map[string]bool{}
	for _, ref := range refs {
		path, _, skip := parseReference(ref.Text)
		if skip || path == "" || antiCommands[path] {
			continue
		}
		if checked[path] {
			continue
		}

		switch resolveCommandReference(root, path) {
		case resolutionSkip:
			continue
		case resolutionInvalid:
			failures = append(failures, formatFailure(rootPath, ref, "command path does not exist"))
			continue
		case resolutionValid:
			checked[path] = true
		}
	}

	sort.Strings(failures)
	if len(failures) > 0 {
		fmt.Fprintf(stderr, "skill command integrity check failed (%d references):\n", len(failures))
		for _, failure := range failures {
			fmt.Fprintf(stderr, "  - %s\n", failure)
		}
		return 1
	}
	fmt.Fprintf(stdout, "skill command integrity check: ok (%d executable command paths)\n", len(checked))
	return 0
}

func extractReferences(root string) ([]commandRef, error) {
	var refs []commandRef
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
		lineNumber := 0
		inFence := false
		for scanner.Scan() {
			lineNumber++
			line := scanner.Text()
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				inFence = !inFence
				continue
			}
			if isAntiPatternLine(line) {
				continue
			}
			seen := map[string]bool{}
			for _, match := range inlineCommand.FindAllStringSubmatch(line, -1) {
				command := strings.TrimSpace(match[1])
				seen[command] = true
				refs = append(refs, commandRef{File: path, Line: lineNumber, Text: command})
			}
			if inFence {
				match := lineCommand.FindStringSubmatch(line)
				if len(match) != 2 {
					continue
				}
				command := strings.TrimSpace(match[1])
				if !seen[command] {
					refs = append(refs, commandRef{File: path, Line: lineNumber, Text: command})
				}
			}
		}
		return scanner.Err()
	})
	return refs, err
}

func isAntiPatternLine(line string) bool {
	for _, marker := range antiMarkers {
		if strings.Contains(line, marker) {
			return true
		}
	}
	return false
}

func parseReference(raw string) (string, []string, bool) {
	if strings.Contains(raw, "|") || strings.Contains(raw, "$(") || strings.Contains(raw, "&&") || strings.Contains(raw, " & ") {
		return "", nil, true
	}
	if comment := strings.Index(raw, " #"); comment >= 0 {
		raw = raw[:comment]
	}
	raw = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(raw), "\\"))
	if strings.Contains(raw, "[flags]") || strings.Contains(raw, "[command]") || strings.Contains(raw, "...") {
		return "", nil, true
	}
	tokens := shellFields(raw)
	if len(tokens) < 2 || tokens[0] != "dws" {
		return "", nil, true
	}
	var pathTokens []string
	var flags []string
	for i := 1; i < len(tokens); i++ {
		token := tokens[i]
		if strings.HasPrefix(token, "--") {
			name := strings.TrimPrefix(strings.SplitN(token, "=", 2)[0], "--")
			if name != "" {
				flags = append(flags, name)
			}
			if !strings.Contains(token, "=") && i+1 < len(tokens) && !strings.HasPrefix(tokens[i+1], "-") {
				i++
			}
			continue
		}
		if strings.HasPrefix(token, "-") && len(token) == 2 {
			// Shorthand validity is already covered by the help compatibility
			// baseline; command references are keyed by long flags where present.
			if i+1 < len(tokens) && !strings.HasPrefix(tokens[i+1], "-") {
				i++
			}
			continue
		}
		pathTokens = append(pathTokens, token)
	}
	for _, token := range pathTokens {
		if strings.Contains(token, "/") || strings.Contains(token, "*") {
			return "", nil, true
		}
	}
	return "dws " + strings.Join(pathTokens, " "), uniqueSorted(flags), false
}

func resolveCommandReference(root *cobra.Command, path string) commandResolution {
	cmd, remaining, err := root.Find(strings.Fields(strings.TrimPrefix(path, "dws ")))
	if err != nil || cmd == nil {
		return resolutionInvalid
	}
	if len(remaining) == 0 {
		return resolutionValid
	}
	if !cmd.HasSubCommands() {
		// Once a leaf command has been found, remaining tokens are positional
		// arguments. This check intentionally validates command paths only.
		return resolutionValid
	}
	if isPlaceholder(remaining[0]) {
		// Documentation such as `dws <cmd> --help` or
		// `dws sheet <command> --help` describes a command shape rather than an
		// executable command reference.
		return resolutionSkip
	}
	return resolutionInvalid
}

func isPlaceholder(token string) bool {
	if len(token) < 2 {
		return false
	}
	first, last := token[0], token[len(token)-1]
	return (first == '<' && last == '>') || (first == '[' && last == ']')
}

func shellFields(input string) []string {
	var fields []string
	var current strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if current.Len() > 0 {
			fields = append(fields, current.String())
			current.Reset()
		}
	}
	for _, char := range input {
		if escaped {
			current.WriteRune(char)
			escaped = false
			continue
		}
		if char == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			} else {
				current.WriteRune(char)
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			continue
		}
		if char == ' ' || char == '\t' {
			flush()
			continue
		}
		current.WriteRune(char)
	}
	flush()
	return fields
}

func uniqueSorted(values []string) []string {
	set := map[string]bool{}
	for _, value := range values {
		set[value] = true
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func formatFailure(root string, ref commandRef, reason string) string {
	relative, _ := filepath.Rel(root, ref.File)
	return fmt.Sprintf("%s:%d: `%s`: %s", relative, ref.Line, ref.Text, reason)
}
