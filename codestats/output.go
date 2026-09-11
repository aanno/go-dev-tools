package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/olekukonko/tablewriter"
)

// checkOutputPaths verifies neither output file exists yet. Called before
// any processing starts, so a doomed run fails fast instead of after all
// the (potentially slow) git work.
func checkOutputPaths(outputPath string) error {
	for _, name := range []string{"codestats.csv", "codestats.json"} {
		p := filepath.Join(outputPath, name)
		if _, err := os.Stat(p); err == nil {
			return fmt.Errorf("output file %s already exists; remove it or use --output to specify a different directory", p)
		}
	}
	return nil
}

// intPtrStr renders a *int as CSV/table cell text: "" when nil (snapshot
// mode, not applicable), the number otherwise (including a real 0).
func intPtrStr(p *int) string {
	if p == nil {
		return ""
	}
	return strconv.Itoa(*p)
}

// groupDeletedSum sums a TypeGroup's per-author Deleted/Sum into a
// type-level total, for the group's "(total)" row. ok is false in
// snapshot mode (no author in the group has Deleted set).
func groupDeletedSum(g TypeGroup) (deleted, sum int, ok bool) {
	for _, a := range g.Authors {
		if a.Deleted == nil {
			continue
		}
		ok = true
		deleted += *a.Deleted
		sum += *a.Sum
	}
	return deleted, sum, ok
}

func outputCSV(stats *AggregatedStats, outputPath string) error {
	csvPath := filepath.Join(outputPath, "codestats.csv")

	file, err := os.Create(csvPath)
	if err != nil {
		return fmt.Errorf("failed to create CSV: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	rangeMode := stats.Total.Deleted != nil

	header := []string{"category", "type", "author", "lines", "percent"}
	if rangeMode {
		header = append(header, "deleted", "sum")
	}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	row := func(base []string, deleted, sum *int) []string {
		if rangeMode {
			return append(base, intPtrStr(deleted), intPtrStr(sum))
		}
		return base
	}

	// By author and type, grouped: one total row per type, then its authors
	// with percent relative to that type's total.
	for _, group := range stats.ByAuthorAndType {
		gDeleted, gSum, gOK := groupDeletedSum(group)
		var gd, gs *int
		if gOK {
			gd, gs = &gDeleted, &gSum
		}
		if err := writer.Write(row([]string{"by_author_and_type_total", group.Type, "",
			strconv.Itoa(group.TotalLines), "100.00%"}, gd, gs)); err != nil {
			return fmt.Errorf("failed to write CSV: %w", err)
		}
		for _, r := range group.Authors {
			if err := writer.Write(row([]string{"by_author_and_type", group.Type, r.Author,
				strconv.Itoa(r.Lines), r.Percent}, r.Deleted, r.Sum)); err != nil {
				return fmt.Errorf("failed to write CSV: %w", err)
			}
		}
	}

	// By type
	for _, r := range stats.ByType {
		if err := writer.Write(row([]string{"by_type", r.Type, "",
			strconv.Itoa(r.Lines), r.Percent}, r.Deleted, r.Sum)); err != nil {
			return fmt.Errorf("failed to write CSV: %w", err)
		}
	}

	// By author
	for _, r := range stats.ByAuthor {
		if err := writer.Write(row([]string{"by_author", "", r.Author,
			strconv.Itoa(r.Lines), r.Percent}, r.Deleted, r.Sum)); err != nil {
			return fmt.Errorf("failed to write CSV: %w", err)
		}
	}

	// Total
	if err := writer.Write(row([]string{"total", "", "", strconv.Itoa(stats.Total.Lines), "100.00%"},
		stats.Total.Deleted, stats.Total.Sum)); err != nil {
		return fmt.Errorf("failed to write CSV: %w", err)
	}

	log.Printf("CSV written to %s", csvPath)
	return nil
}

func outputJSON(stats *AggregatedStats, outputPath string) error {
	jsonPath := filepath.Join(outputPath, "codestats.json")

	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	if err := os.WriteFile(jsonPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write JSON: %w", err)
	}

	log.Printf("JSON written to %s", jsonPath)
	return nil
}

func outputTable(stats *AggregatedStats) {
	rangeMode := stats.Total.Deleted != nil

	fmt.Println("\n=== CODE STATISTICS ===")

	fmt.Println("\n## Lines by Author and Type")
	table1 := tablewriter.NewWriter(os.Stdout)
	header1 := []string{"Type", "Author", "Lines", "%"}
	if rangeMode {
		header1 = append(header1, "Deleted", "Sum")
	}
	table1.SetHeader(header1)
	table1.SetBorder(false)
	table1.SetAutoWrapText(false)
	// The Type column repeats the same value for every row in a group;
	// AutoMergeCells collapses those into one spanning cell, so the whole
	// section reads as a single table with a visual break per group
	// instead of one table per type.
	table1.SetAutoMergeCellsByColumnIndex([]int{0})
	appendRow1 := func(rowType, author string, lines int, percent string, deleted, sum *int) {
		r := []string{rowType, author, strconv.Itoa(lines), percent}
		if rangeMode {
			r = append(r, intPtrStr(deleted), intPtrStr(sum))
		}
		table1.Append(r)
	}
	for _, group := range stats.ByAuthorAndType {
		gDeleted, gSum, gOK := groupDeletedSum(group)
		var gd, gs *int
		if gOK {
			gd, gs = &gDeleted, &gSum
		}
		appendRow1(group.Type, "(total)", group.TotalLines, "100.00%", gd, gs)
		for _, r := range group.Authors {
			appendRow1(group.Type, r.Author, r.Lines, r.Percent, r.Deleted, r.Sum)
		}
	}
	table1.Render()

	fmt.Println("\n## Lines by Type")
	table2 := tablewriter.NewWriter(os.Stdout)
	header2 := []string{"Type", "Lines", "%"}
	if rangeMode {
		header2 = append(header2, "Deleted", "Sum")
	}
	table2.SetHeader(header2)
	table2.SetBorder(false)
	table2.SetAutoWrapText(false)
	for _, r := range stats.ByType {
		row := []string{r.Type, strconv.Itoa(r.Lines), r.Percent}
		if rangeMode {
			row = append(row, intPtrStr(r.Deleted), intPtrStr(r.Sum))
		}
		table2.Append(row)
	}
	table2.Render()

	// In range mode, this table also stands in for what would otherwise be
	// a separate "deleted lines by author" table - see applyDeletions's
	// sort-by-Sum for this slice.
	fmt.Println("\n## Lines by Author")
	table3 := tablewriter.NewWriter(os.Stdout)
	header3 := []string{"Author", "Lines", "%"}
	if rangeMode {
		header3 = append(header3, "Deleted", "Sum")
	}
	table3.SetHeader(header3)
	table3.SetBorder(false)
	table3.SetAutoWrapText(false)
	for _, r := range stats.ByAuthor {
		row := []string{r.Author, strconv.Itoa(r.Lines), r.Percent}
		if rangeMode {
			row = append(row, intPtrStr(r.Deleted), intPtrStr(r.Sum))
		}
		table3.Append(row)
	}
	table3.Render()

	if rangeMode {
		fmt.Printf("\n## Total Lines: %d (Deleted: %d, Sum: %d)\n",
			stats.Total.Lines, *stats.Total.Deleted, *stats.Total.Sum)
	} else {
		fmt.Printf("\n## Total Lines: %d\n", stats.Total.Lines)
	}
}
