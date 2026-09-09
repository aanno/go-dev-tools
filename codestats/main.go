// cmd/codestats/main.go - Updated with .gitignore support

package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/go-git/go-git/v5"
	"github.com/olekukonko/tablewriter"
	"gopkg.in/yaml.v3"
)

// FileType represents categorized file types
type FileType string

const (
	TestCode       FileType = "test_code"
	MainCode       FileType = "main_code"
	TestResources  FileType = "test_resources"
	MainResources  FileType = "main_resources"
	Scripts        FileType = "scripts"
	Documentation  FileType = "documentation"
	Container      FileType = "container"
	Other          FileType = "other"
)

// FileStats holds per-file statistics
type FileStats struct {
	Path     string
	Type     FileType
	Lines    int
	Author   string
	Language string
}

// AuthorConfig for merging author identities
type AuthorConfig struct {
	Authors []AuthorMapping `yaml:"authors"`
}

type AuthorMapping struct {
	Canonical string   `yaml:"canonical"`
	Emails    []string `yaml:"emails"`
	Names     []string `yaml:"names"`
}

// AggregatedStats for output
type AggregatedStats struct {
	ByAuthorAndType []AuthorTypeStats `json:"by_author_and_type"`
	ByType          []TypeStats       `json:"by_type"`
	ByAuthor        []AuthorStats     `json:"by_author"`
	Total           TotalStats        `json:"total"`
}

type AuthorTypeStats struct {
	Author  string `json:"author"`
	Type    string `json:"type"`
	Lines   int    `json:"lines"`
	Percent string `json:"percent"`
}

type TypeStats struct {
	Type    string `json:"type"`
	Lines   int    `json:"lines"`
	Percent string `json:"percent"`
}

type AuthorStats struct {
	Author  string `json:"author"`
	Lines   int    `json:"lines"`
	Percent string `json:"percent"`
}

type TotalStats struct {
	Lines int `json:"total_lines"`
}

// GitignoreMatcher checks if paths should be ignored
type GitignoreMatcher struct {
	patterns []gitignorePattern
}

type gitignorePattern struct {
	pattern *regexp.Regexp
	negate  bool
	dirOnly bool
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: codestats <repo-path> [--from COMMIT] [--to COMMIT] [--config authors.yaml] [--output DIR] [--mode snapshot|range]")
	}

	repoPath := os.Args[1]
	configPath := "authors.yaml"
	fromCommit := ""
	toCommit := ""
	outputPath := "."
	mode := "snapshot"

	// Parse optional args
	for i := 2; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--config":
			i++
			if i < len(os.Args) {
				configPath = os.Args[i]
			}
		case "--from":
			i++
			if i < len(os.Args) {
				fromCommit = os.Args[i]
			}
		case "--to":
			i++
			if i < len(os.Args) {
				toCommit = os.Args[i]
			}
		case "--output":
			i++
			if i < len(os.Args) {
				outputPath = os.Args[i]
			}
		case "--mode":
			i++
			if i < len(os.Args) {
				mode = os.Args[i]
			}
		}
	}

	// Validate mode
	if mode != "snapshot" && mode != "range" {
		log.Fatal("Mode must be 'snapshot' or 'range'")
	}

	if mode == "range" && (fromCommit == "" || toCommit == "") {
		log.Fatal("Range mode requires --from and --to commit hashes")
	}

	// Load author config
	authorMerger := loadAuthorConfig(configPath)

	// Open git repo
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		log.Fatalf("Failed to open repo: %v", err)
	}

	// Get repo root
	worktree, err := repo.Worktree()
	if err != nil {
		log.Fatalf("Failed to get worktree: %v", err)
	}
	repoRoot := worktree.Filesystem.Root()

	// Load .gitignore patterns
	gitignoreMatcher := loadGitignorePatterns(repoRoot)

	// Walk and collect files
	files := make(chan string, 100)
	var wg sync.WaitGroup

	// Start file walker
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				if shouldSkipDir(info.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			
			// Check gitignore
			relPath, _ := filepath.Rel(repoRoot, path)
			if gitignoreMatcher != nil && gitignoreMatcher.Match(relPath, info.IsDir()) {
				return nil
			}
			
			if isCodeFile(path) {
				files <- path
			}
			return nil
		})
		if err != nil {
			log.Printf("Warning: walk error: %v", err)
		}
		close(files)
	}()

	// Process files in parallel
	statsChan := make(chan FileStats, 100)
	var processWg sync.WaitGroup

	numWorkers := 4
	for i := 0; i < numWorkers; i++ {
		processWg.Add(1)
		go func() {
			defer processWg.Done()
			for path := range files {
				stat := processFile(repo, path, repoRoot, fromCommit, toCommit, mode, authorMerger)
				if stat != nil {
					statsChan <- *stat
				}
			}
		}()
	}

	go func() {
		processWg.Wait()
		close(statsChan)
	}()

	// Collect results
	var allStats []FileStats
	for stat := range statsChan {
		allStats = append(allStats, stat)
	}

	wg.Wait()

	if len(allStats) == 0 {
		log.Fatal("No files processed")
	}

	// Aggregate and output
	aggregated := aggregateStats(allStats)
	outputCSV(aggregated, outputPath)
	outputJSON(aggregated, outputPath)
	outputTable(aggregated)
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

func processFile(repo *git.Repository, path string, repoRoot string, fromCommit, toCommit string, mode string, merger *AuthorMerger) *FileStats {
	// Count lines
	lines, err := countLines(path)
	if err != nil {
		return nil
	}

	if lines == 0 {
		return nil
	}

	// Get author
	author := getAuthor(repoRoot, path, fromCommit, toCommit, mode)
	if author == "unknown" {
		// Silently skip files we can't get author for
		return nil
	}
	author = merger.Canonicalize(author)

	// Categorize
	fileType, language := categorizeFile(path)

	return &FileStats{
		Path:     path,
		Type:     fileType,
		Lines:    lines,
		Author:   author,
		Language: language,
	}
}

func countLines(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	lines := 0
	inMultiComment := false

	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)

		// Skip empty lines
		if trimmed == "" {
			continue
		}

		// Handle multi-line comments (/* ... */)
		if inMultiComment {
			if strings.Contains(trimmed, "*/") {
				inMultiComment = false
			}
			continue
		}

		// Skip single-line comments
		if strings.HasPrefix(trimmed, "//") ||
			strings.HasPrefix(trimmed, "#") ||
			strings.HasPrefix(trimmed, "--") ||
			strings.HasPrefix(trimmed, "*") {
			continue
		}

		// Check for multi-line comment start
		if strings.Contains(trimmed, "/*") {
			if !strings.Contains(trimmed, "*/") {
				inMultiComment = true
			}
			// If comment starts and ends on same line, skip it
			if strings.Contains(trimmed, "*/") {
				continue
			}
			continue
		}

		// Count non-empty, non-comment line
		lines++
	}

	return lines, nil
}

func getAuthor(repoRoot string, path string, fromCommit, toCommit string, mode string) string {
	// Get relative path from repo root
	relPath, err := filepath.Rel(repoRoot, path)
	if err != nil {
		relPath = path
	}

	// Normalize path separators for git
	relPath = filepath.ToSlash(relPath)

	if mode == "snapshot" {
		return getAuthorBlame(repoRoot, relPath)
	}

	return "unknown" // Range mode not fully implemented yet
}

func getAuthorBlame(repoRoot string, path string) string {
	// Use git blame CLI (most reliable)
	cmd := exec.Command("git", "-C", repoRoot, "blame", "--line-porcelain", path)
	output, err := cmd.Output()
	if err != nil {
		// Silently ignore files that can't be blamed
		return "unknown"
	}

	authorCounts := make(map[string]int)
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "author ") {
			author := strings.TrimPrefix(line, "author ")
			authorCounts[author]++
		}
	}

	return getTopAuthor(authorCounts)
}

func getTopAuthor(counts map[string]int) string {
	if len(counts) == 0 {
		return "unknown"
	}

	maxCount := 0
	topAuthor := "unknown"
	for author, count := range counts {
		if count > maxCount {
			maxCount = count
			topAuthor = author
		}
	}
	return topAuthor
}

// AuthorMerger handles author identity consolidation
type AuthorMerger struct {
	emailToCanonical map[string]string
	nameToCanonical  map[string]string
}

func loadAuthorConfig(path string) *AuthorMerger {
	merger := &AuthorMerger{
		emailToCanonical: make(map[string]string),
		nameToCanonical:  make(map[string]string),
	}

	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("No author config found at %s, using raw authors", path)
		return merger
	}

	var config AuthorConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		log.Printf("Failed to parse author config: %v", err)
		return merger
	}

	for _, mapping := range config.Authors {
		for _, email := range mapping.Emails {
			merger.emailToCanonical[strings.ToLower(email)] = mapping.Canonical
		}
		for _, name := range mapping.Names {
			merger.nameToCanonical[strings.ToLower(name)] = mapping.Canonical
		}
	}

	log.Printf("Loaded author config with %d mappings", len(config.Authors))
	return merger
}

func (m *AuthorMerger) Canonicalize(author string) string {
	if author == "" || author == "unknown" {
		return author
	}

	// Try exact name match first
	if canonical, ok := m.nameToCanonical[strings.ToLower(author)]; ok {
		return canonical
	}

	// Try email match (author might be "Name <email>")
	if strings.Contains(author, "<") && strings.Contains(author, ">") {
		re := regexp.MustCompile(`<([^>]+)>`)
		matches := re.FindStringSubmatch(author)
		if len(matches) > 1 {
			email := strings.ToLower(matches[1])
			if canonical, ok := m.emailToCanonical[email]; ok {
				return canonical
			}
		}
	}

	// Try extracting just the name part
	if strings.Contains(author, "<") {
		name := strings.TrimSpace(strings.Split(author, "<")[0])
		if canonical, ok := m.nameToCanonical[strings.ToLower(name)]; ok {
			return canonical
		}
	}

	return author
}

func aggregateStats(stats []FileStats) *AggregatedStats {
	authorTypeLines := make(map[string]map[FileType]int)
	typeLines := make(map[FileType]int)
	authorLines := make(map[string]int)
	totalLines := 0

	for _, s := range stats {
		// By author and type
		if authorTypeLines[s.Author] == nil {
			authorTypeLines[s.Author] = make(map[FileType]int)
		}
		authorTypeLines[s.Author][s.Type] += s.Lines

		// By type
		typeLines[s.Type] += s.Lines

		// By author
		authorLines[s.Author] += s.Lines

		// Total
		totalLines += s.Lines
	}

	// Build result
	result := &AggregatedStats{}

	// By author and type (sorted by lines desc, then alphabetically)
	var authorTypeRows []AuthorTypeStats
	for author, types := range authorTypeLines {
		for fileType, lines := range types {
			percent := fmt.Sprintf("%.2f%%", float64(lines)*100/float64(totalLines))
			authorTypeRows = append(authorTypeRows, AuthorTypeStats{
				Author:  author,
				Type:    string(fileType),
				Lines:   lines,
				Percent: percent,
			})
		}
	}
	sort.Slice(authorTypeRows, func(i, j int) bool {
		if authorTypeRows[i].Lines != authorTypeRows[j].Lines {
			return authorTypeRows[i].Lines > authorTypeRows[j].Lines
		}
		if authorTypeRows[i].Author != authorTypeRows[j].Author {
			return authorTypeRows[i].Author < authorTypeRows[j].Author
		}
		return authorTypeRows[i].Type < authorTypeRows[j].Type
	})
	result.ByAuthorAndType = authorTypeRows

	// By type (sorted by lines desc, then alphabetically)
	var typeRows []TypeStats
	for fileType, lines := range typeLines {
		percent := fmt.Sprintf("%.2f%%", float64(lines)*100/float64(totalLines))
		typeRows = append(typeRows, TypeStats{
			Type:    string(fileType),
			Lines:   lines,
			Percent: percent,
		})
	}
	sort.Slice(typeRows, func(i, j int) bool {
		if typeRows[i].Lines != typeRows[j].Lines {
			return typeRows[i].Lines > typeRows[j].Lines
		}
		return typeRows[i].Type < typeRows[j].Type
	})
	result.ByType = typeRows

	// By author (sorted by lines desc, then alphabetically)
	var authorRows []AuthorStats
	for author, lines := range authorLines {
		percent := fmt.Sprintf("%.2f%%", float64(lines)*100/float64(totalLines))
		authorRows = append(authorRows, AuthorStats{
			Author:  author,
			Lines:   lines,
			Percent: percent,
		})
	}
	sort.Slice(authorRows, func(i, j int) bool {
		if authorRows[i].Lines != authorRows[j].Lines {
			return authorRows[i].Lines > authorRows[j].Lines
		}
		return authorRows[i].Author < authorRows[j].Author
	})
	result.ByAuthor = authorRows

	result.Total = TotalStats{Lines: totalLines}

	return result
}

func outputCSV(stats *AggregatedStats, outputPath string) {
	csvPath := filepath.Join(outputPath, "codestats.csv")

	// Check if file exists
	if _, err := os.Stat(csvPath); err == nil {
		log.Fatalf("Output file %s already exists. Remove it or use --output to specify different directory.", csvPath)
	}

	file, err := os.Create(csvPath)
	if err != nil {
		log.Fatalf("Failed to create CSV: %v", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	if err := writer.Write([]string{"category", "author", "type", "lines", "percent"}); err != nil {
		log.Fatalf("Failed to write CSV header: %v", err)
	}

	// By author and type
	for _, row := range stats.ByAuthorAndType {
		if err := writer.Write([]string{"by_author_and_type", row.Author, row.Type,
			strconv.Itoa(row.Lines), row.Percent}); err != nil {
			log.Fatalf("Failed to write CSV: %v", err)
		}
	}

	// By type
	for _, row := range stats.ByType {
		if err := writer.Write([]string{"by_type", "", row.Type,
			strconv.Itoa(row.Lines), row.Percent}); err != nil {
			log.Fatalf("Failed to write CSV: %v", err)
		}
	}

	// By author
	for _, row := range stats.ByAuthor {
		if err := writer.Write([]string{"by_author", row.Author, "",
			strconv.Itoa(row.Lines), row.Percent}); err != nil {
			log.Fatalf("Failed to write CSV: %v", err)
		}
	}

	// Total
	if err := writer.Write([]string{"total", "", "", strconv.Itoa(stats.Total.Lines), "100.00%"}); err != nil {
		log.Fatalf("Failed to write CSV: %v", err)
	}

	log.Printf("CSV written to %s", csvPath)
}

func outputJSON(stats *AggregatedStats, outputPath string) {
	jsonPath := filepath.Join(outputPath, "codestats.json")

	// Check if file exists
	if _, err := os.Stat(jsonPath); err == nil {
		log.Fatalf("Output file %s already exists. Remove it or use --output to specify different directory.", jsonPath)
	}

	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal JSON: %v", err)
	}

	if err := os.WriteFile(jsonPath, data, 0644); err != nil {
		log.Fatalf("Failed to write JSON: %v", err)
	}

	log.Printf("JSON written to %s", jsonPath)
}

func outputTable(stats *AggregatedStats) {
	fmt.Println("\n=== CODE STATISTICS ===\n")

	// By Author and Type
	fmt.Println("## Lines by Author and Type")
	table1 := tablewriter.NewWriter(os.Stdout)
	table1.SetHeader([]string{"Author", "Type", "Lines", "%"})
	table1.SetBorder(false)
	table1.SetAutoWrapText(false)

	for _, row := range stats.ByAuthorAndType {
		table1.Append([]string{row.Author, row.Type,
			strconv.Itoa(row.Lines), row.Percent})
	}
	table1.Render()

	// By Type
	fmt.Println("\n## Lines by Type")
	table2 := tablewriter.NewWriter(os.Stdout)
	table2.SetHeader([]string{"Type", "Lines", "%"})
	table2.SetBorder(false)
	table2.SetAutoWrapText(false)

	for _, row := range stats.ByType {
		table2.Append([]string{row.Type, strconv.Itoa(row.Lines), row.Percent})
	}
	table2.Render()

	// By Author
	fmt.Println("\n## Lines by Author")
	table3 := tablewriter.NewWriter(os.Stdout)
	table3.SetHeader([]string{"Author", "Lines", "%"})
	table3.SetBorder(false)
	table3.SetAutoWrapText(false)

	for _, row := range stats.ByAuthor {
		table3.Append([]string{row.Author, strconv.Itoa(row.Lines), row.Percent})
	}
	table3.Render()

	// Total
	fmt.Printf("\n## Total Lines: %d\n", stats.Total.Lines)
}
