package app

import (
	"strings"
	"testing"

	"bb-project/internal/domain"
)

func TestEligibleFixActions(t *testing.T) {
	t.Parallel()

	base := domain.MachineRepoRecord{
		RepoKey:             "software/api",
		Name:                "api",
		Path:                "/tmp/api",
		OriginURL:           "git@github.com:you/api.git",
		Branch:              "main",
		Upstream:            "origin/main",
		OperationInProgress: domain.OperationNone,
	}

	tests := []struct {
		name    string
		rec     domain.MachineRepoRecord
		meta    *domain.RepoMetadataFile
		ctx     fixEligibilityContext
		actions []string
	}{
		{
			name:    "operation in progress only allows abort",
			rec:     func() domain.MachineRepoRecord { r := base; r.OperationInProgress = domain.OperationRebase; return r }(),
			ctx:     fixEligibilityContext{},
			actions: []string{FixActionAbortOperation},
		},
		{
			name:    "ahead allows push",
			rec:     func() domain.MachineRepoRecord { r := base; r.Ahead = 1; return r }(),
			ctx:     fixEligibilityContext{},
			actions: []string{FixActionPush},
		},
		{
			name:    "dirty allows stage commit push",
			rec:     func() domain.MachineRepoRecord { r := base; r.HasDirtyTracked = true; return r }(),
			ctx:     fixEligibilityContext{},
			actions: []string{FixActionStageCommitPush},
		},
		{
			name:    "behind allows pull ff only",
			rec:     func() domain.MachineRepoRecord { r := base; r.Behind = 2; return r }(),
			ctx:     fixEligibilityContext{},
			actions: []string{FixActionPullFFOnly},
		},
		{
			name:    "missing upstream allows set upstream push",
			rec:     func() domain.MachineRepoRecord { r := base; r.Upstream = ""; r.Ahead = 2; return r }(),
			ctx:     fixEligibilityContext{},
			actions: []string{FixActionSetUpstreamPush},
		},
		{
			name: "missing origin allows create project",
			rec: func() domain.MachineRepoRecord {
				r := base
				r.OriginURL = ""
				r.Upstream = ""
				return r
			}(),
			ctx:     fixEligibilityContext{},
			actions: []string{FixActionCreateProject},
		},
		{
			name: "missing origin with dirty worktree allows create project and stage commit",
			rec: func() domain.MachineRepoRecord {
				r := base
				r.OriginURL = ""
				r.Upstream = ""
				r.HasUntracked = true
				return r
			}(),
			ctx:     fixEligibilityContext{},
			actions: []string{FixActionCreateProject, FixActionStageCommitPush},
		},
		{
			name:    "auto push disabled allows enable action",
			rec:     base,
			meta:    &domain.RepoMetadataFile{RepoKey: "software/api", AutoPush: false},
			ctx:     fixEligibilityContext{},
			actions: []string{FixActionEnableAutoPush},
		},
		{
			name: "read-only push access blocks enable auto push action",
			rec:  base,
			meta: &domain.RepoMetadataFile{
				RepoKey:    "software/api",
				AutoPush:   false,
				PushAccess: domain.PushAccessReadOnly,
			},
			ctx:     fixEligibilityContext{},
			actions: []string{FixActionForkAndRetarget},
		},
		{
			name: "read-only origin blocks push actions for dirty and ahead repos",
			rec: func() domain.MachineRepoRecord {
				r := base
				r.Ahead = 2
				r.HasDirtyTracked = true
				r.Upstream = "origin/main"
				return r
			}(),
			meta: &domain.RepoMetadataFile{
				RepoKey:    "software/api",
				AutoPush:   false,
				PushAccess: domain.PushAccessReadOnly,
			},
			ctx:     fixEligibilityContext{},
			actions: []string{FixActionForkAndRetarget},
		},
		{
			name: "secret-like uncommitted files block stage commit push",
			rec: func() domain.MachineRepoRecord {
				r := base
				r.HasDirtyTracked = true
				return r
			}(),
			ctx: fixEligibilityContext{
				Interactive: false,
				Risk: fixRiskSnapshot{
					SecretLikeChangedPaths: []string{".env"},
				},
			},
			actions: []string{},
		},
		{
			name: "noisy paths with missing root gitignore block stage commit push in non-interactive mode",
			rec: func() domain.MachineRepoRecord {
				r := base
				r.HasDirtyTracked = true
				return r
			}(),
			ctx: fixEligibilityContext{
				Interactive: false,
				Risk: fixRiskSnapshot{
					MissingRootGitignore: true,
					NoisyChangedPaths:    []string{"node_modules/pkg/index.js"},
				},
			},
			actions: []string{},
		},
		{
			name: "noisy paths do not block stage commit push in interactive mode",
			rec: func() domain.MachineRepoRecord {
				r := base
				r.HasDirtyTracked = true
				return r
			}(),
			ctx: fixEligibilityContext{
				Interactive: true,
				Risk: fixRiskSnapshot{
					MissingRootGitignore: true,
					NoisyChangedPaths:    []string{"node_modules/pkg/index.js"},
				},
			},
			actions: []string{FixActionStageCommitPush},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := eligibleFixActions(tt.rec, tt.meta, tt.ctx)
			for _, want := range tt.actions {
				if !containsAction(got, want) {
					t.Fatalf("expected action %q in %v", want, got)
				}
			}
			if len(got) != len(tt.actions) {
				t.Fatalf("actions = %v, want %v", got, tt.actions)
			}
			if len(tt.actions) == 0 && containsAction(got, FixActionStageCommitPush) {
				t.Fatalf("did not expect %q in %v", FixActionStageCommitPush, got)
			}
		})
	}
}

func TestResolveFixTarget(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{Record: domain.MachineRepoRecord{Name: "api", Path: "/repos/api", RepoKey: "software/api", OriginURL: "https://github.com/you/api.git"}},
		{Record: domain.MachineRepoRecord{Name: "web", Path: "/repos/web", RepoKey: "software/web", OriginURL: "https://github.com/you/web.git"}},
	}

	got, err := resolveFixTarget("/repos/api", repos)
	if err != nil {
		t.Fatalf("resolve by path failed: %v", err)
	}
	if got.Record.Name != "api" {
		t.Fatalf("resolved name = %q, want api", got.Record.Name)
	}

	got, err = resolveFixTarget("software/web", repos)
	if err != nil {
		t.Fatalf("resolve by repo_key failed: %v", err)
	}
	if got.Record.Name != "web" {
		t.Fatalf("resolved name = %q, want web", got.Record.Name)
	}

	got, err = resolveFixTarget("api", repos)
	if err != nil {
		t.Fatalf("resolve by name failed: %v", err)
	}
	if got.Record.Path != "/repos/api" {
		t.Fatalf("resolved path = %q, want /repos/api", got.Record.Path)
	}
}

func TestResolveFixTargetNameAmbiguous(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{Record: domain.MachineRepoRecord{Name: "api", Path: "/repos/api-a", RepoKey: "software/api-a", OriginURL: "https://github.com/you/api-a.git"}},
		{Record: domain.MachineRepoRecord{Name: "api", Path: "/repos/api-b", RepoKey: "software/api-b", OriginURL: "https://github.com/you/api-b.git"}},
	}

	_, err := resolveFixTarget("api", repos)
	if err == nil {
		t.Fatal("expected ambiguous selector error")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("unexpected error: %v", err)
	}
}
