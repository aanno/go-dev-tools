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

func TestCountableLine(t *testing.T) {
	cases := []struct {
		content  string
		fileType FileType
		want     bool
	}{
		{"foo();", MainCode, true},
		{"", MainCode, false},
		{"   ", MainCode, false},
		{"// comment", MainCode, false},
		{"# heading", Documentation, true},
		{"/* comment */", MainCode, false},
	}
	for _, c := range cases {
		got := countableLine(c.content, c.fileType)
		if got != c.want {
			t.Errorf("countableLine(%q, %v) = %v, want %v", c.content, c.fileType, got, c.want)
		}
	}
}
