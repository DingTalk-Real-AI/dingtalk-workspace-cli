package helpers

import (
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

func captureDocFragmentStderr(t *testing.T, run func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	previous := os.Stderr
	os.Stderr = writer
	defer func() {
		os.Stderr = previous
		_ = writer.Close()
		_ = reader.Close()
	}()

	run()
	if err := writer.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}
	os.Stderr = previous
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	return string(output)
}

func TestCrossPlatformCoverageJSONMLFragmentStripHelpers(t *testing.T) {
	t.Run("child start", func(t *testing.T) {
		tests := []struct {
			name string
			node []any
			want int
		}{
			{name: "empty", node: nil, want: 1},
			{name: "tag only", node: []any{"p"}, want: 1},
			{name: "first child", node: []any{"p", "text"}, want: 1},
			{name: "attributes", node: []any{"p", map[string]any{}}, want: 2},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if got := jsonmlChildStart(tt.node); got != tt.want {
					t.Fatalf("jsonmlChildStart(%#v) = %d, want %d", tt.node, got, tt.want)
				}
			})
		}
	})

	t.Run("children recursively splice fragments", func(t *testing.T) {
		children := []any{
			"text",
			[]any{},
			[]any{
				"fragment",
				map[string]any{"source": "range"},
				[]any{"p", map[string]any{}},
				[]any{"fragment", []any{"h1", map[string]any{}}},
			},
			[]any{
				"div",
				map[string]any{},
				[]any{"fragment", map[string]any{}, []any{"span", map[string]any{}, "inside"}},
			},
		}
		got, count := stripFragmentChildren(children)
		want := []any{
			"text",
			[]any{},
			[]any{"p", map[string]any{}},
			[]any{"h1", map[string]any{}},
			[]any{"div", map[string]any{}, []any{"span", map[string]any{}, "inside"}},
		}
		if count != 3 || !reflect.DeepEqual(got, want) {
			t.Fatalf("stripFragmentChildren() = %#v, %d; want %#v, 3", got, count, want)
		}
	})

	t.Run("ordinary children are unchanged", func(t *testing.T) {
		children := []any{"text", []any{"p", map[string]any{}, "body"}}
		got, count := stripFragmentChildren(children)
		if count != 0 || !reflect.DeepEqual(got, children) {
			t.Fatalf("stripFragmentChildren() = %#v, %d", got, count)
		}
	})

	t.Run("node branches", func(t *testing.T) {
		leaf := []any{"p", map[string]any{}}
		got, count := stripFragmentInNode(leaf)
		if count != 0 || !reflect.DeepEqual(got, leaf) {
			t.Fatalf("leaf = %#v, %d", got, count)
		}

		ordinary := []any{"p", map[string]any{}, []any{"span", map[string]any{}, "body"}}
		got, count = stripFragmentInNode(ordinary)
		if count != 0 || !reflect.DeepEqual(got, ordinary) {
			t.Fatalf("ordinary = %#v, %d", got, count)
		}

		nested := []any{"p", map[string]any{}, []any{"fragment", map[string]any{}, []any{"span", map[string]any{}, "body"}}}
		got, count = stripFragmentInNode(nested)
		want := []any{"p", map[string]any{}, []any{"span", map[string]any{}, "body"}}
		if count != 1 || !reflect.DeepEqual(got, want) {
			t.Fatalf("nested = %#v, %d; want %#v, 1", got, count, want)
		}
	})

	t.Run("body branches", func(t *testing.T) {
		if got, count := stripBodyFragments(nil); got != nil || count != 0 {
			t.Fatalf("empty body = %#v, %d", got, count)
		}

		ordinary := []any{"root", map[string]any{}, []any{"p", map[string]any{}}}
		got, count := stripBodyFragments(ordinary)
		if count != 0 || !reflect.DeepEqual(got, ordinary) {
			t.Fatalf("ordinary body = %#v, %d", got, count)
		}

		top := []any{
			"fragment",
			map[string]any{"source": "outline"},
			[]any{"fragment", map[string]any{}, []any{"h1", map[string]any{}}},
		}
		got, count = stripBodyFragments(top)
		want := []any{"root", map[string]any{}, []any{"h1", map[string]any{}}}
		if count != 2 || !reflect.DeepEqual(got, want) {
			t.Fatalf("top fragment body = %#v, %d; want %#v, 2", got, count, want)
		}
	})

	t.Run("single node branches", func(t *testing.T) {
		if got, count, err := stripNodeFragments(nil); err != nil || got != nil || count != 0 {
			t.Fatalf("empty node = %#v, %d, %v", got, count, err)
		}

		ordinary := []any{"p", map[string]any{}, "body"}
		got, count, err := stripNodeFragments(ordinary)
		if err != nil || count != 0 || !reflect.DeepEqual(got, ordinary) {
			t.Fatalf("ordinary node = %#v, %d, %v", got, count, err)
		}

		nested := []any{"p", map[string]any{}, []any{"fragment", map[string]any{}, []any{"span", map[string]any{}, "body"}}}
		got, count, err = stripNodeFragments(nested)
		want := []any{"p", map[string]any{}, []any{"span", map[string]any{}, "body"}}
		if err != nil || count != 1 || !reflect.DeepEqual(got, want) {
			t.Fatalf("nested node = %#v, %d, %v; want %#v", got, count, err, want)
		}

		top := []any{
			"fragment",
			map[string]any{"source": "section"},
			[]any{"fragment", map[string]any{}, []any{"p", map[string]any{}, "body"}},
		}
		got, count, err = stripNodeFragments(top)
		want = []any{"p", map[string]any{}, "body"}
		if err != nil || count != 2 || !reflect.DeepEqual(got, want) {
			t.Fatalf("top node = %#v, %d, %v; want %#v", got, count, err, want)
		}
	})

	t.Run("single node top fragment errors", func(t *testing.T) {
		tests := []struct {
			name string
			node []any
			want string
		}{
			{
				name: "empty",
				node: []any{"fragment", map[string]any{}},
				want: "无可写回的节点",
			},
			{
				name: "non node child",
				node: []any{"fragment", map[string]any{}, "text"},
				want: "子节点不是合法 JSONML 节点",
			},
			{
				name: "multiple nodes",
				node: []any{"fragment", map[string]any{}, []any{"p", map[string]any{}}, []any{"p", map[string]any{}}},
				want: "一次只能写一个",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, count, err := stripNodeFragments(tt.node)
				if err == nil || !strings.Contains(err.Error(), tt.want) {
					t.Fatalf("result = %#v, %d, %v; want error containing %q", got, count, err, tt.want)
				}
			})
		}
	})
}

func TestCrossPlatformCoveragePrepareJSONMLStripsReadOnlyFragments(t *testing.T) {
	strict := jsonMLTestCommand(t, false)

	t.Run("body top fragment becomes root", func(t *testing.T) {
		var got string
		var prepareErr error
		stderr := captureDocFragmentStderr(t, func() {
			got, prepareErr = prepareJsonMLBody(
				strict,
				`{"jsonml":["fragment",{"source":"outline"},["p",{},["span",{},"ok"]]]}`,
			)
		})
		if prepareErr != nil {
			t.Fatalf("prepare body: %v", prepareErr)
		}
		want := `["root",{},["p",{},["span",{},"ok"]]]`
		if got != want {
			t.Fatalf("body = %s, want %s", got, want)
		}
		if !strings.Contains(stderr, "fragment 只读容器") || !strings.Contains(stderr, "已自动移除 1 处") {
			t.Fatalf("stderr = %q", stderr)
		}
	})

	t.Run("body nested fragment is spliced", func(t *testing.T) {
		var got string
		var prepareErr error
		stderr := captureDocFragmentStderr(t, func() {
			got, prepareErr = prepareJsonMLBody(
				strict,
				`{"jsonml":["root",{},["fragment",{},["p",{},["span",{},"nested"]]]]}`,
			)
		})
		if prepareErr != nil {
			t.Fatalf("prepare body: %v", prepareErr)
		}
		want := `["root",{},["p",{},["span",{},"nested"]]]`
		if got != want {
			t.Fatalf("body = %s, want %s", got, want)
		}
		if !strings.Contains(stderr, "已自动移除 1 处") {
			t.Fatalf("stderr = %q", stderr)
		}
	})

	t.Run("node top fragment is unwrapped", func(t *testing.T) {
		var got string
		var prepareErr error
		stderr := captureDocFragmentStderr(t, func() {
			got, prepareErr = prepareJsonMLNode(
				strict,
				`["fragment",{"source":"section"},["p",{},["span",{},"ok"]]]`,
			)
		})
		if prepareErr != nil {
			t.Fatalf("prepare node: %v", prepareErr)
		}
		want := `["p",{},["span",{},"ok"]]`
		if got != want {
			t.Fatalf("node = %s, want %s", got, want)
		}
		if !strings.Contains(stderr, "fragment 只读容器") || !strings.Contains(stderr, "已自动移除 1 处") {
			t.Fatalf("stderr = %q", stderr)
		}
	})

	t.Run("node nested fragment is spliced", func(t *testing.T) {
		var got string
		var prepareErr error
		stderr := captureDocFragmentStderr(t, func() {
			got, prepareErr = prepareJsonMLNode(
				strict,
				`["p",{},["fragment",{},["span",{},"nested"]]]`,
			)
		})
		if prepareErr != nil {
			t.Fatalf("prepare node: %v", prepareErr)
		}
		want := `["p",{},["span",{},"nested"]]`
		if got != want {
			t.Fatalf("node = %s, want %s", got, want)
		}
		if !strings.Contains(stderr, "已自动移除 1 处") {
			t.Fatalf("stderr = %q", stderr)
		}
	})

	t.Run("top node fragment errors remain actionable", func(t *testing.T) {
		tests := []struct {
			name string
			raw  string
			want string
		}{
			{name: "empty", raw: `["fragment",{}]`, want: "无可写回的节点"},
			{name: "text", raw: `["fragment",{},"text"]`, want: "子节点不是合法 JSONML 节点"},
			{
				name: "multiple",
				raw:  `["fragment",{},["p",{}],["p",{}]]`,
				want: "一次只能写一个",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if _, err := prepareJsonMLNode(strict, tt.raw); err == nil || !strings.Contains(err.Error(), tt.want) {
					t.Fatalf("prepareJsonMLNode(%s) error = %v, want %q", tt.raw, err, tt.want)
				}
			})
		}
	})
}
