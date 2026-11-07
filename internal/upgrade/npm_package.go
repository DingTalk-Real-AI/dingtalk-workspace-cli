// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	npmPackageMaxDownload = 256 << 20
	npmPackageMaxExpanded = 512 << 20
	npmPackageMaxEntry    = 128 << 20
	npmPackageMaxMetadata = 1 << 20
	npmPackageMaxEntries  = 128
)

var npmPackageOpen = os.Open

// PreparedNPMPackage 是完成外层 SRI、身份和内层 SHA256 校验的安装材料。
// 文件位于 destDir 下的私有子目录；调用方统一清理 destDir。
type PreparedNPMPackage struct {
	BinaryArchivePath string
	SkillsArchivePath string
	ChecksumsContent  string
}

// PrepareNPMPackage 直接下载并验证 npm 包，不执行包内脚本，也不依赖 npm/node。
func PrepareNPMPackage(ctx context.Context, pkg NPMPackage, destDir string, skipSkills bool, showProgress func(float64, int64, int64)) (PreparedNPMPackage, error) {
	integrity, err := parseNPMPackageIntegrity(pkg.Integrity)
	if err != nil {
		return PreparedNPMPackage{}, err
	}
	if !validRegistryURL(pkg.TarballURL) {
		return PreparedNPMPackage{}, fmt.Errorf("npm tarball URL 无效或不安全")
	}
	stage, err := os.MkdirTemp(destDir, "npm-package-")
	if err != nil {
		return PreparedNPMPackage{}, fmt.Errorf("创建 npm 校验目录失败: %w", err)
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(stage)
		}
	}()
	archive := filepath.Join(stage, "package.tgz")
	cfg := DefaultDownloadConfig()
	cfg.MaxBytes = npmPackageMaxDownload
	cfg.CheckRedirect = npmPackageRedirect
	cfg.ProgressCallback = func(downloaded, total int64) {
		if showProgress != nil && total > 0 {
			showProgress(float64(downloaded)/float64(total)*100, downloaded, total)
		}
	}
	size, err := DownloadWithConfig(ctx, pkg.TarballURL, archive, cfg)
	if err != nil {
		return PreparedNPMPackage{}, err
	}
	f, err := npmPackageOpen(archive)
	if err != nil {
		return PreparedNPMPackage{}, err
	}
	defer f.Close()
	if err := integrity.verify(npmPackageContextReader{ctx, io.NewSectionReader(f, 0, size)}); err != nil {
		return PreparedNPMPackage{}, err
	}
	prepared, err := extractNPMPackage(ctx, io.NewSectionReader(f, 0, size), pkg, stage, skipSkills, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return PreparedNPMPackage{}, err
	}
	complete = true
	return prepared, nil
}

func npmPackageRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("npm 下载重定向过多")
	}
	if len(via) > 0 && via[len(via)-1].URL.Scheme == "https" && req.URL.Scheme != "https" {
		return fmt.Errorf("npm 下载禁止 HTTPS 降级重定向")
	}
	if !validRegistryURL(req.URL.String()) {
		return fmt.Errorf("npm 下载重定向 URL 无效或不安全")
	}
	return nil
}

type npmPackageIntegrity struct {
	hash     hash.Hash
	expected [][]byte
}

// SRI 多摘要按最强算法匹配，不能用匹配的 SHA256 绕过不匹配的 SHA512。
func parseNPMPackageIntegrity(value string) (npmPackageIntegrity, error) {
	var result npmPackageIntegrity
	strength := 0
	for _, token := range strings.Fields(value) {
		algorithm, encoded, _ := strings.Cut(token, "-")
		var h hash.Hash
		switch algorithm {
		case "sha512":
			h = sha512.New()
		case "sha256":
			h = sha256.New()
		default:
			return result, fmt.Errorf("npm integrity 使用了不支持的算法")
		}
		digest, err := base64.StdEncoding.Strict().DecodeString(encoded)
		if err != nil || len(digest) != h.Size() {
			return result, fmt.Errorf("npm integrity 摘要格式无效")
		}
		if h.Size() > strength {
			result = npmPackageIntegrity{hash: h}
			strength = h.Size()
		}
		if h.Size() == strength {
			result.expected = append(result.expected, digest)
		}
	}
	if result.hash == nil {
		return result, fmt.Errorf("npm 包缺少 dist.integrity，拒绝安装")
	}
	return result, nil
}

func (i npmPackageIntegrity) verify(r io.Reader) error {
	if _, err := io.Copy(i.hash, r); err != nil {
		return fmt.Errorf("读取 npm 包校验失败: %w", err)
	}
	actual := i.hash.Sum(nil)
	for _, expected := range i.expected {
		if subtle.ConstantTimeCompare(actual, expected) == 1 {
			return nil
		}
	}
	return fmt.Errorf("npm 包 integrity 校验失败")
}

type npmPackageContextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r npmPackageContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func npmPackageBinaryName(goos, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return "dws-" + goos + "-" + goarch + ext
}

// 路径仅用于白名单比较，不使用归档成员的路径在磁盘上创建目录。
func validNPMPackagePath(name string) bool {
	return !strings.ContainsAny(name, "\\:\x00\r\n") && path.Clean(name) == name &&
		(name == "package" || strings.HasPrefix(name, "package/"))
}

func extractNPMPackage(ctx context.Context, r io.Reader, pkg NPMPackage, stage string, skipSkills bool, goos, goarch string) (PreparedNPMPackage, error) {
	var result PreparedNPMPackage
	gz, err := gzip.NewReader(npmPackageContextReader{ctx, r})
	if err != nil {
		return result, fmt.Errorf("npm 包不是有效 gzip: %w", err)
	}
	defer gz.Close()
	// 同时限制已选中与未选中成员，防止跳过其他平台时展开压缩炸弹。
	bounded := &io.LimitedReader{R: gz, N: npmPackageMaxExpanded + 1}
	tr := tar.NewReader(bounded)
	binaryName := npmPackageBinaryName(goos, goarch)
	selected := map[string]bool{binaryName: true}
	if !skipSkills {
		selected[skillsZipName] = true
	}
	seen := map[string]bool{}
	digests := map[string]string{}
	var manifest []byte
	for entries := 0; ; entries++ {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return result, fmt.Errorf("读取 npm 归档失败: %w", err)
		}
		name := hdr.Name
		if hdr.Typeflag == tar.TypeDir {
			name = strings.TrimSuffix(name, "/")
		}
		if entries >= npmPackageMaxEntries || hdr.Size > npmPackageMaxEntry || hdr.Size < 0 {
			return result, fmt.Errorf("npm 归档超过成员数量或大小上限")
		}
		if !validNPMPackagePath(name) || seen[strings.ToLower(name)] {
			return result, fmt.Errorf("npm 归档含不安全或重复路径")
		}
		seen[strings.ToLower(name)] = true
		if hdr.Typeflag == tar.TypeDir && hdr.Size == 0 {
			continue
		}
		if (hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA) || npmPackageSparse(hdr.PAXRecords) {
			return result, fmt.Errorf("npm 归档含不允许的成员类型")
		}
		switch name {
		case "package/package.json", "package/assets/checksums.txt":
			if hdr.Size > npmPackageMaxMetadata {
				return result, fmt.Errorf("npm 包元数据超过大小上限")
			}
			data, err := io.ReadAll(tr)
			if err != nil {
				return result, err
			}
			if name == "package/package.json" {
				manifest = data
			} else {
				result.ChecksumsContent = string(data)
			}
		default:
			filename := strings.TrimPrefix(name, "package/assets/")
			if !selected[filename] || name != "package/assets/"+filename {
				continue
			}
			dest := filepath.Join(stage, filename)
			digest, err := writeNPMPackageMember(dest, tr)
			if err != nil {
				return result, err
			}
			digests[filename] = digest
			if filename == binaryName {
				result.BinaryArchivePath = dest
			} else {
				result.SkillsArchivePath = dest
			}
		}
	}
	// tar EOF 可能早于 gzip 尾部；必须读完，校验 gzip CRC 和整体展开上限。
	if _, err := io.Copy(npmPackagePaddingWriter{}, bounded); err != nil {
		return result, fmt.Errorf("npm gzip 尾部校验失败: %w", err)
	}
	if bounded.N == 0 {
		return result, fmt.Errorf("npm 归档超过展开大小上限")
	}
	if err := verifyNPMPackageManifest(manifest, pkg); err != nil {
		return result, err
	}
	checksums, err := strictNPMPackageChecksums(result.ChecksumsContent)
	if err != nil {
		return result, err
	}
	for filename := range selected {
		if digests[filename] == "" || checksums[filename] != digests[filename] {
			return result, fmt.Errorf("npm 包内 %s 缺失或 SHA256 校验失败", filename)
		}
	}
	return result, nil
}

// 稀疏成员的逻辑展开量可能不经过 gzip reader，必须明确拒绝。
func npmPackageSparse(records map[string]string) bool {
	for key := range records {
		if strings.HasPrefix(key, "GNU.sparse.") {
			return true
		}
	}
	return false
}

func writeNPMPackageMember(dest string, r io.Reader) (string, error) {
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	return copyNPMPackageMember(f, r)
}

func copyNPMPackageMember(w io.WriteCloser, r io.Reader) (string, error) {
	h := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(w, h), r)
	closeErr := w.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type npmPackagePaddingWriter struct{}

func (npmPackagePaddingWriter) Write(p []byte) (int, error) {
	if bytes.Count(p, []byte{0}) != len(p) {
		return 0, fmt.Errorf("npm tar 结束后含额外内容")
	}
	return len(p), nil
}

func verifyNPMPackageManifest(data []byte, pkg NPMPackage) error {
	var manifest struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("npm 包缺少有效 package.json: %w", err)
	}
	if manifest.Name == "" || manifest.Name != pkg.Name || manifest.Version == "" || manifest.Version != pkg.Version {
		return fmt.Errorf("npm 包身份与查询结果不一致")
	}
	return nil
}

func strictNPMPackageChecksums(content string) (map[string]string, error) {
	checksums := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("npm checksums.txt 格式无效")
		}
		name := fields[1]
		digest, err := hex.DecodeString(fields[0])
		if err != nil || len(digest) != sha256.Size || path.Base(name) != name || strings.ContainsAny(name, "\\:") || checksums[name] != "" {
			return nil, fmt.Errorf("npm checksums.txt 含无效或重复记录")
		}
		checksums[name] = strings.ToLower(fields[0])
	}
	if len(checksums) == 0 {
		return nil, fmt.Errorf("npm 包缺少 checksums.txt 校验记录")
	}
	return checksums, nil
}
