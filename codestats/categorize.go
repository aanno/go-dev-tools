package main

import (
	"path/filepath"
	"strings"
)

// categorizeFile determines a file's FileType and detected language from
// its path alone.
//
// The design is deliberately ecosystem-agnostic rather than one big
// per-ecosystem if-chain: a handful of generic signals (a path segment
// named "test", a filename ending "_test.go", ...) happen to be exactly how
// Maven, Gradle, sbt, Go, Python, and JS/TS test frameworks all mark test
// code and resources, so one set of rules covers all of them. None of the
// signals are anchored to the repo root, so monorepos (multiple Maven
// modules, Nx/Turborepo apps/ and packages/, ...) need no special handling
// either - the same rule matches wherever in the tree it occurs.
func categorizeFile(path string) (FileType, string) {
	path = filepath.ToSlash(path)
	base := filepath.Base(path)
	segments := strings.Split(path, "/")
	language := detectLanguage(path)

	if isBuildFile(path, base) {
		return Build, language
	}

	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".sh" || ext == ".bash" || ext == ".zsh" {
		return Scripts, "shell"
	}
	if ext == ".md" || ext == ".txt" || ext == ".rst" || ext == ".adoc" {
		return Documentation, "text"
	}

	// Everything below only fires for a recognized source language; an
	// extension we don't know at all stays Other regardless of which
	// directory it's in.
	if language == "other" {
		return Other, language
	}

	isTest := isTestPath(segments, base)
	isResource := isResourcePath(segments)

	switch {
	case isTest && isResource:
		return TestResources, language
	case isTest:
		return TestCode, language
	case isResource:
		return MainResources, language
	default:
		return MainCode, language
	}
}

// testSegments are directory names that mark everything under them as test
// code, across ecosystems: Maven/Gradle/sbt ("test"), Jest/Playwright/Nx
// ("__tests__", "e2e"), RSpec/Mocha ("spec"/"specs"), Cypress, and Ansible's
// Molecule test framework.
var testSegments = map[string]bool{
	"test": true, "tests": true, "__tests__": true,
	"spec": true, "specs": true,
	"e2e": true, "cypress": true, "molecule": true,
}

// resourceSegments mark non-code, non-test data: Maven/Gradle/sbt
// ("resources"), web build tooling ("static", "public"), and template
// directories (Ansible roles' templates/, web templating).
var resourceSegments = map[string]bool{
	"resources": true, "resource": true,
	"assets": true, "static": true, "public": true, "templates": true,
}

func isTestPath(segments []string, base string) bool {
	if isTestFilename(base) {
		return true
	}
	for _, seg := range segments {
		if testSegments[strings.ToLower(seg)] {
			return true
		}
	}
	return false
}

func isResourcePath(segments []string) bool {
	for _, seg := range segments {
		if resourceSegments[strings.ToLower(seg)] {
			return true
		}
	}
	return false
}

// isTestFilename recognizes filename-based test conventions that aren't
// signaled by any directory name: Go's colocated "_test.go", Python's
// pytest "test_*.py"/"*_test.py" (plus "conftest.py", pytest's fixture
// file), JS/TS's "*.test.ts"/"*.spec.ts" (Jest/Vitest/Playwright/Angular),
// and the JVM's "FooTest.java"/"FooTests.java" convention.
func isTestFilename(base string) bool {
	if base == "conftest.py" {
		return true
	}

	lower := strings.ToLower(base)
	if strings.HasSuffix(lower, "_test.go") {
		return true
	}
	if strings.HasSuffix(lower, "_test.py") ||
		(strings.HasPrefix(lower, "test_") && strings.HasSuffix(lower, ".py")) {
		return true
	}
	for _, ext := range []string{".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs"} {
		if strings.HasSuffix(lower, ".test"+ext) || strings.HasSuffix(lower, ".spec"+ext) {
			return true
		}
	}

	// JVM convention: checked case-sensitively (on the original base, not
	// lower) so e.g. "Latest.java" doesn't false-match "*Test.java" - a
	// case-insensitive suffix check can't tell "La"+"test" from "Foo"+"Test".
	for _, ext := range []string{".java", ".kt", ".scala"} {
		if strings.HasSuffix(base, "Test"+ext) || strings.HasSuffix(base, "Tests"+ext) {
			return true
		}
	}

	return false
}

// quadletExts are Podman Quadlet unit file extensions (systemd-unit-style
// files that define containers, pods, volumes, networks, or images/builds
// to run under systemd).
var quadletExts = map[string]bool{
	".container": true, ".pod": true, ".volume": true,
	".network": true, ".kube": true, ".build": true, ".image": true,
}

// buildOnlyExts are extensions that are essentially always project/build
// configuration, regardless of filename: Gradle (.gradle/.kts), sbt (.sbt),
// and generic config formats (.toml/.cfg/.ini) used by Cargo, Python
// packaging, Ansible, etc.
var buildOnlyExts = map[string]bool{
	".gradle": true, ".kts": true, ".sbt": true,
	".toml": true, ".cfg": true, ".ini": true,
}

// buildBasenames are specific well-known build/project-descriptor
// filenames whose extension alone (.xml, .py, .json, .txt, .yml, .yaml) is
// too generic to route to Build unconditionally.
var buildBasenames = map[string]bool{
	"pom.xml":                 true,
	"setup.py":                true,
	"package.json":            true,
	"package-lock.json":       true,
	"angular.json":            true,
	"requirements.txt":        true,
	"requirements.yml":        true,
	".ansible-lint":           true,
	".pre-commit-config.yaml": true,
	".gitlab-ci.yml":          true,
}

// isBuildFile reports whether path is project/build plumbing rather than
// application code: containers and compose files, Podman Quadlet units,
// and build/project descriptors across ecosystems.
func isBuildFile(path, base string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if quadletExts[ext] || buildOnlyExts[ext] {
		return true
	}

	lowerBase := strings.ToLower(base)
	if buildBasenames[lowerBase] {
		return true
	}
	if strings.HasPrefix(lowerBase, "tsconfig") && strings.HasSuffix(lowerBase, ".json") {
		return true
	}

	// Containers/compose. Compose has, since the Compose Specification,
	// dropped the "docker-" prefix requirement - a bare "compose.yaml" is
	// as standard as "docker-compose.yml" now, so both need matching.
	if strings.HasPrefix(lowerBase, "dockerfile") || strings.HasSuffix(lowerBase, ".dockerfile") ||
		strings.HasPrefix(lowerBase, "docker-compose") || strings.HasPrefix(lowerBase, "podman-compose") ||
		lowerBase == "compose.yml" || lowerBase == "compose.yaml" ||
		lowerBase == "compose.override.yml" || lowerBase == "compose.override.yaml" {
		return true
	}

	if strings.Contains(path, "/.github/") || strings.HasPrefix(path, ".github/") {
		return true
	}

	return false
}
