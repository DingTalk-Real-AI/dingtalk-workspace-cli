// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestLeafSpecPostMountDoesNotRegisterBusinessFlags keeps business flags in
// Flags. PostMount may set surface metadata and may call domain tools such as
// registerDevAppCursorFlags (pagination is a tool, not a LeafSpec field).
func TestLeafSpecPostMountDoesNotRegisterBusinessFlags(t *testing.T) {
	for _, call := range leafSpecCompositeLits(t) {
		post := fieldExpr(call, "PostMount")
		if post == nil {
			continue
		}
		ast.Inspect(post, func(n ast.Node) bool {
			expr, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			fun, ok := expr.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch fun.Sel.Name {
			case "String", "Bool", "Int", "StringSlice", "Int64", "Float64":
				inner, ok := fun.X.(*ast.CallExpr)
				if !ok {
					return true
				}
				innerSel, ok := inner.Fun.(*ast.SelectorExpr)
				if ok && innerSel.Sel.Name == "Flags" {
					t.Errorf("%s: PostMount registers business flag via Flags().%s; declare it in Flags instead (cursor tools use registerDevAppCursorFlags)",
						call.pos, fun.Sel.Name)
				}
			}
			return true
		})
	}
}

// TestLeafSpecCallDoesNotAssembleBusinessParams keeps Call as an execution body.
// Business binding belongs in Flags/ConstParams. Named helpers like
// devAppCall / devAppCallCursor may apply domain tooling (including cursor).
func TestLeafSpecCallDoesNotAssembleBusinessParams(t *testing.T) {
	for _, call := range leafSpecCompositeLits(t) {
		body := fieldExpr(call, "Call")
		if body == nil {
			continue
		}
		// Named helpers (devAppCall / devAppCallCursor) are execution tools.
		if _, ok := body.(*ast.CallExpr); ok {
			continue
		}
		if ident, ok := body.(*ast.Ident); ok {
			if strings.HasPrefix(ident.Name, "devAppCall") {
				continue
			}
		}
		ast.Inspect(body, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for _, lhs := range assign.Lhs {
				index, ok := lhs.(*ast.IndexExpr)
				if !ok {
					continue
				}
				if ident, ok := index.X.(*ast.Ident); ok && ident.Name == "params" {
					t.Errorf("%s: Call assigns params[...]; move business binding to Flags/ConstParams", call.pos)
				}
			}
			return true
		})
	}
}

// TestLeafSpecNoLegacyWriteGuard pins the migration end-state: devapp leaves
// declare SafetySpec.Confirmation and the framework applies it even around a
// RunE escape hatch; the legacy devAppRequireWriteGuard helper is gone.
func TestLeafSpecNoLegacyWriteGuard(t *testing.T) {
	for _, call := range leafSpecCompositeLits(t) {
		if validate := fieldExpr(call, "Validate"); validate != nil && astCallsIdent(validate, "devAppRequireWriteGuard") {
			t.Errorf("%s: LeafSpec must not call devAppRequireWriteGuard; declare Safety.Confirmation instead", call.pos)
		}
	}
}

func astCallsIdent(root ast.Node, name string) bool {
	found := false
	ast.Inspect(root, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			switch fun := x.Fun.(type) {
			case *ast.Ident:
				if fun.Name == name {
					found = true
					return false
				}
			case *ast.SelectorExpr:
				if fun.Sel.Name == name {
					found = true
					return false
				}
			}
		}
		return !found
	})
	return found
}

type leafSpecLit struct {
	pos string
	lit *ast.CompositeLit
}

func leafSpecCompositeLits(t *testing.T) []leafSpecLit {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read helpers dir: %v", err)
	}
	var out []leafSpecLit
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("parse helpers file %s: %v", name, err)
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			fun, ok := call.Fun.(*ast.Ident)
			if !ok || fun.Name != "NewLeafCommand" || len(call.Args) != 1 {
				return true
			}
			lit, ok := call.Args[0].(*ast.CompositeLit)
			if !ok {
				return true
			}
			if typ, ok := lit.Type.(*ast.Ident); !ok || typ.Name != "LeafSpec" {
				return true
			}
			out = append(out, leafSpecLit{
				pos: fset.Position(lit.Pos()).String(),
				lit: lit,
			})
			return true
		})
	}
	if len(out) == 0 {
		t.Fatal("no NewLeafCommand(LeafSpec{...}) literals found")
	}
	return out
}

func fieldExpr(call leafSpecLit, name string) ast.Expr {
	for _, elt := range call.lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != name {
			continue
		}
		return kv.Value
	}
	return nil
}
