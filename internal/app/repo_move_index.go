package app

import (
	"fmt"
	"strings"

	"bb-project/internal/domain"
)

func buildRepoMoveIndex(metas []domain.RepoMetadataFile) (map[string]string, error) {
	index := make(map[string]string)
	for _, meta := range metas {
		current := strings.TrimSpace(meta.RepoKey)
		if current == "" {
			continue
		}
		for _, old := range meta.PreviousRepoKeys {
			old = strings.TrimSpace(old)
			if old == "" || old == current {
				continue
			}
			if existing, ok := index[old]; ok && existing != current {
				return nil, fmt.Errorf("ambiguous previous_repo_keys mapping for %q: %q and %q", old, existing, current)
			}
			index[old] = current
		}
	}
	return index, nil
}
