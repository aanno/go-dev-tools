# CLAUDE.md

Guidance for Claude Code (and other agents) working in this directory.

## What this is

`codestats` is a single Go CLI binary (module `codestats`, no internal
packages - everything lives in `package main` at the repo root) that
analyzes a git repository's source tree: it categorizes files, counts
"real" lines per file (blank lines and comments excluded), attributes those
lines to authors via git, and prints/exports the aggregated result.

See [README.md](README.md) for user-facing usage, flags, and the
`authors.yaml` config format.

## Build / run / test

```sh
go build .              # builds ./codestats
go vet ./...
go test ./...
go run . <repo-path> [flags...]
```

There's no separate test-repo fixture yet; `lines_test.go` covers the
line-counting heuristics in isolation. When testing the CLI end-to-end
against a real repo, prefer a disposable clone (`git clone <repo> /tmp/x`)
over the working checkout if the run might use snapshot mode's `--to`
checkout (see below) - it restores HEAD afterward, but there's no reason to
take the risk against a repo with uncommitted work.

## File layout

The tool used to be one ~850-line `main.go`; it's now split by concern:

| File            | Responsibility                                                  |
|-----------------|------------------------------------------------------------------|
| `main.go`       | flag parsing/validation, top-level `run()` orchestration         |
| `types.go`      | `FileType`, `FileStats`                                          |
| `filetypes.go`  | path/extension-based categorization (`categorizeFile`, `isCodeFile`, `detectLanguage`, `shouldSkipDir`) |
| `gitignore.go`  | hand-rolled `.gitignore` pattern matching                        |
| `tracked.go`    | git-index-based "is this file tracked at all" check              |
| `lines.go`      | `countLines` (whole file) and `countableLine` (single diff line) |
| `authors.go`    | `authors.yaml` loading and author-identity canonicalization      |
| `snapshot.go`   | snapshot-mode engine: walk + git blame                           |
| `checkout.go`   | temporary checkout/restore helper used by snapshot mode's `--to` |
| `rangemode.go`  | range-mode engine: commit-log/diff based, no blame                |
| `aggregate.go`  | turns `[]FileStats` into the grouped `AggregatedStats`            |
| `output.go`     | CSV / JSON / table writers, output-overwrite guard                |

## Two analysis engines, two different statistics

This is the most important thing to know before changing either mode:

- **Snapshot mode** (`snapshot.go`) answers "who currently owns each line of
  the working tree, right now (or at some commit)?" - it walks the
  filesystem and runs `git blame` per file. `FileStats.Lines` is the file's
  *current* line count.

- **Range mode** (`rangemode.go`) answers "how many lines did each author
  *add* within this commit range?" - it runs one `git log --first-parent -p`
  and parses the unified diff itself, deliberately **never** calling
  `git blame`/`git annotate`. `FileStats.Lines` there is *added* lines
  within the range, not current ownership. It only follows first-parent
  history, so merge commits' own diffs aren't double-counted against a
  feature branch's commits.

Because both engines produce the same `[]FileStats` shape, `aggregate.go`
and `output.go` are mode-agnostic - don't special-case mode down there.

Known range-mode limitations (acceptable for now, revisit if it matters):
renames aren't tracked specially across the range, and comment detection on
added lines (`countableLine`) has no cross-line state, unlike `countLines`'s
proper `/* ... */` tracking - a diff line that's part of a multi-line
comment but doesn't itself carry a delimiter can't be recognized as such.

## Mode/flag resolution (see `main()`)

- `--from` given → mode is forced to `range` (overriding an explicit
  `--mode snapshot`, with a warning). Missing `--to` defaults to `HEAD`.
- `--to` given, `--from` not given → `--mode` **must** be passed explicitly:
  - `--mode range`: `--from` is treated as the root of history (i.e. full
    history up to `--to`).
  - `--mode snapshot`: temporarily checks out `--to` (`checkout.go`), which
    requires a clean worktree first, and restores the original HEAD
    afterward via `defer` - even on error, so keep new fatal paths inside
    `run()` returning `error` rather than calling `log.Fatal`, or the
    restore never runs.
- Neither given → today's default: snapshot mode against the current
  working tree, unchanged.

## Two filtering layers in snapshot mode

`walkRepoFiles` applies both, and both must pass for a file to be analyzed:

1. `.gitignore` pattern matching (`gitignore.go`) - hand-rolled, not
   delegated to go-git or the `git` CLI.
2. Git-tracked check (`tracked.go`) - files never `git add`-ed are skipped
   even if not gitignored.

## Output overwrite protection

`checkOutputPaths` runs before any git/walk work in `main()`, not right
before writing - a doomed run (existing `codestats.csv`/`codestats.json`)
fails in milliseconds instead of after a full analysis.

## Output shape

`AggregatedStats.ByAuthorAndType` is grouped **by type first**: each
`TypeGroup` carries that type's `TotalLines` plus its authors' lines with
`Percent` relative to the *type's* total, not the grand total. `ByType` and
`ByAuthor` percentages remain relative to the grand total. Keep CSV and
JSON in sync if this shape changes - see `outputCSV`/`outputJSON` in
`output.go`.
