package app

import (
	"fmt"
	"strings"

	"bb-project/internal/domain"
)

type fixActionSpec struct {
	Label       string
	Description string
	Risky       bool
	BuildPlan   func(ctx fixActionPlanContext) []fixActionPlanEntry
}

type fixActionPlanContext struct {
	Operation               domain.Operation
	Branch                  string
	Upstream                string
	HeadSHA                 string
	OriginURL               string
	SyncStrategy            FixSyncStrategy
	PreferredRemote         string
	GitHubOwner             string
	RemoteProtocol          string
	ForkRemoteExists        bool
	RepoName                string
	CommitMessage           string
	CreateProjectName       string
	CreateProjectVisibility domain.Visibility
	GenerateGitignore       bool
	GitignorePatterns       []string
	MissingRootGitignore    bool
	FetchPrune              bool
}

type fixActionPlanEntry struct {
	ID      string
	Command bool
	Summary string
}

const fixActionPlanRevalidateStateID = "revalidate-state"

var fixActionSpecs = map[string]fixActionSpec{
	FixActionIgnore: {
		Label:       "Ignore for this session",
		Description: "Hide this repository from the current interactive fix run without changing files.",
		Risky:       false,
		BuildPlan:   planFixActionIgnore,
	},
	FixActionAbortOperation: {
		Label:       "Abort operation",
		Description: "Cancel the active git operation (merge, rebase, cherry-pick, or bisect).",
		Risky:       true,
		BuildPlan:   planFixActionAbortOperation,
	},
	FixActionCreateProject: {
		Label:       "Create project & push",
		Description: "Create remote project, set origin, register metadata, and push current branch.",
		Risky:       true,
		BuildPlan:   planFixActionCreateProject,
	},
	FixActionForkAndRetarget: {
		Label:       "Fork & retarget remote",
		Description: "Fork the upstream repository, add your fork as a remote, retarget this branch upstream, and update repo metadata.",
		Risky:       true,
		BuildPlan:   planFixActionForkAndRetarget,
	},
	FixActionSyncWithUpstream: {
		Label:       "Sync with upstream",
		Description: "Integrate upstream commits into your local branch using the selected sync strategy (rebase by default).",
		Risky:       true,
		BuildPlan:   planFixActionSyncWithUpstream,
	},
	FixActionPush: {
		Label:       "Push commits",
		Description: "Push local commits that are ahead of upstream.",
		Risky:       true,
		BuildPlan:   planFixActionPush,
	},
	FixActionStageCommitPush: {
		Label:       "Stage, commit & push",
		Description: "Stage all local changes and create a commit; push when a remote target is configured.",
		Risky:       true,
		BuildPlan:   planFixActionStageCommitPush,
	},
	FixActionCheckpointThenSync: {
		Label:       "Checkpoint, sync & push",
		Description: "Stage and commit local changes, sync with upstream, then push the integrated result.",
		Risky:       true,
		BuildPlan:   planFixActionCheckpointThenSync,
	},
	FixActionPullFFOnly: {
		Label:       "Pull (ff-only)",
		Description: "Fast-forward your branch to upstream without creating a merge commit.",
		Risky:       false,
		BuildPlan:   planFixActionPullFFOnly,
	},
	FixActionSetUpstreamPush: {
		Label:       "Set upstream & push",
		Description: "Set this branch's upstream tracking target and push.",
		Risky:       true,
		BuildPlan:   planFixActionSetUpstreamPush,
	},
	FixActionEnableAutoPush: {
		Label:       "Allow auto-push in sync",
		Description: "Allow future bb sync runs to auto-push this repo by enabling its auto-push policy.",
		Risky:       false,
		BuildPlan:   planFixActionEnableAutoPush,
	},
}

func fixActionSpecFor(action string) (fixActionSpec, bool) {
	spec, ok := fixActionSpecs[action]
	return spec, ok
}

func fixActionPlanFor(action string, ctx fixActionPlanContext) []fixActionPlanEntry {
	spec, ok := fixActionSpecFor(action)
	if !ok || spec.BuildPlan == nil {
		return nil
	}
	return spec.BuildPlan(ctx)
}

func fixActionExecutionPlanFor(action string, ctx fixActionPlanContext) []fixActionPlanEntry {
	entries := append([]fixActionPlanEntry(nil), fixActionPlanFor(action, ctx)...)
	if len(entries) == 0 {
		return nil
	}
	entries = append(entries, fixActionPlanEntry{
		ID:      fixActionPlanRevalidateStateID,
		Command: false,
		Summary: "Revalidate repository status and syncability state.",
	})
	return entries
}

func planFixActionIgnore(_ fixActionPlanContext) []fixActionPlanEntry {
	return []fixActionPlanEntry{
		{
			ID:      "ignore-session",
			Command: false,
			Summary: "Ignore this repository in the current interactive session only (no file changes).",
		},
	}
}

func planFixActionAbortOperation(ctx fixActionPlanContext) []fixActionPlanEntry {
	switch ctx.Operation {
	case domain.OperationMerge:
		return []fixActionPlanEntry{{ID: "abort-merge", Command: true, Summary: "git merge --abort"}}
	case domain.OperationRebase:
		return []fixActionPlanEntry{{ID: "abort-rebase", Command: true, Summary: "git rebase --abort"}}
	case domain.OperationCherryPick:
		return []fixActionPlanEntry{{ID: "abort-cherry-pick", Command: true, Summary: "git cherry-pick --abort"}}
	case domain.OperationBisect:
		return []fixActionPlanEntry{{ID: "abort-bisect", Command: true, Summary: "git bisect reset"}}
	default:
		return []fixActionPlanEntry{
			{ID: "abort-noop", Command: false, Summary: "No merge/rebase/cherry-pick/bisect operation is currently active."},
		}
	}
}

func planFixActionPush(_ fixActionPlanContext) []fixActionPlanEntry {
	return []fixActionPlanEntry{
		{ID: "push-main", Command: true, Summary: "git push"},
	}
}

func planFixActionSyncWithUpstream(ctx fixActionPlanContext) []fixActionPlanEntry {
	upstream := plannedUpstream(ctx.Upstream)
	entries := make([]fixActionPlanEntry, 0, 2)
	if ctx.FetchPrune {
		entries = append(entries, fixActionPlanEntry{ID: "sync-fetch-prune", Command: true, Summary: "git fetch --prune"})
	} else {
		entries = append(entries, fixActionPlanEntry{
			ID:      "sync-fetch-prune",
			Command: false,
			Summary: "Skip fetch prune because sync.fetch_prune is disabled.",
		})
	}
	if normalizeFixSyncStrategy(ctx.SyncStrategy) == FixSyncStrategyMerge {
		entries = append(entries, fixActionPlanEntry{ID: "sync-merge", Command: true, Summary: fmt.Sprintf("git merge --no-edit %s", upstream)})
		return entries
	}
	entries = append(entries, fixActionPlanEntry{ID: "sync-rebase", Command: true, Summary: fmt.Sprintf("git rebase %s", upstream)})
	return entries
}

func planFixActionStageCommitPush(ctx fixActionPlanContext) []fixActionPlanEntry {
	entries := make([]fixActionPlanEntry, 0, 4)
	if ctx.GenerateGitignore && len(ctx.GitignorePatterns) > 0 {
		n := len(ctx.GitignorePatterns)
		if ctx.MissingRootGitignore {
			entries = append(entries, fixActionPlanEntry{
				ID:      "stage-gitignore-generate",
				Command: false,
				Summary: fmt.Sprintf("Generate root .gitignore with %d selected pattern(s).", n),
			})
		} else {
			entries = append(entries, fixActionPlanEntry{
				ID:      "stage-gitignore-append",
				Command: false,
				Summary: fmt.Sprintf("Append %d selected pattern(s) to root .gitignore.", n),
			})
		}
	}

	msg := plannedCommitMessage(ctx.CommitMessage)
	entries = append(entries, fixActionPlanEntry{ID: "stage-git-add", Command: true, Summary: "git add -A"})
	entries = append(entries, fixActionPlanEntry{ID: "stage-git-commit", Command: true, Summary: fmt.Sprintf("git commit -m %q", msg)})

	if strings.TrimSpace(ctx.OriginURL) == "" {
		entries = append(entries, fixActionPlanEntry{
			ID:      "stage-skip-push-no-origin",
			Command: false,
			Summary: "Skip push because no origin remote is configured.",
		})
		return entries
	}
	if strings.TrimSpace(ctx.Upstream) == "" {
		entries = append(entries, fixActionPlanEntry{
			ID:      "stage-push-set-upstream",
			Command: true,
			Summary: fmt.Sprintf("git push -u %s %s", plannedRemote(ctx.PreferredRemote, ctx.Upstream), plannedBranch(ctx.Branch)),
		})
		return entries
	}
	entries = append(entries, fixActionPlanEntry{ID: "stage-push", Command: true, Summary: "git push"})
	return entries
}

func planFixActionCheckpointThenSync(ctx fixActionPlanContext) []fixActionPlanEntry {
	stageEntries := planFixActionStageCommitPush(ctx)
	stageOnly := make([]fixActionPlanEntry, 0, len(stageEntries))
	for _, entry := range stageEntries {
		switch entry.ID {
		case "stage-skip-push-no-origin", "stage-push-set-upstream", "stage-push":
			continue
		default:
			stageOnly = append(stageOnly, entry)
		}
	}
	entries := make([]fixActionPlanEntry, 0, len(stageOnly)+4)
	entries = append(entries, stageOnly...)
	entries = append(entries, planFixActionSyncWithUpstream(ctx)...)
	entries = append(entries, fixActionPlanEntry{
		ID:      "checkpoint-push",
		Command: true,
		Summary: "git push",
	})
	return entries
}

func planFixActionPullFFOnly(ctx fixActionPlanContext) []fixActionPlanEntry {
	entries := make([]fixActionPlanEntry, 0, 2)
	if ctx.FetchPrune {
		entries = append(entries, fixActionPlanEntry{ID: "pull-fetch-prune", Command: true, Summary: "git fetch --prune"})
	} else {
		entries = append(entries, fixActionPlanEntry{
			ID:      "pull-fetch-prune",
			Command: false,
			Summary: "Skip fetch prune because sync.fetch_prune is disabled.",
		})
	}
	entries = append(entries, fixActionPlanEntry{ID: "pull-ff-only", Command: true, Summary: "git pull --ff-only"})
	return entries
}

func planFixActionSetUpstreamPush(ctx fixActionPlanContext) []fixActionPlanEntry {
	return []fixActionPlanEntry{
		{
			ID:      "upstream-push",
			Command: true,
			Summary: fmt.Sprintf("git push -u %s %s", plannedRemote(ctx.PreferredRemote, ctx.Upstream), plannedBranch(ctx.Branch)),
		},
	}
}

func planFixActionCreateProject(ctx fixActionPlanContext) []fixActionPlanEntry {
	entries := make([]fixActionPlanEntry, 0, 5)
	projectName := plannedProjectName(ctx.CreateProjectName, ctx.RepoName)
	owner := strings.TrimSpace(ctx.GitHubOwner)
	if owner == "" {
		return []fixActionPlanEntry{
			{
				ID:      "create-requires-owner",
				Command: false,
				Summary: "Configure github.owner before creating a GitHub project.",
			},
		}
	}
	entries = append(entries, fixActionPlanEntry{
		ID:      "create-gh-repo",
		Command: true,
		Summary: fmt.Sprintf("gh repo create %s/%s %s", owner, projectName, plannedVisibilityFlag(ctx.CreateProjectVisibility)),
	})
	if strings.TrimSpace(ctx.OriginURL) == "" {
		originURL := plannedOriginURL(ctx.GitHubOwner, projectName, ctx.RemoteProtocol)
		if strings.TrimSpace(originURL) == "" {
			return append(entries, fixActionPlanEntry{
				ID:      "create-add-origin",
				Command: false,
				Summary: "Cannot derive origin URL for the configured GitHub owner/protocol.",
			})
		}
		entries = append(entries, fixActionPlanEntry{
			ID:      "create-add-origin",
			Command: true,
			Summary: fmt.Sprintf("git remote add origin %s", originURL),
		})
	} else {
		entries = append(entries, fixActionPlanEntry{
			ID:      "create-validate-origin",
			Command: false,
			Summary: "Validate existing origin URL matches the expected repository identity.",
		})
	}
	entries = append(entries, fixActionPlanEntry{
		ID:      "create-write-metadata",
		Command: false,
		Summary: "Write/update repo metadata (origin URL, visibility, default auto-push policy).",
	})
	if strings.TrimSpace(ctx.Upstream) == "" {
		if strings.TrimSpace(ctx.HeadSHA) != "" {
			entries = append(entries, fixActionPlanEntry{
				ID:      "create-initial-push",
				Command: true,
				Summary: fmt.Sprintf("git push -u %s %s", plannedRemote(ctx.PreferredRemote, ctx.Upstream), plannedBranch(ctx.Branch)),
			})
		} else {
			entries = append(entries, fixActionPlanEntry{
				ID:      "create-initial-push",
				Command: false,
				Summary: "Skip initial push because HEAD has no commits.",
			})
		}
	}
	return entries
}

func planFixActionForkAndRetarget(ctx fixActionPlanContext) []fixActionPlanEntry {
	owner := strings.TrimSpace(ctx.GitHubOwner)
	if owner == "" {
		return []fixActionPlanEntry{
			{
				ID:      "fork-requires-owner",
				Command: false,
				Summary: "Configure github.owner before forking and retargeting.",
			},
		}
	}
	source := plannedForkSource(ctx.OriginURL)
	if source == "" {
		return []fixActionPlanEntry{
			{
				ID:      "fork-source-invalid",
				Command: false,
				Summary: "Cannot derive GitHub source repository from origin URL.",
			},
		}
	}
	forkURL := plannedForkURL(ctx.OriginURL, ctx.GitHubOwner, ctx.RemoteProtocol)
	if forkURL == "" {
		return []fixActionPlanEntry{
			{
				ID:      "fork-url-invalid",
				Command: false,
				Summary: "Cannot derive fork remote URL from GitHub owner/protocol.",
			},
		}
	}
	setRemoteCmd := fmt.Sprintf("git remote add %s %s", owner, forkURL)
	if ctx.ForkRemoteExists {
		setRemoteCmd = fmt.Sprintf("git remote set-url %s %s", owner, forkURL)
	}
	return []fixActionPlanEntry{
		{
			ID:      "fork-gh-fork",
			Command: true,
			Summary: fmt.Sprintf("gh repo fork %s --remote=false --clone=false", source),
		},
		{
			ID:      "fork-set-remote",
			Command: true,
			Summary: setRemoteCmd,
		},
		{
			ID:      "fork-write-metadata",
			Command: false,
			Summary: "Update repo metadata immediately after retargeting remote (preferred remote and push-access probe state reset).",
		},
		{
			ID:      "fork-push-upstream",
			Command: true,
			Summary: fmt.Sprintf("git push -u --force %s %s", owner, plannedBranch(ctx.Branch)),
		},
		{
			ID:      "fork-refresh-metadata",
			Command: false,
			Summary: "Refresh repo metadata push-access probe state after retarget push.",
		},
	}
}

func planFixActionEnableAutoPush(_ fixActionPlanContext) []fixActionPlanEntry {
	return []fixActionPlanEntry{
		{
			ID:      "enable-auto-push",
			Command: false,
			Summary: "Write repo metadata: set auto_push to the enabled mode for this branch.",
		},
	}
}

func plannedCommitMessage(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "auto" {
		return DefaultFixCommitMessage
	}
	return raw
}

func plannedRemote(preferredRemote string, upstream string) string {
	if remote, _, ok := strings.Cut(strings.TrimSpace(upstream), "/"); ok && strings.TrimSpace(remote) != "" {
		return strings.TrimSpace(remote)
	}
	if trimmed := strings.TrimSpace(preferredRemote); trimmed != "" {
		return trimmed
	}
	return "origin"
}

func plannedBranch(branch string) string {
	if trimmed := strings.TrimSpace(branch); trimmed != "" {
		return trimmed
	}
	return "HEAD"
}

func plannedUpstream(upstream string) string {
	if trimmed := strings.TrimSpace(upstream); trimmed != "" {
		return trimmed
	}
	return "@{u}"
}

func plannedProjectName(name string, repoName string) string {
	if sanitized := sanitizeGitHubRepositoryNameInput(name); sanitized != "" {
		return sanitized
	}
	if fallback := sanitizeGitHubRepositoryNameInput(repoName); fallback != "" {
		return fallback
	}
	return "repo"
}

func plannedOriginURL(owner string, projectName string, protocol string) string {
	owner = strings.TrimSpace(owner)
	projectName = strings.TrimSpace(projectName)
	if owner == "" || projectName == "" {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(protocol), "https") {
		return fmt.Sprintf("https://github.com/%s/%s.git", owner, projectName)
	}
	return fmt.Sprintf("git@github.com:%s/%s.git", owner, projectName)
}

func plannedVisibilityFlag(visibility domain.Visibility) string {
	if visibility == domain.VisibilityPublic {
		return "--public"
	}
	return "--private"
}

func plannedForkSource(originURL string) string {
	sourceOwner, repoName, err := sourceRepoForFork(originURL)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%s/%s", sourceOwner, repoName)
}

func plannedForkURL(originURL string, forkOwner string, protocol string) string {
	forkOwner = strings.TrimSpace(forkOwner)
	if forkOwner == "" {
		return ""
	}
	_, repoName, err := sourceRepoForFork(originURL)
	if err != nil {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(protocol), "https") {
		return fmt.Sprintf("https://github.com/%s/%s.git", forkOwner, repoName)
	}
	return fmt.Sprintf("git@github.com:%s/%s.git", forkOwner, repoName)
}
