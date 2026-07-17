package helpers

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func newProxyTestRoot(proxy *cobra.Command, target *cobra.Command) *cobra.Command {
	root := &cobra.Command{Use: "dws"}
	root.AddCommand(proxy)
	if target != nil {
		root.AddCommand(target)
	}
	return root
}

func TestCrossPlatformCoverageProxySubCommandCoverage(t *testing.T) {
	t.Run("missing product", func(t *testing.T) {
		proxy := proxySubCmd("proxy", "doc", "read", nil)
		newProxyTestRoot(proxy, nil)
		if err := proxy.RunE(proxy, nil); err == nil {
			t.Fatal("missing product returned nil")
		}
	})
	t.Run("missing nested path", func(t *testing.T) {
		proxy := proxySubCmd("proxy", "doc", "block missing", nil)
		doc := &cobra.Command{Use: "doc"}
		doc.AddCommand(&cobra.Command{Use: "block"})
		newProxyTestRoot(proxy, doc)
		if err := proxy.RunE(proxy, nil); err == nil {
			t.Fatal("missing nested path returned nil")
		}
	})
	t.Run("disable parsing RunE", func(t *testing.T) {
		var got []string
		proxy := proxySubCmd("proxy", "doc", "block read", map[string]string{"old": "new"})
		doc := &cobra.Command{Use: "doc"}
		block := &cobra.Command{Use: "block"}
		read := &cobra.Command{Use: "read", DisableFlagParsing: true, RunE: func(_ *cobra.Command, args []string) error {
			got = append([]string(nil), args...)
			return nil
		}}
		block.AddCommand(read)
		doc.AddCommand(block)
		newProxyTestRoot(proxy, doc)
		args := []string{"value", "--old=x", "--old", "y", "--other=z"}
		if err := proxy.RunE(proxy, args); err != nil {
			t.Fatal(err)
		}
		if strings.Join(got, " ") != "value --new=x --new y --other=z" {
			t.Fatalf("renamed args = %v", got)
		}
	})
	t.Run("disable parsing Run", func(t *testing.T) {
		called := false
		proxy := proxySubCmd("proxy", "doc", "raw", nil)
		doc := &cobra.Command{Use: "doc"}
		doc.AddCommand(&cobra.Command{Use: "raw", DisableFlagParsing: true, Run: func(*cobra.Command, []string) { called = true }})
		newProxyTestRoot(proxy, doc)
		if err := proxy.RunE(proxy, []string{"x"}); err != nil || !called {
			t.Fatalf("error=%v called=%v", err, called)
		}
	})
	t.Run("parsed RunE", func(t *testing.T) {
		called := false
		proxy := proxySubCmd("proxy", "doc", "read", nil)
		doc := &cobra.Command{Use: "doc"}
		read := &cobra.Command{Use: "read", RunE: func(cmd *cobra.Command, args []string) error {
			called = cmd.Flags().Lookup("name").Value.String() == "value" && len(args) == 1
			return nil
		}}
		read.Flags().String("name", "", "")
		doc.AddCommand(read)
		newProxyTestRoot(proxy, doc)
		if err := proxy.RunE(proxy, []string{"--name", "value", "arg"}); err != nil || !called {
			t.Fatalf("error=%v called=%v", err, called)
		}
	})
	t.Run("parsed Run and parse error", func(t *testing.T) {
		called := false
		proxy := proxySubCmd("proxy", "doc", "read", nil)
		doc := &cobra.Command{Use: "doc"}
		read := &cobra.Command{Use: "read", Run: func(*cobra.Command, []string) { called = true }}
		read.Flags().String("known", "", "")
		doc.AddCommand(read)
		newProxyTestRoot(proxy, doc)
		if err := proxy.RunE(proxy, []string{"--unknown"}); err == nil {
			t.Fatal("parse error returned nil")
		}
		if err := proxy.RunE(proxy, []string{"--known", "x"}); err != nil || !called {
			t.Fatalf("error=%v called=%v", err, called)
		}
	})
	t.Run("target itself and no runner", func(t *testing.T) {
		proxy := proxySubCmd("proxy", "doc", "", nil)
		doc := &cobra.Command{Use: "doc"}
		newProxyTestRoot(proxy, doc)
		if err := proxy.RunE(proxy, nil); err == nil {
			t.Fatal("runner-less target returned nil")
		}
	})
}

func executeWikiEdge(t *testing.T, args ...string) error {
	t.Helper()
	oldDeps := deps
	oldArgs := os.Args
	InitDeps(&scriptedToolCaller{})
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	t.Cleanup(func() {
		deps = oldDeps
		os.Args = oldArgs
	})
	root := newWikiCommand()
	installExampleGlobalFlags(root)
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	os.Args = append([]string{"dws", "wiki"}, args...)
	return root.Execute()
}

func TestCrossPlatformCoverageWikiRoutingAndValidationEdges(t *testing.T) {
	for _, args := range [][]string{
		{"space", "list", "--type", "orgSpace", "--limit", "3", "--cursor", "next"},
		{"space", "list", "--type", "mySpace", "--limit", "not-a-number"},
		{"space", "search", "--type", "myWikiSpace"},
	} {
		if err := executeWikiEdge(t, args...); err != nil {
			t.Fatalf("Execute(%v): %v", args, err)
		}
	}
	for _, args := range [][]string{
		{"node", "list", "--workspace", "space", "--folder", "123"},
		{"node", "create", "--workspace", "space", "--name", "name", "--folder", "123"},
		{"node", "copy", "--workspace", "space", "--node", "node", "--folder", "123"},
		{"node", "move", "--workspace", "space", "--node", "node", "--folder", "123"},
	} {
		if err := executeWikiEdge(t, args...); err == nil {
			t.Fatalf("Execute(%v) returned nil", args)
		}
	}
}

func TestCrossPlatformCoverageWikiDeleteCancellationEdges(t *testing.T) {
	oldStdin := os.Stdin
	t.Cleanup(func() { os.Stdin = oldStdin })
	for _, args := range [][]string{
		{"space", "delete", "--workspace", "space"},
		{"node", "delete", "--workspace", "space", "--node", "node"},
	} {
		stdin, err := os.CreateTemp(t.TempDir(), "stdin")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = stdin.WriteString("no\n")
		_, _ = stdin.Seek(0, 0)
		os.Stdin = stdin
		if err := executeWikiEdge(t, args...); err != nil {
			t.Fatalf("cancel %v: %v", args, err)
		}
		_ = stdin.Close()
	}
}
