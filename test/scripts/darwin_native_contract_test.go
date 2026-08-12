package scripts_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestMacOSNativeJobKeepsDarwinGatedAppTestsReachable couples the macOS test
// job's -run pattern to the set of darwin-gated tests in internal/app.
//
// The Ubuntu race shard runs the whole package but skips anything gated on
// runtime.GOOS != "darwin", and the platform coverage gate only runs
// ^(TestAllShortcuts|TestCrossPlatformCoverage). That makes the macOS job the
// only place a darwin-gated internal/app test can execute, so narrowing its
// -run pattern can silently orphan one — which is exactly what happened in
// #857 before review caught it.
func TestMacOSNativeJobKeepsDarwinGatedAppTestsReachable(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}

	pattern := macOSAppRunPattern(t, root)
	matcher, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatalf("Compile(macOS -run pattern %q) error = %v", pattern, err)
	}

	gated := darwinGatedTestNames(t, filepath.Join(root, "internal", "app"))
	if len(gated) == 0 {
		t.Fatal("found no darwin-gated tests in internal/app; the scanner is broken or the gate style changed")
	}

	for _, name := range gated {
		if !matcher.MatchString(name) {
			t.Errorf(
				"darwin-gated test %s is unreachable in CI: the Ubuntu shard skips it on Linux and the macOS -run pattern %q does not select it",
				name,
				pattern,
			)
		}
	}
}

// macOSAppRunPattern extracts the -run pattern the macOS job applies to
// ./internal/app from the CI workflow.
func macOSAppRunPattern(t *testing.T, root string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("ReadFile(ci.yml) error = %v", err)
	}
	workflow := string(data)

	start := strings.Index(workflow, "\n  test-darwin:\n")
	end := strings.Index(workflow, "\n  test-windows:\n")
	if start < 0 || end <= start {
		t.Fatal("ci.yml missing macOS test job boundaries")
	}

	matches := regexp.MustCompile(`\./internal/app -run '([^']+)'`).FindAllStringSubmatch(workflow[start:end], -1)
	if len(matches) != 1 {
		t.Fatalf("macOS test job ./internal/app -run invocation count = %d, want exactly 1", len(matches))
	}
	return matches[0][1]
}

// darwinGatedTestNames returns every Test function in dir whose body gates on
// runtime.GOOS != "darwin".
func darwinGatedTestNames(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
	if err != nil {
		t.Fatalf("Glob(%s) error = %v", dir, err)
	}

	var names []string
	fset := token.NewFileSet()
	for _, entry := range entries {
		file, parseErr := parser.ParseFile(fset, entry, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			t.Fatalf("ParseFile(%s) error = %v", entry, parseErr)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !strings.HasPrefix(fn.Name.Name, "Test") {
				continue
			}
			if gatesOnNonDarwin(fn.Body) {
				names = append(names, fn.Name.Name)
			}
		}
	}
	return names
}

// gatesOnNonDarwin reports whether body contains a `runtime.GOOS != "darwin"`
// comparison, the idiom this repo uses to skip a test off macOS.
func gatesOnNonDarwin(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		if found {
			return false
		}
		binary, ok := node.(*ast.BinaryExpr)
		if !ok || binary.Op != token.NEQ {
			return true
		}
		if !isRuntimeGOOS(binary.X) {
			return true
		}
		literal, ok := binary.Y.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		if literal.Value == `"darwin"` {
			found = true
			return false
		}
		return true
	})
	return found
}

// isRuntimeGOOS reports whether expr is the selector runtime.GOOS.
func isRuntimeGOOS(expr ast.Expr) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "GOOS" {
		return false
	}
	ident, ok := selector.X.(*ast.Ident)
	return ok && ident.Name == "runtime"
}
