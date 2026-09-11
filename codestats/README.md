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
* Commit-log/diff based range mode (no blame/annotate)
* Author merging via YAML config
* Parallel processing (4 workers) with a CLI progress bar
* Sorted, grouped-by-type output (lines desc, then alpha)
* CSV + JSON output with overwrite protection (checked before processing)
* Proper error logging per file
* Maven standard layout detection
* `.gitignore` matching plus a "must be tracked by git" filter
* Split across several files by concern - see [CLAUDE.md](CLAUDE.md)

## TODOs

### Phase 1

Done - see [CLAUDE.md](CLAUDE.md) for how each of these ended up implemented
(mode/flag resolution, the two analysis engines, the two filtering layers,
the grouped output shape).

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
