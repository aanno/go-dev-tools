package main

import (
	"fmt"
	"sort"
)

// AuthorInType is one author's share of lines within a single TypeGroup;
// Percent is relative to that group's TotalLines, not the grand total.
//
// Deleted and Sum are nil in snapshot mode, where "deleted" has no
// meaning. In range mode (see deletions.go's applyDeletions), every row
// gets both set - even to 0 - so a range-mode run's JSON never mixes rows
// that have the fields with rows that don't. They're pointers rather than
// plain ints specifically so a real 0 (range mode, no deletions) still
// serializes as "deleted": 0 instead of being indistinguishable from "not
// applicable" under omitempty - that's what keeps range mode's JSON a
// strict superset of snapshot mode's, rather than a different shape.
type AuthorInType struct {
	Author  string `json:"author"`
	Lines   int    `json:"lines"`
	Percent string `json:"percent"`
	Deleted *int   `json:"deleted,omitempty"`
	Sum     *int   `json:"sum,omitempty"`
}

// TypeGroup is "Lines by Author and Type", grouped by Type.
type TypeGroup struct {
	Type       string         `json:"type"`
	TotalLines int            `json:"total_lines"`
	Authors    []AuthorInType `json:"authors"`
}

// TypeStats: Deleted/Sum follow the same nil-in-snapshot-mode convention
// as AuthorInType.
type TypeStats struct {
	Type    string `json:"type"`
	Lines   int    `json:"lines"`
	Percent string `json:"percent"`
	Deleted *int   `json:"deleted,omitempty"`
	Sum     *int   `json:"sum,omitempty"`
}

// AuthorStats: Deleted/Sum follow the same nil-in-snapshot-mode convention
// as AuthorInType. In range mode this row also stands in for what would
// otherwise be a separate "deleted lines by author" table - see
// applyDeletions's re-sort by Sum.
type AuthorStats struct {
	Author  string `json:"author"`
	Lines   int    `json:"lines"`
	Percent string `json:"percent"`
	Deleted *int   `json:"deleted,omitempty"`
	Sum     *int   `json:"sum,omitempty"`
}

type TotalStats struct {
	Lines   int  `json:"total_lines"`
	Deleted *int `json:"total_deleted,omitempty"`
	Sum     *int `json:"total_sum,omitempty"`
}

// AggregatedStats for output. It's built once by aggregateStats from the
// engine-agnostic []FileStats (surviving lines only); range mode then
// calls applyDeletions on the result to fold deleted-line counts in - see
// deletions.go. Nothing here is mode-specific by construction; a caller
// that skips applyDeletions just gets nil Deleted/Sum throughout.
type AggregatedStats struct {
	ByAuthorAndType []TypeGroup   `json:"by_author_and_type"`
	ByType          []TypeStats   `json:"by_type"`
	ByAuthor        []AuthorStats `json:"by_author"`
	Total           TotalStats    `json:"total"`
}

func aggregateStats(stats []FileStats) *AggregatedStats {
	typeAuthorLines := make(map[FileType]map[string]int)
	typeLines := make(map[FileType]int)
	authorLines := make(map[string]int)
	totalLines := 0

	for _, s := range stats {
		if typeAuthorLines[s.Type] == nil {
			typeAuthorLines[s.Type] = make(map[string]int)
		}
		typeAuthorLines[s.Type][s.Author] += s.Lines
		typeLines[s.Type] += s.Lines
		authorLines[s.Author] += s.Lines
		totalLines += s.Lines
	}

	result := &AggregatedStats{}

	// By author and type, grouped by type; percent is relative to the
	// type's own total, not the grand total.
	var groups []TypeGroup
	for fileType, authors := range typeAuthorLines {
		typeTotal := typeLines[fileType]

		var authorRows []AuthorInType
		for author, lines := range authors {
			percent := fmt.Sprintf("%.2f%%", float64(lines)*100/float64(typeTotal))
			authorRows = append(authorRows, AuthorInType{
				Author:  author,
				Lines:   lines,
				Percent: percent,
			})
		}
		sort.Slice(authorRows, func(i, j int) bool {
			if authorRows[i].Lines != authorRows[j].Lines {
				return authorRows[i].Lines > authorRows[j].Lines
			}
			return authorRows[i].Author < authorRows[j].Author
		})

		groups = append(groups, TypeGroup{
			Type:       string(fileType),
			TotalLines: typeTotal,
			Authors:    authorRows,
		})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].TotalLines != groups[j].TotalLines {
			return groups[i].TotalLines > groups[j].TotalLines
		}
		return groups[i].Type < groups[j].Type
	})
	result.ByAuthorAndType = groups

	// By type (sorted by lines desc, then alphabetically)
	var typeRows []TypeStats
	for fileType, lines := range typeLines {
		percent := fmt.Sprintf("%.2f%%", float64(lines)*100/float64(totalLines))
		typeRows = append(typeRows, TypeStats{
			Type:    string(fileType),
			Lines:   lines,
			Percent: percent,
		})
	}
	sort.Slice(typeRows, func(i, j int) bool {
		if typeRows[i].Lines != typeRows[j].Lines {
			return typeRows[i].Lines > typeRows[j].Lines
		}
		return typeRows[i].Type < typeRows[j].Type
	})
	result.ByType = typeRows

	// By author (sorted by lines desc, then alphabetically)
	var authorRows []AuthorStats
	for author, lines := range authorLines {
		percent := fmt.Sprintf("%.2f%%", float64(lines)*100/float64(totalLines))
		authorRows = append(authorRows, AuthorStats{
			Author:  author,
			Lines:   lines,
			Percent: percent,
		})
	}
	sort.Slice(authorRows, func(i, j int) bool {
		if authorRows[i].Lines != authorRows[j].Lines {
			return authorRows[i].Lines > authorRows[j].Lines
		}
		return authorRows[i].Author < authorRows[j].Author
	})
	result.ByAuthor = authorRows

	result.Total = TotalStats{Lines: totalLines}

	return result
}
