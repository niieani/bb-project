package app

import (
	"fmt"
	"strings"

	"bb-project/internal/domain"
)

func validateGitHubRemoteURLTemplate(template string) error {
	template = strings.TrimSpace(template)
	if template == "" {
		return nil
	}
	if !strings.Contains(template, "${repo}") {
		return fmt.Errorf("github.preferred_remote_url_template must include ${repo}")
	}
	if !strings.Contains(template, "${org}") && !strings.Contains(template, "${owner}") {
		return fmt.Errorf("github.preferred_remote_url_template must include ${org} or ${owner}")
	}

	rendered, err := renderGitHubRemoteURLTemplate(template, "org", "repo")
	if err != nil {
		return err
	}
	if strings.TrimSpace(rendered) == "" {
		return fmt.Errorf("github.preferred_remote_url_template renders an empty URL")
	}
	return nil
}

func githubRemoteURL(owner string, repo string, protocol string, template string) (string, error) {
	owner = strings.TrimSpace(owner)
	repo = strings.TrimSpace(repo)
	if owner == "" {
		return "", fmt.Errorf("github owner is required")
	}
	if repo == "" {
		return "", fmt.Errorf("github repository is required")
	}

	template = strings.TrimSpace(template)
	if template != "" {
		return renderGitHubRemoteURLTemplate(template, owner, repo)
	}

	if strings.EqualFold(strings.TrimSpace(protocol), "https") {
		return fmt.Sprintf("https://github.com/%s/%s.git", owner, repo), nil
	}
	return fmt.Sprintf("git@github.com:%s/%s.git", owner, repo), nil
}

func renderGitHubRemoteURLTemplate(template string, owner string, repo string) (string, error) {
	template = strings.TrimSpace(template)
	if template == "" {
		return "", fmt.Errorf("github.preferred_remote_url_template is empty")
	}
	replacer := strings.NewReplacer(
		"${org}", owner,
		"${owner}", owner,
		"${repo}", repo,
	)
	rendered := strings.TrimSpace(replacer.Replace(template))
	if strings.Contains(rendered, "${") {
		return "", fmt.Errorf("github.preferred_remote_url_template contains an unsupported placeholder")
	}
	if rendered == "" {
		return "", fmt.Errorf("github.preferred_remote_url_template renders an empty URL")
	}
	return rendered, nil
}

func preferredGitHubRemoteURLForOrigin(cfg domain.GitHubConfig, originURL string) (string, bool, error) {
	owner, repo, ok := githubSourceRepoForOrigin(originURL)
	if !ok {
		return "", false, nil
	}
	url, err := githubRemoteURL(owner, repo, cfg.RemoteProtocol, cfg.PreferredRemoteURLTemplate)
	if err != nil {
		return "", true, err
	}
	return url, true, nil
}
