package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/schollz/progressbar/v3"
)

// walkRepoFiles collects every code file under repoPath that passes the
// .gitignore matcher and is tracked by git (TODO: files never `git add`-ed
// are ignored too, not just gitignored ones).
func walkRepoFiles(repoPath, repoRoot string, gitignoreMatcher *GitignoreMatcher, tracked map[string]bool) ([]string, error) {
	var files []string
	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if shouldSkipDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, _ := filepath.Rel(repoRoot, path)
		relPath = filepath.ToSlash(relPath)

		if gitignoreMatcher != nil && gitignoreMatcher.Match(relPath, info.IsDir()) {
			return nil
		}

		if !tracked[relPath] {
			return nil
		}

		if isCodeFile(path) {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// processSnapshotFiles runs the current-HEAD blame-based analysis over
// paths using a small worker pool, showing a progress bar as files
// complete.
func processSnapshotFiles(repoRoot string, paths []string, merger *AuthorMerger) []FileStats {
	work := make(chan string, len(paths))
	for _, p := range paths {
		work <- p
	}
	close(work)

	statsChan := make(chan FileStats, len(paths))
	bar := progressbar.Default(int64(len(paths)), "Processing files")

	var wg sync.WaitGroup
	numWorkers := 4
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range work {
				stat := processSnapshotFile(repoRoot, path, merger)
				_ = bar.Add(1)
				if stat != nil {
					statsChan <- *stat
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(statsChan)
	}()

	var allStats []FileStats
	for stat := range statsChan {
		allStats = append(allStats, stat)
	}
	return allStats
}

func processSnapshotFile(repoRoot, path string, merger *AuthorMerger) *FileStats {
	fileType, language := categorizeFile(path)

	lines, err := countLines(path, fileType)
	if err != nil || lines == 0 {
		return nil
	}

	author := getAuthorBlame(repoRoot, path)
	if author == "unknown" {
		// Silently skip files we can't get author for
		return nil
	}
	author = merger.Canonicalize(author)

	return &FileStats{
		Path:     path,
		Type:     fileType,
		Lines:    lines,
		Author:   author,
		Language: language,
	}
}

func getAuthorBlame(repoRoot, path string) string {
	relPath, err := filepath.Rel(repoRoot, path)
	if err != nil {
		relPath = path
	}
	relPath = filepath.ToSlash(relPath)

	// Use git blame CLI (most reliable)
	cmd := exec.Command("git", "-C", repoRoot, "blame", "--line-porcelain", relPath)
	output, err := cmd.Output()
	if err != nil {
		// Silently ignore files that can't be blamed
		return "unknown"
	}

	authorCounts := make(map[string]int)
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "author ") {
			author := strings.TrimPrefix(line, "author ")
			authorCounts[author]++
		}
	}

	return getTopAuthor(authorCounts)
}

func getTopAuthor(counts map[string]int) string {
	if len(counts) == 0 {
		return "unknown"
	}

	maxCount := 0
	topAuthor := "unknown"
	for author, count := range counts {
		if count > maxCount {
			maxCount = count
			topAuthor = author
		}
	}
	return topAuthor
}
