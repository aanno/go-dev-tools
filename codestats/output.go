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
	for i, group := range stats.ByAuthorAndType {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("--- %s (%d lines) ---\n", group.Type, group.TotalLines)
		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"Author", "Lines", "%"})
		table.SetBorder(false)
		table.SetAutoWrapText(false)
		for _, row := range group.Authors {
			table.Append([]string{row.Author, strconv.Itoa(row.Lines), row.Percent})
		}
		table.Render()
	}

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
}
