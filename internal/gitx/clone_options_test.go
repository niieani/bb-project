package gitx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCloneWithOptions(t *testing.T) {
	t.Parallel()

	t.Run("shallow clone", func(t *testing.T) {
		t.Parallel()

		runner := Runner{}
		root := t.TempDir()
		origin := setupCloneOptionsTestRemote(t, runner, root)
		clonePath := filepath.Join(root, "clone-shallow")

		err := runner.CloneWithOptions(CloneOptions{
			Origin:  "file://" + origin,
			Path:    clonePath,
			Shallow: true,
		})
		if err != nil {
			t.Fatalf("CloneWithOptions failed: %v", err)
		}
		out, err := runner.RunGit(clonePath, "rev-parse", "--is-shallow-repository")
		if err != nil {
			t.Fatalf("rev-parse shallow: %v", err)
		}
		if out == "true" {
			return
		}
		count, err := runner.RunGit(clonePath, "rev-list", "--count", "HEAD")
		if err != nil {
			t.Fatalf("rev-list --count HEAD: %v", err)
		}
		if count != "1" {
			t.Fatalf("expected shallow history count=1, got %q", count)
		}
	})

	t.Run("partial clone filter", func(t *testing.T) {
		t.Parallel()

		runner := Runner{}
		root := t.TempDir()
		origin := setupCloneOptionsTestRemote(t, runner, root)
		clonePath := filepath.Join(root, "clone-filter")

		err := runner.CloneWithOptions(CloneOptions{
			Origin: "file://" + origin,
			Path:   clonePath,
			Filter: "blob:none",
		})
		if err != nil {
			t.Fatalf("CloneWithOptions failed: %v", err)
		}
		filter, err := runner.RunGit(clonePath, "config", "--get", "remote.origin.partialclonefilter")
		if err != nil {
			t.Fatalf("git config partialclonefilter: %v", err)
		}
		if filter != "blob:none" {
			t.Fatalf("partialclonefilter = %q, want %q", filter, "blob:none")
		}
	})

	t.Run("sparse checkout paths", func(t *testing.T) {
		t.Parallel()

		runner := Runner{}
		root := t.TempDir()
		origin := setupCloneOptionsTestRemote(t, runner, root)
		clonePath := filepath.Join(root, "clone-sparse")

		err := runner.CloneWithOptions(CloneOptions{
			Origin: "file://" + origin,
			Path:   clonePath,
			Only:   []string{"docs/guide.md"},
		})
		if err != nil {
			t.Fatalf("CloneWithOptions failed: %v", err)
		}
		if _, err := os.Stat(filepath.Join(clonePath, "docs", "guide.md")); err != nil {
			t.Fatalf("expected sparse path present: %v", err)
		}
		if _, err := os.Stat(filepath.Join(clonePath, "README.md")); err == nil {
			t.Fatal("expected README.md to be omitted by sparse checkout")
		}
	})
}

func setupCloneOptionsTestRemote(t *testing.T, runner Runner, root string) string {
	t.Helper()

	remotePath := filepath.Join(root, "remote.git")
	if _, err := runner.RunGit(root, "init", "--bare", remotePath); err != nil {
		t.Fatalf("init bare remote: %v", err)
	}
	if _, err := runner.RunGit(remotePath, "config", "uploadpack.allowFilter", "true"); err != nil {
		t.Fatalf("config allowFilter: %v", err)
	}
	if _, err := runner.RunGit(remotePath, "config", "uploadpack.allowAnySHA1InWant", "true"); err != nil {
		t.Fatalf("config allowAnySHA1InWant: %v", err)
	}

	workPath := filepath.Join(root, "work")
	if _, err := runner.RunGit(root, "clone", remotePath, workPath); err != nil {
		t.Fatalf("clone work repo: %v", err)
	}
	if _, err := runner.RunGit(workPath, "checkout", "-B", "main"); err != nil {
		t.Fatalf("checkout main: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workPath, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workPath, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workPath, "docs", "guide.md"), []byte("guide\n"), 0o644); err != nil {
		t.Fatalf("write guide: %v", err)
	}
	if _, err := runner.RunGit(workPath, "add", "-A"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := runner.RunGit(workPath, "commit", "-m", "init"); err != nil {
		t.Fatalf("git commit: %v", err)
	}
	if _, err := runner.RunGit(workPath, "push", "-u", "origin", "main"); err != nil {
		t.Fatalf("git push: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workPath, "CHANGELOG.md"), []byte("v1\n"), 0o644); err != nil {
		t.Fatalf("write CHANGELOG: %v", err)
	}
	if _, err := runner.RunGit(workPath, "add", "CHANGELOG.md"); err != nil {
		t.Fatalf("git add CHANGELOG: %v", err)
	}
	if _, err := runner.RunGit(workPath, "commit", "-m", "second"); err != nil {
		t.Fatalf("git commit second: %v", err)
	}
	if _, err := runner.RunGit(workPath, "push", "origin", "main"); err != nil {
		t.Fatalf("git push second: %v", err)
	}
	if _, err := runner.RunGit(remotePath, "symbolic-ref", "HEAD", "refs/heads/main"); err != nil {
		t.Fatalf("set remote HEAD: %v", err)
	}
	return remotePath
}
