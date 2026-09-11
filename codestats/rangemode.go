package main

import (
	"bufio"
	"fmt"
	"log"
	"os/exec"
	"strings"
)

// computeRangeStats attributes each line still present in the file at
// toCommit to whichever commit in (fromCommit, toCommit] last touched it,
// via a range-scoped `git blame --first-parent`. A line whose last touch
// falls at or before fromCommit ("boundary", in git's terminology) is
// excluded - it didn't change within this window.
//
// This naturally dedupes a line modified several times within the range
// (e.g. a CI job bumping a pom.xml version string on every release) down
// to a single count, credited to whoever made its last change in the
// window - rather than once per commit that happened to touch it, which is
// what counting each commit's diff separately would do.
//
// fromCommit == "" means "from the root of history": every surviving line
// counts, including ones from the very first commit. Git still marks those
// "boundary" (blame has nothing further back to compare against), but that
// marker is only meaningful - and only excluded - when an explicit
// fromCommit was given.
//
// Known limitation: renames across the range are not tracked specially,
// so a renamed-and-modified file may undercount (blame at the new path
// won't see history recorded under the old path without git's rename
// detection, which --first-parent blame does not follow across renames by
// itself here).
func computeRangeStats(repoRoot, fromCommit, toCommit string, merger *AuthorMerger) ([]FileStats, error) {
	files, err := touchedFiles(repoRoot, fromCommit, toCommit)
	if err != nil {
		return nil, err
	}

	excludeBoundary := fromCommit != ""

	type key struct{ author, path string }
	counts := make(map[key]int)

	for _, path := range files {
		if !isCodeFile(path) {
			continue
		}
		fileType, _ := categorizeFile(path)

		blamed, err := blameRange(repoRoot, fromCommit, toCommit, path)
		if err != nil {
			// Most commonly: the file no longer exists at toCommit (it was
			// deleted somewhere in the range) - nothing survives to count.
			continue
		}

		lines := make([]string, len(blamed))
		for i, bl := range blamed {
			lines[i] = bl.content
		}
		countable := linesCountable(lines, fileType)

		for i, bl := range blamed {
			if !countable[i] {
				continue
			}
			if excludeBoundary && bl.boundary {
				continue
			}
			counts[key{merger.Canonicalize(bl.author), path}]++
		}
	}

	var stats []FileStats
	for k, lines := range counts {
		fileType, language := categorizeFile(k.path)
		stats = append(stats, FileStats{
			Path:     k.path,
			Type:     fileType,
			Lines:    lines,
			Author:   k.author,
			Language: language,
		})
	}

	if len(stats) == 0 {
		log.Printf("Range %s produced no counted lines", displayRange(fromCommit, toCommit))
	}

	return stats, nil
}

// touchedFiles returns every path worth blaming: in the bounded case,
// everything whose content differs between fromCommit and toCommit (a file
// unchanged between the two endpoints can't have any surviving in-range
// lines, so it's safe to skip); in the root-sentinel case, every path that
// exists at toCommit.
func touchedFiles(repoRoot, fromCommit, toCommit string) ([]string, error) {
	var cmd *exec.Cmd
	if fromCommit != "" {
		cmd = exec.Command("git", "-C", repoRoot, "diff", "--name-only", fromCommit, toCommit)
	} else {
		cmd = exec.Command("git", "-C", repoRoot, "ls-tree", "-r", "--name-only", toCommit)
	}

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list touched files for %s: %w", displayRange(fromCommit, toCommit), err)
	}

	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

type blamedLine struct {
	author   string
	content  string
	boundary bool
}

// blameRange runs `git blame --line-porcelain --first-parent` for path as
// of toCommit, scoped to fromCommit..toCommit when fromCommit is given,
// returning the file's lines in order.
func blameRange(repoRoot, fromCommit, toCommit, path string) ([]blamedLine, error) {
	rev := toCommit
	if fromCommit != "" {
		rev = fromCommit + ".." + toCommit
	}

	output, err := exec.Command("git", "-C", repoRoot, "blame", "--line-porcelain", "--first-parent", rev, "--", path).Output()
	if err != nil {
		return nil, err
	}
	return parseBlamePorcelain(output)
}

// blameLinesAt runs `git blame --line-porcelain --first-parent` for path as
// of rev, restricted to the given 1-based, inclusive line ranges (deletions.go
// uses this to find who originally wrote a range of lines a later commit
// removed). rev is a single revision, not a fromCommit..toCommit range, so
// there's no "boundary" concept here - every returned line just has a
// normal author.
func blameLinesAt(repoRoot, rev, path string, start, count int) ([]blamedLine, error) {
	lRange := fmt.Sprintf("%d,%d", start, start+count-1)
	output, err := exec.Command("git", "-C", repoRoot, "blame", "--line-porcelain", "--first-parent", "-L", lRange, rev, "--", path).Output()
	if err != nil {
		return nil, err
	}
	return parseBlamePorcelain(output)
}

// parseBlamePorcelain parses `git blame --line-porcelain` output into an
// ordered slice of blamedLine, shared by blameRange and blameLinesAt.
func parseBlamePorcelain(output []byte) ([]blamedLine, error) {
	var lines []blamedLine
	var author string
	var boundary bool

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()

		switch {
		case strings.HasPrefix(line, "\t"):
			lines = append(lines, blamedLine{
				author:   author,
				content:  strings.TrimPrefix(line, "\t"),
				boundary: boundary,
			})
			author, boundary = "", false

		case line == "boundary":
			boundary = true

		case strings.HasPrefix(line, "author "):
			author = strings.TrimPrefix(line, "author ")

		case isBlameHeaderStart(line):
			// Start of a new block's header (a commit hash line); reset
			// per-block state defensively in case a block's content line
			// is ever missing.
			author, boundary = "", false
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return lines, nil
}

// isBlameHeaderStart reports whether line starts a new --line-porcelain
// block: a 40-character hex commit hash followed by 2-3 numbers
// (orig-line, final-line, and optionally a group line count).
func isBlameHeaderStart(line string) bool {
	fields := strings.Fields(line)
	if len(fields) < 3 || len(fields) > 4 {
		return false
	}
	if len(fields[0]) != 40 {
		return false
	}
	for _, c := range fields[0] {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}

func displayRange(fromCommit, toCommit string) string {
	if fromCommit == "" {
		return "<root>.." + toCommit
	}
	return fromCommit + ".." + toCommit
}
