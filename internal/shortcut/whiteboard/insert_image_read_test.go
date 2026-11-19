// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package whiteboard

import (
	"io"
	"strings"
	"testing"
	"testing/iotest"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageImageReadRejectsIOFailureAndChangingLength(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reader io.Reader
	}{
		{"read-error", iotest.ErrReader(io.ErrUnexpectedEOF)},
		{"file-shortened", strings.NewReader("abc")},
		{"file-grew", strings.NewReader("abcde")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := inspectImageContents("image.png", tc.reader, 4)
			if result != nil || err == nil || apperrors.ExitCode(err) != 3 {
				t.Fatalf("unreliable image input was not rejected as validation: result=%#v error=%v", result, err)
			}
		})
	}
}
