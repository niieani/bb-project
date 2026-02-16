package app

import (
	"fmt"
	"os"
	"strings"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func (a *App) runInfo(opts InfoOptions) (int, error) {
	cfg, machine, err := a.loadContext()
	if err != nil {
		return 2, err
	}

	selector := strings.TrimSpace(opts.Selector)
	rec, found, err := a.resolveProjectOrRepoSelector(cfg, &machine, selector, resolveProjectOrRepoSelectorOptions{AllowClone: false})
	if err != nil {
		return 2, err
	}
	if !found {
		fmt.Fprintf(a.Stdout, "project or repository %q not found locally (not cloned or not present in machine projects list)\n", selector)
		return 1, nil
	}
	if strings.TrimSpace(rec.Path) == "" || !a.Git.IsGitRepo(rec.Path) {
		fmt.Fprintf(a.Stdout, "project or repository %q is not cloned locally at %s\n", selector, valueOrDash(rec.Path))
		return 1, nil
	}

	printProjectInfo(a.Stdout, a.Paths, rec)
	return 0, nil
}

func printProjectInfo(stdout anyWriter, paths state.Paths, rec domain.MachineRepoRecord) {
	fmt.Fprintf(stdout, "Project: %s\n", valueOrDash(rec.Name))
	fmt.Fprintf(stdout, "Repo Key: %s\n", valueOrDash(rec.RepoKey))
	fmt.Fprintf(stdout, "Catalog: %s\n", valueOrDash(rec.Catalog))
	fmt.Fprintf(stdout, "Path: %s\n", valueOrDash(rec.Path))
	fmt.Fprintf(stdout, "Origin: %s\n", valueOrDash(rec.OriginURL))
	fmt.Fprintf(stdout, "Branch: %s\n", valueOrDash(rec.Branch))
	fmt.Fprintf(stdout, "Upstream: %s\n", valueOrDash(rec.Upstream))
	fmt.Fprintf(stdout, "Ahead/Behind: %d/%d\n", rec.Ahead, rec.Behind)
	fmt.Fprintf(stdout, "Dirty: tracked=%s untracked=%s\n", onOffLabel(rec.HasDirtyTracked), onOffLabel(rec.HasUntracked))
	fmt.Fprintf(stdout, "Syncable: %s\n", yesNo(rec.Syncable))

	if strings.TrimSpace(rec.RepoKey) == "" {
		return
	}
	meta, err := state.LoadRepoMetadata(paths, rec.RepoKey)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(stdout, "Metadata: unavailable (%v)\n", err)
		}
		return
	}
	fmt.Fprintf(stdout, "Visibility: %s\n", valueOrDash(string(meta.Visibility)))
	fmt.Fprintf(stdout, "Preferred Remote: %s\n", valueOrDash(meta.PreferredRemote))
	fmt.Fprintf(stdout, "Auto Push: %s\n", valueOrDash(string(meta.AutoPush)))
	fmt.Fprintf(stdout, "Push Access: %s\n", valueOrDash(string(meta.PushAccess)))
}

type anyWriter interface {
	Write([]byte) (int, error)
}

func valueOrDash(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "-"
	}
	return v
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
