package appguard

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// Plan 4.14 item 6: updater.HandleHelperMode() is the first statement of main(), so nothing (paths, log file,
// services, the old-app prompt) runs when the binary is the updater's helper.
func TestHandleHelperModeIsFirst(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "../../main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range file.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "main" || fd.Recv != nil {
			continue
		}
		es, ok := fd.Body.List[0].(*ast.ExprStmt)
		if !ok {
			t.Fatal("first statement of main is not a call")
		}
		call, ok := es.X.(*ast.CallExpr)
		if !ok {
			t.Fatal("first statement of main is not a call")
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		id, ok2 := sel.X.(*ast.Ident)
		if !ok || !ok2 || sel.Sel.Name != "HandleHelperMode" || id.Name != "updater" {
			t.Fatal("first statement of main must be updater.HandleHelperMode()")
		}
		return
	}
	t.Fatal("no main()")
}

// The old-app check must run before Wails builds its single-instance manager (inside application.New).
func TestOldAppCheckRunsBeforeApplicationNew(t *testing.T) {
	b, err := os.ReadFile("../../main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	g, n := strings.Index(src, "appguard.Run("), strings.Index(src, "application.New(")
	if g < 0 || n < 0 || g > n {
		t.Fatalf("appguard.Run at %d, application.New at %d", g, n)
	}
}
