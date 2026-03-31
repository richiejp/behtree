package galcheck

import (
	"os"
	"path/filepath"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestExtractHFRepos_MultiRepo(t *testing.T) {
	// Z-Image-Turbo style entry: 3 files from 3 different repos
	entry := GalleryEntry{
		Name: "Z-Image-Turbo",
		URLs: []string{"https://github.com/Tongyi-MAI/Z-Image"}, // not HF
		Files: []GalleryFile{
			{
				Filename: "Qwen3-4B.Q4_K_M.gguf",
				SHA256:   "a379319376",
				URI:      "huggingface://MaziyarPanahi/Qwen3-4B-GGUF/Qwen3-4B.Q4_K_M.gguf",
			},
			{
				Filename: "z_image_turbo-Q4_0.gguf",
				SHA256:   "14b375ab4f",
				URI:      "https://huggingface.co/leejet/Z-Image-Turbo-GGUF/resolve/main/z_image_turbo-Q4_K.gguf",
			},
			{
				Filename: "ae.safetensors",
				SHA256:   "afc8e28272",
				URI:      "https://huggingface.co/ChuckMcSneed/FLUX.1-dev/resolve/main/ae.safetensors",
			},
		},
	}

	repos, mappings := ExtractHFRepos(&entry)

	// Should find all 3 repos
	if len(repos) != 3 {
		t.Fatalf("expected 3 repos, got %d: %v", len(repos), repos)
	}

	expected := []string{
		"MaziyarPanahi/Qwen3-4B-GGUF",
		"leejet/Z-Image-Turbo-GGUF",
		"ChuckMcSneed/FLUX.1-dev",
	}
	for i, want := range expected {
		if repos[i] != want {
			t.Errorf("repos[%d] = %q, want %q", i, repos[i], want)
		}
	}

	// Should have 3 file mappings
	if len(mappings) != 3 {
		t.Fatalf("expected 3 mappings, got %d", len(mappings))
	}
	for i, m := range mappings {
		if m.Filename != entry.Files[i].Filename {
			t.Errorf("mapping[%d].Filename = %q, want %q", i, m.Filename, entry.Files[i].Filename)
		}
		if m.Repo != expected[i] {
			t.Errorf("mapping[%d].Repo = %q, want %q", i, m.Repo, expected[i])
		}
	}
}

func TestExtractHFRepos_SingleRepo(t *testing.T) {
	entry := GalleryEntry{
		Name: "some-model",
		Files: []GalleryFile{
			{
				Filename: "model.gguf",
				URI:      "huggingface://TheBloke/some-model-GGUF/model.Q4_K_M.gguf",
			},
			{
				Filename: "model2.gguf",
				URI:      "huggingface://TheBloke/some-model-GGUF/model.Q5_K_M.gguf",
			},
		},
	}

	repos, mappings := ExtractHFRepos(&entry)

	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d: %v", len(repos), repos)
	}
	if repos[0] != "TheBloke/some-model-GGUF" {
		t.Errorf("repo = %q, want TheBloke/some-model-GGUF", repos[0])
	}
	if len(mappings) != 2 {
		t.Fatalf("expected 2 mappings, got %d", len(mappings))
	}
}

func TestExtractHFRepos_URLsFirst(t *testing.T) {
	entry := GalleryEntry{
		Name: "my-model",
		URLs: []string{"https://huggingface.co/org/primary-model"},
		Files: []GalleryFile{
			{
				Filename: "model.gguf",
				URI:      "huggingface://org/gguf-variant/model.gguf",
			},
		},
	}

	repos, _ := ExtractHFRepos(&entry)

	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d: %v", len(repos), repos)
	}
	// URLs repo should come first
	if repos[0] != "org/primary-model" {
		t.Errorf("repos[0] = %q, want org/primary-model", repos[0])
	}
	if repos[1] != "org/gguf-variant" {
		t.Errorf("repos[1] = %q, want org/gguf-variant", repos[1])
	}
}

func TestExtractHFRepos_NoHFURLs(t *testing.T) {
	entry := GalleryEntry{
		Name: "non-hf-model",
		URLs: []string{"https://github.com/example/model"},
		Files: []GalleryFile{
			{
				Filename: "model.bin",
				URI:      "https://example.com/model.bin",
			},
		},
	}

	repos, mappings := ExtractHFRepos(&entry)

	if len(repos) != 0 {
		t.Fatalf("expected 0 repos, got %d: %v", len(repos), repos)
	}
	if len(mappings) != 1 {
		t.Fatalf("expected 1 mapping, got %d", len(mappings))
	}
	if mappings[0].Repo != "" {
		t.Errorf("expected empty repo for non-HF file, got %q", mappings[0].Repo)
	}
}

func TestExtractHFRepo_BackwardsCompat(t *testing.T) {
	entry := GalleryEntry{
		Files: []GalleryFile{
			{
				Filename: "model.gguf",
				URI:      "huggingface://TheBloke/model-GGUF/model.gguf",
			},
		},
	}

	repo := ExtractHFRepo(&entry)
	if repo != "TheBloke/model-GGUF" {
		t.Errorf("ExtractHFRepo = %q, want TheBloke/model-GGUF", repo)
	}
}

func TestParseProposedUsecases(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  []string
	}{
		{"[chat completion]", []string{"chat", "completion"}},
		{"[chat]", []string{"chat"}},
		{"[]", nil},
		{"", nil},
	} {
		got := parseProposedUsecases(tc.input)
		if len(got) != len(tc.want) {
			t.Errorf("parseProposedUsecases(%q) = %v, want %v", tc.input, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("parseProposedUsecases(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
			}
		}
	}
}

func TestUpdateConfigFileUsecases(t *testing.T) {
	dir := t.TempDir()

	// Write a config file similar to nanbeige4.1.yaml
	configContent := `---
name: test-model

config_file: |
  backend: llama-cpp
  known_usecases:
      - chat
  parameters:
      model: llama-cpp/models/test.gguf
`
	if err := os.WriteFile(filepath.Join(dir, "test-model.yaml"), []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	entry := &GalleryEntry{
		Name: "test-model",
		URL:  "github:owner/repo/gallery/test-model.yaml@master",
	}

	err := updateConfigFileUsecases(dir, entry, "[chat completion]")
	if err != nil {
		t.Fatal(err)
	}

	// Re-read and verify
	data, err := os.ReadFile(filepath.Join(dir, "test-model.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	var outer struct {
		ConfigFile string `yaml:"config_file"`
	}
	if err := yaml.Unmarshal(data, &outer); err != nil {
		t.Fatal(err)
	}

	var inner struct {
		KnownUsecases []string `yaml:"known_usecases"`
		Backend       string   `yaml:"backend"`
	}
	if err := yaml.Unmarshal([]byte(outer.ConfigFile), &inner); err != nil {
		t.Fatal(err)
	}

	if len(inner.KnownUsecases) != 2 || inner.KnownUsecases[0] != "chat" || inner.KnownUsecases[1] != "completion" {
		t.Errorf("known_usecases = %v, want [chat completion]", inner.KnownUsecases)
	}
	// Verify other fields preserved
	if inner.Backend != "llama-cpp" {
		t.Errorf("backend = %q, want llama-cpp (other fields should be preserved)", inner.Backend)
	}
}

func TestExtractRepoFromURI(t *testing.T) {
	tests := []struct {
		uri  string
		want string
	}{
		{"huggingface://owner/repo/file.gguf", "owner/repo"},
		{"hf://owner/repo/file.gguf", "owner/repo"},
		{"https://huggingface.co/owner/repo/resolve/main/file.gguf", "owner/repo"},
		{"https://github.com/owner/repo", ""},
		{"https://example.com/file.bin", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := extractRepoFromURI(tt.uri)
		if got != tt.want {
			t.Errorf("extractRepoFromURI(%q) = %q, want %q", tt.uri, got, tt.want)
		}
	}
}
