package app

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"bb-project/internal/gitx"
)

type fixChangedFile struct {
	Path    string
	Status  string
	Added   int
	Deleted int
}

type fixRiskSnapshot struct {
	ChangedFiles               []fixChangedFile
	SecretLikeChangedPaths     []string
	NoisyChangedPaths          []string
	MissingRootGitignore       bool
	SuggestedGitignorePatterns []string
}

type fixEligibilityContext struct {
	Interactive bool
	Risk        fixRiskSnapshot
}

func collectFixRiskSnapshot(repoPath string, git gitx.Runner) (fixRiskSnapshot, error) {
	out := fixRiskSnapshot{}
	if _, err := os.Stat(filepath.Join(repoPath, ".gitignore")); err != nil {
		out.MissingRootGitignore = os.IsNotExist(err)
	}

	statusOut, err := git.RunGit(repoPath, "status", "--porcelain")
	if err != nil {
		return out, err
	}
	statusEntries := parseGitStatusPorcelain(statusOut)
	stats := collectNumstat(repoPath, git)

	changed := make([]fixChangedFile, 0, len(statusEntries))
	secret := make([]string, 0, 1)
	noisy := make([]string, 0, 2)

	for _, entry := range statusEntries {
		file := fixChangedFile{
			Path:   entry.Path,
			Status: entry.Status,
		}
		if d, ok := stats[entry.Path]; ok {
			file.Added = d.Added
			file.Deleted = d.Deleted
		} else if entry.Status == "untracked" {
			file.Added = countFileLines(filepath.Join(repoPath, entry.Path))
		}
		if isSecretLikeChangedPath(entry.Path) {
			secret = append(secret, entry.Path)
		}
		if noisyPatternForPath(entry.Path) != "" {
			noisy = append(noisy, entry.Path)
		}
		changed = append(changed, file)
	}

	sort.Slice(changed, func(i, j int) bool {
		return changed[i].Path < changed[j].Path
	})
	sort.Strings(secret)
	sort.Strings(noisy)

	suggested := collectSuggestedGitignorePatterns(repoPath, changed)

	out.ChangedFiles = changed
	out.SecretLikeChangedPaths = dedupeStrings(secret)
	out.NoisyChangedPaths = dedupeStrings(noisy)
	out.SuggestedGitignorePatterns = suggested
	return out, nil
}

func (r fixRiskSnapshot) hasSecretLikeChanges() bool {
	return len(r.SecretLikeChangedPaths) > 0
}

func (r fixRiskSnapshot) hasNoisyChangesWithoutGitignore() bool {
	return r.MissingRootGitignore && len(r.NoisyChangedPaths) > 0
}

type statusEntry struct {
	Path   string
	Status string
}

func parseGitStatusPorcelain(raw string) []statusEntry {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	out := make([]statusEntry, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" || len(line) < 4 {
			continue
		}

		code := line[:2]
		pathPart := strings.TrimSpace(line[3:])
		if pathPart == "" {
			continue
		}
		if strings.Contains(pathPart, " -> ") {
			parts := strings.Split(pathPart, " -> ")
			pathPart = parts[len(parts)-1]
		}

		status := "modified"
		switch {
		case code == "??":
			status = "untracked"
		case strings.Contains(code, "D"):
			status = "deleted"
		case strings.Contains(code, "A"):
			status = "added"
		case strings.Contains(code, "R"):
			status = "renamed"
		}

		out = append(out, statusEntry{
			Path:   filepath.ToSlash(pathPart),
			Status: status,
		})
	}
	return out
}

type diffCounts struct {
	Added   int
	Deleted int
}

func collectNumstat(repoPath string, git gitx.Runner) map[string]diffCounts {
	out := map[string]diffCounts{}
	mergeNumstat := func(raw string) {
		for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 3 {
				continue
			}
			added, addOK := parseNumstatCount(parts[0])
			deleted, delOK := parseNumstatCount(parts[1])
			if !addOK || !delOK {
				continue
			}
			path := filepath.ToSlash(parts[len(parts)-1])
			entry := out[path]
			entry.Added += added
			entry.Deleted += deleted
			out[path] = entry
		}
	}

	if raw, err := git.RunGit(repoPath, "diff", "--numstat"); err == nil {
		mergeNumstat(raw)
	}
	if raw, err := git.RunGit(repoPath, "diff", "--cached", "--numstat"); err == nil {
		mergeNumstat(raw)
	}
	return out
}

func parseNumstatCount(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "-" {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return n, true
}

func countFileLines(path string) int {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return 0
	}
	lines := 1
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	return lines
}

func isSecretLikeChangedPath(path string) bool {
	path = filepath.ToSlash(path)
	base := strings.ToLower(filepath.Base(path))
	if base == ".env" {
		return true
	}
	switch base {
	case "id_rsa", "id_dsa", "id_ecdsa", "id_ed25519":
		return true
	}
	switch strings.ToLower(filepath.Ext(base)) {
	case ".pem", ".key", ".p12", ".pfx", ".jks", ".keystore":
		return true
	default:
		return false
	}
}

func noisyPatternForPath(path string) string {
	path = filepath.ToSlash(strings.ToLower(path))
	segments := strings.Split(path, "/")
	for _, seg := range segments {
		switch seg {
		case "node_modules":
			return "node_modules/"
		case ".venv":
			return ".venv/"
		case "venv":
			return "venv/"
		case "dist":
			return "dist/"
		case "build":
			return "build/"
		case "target":
			return "target/"
		case "coverage":
			return "coverage/"
		case ".next":
			return ".next/"
		case ".turbo":
			return ".turbo/"
		}
	}
	return ""
}

func collectSuggestedGitignorePatterns(repoPath string, changed []fixChangedFile) []string {
	patternSet := map[string]struct{}{}
	for _, file := range changed {
		if p := noisyPatternForPath(file.Path); p != "" {
			patternSet[p] = struct{}{}
		}
	}

	rootCandidates := map[string]string{
		"node_modules": "node_modules/",
		".venv":        ".venv/",
		"venv":         "venv/",
		"dist":         "dist/",
		"build":        "build/",
		"target":       "target/",
		"coverage":     "coverage/",
		".next":        ".next/",
		".turbo":       ".turbo/",
	}
	for dir, pattern := range rootCandidates {
		if info, err := os.Stat(filepath.Join(repoPath, dir)); err == nil && info.IsDir() {
			patternSet[pattern] = struct{}{}
		}
	}

	out := make([]string, 0, len(patternSet))
	for pattern := range patternSet {
		out = append(out, pattern)
	}
	sort.Strings(out)
	return out
}

func dedupeStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	sort.Strings(in)
	out := make([]string, 0, len(in))
	last := ""
	for i, v := range in {
		if i == 0 || v != last {
			out = append(out, v)
			last = v
		}
	}
	return out
}
