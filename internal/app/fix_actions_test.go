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
			actions: []string{FixActionStageCommitPush, FixActionPublishNewBranch},
		},
		{
			name: "dirty diverged allows publish new branch",
			rec: func() domain.MachineRepoRecord {
				r := base
				r.HasDirtyTracked = true
				r.Diverged = true
				r.Ahead = 1
				r.Behind = 1
				return r
			}(),
			ctx:     fixEligibilityContext{},
			actions: []string{FixActionPublishNewBranch},
		},
		{
			name: "dirty behind branch offers checkpoint then sync",
			rec: func() domain.MachineRepoRecord {
				r := base
				r.HasDirtyTracked = true
				r.Behind = 1
				return r
			}(),
			ctx:     fixEligibilityContext{},
			actions: []string{FixActionCheckpointThenSync, FixActionPublishNewBranch},
		},
		{
			name:    "behind allows pull ff only",
			rec:     func() domain.MachineRepoRecord { r := base; r.Behind = 2; return r }(),
			ctx:     fixEligibilityContext{},
			actions: []string{FixActionPullFFOnly},
		},
		{
			name: "diverged clean allows sync with upstream when selected strategy is clean",
			rec: func() domain.MachineRepoRecord {
				r := base
				r.Ahead = 2
				r.Behind = 1
				r.Diverged = true
				return r
			}(),
			ctx: fixEligibilityContext{
				SyncStrategy: FixSyncStrategyRebase,
				SyncFeasibility: fixSyncFeasibility{
					Checked:       true,
					RebaseOutcome: fixSyncProbeClean,
				},
			},
			actions: []string{FixActionSyncWithUpstream},
		},
		{
			name: "diverged clean hides sync action when selected strategy would conflict",
			rec: func() domain.MachineRepoRecord {
				r := base
				r.Ahead = 2
				r.Behind = 1
				r.Diverged = true
				return r
			}(),
			ctx: fixEligibilityContext{
				SyncStrategy: FixSyncStrategyRebase,
				SyncFeasibility: fixSyncFeasibility{
					Checked:       true,
					RebaseOutcome: fixSyncProbeConflict,
					MergeOutcome:  fixSyncProbeClean,
				},
			},
			actions: []string{},
		},
		{
			name: "diverged probe failure blocks sync action",
			rec: func() domain.MachineRepoRecord {
				r := base
				r.Ahead = 2
				r.Behind = 1
				r.Diverged = true
				return r
			}(),
			ctx: fixEligibilityContext{
				SyncStrategy: FixSyncStrategyRebase,
				SyncFeasibility: fixSyncFeasibility{
					Checked:       true,
					RebaseOutcome: fixSyncProbeFailed,
					MergeOutcome:  fixSyncProbeConflict,
				},
			},
			actions: []string{},
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
			meta:    &domain.RepoMetadataFile{RepoKey: "software/api", AutoPush: domain.AutoPushModeDisabled},
			ctx:     fixEligibilityContext{},
			actions: []string{FixActionEnableAutoPush},
		},
		{
			name: "read-only push access blocks enable auto push action",
			rec:  base,
			meta: &domain.RepoMetadataFile{
				RepoKey:    "software/api",
				AutoPush:   domain.AutoPushModeDisabled,
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
				AutoPush:   domain.AutoPushModeDisabled,
				PushAccess: domain.PushAccessReadOnly,
			},
			ctx:     fixEligibilityContext{},
			actions: []string{FixActionForkAndRetarget},
		},
		{
			name: "default-branch push-policy-blocked with true mode offers enable auto push",
			rec: func() domain.MachineRepoRecord {
				r := base
				r.Ahead = 2
				r.UnsyncableReasons = []domain.UnsyncableReason{domain.ReasonPushPolicyBlocked}
				return r
			}(),
			meta: &domain.RepoMetadataFile{
				RepoKey:    "software/api",
				AutoPush:   domain.AutoPushModeEnabled,
				PushAccess: domain.PushAccessReadWrite,
			},
			ctx:     fixEligibilityContext{},
			actions: []string{FixActionPush, FixActionEnableAutoPush},
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
			actions: []string{FixActionStageCommitPush, FixActionPublishNewBranch},
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

func TestIneligibleFixReasonSyncProbeFailedBlocksSyncAction(t *testing.T) {
	t.Parallel()

	rec := domain.MachineRepoRecord{
		OriginURL: "git@github.com:you/api.git",
		Upstream:  "origin/main",
		Diverged:  true,
		Ahead:     1,
		Behind:    1,
	}
	reason := ineligibleFixReason(FixActionSyncWithUpstream, rec, fixEligibilityContext{
		SyncStrategy: FixSyncStrategyRebase,
		SyncFeasibility: fixSyncFeasibility{
			Checked:       true,
			RebaseOutcome: fixSyncProbeFailed,
			MergeOutcome:  fixSyncProbeConflict,
		},
	})
	if !strings.Contains(reason, string(domain.ReasonSyncProbeFailed)) {
		t.Fatalf("reason = %q, want to contain %q", reason, domain.ReasonSyncProbeFailed)
	}
}

func TestIneligibleFixReasonPublishNewBranch(t *testing.T) {
	t.Parallel()

	t.Run("operation in progress", func(t *testing.T) {
		t.Parallel()

		rec := domain.MachineRepoRecord{
			OperationInProgress: domain.OperationRebase,
		}
		reason := ineligibleFixReason(FixActionPublishNewBranch, rec, fixEligibilityContext{})
		if !strings.Contains(reason, "operation is in progress") {
			t.Fatalf("reason = %q, want operation-in-progress guidance", reason)
		}
	})

	t.Run("missing origin", func(t *testing.T) {
		t.Parallel()

		rec := domain.MachineRepoRecord{
			HasDirtyTracked: true,
		}
		reason := ineligibleFixReason(FixActionPublishNewBranch, rec, fixEligibilityContext{})
		if !strings.Contains(reason, "origin remote is required") {
			t.Fatalf("reason = %q, want missing-origin guidance", reason)
		}
	})

	t.Run("no local changes", func(t *testing.T) {
		t.Parallel()

		rec := domain.MachineRepoRecord{
			OriginURL: "git@github.com:you/api.git",
			Branch:    "main",
		}
		reason := ineligibleFixReason(FixActionPublishNewBranch, rec, fixEligibilityContext{})
		if !strings.Contains(reason, "no local uncommitted changes") {
			t.Fatalf("reason = %q, want no-changes guidance", reason)
		}
	})

	t.Run("secret-like changes", func(t *testing.T) {
		t.Parallel()

		rec := domain.MachineRepoRecord{
			OriginURL:       "git@github.com:you/api.git",
			Branch:          "main",
			HasDirtyTracked: true,
		}
		reason := ineligibleFixReason(FixActionPublishNewBranch, rec, fixEligibilityContext{
			Risk: fixRiskSnapshot{
				SecretLikeChangedPaths: []string{".env"},
			},
		})
		if !strings.Contains(reason, "secret-like uncommitted files") {
			t.Fatalf("reason = %q, want secret-like guidance", reason)
		}
	})

	t.Run("noisy changes in non-interactive mode", func(t *testing.T) {
		t.Parallel()

		rec := domain.MachineRepoRecord{
			OriginURL:    "git@github.com:you/api.git",
			Branch:       "main",
			HasUntracked: true,
		}
		reason := ineligibleFixReason(FixActionPublishNewBranch, rec, fixEligibilityContext{
			Interactive: false,
			Risk: fixRiskSnapshot{
				MissingRootGitignore: true,
				NoisyChangedPaths:    []string{"node_modules/pkg/index.js"},
			},
		})
		if !strings.Contains(reason, "root .gitignore is missing") {
			t.Fatalf("reason = %q, want .gitignore guidance", reason)
		}
	})
}
