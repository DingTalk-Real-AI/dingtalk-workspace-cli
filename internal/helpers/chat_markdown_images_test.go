package helpers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func writeMarkdownTestPNG(t *testing.T, path string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	canvas := image.NewRGBA(image.Rect(0, 0, 2, 2))
	canvas.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(file, canvas); err != nil {
		t.Fatal(err)
	}
}

// TC-001: Only Goldmark Image nodes are rewritten; source ordering is preserved.
func TestCrossPlatformCoverageShouldPreserveOrderWhenResolveMarkdownLocalImagesGivenMixedNodes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.png")
	writeMarkdownTestPNG(t, path)
	input := "前文\n![一](" + path + ")\n中间\n![二](" + path + ")\n![远程](https://example.com/x.png)\n`![代码](./missing.png)`\n后文"
	var calls atomic.Int32
	result, report, err := ResolveMarkdownLocalImages(t.Context(), input, MarkdownImageOptions{
		Upload: func(context.Context, *personalEmotionImage) (string, error) {
			calls.Add(1)
			return "@media-1", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || report.LocalCount != 2 || report.UniqueCount != 1 {
		t.Fatalf("calls=%d report=%+v", calls.Load(), report)
	}
	want := strings.ReplaceAll(input, path, "@media-1")
	if result != want {
		t.Fatalf("result mismatch:\n%s\nwant:\n%s", result, want)
	}
}

// TC-002: Dry-run neither reads local files nor uploads them.
func TestCrossPlatformCoverageShouldNotUploadWhenResolveMarkdownLocalImagesGivenDryRun(t *testing.T) {
	input := "![图](./does-not-exist.png)"
	result, report, err := ResolveMarkdownLocalImages(t.Context(), input, MarkdownImageOptions{
		DryRun: true,
		Upload: func(context.Context, *personalEmotionImage) (string, error) {
			t.Fatal("upload must not run")
			return "", nil
		},
	})
	if err != nil || result != input || report.LocalCount != 1 {
		t.Fatalf("result=%q report=%+v err=%v", result, report, err)
	}
}

// TC-003: An invalid later image stops all uploads.
func TestCrossPlatformCoverageShouldNotUploadWhenResolveMarkdownLocalImagesGivenInvalidLaterFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "good.png")
	writeMarkdownTestPNG(t, path)
	_, _, err := ResolveMarkdownLocalImages(t.Context(), "![一]("+path+") ![二](./missing.png)", MarkdownImageOptions{
		Upload: func(context.Context, *personalEmotionImage) (string, error) {
			t.Fatal("upload must not run")
			return "", nil
		},
	})
	if err == nil {
		t.Fatal("expected file validation error")
	}
}

// TC-004: Existing media IDs are reused and non-image Markdown is untouched.
func TestCrossPlatformCoverageShouldReplaceOnlyImagesWhenResolveMarkdownLocalImagesGivenMediaID(t *testing.T) {
	input := "[链接](./a.png) ![图](mediaid:@existing) ![已有](@direct) \\![转义](./a.png)"
	got, report, err := ResolveMarkdownLocalImages(t.Context(), input, MarkdownImageOptions{})
	if err != nil || report.LocalCount != 0 {
		t.Fatalf("report=%+v err=%v", report, err)
	}
	if got != "[链接](./a.png) ![图](@existing) ![已有](@direct) \\![转义](./a.png)" {
		t.Fatalf("unexpected result: %q", got)
	}
}

// TC-005: Unknown protocols and unsafe local paths are rejected before upload.
func TestCrossPlatformCoverageShouldRejectWhenResolveMarkdownLocalImagesGivenUnsafeDestination(t *testing.T) {
	for _, body := range []string{"![图](ftp://example/a.png)", "![图](file://remote/a.png)"} {
		if _, _, err := ResolveMarkdownLocalImages(t.Context(), body, MarkdownImageOptions{}); err == nil {
			t.Fatalf("expected error for %q", body)
		}
	}
	if _, _, err := ResolveMarkdownLocalImages(t.Context(), "![图](../outside.png)", MarkdownImageOptions{
		SafePath: func(string) (string, error) { return "", os.ErrPermission },
	}); err == nil {
		t.Fatal("expected safe-path error")
	}
}

// TC-006: File URI and percent-encoded spaces point to one local image.
func TestCrossPlatformCoverageShouldDecodeWhenResolveMarkdownLocalImagesGivenFileURI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "with space.png")
	writeMarkdownTestPNG(t, path)
	uri := markdownTestFileURI(path)
	var calls atomic.Int32
	result, report, err := ResolveMarkdownLocalImages(t.Context(), "![一]("+uri+")", MarkdownImageOptions{
		Upload: func(context.Context, *personalEmotionImage) (string, error) {
			calls.Add(1)
			return "@file-uri", nil
		},
	})
	if err != nil || result != "![一](@file-uri)" || report.LocalCount != 1 || calls.Load() != 1 {
		t.Fatalf("result=%q report=%+v calls=%d err=%v", result, report, calls.Load(), err)
	}
}

// TC-006b: file:// paths are decoded exactly once, including literal percent sequences.
func TestCrossPlatformCoverageShouldDecodeOnceWhenResolveMarkdownLocalImagesGivenEncodedPercent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "literal%20name.png")
	writeMarkdownTestPNG(t, path)
	uri := markdownTestFileURI(path)
	result, _, err := ResolveMarkdownLocalImages(t.Context(), "![图]("+uri+")", MarkdownImageOptions{
		Upload: func(context.Context, *personalEmotionImage) (string, error) { return "@once", nil },
	})
	if err != nil || result != "![图](@once)" {
		t.Fatalf("result=%q err=%v", result, err)
	}
}

func TestCrossPlatformCoverageDecodeOnceWhenResolveMarkdownLocalImagesGivenRelativeEncodedPercent(t *testing.T) {
	t.Chdir(t.TempDir())
	writeMarkdownTestPNG(t, "literal%20name.png")
	result, _, err := ResolveMarkdownLocalImages(t.Context(), "![图](./literal%2520name.png)", MarkdownImageOptions{
		Upload: func(context.Context, *personalEmotionImage) (string, error) { return "@once", nil },
	})
	if err != nil || result != "![图](@once)" {
		t.Fatalf("result=%q err=%v", result, err)
	}
}

// TC-007: Distinct paths with identical bytes share one uploaded media object.
func TestCrossPlatformCoverageShouldDedupeContentWhenResolveMarkdownLocalImagesGivenCopiedFiles(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.png")
	second := filepath.Join(dir, "b.png")
	writeMarkdownTestPNG(t, first)
	data, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	_, report, err := ResolveMarkdownLocalImages(t.Context(), "![一]("+first+") ![二]("+second+")", MarkdownImageOptions{
		Upload: func(context.Context, *personalEmotionImage) (string, error) {
			calls.Add(1)
			return "@same", nil
		},
	})
	if err != nil || report.LocalCount != 2 || report.UniqueCount != 1 || calls.Load() != 1 {
		t.Fatalf("report=%+v calls=%d err=%v", report, calls.Load(), err)
	}
}

// TC-008: Upload fan-out never exceeds three in-flight requests.
func TestCrossPlatformCoverageShouldLimitConcurrencyWhenUploadMarkdownImagesGivenManyImages(t *testing.T) {
	images := make([]*personalEmotionImage, 8)
	for i := range images {
		images[i] = &personalEmotionImage{content: string(rune('a' + i))}
	}
	var active atomic.Int32
	var peak atomic.Int32
	ids, err := uploadMarkdownImages(t.Context(), images, func(_ context.Context, image *personalEmotionImage) (string, error) {
		current := active.Add(1)
		for old := peak.Load(); current > old && !peak.CompareAndSwap(old, current); old = peak.Load() {
		}
		time.Sleep(5 * time.Millisecond)
		active.Add(-1)
		return image.content, nil
	})
	if err != nil || len(ids) != len(images) || peak.Load() > 3 || peak.Load() < 2 {
		t.Fatalf("ids=%v peak=%d err=%v", ids, peak.Load(), err)
	}
}

// TC-009: A supported suffix is insufficient when the bytes are not an image.
func TestCrossPlatformCoverageShouldRejectWhenLoadChatUploadImageFileGivenCorruptBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.png")
	if err := os.WriteFile(path, []byte("not a PNG"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadChatUploadImageFile(path); err == nil {
		t.Fatal("expected corrupt image error")
	}
}

// TC-010: Reference definitions cannot be safely rewritten when shared with links.
func TestCrossPlatformCoverageShouldRejectWhenResolveMarkdownLocalImagesGivenReferenceImage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reference.png")
	writeMarkdownTestPNG(t, path)
	input := "![图][asset] [链接][asset]\n\n[asset]: " + path
	_, _, err := ResolveMarkdownLocalImages(t.Context(), input, MarkdownImageOptions{
		Upload: func(context.Context, *personalEmotionImage) (string, error) {
			t.Fatal("upload must not run")
			return "", nil
		},
	})
	if err == nil || strings.Contains(err.Error(), path) {
		t.Fatalf("expected redacted validation error, got %v", err)
	}
}

// TC-011: Collapsed and shortcut local image references fail before upload.
func TestCrossPlatformCoverageShouldRejectWhenResolveMarkdownLocalImagesGivenShortReferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reference.png")
	writeMarkdownTestPNG(t, path)
	input := "![asset][] ![asset]\n\n[asset]: " + path
	_, _, err := ResolveMarkdownLocalImages(t.Context(), input, MarkdownImageOptions{
		Upload: func(context.Context, *personalEmotionImage) (string, error) {
			t.Fatal("upload must not run")
			return "", nil
		},
	})
	if err == nil || strings.Contains(err.Error(), path) {
		t.Fatalf("expected redacted validation error, got %v", err)
	}
}

// TC-012: Signature checks cover every supported extension and mismatch.
func TestCrossPlatformCoverageShouldMatchWhenChatImageSignatureMatchesGivenSupportedFormats(t *testing.T) {
	for _, tc := range []struct {
		kind string
		data []byte
	}{
		{"jpg", []byte{0xff, 0xd8, 0xff}},
		{"jpeg", []byte{0xff, 0xd8, 0xff}},
		{"png", []byte("\x89PNG\r\n\x1a\n")},
		{"gif", []byte("GIF89a")},
		{"webp", []byte("RIFFxxxxWEBP")},
		{"bmp", []byte("BM")},
	} {
		if !chatImageSignatureMatches(tc.data, tc.kind) {
			t.Errorf("expected %s signature to match", tc.kind)
		}
		if chatImageSignatureMatches([]byte("wrong"), tc.kind) {
			t.Errorf("expected %s mismatch", tc.kind)
		}
	}
	if chatImageSignatureMatches([]byte("any"), "tiff") {
		t.Fatal("unsupported format matched")
	}
}

// TC-013: Inline span location preserves nested and angle-bracket syntax.
func TestCrossPlatformCoverageShouldLocateWhenMarkdownImageDestinationSpanGivenValidSyntax(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"![图](./a.png)", "./a.png"},
		{"![图](<./a b.png>)", "./a b.png"},
		{"![图](./a(b).png \"标题\")", "./a(b).png"},
		{"![a[b]c](./a.png)", "./a.png"},
		{"![图](  ./a.png)", "./a.png"},
		{`![图](./a\)b.png)`, `./a\)b.png`},
	} {
		start, end, err := markdownImageDestinationSpan([]byte(tc.input), 0)
		if err != nil || tc.input[start:end] != tc.want {
			t.Fatalf("input=%q span=%q err=%v", tc.input, tc.input[start:end], err)
		}
	}
	for _, input := range []string{"plain", "![a]", "![a]("} {
		if _, _, err := markdownImageDestinationSpan([]byte(input), 0); err == nil {
			t.Fatalf("expected invalid span for %q", input)
		}
	}
}

func TestCrossPlatformCoverageMarkdownImageSpanWhitespaceAndEscapes(t *testing.T) {
	for _, raw := range []string{"![图](  ./a.png)", `![图](./a\)b.png)`} {
		if _, _, err := markdownImageDestinationSpan([]byte(raw), 0); err != nil {
			t.Fatalf("input=%q err=%v", raw, err)
		}
	}
}

func TestCrossPlatformCoverageMarkdownImageDestinationParseErrors(t *testing.T) {
	for _, raw := range []string{"file:", "%"} {
		if _, _, err := classifyMarkdownImageDestination(raw); err == nil {
			t.Fatalf("input=%q unexpectedly accepted", raw)
		}
	}
	for _, raw := range []string{"./a#b.png", "./a?b.png"} {
		kind, path, err := classifyMarkdownImageDestination(raw)
		if err != nil || kind != "local" || path != raw {
			t.Fatalf("input=%q kind=%q path=%q err=%v", raw, kind, path, err)
		}
	}
}

func TestCrossPlatformCoverageResolveLiteralQueryAndFragmentFilenames(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, name := range []string{"a#b.png", "a?b.png"} {
		if runtime.GOOS == "windows" && strings.Contains(name, "?") {
			// Windows 不允许问号文件名；仍验证分类器保留字面值。
			kind, path, err := classifyMarkdownImageDestination("./" + name)
			if err != nil || kind != "local" || path != "./"+name {
				t.Fatalf("字面问号未保留: %q %v", path, err)
			}
			continue
		}
		writeMarkdownTestPNG(t, name)
		got, _, err := ResolveMarkdownLocalImages(t.Context(), "![图](./"+name+")", MarkdownImageOptions{
			Upload: func(context.Context, *personalEmotionImage) (string, error) { return "@literal", nil },
		})
		if err != nil || got != "![图](@literal)" {
			t.Fatalf("name=%q result=%q err=%v", name, got, err)
		}
	}
}

func TestCrossPlatformCoverageRejectWhenResolveMarkdownLocalImagesGivenTooManyImages(t *testing.T) {
	body := strings.Repeat("![图](./a.png) ", 10)
	if _, report, err := ResolveMarkdownLocalImages(t.Context(), body, MarkdownImageOptions{}); err == nil || report.LocalCount != 10 {
		t.Fatalf("report=%+v err=%v", report, err)
	}
}

func TestCrossPlatformCoverageUploadWhenResolveMarkdownLocalImagesGivenNilContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.png")
	writeMarkdownTestPNG(t, path)
	_, report, err := ResolveMarkdownLocalImages(nil, "![图]("+path+")", MarkdownImageOptions{
		Upload: func(context.Context, *personalEmotionImage) (string, error) { return "@uploaded", nil },
	})
	if err != nil || report.LocalCount != 1 {
		t.Fatalf("report=%+v err=%v", report, err)
	}
}

func TestCrossPlatformCoverageRejectWhenUploadMarkdownImagesGivenCancelledContextOrEmptyID(t *testing.T) {
	images := []*personalEmotionImage{{content: "a"}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := uploadMarkdownImages(ctx, images, func(context.Context, *personalEmotionImage) (string, error) {
		t.Fatal("upload must not run")
		return "", nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled upload err=%v", err)
	}
	if _, err := uploadMarkdownImages(t.Context(), images, func(context.Context, *personalEmotionImage) (string, error) {
		return " ", nil
	}); err == nil || !strings.Contains(err.Error(), "mediaId") {
		t.Fatalf("empty upload ID err=%v", err)
	}
}

func TestCrossPlatformCoverageSkipMediaWhenPrepareMarkdownImagesGivenMixedReferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.png")
	writeMarkdownTestPNG(t, path)
	images, paths, err := prepareMarkdownImages([]markdownImageRef{{mediaID: "@existing"}, {path: path}})
	if err != nil || len(images) != 1 || paths[path] != 0 {
		t.Fatalf("images=%d paths=%v err=%v", len(images), paths, err)
	}
}

func TestCrossPlatformCoverageClassifyWhenCollectMarkdownImageRefsGivenNoImage(t *testing.T) {
	refs, report, err := collectMarkdownImageRefs("plain Markdown", nil)
	if err != nil || len(refs) != 0 || report.ImageCount != 0 {
		t.Fatalf("refs=%v report=%+v err=%v", refs, report, err)
	}
}

func TestCrossPlatformCoverageCountCompressionWhenResolveMarkdownLocalImagesGivenLargePNG(t *testing.T) {
	path, _ := writeLargePersonalEmotionPNG(t)
	_, report, err := ResolveMarkdownLocalImages(t.Context(), "![图]("+path+")", MarkdownImageOptions{
		Upload: func(context.Context, *personalEmotionImage) (string, error) { return "@compressed", nil },
	})
	if err != nil || report.CompressedCount != 1 {
		t.Fatalf("report=%+v err=%v", report, err)
	}
}

func TestCrossPlatformCoverageRejectWhenLoadChatUploadImageFileGivenInvalidPixels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.png")
	if err := os.WriteFile(path, []byte("\x89PNG\r\n\x1a\ninvalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadChatUploadImageFile(path); err == nil || !strings.Contains(err.Error(), "尺寸") {
		t.Fatalf("err=%v", err)
	}
}

func TestCrossPlatformCoverageRejectWhenValidateChatUploadImageContentGivenInvalidBase64(t *testing.T) {
	if err := validateChatUploadImageContent(&personalEmotionImage{content: "not-base64%", imageType: "png"}); err == nil || !strings.Contains(err.Error(), "编码") {
		t.Fatalf("err=%v", err)
	}
}

func TestCrossPlatformCoverageRejectWhenCollectMarkdownImageRefsGivenMissingSpan(t *testing.T) {
	testseam.Swap(t, &markdownImageSpan, func([]byte, int) (int, int, error) { return 0, 0, os.ErrInvalid })
	_, _, err := collectMarkdownImageRefs("![图](./a.png)", nil)
	if !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("err=%v", err)
	}
}

func TestCrossPlatformCoverageRejectWhenCollectMarkdownImageRefsGivenAbsolutePathFailure(t *testing.T) {
	testseam.Swap(t, &markdownImageAbsPath, func(string) (string, error) { return "", os.ErrNotExist })
	_, _, got := collectMarkdownImageRefs("![图](./a.png)", nil)
	if got == nil || !strings.Contains(got.Error(), "无法解析") {
		t.Fatalf("err=%v", got)
	}
}

// TC-014: Classification rejects empty IDs and unknown schemes without file I/O.
func TestCrossPlatformCoverageShouldClassifyWhenClassifyMarkdownImageDestinationGivenBoundaryInputs(t *testing.T) {
	for _, destination := range []string{"", "mediaid:", "ftp://example/a.png", "file://remote/a.png", "file:", "%"} {
		if _, _, err := classifyMarkdownImageDestination(destination); err == nil {
			t.Errorf("expected error for %q", destination)
		}
	}
	for _, destination := range []string{"http://example/a.png", "https://example/a.png"} {
		kind, _, err := classifyMarkdownImageDestination(destination)
		if err != nil || kind != "remote" {
			t.Errorf("destination=%q kind=%q err=%v", destination, kind, err)
		}
	}
}

// TC-015: Escaped brackets do not terminate the image label early.
func TestCrossPlatformCoverageShouldLocateWhenMarkdownImageLabelEndGivenEscapedAndInvalidLabels(t *testing.T) {
	input := []byte(`![a\]b](./image.png)`)
	end, err := markdownImageLabelEnd(input, 0)
	if err != nil || input[end] != '(' {
		t.Fatalf("end=%d err=%v", end, err)
	}
	for _, raw := range []string{"![unfinished", "![nested[unfinished]"} {
		if _, err := markdownImageLabelEnd([]byte(raw), 0); err == nil {
			t.Errorf("expected invalid label for %q", raw)
		}
	}
}

func TestCrossPlatformCoverageMarkdownImageReadDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cause error
		want  string
	}{
		{"missing", os.ErrNotExist, "文件不存在"},
		{"permission", os.ErrPermission, "无读取权限"},
		{"other_io", os.ErrInvalid, "无法读取本地图片"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			secret := filepath.Join(t.TempDir(), "confidential-photo.png")
			testseam.Swap(t, &personalEmotionOSStat, func(path string) (os.FileInfo, error) {
				if path != secret {
					t.Fatalf("read path=%q want=%q", path, secret)
				}
				return nil, &os.PathError{Op: "stat", Path: secret, Err: tc.cause}
			})
			_, _, err := ResolveMarkdownLocalImages(t.Context(), "![remote](https://example.com/a.png) ![local]("+secret+")", MarkdownImageOptions{
				Upload: func(context.Context, *personalEmotionImage) (string, error) {
					t.Fatal("unexpected upload")
					return "", nil
				},
			})
			if err == nil {
				t.Fatal("expected failure")
			}
			msg := err.Error()
			for _, want := range []string{tc.want} {
				if !strings.Contains(msg, want) {
					t.Errorf("missing %q: %s", want, msg)
				}
			}
			if strings.Contains(msg, secret) || strings.Contains(msg, filepath.Base(secret)) {
				t.Fatalf("leaked path: %s", msg)
			}
		})
	}
}

func TestCrossPlatformCoverageMarkdownImageContentDiagnostics(t *testing.T) {
	good := filepath.Join(t.TempDir(), "good.png")
	writeMarkdownTestPNG(t, good)
	missing := filepath.Join(t.TempDir(), "secret-missing.png")
	_, _, err := ResolveMarkdownLocalImages(t.Context(), "![one]("+good+") ![duplicate]("+good+") ![missing]("+missing+")", MarkdownImageOptions{})
	if err == nil || !strings.Contains(err.Error(), "文件不存在") || strings.Contains(err.Error(), "secret-missing") {
		t.Fatalf("err=%v", err)
	}
	directory := t.TempDir()
	_, _, err = ResolveMarkdownLocalImages(t.Context(), "![directory]("+directory+")", MarkdownImageOptions{})
	if err == nil || !strings.Contains(err.Error(), "图片校验失败") || strings.Contains(err.Error(), directory) {
		t.Fatalf("err=%v", err)
	}
}

func TestCrossPlatformCoverageMarkdownImagePathPolicyDiagnostics(t *testing.T) {
	for _, tc := range []struct{ name, path, want string }{
		{"absolute", filepath.Join(t.TempDir(), "private.png"), "需要相对路径"},
		{"escape", "../private.png", "路径校验失败"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := ResolveMarkdownLocalImages(t.Context(), "![private]("+tc.path+")", MarkdownImageOptions{SafePath: apperrors.SafeInputPath})
			if err == nil {
				t.Fatal("expected policy failure")
			}
			msg := err.Error()
			for _, want := range []string{tc.want, "DWS 进程工作目录"} {
				if !strings.Contains(msg, want) {
					t.Errorf("missing %q: %s", want, msg)
				}
			}
			if strings.Contains(msg, tc.path) || strings.Contains(msg, "private.png") {
				t.Fatalf("leaked path: %s", msg)
			}
		})
	}
}

func TestCrossPlatformCoverageMarkdownImagePrivateErrors(t *testing.T) {
	secret := "https://private.example/image?token=secret-value"
	for _, cause := range []error{errors.New(secret), &os.PathError{Op: "read", Path: secret, Err: os.ErrPermission}, &os.PathError{Op: "stat", Path: secret, Err: os.ErrInvalid}} {
		msg := markdownImageReadError(cause).Error()
		for _, forbidden := range []string{"private.example", "secret-value", "https://", "脱敏", "第 1 张"} {
			if strings.Contains(msg, forbidden) {
				t.Fatalf("unexpected diagnostic content: %s", msg)
			}
		}
	}
	path := filepath.Join(t.TempDir(), "private.png")
	writeMarkdownTestPNG(t, path)
	_, _, err := ResolveMarkdownLocalImages(t.Context(), "![image]("+path+")", MarkdownImageOptions{
		Upload: func(context.Context, *personalEmotionImage) (string, error) { return "", errors.New(secret) },
	})
	if err == nil || !strings.Contains(err.Error(), "上传失败") || strings.Contains(err.Error(), "private.example") || strings.Contains(err.Error(), "secret-value") || strings.Contains(err.Error(), "脱敏") {
		t.Fatalf("err=%v", err)
	}
}

func TestCrossPlatformCoverageMarkdownImageUploadBusinessClassification(t *testing.T) {
	secret := "https://private.example/upload?token=secret-value"
	source := &apperrors.Error{Category: apperrors.CategoryAPI, Message: secret, Reason: "business_error", RetryableSet: true}
	source.Hint = secret
	source.Cause = errors.New(secret)
	source.Details = map[string]any{"url": secret}
	source.RPCData = []byte(`{"url":"` + secret + `"}`)
	source.ServerDiag = apperrors.ServerDiagnostics{ServerErrorCode: "PARAM_ERROR", ActionURL: secret, TechnicalDetail: secret, TraceID: secret}
	path := filepath.Join(t.TempDir(), "private.png")
	writeMarkdownTestPNG(t, path)
	_, _, err := ResolveMarkdownLocalImages(t.Context(), "![image]("+path+")", MarkdownImageOptions{
		Upload: func(context.Context, *personalEmotionImage) (string, error) {
			return "", fmt.Errorf("wrapped %s: %w", secret, source)
		},
	})
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Category != apperrors.CategoryAPI || typed.Reason != "business_error" || typed.ServerDiag.ServerErrorCode != "PARAM_ERROR" || apperrors.ExitCode(err) != 1 {
		t.Fatalf("classification lost: %#v", err)
	}
	var wire bytes.Buffer
	if err := apperrors.PrintJSON(&wire, err); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"https://", "private.example", "secret-value", "脱敏"} {
		if strings.Contains(wire.String(), token) {
			t.Fatalf("private data in output: %s", wire.String())
		}
	}
	if source.Message != secret || source.Cause == nil {
		t.Fatal("source error mutated")
	}
}

func TestCrossPlatformCoverageMarkdownImageUploadUntrustedClassification(t *testing.T) {
	source := &apperrors.Error{Category: apperrors.CategoryAuth, Message: "private", Reason: "https://private.example"}
	source.ServerDiag.ServerErrorCode = "SECRET_TOKEN_VALUE"
	source.RPCCode = -32000
	result := markdownImageUploadError(source)
	var typed *apperrors.Error
	if !errors.As(result, &typed) || typed.Category != apperrors.CategoryAuth || typed.Reason != "image_upload_failed" || typed.ServerDiag.ServerErrorCode != "" || typed.RPCCode != -32000 {
		t.Fatalf("err=%#v", result)
	}
}

func TestCrossPlatformCoverageMarkdownWindowsDrivePaths(t *testing.T) {
	for _, tc := range []struct {
		input, want string
		bad         bool
	}{
		{`C:\images\a.png`, `C:\images\a.png`, false},
		{`d:/images/a%20b.png`, `d:/images/a b.png`, false},
		{`file:///C:/images/a%20b.png`, `C:/images/a b.png`, false},
		{`file://localhost/d:/images/a%2520b.png`, `d:/images/a%20b.png`, false},
		{`C:/images/bad%zz.png`, "", true},
	} {
		kind, path, err := classifyMarkdownImageDestination(tc.input)
		if tc.bad {
			if err == nil {
				t.Fatalf("无效转义被接受: %q", tc.input)
			}
			continue
		}
		if err != nil || kind != "local" || path != filepath.FromSlash(tc.want) {
			t.Fatalf("Windows 路径分类失败: %q kind=%q path=%q err=%v", tc.input, kind, path, err)
		}
	}
}

func markdownTestFileURI(path string) string {
	path = filepath.ToSlash(filepath.Clean(path))
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func TestCrossPlatformCoverageResolveWindowsDriveMarkdownImage(t *testing.T) {
	local := filepath.Join(t.TempDir(), "image.png")
	writeMarkdownTestPNG(t, local)
	for _, dest := range []string{`C:\images\a.png`, `file:///C:/images/a.png`} {
		seen := ""
		got, report, err := ResolveMarkdownLocalImages(t.Context(), "![图]("+dest+")", MarkdownImageOptions{
			SafePath: func(path string) (string, error) { seen = path; return local, nil },
			Upload:   func(context.Context, *personalEmotionImage) (string, error) { return "@windows", nil },
		})
		if err != nil || got != "![图](@windows)" || report.LocalCount != 1 || seen != filepath.FromSlash(`C:/images/a.png`) && seen != `C:\images\a.png` {
			t.Fatalf("盘符未贯穿 Markdown 解析: dest=%q seen=%q got=%q err=%v", dest, seen, got, err)
		}
	}
}
