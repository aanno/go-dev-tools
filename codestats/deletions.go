package main

import (
	"bufio"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// DeletionCounts holds raw deleted-line tallies, keyed the same way
// aggregateStats keys its own maps - ready for applyDeletions to fold into
// an already-built AggregatedStats.
type DeletionCounts struct {
	ByAuthor        map[string]int
	ByType          map[FileType]int
	ByAuthorAndType map[FileType]map[string]int
}

// computeDeletionStats finds every line deleted within (fromCommit,
// toCommit] and attributes each one to whoever originally wrote it - not
// to whoever deleted it.
//
// "Originally wrote it" means: for each commit's diff, for each hunk that
// removes lines, blame the commit's parent at exactly that line range
// (`git blame -L`) to find who last touched those lines *before* this
// commit deleted them. This is a second, much finer-grained use of git
// blame than computeRangeStats's one-call-per-file: potentially one blame
// call per hunk that deletes something, which is a real cost on a range
// with heavy churn.
//
// A deletion is skipped when the deleting commit's own author is the same
// person who originally wrote the line - a self-overwrite (a CI bot
// bumping its own version string on every release is the case that
// prompted this) isn't "losing work to someone else", it's the same
// author continuing to edit their own contribution. Without this, an
// author or bot that repeatedly touches the same spot racks up a large
// Deleted count that reflects churn, not displacement.
//
// Known limitation, shared with computeRangeStats: a rename is not
// followed, so a line that moved to a new path and was then deleted there
// is blamed at its new path, not traced back through the rename.
func computeDeletionStats(repoRoot, fromCommit, toCommit string, merger *AuthorMerger) (*DeletionCounts, error) {
	hunks, err := deletedHunks(repoRoot, fromCommit, toCommit)
	if err != nil {
		return nil, err
	}

	counts := &DeletionCounts{
		ByAuthor:        make(map[string]int),
		ByType:          make(map[FileType]int),
		ByAuthorAndType: make(map[FileType]map[string]int),
	}

	for _, h := range hunks {
		fileType, _ := categorizeFile(h.oldPath)
		deletingAuthor := merger.Canonicalize(h.deletingAuthor)

		blamed, err := blameLinesAt(repoRoot, h.commit+"^", h.oldPath, h.start, h.count)
		if err != nil {
			// e.g. the parent revision genuinely doesn't have this path
			// under this exact name (most likely a rename) - skip it,
			// per the limitation above.
			continue
		}

		contents := make([]string, len(blamed))
		for i, bl := range blamed {
			contents[i] = bl.content
		}
		countable := linesCountable(contents, fileType)

		for i, bl := range blamed {
			if !countable[i] {
				continue
			}
			author := merger.Canonicalize(bl.author)
			if author == deletingAuthor {
				continue // self-overwrite, not a loss to another author
			}
			counts.ByAuthor[author]++
			counts.ByType[fileType]++
			if counts.ByAuthorAndType[fileType] == nil {
				counts.ByAuthorAndType[fileType] = make(map[string]int)
			}
			counts.ByAuthorAndType[fileType][author]++
		}
	}

	return counts, nil
}

// applyDeletions folds counts into an already-built AggregatedStats (see
// aggregateStats), setting Deleted/Sum on every row - including adding a
// new row, with Lines 0, for an author or type that has deletions but no
// surviving lines at all - and re-sorting ByAuthor by Sum (descending),
// since that table now also stands in for what would otherwise be a
// separate "who's ahead net of deletions" table.
//
// Call this only in range mode. An AggregatedStats this was never called
// on (snapshot mode) keeps Deleted/Sum nil throughout, which is what keeps
// range mode's JSON a strict superset of snapshot mode's rather than a
// different shape.
func applyDeletions(stats *AggregatedStats, counts *DeletionCounts) {
	// By author and type.
	groupIndex := make(map[string]int, len(stats.ByAuthorAndType))
	for i, g := range stats.ByAuthorAndType {
		groupIndex[g.Type] = i
	}
	for fileType := range counts.ByAuthorAndType {
		typeStr := string(fileType)
		if _, ok := groupIndex[typeStr]; !ok {
			stats.ByAuthorAndType = append(stats.ByAuthorAndType, TypeGroup{Type: typeStr})
			groupIndex[typeStr] = len(stats.ByAuthorAndType) - 1
		}
	}
	for i := range stats.ByAuthorAndType {
		ft := FileType(stats.ByAuthorAndType[i].Type)
		setGroupDeletions(&stats.ByAuthorAndType[i], counts.ByAuthorAndType[ft])
	}
	sort.Slice(stats.ByAuthorAndType, func(i, j int) bool {
		if stats.ByAuthorAndType[i].TotalLines != stats.ByAuthorAndType[j].TotalLines {
			return stats.ByAuthorAndType[i].TotalLines > stats.ByAuthorAndType[j].TotalLines
		}
		return stats.ByAuthorAndType[i].Type < stats.ByAuthorAndType[j].Type
	})

	// By type.
	typeIndex := make(map[string]int, len(stats.ByType))
	for i, t := range stats.ByType {
		typeIndex[t.Type] = i
	}
	for fileType, deleted := range counts.ByType {
		typeStr := string(fileType)
		idx, ok := typeIndex[typeStr]
		if !ok {
			stats.ByType = append(stats.ByType, TypeStats{Type: typeStr, Percent: "0.00%"})
			idx = len(stats.ByType) - 1
			typeIndex[typeStr] = idx
		}
		setDeletedSum(&stats.ByType[idx].Lines, &stats.ByType[idx].Deleted, &stats.ByType[idx].Sum, deleted)
	}
	for i := range stats.ByType {
		if stats.ByType[i].Deleted == nil {
			setDeletedSum(&stats.ByType[i].Lines, &stats.ByType[i].Deleted, &stats.ByType[i].Sum, 0)
		}
	}
	sort.Slice(stats.ByType, func(i, j int) bool {
		if stats.ByType[i].Lines != stats.ByType[j].Lines {
			return stats.ByType[i].Lines > stats.ByType[j].Lines
		}
		return stats.ByType[i].Type < stats.ByType[j].Type
	})

	// By author.
	authorIndex := make(map[string]int, len(stats.ByAuthor))
	for i, a := range stats.ByAuthor {
		authorIndex[a.Author] = i
	}
	for author, deleted := range counts.ByAuthor {
		idx, ok := authorIndex[author]
		if !ok {
			stats.ByAuthor = append(stats.ByAuthor, AuthorStats{Author: author, Percent: "0.00%"})
			idx = len(stats.ByAuthor) - 1
			authorIndex[author] = idx
		}
		setDeletedSum(&stats.ByAuthor[idx].Lines, &stats.ByAuthor[idx].Deleted, &stats.ByAuthor[idx].Sum, deleted)
	}
	for i := range stats.ByAuthor {
		if stats.ByAuthor[i].Deleted == nil {
			setDeletedSum(&stats.ByAuthor[i].Lines, &stats.ByAuthor[i].Deleted, &stats.ByAuthor[i].Sum, 0)
		}
	}
	sort.Slice(stats.ByAuthor, func(i, j int) bool {
		si, sj := *stats.ByAuthor[i].Sum, *stats.ByAuthor[j].Sum
		if si != sj {
			return si > sj
		}
		return stats.ByAuthor[i].Author < stats.ByAuthor[j].Author
	})

	// Total.
	totalDeleted := 0
	for _, d := range counts.ByAuthor {
		totalDeleted += d
	}
	setDeletedSum(&stats.Total.Lines, &stats.Total.Deleted, &stats.Total.Sum, totalDeleted)
}

// setGroupDeletions sets Deleted/Sum on every author row within group,
// adding rows (Lines 0) for authors present in byAuthor but not already in
// the group.
func setGroupDeletions(group *TypeGroup, byAuthor map[string]int) {
	authorIndex := make(map[string]int, len(group.Authors))
	for i, a := range group.Authors {
		authorIndex[a.Author] = i
	}
	for author, deleted := range byAuthor {
		idx, ok := authorIndex[author]
		if !ok {
			group.Authors = append(group.Authors, AuthorInType{Author: author})
			idx = len(group.Authors) - 1
			authorIndex[author] = idx
		}
		setDeletedSum(&group.Authors[idx].Lines, &group.Authors[idx].Deleted, &group.Authors[idx].Sum, deleted)
	}
	for i := range group.Authors {
		if group.Authors[i].Deleted == nil {
			setDeletedSum(&group.Authors[i].Lines, &group.Authors[i].Deleted, &group.Authors[i].Sum, 0)
		}
	}
}

// setDeletedSum sets *deleted and *sum (sum = lines - deleted), allocating
// fresh ints so each row gets its own pointer.
func setDeletedSum(lines *int, deleted, sum **int, deletedCount int) {
	d := deletedCount
	s := *lines - deletedCount
	*deleted = &d
	*sum = &s
}

const deletionCommitMarker = "@@CODESTATS_COMMIT@@"

type deletedHunk struct {
	commit         string // the commit whose diff (against its parent) removed these lines
	deletingAuthor string // that commit's author, raw (not yet merger-canonicalized)
	oldPath        string // the path as it existed at commit^, where the removed lines lived
	start          int    // 1-based start line in commit^'s version of oldPath
	count          int    // number of lines removed
}

var hunkHeaderRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+\d+(?:,\d+)? @@`)

// deletedHunks walks the same first-parent commit sequence computeRangeStats
// does, but instead of blaming for surviving lines, it collects every
// diff hunk that removes at least one line - using `git log -p
// --unified=0`, which gives just the hunk headers (old-start, old-count)
// without any of the actual +/- line content we don't need at this stage.
func deletedHunks(repoRoot, fromCommit, toCommit string) ([]deletedHunk, error) {
	rangeArg := toCommit
	if fromCommit != "" {
		rangeArg = fromCommit + ".." + toCommit
	}

	output, err := exec.Command("git", "-C", repoRoot, "log",
		"--first-parent", "--reverse", "--no-color", "--unified=0",
		"--format="+deletionCommitMarker+":%H%x09%an",
		"-p", rangeArg).Output()
	if err != nil {
		return nil, fmt.Errorf("git log %s failed: %w", displayRange(fromCommit, toCommit), err)
	}

	var hunks []deletedHunk
	var commit, deletingAuthor, oldPath string

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()

		switch {
		case strings.HasPrefix(line, deletionCommitMarker+":"):
			rest := strings.TrimPrefix(line, deletionCommitMarker+":")
			parts := strings.SplitN(rest, "\t", 2)
			commit = parts[0]
			deletingAuthor = ""
			if len(parts) == 2 {
				deletingAuthor = parts[1]
			}
			oldPath = ""

		case strings.HasPrefix(line, "diff --git "):
			oldPath = ""

		case strings.HasPrefix(line, "--- "):
			p := strings.TrimPrefix(line, "--- ")
			if p == "/dev/null" {
				oldPath = "" // newly created file: nothing existed to delete
				continue
			}
			p = strings.TrimPrefix(p, "a/")
			if isCodeFile(p) {
				oldPath = p
			}

		case strings.HasPrefix(line, "@@ "):
			if oldPath == "" {
				continue
			}
			m := hunkHeaderRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			start, _ := strconv.Atoi(m[1])
			count := 1
			if m[2] != "" {
				count, _ = strconv.Atoi(m[2])
			}
			if count == 0 {
				continue // pure insertion here, nothing removed
			}
			hunks = append(hunks, deletedHunk{
				commit:         commit,
				deletingAuthor: deletingAuthor,
				oldPath:        oldPath,
				start:          start,
				count:          count,
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return hunks, nil
}
