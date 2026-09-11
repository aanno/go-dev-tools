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
	Container     FileType = "container"
	Other         FileType = "other"
)

// FileStats holds per-file statistics.
//
// In snapshot mode, Lines is the file's current line count (as seen on
// disk), attributed to whichever author's git-blame covers the most lines
// of that file.
//
// In range mode, Lines is the number of lines that author *added* to that
// file within the given commit range (see rangemode.go) - a different
// statistic from snapshot's "current ownership", computed from commit diffs
// rather than blame/annotate.
type FileStats struct {
	Path     string
	Type     FileType
	Lines    int
	Author   string
	Language string
}
