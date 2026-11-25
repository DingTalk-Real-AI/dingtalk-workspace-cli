// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package render

import (
	"errors"
	"math"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/whiteboard/opennodes"
)

type previewTextRun struct {
	text     string
	fontSize float64
}
type previewTextLine struct {
	runs                         []previewTextRun
	fontSize, lineHeight, height float64
}

func textFallbackFont(current *item) float64 {
	if style, ok := current.node["style"].(map[string]any); ok {
		if size, valid := positiveNumber(style["fontSize"]); valid && size <= 256 {
			return size
		}
	}
	return 14
}

// Text is still a semantic preview: browser font metrics and automatic wrapping
// remain approximate. Block multipliers, mixed font sizes and empty lines are
// retained instead of flattening the source into strings at a fixed 1.25 ratio.
func layoutText(value any, fallbackFont float64) ([]previewTextLine, float64, error) {
	var lines []previewTextLine
	switch text := value.(type) {
	case string:
		for _, line := range splitLines(text) {
			lines = append(lines, previewTextLine{runs: []previewTextRun{{text: line, fontSize: fallbackFont}}, fontSize: fallbackFont, lineHeight: opennodes.DefaultLineHeight})
		}
	case map[string]any:
		if nested, ok := text["text"]; ok {
			return layoutText(nested, fallbackFont)
		}
		blocks, _ := text["blocks"].([]any)
		for _, raw := range blocks {
			block, ok := raw.(map[string]any)
			if !ok {
				return nil, 0, errors.New("invalid text block in SVG layout")
			}
			line := previewTextLine{lineHeight: opennodes.DefaultLineHeight}
			if value, present := block["lineHeight"]; present {
				height, valid := finiteNumber(value)
				if !valid || height < 1 {
					return nil, 0, errors.New("invalid lineHeight in SVG layout")
				}
				line.lineHeight = height
			}
			runs, _ := block["runs"].([]any)
			for _, rawRun := range runs {
				run, ok := rawRun.(map[string]any)
				if !ok {
					return nil, 0, errors.New("invalid text run in SVG layout")
				}
				content, _ := run["text"].(string)
				size := fallbackFont
				if marks, ok := run["marks"].(map[string]any); ok {
					if value, present := marks["fontSize"]; present {
						var valid bool
						size, valid = positiveNumber(value)
						if !valid {
							return nil, 0, errors.New("invalid fontSize in SVG layout")
						}
					}
				}
				line.runs = append(line.runs, previewTextRun{text: content, fontSize: size})
				line.fontSize = math.Max(line.fontSize, size)
			}
			if len(line.runs) == 0 {
				line.runs = []previewTextRun{{fontSize: fallbackFont}}
				line.fontSize = fallbackFont
			}
			lines = append(lines, line)
		}
	}
	total := 0.0
	for index := range lines {
		lines[index].height = lines[index].fontSize * lines[index].lineHeight
		total += lines[index].height
		if !isFinite(total) {
			return nil, 0, errors.New("text line height overflows SVG layout")
		}
	}
	return lines, total, nil
}

func textValueAndCenter(current *item) (any, float64) {
	if current.nodeType == "frame" {
		return current.node["title"], current.worldY + 22
	}
	return current.node["text"], current.worldY + current.height/2
}
