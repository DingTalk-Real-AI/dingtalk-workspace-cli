// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0
package doc

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var docClipboardImage = readDocClipboardImage
var docClipboardLookPath = exec.LookPath
var docClipboardRun = runDocClipboardCommand

type docClipboardFile interface {
	io.WriteCloser
	Name() string
}

var docClipboardCreateTemp = func(dir, pattern string) (docClipboardFile, error) { return os.CreateTemp(dir, pattern) }

const docClipboardLimit = 20 * 1024 * 1024

// Read only on explicit --from-clipboard. Native commands receive no user text.
func readDocClipboardImage(ctx context.Context) ([]byte, error) {
	return readDocClipboardImageForOS(ctx, runtime.GOOS)
}
func readDocClipboardImageForOS(ctx context.Context, platform string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	name := ""
	args := []string{}
	encoding := "raw"
	switch platform {
	case "darwin":
		name = "osascript"
		args = []string{"-e", "get the clipboard as «class PNGf»"}
		encoding = "hex"
	case "windows":
		name = "powershell"
		args = []string{"-NoProfile", "-STA", "-Command", `Add-Type -AssemblyName System.Windows.Forms; $im=[System.Windows.Forms.Clipboard]::GetImage(); if($null -eq $im){exit 2}; $stream=New-Object System.IO.MemoryStream; $im.Save($stream,[System.Drawing.Imaging.ImageFormat]::Png); [Convert]::ToBase64String($stream.ToArray()); $stream.Dispose(); $im.Dispose()`}
		encoding = "base64"
	case "linux":
		if _, err := docClipboardLookPath("wl-paste"); err == nil {
			name = "wl-paste"
			args = []string{"--type", "image/png"}
		} else {
			name = "xclip"
			args = []string{"-selection", "clipboard", "-t", "image/png", "-o"}
		}
	default:
		return nil, apperrors.NewValidation("当前平台没有剪贴板图片读取支持")
	}
	data, runErr := docClipboardRun(ctx, name, args)
	if runErr != nil {
		return nil, apperrors.NewValidation("无法读取剪贴板PNG图片；确认剪贴板包含图片和系统读取工具可用")
	}
	var err error
	switch encoding {
	case "hex":
		s := strings.TrimSpace(string(data))
		const prefix = "«data PNGf"
		if !strings.HasPrefix(s, prefix) || !strings.HasSuffix(s, "»") {
			return nil, apperrors.NewValidation("剪贴板未返回PNG数据")
		}
		data, err = hex.DecodeString(strings.TrimSuffix(strings.TrimPrefix(s, prefix), "»"))
	case "base64":
		data, err = base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	}
	if err != nil || len(data) == 0 || len(data) > docClipboardLimit {
		return nil, apperrors.NewValidation("剪贴板图片无效或超过20MiB")
	}
	if _, err := png.DecodeConfig(bytes.NewReader(data)); err != nil {
		return nil, apperrors.NewValidation("剪贴板内容不是有效PNG")
	}
	return data, nil
}

func runDocClipboardCommand(ctx context.Context, name string, args []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out limitedClipboardBuffer
	cmd.Stdout, cmd.Stderr = &out, io.Discard
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

type limitedClipboardBuffer struct{ bytes.Buffer }

func (b *limitedClipboardBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > docClipboardLimit*2+1024 {
		return 0, fmt.Errorf("clipboard output too large")
	}
	return b.Buffer.Write(p)
}
func validateDocImageInput(rt *shortcut.RuntimeContext) error {
	if rt.Bool("from-clipboard") {
		if rt.Str("file") != "" || rt.StrFirst("image", "url") != "" {
			return apperrors.NewValidation("from-clipboard与file/image互斥")
		}
		return nil
	}
	if rt.Str("file") != "" {
		return validateWorkspaceInputPath("file", rt.Str("file"))
	}
	if rt.StrFirst("image", "url") == "" {
		return apperrors.NewValidation("需要file、from-clipboard或封面image")
	}
	return nil
}
func withDocClipboard(rt *shortcut.RuntimeContext, run func() error) error {
	if !rt.Bool("from-clipboard") {
		return run()
	}
	if rt.DryRun() {
		return rt.Output(docEnvelope("doc.clipboard", map[string]any{"executed": false, "source": "clipboard_png"}))
	}
	data, err := docClipboardImage(rt.Command().Context())
	if err != nil {
		return err
	}
	file, err := docClipboardCreateTemp(".", ".dws-clipboard-*.png")
	if err != nil {
		return err
	}
	path := file.Name()
	defer os.Remove(path)
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	flag := rt.Command().Flags().Lookup("file")
	old, changed := flag.Value.String(), flag.Changed
	defer func() { _ = flag.Value.Set(old); flag.Changed = changed }()
	if err := rt.Command().Flags().Set("file", filepath.Base(path)); err != nil {
		return err
	}
	return run()
}
