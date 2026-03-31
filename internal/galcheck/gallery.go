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
// with their proposed entries, writes the result back atomically, and applies
// any config_file-targeted changes to individual model config YAMLs.
func ApplyReports(galleryPath string, reports []*PersistentReport) (int, error) {
	entries, err := LoadGallery(galleryPath)
	if err != nil {
		return 0, err
	}

	// Build lookups by name: updates and deletions
	updates := make(map[string]*GalleryEntry, len(reports))
	deletions := make(map[string]bool)
	for _, r := range reports {
		// Skip non-approved reports when review data is present
		if r.ReviewStatus != "" && r.ReviewStatus != "approved" {
			continue
		}

		if hasReviewData(r) {
			rebuilt := RebuildProposedEntry(r)
			if rebuilt != nil {
				updates[r.Name] = rebuilt
			} else {
				deletions[r.Name] = true
			}
		} else if r.ProposedEntry != nil {
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

	// Apply config_file-targeted changes (e.g. known_usecases in model YAMLs)
	galleryDir := filepath.Dir(galleryPath)
	cfgApplied, err := ApplyConfigFileChanges(galleryDir, reports)
	if err != nil {
		return 0, fmt.Errorf("apply config file changes: %w", err)
	}
	applied += cfgApplied

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

// ConfigFileSettings holds fields from the model config YAML that are relevant
// to gallery metadata.
type ConfigFileSettings struct {
	KnownUsecases []string `yaml:"known_usecases"`
}

// resolveConfigFilename extracts the local filename from a gallery entry's URL.
// Handles the github:owner/repo/path@ref scheme.
func resolveConfigFilename(entryURL string) string {
	// github:mudler/LocalAI/gallery/foo.yaml@master → foo.yaml
	after, ok := strings.CutPrefix(entryURL, "github:")
	if !ok {
		return ""
	}
	path := strings.SplitN(after, "@", 2)[0] // strip @ref
	return filepath.Base(path)
}

// LoadConfigFileSettings reads the model config YAML (the file pointed to by the
// entry's url field) from galleryDir and extracts gallery-relevant settings.
func LoadConfigFileSettings(galleryDir string, entry *GalleryEntry) (*ConfigFileSettings, error) {
	filename := resolveConfigFilename(entry.URL)
	if filename == "" {
		return nil, nil
	}

	data, err := os.ReadFile(filepath.Join(galleryDir, filename))
	if err != nil {
		return nil, fmt.Errorf("read config file %s: %w", filename, err)
	}

	// Outer YAML has config_file as a string containing inner YAML
	var outer struct {
		ConfigFile string `yaml:"config_file"`
	}
	if err := yaml.Unmarshal(data, &outer); err != nil {
		return nil, fmt.Errorf("parse outer YAML %s: %w", filename, err)
	}
	if outer.ConfigFile == "" {
		return nil, nil
	}

	var settings ConfigFileSettings
	if err := yaml.Unmarshal([]byte(outer.ConfigFile), &settings); err != nil {
		return nil, fmt.Errorf("parse config_file in %s: %w", filename, err)
	}

	return &settings, nil
}

// ApplyConfigFileChanges updates known_usecases in model config YAML files
// for any approved findings with Target=="config_file".
func ApplyConfigFileChanges(galleryDir string, reports []*PersistentReport) (int, error) {
	applied := 0

	for _, r := range reports {
		if r.ReviewStatus != "" && r.ReviewStatus != "approved" {
			continue
		}

		entry := r.OriginalEntry
		if entry == nil {
			continue
		}

		for _, f := range r.Findings {
			if f.Field != "known_usecases" || f.Target != TargetConfigFile {
				continue
			}
			if f.Accepted == nil || !*f.Accepted {
				continue
			}
			if err := updateConfigFileUsecases(galleryDir, entry, f.Proposed); err != nil {
				return applied, fmt.Errorf("update config %s: %w", r.Name, err)
			}
			applied++
		}
	}

	return applied, nil
}

// updateConfigFileUsecases rewrites known_usecases in a model's config_file
// string, preserving all other settings.
func updateConfigFileUsecases(galleryDir string, entry *GalleryEntry, proposed string) error {
	filename := resolveConfigFilename(entry.URL)
	if filename == "" {
		return fmt.Errorf("no config filename for url %q", entry.URL)
	}
	path := filepath.Join(galleryDir, filename)

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// Parse outer YAML to get config_file string
	var outer map[string]any
	if err := yaml.Unmarshal(data, &outer); err != nil {
		return fmt.Errorf("parse outer: %w", err)
	}

	cfgStr, _ := outer["config_file"].(string)
	if cfgStr == "" {
		return fmt.Errorf("no config_file in %s", filename)
	}

	// Parse the inner YAML
	var inner map[string]any
	if err := yaml.Unmarshal([]byte(cfgStr), &inner); err != nil {
		return fmt.Errorf("parse inner: %w", err)
	}

	// Parse proposed value like "[chat completion]"
	usecases := parseProposedUsecases(proposed)
	inner["known_usecases"] = usecases

	// Re-marshal inner back to string
	innerData, err := yaml.Marshal(inner)
	if err != nil {
		return fmt.Errorf("marshal inner: %w", err)
	}
	outer["config_file"] = string(innerData)

	// Re-marshal outer
	outData, err := yaml.Marshal(outer)
	if err != nil {
		return fmt.Errorf("marshal outer: %w", err)
	}

	return atomicWriteFile(path, outData)
}

// parseProposedUsecases parses the "[chat completion]" format back to a slice.
func parseProposedUsecases(s string) []string {
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if s == "" {
		return nil
	}
	return strings.Fields(s)
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
