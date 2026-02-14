package app

import (
	"io"
	"os"
	"testing"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func TestEnsureRepoMetadataSkipsUnchangedWrite(t *testing.T) {
	t.Parallel()

	paths := state.NewPaths(t.TempDir())
	a := New(paths, io.Discard, io.Discard)
	cfg := state.DefaultConfig()

	repoKey := "references/netclode"
	name := "netclode"
	origin := "git@github.com:you/netclode.git"
	visibility := domain.VisibilityPrivate
	preferredCatalog := "references"

	_, created, err := a.ensureRepoMetadata(cfg, repoKey, name, origin, visibility, preferredCatalog)
	if err != nil {
		t.Fatalf("first ensureRepoMetadata: %v", err)
	}
	if !created {
		t.Fatal("expected first ensureRepoMetadata call to create metadata")
	}

	metaPath := state.RepoMetaPath(paths, repoKey)
	old := time.Date(2001, time.January, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(metaPath, old, old); err != nil {
		t.Fatalf("set old modtime: %v", err)
	}

	_, created, err = a.ensureRepoMetadata(cfg, repoKey, name, origin, visibility, preferredCatalog)
	if err != nil {
		t.Fatalf("second ensureRepoMetadata: %v", err)
	}
	if created {
		t.Fatal("expected second ensureRepoMetadata call to reuse existing metadata")
	}

	info, err := os.Stat(metaPath)
	if err != nil {
		t.Fatalf("stat repo metadata: %v", err)
	}
	if !info.ModTime().Equal(old) {
		t.Fatalf("metadata modtime changed on no-op ensure: got %s want %s", info.ModTime().UTC(), old)
	}
}

func TestEnsureRepoMetadataWritesWhenBackfillNeeded(t *testing.T) {
	t.Parallel()

	paths := state.NewPaths(t.TempDir())
	a := New(paths, io.Discard, io.Discard)
	cfg := state.DefaultConfig()

	repoKey := "references/netclode"
	metaPath := state.RepoMetaPath(paths, repoKey)
	raw := "version: 1\nrepo_key: references/netclode\nauto_push: false\nbranch_follow_enabled: false\n"
	if err := os.MkdirAll(paths.RepoDir(), 0o755); err != nil {
		t.Fatalf("mkdir repo dir: %v", err)
	}
	if err := os.WriteFile(metaPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("write seed metadata: %v", err)
	}

	old := time.Date(2001, time.January, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(metaPath, old, old); err != nil {
		t.Fatalf("set old modtime: %v", err)
	}

	name := "netclode"
	origin := "git@github.com:you/netclode.git"
	visibility := domain.VisibilityPrivate
	preferredCatalog := "references"
	meta, created, err := a.ensureRepoMetadata(cfg, repoKey, name, origin, visibility, preferredCatalog)
	if err != nil {
		t.Fatalf("ensureRepoMetadata: %v", err)
	}
	if created {
		t.Fatal("expected ensureRepoMetadata to update existing metadata, not create")
	}
	if meta.Name != name {
		t.Fatalf("meta.Name = %q, want %q", meta.Name, name)
	}
	if meta.OriginURL != origin {
		t.Fatalf("meta.OriginURL = %q, want %q", meta.OriginURL, origin)
	}
	if meta.Visibility != visibility {
		t.Fatalf("meta.Visibility = %q, want %q", meta.Visibility, visibility)
	}
	if meta.PreferredCatalog != preferredCatalog {
		t.Fatalf("meta.PreferredCatalog = %q, want %q", meta.PreferredCatalog, preferredCatalog)
	}

	info, err := os.Stat(metaPath)
	if err != nil {
		t.Fatalf("stat repo metadata: %v", err)
	}
	if !info.ModTime().After(old) {
		t.Fatalf("metadata modtime did not advance after backfill write: got %s old %s", info.ModTime().UTC(), old)
	}
}
