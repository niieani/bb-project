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
	OriginURL               string
	SyncStrategy            FixSyncStrategy
	PreferredRemote         string
	GitHubOwner             string
	RemoteProtocol          string
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
			{ID: "abort-merge-conditional", Command: true, Summary: "git merge --abort (when merge is in progress)"},
			{ID: "abort-rebase-conditional", Command: true, Summary: "git rebase --abort (when rebase is in progress)"},
			{ID: "abort-cherry-pick-conditional", Command: true, Summary: "git cherry-pick --abort (when cherry-pick is in progress)"},
			{ID: "abort-bisect-conditional", Command: true, Summary: "git bisect reset (when bisect is in progress)"},
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
	entries := []fixActionPlanEntry{
		{ID: "sync-fetch-prune", Command: true, Summary: "git fetch --prune (if sync.fetch_prune is enabled)"},
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

func planFixActionPullFFOnly(_ fixActionPlanContext) []fixActionPlanEntry {
	return []fixActionPlanEntry{
		{ID: "pull-fetch-prune", Command: true, Summary: "git fetch --prune (if sync.fetch_prune is enabled)"},
		{ID: "pull-ff-only", Command: true, Summary: "git pull --ff-only"},
	}
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
	owner := plannedGitHubOwner(ctx.GitHubOwner)
	entries = append(entries, fixActionPlanEntry{
		ID:      "create-gh-repo",
		Command: true,
		Summary: fmt.Sprintf("gh repo create %s/%s %s", owner, projectName, plannedVisibilityFlag(ctx.CreateProjectVisibility)),
	})
	if strings.TrimSpace(ctx.OriginURL) == "" {
		originURL := plannedOriginURL(ctx.GitHubOwner, projectName, ctx.RemoteProtocol)
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
		entries = append(entries, fixActionPlanEntry{
			ID:      "create-initial-push",
			Command: true,
			Summary: fmt.Sprintf("git push -u %s %s (when HEAD has commits)", plannedRemote(ctx.PreferredRemote, ctx.Upstream), plannedBranch(ctx.Branch)),
		})
	}
	return entries
}

func planFixActionForkAndRetarget(ctx fixActionPlanContext) []fixActionPlanEntry {
	owner := plannedGitHubOwner(ctx.GitHubOwner)
	return []fixActionPlanEntry{
		{
			ID:      "fork-gh-fork",
			Command: true,
			Summary: "gh repo fork <source-owner>/<repo> --remote=false --clone=false",
		},
		{
			ID:      "fork-set-remote",
			Command: true,
			Summary: fmt.Sprintf("git remote add %s <fork-url> (or git remote set-url when that remote already exists)", owner),
		},
		{
			ID:      "fork-push-upstream",
			Command: true,
			Summary: fmt.Sprintf("git push -u %s %s", owner, plannedBranch(ctx.Branch)),
		},
		{
			ID:      "fork-write-metadata",
			Command: false,
			Summary: "Update repo metadata (preferred remote and push-access probe state).",
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
	return "<current-branch>"
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
	return "<repository-name>"
}

func plannedGitHubOwner(owner string) string {
	if trimmed := strings.TrimSpace(owner); trimmed != "" {
		return trimmed
	}
	return "<github.owner>"
}

func plannedOriginURL(owner string, projectName string, protocol string) string {
	owner = strings.TrimSpace(owner)
	projectName = strings.TrimSpace(projectName)
	if owner == "" || projectName == "" || strings.HasPrefix(owner, "<") || strings.HasPrefix(projectName, "<") {
		return "<new-origin-url>"
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
