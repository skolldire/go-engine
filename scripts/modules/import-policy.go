package main

import (
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type catalogModule struct{ dir, path, track string }

func sourcePolicy(root string) error {
	//nolint:gosec // G304: repository path is local operator input, never a request path.
	data, err := os.ReadFile(filepath.Join(root, "modules.tsv"))
	if err != nil {
		return err
	}
	var modules []catalogModule
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 7 {
			return errors.New("source policy: invalid catalog row")
		}
		modules = append(modules, catalogModule{fields[0], fields[1], fields[3]})
	}
	var violations []string
	for _, owner := range modules {
		if owner.track != "coordinated" {
			continue
		}
		base := filepath.Clean(filepath.Join(root, owner.dir))
		//nolint:gosec // G703: catalog paths are validated by modules.sh; this read-only CLI has no request input.
		err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, walkError error) error {
			if walkError != nil {
				return walkError
			}
			if entry.IsDir() {
				if path != base {
					if strings.HasPrefix(entry.Name(), ".") {
						return filepath.SkipDir
					}
					if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
						return filepath.SkipDir
					}
				}
				return nil
			}
			if !strings.HasSuffix(entry.Name(), ".go") || strings.Contains(entry.Name(), " 2") {
				return nil
			}
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if err != nil {
				return fmt.Errorf("source policy parse %s: %w", path, err)
			}
			for _, spec := range file.Imports {
				imported, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					return err
				}
				var target *catalogModule
				for i := range modules {
					candidate := &modules[i]
					if (imported == candidate.path || strings.HasPrefix(imported, candidate.path+"/")) && (target == nil || len(candidate.path) > len(target.path)) {
						target = candidate
					}
				}
				if target != nil && target.track == "unreleased" {
					relative, err := filepath.Rel(root, path)
					if err != nil {
						return err
					}
					violations = append(violations, fmt.Sprintf("VIOLATION: coordinated %s imports unreleased %s at %s:%d (%s)", owner.dir, target.dir, relative, fset.Position(spec.Pos()).Line, imported))
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	if len(violations) > 0 {
		return errors.New(strings.Join(violations, "\n"))
	}
	return nil
}
func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	if err := sourcePolicy(root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
