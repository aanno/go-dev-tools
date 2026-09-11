package main

import (
	"path/filepath"
	"strings"
)

func shouldSkipDir(name string) bool {
	skip := []string{"target", ".git", ".mvn", "node_modules", "dist", "build", ".idea", ".vscode"}
	for _, s := range skip {
		if name == s {
			return true
		}
	}
	return false
}

func isCodeFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	base := strings.ToLower(filepath.Base(path))

	codeExts := []string{
		".java", ".kt", ".scala", ".groovy",
		".py", ".rb", ".js", ".ts", ".go", ".rs",
		".xml", ".properties", ".yaml", ".yml", ".json",
		".sh", ".bash", ".zsh",
		".md", ".txt", ".rst", ".adoc",
	}

	for _, e := range codeExts {
		if strings.HasSuffix(ext, e) {
			return true
		}
	}

	// Dockerfiles
	if strings.HasPrefix(base, "dockerfile") || strings.HasSuffix(base, ".dockerfile") ||
		strings.HasPrefix(base, "docker-compose") {
		return true
	}

	return false
}

// categorizeFile determines a file's FileType and detected language from its
// path alone (Maven standard layout awareness plus extension/name rules).
func categorizeFile(path string) (FileType, string) {
	// Normalize path separators
	path = filepath.ToSlash(path)

	// Check path patterns first (Maven standard layout)
	if strings.Contains(path, "/src/test/") {
		if strings.Contains(path, "/resources/") {
			return TestResources, detectLanguage(path)
		}
		return TestCode, detectLanguage(path)
	}

	if strings.Contains(path, "/src/main/") {
		if strings.Contains(path, "/resources/") {
			return MainResources, detectLanguage(path)
		}
		return MainCode, detectLanguage(path)
	}

	// Check by extension
	ext := strings.ToLower(filepath.Ext(path))
	base := strings.ToLower(filepath.Base(path))

	if ext == ".sh" || ext == ".bash" || ext == ".zsh" {
		return Scripts, "shell"
	}

	if ext == ".md" || ext == ".txt" || ext == ".rst" || ext == ".adoc" {
		return Documentation, "text"
	}

	if strings.HasPrefix(base, "dockerfile") || strings.HasSuffix(base, ".dockerfile") ||
		strings.HasPrefix(base, "docker-compose") {
		return Container, "docker"
	}

	// Check for other common patterns
	if ext == ".xml" && strings.Contains(path, "/pom.xml") {
		return Other, "maven"
	}

	// Default to other
	return Other, detectLanguage(path)
}

func detectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	langMap := map[string]string{
		".java": "java", ".kt": "kotlin", ".scala": "scala", ".groovy": "groovy",
		".py": "python", ".rb": "ruby", ".js": "javascript", ".ts": "typescript",
		".go": "go", ".rs": "rust", ".xml": "xml", ".properties": "properties",
		".yaml": "yaml", ".yml": "yaml", ".json": "json", ".md": "markdown",
		".txt": "text", ".sh": "shell", ".bash": "shell",
	}
	if lang, ok := langMap[ext]; ok {
		return lang
	}
	return "other"
}
