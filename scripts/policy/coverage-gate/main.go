// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

// Command coverage-gate enforces non-regressing overall coverage and a strict
// threshold for executable statements changed by the current PR.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	profileLine = regexp.MustCompile(`^(.+):(\d+)\.(\d+),(\d+)\.(\d+)\s+(\d+)\s+(\d+)$`)
	hunkHeader  = regexp.MustCompile(`^@@\s+-\d+(?:,\d+)?\s+\+(\d+)(?:,(\d+))?\s+@@`)
)

type stringList []string

func (values *stringList) String() string { return strings.Join(*values, ",") }
func (values *stringList) Set(value string) error {
	*values = append(*values, value)
	return nil
}

type coverageBlock struct {
	File       string
	StartLine  int
	EndLine    int
	Statements int
	Count      int
}

type lineRange struct {
	Start int
	End   int
}

type gateInput struct {
	Overall          []coverageBlock
	Diff             []coverageBlock
	Changed          map[string][]lineRange
	BaselineOverall  float64
	OverallTolerance float64
	Target           float64
	EnforceOverall   bool
	ChangedOnly      bool
}

type gateResult struct {
	Overall           float64
	ChangedCoverage   float64
	ChangedStatements int
	Failures          []string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, gitChangedLines, goListBuildableFiles))
}

func run(
	args []string,
	stdout, stderr io.Writer,
	changedLoader func(string) (map[string][]lineRange, error),
	buildableLoader func() (map[string]bool, error),
) int {
	var overallPaths stringList
	var diffPaths stringList
	var baseRef string
	var modulePath string
	var baselineOverall float64
	var overallTolerance float64
	var target float64
	var enforceOverall bool
	var changedOnly bool
	var scopeBuildable bool
	flags := flag.NewFlagSet("coverage-gate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Var(&overallPaths, "overall-profile", "coverage profile used for overall coverage (repeatable)")
	flags.Var(&diffPaths, "diff-profile", "coverage profile used for changed-code coverage (repeatable)")
	flags.StringVar(&baseRef, "base-ref", "", "Git merge-base or previous main SHA")
	flags.StringVar(&modulePath, "module", "", "Go module path used to normalize profile filenames")
	flags.Float64Var(&baselineOverall, "baseline-overall", -1, "authoritative overall coverage percentage")
	flags.Float64Var(&overallTolerance, "overall-tolerance", 0.1, "allowed overall coverage measurement variance in percentage points")
	flags.Float64Var(&target, "target", 80, "required changed-code and eventual overall coverage percentage")
	flags.BoolVar(&enforceOverall, "enforce-overall-target", false, "require overall coverage to reach target")
	flags.BoolVar(&changedOnly, "changed-only", false, "enforce changed-code coverage without an overall baseline")
	flags.BoolVar(&scopeBuildable, "scope-buildable", false, "only evaluate changed files buildable on the current platform")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	if len(diffPaths) == 0 || baseRef == "" || modulePath == "" || (!changedOnly && (len(overallPaths) == 0 || baselineOverall < 0)) {
		fmt.Fprintln(stderr, "coverage-gate requires --diff-profile, --base-ref, and --module; overall mode also requires --overall-profile and --baseline-overall")
		return 2
	}
	var overall []coverageBlock
	if !changedOnly {
		var err error
		overall, err = readProfiles(overallPaths, modulePath)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
	}
	diff, err := readProfiles(diffPaths, modulePath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	changed, err := changedLoader(baseRef)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if scopeBuildable {
		buildable, err := buildableLoader()
		if err != nil {
			fmt.Fprintf(stderr, "resolve buildable Go files: %v\n", err)
			return 2
		}
		changed = filterChangedFiles(changed, buildable)
	}
	result := evaluate(gateInput{
		Overall:          overall,
		Diff:             diff,
		Changed:          changed,
		BaselineOverall:  baselineOverall,
		OverallTolerance: overallTolerance,
		Target:           target,
		EnforceOverall:   enforceOverall,
		ChangedOnly:      changedOnly,
	})

	if !changedOnly {
		mode := "transition: non-regression"
		if enforceOverall {
			mode = "required"
		}
		fmt.Fprintf(stdout, "overall coverage: %.1f%% (merge-base %.1f%%; tolerance %.1fpp; target %.1f%%; %s)\n", result.Overall, baselineOverall, overallTolerance, target, mode)
	}
	if result.ChangedStatements == 0 {
		fmt.Fprintf(stdout, "changed code coverage: n/a (no changed executable statements; target %.1f%%)\n", target)
	} else {
		fmt.Fprintf(stdout, "changed code coverage: %.1f%% (%d executable statements; target %.1f%%)\n", result.ChangedCoverage, result.ChangedStatements, target)
	}
	if len(result.Failures) > 0 {
		fmt.Fprintln(stderr, "coverage gate failed:")
		for _, failure := range result.Failures {
			fmt.Fprintf(stderr, "  - %s\n", failure)
		}
		return 1
	}
	return 0
}

func evaluate(input gateInput) gateResult {
	result := gateResult{Failures: []string{}}
	if !input.ChangedOnly {
		result.Overall = coveragePercent(input.Overall)
		baselineRounded := roundOne(input.BaselineOverall)
		overallRounded := roundOne(result.Overall)
		if overallRounded+input.OverallTolerance+1e-9 < baselineRounded {
			result.Failures = append(result.Failures, fmt.Sprintf("overall coverage regressed from %.1f%% to %.1f%%", baselineRounded, overallRounded))
		}
		if input.EnforceOverall && overallRounded < input.Target {
			result.Failures = append(result.Failures, fmt.Sprintf("overall coverage %.1f%% is below target %.1f%%", overallRounded, input.Target))
		}
	}

	profiledFiles := map[string]bool{}
	covered, total := 0, 0
	for _, block := range mergeCoverageBlocks(input.Diff) {
		profiledFiles[block.File] = true
		if !intersectsAny(block, input.Changed[block.File]) {
			continue
		}
		total += block.Statements
		if block.Count > 0 {
			covered += block.Statements
		}
	}
	var missing []string
	for path := range input.Changed {
		if !profiledFiles[path] {
			missing = append(missing, path)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		result.Failures = append(result.Failures, "changed production Go files missing from coverage profiles: "+strings.Join(missing, ", "))
	}
	result.ChangedStatements = total
	if total > 0 {
		result.ChangedCoverage = float64(covered) * 100 / float64(total)
		if result.ChangedCoverage+1e-9 < input.Target {
			result.Failures = append(result.Failures, fmt.Sprintf("changed code coverage %.1f%% is below target %.1f%%", result.ChangedCoverage, input.Target))
		}
	}
	return result
}

func filterChangedFiles(changed map[string][]lineRange, allowed map[string]bool) map[string][]lineRange {
	filtered := map[string][]lineRange{}
	for path, ranges := range changed {
		if allowed[path] {
			filtered[path] = ranges
		}
	}
	return filtered
}

func readProfiles(paths []string, modulePath string) ([]coverageBlock, error) {
	var blocks []coverageBlock
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open coverage profile %s: %w", path, err)
		}
		scanner := bufio.NewScanner(file)
		line := 0
		for scanner.Scan() {
			line++
			text := strings.TrimSpace(scanner.Text())
			if line == 1 && strings.HasPrefix(text, "mode:") {
				continue
			}
			match := profileLine.FindStringSubmatch(text)
			if len(match) != 8 {
				file.Close()
				return nil, fmt.Errorf("%s:%d: invalid coverage profile line", path, line)
			}
			values := make([]int, 0, 6)
			for _, raw := range match[2:] {
				value, err := strconv.Atoi(raw)
				if err != nil {
					file.Close()
					return nil, fmt.Errorf("%s:%d: parse coverage number: %w", path, line, err)
				}
				values = append(values, value)
			}
			blocks = append(blocks, coverageBlock{
				File:       normalizeProfilePath(match[1], modulePath),
				StartLine:  values[0],
				EndLine:    values[2],
				Statements: values[4],
				Count:      values[5],
			})
		}
		err = scanner.Err()
		file.Close()
		if err != nil {
			return nil, fmt.Errorf("read coverage profile %s: %w", path, err)
		}
	}
	return blocks, nil
}

func gitChangedLines(baseRef string) (map[string][]lineRange, error) {
	command := exec.Command("git", "diff", "--unified=0", "--no-color", baseRef, "--", "*.go")
	output, err := command.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("git diff failed: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("run git diff: %w", err)
	}
	return parseChangedLines(output)
}

func goListBuildableFiles() (map[string]bool, error) {
	rootCommand := exec.Command("git", "rev-parse", "--show-toplevel")
	rootOutput, err := rootCommand.Output()
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	root := strings.TrimSpace(string(rootOutput))

	command := exec.Command("go", "list", "-json", "./...")
	output, err := command.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("go list failed: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("run go list: %w", err)
	}

	buildable := map[string]bool{}
	decoder := json.NewDecoder(bytes.NewReader(output))
	for {
		var packageInfo struct {
			Dir      string
			GoFiles  []string
			CgoFiles []string
		}
		if err := decoder.Decode(&packageInfo); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("decode go list output: %w", err)
		}
		for _, name := range append(packageInfo.GoFiles, packageInfo.CgoFiles...) {
			relative, err := filepath.Rel(root, filepath.Join(packageInfo.Dir, name))
			if err != nil {
				return nil, fmt.Errorf("normalize buildable file %s: %w", name, err)
			}
			relative = filepath.ToSlash(relative)
			if relative != ".." && !strings.HasPrefix(relative, "../") && isProductionGo(relative) {
				buildable[relative] = true
			}
		}
	}
	return buildable, nil
}

func parseChangedLines(diff []byte) (map[string][]lineRange, error) {
	changed := map[string][]lineRange{}
	var path string
	scanner := bufio.NewScanner(bytes.NewReader(diff))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "+++ ") {
			path = strings.TrimPrefix(line, "+++ ")
			path = strings.TrimPrefix(path, "b/")
			if path == "/dev/null" || !isProductionGo(path) {
				path = ""
			}
			continue
		}
		if path == "" || !strings.HasPrefix(line, "@@") {
			continue
		}
		match := hunkHeader.FindStringSubmatch(line)
		if len(match) == 0 {
			return nil, fmt.Errorf("invalid diff hunk header %q", line)
		}
		start, _ := strconv.Atoi(match[1])
		count := 1
		if match[2] != "" {
			count, _ = strconv.Atoi(match[2])
		}
		if count > 0 {
			changed[path] = append(changed[path], lineRange{Start: start, End: start + count - 1})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return changed, nil
}

func isProductionGo(path string) bool {
	return strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") && !strings.HasPrefix(path, "test/")
}

func normalizeProfilePath(path, modulePath string) string {
	return strings.TrimPrefix(path, strings.TrimSuffix(modulePath, "/")+"/")
}

func intersectsAny(block coverageBlock, ranges []lineRange) bool {
	for _, changed := range ranges {
		if block.StartLine <= changed.End && block.EndLine >= changed.Start {
			return true
		}
	}
	return false
}

func coveragePercent(blocks []coverageBlock) float64 {
	covered, total := 0, 0
	for _, block := range mergeCoverageBlocks(blocks) {
		total += block.Statements
		if block.Count > 0 {
			covered += block.Statements
		}
	}
	if total == 0 {
		return 0
	}
	return float64(covered) * 100 / float64(total)
}

// mergeCoverageBlocks unions duplicate blocks emitted by cross-package
// coverage. A block is covered when any instrumented test binary executed it;
// counting the same source block once per binary would both dilute the overall
// percentage and make the result depend on package execution order.
func mergeCoverageBlocks(blocks []coverageBlock) []coverageBlock {
	merged := make(map[string]coverageBlock, len(blocks))
	for _, block := range blocks {
		key := fmt.Sprintf("%s:%d:%d:%d", block.File, block.StartLine, block.EndLine, block.Statements)
		current, ok := merged[key]
		if !ok || block.Count > current.Count {
			merged[key] = block
		}
	}
	keys := make([]string, 0, len(merged))
	for key := range merged {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]coverageBlock, 0, len(keys))
	for _, key := range keys {
		result = append(result, merged[key])
	}
	return result
}

func roundOne(value float64) float64 { return math.Round(value*10) / 10 }
