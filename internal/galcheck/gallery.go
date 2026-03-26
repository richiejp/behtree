package galcheck

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

type GalleryEntry struct {
	Name        string         `yaml:"name" json:"name"`
	URL         string         `yaml:"url,omitempty" json:"url,omitempty"`
	URLs        []string       `yaml:"urls,omitempty" json:"urls,omitempty"`
	Description string         `yaml:"description,omitempty" json:"description,omitempty"`
	License     string         `yaml:"license,omitempty" json:"license,omitempty"`
	Icon        string         `yaml:"icon,omitempty" json:"icon,omitempty"`
	Tags        []string       `yaml:"tags,omitempty" json:"tags,omitempty"`
	Size        string         `yaml:"size,omitempty" json:"size,omitempty"`
	LastChecked string         `yaml:"last_checked,omitempty" json:"last_checked,omitempty"`
	Overrides   map[string]any `yaml:"overrides,omitempty" json:"overrides,omitempty"`
	Files       []GalleryFile  `yaml:"files,omitempty" json:"files,omitempty"`
	ConfigFile  map[string]any `yaml:"config_file,omitempty" json:"config_file,omitempty"`
	Extra       map[string]any `yaml:",inline" json:"extra,omitempty"`
}

type GalleryFile struct {
	Filename string `yaml:"filename" json:"filename"`
	SHA256   string `yaml:"sha256" json:"sha256"`
	URI      string `yaml:"uri" json:"uri"`
}

func LoadGallery(path string) ([]GalleryEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read gallery: %w", err)
	}

	var entries []GalleryEntry
	if err := yaml.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse gallery: %w", err)
	}

	return entries, nil
}

func NeedsCheck(entry *GalleryEntry, maxAge time.Duration) bool {
	if entry.LastChecked == "" {
		return true
	}

	checked, err := time.Parse("2006-01-02", entry.LastChecked)
	if err != nil {
		return true
	}

	return time.Since(checked) > maxAge
}

// FileRepoMapping records which HF repo a gallery file comes from.
type FileRepoMapping struct {
	Filename string `json:"filename"`
	Repo     string `json:"repo"` // "owner/repo" or "" if non-HF
}

// extractRepoFromURI extracts an "owner/repo" HF repo ID from a file URI.
// Handles https://huggingface.co/..., huggingface://..., and hf://... schemes.
func extractRepoFromURI(uri string) string {
	if repo := parseHFURL(uri); repo != "" {
		return repo
	}
	for _, prefix := range []string{"huggingface://", "hf://"} {
		if after, ok := strings.CutPrefix(uri, prefix); ok {
			parts := strings.SplitN(after, "/", 3)
			if len(parts) >= 2 {
				return parts[0] + "/" + parts[1]
			}
		}
	}
	return ""
}

// ExtractHFRepos returns all unique HF repos referenced by an entry and
// a per-file mapping. Repos from entry.URLs come first, then repos from
// file URIs, deduplicated and in order of first appearance.
func ExtractHFRepos(entry *GalleryEntry) ([]string, []FileRepoMapping) {
	seen := make(map[string]bool)
	var repos []string

	addRepo := func(repo string) {
		if repo != "" && !seen[repo] {
			seen[repo] = true
			repos = append(repos, repo)
		}
	}

	// URLs first (often the canonical project page on HF)
	for _, u := range entry.URLs {
		addRepo(parseHFURL(u))
	}

	// File URIs
	var mappings []FileRepoMapping
	for _, f := range entry.Files {
		repo := extractRepoFromURI(f.URI)
		addRepo(repo)
		mappings = append(mappings, FileRepoMapping{
			Filename: f.Filename,
			Repo:     repo,
		})
	}

	return repos, mappings
}

// ExtractHFRepo returns the first HF repo found in the entry.
// Kept for backward compatibility; prefer ExtractHFRepos.
func ExtractHFRepo(entry *GalleryEntry) string {
	repos, _ := ExtractHFRepos(entry)
	if len(repos) > 0 {
		return repos[0]
	}
	return ""
}

// ApplyReports loads the gallery YAML, replaces entries matching report names
// with their proposed entries, and writes the result back atomically.
func ApplyReports(galleryPath string, reports []*PersistentReport) (int, error) {
	entries, err := LoadGallery(galleryPath)
	if err != nil {
		return 0, err
	}

	// Build lookups by name: updates and deletions
	updates := make(map[string]*GalleryEntry, len(reports))
	deletions := make(map[string]bool)
	for _, r := range reports {
		if r.ProposedEntry != nil {
			updates[r.Name] = r.ProposedEntry
		} else {
			deletions[r.Name] = true
		}
	}

	applied := 0
	var kept []GalleryEntry
	for _, entry := range entries {
		if deletions[entry.Name] {
			applied++
			continue // remove from gallery
		}
		if p, ok := updates[entry.Name]; ok {
			kept = append(kept, *p)
			applied++
		} else {
			kept = append(kept, entry)
		}
	}
	entries = kept

	if applied == 0 {
		return 0, nil
	}

	data, err := yaml.Marshal(entries)
	if err != nil {
		return 0, fmt.Errorf("marshal gallery: %w", err)
	}

	if err := atomicWriteFile(galleryPath, data); err != nil {
		return 0, fmt.Errorf("write gallery: %w", err)
	}

	return applied, nil
}

func atomicWriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return os.Rename(tmpPath, path)
}

func parseHFURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}

	if u.Host != "huggingface.co" {
		return ""
	}

	// Path like /owner/repo or /owner/repo/resolve/main/...
	parts := strings.SplitN(strings.TrimPrefix(u.Path, "/"), "/", 3)
	if len(parts) >= 2 {
		return parts[0] + "/" + parts[1]
	}

	return ""
}
