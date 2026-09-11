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
| `filetypes.go`  | extension/language tables (`codeExts`), `isCodeFile`, `detectLanguage`, `shouldSkipDir` |
| `categorize.go` | ecosystem-agnostic `categorizeFile`: test/resource/build detection      |
| `gitignore.go`  | hand-rolled `.gitignore` pattern matching                        |
| `tracked.go`    | git-index-based "is this file tracked at all" check              |
| `lines.go`      | `countLines` (reads a file) and `linesCountable` (the shared blank/comment heuristics, given any ordered slice of lines) |
| `authors.go`    | `authors.yaml` loading and author-identity canonicalization      |
| `snapshot.go`   | snapshot-mode engine: walk + git blame at HEAD (or `--to`)        |
| `checkout.go`   | temporary checkout/restore helper used by snapshot mode's `--to` |
| `rangemode.go`  | range-mode engine: git blame scoped to a commit range (also the shared `--line-porcelain` parser used by `deletions.go`) |
| `deletions.go`  | range-mode-only: deleted-lines tracking (`computeDeletionStats`)   |
| `aggregate.go`  | turns `[]FileStats` into the grouped `AggregatedStats`            |
| `output.go`     | CSV / JSON / table writers, output-overwrite guard                |

## Two analysis engines, two different statistics

This is the most important thing to know before changing either mode:

- **Snapshot mode** (`snapshot.go`) answers "who currently owns each line of
  the working tree, right now (or at some commit)?" - it walks the
  filesystem and runs `git blame` per file. `FileStats.Lines` is the file's
  *current* line count.

- **Range mode** (`rangemode.go`) answers "who owns each surviving line,
  considering only commits within this range?" - for every file touched
  between the two endpoints, it runs one `git blame --line-porcelain
  --first-parent` scoped to `fromCommit..toCommit`, and excludes lines git
  marks "boundary" (last touched at or before `fromCommit`, i.e. outside
  the window). `FileStats.Lines` there is *surviving in-range* lines, not
  "lines added" - a line modified several times within the range (a CI job
  bumping a version string on every release, say) is credited exactly once,
  to whoever made its last change in the window, instead of once per commit
  that happened to touch it. It only follows first-parent history (via
  blame's own `--first-parent`, added in git 2.29+), so a merge commit
  doesn't get its own attribution separate from the feature branch commits
  it merged.

  fromCommit == "" is the "from the root of history" case (see Mode/flag
  resolution below): every surviving line counts there, including ones
  from the very first commit - git still marks those "boundary" too (blame
  has nothing further back to diff against), but that marker is only
  excluded when an explicit fromCommit was given.

Because both engines produce the same `[]FileStats` shape, `aggregate.go`
and `output.go` are mode-agnostic - don't special-case mode down there.

`linesCountable` (`lines.go`) does the blank/comment filtering for both
engines. It takes a file's lines *in order* and tracks `/* ... */` state
across them - snapshot mode feeds it a file read straight off disk, range
mode feeds it the file's lines as reconstructed from blame output (blame
always returns the whole file top-to-bottom), so both get proper
multi-line-comment handling, not just per-line heuristics.

Known range-mode limitation (acceptable for now, revisit if it matters):
renames aren't tracked specially across the range, so a renamed-and-modified
file may undercount lines recorded under its old path.

## Deleted lines (range mode only): `deletions.go`

`computeRangeStats` (above) answers "who's credited with each surviving
line". `computeDeletionStats` answers a different question: "of the lines
that got deleted somewhere within this range, who originally wrote them?" -
crediting the deletion to whoever wrote the line, not to whoever deleted
it. Combined with the surviving-line count (`AggregatedStats.ByAuthor`,
passed in as `survivingByAuthor`), that gives each author a net `Sum =
LinesAdded - LinesDeleted`.

Mechanism: `deletedHunks` runs one `git log -p --unified=0 --first-parent`
over the whole range and keeps only each diff hunk's header (old-start,
old-count) - deliberately not the actual +/- content, since `--unified=0`
strips context and we don't need it yet. For every hunk that removes at
least one line, `blameLinesAt` runs a *second*, much finer-grained blame:
`git blame -L <oldStart>,<oldStart+oldCount-1> <commit>^ -- <oldPath>`,
finding who last touched exactly those lines *before* this commit deleted
them. That's potentially one blame call per hunk-that-deletes-something in
the whole range, not one per file like `computeRangeStats` - a real cost on
a range with heavy churn, called out in `computeDeletionStats`'s doc
comment.

`AggregatedStats.Deletions` is a `*DeletionStats`, populated only in
`main.go`'s range branch (nil in snapshot mode, where "deleted within a
range" has no meaning) - `output.go` checks for nil rather than taking a
mode argument, keeping it structurally mode-agnostic even though only one
mode ever populates the field.

A line can be "not counted as surviving" for a reason that has nothing to
do with deletion: `computeRangeStats` only counts a line if its *last*
touch falls within (fromCommit, toCommit] (see boundary exclusion, above) -
a line added right at the boundary commit itself and never touched again
is excluded from `ByAuthor`, even though it's still in the file. That
author can then show up in the deletions table with `LinesAdded: 0` and a
negative `Sum`, which looks alarming until you remember `LinesAdded` here
means "credited as surviving *within this window*", not "still exists in
the file at all". `deletions_test.go`'s three-commit fixture (Alice adds
two lines, Bob adds one, Carol deletes one of Alice's) exercises exactly
this interaction - read it before changing either engine.

Same known limitation as above: a rename isn't followed back through
`blameLinesAt`, so a deleted line that had been renamed to a new path
along the way is blamed at that new path, not traced through history.

## Categorization is ecosystem-agnostic, not a per-ecosystem if-chain

`categorizeFile` (`categorize.go`) used to be Maven-specific: it looked for
the literal substrings `/src/test/` and `/src/main/`, and everything else
fell through to `Other`. It's now built from a handful of generic signals
that happen to be exactly how most ecosystems mark test code and resources:

- **Test signal**: a path segment exactly matching `test`/`tests`/
  `__tests__`/`spec`/`specs`/`e2e`/`cypress`/`molecule` (`testSegments`), OR
  a filename matching a test convention (`isTestFilename`): Go's `_test.go`,
  Python's `test_*.py`/`*_test.py`/`conftest.py`, JS/TS's
  `*.test.ts`/`*.spec.ts` (and `.js/.jsx/.tsx/.mjs/.cjs`), or the JVM's
  `FooTest.java`/`FooTests.java` (checked case-*sensitively* so
  `Latest.java` can't false-match `*Test.java` - a case-insensitive check
  can't tell "La"+"test" from "Foo"+"Test").
- **Resource signal**: a path segment exactly matching `resources`/
  `resource`/`assets`/`static`/`public`/`templates` (`resourceSegments`).
- Neither signal anchors to the repo root (segment membership, not a
  root-relative prefix), so **monorepos need no special handling** - the
  same rule matches a `packages/foo/src/main/java/...` or
  `apps/bar/src/app/x.spec.ts` wherever it sits in the tree.
- A file with no recognized source-language extension (`detectLanguage`
  returns `"other"`) always stays `Other`, regardless of directory -
  otherwise: test&&resource → `TestResources`, test → `TestCode`, resource
  → `MainResources`, neither → `MainCode`.

This one rule set is what makes Maven, Gradle, and sbt all work identically
(they deliberately mirror Maven's `src/main`/`src/test`/`resources`
layout) as well as Go, Python (+ maturin/Rust), and React/Angular/
TypeScript/Playwright - see `categorize_test.go` for a case per ecosystem,
built from real example trees. Known gap, accepted rather than risking a
false-positive: sbt's own `project/*.scala` meta-build files read as
`MainCode`/scala rather than build tooling, since a generic `project`
segment rule would be too eager to false-match unrelated repos.

`isBuildFile` (also `categorize.go`) is checked *before* any of the above
and routes project/build plumbing to `Build`, regardless of test/resource
signals: containers and compose files (`Dockerfile*`, `docker-compose*.yml`,
and bare Compose-spec names like `compose.yaml`/`compose.override.yml` -
Compose dropped the `docker-` prefix requirement, so both forms need
matching), Podman Quadlet units (`.container`/`.pod`/`.volume`/`.network`/
`.kube`/`.build`/`.image`), and build/project descriptors - either by
extension alone (`.gradle`/`.kts`/`.sbt`/`.toml`/`.cfg`/`.ini` are
essentially always build config) or by exact basename for descriptors whose
extension is otherwise too generic to blanket-route (`pom.xml`,
`setup.py`, `package.json`, `requirements.txt`, `.gitlab-ci.yml`, ...) or
prefix (`.github/`, `tsconfig*.json`). `Build` absorbed the old standalone
`Container` type - there's no `Container` constant any more.

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
