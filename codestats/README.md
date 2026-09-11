# codestats

A code analysis tool that:

* Walks the directory tree identifying files by extension and path patterns
* Categorizes files into types (test/main code, resources, scripts, docs,
  container files, other)
* Counts lines per file
* Extracts author information from git history
* Aggregates statistics with percentages

Key categorization logic (ecosystem-agnostic, works the same in a monorepo

* see [CLAUDE.md](CLAUDE.md) for the full rule set and per-ecosystem
coverage table):

* Test code: a directory named `test`/`tests`/`__tests__`/`spec`/`specs`/
  `e2e`/`cypress`/`molecule` anywhere in the path, or a filename matching
  the ecosystem's test convention (Go's `_test.go`, Python's `test_*.py`,
  JS/TS's `*.test.ts`/`*.spec.ts`, the JVM's `FooTest.java`, ...)
* Main code: any other file with a recognized source-language extension
* Test/main resources: as above, but under a `resources`/`assets`/`static`/
  `public`/`templates` directory
* Scripts: `*.sh`, `*.bash`, `*.zsh`
* Documentation: `*.md`, `*.txt`, `*.rst`, `*.adoc`
* Build: project/build plumbing rather than application code - containers
  and compose files (`Dockerfile*`, `docker-compose*.yml`, bare
  `compose.yaml`), Podman Quadlet units (`*.container`, `*.pod`, ...), and
  build/project descriptors (`pom.xml`, `build.gradle`, `build.sbt`,
  `Cargo.toml`, `package.json`, CI config, ...)
* Other: an extension this tool doesn't recognize at all

## Implementation

The tool is now complete with:

* Git blame for snapshot mode
* Range mode via git blame scoped to the commit range, so a line changed
  several times within the range (e.g. a CI-bumped version string) is
  credited once, to whoever last touched it in that window
* Author merging via YAML config
* Parallel processing (4 workers) with a CLI progress bar
* Sorted, grouped-by-type output (lines desc, then alpha)
* CSV + JSON output with overwrite protection (checked before processing)
* Proper error logging per file
* Ecosystem-agnostic categorization (Maven/Gradle/sbt, Go, Python (+
  maturin/Rust), React/Angular/TypeScript, Ansible, containers/compose/
  Quadlet), monorepos included for free
* `.gitignore` matching plus a "must be tracked by git" filter
* Split across several files by concern - see [CLAUDE.md](CLAUDE.md)

## TODOs

### Phase 1

Done - see [CLAUDE.md](CLAUDE.md) for how each of these ended up implemented
(mode/flag resolution, the two analysis engines, the two filtering layers,
the grouped output shape).

### Phase 2

1. Done - see [CLAUDE.md](CLAUDE.md) for the categorization rule set and
   per-ecosystem coverage table (react/angular, python, python/maturin, go,
   java gradle, scala sbt, ansible - simple and monorepo alike - plus the
   new `build` category for containers/compose/Quadlet/build descriptors).
2. Done - range mode now also reports deleted lines, separately from
   surviving lines, credited to whoever originally wrote the deleted line
   (not whoever deleted it) - plus a per-author `sum = lines_added -
   lines_deleted_that_originate_from_this_author` column. See
   [CLAUDE.md](CLAUDE.md) for the mechanism and a caveat worth reading
   before trusting a negative `sum` at face value.

Example react simple project layout:

```text
. (root: *.code-workspace *.json *.yaml *.d.ts *.xml *.md *.conf *.properties *.config.js .env .npmrc .prettierrc .gitlab-ci.yml)
├── dataport_deployment
├── docker
├── docs
│   ├── arc42 (mostly markdown)
│   ├── plugin-development
│   └── resources
│       └── img (icons, screenshots, etc., as *.jpg or other image formats))
├── node_modules <ignored>
├── public
│   └── assets
│       └── themes
│           ├── lara-dark-indigo (*.css)
│           │   └── fonts (*.woff2 and other font formats)
│           └── lara-light-indigo
│               └── fonts
├── scripts (*.sh bash and sh scripts, *.js *.mjs javascript)
├── src (*.ts *.tsx *.test.ts typescript)
│   ├── api-clients
│   ├── assets
│   │   ├── fonts
│   │   │   └── fira-sans (*.woff2 and other font formats)
│   │   └── icons (*.svg and other image formats)
│   ├── components
│   │   ├── fehlerliste
│   │   ├── filter-templates
│   │   ├── footer
│   │   ├── header
│   │   │   └── bread-crumb
│   │   └── sidebar
│   ├── configs
│   ├── constants (*.ts)
│   │   └── user-permissions
│   ├── features
│   │   ├── batchjob-monitor
│   │   ├── error
│   │   ├── export-overview
│   │   │   ├── components
│   │   │   ├── data
│   │   │   └── utils
│   │   ├── config
│   │   │   ├── status
│   │   │   └── steps
│   │   │       ├── erhebungsdaten
│   │   │       ├── erhebungsteilnehmer
│   │   │       │   ├── data
│   │   │       │   ├── hooks
│   │   │       │   └── utils
│   │   │       └── plausibilisierung
│   │   ├── navigationsansicht
│   │   │   ├── fehlerliste
│   │   │   ├── table-columns
│   │   │   └── utils
│   │   └── plugins-config
│   ├── hooks
│   │   └── data-table
│   │       └── utils
│   ├── i18n
│   │   └── translations (*.json)
│   ├── layouts (*.scss *.tsx *.test.tsx)
│   ├── pages
│   │   ├── batchjob-monitor
│   │   ├── dashboard
│   │   ├── detailansicht
│   │   ├── error
│   │   ├── export-overview
│   │   └── navigationsansicht
│   ├── routes
│   │   └── loaders
│   │       └── utils
│   ├── stores
│   ├── theme
│   └── utils
└── tests (*.ts *.json)
    ├── data
    ├── fixtures
    ├── integration-tests
    ├── mocks
    └── utils
```

Example of an playwright typescript simple project:

```text
. (root: *.code-workspace) *.json *.ts *.md .gitlab-ci.yml .gitignore)
├── devcontainer (devcontainer-lock.json devcontainer.json)
├── fixtures (*.ts)
├── helpers (*.ts)
├── node_modules <ignored>
playwright-junit-reporter
├── pages (*.ts)
├── results <ignored>
├── services (*.ts)
│   ├── infrastructure
│   └── model
├── test-results <ignored>
└── tests (*.ts)
    ├── integration
    ├── performance
    └── ui
```

Example of an ansible monorepo project:

```text
. (pyproject.toml yamlfmt.yml requirements.yml *.md *.code-workspace ansible.cfg pyproject.toml .ansible-lint .dockerignore .ensure-ansiblevaulted.yml .gitlab-ci.yml .pre-commit-config.yaml)
├── accso
│   ├── docs (*.md)
│   ├── meta (*.yml)
│   ├── molecule (*.yml *.md *.j2)
│   │   └── shared
│   │       └── templates
│   │           └── vagrant
│   ├── plugins
│   │   └── filter (*.py)
│   │       └── __pycache__ <ignored>
│   └── roles (*.yml *.md *.j2 *.py)
│       ├── gaeko_instance
│       │   ├── defaults
│       │   ├── docs
│       │   ├── handlers
│       │   ├── meta
│       │   ├── tasks
│       │   ├── templates
│       │   ├── tests
│       │   └── vars
│       ├── gitlab_runner_podman
│       │   ├── defaults
│       │   ├── files
│       │   ├── handlers
│       │   ├── meta
│       │   ├── tasks
│       │   ├── templates
│       │   ├── tests
│       │   └── vars
├── ansible_collections <ignored>
├── config
│   └── ssh
├── dc <several .devcontainer configurations>
│   ├── alpine
│   └── standard-not-working
├── fact_cache <ignored>
├── features (*.md)
├── project_ansible.egg-info <ignored> (*.txt)
├── inventories (*.yml)
│   ├── production
│   ├── staging
│   │   └── group_vars
│   │       └── all
│   └── testing
├── kc-backup (*.yml)
├── playbooks (*.yml)
│   └── templates (*.j2)
├── resources
│   ├── img (*.jpg *.svg or other image formats))
│   └── kc (*.yml)
├── scripts (*.sh)
└── tests (*.py)
    └── __pycache__ <ignored>
```

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
