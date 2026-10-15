package helpers

import "fmt"

type markdownImageRef struct {
	start, end int
	path       string
	mediaID    string
}

// Goldmark confirms this is an Image node; this scanner only locates its inline destination.
func markdownImageDestinationSpan(source []byte, pos int) (int, int, error) {
	index, err := markdownImageLabelEnd(source, pos)
	if err != nil {
		return 0, 0, err
	}
	if index >= len(source) || source[index] != '(' {
		return 0, 0, fmt.Errorf("无法定位 Markdown 图片地址")
	}
	index++
	for index < len(source) && (source[index] == ' ' || source[index] == '\n') {
		index++
	}
	angle := index < len(source) && source[index] == '<'
	if angle {
		index++
	}
	start := index
	parenDepth := 0
	for ; index < len(source); index++ {
		if source[index] == '\\' {
			index++
			continue
		}
		if angle && source[index] == '>' {
			return start, index, nil
		}
		if !angle && source[index] == '(' {
			parenDepth++
		}
		if !angle && source[index] == ')' {
			if parenDepth == 0 {
				return start, index, nil
			}
			parenDepth--
		}
		if !angle && (source[index] == ' ' || source[index] == '\n') && parenDepth == 0 {
			return start, index, nil
		}
	}
	return 0, 0, fmt.Errorf("无法定位 Markdown 图片地址")
}

func markdownImageLabelEnd(source []byte, pos int) (int, error) {
	if pos < 0 || pos+1 >= len(source) || source[pos] != '!' || source[pos+1] != '[' {
		return 0, fmt.Errorf("无法定位 Markdown 图片")
	}
	depth := 1
	index := pos + 2
	for ; index < len(source) && depth > 0; index++ {
		if source[index] == '\\' {
			index++
			continue
		}
		switch source[index] {
		case '[':
			depth++
		case ']':
			depth--
		}
	}
	if depth != 0 {
		return 0, fmt.Errorf("无法定位 Markdown 图片标签")
	}
	return index, nil
}
