package app

import (
	"fmt"
	"strings"
	"time"

	"bb-project/internal/domain"
)

const remoteFormatVerifyTimeout = 15 * time.Second

func (a *App) alignRemoteFormatVerified(repoKey, path, remoteName, previousURL, preferredURL string) error {
	if strings.TrimSpace(remoteName) == "" {
		remoteName = "origin"
	}
	if err := a.Git.SetRemoteURL(path, remoteName, preferredURL); err != nil {
		return err
	}
	if err := a.Git.VerifyRemoteHeads(path, remoteName, remoteFormatVerifyTimeout); err != nil {
		if revertErr := a.Git.SetRemoteURL(path, remoteName, previousURL); revertErr != nil {
			failure := fmt.Errorf("remote format verification failed: %w; revert failed: %v", err, revertErr)
			a.appendJournal("remote_align_reverted", repoKey, failure.Error())
			return failure
		}
		failure := fmt.Errorf("remote format verification failed; previous URL restored: %w", err)
		a.appendJournal("remote_align_reverted", repoKey, failure.Error())
		return failure
	}
	a.appendJournal("remote_aligned", repoKey, fmt.Sprintf("%s remote updated", remoteName))
	return nil
}

func (a *App) alignRemoteFormatsBeforeObservation(cfg domain.ConfigFile, catalogs []domain.Catalog, dryRun bool) error {
	if !cfg.Sync.AutoAlignRemoteFormat || dryRun {
		return nil
	}
	repos, err := discoverRepos(catalogs)
	if err != nil {
		return err
	}
	for _, repo := range repos {
		origin, err := a.Git.RemoteURLRaw(repo.Path, "origin")
		if err != nil || strings.TrimSpace(origin) == "" {
			continue
		}
		expected, isGitHub, err := preferredGitHubRemoteURLForOrigin(cfg.GitHub, origin)
		if err != nil {
			return err
		}
		if !isGitHub || strings.TrimSpace(expected) == "" || strings.TrimSpace(expected) == strings.TrimSpace(origin) {
			continue
		}
		if err := a.alignRemoteFormatVerified(repo.RepoKey, repo.Path, "origin", origin, expected); err != nil {
			a.logf("sync: remote format alignment reverted for %s: %v", repo.Path, err)
		}
	}
	return nil
}
