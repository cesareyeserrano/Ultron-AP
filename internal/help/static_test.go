package help

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// goListDeps shells out to "go list -deps <pkgPattern>" from the repo root.
// The test working directory is the package dir (internal/help); we resolve
// the repo root by walking up until we find go.mod.
func goListDeps(t *testing.T, pattern string) (string, error) {
	t.Helper()
	root, err := repoRoot()
	if err != nil {
		return "", err
	}
	cmd := exec.Command("go", "list", "-deps", pattern)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// globGoFiles returns every *.go file under dir excluding *_test.go and the
// contract subpackage (which is intentionally a peer; we only check the help
// package itself).
func globGoFiles(t *testing.T, dir string) ([]string, error) {
	t.Helper()
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Skip contract subpackage and glossary data dir.
			if info.Name() == "contract" || info.Name() == "glossary" || info.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".go") {
			return nil
		}
		if strings.HasSuffix(info.Name(), "_test.go") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files, err
}

// readImports parses the given Go source file and returns the import paths.
func readImports(path string) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(f.Imports))
	for _, imp := range f.Imports {
		out = append(out, strings.Trim(imp.Path.Value, `"`))
	}
	return out, nil
}

// repoRoot walks up from the test's CWD until a go.mod is found.
func repoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

// silence unused imports when only goListDeps is used directly.
var _ = ast.NewIdent
