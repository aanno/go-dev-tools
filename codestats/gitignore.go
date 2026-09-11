package main

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// GitignoreMatcher checks if paths should be ignored
type GitignoreMatcher struct {
	patterns []gitignorePattern
}

type gitignorePattern struct {
	pattern *regexp.Regexp
	negate  bool
	dirOnly bool
}

func loadGitignorePatterns(repoRoot string) *GitignoreMatcher {
	gitignorePath := filepath.Join(repoRoot, ".gitignore")
	file, err := os.Open(gitignorePath)
	if err != nil {
		return nil
	}
	defer file.Close()

	var patterns []gitignorePattern
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		negate := false
		if strings.HasPrefix(line, "!") {
			negate = true
			line = line[1:]
		}

		dirOnly := strings.HasSuffix(line, "/")
		line = strings.TrimSuffix(line, "/")

		// Convert gitignore pattern to regex
		regex := convertGitignorePattern(line)
		if re, err := regexp.Compile(regex); err == nil {
			patterns = append(patterns, gitignorePattern{
				pattern: re,
				negate:  negate,
				dirOnly: dirOnly,
			})
		}
	}

	if len(patterns) == 0 {
		return nil
	}

	return &GitignoreMatcher{patterns: patterns}
}

func convertGitignorePattern(pattern string) string {
	// Escape regex special chars except * and ?
	escaped := strings.ReplaceAll(pattern, ".", "\\.")
	escaped = strings.ReplaceAll(escaped, "[", "\\[")
	escaped = strings.ReplaceAll(escaped, "]", "\\]")
	escaped = strings.ReplaceAll(escaped, "(", "\\(")
	escaped = strings.ReplaceAll(escaped, ")", "\\)")
	escaped = strings.ReplaceAll(escaped, "{", "\\{")
	escaped = strings.ReplaceAll(escaped, "}", "\\}")
	escaped = strings.ReplaceAll(escaped, "^", "\\^")
	escaped = strings.ReplaceAll(escaped, "$", "\\$")
	escaped = strings.ReplaceAll(escaped, "+", "\\+")
	escaped = strings.ReplaceAll(escaped, "|", "\\|")

	// Convert glob patterns
	escaped = strings.ReplaceAll(escaped, "**", ".*")
	escaped = strings.ReplaceAll(escaped, "*", "[^/]*")
	escaped = strings.ReplaceAll(escaped, "?", ".")

	return "^" + escaped + "$"
}

func (m *GitignoreMatcher) Match(path string, isDir bool) bool {
	path = filepath.ToSlash(path)

	matched := false
	for _, p := range m.patterns {
		if p.dirOnly && !isDir {
			continue
		}

		// Check full path and basename
		if p.pattern.MatchString(path) || p.pattern.MatchString(filepath.Base(path)) {
			matched = !p.negate
		}
	}

	return matched
}
