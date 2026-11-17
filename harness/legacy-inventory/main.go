// Command legacy-inventory lists every exported identifier of the legacy
// go-engine code and checks a parity matrix against that list.
//
// "Legacy" is derived from the catalog rather than maintained by hand: the
// modules on the coordinated track, plus the example application. Nested
// modules are separate boundaries and are never descended into, which is
// what keeps the new standalone modules out of the inventory.
package main

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// entry is one exported identifier. ID is what a matrix row must name.
type entry struct {
	Module, Package, Kind, Name string
	// Doc is the first sentence of the declaration's comment, or empty.
	Doc string
}

func (e entry) ID() string { return e.Package + "." + e.Name }

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: legacy-inventory inventory <repo> | check <repo> <matrix.md>")
		os.Exit(2)
	}
	entries, err := inventory(os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	switch os.Args[1] {
	case "inventory":
		if err := write(os.Stdout, entries); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "check":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "check needs the matrix file")
			os.Exit(2)
		}
		if err := check(entries, os.Args[3]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, "unknown command: "+os.Args[1])
		os.Exit(2)
	}
}

// write prints the inventory as TSV and reports a failed write, which a
// closed pipe would otherwise turn into a silently truncated inventory.
func write(out io.Writer, entries []entry) error {
	w := bufio.NewWriter(out)
	if _, err := fmt.Fprintln(w, "# module\tpackage\tkind\tidentifier\tdoc"); err != nil {
		return err
	}
	for _, e := range entries {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", e.Module, e.Package, e.Kind, e.Name, e.Doc); err != nil {
			return err
		}
	}
	return w.Flush()
}

// legacyModules reads the catalog and returns the directories to scan.
// The repository is read through an fs.FS rooted at repo, so no path built
// from catalog data can leave it.
func legacyModules(repo fs.FS) ([]string, error) {
	data, err := fs.ReadFile(repo, "modules.tsv")
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Split(line, "\t")
		if strings.HasPrefix(line, "#") || len(fields) < 4 {
			continue
		}
		if fields[3] == "coordinated" || fields[0] == "example" {
			dirs = append(dirs, fields[0])
		}
	}
	sort.Strings(dirs)
	return dirs, nil
}

func inventory(dir string) ([]entry, error) {
	repo := os.DirFS(dir)
	modules, err := legacyModules(repo)
	if err != nil {
		return nil, err
	}
	var out []entry
	for _, module := range modules {
		root := path.Clean(module)
		err := fs.WalkDir(repo, root, func(name string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			base := d.Name()
			if d.IsDir() {
				if name == root {
					return nil
				}
				if strings.HasPrefix(base, ".") || base == "testdata" || base == "docs" || base == "vendor" {
					return fs.SkipDir
				}
				// A go.mod below the module root starts another module.
				if _, statErr := fs.Stat(repo, path.Join(name, "go.mod")); statErr == nil {
					return fs.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(base, ".go") || strings.HasSuffix(base, "_test.go") || strings.Contains(base, " 2") {
				return nil
			}
			src, readErr := fs.ReadFile(repo, name)
			if readErr != nil {
				return readErr
			}
			found, parseErr := exported(name, src)
			if parseErr != nil {
				return parseErr
			}
			for _, e := range found {
				e.Module, e.Package = module, path.Dir(name)
				out = append(out, e)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	// Build tags can declare one identifier in two files.
	unique := out[:0]
	for i, e := range out {
		if i == 0 || e.ID() != out[i-1].ID() {
			unique = append(unique, e)
		}
	}
	return unique, nil
}

// exported returns the exported constants, variables, types, functions and
// methods declared in one file. Methods are named Type.Method and listed
// only when their receiver type is itself exported.
func exported(name string, src []byte) ([]entry, error) {
	file, err := parser.ParseFile(token.NewFileSet(), name, src, parser.SkipObjectResolution|parser.ParseComments)
	if err != nil {
		return nil, err
	}
	var out []entry
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if !d.Name.IsExported() {
				continue
			}
			if d.Recv == nil {
				out = append(out, entry{Kind: "func", Name: d.Name.Name, Doc: sentence(d.Doc)})
				continue
			}
			if recv := receiver(d.Recv.List[0].Type); ast.IsExported(recv) {
				out = append(out, entry{Kind: "method", Name: recv + "." + d.Name.Name, Doc: sentence(d.Doc)})
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if s.Name.IsExported() {
						out = append(out, entry{Kind: "type", Name: s.Name.Name, Doc: sentence(firstDoc(s.Doc, d.Doc))})
					}
				case *ast.ValueSpec:
					kind := "var"
					if d.Tok == token.CONST {
						kind = "const"
					}
					for _, name := range s.Names {
						if name.IsExported() {
							out = append(out, entry{Kind: kind, Name: name.Name, Doc: sentence(firstDoc(s.Doc, s.Comment, d.Doc))})
						}
					}
				}
			}
		}
	}
	return out, nil
}

// firstDoc returns the first non-empty comment group: a spec's own comment
// before the comment of the declaration that groups it.
func firstDoc(groups ...*ast.CommentGroup) *ast.CommentGroup {
	for _, g := range groups {
		if g != nil && strings.TrimSpace(g.Text()) != "" {
			return g
		}
	}
	return nil
}

// sentence returns the first sentence of a comment on one line, bounded so
// that a matrix cell stays readable. Tabs and pipes are replaced because the
// output is both TSV and a Markdown table cell.
func sentence(g *ast.CommentGroup) string {
	if g == nil {
		return ""
	}
	text := strings.Join(strings.Fields(g.Text()), " ")
	if i := strings.Index(text, ". "); i >= 0 {
		text = text[:i+1]
	}
	text = strings.NewReplacer("|", "/", "\t", " ").Replace(text)
	if r := []rune(text); len(r) > 160 {
		text = string(r[:157]) + "..."
	}
	return text
}

func receiver(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return receiver(t.X)
	case *ast.IndexExpr:
		return receiver(t.X)
	case *ast.IndexListExpr:
		return receiver(t.X)
	case *ast.Ident:
		return t.Name
	}
	return ""
}

// A matrix row starts with the identifier in backticks and carries its
// status in the fourth cell: | `pkg/path.Ident` | what | destination | STATUS | ...
var rowPattern = regexp.MustCompile("^\\| `([^`]+)` \\|")

var statuses = []string{"MIGRADO", "MIGRADO CON RUPTURA", "PLANIFICADO", "HUECO", "DESCARTE PROPUESTO", "DESCARTE APROBADO"}

// check compares the matrix with the inventory in both directions and
// prints the count per status. Rows whose identifier starts with "cap:"
// are capabilities, which have no declaration to compare against.
func check(entries []entry, matrix string) error {
	// Opened through a filesystem rooted at the matrix's directory: the tool
	// reads exactly the file it was given and nothing relative to it.
	file, err := os.DirFS(filepath.Dir(matrix)).Open(filepath.Base(matrix))
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	want := map[string]bool{}
	for _, e := range entries {
		want[e.ID()] = true
	}
	seen := map[string]int{}
	counts := map[string]int{}
	capabilities := map[string]int{}
	var problems []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1<<20), 1<<24)
	for line := 1; scanner.Scan(); line++ {
		match := rowPattern.FindStringSubmatch(scanner.Text())
		if match == nil {
			continue
		}
		cells := strings.Split(scanner.Text(), "|")
		if len(cells) < 6 {
			problems = append(problems, fmt.Sprintf("line %d: row has too few cells", line))
			continue
		}
		status := strings.TrimSpace(cells[4])
		valid := false
		for _, s := range statuses {
			valid = valid || s == status
		}
		if !valid {
			problems = append(problems, fmt.Sprintf("line %d: unknown status %q", line, status))
			continue
		}
		id := match[1]
		if strings.HasPrefix(id, "cap:") {
			capabilities[status]++
			continue
		}
		seen[id]++
		if seen[id] > 1 {
			problems = append(problems, fmt.Sprintf("line %d: duplicate row for %s", line, id))
			continue
		}
		if !want[id] {
			problems = append(problems, fmt.Sprintf("line %d: %s is not an exported legacy identifier", line, id))
			continue
		}
		counts[status]++
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	for _, e := range entries {
		if seen[e.ID()] == 0 {
			problems = append(problems, "missing row: "+e.ID())
		}
	}
	fmt.Printf("legacy identifiers: %d\nmatrix rows for identifiers: %d\n", len(entries), len(seen))
	for _, s := range statuses {
		fmt.Printf("%-20s identifiers %5d   capabilities %4d\n", s, counts[s], capabilities[s])
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		limit := len(problems)
		if limit > 40 {
			limit = 40
		}
		return fmt.Errorf("matrix is not exhaustive: %d problems\n%s", len(problems), strings.Join(problems[:limit], "\n"))
	}
	fmt.Println("exhaustive: every exported legacy identifier has exactly one row")
	return nil
}
