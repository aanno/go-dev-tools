package main

import (
	"bufio"
	"fmt"
	"log"
	"os/exec"
	"strings"
)

const rangeCommitMarker = "@@CODESTATS_COMMIT@@"

// computeRangeStats walks the first-parent commit history strictly after
// fromCommit (exclusive) up to toCommit (inclusive) and attributes every
// added, non-blank, non-comment line to the commit's author - entirely from
// `git log -p`, without ever calling git blame/annotate.
//
// fromCommit == "" means "from the root of history": every first-parent
// commit reachable from toCommit, including the root commit's own diff
// (git shows a root commit's diff against the empty tree automatically).
//
// This intentionally counts *lines added within the range*, not "who
// currently owns each line" (that would be the blame-based snapshot
// semantics). Merge commits are not diffed themselves (--first-parent), so
// a feature branch's commits are counted once, via the mainline.
//
// Known limitation: renames across the range are not tracked specially,
// and comment detection on added lines has no cross-line state (see
// countableLine), so a line that is part of a multi-line comment but
// doesn't itself carry a /* or */ delimiter cannot be recognized as such.
func computeRangeStats(repoRoot, fromCommit, toCommit string, merger *AuthorMerger) ([]FileStats, error) {
	rangeArg := toCommit
	if fromCommit != "" {
		rangeArg = fromCommit + ".." + toCommit
	}

	cmd := exec.Command("git", "-C", repoRoot, "log",
		"--first-parent", "--reverse", "--no-color",
		"--format="+rangeCommitMarker+":%H%x09%an",
		"-p", rangeArg)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log %s failed: %w", rangeArg, err)
	}

	type key struct {
		author, path string
	}
	added := make(map[key]int)

	var currentAuthor, currentPath string
	var currentType FileType
	var currentCounts bool

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()

		switch {
		case strings.HasPrefix(line, rangeCommitMarker+":"):
			rest := strings.TrimPrefix(line, rangeCommitMarker+":")
			parts := strings.SplitN(rest, "\t", 2)
			currentAuthor = ""
			if len(parts) == 2 {
				currentAuthor = parts[1]
			}
			currentPath, currentCounts = "", false

		case strings.HasPrefix(line, "diff --git "):
			currentPath, currentCounts = "", false

		case strings.HasPrefix(line, "Binary files "):
			currentCounts = false

		case strings.HasPrefix(line, "+++ "):
			p := strings.TrimPrefix(line, "+++ ")
			if p == "/dev/null" {
				currentCounts = false
				continue
			}
			currentPath = strings.TrimPrefix(p, "b/")
			currentType, _ = categorizeFile(currentPath)
			currentCounts = isCodeFile(currentPath)

		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			if !currentCounts {
				continue
			}
			if countableLine(line[1:], currentType) {
				added[key{currentAuthor, currentPath}]++
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to parse git log output: %w", err)
	}

	var stats []FileStats
	for k, lines := range added {
		fileType, language := categorizeFile(k.path)
		stats = append(stats, FileStats{
			Path:     k.path,
			Type:     fileType,
			Lines:    lines,
			Author:   merger.Canonicalize(k.author),
			Language: language,
		})
	}

	if len(stats) == 0 {
		log.Printf("Range %s produced no counted added lines", rangeArg)
	}

	return stats, nil
}
