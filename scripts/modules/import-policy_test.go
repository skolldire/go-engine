package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaggedSourcePolicy(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("modules.tsv", ".\texample.com/repo\tcore\tcoordinated\tv0.30.0\tbasic\t-\nconfig\texample.com/repo/config\ttool\tunreleased\t-\tproduction\tstdlib\n")
	write("go.mod", "module example.com/repo\n")
	write("config/go.mod", "module example.com/repo/config\n")
	write("config/own.go", "package config\nimport _ \"example.com/repo/config\"\n")
	write("consumer/go.mod", "module example.com/consumer\n")
	write("consumer/main.go", "package main\nimport _ \"example.com/repo/config\"\n")
	write("safe.go", "package repo\n// import \"example.com/repo/config\"\nvar text = `import \"example.com/repo/config\"`\n")
	if err := sourcePolicy(root); err != nil {
		t.Fatal(err)
	}
	write("tagged_test.go", "//go:build hiddenpolicy\n\npackage repo\nimport renamed `example.com/repo/config/subpackage`\n")
	err := sourcePolicy(root)
	if err == nil || !strings.Contains(err.Error(), "tagged_test.go:4") || !strings.Contains(err.Error(), "imports unreleased config") {
		t.Fatal(err)
	}
}
