package main

import (
	"log"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// AuthorConfig for merging author identities
type AuthorConfig struct {
	Authors []AuthorMapping `yaml:"authors"`
}

type AuthorMapping struct {
	Canonical string   `yaml:"canonical"`
	Emails    []string `yaml:"emails"`
	Names     []string `yaml:"names"`
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
