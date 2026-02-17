package app

import (
	"strings"
	"testing"

	"bb-project/internal/domain"
)

func TestBuildRepoMoveIndex(t *testing.T) {
	t.Parallel()

	index, err := buildRepoMoveIndex([]domain.RepoMetadataFile{
		{RepoKey: "references/api", PreviousRepoKeys: []string{"software/api", "legacy/api", "software/api"}},
		{RepoKey: "software/web", PreviousRepoKeys: []string{"old/web"}},
	})
	if err != nil {
		t.Fatalf("buildRepoMoveIndex error: %v", err)
	}
	if got := index["software/api"]; got != "references/api" {
		t.Fatalf("index[software/api] = %q, want references/api", got)
	}
	if got := index["legacy/api"]; got != "references/api" {
		t.Fatalf("index[legacy/api] = %q, want references/api", got)
	}
	if got := index["old/web"]; got != "software/web" {
		t.Fatalf("index[old/web] = %q, want software/web", got)
	}
}

func TestBuildRepoMoveIndexRejectsAmbiguousOldKey(t *testing.T) {
	t.Parallel()

	_, err := buildRepoMoveIndex([]domain.RepoMetadataFile{
		{RepoKey: "references/api", PreviousRepoKeys: []string{"software/api"}},
		{RepoKey: "projects/api", PreviousRepoKeys: []string{"software/api"}},
	})
	if err == nil {
		t.Fatal("expected ambiguous previous_repo_keys error")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("unexpected error: %v", err)
	}
}
