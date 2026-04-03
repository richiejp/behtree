package galcheck

import (
	"slices"
	"testing"
)

func TestValidUsecasesForBackend_Known(t *testing.T) {
	tests := []struct {
		backend string
		want    []string
	}{
		{"llama-cpp", []string{"chat", "completion", "embeddings", "tokenize", "vision"}},
		{"piper", []string{"tts"}},
		{"diffusers", []string{"image", "video"}},
		{"rfdetr", []string{"detection"}},
		{"whisper", []string{"transcript", "vad"}},
		{"rerankers", []string{"rerank"}},
	}
	for _, tc := range tests {
		got := ValidUsecasesForBackend(tc.backend)
		slices.Sort(got)
		want := slices.Clone(tc.want)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("ValidUsecasesForBackend(%q) = %v, want %v", tc.backend, got, want)
		}
	}
}

func TestValidUsecasesForBackend_Unknown(t *testing.T) {
	got := ValidUsecasesForBackend("unknown-backend")
	// Should return all non-deprecated usecases
	wantCount := 0
	for _, desc := range UsecaseDescriptions {
		if !desc.Deprecated {
			wantCount++
		}
	}
	if len(got) != wantCount {
		t.Errorf("unknown backend: got %d usecases, want %d (all non-deprecated)", len(got), wantCount)
	}
	for _, u := range got {
		if UsecaseDescriptions[u].Deprecated {
			t.Errorf("unknown backend returned deprecated usecase %q", u)
		}
	}
}

func TestNormalizeBackend(t *testing.T) {
	// llama.cpp should be treated as llama-cpp
	got := ValidUsecasesForBackend("llama.cpp")
	if !slices.Contains(got, "chat") {
		t.Error("llama.cpp normalization failed: expected chat in valid usecases")
	}
	if len(got) == len(UsecaseDescriptions) {
		t.Error("llama.cpp returned all usecases, normalization not working")
	}
}

func TestDefaultUsecasesForBackend(t *testing.T) {
	tests := []struct {
		backend string
		want    []string
	}{
		{"llama-cpp", []string{"chat"}},
		{"piper", []string{"tts"}},
		{"diffusers", []string{"image"}},
		{"mlx-vlm", []string{"chat", "vision"}},
	}
	for _, tc := range tests {
		got := DefaultUsecasesForBackend(tc.backend)
		slices.Sort(got)
		want := slices.Clone(tc.want)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("DefaultUsecasesForBackend(%q) = %v, want %v", tc.backend, got, want)
		}
	}
}

func TestDefaultUsecasesForBackend_Unknown(t *testing.T) {
	got := DefaultUsecasesForBackend("no-such-backend")
	if got != nil {
		t.Errorf("unknown backend: got %v, want nil", got)
	}
}

func TestIsKnownBackend(t *testing.T) {
	if !IsKnownBackend("llama-cpp") {
		t.Error("llama-cpp should be known")
	}
	if !IsKnownBackend("llama.cpp") {
		t.Error("llama.cpp should normalize to known")
	}
	if IsKnownBackend("nonexistent") {
		t.Error("nonexistent should not be known")
	}
}

func TestAllKnownBackends(t *testing.T) {
	backends := AllKnownBackends()
	if len(backends) < 30 {
		t.Errorf("expected 30+ backends, got %d", len(backends))
	}
	if !slices.Contains(backends, "llama-cpp") {
		t.Error("llama-cpp missing from AllKnownBackends")
	}
	// Verify sorted
	if !slices.IsSorted(backends) {
		t.Error("AllKnownBackends should return sorted list")
	}
}

func TestBackendUsecases_DefaultsSubsetOfPossible(t *testing.T) {
	for name, info := range BackendUsecases {
		for _, d := range info.Default {
			if !slices.Contains(info.Possible, d) {
				t.Errorf("backend %q: default usecase %q not in possible %v", name, d, info.Possible)
			}
		}
	}
}

func TestBackendUsecases_AllUsecasesValid(t *testing.T) {
	for name, info := range BackendUsecases {
		for _, u := range info.Possible {
			if _, ok := UsecaseDescriptions[u]; !ok {
				t.Errorf("backend %q: usecase %q not in UsecaseDescriptions", name, u)
			}
		}
	}
}

func TestFilterValidUsecases_WithBackend(t *testing.T) {
	// piper only supports tts
	got := filterValidUsecases([]string{"chat", "tts", "image"}, "piper")
	if len(got) != 1 || got[0] != "tts" {
		t.Errorf("filterValidUsecases with piper: got %v, want [tts]", got)
	}
}

func TestFilterValidUsecases_UnknownBackend(t *testing.T) {
	// Unknown backend should allow all valid usecases
	got := filterValidUsecases([]string{"chat", "tts", "image"}, "")
	if len(got) != 3 {
		t.Errorf("filterValidUsecases with empty backend: got %v, want all 3", got)
	}
}

func TestExtractBackend_Overrides(t *testing.T) {
	entry := &GalleryEntry{
		Overrides: map[string]any{"backend": "llama-cpp"},
	}
	if got := ExtractBackend(entry); got != "llama-cpp" {
		t.Errorf("ExtractBackend overrides: got %q, want llama-cpp", got)
	}
}

func TestExtractBackend_ConfigFile(t *testing.T) {
	entry := &GalleryEntry{
		ConfigFile: map[string]any{"backend": "whisper"},
	}
	if got := ExtractBackend(entry); got != "whisper" {
		t.Errorf("ExtractBackend config_file: got %q, want whisper", got)
	}
}

func TestExtractBackend_OverridesPriority(t *testing.T) {
	entry := &GalleryEntry{
		Overrides:  map[string]any{"backend": "llama-cpp"},
		ConfigFile: map[string]any{"backend": "whisper"},
	}
	if got := ExtractBackend(entry); got != "llama-cpp" {
		t.Errorf("ExtractBackend priority: got %q, want llama-cpp (overrides wins)", got)
	}
}

func TestExtractBackend_Nil(t *testing.T) {
	if got := ExtractBackend(nil); got != "" {
		t.Errorf("ExtractBackend nil: got %q, want empty", got)
	}
}

func TestExtractBackend_Empty(t *testing.T) {
	entry := &GalleryEntry{}
	if got := ExtractBackend(entry); got != "" {
		t.Errorf("ExtractBackend empty: got %q, want empty", got)
	}
}
