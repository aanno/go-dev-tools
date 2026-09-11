package main

import (
	"os"
	"strings"
)

// countLines counts the non-blank, non-comment lines in a file.
//
// Comment stripping is skipped entirely for Documentation-typed files
// (.md/.txt/.rst/.adoc): there, a leading '#', '-' or '*' is heading/list
// markup, not a comment, so every non-blank line counts.
//
// For all other file types, a line is dropped if it is blank, a full-line
// comment (//, #, --, or a continuation line starting with *), or entirely
// inside a /* ... */ block comment. A line that shares a /* or */ delimiter
// with real code (e.g. "foo(); /*" or "*/ bar();") still counts, since it
// does carry code.
func countLines(path string, fileType FileType) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	if fileType == Documentation {
		lines := 0
		for _, line := range strings.Split(string(data), "\n") {
			if strings.TrimSpace(line) != "" {
				lines++
			}
		}
		return lines, nil
	}

	lines := 0
	inMultiComment := false

	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if inMultiComment {
			if idx := strings.Index(trimmed, "*/"); idx != -1 {
				inMultiComment = false
				after := strings.TrimSpace(trimmed[idx+2:])
				if after != "" && !isLineCommentPrefix(after) {
					lines++
				}
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
				if before != "" || (after != "" && !isLineCommentPrefix(after)) {
					lines++
				}
			} else {
				inMultiComment = true
				if before != "" {
					lines++
				}
			}
			continue
		}

		lines++
	}

	return lines, nil
}

// isLineCommentPrefix reports whether s (already trimmed) is a full-line
// comment under any of the simple line-comment styles this tool recognizes.
func isLineCommentPrefix(s string) bool {
	return strings.HasPrefix(s, "//") ||
		strings.HasPrefix(s, "#") ||
		strings.HasPrefix(s, "--") ||
		strings.HasPrefix(s, "*")
}

// countableLine reports whether a single line of *added* content, as seen in
// a diff (with no surrounding file context), should be counted - using the
// same blank/comment heuristics as countLines.
//
// Limitation: unlike countLines this has no cross-line state, so an added
// line that is part of a multi-line /* ... */ comment but doesn't itself
// carry a delimiter cannot be recognized as a comment here. This only
// affects range mode's diff-based counting (see rangemode.go).
func countableLine(content string, fileType FileType) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}
	if fileType == Documentation {
		return true
	}
	if isLineCommentPrefix(trimmed) {
		return false
	}
	if strings.HasPrefix(trimmed, "/*") && strings.HasSuffix(trimmed, "*/") {
		return false
	}
	return true
}
