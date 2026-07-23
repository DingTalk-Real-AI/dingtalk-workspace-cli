package app

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/spf13/cobra"
)

type skillErrorReadCloser struct{ err error }

func (r skillErrorReadCloser) Read([]byte) (int, error) { return 0, r.err }
func (skillErrorReadCloser) Close() error               { return nil }

func skillCoverageResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func skillCoverageCommand() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetOut(io.Discard)
	cmd.Flags().String("skill-id", "id", "")
	cmd.Flags().String("query", "query", "")
	cmd.Flags().String("source", "", "")
	cmd.Flags().String("scopes", "scope", "")
	return cmd
}

func TestCrossPlatformCoverageSkillCommandHighLevelRemainingCoverage(t *testing.T) {
	oldToken := skillLoadAccessToken
	oldTmp := skillDownloadToTmp
	oldHTTP := skillHTTPDo
	oldNewRequest := skillNewRequest
	oldTarget := skillResolveTargetPath
	oldFetch := skillFetchDownloadInfo
	oldDownload := skillDownloadFile
	oldExtract := skillExtractZip
	t.Cleanup(func() {
		skillLoadAccessToken = oldToken
		skillDownloadToTmp = oldTmp
		skillHTTPDo = oldHTTP
		skillNewRequest = oldNewRequest
		skillResolveTargetPath = oldTarget
		skillFetchDownloadInfo = oldFetch
		skillDownloadFile = oldDownload
		skillExtractZip = oldExtract
	})
	fail := errors.New("failure")
	cmd := skillCoverageCommand()
	skillLoadAccessToken = func(context.Context) (string, error) { return "", fail }
	if err := runSkillGet(cmd, nil); !errors.Is(err, fail) {
		t.Fatalf("skill get auth error = %v", err)
	}
	skillLoadAccessToken = func(context.Context) (string, error) { return "token", nil }
	skillNewRequest = func(context.Context, string, string, io.Reader) (*http.Request, error) { return nil, fail }
	if err := runSkillFind(cmd, nil); err == nil {
		t.Fatal("skill find request failure should propagate")
	}
	skillNewRequest = oldNewRequest
	skillDownloadToTmp = func(context.Context, string, string) (string, error) { return "", fail }
	if err := runSkillGet(cmd, nil); !errors.Is(err, fail) {
		t.Fatalf("skill get download error = %v", err)
	}

	skillLoadAccessToken = func(context.Context) (string, error) { return "", fail }
	if err := runSkillFind(cmd, nil); !errors.Is(err, fail) {
		t.Fatalf("skill find auth error = %v", err)
	}
	skillLoadAccessToken = func(context.Context) (string, error) { return "token", nil }
	skillHTTPDo = func(*http.Client, *http.Request) (*http.Response, error) { return nil, fail }
	if err := runSkillFind(cmd, nil); err == nil {
		t.Fatal("skill find network failure should propagate")
	}
	skillHTTPDo = func(*http.Client, *http.Request) (*http.Response, error) {
		return skillCoverageResponse(http.StatusBadRequest, "bad"), nil
	}
	if err := runSkillFind(cmd, nil); err == nil {
		t.Fatal("skill find HTTP failure should propagate")
	}
	skillHTTPDo = func(*http.Client, *http.Request) (*http.Response, error) {
		return skillCoverageResponse(http.StatusOK, "{"), nil
	}
	if err := runSkillFind(cmd, nil); err == nil {
		t.Fatal("skill find malformed response should fail")
	}
	for _, body := range []string{
		`{"success":false,"errorMsg":"message"}`,
		`{"success":false,"errorCode":"code"}`,
		`{"success":false}`,
	} {
		skillHTTPDo = func(*http.Client, *http.Request) (*http.Response, error) {
			return skillCoverageResponse(http.StatusOK, body), nil
		}
		if err := runSkillFind(cmd, nil); err == nil {
			t.Fatalf("skill find API failure %s should propagate", body)
		}
	}

	skillResolveTargetPath = func(string) (string, error) { return "", fail }
	if err := runSkillAdd(cmd, []string{"id", "target"}); err == nil {
		t.Fatal("invalid skill target should fail")
	}
	skillResolveTargetPath = func(string) (string, error) { return "dest", nil }
	skillLoadAccessToken = func(context.Context) (string, error) { return "", fail }
	if err := runSkillAdd(cmd, []string{"id", "target"}); !errors.Is(err, fail) {
		t.Fatalf("skill add auth error = %v", err)
	}
	skillLoadAccessToken = func(context.Context) (string, error) { return "token", nil }
	skillFetchDownloadInfo = func(context.Context, string, string) (*downloadSkillResponse, error) { return nil, fail }
	if err := runSkillAdd(cmd, []string{"id", "target"}); !errors.Is(err, fail) {
		t.Fatalf("skill info error = %v", err)
	}
	for _, response := range []*downloadSkillResponse{
		{Success: false, ErrorMsg: "message"},
		{Success: false, ErrorCode: "code"},
		{Success: false},
		{Success: true},
		{Success: true, Result: &downloadSkillResult{}},
	} {
		skillFetchDownloadInfo = func(context.Context, string, string) (*downloadSkillResponse, error) { return response, nil }
		if err := runSkillAdd(cmd, []string{"id", "target"}); err == nil {
			t.Fatalf("invalid download response %#v should fail", response)
		}
	}
	skillFetchDownloadInfo = func(context.Context, string, string) (*downloadSkillResponse, error) {
		return &downloadSkillResponse{Success: true, Result: &downloadSkillResult{DownloadURL: "url", FileName: "skill.zip"}}, nil
	}
	skillDownloadFile = func(context.Context, string, string) (string, error) { return "", fail }
	if err := runSkillAdd(cmd, []string{"id", "target"}); !errors.Is(err, fail) {
		t.Fatalf("skill file download error = %v", err)
	}
	skillDownloadFile = func(context.Context, string, string) (string, error) { return "temp.zip", nil }
	skillExtractZip = func(string, string) error { return fail }
	if err := runSkillAdd(cmd, []string{"id", "target"}); !errors.Is(err, fail) {
		t.Fatalf("skill extraction error = %v", err)
	}
	skillExtractZip = func(string, string) error { return nil }
	if err := runSkillAdd(cmd, []string{"id", "target"}); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageSkillCommandLowLevelRemainingCoverage(t *testing.T) {
	oldHTTP := skillHTTPDo
	oldNewRequest, oldResolveToken := skillNewRequest, skillResolveAccessToken
	oldHome := skillUserHomeDir
	oldMkdirTemp, oldCreate, oldCreateTemp := skillMkdirTemp, skillCreate, skillCreateTemp
	oldRemoveAll, oldRemove, oldMkdir := skillRemoveAll, skillRemove, skillMkdirAll
	oldOpen, oldCopy, oldZipOpen := skillOpenFile, skillCopy, skillOpenZipFile
	t.Cleanup(func() {
		skillHTTPDo = oldHTTP
		skillNewRequest, skillResolveAccessToken = oldNewRequest, oldResolveToken
		skillUserHomeDir = oldHome
		skillMkdirTemp, skillCreate, skillCreateTemp = oldMkdirTemp, oldCreate, oldCreateTemp
		skillRemoveAll, skillRemove, skillMkdirAll = oldRemoveAll, oldRemove, oldMkdir
		skillOpenFile, skillCopy, skillOpenZipFile = oldOpen, oldCopy, oldZipOpen
	})
	fail := errors.New("failure")
	skillResolveAccessToken = func(context.Context, string, string) (string, error) {
		return "", authpkg.ErrTokenDataNotFound
	}
	if _, err := loadSkillAccessToken(context.Background()); err == nil {
		t.Fatal("invalid skill access token succeeded")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	skillResolveAccessToken = func(ctx context.Context, _, _ string) (string, error) {
		return "", ctx.Err()
	}
	if _, err := loadSkillAccessToken(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("skill token cancellation = %v", err)
	}
	skillResolveAccessToken = oldResolveToken
	skillNewRequest = func(context.Context, string, string, io.Reader) (*http.Request, error) { return nil, fail }
	if _, err := fetchSkillDownloadInfo(context.Background(), "token", "id"); err == nil {
		t.Fatal("download-info request failure should propagate")
	}
	if _, err := downloadSkillFile(context.Background(), "https://skill.test", "token"); err == nil {
		t.Fatal("skill-file request failure should propagate")
	}
	skillNewRequest = oldNewRequest
	skillUserHomeDir = func() (string, error) { return "", fail }
	if _, err := resolveSkillTargetPath("codex"); err == nil {
		t.Fatal("skill target HOME error should fail")
	}
	t.Setenv("DWS_SKILL_API_HOST", "https://skill.test/")
	if skillAPIHost() != "https://skill.test" {
		t.Fatal("skill API override was not normalized")
	}

	skillHTTPDo = func(*http.Client, *http.Request) (*http.Response, error) { return nil, fail }
	if _, err := fetchSkillDownloadInfo(context.Background(), "token", "id"); err == nil {
		t.Fatal("download-info network failure should propagate")
	}
	for _, status := range []int{http.StatusUnauthorized, http.StatusBadGateway} {
		skillHTTPDo = func(*http.Client, *http.Request) (*http.Response, error) {
			return skillCoverageResponse(status, "body"), nil
		}
		if _, err := fetchSkillDownloadInfo(context.Background(), "token", "id"); err == nil {
			t.Fatalf("download-info HTTP %d should fail", status)
		}
	}
	skillHTTPDo = func(*http.Client, *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: skillErrorReadCloser{err: fail}}, nil
	}
	if _, err := fetchSkillDownloadInfo(context.Background(), "token", "id"); err == nil {
		t.Fatal("download-info body failure should propagate")
	}
	skillHTTPDo = func(*http.Client, *http.Request) (*http.Response, error) {
		return skillCoverageResponse(http.StatusOK, "{"), nil
	}
	if _, err := fetchSkillDownloadInfo(context.Background(), "token", "id"); err == nil {
		t.Fatal("malformed download-info should fail")
	}

	skillHTTPDo = func(*http.Client, *http.Request) (*http.Response, error) {
		return skillCoverageResponse(http.StatusOK, "body"), nil
	}
	skillHTTPDo = func(*http.Client, *http.Request) (*http.Response, error) { return nil, fail }
	if _, err := downloadSkillToTmpDir(context.Background(), "https://skill.test", "token"); err == nil {
		t.Fatal("temporary skill download network failure should propagate")
	}
	skillHTTPDo = func(*http.Client, *http.Request) (*http.Response, error) {
		return skillCoverageResponse(http.StatusOK, "body"), nil
	}
	skillMkdirTemp = func(string, string) (string, error) { return "", fail }
	if _, err := downloadSkillToTmpDir(context.Background(), "https://skill.test", "token"); err == nil {
		t.Fatal("download temp-dir failure should propagate")
	}
	tmpDir := t.TempDir()
	skillMkdirTemp = func(string, string) (string, error) { return tmpDir, nil }
	skillCreate = func(string) (*os.File, error) { return nil, fail }
	if _, err := downloadSkillToTmpDir(context.Background(), "https://skill.test", "token"); err == nil {
		t.Fatal("download temp-file failure should propagate")
	}
	skillCreate = func(string) (*os.File, error) { return os.CreateTemp(t.TempDir(), "skill") }
	skillCopy = func(io.Writer, io.Reader) (int64, error) { return 0, fail }
	if _, err := downloadSkillToTmpDir(context.Background(), "https://skill.test", "token"); err == nil {
		t.Fatal("download save failure should propagate")
	}

	skillHTTPDo = func(*http.Client, *http.Request) (*http.Response, error) { return nil, fail }
	if _, err := downloadSkillFile(context.Background(), "https://skill.test", "x"); err == nil {
		t.Fatal("skill-file network failure should propagate")
	}
	skillHTTPDo = func(*http.Client, *http.Request) (*http.Response, error) {
		return skillCoverageResponse(http.StatusBadGateway, "body"), nil
	}
	if _, err := downloadSkillFile(context.Background(), "https://skill.test", "x"); err == nil {
		t.Fatal("skill-file HTTP failure should propagate")
	}
	skillHTTPDo = func(*http.Client, *http.Request) (*http.Response, error) {
		return skillCoverageResponse(http.StatusOK, "body"), nil
	}
	skillCreateTemp = func(string, string) (*os.File, error) { return nil, fail }
	if _, err := downloadSkillFile(context.Background(), "https://skill.test", "x"); err == nil {
		t.Fatal("skill-file temp failure should propagate")
	}
	skillCreateTemp = func(string, string) (*os.File, error) { return os.CreateTemp(t.TempDir(), "skill") }
	skillCopy = func(io.Writer, io.Reader) (int64, error) { return 0, fail }
	if _, err := downloadSkillFile(context.Background(), "https://skill.test", "x"); err == nil {
		t.Fatal("skill-file copy failure should propagate")
	}
	skillCopy = func(w io.Writer, _ io.Reader) (int64, error) {
		_ = w.(*os.File).Close()
		return 0, nil
	}
	if _, err := downloadSkillFile(context.Background(), "https://skill.test", "x"); err == nil {
		t.Fatal("skill-file close failure should propagate")
	}

	skillMkdirAll = func(string, os.FileMode) error { return fail }
	if err := extractSkillZip("missing", "dest"); err == nil {
		t.Fatal("zip destination mkdir failure should propagate")
	}
	zipPath := filepath.Join(t.TempDir(), "files.zip")
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	_, _ = zw.Create("dir/")
	w, _ := zw.Create("file")
	_, _ = w.Write([]byte("content"))
	header := &zip.FileHeader{Name: "mode-file", Method: zip.Store}
	header.SetMode(0o111)
	w, _ = zw.CreateHeader(header)
	_, _ = w.Write([]byte("mode"))
	_ = zw.Close()
	if err := os.WriteFile(zipPath, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	skillMkdirAll = func(string, os.FileMode) error { return fail }
	if err := extractZipFile(zr.File[0], t.TempDir()); err == nil {
		t.Fatal("zip directory mkdir failure should propagate")
	}
	if err := extractZipFile(zr.File[1], t.TempDir()); err == nil {
		t.Fatal("zip parent mkdir failure should propagate")
	}
	skillMkdirAll = func(string, os.FileMode) error { return nil }
	skillOpenZipFile = func(*zip.File) (io.ReadCloser, error) { return nil, fail }
	if err := extractZipFile(zr.File[1], t.TempDir()); err == nil {
		t.Fatal("zip source-open failure should propagate")
	}
	skillOpenZipFile = oldZipOpen
	skillOpenFile = func(string, int, os.FileMode) (*os.File, error) { return nil, fail }
	if err := extractZipFile(zr.File[1], t.TempDir()); err == nil {
		t.Fatal("zip destination-open failure should propagate")
	}
	skillOpenFile = func(string, int, os.FileMode) (*os.File, error) { return os.CreateTemp(t.TempDir(), "out") }
	skillCopy = func(io.Writer, io.Reader) (int64, error) { return 0, fail }
	if err := extractZipFile(zr.File[1], t.TempDir()); err == nil {
		t.Fatal("zip copy failure should propagate")
	}
	skillCopy = func(io.Writer, io.Reader) (int64, error) { return 0, nil }
	var usedMode os.FileMode
	skillOpenFile = func(_ string, _ int, mode os.FileMode) (*os.File, error) {
		usedMode = mode
		return os.CreateTemp(t.TempDir(), "mode")
	}
	if err := extractZipFile(zr.File[2], t.TempDir()); err != nil || usedMode != 0o644 {
		t.Fatalf("zip fallback mode = %v, %v", usedMode, err)
	}
}
