package galcheck

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

type GalleryEntry struct {
	Name        string         `yaml:"name"`
	URL         string         `yaml:"url,omitempty"`
	URLs        []string       `yaml:"urls,omitempty"`
	Description string         `yaml:"description,omitempty"`
	License     string         `yaml:"license,omitempty"`
	Icon        string         `yaml:"icon,omitempty"`
	Tags        []string       `yaml:"tags,omitempty"`
	Size        string         `yaml:"size,omitempty"`
	LastChecked string         `yaml:"last_checked,omitempty"`
	Overrides   map[string]any `yaml:"overrides,omitempty"`
	Files       []GalleryFile  `yaml:"files,omitempty"`
	ConfigFile  map[string]any `yaml:"config_file,omitempty"`
	Extra       map[string]any `yaml:",inline"`
}

type GalleryFile struct {
	Filename string `yaml:"filename"`
	SHA256   string `yaml:"sha256"`
	URI      string `yaml:"uri"`
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

func ExtractHFRepo(entry *GalleryEntry) string {
	for _, u := range entry.URLs {
		if repo := parseHFURL(u); repo != "" {
			return repo
		}
	}

	// Try file URIs as fallback
	for _, f := range entry.Files {
		if repo := parseHFURL(f.URI); repo != "" {
			return repo
		}
		if strings.HasPrefix(f.URI, "huggingface://") {
			parts := strings.SplitN(strings.TrimPrefix(f.URI, "huggingface://"), "/", 3)
			if len(parts) >= 2 {
				return parts[0] + "/" + parts[1]
			}
		}
	}

	return ""
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
