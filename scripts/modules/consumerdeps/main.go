// Command consumerdeps validates additional external dependencies of composed consumers.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type moduleRow struct{ dir, path, kind, track, checks, deps string }
type policy struct {
	root      string
	modules   map[string]moduleRow
	extras    map[string][]string
	locations map[string]string
}

func main() {
	if len(os.Args) < 2 || len(os.Args) > 3 {
		fmt.Fprintln(os.Stderr, "usage: consumerdeps ROOT [MODULE]")
		os.Exit(2)
	}
	p, err := readPolicy(os.Args[1])
	if err == nil {
		if len(os.Args) == 3 {
			err = p.check(os.Args[2])
			if err == nil {
				for _, dep := range p.extras[os.Args[2]] {
					fmt.Println(dep)
				}
			}
		} else {
			var failures []error
			for _, dir := range sortedKeys(p.modules) {
				if p.modules[dir].checks == "production" {
					if e := p.check(dir); e != nil {
						failures = append(failures, e)
					}
				}
			}
			err = errors.Join(failures...)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
func readLines(path string) ([]string, error) {
	//nolint:gosec // Repository-local catalog/data paths, not remote input.
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	var lines []string
	for _, line := range strings.Split(string(b), "\n") {
		if line != "" && !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	return lines, nil
}
func readPolicy(root string) (*policy, error) {
	p := &policy{root: root, modules: map[string]moduleRow{}, extras: map[string][]string{}, locations: map[string]string{}}
	lines, e := readLines(filepath.Join(root, "modules.tsv"))
	if e != nil {
		return nil, e
	}
	var repoPaths []string
	for _, line := range lines {
		f := strings.Split(line, "\t")
		if len(f) != 7 {
			return nil, errors.New("consumer-deps: invalid catalog")
		}
		if !fs.ValidPath(f[0]) {
			return nil, errors.New("consumer-deps: invalid catalog directory")
		}
		p.modules[f[0]] = moduleRow{f[0], f[1], f[2], f[3], f[5], f[6]}
		repoPaths = append(repoPaths, f[1])
	}
	lines, e = readLines(filepath.Join(root, "scripts/modules/overrides.tsv"))
	if e != nil && !errors.Is(e, fs.ErrNotExist) {
		return nil, e
	}
	for _, line := range lines {
		f := strings.Split(line, "\t")
		if len(f) == 3 && f[1] == "consumers-dir" {
			if !fs.ValidPath(f[2]) {
				return nil, errors.New("consumer-deps: invalid consumer directory")
			}
			p.locations[f[0]] = f[2]
		}
	}
	lines, e = readLines(filepath.Join(root, "scripts/modules/consumer-deps.tsv"))
	if e != nil && !errors.Is(e, fs.ErrNotExist) {
		return nil, e
	}
	exact := regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.~-]*\.[A-Za-z0-9.~-]+(/[A-Za-z0-9_.~-]+)+$`)
	for _, line := range lines {
		f := strings.Split(line, "\t")
		if len(f) != 3 {
			return nil, errors.New("consumer-deps: expected module, composed, external paths")
		}
		row, ok := p.modules[f[0]]
		if !ok || row.checks != "production" {
			return nil, fmt.Errorf("consumer-deps: unknown/nonproduction module %s", f[0])
		}
		if f[1] != "composed" {
			return nil, errors.New("consumer-deps: only composed entries are allowed")
		}
		if _, seen := p.extras[f[0]]; seen {
			return nil, fmt.Errorf("consumer-deps: duplicate module %s", f[0])
		}
		seen := map[string]bool{}
		for _, dep := range strings.Split(f[2], ",") {
			if !exact.MatchString(dep) || strings.Contains(dep, "...") || seen[dep] {
				return nil, fmt.Errorf("consumer-deps: invalid/duplicate exact external path %s", dep)
			}
			for _, rp := range repoPaths {
				if dep == rp || strings.HasPrefix(dep, rp+"/") {
					return nil, fmt.Errorf("consumer-deps: repository module prohibited: %s", dep)
				}
			}
			if dep == "github.com/sirupsen/logrus" {
				return nil, errors.New("consumer-deps: logrus is prohibited (D44)")
			}
			seen[dep] = true
			p.extras[f[0]] = append(p.extras[f[0]], dep)
		}
	}
	return p, nil
}
func (p *policy) check(dir string) error {
	row, ok := p.modules[dir]
	if !ok || row.checks != "production" {
		return fmt.Errorf("consumer-deps: unknown/nonproduction module %s", dir)
	}
	//nolint:gosec // Directory belongs to the parsed catalog and passes fs.ValidPath; root is the local checkout.
	if _, err := os.Stat(filepath.Join(p.root, dir, "go.mod")); err != nil {
		return fmt.Errorf("consumer-deps: %s missing module go.mod: %w", dir, err)
	}
	location := p.locations[dir]
	if location == "" {
		location = filepath.Join("examples/standalone", dir)
	}
	target := filepath.Join(p.root, location, "composed")
	//nolint:gosec // Target is a validated repository-local consumer directory, not a remote request path.
	if _, err := os.Stat(filepath.Join(target, "go.mod")); err != nil {
		return fmt.Errorf("consumer-deps: %s missing composed go.mod: %w", dir, err)
	}
	allowed := map[string]bool{row.path: true}
	for _, m := range p.modules {
		if m.kind == "tool" || m.kind == "composition" || m.kind == "source" {
			allowed[m.path] = true
		}
	}
	if row.deps != "stdlib" && row.deps != "-" {
		for _, dep := range strings.Split(row.deps, ",") {
			if m, ok := p.modules[dep]; ok {
				allowed[m.path] = true
			} else {
				allowed[dep] = true
			}
		}
	}
	extra := map[string]bool{}
	for _, dep := range p.extras[dir] {
		if allowed[dep] {
			return fmt.Errorf("consumer-deps: %s redundant declaration %s", dir, dep)
		}
		extra[dep] = true
	}
	var mod struct {
		Module  struct{ Path string }
		Require []struct{ Path string }
	}
	raw, err := runGo(target, "mod", "edit", "-json")
	if err != nil {
		return err
	}
	if err = json.Unmarshal(raw, &mod); err != nil {
		return err
	}
	allowed[mod.Module.Path] = true
	required := map[string]bool{}
	for _, r := range mod.Require {
		required[r.Path] = true
	}
	for dep := range extra {
		if !required[dep] {
			return fmt.Errorf("consumer-deps: %s declared dependency absent from go.mod: %s", dir, dep)
		}
	}
	imports := map[string]bool{}
	//nolint:gosec // Traverse the validated local consumer directory to parse its imports.
	err = filepath.WalkDir(target, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if strings.Contains(entry.Name(), " 2") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if path != target {
				if strings.HasPrefix(entry.Name(), ".") || entry.Name() == "testdata" {
					return filepath.SkipDir
				}
				if _, e := os.Stat(filepath.Join(path, "go.mod")); e == nil {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, e := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if e != nil {
			return e
		}
		for _, spec := range file.Imports {
			imp, e := strconv.Unquote(spec.Path.Value)
			if e != nil {
				return e
			}
			if imp != "C" {
				imports[imp] = true
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	args := append([]string{"list", "-json"}, sortedKeys(imports)...)
	if len(imports) == 0 {
		for dep := range extra {
			return fmt.Errorf("consumer-deps: %s unused declaration %s", dir, dep)
		}
		return nil
	}
	raw, err = runGo(target, args...)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	used := map[string]bool{}
	for decoder.More() {
		var pkg struct {
			ImportPath string
			Module     *struct{ Path string }
		}
		if err = decoder.Decode(&pkg); err != nil {
			return err
		}
		if pkg.Module == nil {
			continue
		}
		owner := pkg.Module.Path
		if allowed[owner] {
			continue
		}
		if !extra[owner] {
			return fmt.Errorf("consumer-deps: %s undeclared external direct import %s (module %s)", dir, pkg.ImportPath, owner)
		}
		used[owner] = true
	}
	for dep := range extra {
		if !used[dep] {
			return fmt.Errorf("consumer-deps: %s unused declaration %s", dir, dep)
		}
	}
	return nil
}
func runGo(dir string, args ...string) ([]byte, error) {
	//nolint:gosec // Fixed Go operations over local repository paths/imports, never a shell command.
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local", "GOPROXY=off", "GOFLAGS=-mod=readonly")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("consumer-deps: %s: go %s: %w\n%s", dir, strings.Join(args, " "), err, out)
	}
	return out, nil
}
