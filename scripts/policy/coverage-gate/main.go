// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

// Command coverage-gate enforces non-regressing overall coverage and a strict
// threshold for executable statements changed by the current PR.
package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
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
}

type gateResult struct {
	Overall           float64
	ChangedCoverage   float64
	ChangedStatements int
	Failures          []string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, gitChangedLines))
}

func run(args []string, stdout, stderr io.Writer, changedLoader func(string) (map[string][]lineRange, error)) int {
	var overallPath string
	var diffPaths stringList
	var baseRef string
	var modulePath string
	var baselineOverall float64
	var overallTolerance float64
	var target float64
	var enforceOverall bool
	flags := flag.NewFlagSet("coverage-gate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&overallPath, "overall-profile", "", "coverage profile used for overall coverage")
	flags.Var(&diffPaths, "diff-profile", "coverage profile used for changed-code coverage (repeatable)")
	flags.StringVar(&baseRef, "base-ref", "", "Git merge-base or previous main SHA")
	flags.StringVar(&modulePath, "module", "", "Go module path used to normalize profile filenames")
	flags.Float64Var(&baselineOverall, "baseline-overall", -1, "authoritative overall coverage percentage")
	flags.Float64Var(&overallTolerance, "overall-tolerance", 0.1, "allowed overall coverage measurement variance in percentage points")
	flags.Float64Var(&target, "target", 80, "required changed-code and eventual overall coverage percentage")
	flags.BoolVar(&enforceOverall, "enforce-overall-target", false, "require overall coverage to reach target")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	if overallPath == "" || len(diffPaths) == 0 || baseRef == "" || modulePath == "" || baselineOverall < 0 {
		fmt.Fprintln(stderr, "coverage-gate requires --overall-profile, --diff-profile, --base-ref, --module, and --baseline-overall")
		return 2
	}
	overall, err := readProfiles([]string{overallPath}, modulePath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
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
	result := evaluate(gateInput{
		Overall:          overall,
		Diff:             diff,
		Changed:          changed,
		BaselineOverall:  baselineOverall,
		OverallTolerance: overallTolerance,
		Target:           target,
		EnforceOverall:   enforceOverall,
	})

	mode := "transition: non-regression"
	if enforceOverall {
		mode = "required"
	}
	fmt.Fprintf(stdout, "overall coverage: %.1f%% (merge-base %.1f%%; tolerance %.1fpp; target %.1f%%; %s)\n", result.Overall, baselineOverall, overallTolerance, target, mode)
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
	result.Overall = coveragePercent(input.Overall)
	baselineRounded := roundOne(input.BaselineOverall)
	overallRounded := roundOne(result.Overall)
	if overallRounded+input.OverallTolerance+1e-9 < baselineRounded {
		result.Failures = append(result.Failures, fmt.Sprintf("overall coverage regressed from %.1f%% to %.1f%%", baselineRounded, overallRounded))
	}
	if input.EnforceOverall && overallRounded < input.Target {
		result.Failures = append(result.Failures, fmt.Sprintf("overall coverage %.1f%% is below target %.1f%%", overallRounded, input.Target))
	}

	profiledFiles := map[string]bool{}
	covered, total := 0, 0
	seen := map[string]bool{}
	for _, block := range input.Diff {
		profiledFiles[block.File] = true
		if !intersectsAny(block, input.Changed[block.File]) {
			continue
		}
		key := fmt.Sprintf("%s:%d:%d:%d", block.File, block.StartLine, block.EndLine, block.Statements)
		if seen[key] {
			continue
		}
		seen[key] = true
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
	command := exec.Command("git", "diff", "--unified=0", "--no-color", baseRef+"...HEAD", "--", "*.go")
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
	for _, block := range blocks {
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

func roundOne(value float64) float64 { return math.Round(value*10) / 10 }
