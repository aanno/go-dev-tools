package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	return path
}

func TestCountLines_CodeAndBlankAndComments(t *testing.T) {
	content := "package main\n" +
		"\n" + // blank line, not counted
		"// full-line comment, not counted\n" +
		"foo(); /*\n" + // code sharing line with comment start - must count
		"this is inside a multi-line comment, not counted\n" +
		"*/ bar();\n" + // comment closes with trailing code - must count
		"/* pure single-line comment */\n" + // not counted
		"baz()\n"

	path := writeTempFile(t, "sample.go", content)

	lines, err := countLines(path, MainCode)
	if err != nil {
		t.Fatalf("countLines returned error: %v", err)
	}

	// package main / foo();/* / */bar(); / baz()
	const want = 4
	if lines != want {
		t.Errorf("countLines() = %d, want %d", lines, want)
	}
}

func TestCountLines_DocumentationSkipsCommentHeuristics(t *testing.T) {
	content := "# Heading\n" +
		"\n" +
		"- bullet one\n" +
		"- bullet two\n" +
		"Some text.\n"

	path := writeTempFile(t, "sample.md", content)

	lines, err := countLines(path, Documentation)
	if err != nil {
		t.Fatalf("countLines returned error: %v", err)
	}

	// Every non-blank line counts: heading + 2 bullets + text = 4.
	const want = 4
	if lines != want {
		t.Errorf("countLines() = %d, want %d", lines, want)
	}
}

func TestLinesCountable_MultiLineCommentAcrossCalls(t *testing.T) {
	// rangemode.go feeds linesCountable the file's lines as reconstructed
	// from git blame, in order - this exercises that same multi-line
	// /* ... */ tracking a fragmented, per-line diff view couldn't do.
	lines := []string{
		"foo(); /*",
		"still inside the comment",
		"*/ bar();",
		"// full comment",
		"baz();",
		"/* single line comment */",
	}

	got := linesCountable(lines, MainCode)
	want := []bool{true, false, true, false, true, false}

	if len(got) != len(want) {
		t.Fatalf("linesCountable() returned %d entries, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("linesCountable()[%d] (%q) = %v, want %v", i, lines[i], got[i], want[i])
		}
	}
}
