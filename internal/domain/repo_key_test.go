package domain

import (
	"path/filepath"
	"testing"
)

func TestEffectiveRepoPathDepth(t *testing.T) {
	t.Parallel()

	if got := EffectiveRepoPathDepth(Catalog{}); got != 1 {
		t.Fatalf("default depth = %d, want 1", got)
	}
	if got := EffectiveRepoPathDepth(Catalog{RepoPathDepth: 1}); got != 1 {
		t.Fatalf("explicit depth 1 = %d, want 1", got)
	}
	if got := EffectiveRepoPathDepth(Catalog{RepoPathDepth: 2}); got != 2 {
		t.Fatalf("explicit depth 2 = %d, want 2", got)
	}
	if got := EffectiveRepoPathDepth(Catalog{RepoPathDepth: 42}); got != 1 {
		t.Fatalf("invalid depth fallback = %d, want 1", got)
	}
}

func TestValidateRepoPathDepth(t *testing.T) {
	t.Parallel()

	for _, depth := range []int{0, 1, 2} {
		if err := ValidateRepoPathDepth(depth); err != nil {
			t.Fatalf("ValidateRepoPathDepth(%d) returned error: %v", depth, err)
		}
	}
	if err := ValidateRepoPathDepth(3); err == nil {
		t.Fatal("expected error for depth=3")
	}
}

func TestDeriveRepoKeyFromRelative(t *testing.T) {
	t.Parallel()

	t.Run("depth 1 valid", func(t *testing.T) {
		t.Parallel()
		key, rel, name, ok := DeriveRepoKeyFromRelative(
			Catalog{Name: "software", RepoPathDepth: 1},
			"api",
		)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if key != "software/api" || rel != "api" || name != "api" {
			t.Fatalf("unexpected values key=%q rel=%q name=%q", key, rel, name)
		}
	})

	t.Run("depth 2 valid", func(t *testing.T) {
		t.Parallel()
		key, rel, name, ok := DeriveRepoKeyFromRelative(
			Catalog{Name: "software", RepoPathDepth: 2},
			"openai/codex",
		)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if key != "software/openai/codex" || rel != "openai/codex" || name != "codex" {
			t.Fatalf("unexpected values key=%q rel=%q name=%q", key, rel, name)
		}
	})

	t.Run("depth mismatch rejected", func(t *testing.T) {
		t.Parallel()
		if _, _, _, ok := DeriveRepoKeyFromRelative(Catalog{Name: "software", RepoPathDepth: 1}, "owner/repo"); ok {
			t.Fatal("expected depth mismatch to be rejected for depth=1")
		}
		if _, _, _, ok := DeriveRepoKeyFromRelative(Catalog{Name: "software", RepoPathDepth: 2}, "repo"); ok {
			t.Fatal("expected depth mismatch to be rejected for depth=2")
		}
	})
}

func TestDeriveRepoKey(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "software")
	catalog := Catalog{Name: "software", Root: root, RepoPathDepth: 2}

	t.Run("valid path under root", func(t *testing.T) {
		t.Parallel()
		repoPath := filepath.Join(root, "openai", "codex")
		key, rel, name, ok := DeriveRepoKey(catalog, repoPath)
		if !ok {
			t.Fatal("expected key derivation to succeed")
		}
		if key != "software/openai/codex" || rel != "openai/codex" || name != "codex" {
			t.Fatalf("unexpected values key=%q rel=%q name=%q", key, rel, name)
		}
	})

	t.Run("path with invalid depth rejected", func(t *testing.T) {
		t.Parallel()
		repoPath := filepath.Join(root, "codex")
		if _, _, _, ok := DeriveRepoKey(catalog, repoPath); ok {
			t.Fatal("expected invalid depth to be rejected")
		}
	})

	t.Run("path outside root rejected", func(t *testing.T) {
		t.Parallel()
		repoPath := filepath.Join(t.TempDir(), "other", "codex")
		if _, _, _, ok := DeriveRepoKey(catalog, repoPath); ok {
			t.Fatal("expected path outside root to be rejected")
		}
	})
}

func TestParseRepoKey(t *testing.T) {
	t.Parallel()

	catalog, rel, name, err := ParseRepoKey("software/openai/codex")
	if err != nil {
		t.Fatalf("ParseRepoKey returned error: %v", err)
	}
	if catalog != "software" || rel != "openai/codex" || name != "codex" {
		t.Fatalf("unexpected parse values catalog=%q rel=%q name=%q", catalog, rel, name)
	}

	if _, _, _, err := ParseRepoKey("software"); err == nil {
		t.Fatal("expected parse error for missing relative path")
	}
	if _, _, _, err := ParseRepoKey("software//codex"); err == nil {
		t.Fatal("expected parse error for malformed key")
	}
}
