package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeRepoFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", name, err)
	}
}

func trimNL(s string) string {
	return strings.TrimRight(s, "\n")
}

// setupDeletionTestRepo builds: c1 (Alice adds lineA, lineB) -> c2 (Bob
// adds lineC) -> c3 (Carol deletes lineA). lineB survives untouched after
// c1 (a boundary line for the (c1,c3] range); lineA is deleted at c3 but
// originated from Alice; lineC survives, added by Bob within the range.
func setupDeletionTestRepo(t *testing.T) (repoRoot, c1, c3 string) {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
		return string(out)
	}
	runQuiet := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}

	runQuiet("init", "-q")
	runQuiet("config", "user.email", "alice@example.com")
	runQuiet("config", "user.name", "Alice")
	writeRepoFile(t, dir, "f.txt", "lineA\nlineB\n")
	runQuiet("add", "f.txt")
	runQuiet("commit", "-q", "-m", "c1 alice adds A and B")
	c1Hash := trimNL(run("rev-parse", "HEAD"))

	runQuiet("config", "user.email", "bob@example.com")
	runQuiet("config", "user.name", "Bob")
	writeRepoFile(t, dir, "f.txt", "lineA\nlineB\nlineC\n")
	runQuiet("commit", "-qam", "c2 bob adds C")

	runQuiet("config", "user.email", "carol@example.com")
	runQuiet("config", "user.name", "Carol")
	writeRepoFile(t, dir, "f.txt", "lineB\nlineC\n")
	runQuiet("commit", "-qam", "c3 carol deletes A")
	c3Hash := trimNL(run("rev-parse", "HEAD"))

	return dir, c1Hash, c3Hash
}

// TestComputeDeletionStats_SelfOverwriteExcluded reproduces the reported
// GitLab-CI-bumping-its-own-version-string scenario: the same author
// repeatedly deletes their own previous line and replaces it. None of
// that should count as a "loss to another author" - only Bob's deletion
// of one of CI's lines should.
func TestComputeDeletionStats_SelfOverwriteExcluded(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
		return string(out)
	}
	runQuiet := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}

	runQuiet("init", "-q")
	runQuiet("config", "user.email", "ci@example.com")
	runQuiet("config", "user.name", "GitLab")
	writeRepoFile(t, dir, "f.txt", "version=1.0.0-SNAPSHOT\n")
	runQuiet("add", "f.txt")
	runQuiet("commit", "-q", "-m", "c1 ci adds version line")
	c1 := trimNL(run("rev-parse", "HEAD"))

	// GitLab bumps its own line three times in a row (self-overwrite).
	writeRepoFile(t, dir, "f.txt", "version=1.0.0\n")
	runQuiet("commit", "-qam", "c2 ci bumps to release")
	writeRepoFile(t, dir, "f.txt", "version=1.0.1-SNAPSHOT\n")
	runQuiet("commit", "-qam", "c3 ci bumps to next dev")
	writeRepoFile(t, dir, "f.txt", "version=1.0.1\n")
	runQuiet("commit", "-qam", "c4 ci bumps to release again")

	// Bob then deletes CI's line outright - this one SHOULD count,
	// crediting GitLab (the original author), not Bob (the deleter).
	runQuiet("config", "user.email", "bob@example.com")
	runQuiet("config", "user.name", "Bob")
	writeRepoFile(t, dir, "f.txt", "")
	runQuiet("commit", "-qam", "c5 bob removes the version line entirely")
	c5 := trimNL(run("rev-parse", "HEAD"))

	merger := &AuthorMerger{emailToCanonical: map[string]string{}, nameToCanonical: map[string]string{}}
	counts, err := computeDeletionStats(dir, c1, c5, merger)
	if err != nil {
		t.Fatalf("computeDeletionStats: %v", err)
	}

	if counts.ByAuthor["GitLab"] != 1 {
		t.Errorf(`ByAuthor["GitLab"] = %d, want 1 (only Bob's deletion counts, not CI's 2 self-overwrites)`,
			counts.ByAuthor["GitLab"])
	}
	if counts.ByAuthor["Bob"] != 0 {
		t.Errorf(`ByAuthor["Bob"] = %d, want 0 (Bob never authored a line that got deleted)`, counts.ByAuthor["Bob"])
	}
}

func TestComputeDeletionStats(t *testing.T) {
	repoRoot, c1, c3 := setupDeletionTestRepo(t)
	merger := &AuthorMerger{emailToCanonical: map[string]string{}, nameToCanonical: map[string]string{}}

	counts, err := computeDeletionStats(repoRoot, c1, c3, merger)
	if err != nil {
		t.Fatalf("computeDeletionStats: %v", err)
	}

	if counts.ByAuthor["Alice"] != 1 {
		t.Errorf("ByAuthor[Alice] = %d, want 1 (lineA, originally hers, deleted at c3)", counts.ByAuthor["Alice"])
	}
	if counts.ByAuthor["Bob"] != 0 {
		t.Errorf("ByAuthor[Bob] = %d, want 0 (Bob's lineC was never deleted)", counts.ByAuthor["Bob"])
	}
	if counts.ByType[Documentation] != 1 {
		t.Errorf("ByType[Documentation] = %d, want 1", counts.ByType[Documentation])
	}
	if counts.ByAuthorAndType[Documentation]["Alice"] != 1 {
		t.Errorf("ByAuthorAndType[Documentation][Alice] = %d, want 1", counts.ByAuthorAndType[Documentation]["Alice"])
	}
}

func TestApplyDeletions(t *testing.T) {
	repoRoot, c1, c3 := setupDeletionTestRepo(t)
	merger := &AuthorMerger{emailToCanonical: map[string]string{}, nameToCanonical: map[string]string{}}

	// Surviving-lines side, as computeRangeStats would produce it: within
	// (c1, c3], only Bob's lineC survives untouched - lineB was last
	// touched at c1 itself (the boundary), and lineA no longer exists.
	stats := &AggregatedStats{
		ByAuthorAndType: []TypeGroup{{
			Type:       string(Documentation),
			TotalLines: 1,
			Authors:    []AuthorInType{{Author: "Bob", Lines: 1, Percent: "100.00%"}},
		}},
		ByType:   []TypeStats{{Type: string(Documentation), Lines: 1, Percent: "100.00%"}},
		ByAuthor: []AuthorStats{{Author: "Bob", Lines: 1, Percent: "100.00%"}},
		Total:    TotalStats{Lines: 1},
	}

	counts, err := computeDeletionStats(repoRoot, c1, c3, merger)
	if err != nil {
		t.Fatalf("computeDeletionStats: %v", err)
	}
	applyDeletions(stats, counts)

	// ByAuthor: Alice must appear even though she has 0 surviving lines,
	// and the slice must be sorted by Sum descending (Bob:+1, Alice:-1),
	// not by Lines.
	if len(stats.ByAuthor) != 2 {
		t.Fatalf("ByAuthor = %+v, want 2 rows", stats.ByAuthor)
	}
	if stats.ByAuthor[0].Author != "Bob" || *stats.ByAuthor[0].Sum != 1 {
		t.Errorf("ByAuthor[0] = %+v, want Bob with Sum 1", stats.ByAuthor[0])
	}
	if stats.ByAuthor[1].Author != "Alice" || stats.ByAuthor[1].Lines != 0 ||
		*stats.ByAuthor[1].Deleted != 1 || *stats.ByAuthor[1].Sum != -1 {
		t.Errorf("ByAuthor[1] = %+v, want Alice with {Lines:0 Deleted:1 Sum:-1}", stats.ByAuthor[1])
	}

	// ByType and ByAuthorAndType both get Deleted/Sum set on the existing
	// Documentation row too.
	if len(stats.ByType) != 1 || *stats.ByType[0].Deleted != 1 || *stats.ByType[0].Sum != 0 {
		t.Errorf("ByType = %+v, want one Documentation row with {Deleted:1 Sum:0}", stats.ByType)
	}
	if len(stats.ByAuthorAndType) != 1 || len(stats.ByAuthorAndType[0].Authors) != 2 {
		t.Fatalf("ByAuthorAndType = %+v, want one group with 2 authors (Bob + newly added Alice)", stats.ByAuthorAndType)
	}

	if *stats.Total.Deleted != 1 || *stats.Total.Sum != 0 {
		t.Errorf("Total = {Deleted:%v Sum:%v}, want {Deleted:1 Sum:0}", stats.Total.Deleted, stats.Total.Sum)
	}
}
