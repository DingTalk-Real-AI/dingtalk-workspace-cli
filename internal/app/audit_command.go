package app

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	"github.com/spf13/cobra"
)

var (
	auditCSVWrite = func(writer *csv.Writer, record []string) error { return writer.Write(record) }
	auditCSVFlush = func(writer *csv.Writer) { writer.Flush() }
	auditCSVError = func(writer *csv.Writer) error { return writer.Error() }
	auditExit     = os.Exit
	auditVerify   = audit.VerifyFile
)

func newAuditCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "操作审计日志管理",
		Long:  "查看、导出和校验本地操作审计日志。",
	}
	cmd.AddCommand(
		newAuditTailCommand(),
		newAuditExportCommand(),
		newAuditVerifyCommand(),
	)
	return cmd
}

func newAuditTailCommand() *cobra.Command {
	var n int
	cmd := &cobra.Command{
		Use:   "tail",
		Short: "查看最近的审计记录",
		RunE: func(cmd *cobra.Command, args []string) error {
			if n < 1 {
				return fmt.Errorf("--lines 必须为正整数，收到 %d", n)
			}
			dir := auditDir()
			file, err := audit.LatestAuditFile(dir)
			if err != nil {
				return fmt.Errorf("无审计记录: %w", err)
			}
			lines, err := tailFile(file, n)
			if err != nil {
				return err
			}
			for _, line := range lines {
				fmt.Println(line)
			}
			return nil
		},
	}
	cmd.Flags().IntVarP(&n, "lines", "n", 20, "显示最近 N 条记录")
	return cmd
}

func newAuditExportCommand() *cobra.Command {
	var since, until, format string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "导出审计日志",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := auditDir()

			sinceDate := strings.ReplaceAll(since, "-", "")
			untilDate := strings.ReplaceAll(until, "-", "")

			files, err := audit.AuditFilesInRange(dir, sinceDate, untilDate)
			if err != nil {
				return fmt.Errorf("查找审计文件失败: %w", err)
			}
			if len(files) == 0 {
				return fmt.Errorf("指定范围内无审计文件")
			}

			switch format {
			case "jsonl":
				return exportJSONL(files)
			case "csv":
				return exportCSV(files)
			default:
				return fmt.Errorf("不支持的格式: %s（可选 jsonl, csv）", format)
			}
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "起始日期 (YYYY-MM-DD)")
	cmd.Flags().StringVar(&until, "until", "", "截止日期 (YYYY-MM-DD)")
	cmd.Flags().StringVar(&format, "format", "jsonl", "输出格式: jsonl 或 csv")
	return cmd
}

func newAuditVerifyCommand() *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "校验审计日志哈希链完整性",
		RunE: func(cmd *cobra.Command, args []string) error {
			target := file
			if target == "" {
				dir := auditDir()
				var err error
				target, err = audit.LatestAuditFile(dir)
				if err != nil {
					return fmt.Errorf("无审计文件: %w", err)
				}
			}

			valid, brokenAt, err := auditVerify(target)
			if err != nil {
				return fmt.Errorf("校验失败: %w", err)
			}
			if valid {
				fmt.Printf("✓ %s 哈希链完整（全部通过）\n", filepath.Base(target))
			} else {
				fmt.Printf("✗ %s 哈希链在第 %d 行断裂\n", filepath.Base(target), brokenAt)
				auditExit(1)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "指定审计文件路径（默认最新文件）")
	return cmd
}

func auditDir() string {
	if dir := os.Getenv(audit.EnvAuditDir); dir != "" {
		return dir
	}
	return filepath.Join(defaultConfigDir(), "audit")
}

func tailFile(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}

func exportJSONL(files []string) error {
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			fmt.Println(scanner.Text())
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			return err
		}
	}
	return nil
}

func exportCSV(files []string) error {
	w := csv.NewWriter(os.Stdout)

	header := []string{"timestamp", "execution_id", "user_id", "corp_id", "product", "command", "result", "duration_ms", "error_category"}
	if err := auditCSVWrite(w, header); err != nil {
		return fmt.Errorf("写入 CSV 表头失败: %w", err)
	}

	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Bytes()
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			var evt audit.Event
			if err := json.Unmarshal(line, &evt); err != nil {
				f.Close()
				return fmt.Errorf("解析审计记录失败 %s:%d: %w", file, lineNum, err)
			}
			row := []string{
				evt.Timestamp.Format(time.RFC3339),
				evt.ExecutionID,
				evt.Actor.UserID,
				evt.Actor.CorpID,
				evt.Product,
				evt.Command,
				evt.Result,
				strconv.FormatInt(evt.DurationMs, 10),
				evt.ErrCategory,
			}
			if err := auditCSVWrite(w, row); err != nil {
				f.Close()
				return fmt.Errorf("写入 CSV 记录失败: %w", err)
			}
		}
		if err := scanner.Err(); err != nil {
			f.Close()
			return err
		}
		f.Close()
	}

	auditCSVFlush(w)
	if err := auditCSVError(w); err != nil {
		return fmt.Errorf("刷新 CSV 输出失败: %w", err)
	}
	return nil
}
