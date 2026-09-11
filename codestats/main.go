// cmd/codestats/main.go - CLI entry point: flag parsing, validation and
// top-level orchestration. See snapshot.go / rangemode.go for the two
// analysis engines, aggregate.go / output.go for reporting.

package main

import (
	"fmt"
	"log"
	"os"

	"github.com/go-git/go-git/v5"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: codestats <repo-path> [--from COMMIT] [--to COMMIT] [--config authors.yaml] [--output DIR] [--mode snapshot|range]")
	}

	repoPath := os.Args[1]
	configPath := "authors.yaml"
	fromCommit := ""
	toCommit := ""
	outputPath := "."
	mode := "snapshot"

	var fromGiven, toGiven, modeGiven bool

	for i := 2; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--config":
			i++
			if i < len(os.Args) {
				configPath = os.Args[i]
			}
		case "--from":
			i++
			if i < len(os.Args) {
				fromCommit = os.Args[i]
				fromGiven = true
			}
		case "--to":
			i++
			if i < len(os.Args) {
				toCommit = os.Args[i]
				toGiven = true
			}
		case "--output":
			i++
			if i < len(os.Args) {
				outputPath = os.Args[i]
			}
		case "--mode":
			i++
			if i < len(os.Args) {
				mode = os.Args[i]
				modeGiven = true
			}
		}
	}

	// --from always means range mode, regardless of --mode.
	if fromGiven {
		if modeGiven && mode != "range" {
			log.Printf("Warning: --from given, forcing range mode (--mode %s ignored)", mode)
		}
		mode = "range"
		if !toGiven {
			toCommit = "HEAD"
			log.Printf("No --to given, defaulting to HEAD")
		}
	}

	// --to without --from needs an explicit --mode: snapshot checks out
	// --to (requires a clean worktree, see checkout.go); range treats the
	// missing --from as the root of history.
	if !fromGiven && toGiven && !modeGiven {
		log.Fatal("--to given without --from requires --mode to be specified explicitly (snapshot or range)")
	}

	if mode != "snapshot" && mode != "range" {
		log.Fatal("Mode must be 'snapshot' or 'range'")
	}

	if mode == "range" && toCommit == "" {
		toCommit = "HEAD"
	}

	// Overwrite check happens before any processing (TODO: used to happen
	// only once we tried to write the file, after all the work was done).
	if err := checkOutputPaths(outputPath); err != nil {
		log.Fatal(err)
	}

	if err := run(repoPath, configPath, fromCommit, toCommit, outputPath, mode); err != nil {
		log.Fatal(err)
	}
}

func run(repoPath, configPath, fromCommit, toCommit, outputPath, mode string) error {
	authorMerger := loadAuthorConfig(configPath)

	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return fmt.Errorf("failed to open repo: %w", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}
	repoRoot := worktree.Filesystem.Root()

	var allStats []FileStats

	switch mode {
	case "range":
		log.Printf("Computing range stats %s", displayRange(fromCommit, toCommit))
		allStats, err = computeRangeStats(repoRoot, fromCommit, toCommit, authorMerger)
		if err != nil {
			return err
		}

	case "snapshot":
		if toCommit != "" {
			restore, err := checkoutForSnapshot(repo, worktree, toCommit)
			if err != nil {
				return err
			}
			defer restore()
		}

		gitignoreMatcher := loadGitignorePatterns(repoRoot)

		tracked, err := loadTrackedFiles(repo)
		if err != nil {
			return fmt.Errorf("failed to load tracked files: %w", err)
		}

		files, err := walkRepoFiles(repoPath, repoRoot, gitignoreMatcher, tracked)
		if err != nil {
			log.Printf("Warning: walk error: %v", err)
		}

		allStats = processSnapshotFiles(repoRoot, files, authorMerger)
	}

	if len(allStats) == 0 {
		return fmt.Errorf("no files processed")
	}

	aggregated := aggregateStats(allStats)

	if mode == "range" {
		deletions, err := computeDeletionStats(repoRoot, fromCommit, toCommit, authorMerger)
		if err != nil {
			return err
		}
		applyDeletions(aggregated, deletions)
	}

	if err := outputCSV(aggregated, outputPath); err != nil {
		return err
	}
	if err := outputJSON(aggregated, outputPath); err != nil {
		return err
	}
	outputTable(aggregated)

	return nil
}
