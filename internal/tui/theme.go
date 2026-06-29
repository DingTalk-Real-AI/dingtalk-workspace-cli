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

package tui

import (
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/fatih/color"
)

const (
	MaxTableColumnWidth = 60
	MaxPanelWidth       = 96
	MinPanelWidth       = 52
)

var (
	brandBlue = color.New(color.FgHiBlue, color.Bold).SprintFunc()
	blue      = color.New(color.FgBlue).SprintFunc()
	cyan      = color.New(color.FgCyan).SprintFunc()
	white     = color.New(color.FgHiWhite).SprintFunc()
	gray      = color.New(color.FgHiBlack).SprintFunc()
	success   = color.New(color.FgGreen, color.Bold).SprintFunc()
	warning   = color.New(color.FgYellow, color.Bold).SprintFunc()
	danger    = color.New(color.FgRed, color.Bold).SprintFunc()
	bold      = color.New(color.Bold).SprintFunc()
	dim       = color.New(color.Faint).SprintFunc()
)

func Brand(v string) string   { return brandBlue(v) }
func Blue(v string) string    { return blue(v) }
func Cyan(v string) string    { return cyan(v) }
func White(v string) string   { return white(v) }
func Gray(v string) string    { return gray(v) }
func Success(v string) string { return success(v) }
func Warning(v string) string { return warning(v) }
func Danger(v string) string  { return danger(v) }
func Bold(v string) string    { return bold(v) }
func Dim(v string) string     { return dim(v) }

func Header(title, subtitle string) string {
	title = strings.TrimSpace(title)
	subtitle = strings.TrimSpace(subtitle)
	if subtitle == "" {
		return fmt.Sprintf("%s %s", Brand("DingTalk"), White(title))
	}
	return fmt.Sprintf("%s %s %s", Brand("DingTalk"), White(title), Dim("· "+subtitle))
}

func Section(title string) string {
	return fmt.Sprintf("%s %s", Blue("▍"), Bold(title))
}

func Key(key string) string {
	return Gray(key + ":")
}

func Bullet() string {
	return Cyan("•")
}

func Arrow() string {
	return Dim("→")
}

func StateMark(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "ok", "success", "pass":
		return Success("●")
	case "warn", "warning", "retryable":
		return Warning("●")
	case "error", "danger", "failed", "fail":
		return Danger("●")
	default:
		return Cyan("●")
	}
}

func Border() string {
	return Blue("─")
}

func SoftBorder() string {
	return Gray("─")
}

func Rule(width int) string {
	if width <= 0 {
		width = MinPanelWidth
	}
	if width > MaxPanelWidth {
		width = MaxPanelWidth
	}
	return Blue(strings.Repeat("─", width))
}

func PlainRuneWidth(s string) int {
	width := 0
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		if isZeroWidthRune(r) {
			continue
		}
		if isWideRune(r) {
			width += 2
			continue
		}
		width++
	}
	return width
}

func isZeroWidthRune(r rune) bool {
	return r == '\u200d' ||
		(r >= 0xFE00 && r <= 0xFE0F) ||
		(r >= 0xE0100 && r <= 0xE01EF) ||
		unicode.Is(unicode.Mn, r) ||
		unicode.Is(unicode.Me, r)
}

func isWideRune(r rune) bool {
	return (r >= 0x1100 && r <= 0x115F) ||
		(r >= 0x2329 && r <= 0x232A) ||
		(r >= 0x2E80 && r <= 0xA4CF) ||
		(r >= 0xAC00 && r <= 0xD7A3) ||
		(r >= 0xF900 && r <= 0xFAFF) ||
		(r >= 0xFE10 && r <= 0xFE19) ||
		(r >= 0xFE30 && r <= 0xFE6F) ||
		(r >= 0xFF00 && r <= 0xFF60) ||
		(r >= 0xFFE0 && r <= 0xFFE6) ||
		(r >= 0x1F1E6 && r <= 0x1F1FF) ||
		(r >= 0x1F300 && r <= 0x1FAFF)
}

func PadRightANSI(s string, width int) string {
	pad := width - PlainRuneWidth(s)
	if pad <= 0 {
		return s
	}
	return s + strings.Repeat(" ", pad)
}

func Panel(w io.Writer, title string, lines []string) error {
	width := MinPanelWidth
	for _, line := range append([]string{title}, lines...) {
		if lineWidth := PlainRuneWidth(line) + 4; lineWidth > width {
			width = lineWidth
		}
	}
	if width > MaxPanelWidth {
		width = MaxPanelWidth
	}

	top := Blue("╭" + strings.Repeat("─", width-2) + "╮")
	mid := Blue("├" + strings.Repeat("─", width-2) + "┤")
	bottom := Blue("╰" + strings.Repeat("─", width-2) + "╯")
	if _, err := fmt.Fprintln(w, top); err != nil {
		return err
	}
	if title != "" {
		content := " " + title
		if _, err := fmt.Fprintf(w, "%s%s%s\n", Blue("│"), PadRightANSI(content, width-2), Blue("│")); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, mid); err != nil {
			return err
		}
	}
	for _, line := range lines {
		content := " " + line
		if _, err := fmt.Fprintf(w, "%s%s%s\n", Blue("│"), PadRightANSI(content, width-2), Blue("│")); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w, bottom)
	return err
}
