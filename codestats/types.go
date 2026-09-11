package main

// FileType represents categorized file types
type FileType string

const (
	TestCode      FileType = "test_code"
	MainCode      FileType = "main_code"
	TestResources FileType = "test_resources"
	MainResources FileType = "main_resources"
	Scripts       FileType = "scripts"
	Documentation FileType = "documentation"
	// Build covers project/build plumbing rather than application code:
	// containers (Dockerfile, compose files), Podman Quadlet units, and
	// build/project descriptors (pom.xml, build.gradle, Cargo.toml, CI
	// config, ...). See categorize.go's isBuildFile.
	Build FileType = "build"
	Other FileType = "other"
)

// FileStats holds per-file statistics.
//
// In snapshot mode, Lines is the file's current line count (as seen on
// disk), attributed to whichever author's git-blame covers the most lines
// of that file.
//
// In range mode, Lines is the number of lines still present at toCommit
// that were last touched by that author within the given commit range (see
// rangemode.go) - a different statistic from snapshot's "current
// ownership at HEAD", scoped to a window of history rather than all of it.
type FileStats struct {
	Path     string
	Type     FileType
	Lines    int
	Author   string
	Language string
}
