package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
	raw := "version: 1\nrepo_key: references/netclode\nauto_push: 'false'\nbranch_follow_enabled: false\n"
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

func TestEnsureRepoMetadataNormalizesInvalidPushAccess(t *testing.T) {
	t.Parallel()

	paths := state.NewPaths(t.TempDir())
	a := New(paths, io.Discard, io.Discard)
	cfg := state.DefaultConfig()

	repoKey := "references/netclode"
	metaPath := state.RepoMetaPath(paths, repoKey)
	raw := "version: 1\nrepo_key: references/netclode\npush_access: admin\n"
	if err := os.MkdirAll(paths.RepoDir(), 0o755); err != nil {
		t.Fatalf("mkdir repo dir: %v", err)
	}
	if err := os.WriteFile(metaPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("write seed metadata: %v", err)
	}

	meta, _, err := a.ensureRepoMetadata(
		cfg,
		repoKey,
		"netclode",
		"git@github.com:you/netclode.git",
		domain.VisibilityPrivate,
		"references",
	)
	if err != nil {
		t.Fatalf("ensureRepoMetadata: %v", err)
	}
	if meta.PushAccess != domain.PushAccessUnknown {
		t.Fatalf("meta.PushAccess = %q, want %q", meta.PushAccess, domain.PushAccessUnknown)
	}
}

func TestRunRepoPolicyBlocksReadOnlyAutoPush(t *testing.T) {
	t.Parallel()

	paths := state.NewPaths(t.TempDir())
	a := New(paths, io.Discard, io.Discard)

	meta := domain.RepoMetadataFile{
		RepoKey:    "software/demo",
		Name:       "demo",
		AutoPush:   domain.AutoPushModeDisabled,
		PushAccess: domain.PushAccessReadOnly,
	}
	if err := state.SaveRepoMetadata(paths, meta); err != nil {
		t.Fatalf("save metadata: %v", err)
	}

	code, err := a.RunRepoPolicy("software/demo", domain.AutoPushModeEnabled)
	if err == nil {
		t.Fatal("expected error when enabling auto-push for read-only remote")
	}
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
}

func TestRunRepoPushAccessSet(t *testing.T) {
	t.Parallel()

	paths := state.NewPaths(t.TempDir())
	a := New(paths, io.Discard, io.Discard)
	now := time.Date(2026, time.February, 14, 15, 0, 0, 0, time.UTC)
	a.Now = func() time.Time { return now }

	meta := domain.RepoMetadataFile{
		RepoKey:    "software/demo",
		Name:       "demo",
		AutoPush:   domain.AutoPushModeEnabled,
		PushAccess: domain.PushAccessUnknown,
	}
	if err := state.SaveRepoMetadata(paths, meta); err != nil {
		t.Fatalf("save metadata: %v", err)
	}

	code, err := a.RunRepoPushAccessSet("software/demo", "read_only")
	if err != nil {
		t.Fatalf("RunRepoPushAccessSet error: %v", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	updated, err := state.LoadRepoMetadata(paths, "software/demo")
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if updated.PushAccess != domain.PushAccessReadOnly {
		t.Fatalf("push access = %q, want %q", updated.PushAccess, domain.PushAccessReadOnly)
	}
	if !updated.PushAccessManualOverride {
		t.Fatal("expected manual override to be enabled")
	}
	if updated.AutoPush != domain.AutoPushModeDisabled {
		t.Fatal("expected auto_push to be disabled for read-only push access")
	}
	if !updated.PushAccessCheckedAt.Equal(now) {
		t.Fatalf("checked_at = %s, want %s", updated.PushAccessCheckedAt, now)
	}
}

func TestRunRepoPushAccessSetRejectsInvalidValue(t *testing.T) {
	t.Parallel()

	paths := state.NewPaths(t.TempDir())
	a := New(paths, io.Discard, io.Discard)

	code, err := a.RunRepoPushAccessSet("software/demo", "admin")
	if err == nil {
		t.Fatal("expected invalid push access error")
	}
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
}

func TestShouldProbePushAccessForOrigin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		origin string
		want   bool
	}{
		{name: "github ssh", origin: "git@github.com:you/demo.git", want: true},
		{name: "github https", origin: "https://github.com/you/demo.git", want: true},
		{name: "github alias host", origin: "git@niieani.github.com:niieani/condu.git", want: true},
		{name: "file path remote", origin: "/tmp/remotes/you/demo.git", want: true},
		{name: "non github host", origin: "https://gitflic.ru/project/demo.git", want: false},
		{name: "empty", origin: "", want: false},
		{name: "invalid", origin: "::not-a-url::", want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := shouldProbePushAccessForOrigin(tt.origin); got != tt.want {
				t.Fatalf("shouldProbePushAccessForOrigin(%q) = %v, want %v", tt.origin, got, tt.want)
			}
		})
	}
}

func TestLoadRepoMetadataWithPushAccessSkipsProbeWhenNotRequested(t *testing.T) {
	t.Parallel()

	paths := state.NewPaths(t.TempDir())
	a := New(paths, io.Discard, io.Discard)
	repoKey := "software/demo"

	meta := domain.RepoMetadataFile{
		RepoKey:    repoKey,
		Name:       "demo",
		OriginURL:  "https://github.com/you/demo.git",
		PushAccess: domain.PushAccessUnknown,
	}
	if err := state.SaveRepoMetadata(paths, meta); err != nil {
		t.Fatalf("save metadata: %v", err)
	}

	loaded, hasMeta, err := a.loadRepoMetadataWithPushAccess("/tmp/does-not-matter", repoKey, meta.OriginURL, false)
	if err != nil {
		t.Fatalf("loadRepoMetadataWithPushAccess error: %v", err)
	}
	if !hasMeta {
		t.Fatal("expected metadata to be loaded")
	}
	if !loaded.PushAccessCheckedAt.IsZero() {
		t.Fatalf("expected no push-access probe when shouldProbe=false, checked_at=%s", loaded.PushAccessCheckedAt)
	}
}

func TestProbeAndUpdateRepoPushAccessUsesGitHubViewerPermission(t *testing.T) {
	t.Parallel()

	paths := state.NewPaths(t.TempDir())
	a := New(paths, io.Discard, io.Discard)
	now := time.Date(2026, time.February, 16, 9, 0, 0, 0, time.UTC)
	a.Now = func() time.Time { return now }

	repoPath := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}
	if err := a.Git.InitRepo(repoPath); err != nil {
		t.Fatalf("init repo: %v", err)
	}
	if err := a.Git.AddOrigin(repoPath, "git@niieani.github.com:acme/demo.git"); err != nil {
		t.Fatalf("add origin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	if err := a.Git.AddAll(repoPath); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if err := a.Git.Commit(repoPath, "init"); err != nil {
		t.Fatalf("git commit: %v", err)
	}
	if _, err := a.Git.RunGit(repoPath, "checkout", "--detach"); err != nil {
		t.Fatalf("detach head: %v", err)
	}

	var ghCalls int
	a.LookPath = func(file string) (string, error) {
		if file == "gh" {
			return "/usr/bin/gh", nil
		}
		return "", fmt.Errorf("unexpected executable lookup for %q", file)
	}
	a.RunCommand = func(name string, args ...string) (string, error) {
		if name != "gh" {
			return "", fmt.Errorf("unexpected command %q", name)
		}
		ghCalls++
		want := []string{"repo", "view", "acme/demo", "--json", "viewerPermission"}
		if len(args) != len(want) {
			t.Fatalf("gh args len=%d, want %d (%v)", len(args), len(want), args)
		}
		for i := range want {
			if args[i] != want[i] {
				t.Fatalf("gh arg[%d]=%q, want %q (args=%v)", i, args[i], want[i], args)
			}
		}
		return `{"viewerPermission":"READ"}`, nil
	}

	meta := domain.RepoMetadataFile{
		RepoKey:             "software/demo",
		Name:                "demo",
		OriginURL:           "git@niieani.github.com:acme/demo.git",
		PushAccess:          domain.PushAccessUnknown,
		BranchFollowEnabled: true,
	}

	updated, changed, err := a.probeAndUpdateRepoPushAccess(repoPath, meta.OriginURL, meta, true)
	if err != nil {
		t.Fatalf("probeAndUpdateRepoPushAccess error: %v", err)
	}
	if !changed {
		t.Fatal("expected metadata change from github viewer permission probe")
	}
	if ghCalls != 1 {
		t.Fatalf("gh call count=%d, want 1", ghCalls)
	}
	if updated.PushAccess != domain.PushAccessReadOnly {
		t.Fatalf("push access=%q, want %q", updated.PushAccess, domain.PushAccessReadOnly)
	}
	if strings.TrimSpace(updated.PushAccessCheckedRemote) != "origin" {
		t.Fatalf("checked_remote=%q, want origin", updated.PushAccessCheckedRemote)
	}
	if !updated.PushAccessCheckedAt.Equal(now) {
		t.Fatalf("checked_at=%s, want %s", updated.PushAccessCheckedAt, now)
	}
}

func TestParseGitHubViewerPermissionPushAccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want domain.PushAccess
		ok   bool
	}{
		{name: "read means read-only", raw: `{"viewerPermission":"READ"}`, want: domain.PushAccessReadOnly, ok: true},
		{name: "triage means read-only", raw: `{"viewerPermission":"TRIAGE"}`, want: domain.PushAccessReadOnly, ok: true},
		{name: "write means read-write", raw: `{"viewerPermission":"WRITE"}`, want: domain.PushAccessReadWrite, ok: true},
		{name: "admin means read-write", raw: `{"viewerPermission":"ADMIN"}`, want: domain.PushAccessReadWrite, ok: true},
		{name: "missing field", raw: `{}`, want: domain.PushAccessUnknown, ok: false},
		{name: "invalid json", raw: `not-json`, want: domain.PushAccessUnknown, ok: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := parseGitHubViewerPermissionPushAccess(tt.raw)
			if ok != tt.ok {
				t.Fatalf("ok=%v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("push_access=%q, want %q", got, tt.want)
			}
		})
	}
}
