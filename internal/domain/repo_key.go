package domain

import (
	"fmt"
	"path/filepath"
	"strings"
)

const DefaultRepoPathDepth = 1

func EffectiveRepoPathDepth(c Catalog) int {
	if c.RepoPathDepth == 2 {
		return 2
	}
	return DefaultRepoPathDepth
}

func ValidateRepoPathDepth(depth int) error {
	if depth == 0 || depth == 1 || depth == 2 {
		return nil
	}
	return fmt.Errorf("repo_path_depth must be 1 or 2")
}

func DeriveRepoKey(catalog Catalog, repoPath string) (repoKey string, relativePath string, repoName string, ok bool) {
	relativePath, ok = relativePathUnderRoot(repoPath, catalog.Root)
	if !ok {
		return "", "", "", false
	}
	return DeriveRepoKeyFromRelative(catalog, relativePath)
}

func DeriveRepoKeyFromRelative(catalog Catalog, relativePath string) (repoKey string, normalizedRelativePath string, repoName string, ok bool) {
	name := strings.TrimSpace(catalog.Name)
	if name == "" {
		return "", "", "", false
	}
	parts := splitPathSegments(relativePath)
	if len(parts) != EffectiveRepoPathDepth(catalog) {
		return "", "", "", false
	}
	normalizedRelativePath = strings.Join(parts, "/")
	repoName = parts[len(parts)-1]
	return name + "/" + normalizedRelativePath, normalizedRelativePath, repoName, true
}

func ParseRepoKey(repoKey string) (catalog string, relativePath string, repoName string, err error) {
	parts := splitRepoKey(repoKey)
	if len(parts) < 2 {
		return "", "", "", fmt.Errorf("invalid repo_key %q", repoKey)
	}
	catalog = parts[0]
	relativePath = strings.Join(parts[1:], "/")
	repoName = parts[len(parts)-1]
	return catalog, relativePath, repoName, nil
}

func splitRepoKey(repoKey string) []string {
	repoKey = strings.TrimSpace(repoKey)
	if repoKey == "" {
		return nil
	}
	repoKey = strings.ReplaceAll(repoKey, "\\", "/")
	rawParts := strings.Split(repoKey, "/")
	out := make([]string, 0, len(rawParts))
	for _, part := range rawParts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." {
			return nil
		}
		out = append(out, part)
	}
	return out
}

func splitPathSegments(raw string) []string {
	raw = filepath.Clean(strings.TrimSpace(raw))
	if raw == "" || raw == "." {
		return nil
	}
	parts := strings.Split(raw, string(filepath.Separator))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." {
			continue
		}
		out = append(out, part)
	}
	return out
}

func relativePathUnderRoot(path string, root string) (string, bool) {
	cleanPath := filepath.Clean(path)
	cleanRoot := filepath.Clean(root)
	if resolved, err := filepath.EvalSymlinks(cleanPath); err == nil {
		cleanPath = resolved
	}
	if resolved, err := filepath.EvalSymlinks(cleanRoot); err == nil {
		cleanRoot = resolved
	}
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil {
		return "", false
	}
	if rel == "." || rel == "" {
		return "", false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}
