package main

import (
	"path/filepath"

	"github.com/go-git/go-git/v5"
)

// loadTrackedFiles returns the set of paths (repo-root-relative, git-style
// forward slashes) that are tracked in the git index. Combined with
// GitignoreMatcher, this makes sure files that were never `git add`-ed are
// excluded from analysis too, not just gitignored ones.
func loadTrackedFiles(repo *git.Repository) (map[string]bool, error) {
	idx, err := repo.Storer.Index()
	if err != nil {
		return nil, err
	}

	tracked := make(map[string]bool, len(idx.Entries))
	for _, entry := range idx.Entries {
		tracked[filepath.ToSlash(entry.Name)] = true
	}
	return tracked, nil
}
