package domain

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
)

var scpLikeOriginPattern = regexp.MustCompile(`^([^@\s]+)@([^:\s]+):(.+)$`)

func NormalizeOriginIdentity(origin string) (string, error) {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return "", errors.New("empty origin")
	}

	if matches := scpLikeOriginPattern.FindStringSubmatch(origin); matches != nil {
		host := strings.ToLower(strings.TrimSpace(matches[2]))
		repoPath := normalizeRepoPath(matches[3])
		if host == "" || repoPath == "" {
			return "", fmt.Errorf("invalid origin %q", origin)
		}
		return host + "/" + repoPath, nil
	}

	if strings.HasPrefix(origin, "/") {
		n := normalizeRepoPath(origin)
		if n == "" {
			return "", fmt.Errorf("invalid origin path %q", origin)
		}
		return "file/" + n, nil
	}

	u, err := url.Parse(origin)
	if err != nil {
		return "", fmt.Errorf("parse origin: %w", err)
	}

	if u.Scheme == "" && u.Host == "" {
		return "", fmt.Errorf("invalid origin %q", origin)
	}

	host := strings.ToLower(strings.TrimSpace(u.Host))
	repoPath := normalizeRepoPath(u.Path)
	if u.Scheme == "file" {
		repoPath = normalizeRepoPath(strings.TrimPrefix(origin, "file://"))
		if repoPath == "" {
			repoPath = normalizeRepoPath(u.Path)
		}
		return "file/" + repoPath, nil
	}
	if host == "" || repoPath == "" {
		return "", fmt.Errorf("invalid origin %q", origin)
	}

	return host + "/" + repoPath, nil
}

func normalizeRepoPath(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "/")
	raw = strings.TrimSuffix(raw, "/")
	raw = strings.TrimSuffix(raw, ".git")
	raw = path.Clean(raw)
	if raw == "." || raw == "" {
		return ""
	}
	raw = strings.TrimPrefix(raw, "./")
	raw = strings.TrimPrefix(raw, "/")
	return strings.ToLower(raw)
}
