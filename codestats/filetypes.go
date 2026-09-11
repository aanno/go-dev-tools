package main

import (
	"path/filepath"
	"strings"
)

func shouldSkipDir(name string) bool {
	skip := []string{
		"target", ".git", ".mvn", "node_modules", "dist", "build", ".idea", ".vscode",
		// Python
		"__pycache__", ".pytest_cache", "venv", ".venv",
		// Java/Kotlin Gradle cache
		".gradle",
		// Ansible
		"ansible_collections",
		// JS/TS monorepo tool caches
		".next", ".nuxt", ".angular", ".turbo", ".nx",
		// Test-run artifacts (e.g. Playwright)
		"test-results",
	}
	for _, s := range skip {
		if name == s {
			return true
		}
	}
	// Python packaging metadata dirs are named "<pkg>.egg-info".
	return strings.HasSuffix(name, ".egg-info")
}

// codeExts is every extension isCodeFile and detectLanguage know about.
// isCodeFile gates the file walk (see snapshot.go); detectLanguage feeds
// categorize.go's categorizeFile.
var codeExts = map[string]string{
	".java": "java", ".kt": "kotlin", ".scala": "scala", ".groovy": "groovy",
	".py": "python", ".rb": "ruby",
	".js": "javascript", ".jsx": "javascript", ".mjs": "javascript", ".cjs": "javascript",
	".ts": "typescript", ".tsx": "typescript",
	".go": "go", ".rs": "rust",
	".xml": "xml", ".properties": "properties",
	".yaml": "yaml", ".yml": "yaml", ".json": "json",
	".sh": "shell", ".bash": "shell", ".zsh": "shell",
	".md": "markdown", ".txt": "text", ".rst": "text", ".adoc": "text",
	".html": "html", ".scss": "scss", ".css": "css",
	".j2": "jinja2",
	// Build/project-descriptor-only extensions (see categorize.go's
	// buildOnlyExts - any file with these is routed to Build regardless of
	// its exact name).
	".gradle": "gradle", ".kts": "kotlin", ".sbt": "sbt",
	".toml": "toml", ".cfg": "ini", ".ini": "ini",
	// Podman Quadlet unit extensions.
	".container": "quadlet", ".pod": "quadlet", ".volume": "quadlet",
	".network": "quadlet", ".kube": "quadlet", ".build": "quadlet", ".image": "quadlet",
}

// extensionlessCodeFiles is for build descriptors that don't have a
// filepath.Ext Go recognizes as an extension at all (a single-dot dotfile
// like ".ansible-lint" reports its whole name as the "extension").
var extensionlessCodeFiles = map[string]bool{
	".ansible-lint": true,
}

func isCodeFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	base := strings.ToLower(filepath.Base(path))

	if _, ok := codeExts[ext]; ok {
		return true
	}
	if extensionlessCodeFiles[base] {
		return true
	}

	// Dockerfiles have no extension of their own.
	if strings.HasPrefix(base, "dockerfile") || strings.HasSuffix(base, ".dockerfile") ||
		strings.HasPrefix(base, "docker-compose") || strings.HasPrefix(base, "podman-compose") {
		return true
	}

	return false
}

func detectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if lang, ok := codeExts[ext]; ok {
		return lang
	}
	return "other"
}
