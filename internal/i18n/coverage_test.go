package i18n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestEveryLiteralKeyIsTranslated prevents drift between English source
// strings passed to T() and the zh-CN catalog. Dynamic keys (registry
// summaries/groups) are covered by registry tests and the docs generator.
func TestEveryLiteralKeyIsTranslated(t *testing.T) {
	var missing []string
	for _, dir := range []string{"../cli", "../daemon"} {
		paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range paths {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				name := ""
				switch fn := call.Fun.(type) {
				case *ast.Ident:
					name = fn.Name
				case *ast.SelectorExpr:
					name = fn.Sel.Name
				}
				if name != "T" && name != "tr" {
					return true
				}
				for _, arg := range call.Args {
					lit, ok := arg.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					key, err := strconv.Unquote(lit.Value)
					if err != nil || strings.TrimSpace(key) == "" {
						continue
					}
					if _, ok := catalogZHCN[key]; !ok {
						missing = append(missing, path+": "+key)
					}
					return true
				}
				return true
			})
		}
	}
	for _, path := range []string{"../cli/admin.go", "../cli/actions.go", "../cli/docs.go"} {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			kv, ok := n.(*ast.KeyValueExpr)
			if !ok {
				return true
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok || (key.Name != "Group" && key.Name != "Summary" && key.Name != "Doc") {
				return true
			}
			var collect func(ast.Expr)
			collect = func(e ast.Expr) {
				switch v := e.(type) {
				case *ast.BasicLit:
					if v.Kind != token.STRING {
						return
					}
					text, err := strconv.Unquote(v.Value)
					if err == nil && strings.TrimSpace(text) != "" {
						if _, ok := catalogZHCN[text]; !ok {
							missing = append(missing, path+": registry "+text)
						}
					}
				case *ast.CompositeLit:
					for _, el := range v.Elts {
						collect(el)
					}
				case *ast.KeyValueExpr:
					collect(v.Value)
				}
			}
			collect(kv.Value)
			return true
		})
	}
	for _, note := range []string{
		"macOS: Touch ID confirmation via a bundled helper; same-user malware is outside the trust model.",
		"Linux: no biometric confirmation (master password is required); PR_SET_DUMPABLE blocks core dumps but same-user ptrace remains possible.",
		"Windows: pipe ACL restricted to the current user; Windows Hello is used when available, otherwise the master password is required.",
	} {
		if _, ok := catalogZHCN[note]; !ok {
			missing = append(missing, "platform SecurityNote: "+note)
		}
	}
	if len(missing) > 0 {
		t.Errorf("missing zh-CN translations:\n%s", strings.Join(missing, "\n"))
	}
}
