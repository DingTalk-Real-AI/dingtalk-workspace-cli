package helpers

import (
	"os"
	"path/filepath"
	"testing"
)

// ──────────────────────────────────────────────────────────
// validateLocalDirAbs — 本地路径校验（必须为绝对路径）
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageValidateLocalDirAbs_empty(t *testing.T) {
	if _, err := validateLocalDirAbs(""); err == nil {
		t.Fatal("expected error for empty --local-folder")
	}
}

func TestCrossPlatformCoverageValidateLocalDirAbs_relative(t *testing.T) {
	if _, err := validateLocalDirAbs(filepath.Join(".", "repo")); err == nil {
		t.Fatal("expected error for relative path")
	}
}

func TestCrossPlatformCoverageValidateLocalDirAbs_ok(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "repo")
	got, err := validateLocalDirAbs(abs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != filepath.Clean(abs) {
		t.Errorf("got %q, want %q", got, filepath.Clean(abs))
	}
}

// ──────────────────────────────────────────────────────────
// walkLocalTree — 本地遍历
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageWalkLocalTree_collectsRegularFiles(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "hello")
	mustWrite(t, filepath.Join(root, "sub", "b.txt"), "world")

	files, err := walkLocalTree(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(files), files)
	}
	// rel_path 用 / 分隔（跨平台一致）
	if _, ok := files["sub/b.txt"]; !ok {
		t.Errorf("expected key sub/b.txt, got keys %v", keys(files))
	}
	// walk 阶段不计算 hash（惰性策略），但记录 AbsPath 与 mtime
	if files["a.txt"].Hash != "" {
		t.Error("walkLocalTree should not compute MD5 eagerly")
	}
	if files["a.txt"].AbsPath == "" {
		t.Error("walkLocalTree should record AbsPath")
	}
	if files["a.txt"].ModTimeMillis == 0 {
		t.Error("walkLocalTree should record mtime")
	}
}

// ──────────────────────────────────────────────────────────
// compareTrees — 四类差异分类
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageCompareTrees_exact(t *testing.T) {
	local := map[string]*localFile{
		"same.txt":   {RelPath: "same.txt", Hash: "h1"},
		"diff.txt":   {RelPath: "diff.txt", Hash: "local"},
		"onlyme.txt": {RelPath: "onlyme.txt", Hash: "h3"},
	}
	remote := map[string]*remoteFile{
		"same.txt":     {RelPath: "same.txt", Hash: "h1"},
		"diff.txt":     {RelPath: "diff.txt", Hash: "remote"},
		"onlythem.txt": {RelPath: "onlythem.txt", Hash: "h4"},
	}
	res, err := compareTrees(local, remote, "exact", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertPaths(t, "unchanged", res.Unchanged, "same.txt")
	assertPaths(t, "modified", res.Modified, "diff.txt")
	assertPaths(t, "new_local", res.NewLocal, "onlyme.txt")
	assertPaths(t, "new_remote", res.NewRemote, "onlythem.txt")
	if res.Detection != "exact" {
		t.Errorf("detection = %q, want exact", res.Detection)
	}
}

func TestCrossPlatformCoverageCompareTrees_exact_unknownRemoteHashIsUnknown(t *testing.T) {
	local := map[string]*localFile{"a.txt": {RelPath: "a.txt", Hash: "h1"}}
	remote := map[string]*remoteFile{"a.txt": {RelPath: "a.txt", Hash: ""}}
	res, err := compareTrees(local, remote, "exact", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 远端无 md5 → 无法核对内容，归入 unknown（既非 unchanged 也非 modified）。
	assertPaths(t, "unknown", res.Unknown, "a.txt")
	if len(res.Unchanged) != 0 {
		t.Error("unknown remote hash must not be judged unchanged")
	}
	if len(res.Modified) != 0 {
		t.Error("unknown remote hash must not be judged modified")
	}
}

// TestCompareTrees_exact_hashesOnlyIntersection 验证本地 hash 只在双端都存在时才
// 计算：local-only 文件即使 AbsPath 指向不存在的文件也不会触发 hash（不报错）。
func TestCrossPlatformCoverageCompareTrees_exact_hashesOnlyIntersection(t *testing.T) {
	root := t.TempDir()
	bothPath := filepath.Join(root, "both.txt")
	mustWrite(t, bothPath, "same-content")
	h, err := md5File(bothPath)
	if err != nil {
		t.Fatalf("md5File: %v", err)
	}

	local := map[string]*localFile{
		"both.txt":  {RelPath: "both.txt", AbsPath: bothPath},
		"local.txt": {RelPath: "local.txt", AbsPath: filepath.Join(root, "does-not-exist.txt")},
	}
	remote := map[string]*remoteFile{
		"both.txt": {RelPath: "both.txt", Hash: h},
	}
	res, err := compareTrees(local, remote, "exact", false)
	if err != nil {
		t.Fatalf("local-only file must not be hashed, got error: %v", err)
	}
	assertPaths(t, "unchanged", res.Unchanged, "both.txt")
	assertPaths(t, "new_local", res.NewLocal, "local.txt")
	// 双端都存在的 both.txt 被惰性 hash 后应回填 Hash
	if local["both.txt"].Hash == "" {
		t.Error("intersection file should have been hashed lazily")
	}
	if local["local.txt"].Hash != "" {
		t.Error("local-only file should never be hashed")
	}
}

// TestCompareTrees_exact_noRemoteMD5AllUnknown 验证 exact 模式在远端无 md5 时
// 一律归入 unknown——不再用 fileSize + modified_time 近似判 unchanged（否则大小与
// mtime 恰好相同但内容不同的文件会被误报为未变更），无论大小/时间是否相同。
func TestCrossPlatformCoverageCompareTrees_exact_noRemoteMD5AllUnknown(t *testing.T) {
	local := map[string]*localFile{
		"sameSizeTime.txt": {RelPath: "sameSizeTime.txt", Size: 100, ModTimeMillis: 1000},
		"diffsize.txt":     {RelPath: "diffsize.txt", Size: 100, ModTimeMillis: 1000},
		"difftime.txt":     {RelPath: "difftime.txt", Size: 100, ModTimeMillis: 1000},
	}
	remote := map[string]*remoteFile{
		// 远端均无 md5（Hash 空）→ 全部 unknown，绝不判 unchanged。
		"sameSizeTime.txt": {RelPath: "sameSizeTime.txt", Size: 100, ModifiedTime: 1000, ModifiedTimeValid: true},
		"diffsize.txt":     {RelPath: "diffsize.txt", Size: 200, ModifiedTime: 1000, ModifiedTimeValid: true},
		"difftime.txt":     {RelPath: "difftime.txt", Size: 100, ModifiedTime: 2000, ModifiedTimeValid: true},
	}
	res, err := compareTrees(local, remote, "exact", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertPaths(t, "unknown", res.Unknown, "diffsize.txt", "difftime.txt", "sameSizeTime.txt")
	if len(res.Unchanged) != 0 {
		t.Errorf("远端无 md5 时不应有 unchanged, got %v", entryPaths(res.Unchanged))
	}
	if len(res.Modified) != 0 {
		t.Errorf("远端无 md5 时不应有 modified, got %v", entryPaths(res.Modified))
	}
}

// TestCompareTrees_exact_base64RemoteMD5 验证 exact 用 base64 md5 比对：本地按
// base64 算 md5，与钉盘返回的 base64 md5 直接相等 → unchanged。
func TestCrossPlatformCoverageCompareTrees_exact_base64RemoteMD5(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "a.txt")
	mustWrite(t, p, "hello root\n") // 11 字节，md5 base64 = 6y9K7B0b2K0AInNZZV/70A==
	remoteB64 := "6y9K7B0b2K0AInNZZV/70A=="
	local := map[string]*localFile{"a.txt": {RelPath: "a.txt", AbsPath: p}}
	remote := map[string]*remoteFile{"a.txt": {RelPath: "a.txt", Hash: remoteB64}}
	res, err := compareTrees(local, remote, "exact", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertPaths(t, "unchanged", res.Unchanged, "a.txt")
	if len(res.Modified) != 0 {
		t.Errorf("同一 md5(base64) 不应判 modified, got modified=%d", len(res.Modified))
	}
	// 本地按需 hash 后应回填为 base64
	if local["a.txt"].Hash != remoteB64 {
		t.Errorf("本地 md5 应回填为 base64 %q, got %q", remoteB64, local["a.txt"].Hash)
	}
}

func TestCrossPlatformCoverageCompareTrees_quick(t *testing.T) {
	local := map[string]*localFile{
		"eq.txt":  {RelPath: "eq.txt", ModTimeMillis: 1000},
		"ne.txt":  {RelPath: "ne.txt", ModTimeMillis: 1000},
		"bad.txt": {RelPath: "bad.txt", ModTimeMillis: 1000},
	}
	remote := map[string]*remoteFile{
		"eq.txt":  {RelPath: "eq.txt", ModifiedTime: 1000, ModifiedTimeValid: true},
		"ne.txt":  {RelPath: "ne.txt", ModifiedTime: 2000, ModifiedTimeValid: true},
		"bad.txt": {RelPath: "bad.txt", ModifiedTime: 1000, ModifiedTimeValid: false},
	}
	res, err := compareTrees(local, remote, "quick", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertPaths(t, "unchanged", res.Unchanged, "eq.txt")
	// ne.txt 时间不等、bad.txt 远端时间不可信 → 都记为 modified
	assertPaths(t, "modified", res.Modified, "bad.txt", "ne.txt")
}

func TestCrossPlatformCoverageCompareTrees_emptyRemote_allNewLocal(t *testing.T) {
	local := map[string]*localFile{
		"a.txt": {RelPath: "a.txt", Hash: "h1"},
		"b.txt": {RelPath: "b.txt", Hash: "h2"},
	}
	res, err := compareTrees(local, map[string]*remoteFile{}, "exact", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertPaths(t, "new_local", res.NewLocal, "a.txt", "b.txt")
	if len(res.NewRemote) != 0 || len(res.Modified) != 0 || len(res.Unchanged) != 0 {
		t.Error("with empty remote, only new_local should be populated")
	}
}

// ──────────────────────────────────────────────────────────
// helpers
// ──────────────────────────────────────────────────────────

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func keys(m map[string]*localFile) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func assertPaths(t *testing.T, label string, entries []driveStatusEntry, want ...string) {
	t.Helper()
	if len(entries) != len(want) {
		t.Fatalf("%s: got %d entries %v, want %v", label, len(entries), entryPaths(entries), want)
	}
	for i, w := range want {
		if entries[i].RelPath != w {
			t.Errorf("%s[%d] = %q, want %q", label, i, entries[i].RelPath, w)
		}
	}
}

func entryPaths(entries []driveStatusEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.RelPath
	}
	return out
}

// ──────────────────────────────────────────────────────────
// parseDriveList — list_files 返回解析
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageParseDriveList_resultItems(t *testing.T) {
	text := `{"result":{"items":[{"name":"a.txt","type":"file"},{"name":"sub","type":"folder"}],"nextToken":"tok"}}`
	items, token, err := parseDriveList(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if token != "tok" {
		t.Errorf("token = %q, want tok", token)
	}
}

func TestCrossPlatformCoverageParseDriveList_topLevelArray(t *testing.T) {
	text := `{"result":[{"name":"a.txt","type":"file"}]}`
	items, token, err := parseDriveList(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 || items[0].name() != "a.txt" {
		t.Errorf("unexpected items: %v", items)
	}
	if token != "" {
		t.Errorf("expected empty token, got %q", token)
	}
}

func TestCrossPlatformCoverageParseDriveList_invalid(t *testing.T) {
	if _, _, err := parseDriveList("not json"); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// ──────────────────────────────────────────────────────────
// driveItem — 字段访问器（带 fallback）
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageDriveItem_accessors(t *testing.T) {
	it := driveItem{
		"name":       "报告.pdf",
		"type":       "file",
		"fileId":     "d-123",
		"md5":        "abc",
		"path":       "/dir/报告.pdf",
		"modifyTime": float64(1700000000000),
	}
	if it.name() != "报告.pdf" {
		t.Errorf("name() = %q", it.name())
	}
	if it.id() != "d-123" {
		t.Errorf("id() = %q", it.id())
	}
	if it.typ() != "file" {
		t.Errorf("typ() = %q, want file", it.typ())
	}
	if it.hash() != "abc" {
		t.Errorf("hash() = %q", it.hash())
	}
	if it.path() != "/dir/报告.pdf" {
		t.Errorf("path() = %q", it.path())
	}
	if it.isFolder() {
		t.Error("type=file should not be a folder")
	}
	if !it.isFile() {
		t.Error("type=file should be a file")
	}
	ms, ok := it.modifiedMillis()
	if !ok || ms != 1700000000000 {
		t.Errorf("modifiedMillis() = %d,%v", ms, ok)
	}
}

func TestCrossPlatformCoverageDriveItem_folderAndFile(t *testing.T) {
	folder := driveItem{"name": "sub", "type": "folder"}
	if !folder.isFolder() || folder.isFile() {
		t.Error("type=folder should be folder, not file")
	}
	file := driveItem{"name": "a.txt", "type": "file"}
	if !file.isFile() || file.isFolder() {
		t.Error("type=file should be file, not folder")
	}
	// 非 file / 非 folder 的类型（在线文档、快捷方式等）都不纳入比对。
	doc := driveItem{"name": "设计稿", "type": "docx"}
	if doc.isFile() {
		t.Error("docx should not be treated as file")
	}
	shortcut := driveItem{"name": "快捷方式", "type": "shortcut"}
	if shortcut.isFile() {
		t.Error("shortcut should not be treated as file")
	}
}

// ──────────────────────────────────────────────────────────
// toMillis — 时间字段归一化
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageToMillis(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want int64
		ok   bool
	}{
		{"epoch-ms-number", float64(1700000000000), 1700000000000, true},
		{"epoch-ms-string", "1700000000000", 1700000000000, true},
		{"rfc3339", "2023-11-14T22:13:20Z", 1700000000000, true},
		{"zero", float64(0), 0, false},
		{"empty", "", 0, false},
		{"garbage", "not-a-time", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := toMillis(c.in)
			if ok != c.ok || (ok && got != c.want) {
				t.Errorf("toMillis(%v) = %d,%v; want %d,%v", c.in, got, ok, c.want, c.ok)
			}
		})
	}
}
