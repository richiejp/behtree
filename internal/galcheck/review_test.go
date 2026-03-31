package galcheck

import (
	"testing"
)

func boolPtr(b bool) *bool { return &b }

func TestRebuildProposedEntry_AllAccepted(t *testing.T) {
	report := &PersistentReport{
		OriginalEntry: &GalleryEntry{
			Name:    "test-model",
			License: "",
			Tags:    []string{"old"},
		},
		ProposedEntry: &GalleryEntry{
			Name:        "test-model",
			License:     "apache-2.0",
			Tags:        []string{"new", "tags"},
			LastChecked: "2026-03-26",
		},
		Findings: []Finding{
			{Field: "License", Proposed: "apache-2.0", Accepted: boolPtr(true)},
			{Field: "Tags", Proposed: "[new tags]", Accepted: boolPtr(true)},
			{Field: "last_checked", Proposed: "2026-03-26", Accepted: boolPtr(true)},
		},
	}

	result := RebuildProposedEntry(report)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.License != "apache-2.0" {
		t.Errorf("License = %q, want apache-2.0", result.License)
	}
	if len(result.Tags) != 2 || result.Tags[0] != "new" {
		t.Errorf("Tags = %v, want [new tags]", result.Tags)
	}
	if result.LastChecked != "2026-03-26" {
		t.Errorf("LastChecked = %q, want 2026-03-26", result.LastChecked)
	}
}

func TestRebuildProposedEntry_PartialAccept(t *testing.T) {
	report := &PersistentReport{
		OriginalEntry: &GalleryEntry{
			Name:    "test-model",
			License: "mit",
			Tags:    []string{"old"},
		},
		ProposedEntry: &GalleryEntry{
			Name:        "test-model",
			License:     "apache-2.0",
			Tags:        []string{"new", "tags"},
			LastChecked: "2026-03-26",
		},
		Findings: []Finding{
			{Field: "License", Proposed: "apache-2.0", Accepted: boolPtr(false)},
			{Field: "Tags", Proposed: "[new tags]", Accepted: boolPtr(true)},
			{Field: "last_checked", Proposed: "2026-03-26", Accepted: boolPtr(true)},
		},
	}

	result := RebuildProposedEntry(report)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.License != "mit" {
		t.Errorf("License = %q, want mit (rejected)", result.License)
	}
	if len(result.Tags) != 2 || result.Tags[0] != "new" {
		t.Errorf("Tags = %v, want [new tags] (accepted)", result.Tags)
	}
}

func TestRebuildProposedEntry_PendingSkipped(t *testing.T) {
	report := &PersistentReport{
		OriginalEntry: &GalleryEntry{
			Name:    "test-model",
			License: "mit",
		},
		ProposedEntry: &GalleryEntry{
			Name:    "test-model",
			License: "apache-2.0",
		},
		Findings: []Finding{
			{Field: "License", Proposed: "apache-2.0", Accepted: nil},
		},
	}

	result := RebuildProposedEntry(report)
	if result.License != "mit" {
		t.Errorf("License = %q, want mit (pending should not apply)", result.License)
	}
}

func TestRebuildProposedEntry_Delete(t *testing.T) {
	report := &PersistentReport{
		OriginalEntry: &GalleryEntry{Name: "test-model"},
		ProposedEntry: nil,
		Findings: []Finding{
			{Field: "Delete", Accepted: boolPtr(true)},
		},
	}

	// ProposedEntry is nil for deletion — RebuildProposedEntry returns nil
	result := RebuildProposedEntry(report)
	if result != nil {
		t.Errorf("expected nil for accepted deletion, got %+v", result)
	}
}

func TestRebuildProposedEntry_SHA(t *testing.T) {
	for _, tc := range []struct {
		name     string
		accepted bool
		wantSHA  string
	}{
		{"accepted", true, "newsha"},
		{"rejected", false, "oldsha"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report := &PersistentReport{
				OriginalEntry: &GalleryEntry{
					Name:  "test-model",
					Files: []GalleryFile{{Filename: "model.gguf", SHA256: "oldsha", URI: "hf://o/r/model.gguf"}},
				},
				ProposedEntry: &GalleryEntry{
					Name:  "test-model",
					Files: []GalleryFile{{Filename: "model.gguf", SHA256: "newsha", URI: "hf://o/r/model.gguf"}},
				},
				Findings: []Finding{
					{Field: "SHA256:model.gguf", Current: "oldsha", Proposed: "newsha", Accepted: boolPtr(tc.accepted)},
				},
			}
			result := RebuildProposedEntry(report)
			if result == nil {
				t.Fatal("expected non-nil result")
			}
			if result.Files[0].SHA256 != tc.wantSHA {
				t.Errorf("Files[0].SHA256 = %q, want %q", result.Files[0].SHA256, tc.wantSHA)
			}
		})
	}
}

func TestRebuildProposedEntry_UsecasesConfigTarget(t *testing.T) {
	// known_usecases with target=config_file should NOT be applied to overrides
	report := &PersistentReport{
		OriginalEntry: &GalleryEntry{
			Name: "test-model",
		},
		ProposedEntry: &GalleryEntry{
			Name: "test-model",
		},
		Findings: []Finding{
			{Field: "known_usecases", Proposed: "[chat completion]", Target: TargetConfigFile, Accepted: boolPtr(true)},
		},
	}

	result := RebuildProposedEntry(report)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Overrides != nil {
		if _, ok := result.Overrides["known_usecases"]; ok {
			t.Error("config_file-targeted known_usecases should not appear in overrides")
		}
	}
}

func TestHasReviewData(t *testing.T) {
	// No review data
	r := &PersistentReport{Findings: []Finding{{Field: "License"}}}
	if hasReviewData(r) {
		t.Error("expected false for unreviewed report")
	}

	// Has finding review
	r.Findings[0].Accepted = boolPtr(true)
	if !hasReviewData(r) {
		t.Error("expected true when finding has Accepted set")
	}

	// Has status only
	r2 := &PersistentReport{ReviewStatus: "approved"}
	if !hasReviewData(r2) {
		t.Error("expected true when ReviewStatus is set")
	}
}
