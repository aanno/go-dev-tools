package main

import (
	"os"
	"strings"
)

// countLines counts the non-blank, non-comment lines in a file.
func countLines(path string, fileType FileType) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	lines := strings.Split(string(data), "\n")
	count := 0
	for _, countable := range linesCountable(lines, fileType) {
		if countable {
			count++
		}
	}
	return count, nil
}

// linesCountable reports, for each line in lines (already split on "\n",
// in file order), whether it is a "real" line to count - not blank, not a
// full-line comment, not entirely inside a /* ... */ block comment.
//
// Comment stripping is skipped entirely for Documentation-typed files
// (.md/.txt/.rst/.adoc): there, a leading '#', '-' or '*' is heading/list
// markup, not a comment, so every non-blank line counts.
//
// For all other file types, a line is dropped if it is blank, a full-line
// comment (//, #, --, or a continuation line starting with *), or entirely
// inside a /* ... */ block comment. A line that shares a /* or */ delimiter
// with real code (e.g. "foo(); /*" or "*/ bar();") still counts, since it
// does carry code. Because this walks the lines in order, a multi-line
// comment is tracked correctly regardless of where it starts or ends -
// callers just need to supply the file's lines in their original order
// (countLines does; so does rangemode.go's blame-based line collection).
func linesCountable(lines []string, fileType FileType) []bool {
	result := make([]bool, len(lines))

	if fileType == Documentation {
		for i, line := range lines {
			result[i] = strings.TrimSpace(line) != ""
		}
		return result
	}

	inMultiComment := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if inMultiComment {
			if idx := strings.Index(trimmed, "*/"); idx != -1 {
				inMultiComment = false
				after := strings.TrimSpace(trimmed[idx+2:])
				result[i] = after != "" && !isLineCommentPrefix(after)
			}
			continue
		}

		if isLineCommentPrefix(trimmed) {
			continue
		}

		if idx := strings.Index(trimmed, "/*"); idx != -1 {
			before := strings.TrimSpace(trimmed[:idx])
			if closeIdx := strings.Index(trimmed[idx+2:], "*/"); closeIdx != -1 {
				// Comment opens and closes on the same line.
				after := strings.TrimSpace(trimmed[idx+2+closeIdx+2:])
				result[i] = before != "" || (after != "" && !isLineCommentPrefix(after))
			} else {
				inMultiComment = true
				result[i] = before != ""
			}
			continue
		}

		result[i] = true
	}

	return result
}

// isLineCommentPrefix reports whether s (already trimmed) is a full-line
// comment under any of the simple line-comment styles this tool recognizes.
func isLineCommentPrefix(s string) bool {
	return strings.HasPrefix(s, "//") ||
		strings.HasPrefix(s, "#") ||
		strings.HasPrefix(s, "--") ||
		strings.HasPrefix(s, "*")
}
