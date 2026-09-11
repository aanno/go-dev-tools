# codestats

A code analysis tool that:

* Walks the directory tree identifying files by extension and path patterns
* Categorizes files into types (test/main code, resources, scripts, docs,
  container files, other)
* Counts lines per file
* Extracts author information from git history
* Aggregates statistics with percentages

Key categorization logic for Maven standard layout:

* Test code: **/src/test/java/**, **/src/test/kotlin/**, etc.
  (language-specific test dirs)
* Main code: **/src/main/java/**, **/src/main/kotlin/**, etc.
* Test resources: **/src/test/resources/**
* Main resources: **/src/main/resources/**
* Scripts: *.sh,*.bash, *.zsh
* Documentation: *.md,*.txt, *.rst
* Container: Dockerfile*, *.dockerfile, docker-compose*.yml
* Other: Everything else

## Implementation

The tool is now complete with:

* Git blame for snapshot mode
* Commit range filtering for range mode
* Author merging via YAML config
* Parallel processing (4 workers)
* Sorted output (lines desc, then alpha)
* CSV + JSON output with overwrite protection
* Proper error logging per file
* Maven standard layout detection

## TODOs

### Phase 1

1. File overwrite check comes after processing but should be before processing.
2. Result should be tweaked:
   * Lines by Author and Type: Should be grouped by TYPE, % should be for each TYPE
     (not for all lines); add a line between groups for cli output.
   * Lines by Type: works as expected
   * Lines by Author: works as expected
   * CSV: adapt accordingly
   * JSON: grouping is missing, also should include a sum (total) for each TYPE
3. `codestats/main.go` is currently a bit too big. To be split into several files.
4. CLI progressbar to be displayed while processing.
5. Recheck that empty lines and comments lines are not counted.
6. In addition to all files maching '.gitignore' (this is already implemented),
   all files NOT under version control (i.e. all files NOT added to git) should
   be ignored.
7. Range mode:
   * Does not work, e.g. if I try is on a repo which has tags '0.4.20' and '0.4.33':

     ```sh
     codestats . --from 0.4.20 --to 0.4.33 --mode range
     2026/09/11 08:58:09 No author config found at authors.yaml, using raw authors
     2026/09/11 08:58:09 No files processed
     ```

   * Range mode should be based on commit (i.e. avoid annotate/blame if possible)
   * Range mode should always be used if '--from' flag is given.
   * In case of NO '--from' but with '--to' calculation is possible in either
     snapshot or range mode. In this case '--mode' MUST be given.
     * In this case '--mode snapshot' needs a repo without uncommited changes
       (i.e. a 'clean' repo) in order to switch to the '--to' commit. In this
       case a unclean repo should result in an error message and no processing.
   * Write an CLAUDE.md file for me.

### Phase 2

1. Support not only maven layouts; categorisation in TYPEs should at least
   also work for:
   * react and angular projects (simple and monorepos)
   * python projects (simple and monorepos)
   * python/maturin projects (that include python and rust sources)
   * go projects (simple and monorepos)
   * java gradle projects
   * scala sbt projects
2. In range mode, also calculate deleted lines
   * count them separate
   * for each author, also introduce a new column with `sum=<lines_added> - <lines_deleted_that_originate_from_this_author>

## Configuration

## Author Config Example

Merge authors with multiple emails and names into a canonical author name.
The `authors.yaml` file is used to define these mappings.

Create `authors.yaml`:

```text
authors:
  - canonical: "A. Anno"
    emails:
      - "a.anno@it.nrw.de"
      - "alice.anno@company.com"
    names:
      - "A. Anno"
      - "Alice Anno"
      - "alice"
  - canonical: "Bob Developer"
    emails:
      - "bob@company.com"
    names:
      - "Bob Dev"
      - "Bob Developer"
      - "bob.d"
```

## Usage

```bash
# help
codestats
2026/09/11 08:51:33 Usage: codestats <repo-path> [--from COMMIT] [--to COMMIT] [--config authors.yaml] [--output DIR] [--mode snapshot|range]

# Snapshot mode (current HEAD with blame)
codestats /path/to/repo --mode snapshot --config authors.yaml

# Range mode (specific commits)
codestats /path/to/repo --mode range --from abc123 --to def456 --config authors.yaml

# Custom output directory
codestats /path/to/repo --output ./stats --config authors.yaml
```
