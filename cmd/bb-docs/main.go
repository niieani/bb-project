package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"bb-project/internal/cli"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	root := cli.NewRootCommand(io.Discard, io.Discard)
	markdownDir := filepath.Join("docs", "cli")
	manDir := filepath.Join("docs", "man", "man1")
	return generateDocs(root, markdownDir, manDir)
}

func generateDocs(root *cobra.Command, markdownDir string, manDir string) error {
	if root == nil {
		return errors.New("root command is required")
	}

	markDisableAutoGen(root)

	if err := resetDirectory(markdownDir); err != nil {
		return err
	}
	if err := resetDirectory(manDir); err != nil {
		return err
	}

	if err := doc.GenMarkdownTree(root, markdownDir); err != nil {
		return fmt.Errorf("generate markdown docs: %w", err)
	}

	head := &doc.GenManHeader{Title: "BB", Section: "1", Source: "bb"}
	if err := doc.GenManTree(root, head, manDir); err != nil {
		return fmt.Errorf("generate man pages: %w", err)
	}

	return nil
}

func markDisableAutoGen(cmd *cobra.Command) {
	cmd.DisableAutoGenTag = true
	for _, child := range cmd.Commands() {
		markDisableAutoGen(child)
	}
}

func resetDirectory(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", path, err)
	}
	return nil
}
