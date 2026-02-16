package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bb-project/internal/cli"
)

func TestGenerateDocs(t *testing.T) {
	t.Parallel()

	root := cli.NewRootCommand(io.Discard, io.Discard)
	tempDir := t.TempDir()
	markdownDir := filepath.Join(tempDir, "cli")
	manDir := filepath.Join(tempDir, "man", "man1")

	if err := generateDocs(root, markdownDir, manDir); err != nil {
		t.Fatalf("generateDocs error = %v", err)
	}

	mustFileExists(t, filepath.Join(markdownDir, "bb.md"))
	mustFileExists(t, filepath.Join(manDir, "bb.1"))

	content, err := os.ReadFile(filepath.Join(markdownDir, "bb.md"))
	if err != nil {
		t.Fatalf("read generated root markdown: %v", err)
	}
	if !strings.Contains(string(content), "completion") {
		t.Fatalf("expected root markdown to document completion command, got:\n%s", string(content))
	}
	if !strings.Contains(string(content), "clone") {
		t.Fatalf("expected root markdown to document clone command, got:\n%s", string(content))
	}
	if !strings.Contains(string(content), "link") {
		t.Fatalf("expected root markdown to document link command, got:\n%s", string(content))
	}
}

func TestGenerateDocsRequiresRoot(t *testing.T) {
	t.Parallel()

	if err := generateDocs(nil, t.TempDir(), t.TempDir()); err == nil {
		t.Fatal("expected error for nil root command")
	}
}

func mustFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file %s to exist: %v", path, err)
	}
}
