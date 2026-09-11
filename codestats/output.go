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

func outputCSV(stats *AggregatedStats, outputPath string) error {
	csvPath := filepath.Join(outputPath, "codestats.csv")

	file, err := os.Create(csvPath)
	if err != nil {
		return fmt.Errorf("failed to create CSV: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	if err := writer.Write([]string{"category", "type", "author", "lines", "percent"}); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// By author and type, grouped: one total row per type, then its authors
	// with percent relative to that type's total.
	for _, group := range stats.ByAuthorAndType {
		if err := writer.Write([]string{"by_author_and_type_total", group.Type, "",
			strconv.Itoa(group.TotalLines), "100.00%"}); err != nil {
			return fmt.Errorf("failed to write CSV: %w", err)
		}
		for _, row := range group.Authors {
			if err := writer.Write([]string{"by_author_and_type", group.Type, row.Author,
				strconv.Itoa(row.Lines), row.Percent}); err != nil {
				return fmt.Errorf("failed to write CSV: %w", err)
			}
		}
	}

	// By type
	for _, row := range stats.ByType {
		if err := writer.Write([]string{"by_type", row.Type, "",
			strconv.Itoa(row.Lines), row.Percent}); err != nil {
			return fmt.Errorf("failed to write CSV: %w", err)
		}
	}

	// By author
	for _, row := range stats.ByAuthor {
		if err := writer.Write([]string{"by_author", "", row.Author,
			strconv.Itoa(row.Lines), row.Percent}); err != nil {
			return fmt.Errorf("failed to write CSV: %w", err)
		}
	}

	// Total
	if err := writer.Write([]string{"total", "", "", strconv.Itoa(stats.Total.Lines), "100.00%"}); err != nil {
		return fmt.Errorf("failed to write CSV: %w", err)
	}

	// Deletions (range mode only) - kept in the same 5-column shape rather
	// than widening the schema: "lines" carries whichever single number
	// each category is about (a deleted count, or the net added-minus-
	// deleted sum), distinguished by the category label.
	if stats.Deletions != nil {
		for _, row := range stats.Deletions.ByType {
			if err := writer.Write([]string{"deleted_by_type", row.Type, "", strconv.Itoa(row.LinesDeleted), ""}); err != nil {
				return fmt.Errorf("failed to write CSV: %w", err)
			}
		}
		for _, row := range stats.Deletions.ByAuthor {
			if err := writer.Write([]string{"deleted_by_author", "", row.Author, strconv.Itoa(row.LinesDeleted), ""}); err != nil {
				return fmt.Errorf("failed to write CSV: %w", err)
			}
			if err := writer.Write([]string{"net_sum_by_author", "", row.Author, strconv.Itoa(row.Sum), ""}); err != nil {
				return fmt.Errorf("failed to write CSV: %w", err)
			}
		}
		if err := writer.Write([]string{"deleted_total", "", "", strconv.Itoa(stats.Deletions.Total), ""}); err != nil {
			return fmt.Errorf("failed to write CSV: %w", err)
		}
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
	fmt.Println("\n=== CODE STATISTICS ===")

	fmt.Println("\n## Lines by Author and Type")
	table1 := tablewriter.NewWriter(os.Stdout)
	table1.SetHeader([]string{"Type", "Author", "Lines", "%"})
	table1.SetBorder(false)
	table1.SetAutoWrapText(false)
	// The Type column repeats the same value for every row in a group;
	// AutoMergeCells collapses those into one spanning cell, so the whole
	// section reads as a single table with a visual break per group
	// instead of one table per type.
	table1.SetAutoMergeCellsByColumnIndex([]int{0})
	for _, group := range stats.ByAuthorAndType {
		table1.Append([]string{group.Type, "(total)", strconv.Itoa(group.TotalLines), "100.00%"})
		for _, row := range group.Authors {
			table1.Append([]string{group.Type, row.Author, strconv.Itoa(row.Lines), row.Percent})
		}
	}
	table1.Render()

	fmt.Println("\n## Lines by Type")
	table2 := tablewriter.NewWriter(os.Stdout)
	table2.SetHeader([]string{"Type", "Lines", "%"})
	table2.SetBorder(false)
	table2.SetAutoWrapText(false)
	for _, row := range stats.ByType {
		table2.Append([]string{row.Type, strconv.Itoa(row.Lines), row.Percent})
	}
	table2.Render()

	fmt.Println("\n## Lines by Author")
	table3 := tablewriter.NewWriter(os.Stdout)
	table3.SetHeader([]string{"Author", "Lines", "%"})
	table3.SetBorder(false)
	table3.SetAutoWrapText(false)
	for _, row := range stats.ByAuthor {
		table3.Append([]string{row.Author, strconv.Itoa(row.Lines), row.Percent})
	}
	table3.Render()

	fmt.Printf("\n## Total Lines: %d\n", stats.Total.Lines)

	if stats.Deletions != nil {
		outputDeletionsTable(stats.Deletions)
	}
}

// outputDeletionsTable renders the range-mode-only deleted-lines section:
// how many lines originally attributed to each author (or type) were
// deleted somewhere within the range, and each author's net Sum once
// that's subtracted from their surviving line count.
func outputDeletionsTable(d *DeletionStats) {
	fmt.Println("\n## Deleted Lines by Type (range mode)")
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Type", "Deleted"})
	table.SetBorder(false)
	table.SetAutoWrapText(false)
	for _, row := range d.ByType {
		table.Append([]string{row.Type, strconv.Itoa(row.LinesDeleted)})
	}
	table.Render()

	fmt.Println("\n## Deleted Lines by Author (Sum = Added - Deleted)")
	table2 := tablewriter.NewWriter(os.Stdout)
	table2.SetHeader([]string{"Author", "Added", "Deleted", "Sum"})
	table2.SetBorder(false)
	table2.SetAutoWrapText(false)
	for _, row := range d.ByAuthor {
		table2.Append([]string{row.Author, strconv.Itoa(row.LinesAdded), strconv.Itoa(row.LinesDeleted), strconv.Itoa(row.Sum)})
	}
	table2.Render()

	fmt.Printf("\n## Total Deleted Lines: %d\n", d.Total)
}
