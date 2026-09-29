package domain

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestDomainDependencyRuleRejectsForbiddenImports(t *testing.T) {
	for _, importPath := range []string{
		"net/http",
		"net/http/httptest",
		"database/sql",
		"database/sql/driver",
		"github.com/jackc/pgx/v5",
		"github.com/jackc/pgx/v5/pgxpool",
		"scheduler/api/internal/postgres",
	} {
		t.Run(importPath, func(t *testing.T) {
			dir := t.TempDir()
			source := "package domain\nimport \"" + importPath + "\"\n"
			if err := os.WriteFile(filepath.Join(dir, "forbidden.go"), []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			err := checkDomainImports(dir)
			if err == nil || !strings.Contains(err.Error(), importPath) {
				t.Errorf("checkDomainImports(%q) = %v, want forbidden import %q", dir, err, importPath)
			}
		})
	}
}

func TestDomainDependencyRuleAllowsStandardPackages(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "allowed.go"), []byte("package domain\nimport \"time\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkDomainImports(dir); err != nil {
		t.Errorf("checkDomainImports() = %v, want standard library import allowed", err)
	}
}

func TestDomainPackagesKeepDependencyBoundary(t *testing.T) {
	if err := checkDomainImports("."); err != nil {
		t.Fatal(err)
	}
}

func checkDomainImports(root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return err
			}
			firstSegment, _, _ := strings.Cut(importPath, "/")
			if importPath == "net/http" || strings.HasPrefix(importPath, "net/http/") || importPath == "database/sql" || strings.HasPrefix(importPath, "database/sql/") || strings.Contains(firstSegment, ".") || strings.HasPrefix(importPath, "scheduler/api/") {
				return fmt.Errorf("%s imports forbidden package %s", path, importPath)
			}
		}
		return nil
	})
}
