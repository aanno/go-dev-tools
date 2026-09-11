package main

import (
	"fmt"
	"sort"
)

// AuthorInType is one author's share of lines within a single TypeGroup;
// Percent is relative to that group's TotalLines, not the grand total.
type AuthorInType struct {
	Author  string `json:"author"`
	Lines   int    `json:"lines"`
	Percent string `json:"percent"`
}

// TypeGroup is "Lines by Author and Type", grouped by Type.
type TypeGroup struct {
	Type       string         `json:"type"`
	TotalLines int            `json:"total_lines"`
	Authors    []AuthorInType `json:"authors"`
}

type TypeStats struct {
	Type    string `json:"type"`
	Lines   int    `json:"lines"`
	Percent string `json:"percent"`
}

type AuthorStats struct {
	Author  string `json:"author"`
	Lines   int    `json:"lines"`
	Percent string `json:"percent"`
}

type TotalStats struct {
	Lines int `json:"total_lines"`
}

// AggregatedStats for output
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
