package galcheck

import (
	"testing"
)

func TestConservativeAutoApprove_ValidUsecases(t *testing.T) {
	reports := []*PersistentReport{{
		Name: "test-model",
		OriginalEntry: &GalleryEntry{
			Overrides: map[string]any{"backend": "llama-cpp"},
		},
		Findings: []Finding{
			{Field: "known_usecases", Proposed: "[chat]", Source: "LLM"},
			{Field: "last_checked", Proposed: "2026-04-01", Source: "scan"},
		},
	}}

	result := ConservativeAutoApprove(reports)

	if result.Approved != 1 {
		t.Errorf("approved = %d, want 1", result.Approved)
	}
	if reports[0].ReviewStatus != "approved" {
		t.Errorf("status = %q, want approved", reports[0].ReviewStatus)
	}
	for i, f := range reports[0].Findings {
		if f.Accepted == nil || !*f.Accepted {
			t.Errorf("finding %d (%s): not accepted", i, f.Field)
		}
	}
}

func TestConservativeAutoApprove_InvalidUsecases(t *testing.T) {
	reports := []*PersistentReport{{
		Name: "tts-on-llm",
		OriginalEntry: &GalleryEntry{
			Overrides: map[string]any{"backend": "llama-cpp"},
		},
		Findings: []Finding{
			{Field: "known_usecases", Proposed: "[tts image]", Source: "LLM"},
			{Field: "last_checked", Proposed: "2026-04-01", Source: "scan"},
		},
	}}

	result := ConservativeAutoApprove(reports)

	if result.Approved != 1 {
		t.Errorf("approved = %d, want 1", result.Approved)
	}
	// Invalid usecases filtered, falls back to default
	f := reports[0].Findings[0]
	if f.Proposed != "[chat]" {
		t.Errorf("proposed = %q, want [chat] (default for llama-cpp)", f.Proposed)
	}
}

func TestConservativeAutoApprove_UnknownBackend(t *testing.T) {
	reports := []*PersistentReport{{
		Name: "unknown",
		OriginalEntry: &GalleryEntry{
			Overrides: map[string]any{"backend": "mystery-backend"},
		},
		Findings: []Finding{
			{Field: "known_usecases", Proposed: "[chat]", Source: "LLM"},
		},
	}}

	result := ConservativeAutoApprove(reports)

	if result.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", result.Skipped)
	}
	if reports[0].ReviewStatus != "" {
		t.Errorf("status = %q, want empty (untouched)", reports[0].ReviewStatus)
	}
}

func TestConservativeAutoApprove_AlreadyReviewed(t *testing.T) {
	reports := []*PersistentReport{{
		Name:         "already-done",
		ReviewStatus: "approved",
		OriginalEntry: &GalleryEntry{
			Overrides: map[string]any{"backend": "llama-cpp"},
		},
		Findings: []Finding{
			{Field: "known_usecases", Proposed: "[chat]", Source: "LLM"},
		},
	}}

	result := ConservativeAutoApprove(reports)

	if result.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", result.Skipped)
	}
}

func TestConservativeAutoApprove_DescriptionDoesNotBlock(t *testing.T) {
	reports := []*PersistentReport{{
		Name: "has-description",
		OriginalEntry: &GalleryEntry{
			Overrides: map[string]any{"backend": "llama-cpp"},
		},
		Findings: []Finding{
			{Field: "known_usecases", Proposed: "[chat]", Source: "LLM"},
			{Field: "Description", Proposed: "A cool model", Source: "LLM"},
			{Field: "last_checked", Proposed: "2026-04-01", Source: "scan"},
		},
	}}

	result := ConservativeAutoApprove(reports)

	if result.Approved != 1 {
		t.Errorf("approved = %d, want 1 (Description should not block)", result.Approved)
	}
	if reports[0].ReviewStatus != "approved" {
		t.Errorf("status = %q, want approved", reports[0].ReviewStatus)
	}
	// Mechanical findings should be accepted
	if reports[0].Findings[0].Accepted == nil || !*reports[0].Findings[0].Accepted {
		t.Error("known_usecases should be accepted")
	}
	if reports[0].Findings[2].Accepted == nil || !*reports[0].Findings[2].Accepted {
		t.Error("last_checked should be accepted")
	}
	// Description left for human review (not accepted)
	if reports[0].Findings[1].Accepted != nil {
		t.Error("Description should be nil (pending human review)")
	}
}

func TestConservativeAutoApprove_DeleteBlocks(t *testing.T) {
	reports := []*PersistentReport{{
		Name: "has-delete",
		OriginalEntry: &GalleryEntry{
			Overrides: map[string]any{"backend": "llama-cpp"},
		},
		Findings: []Finding{
			{Field: "known_usecases", Proposed: "[chat]", Source: "LLM"},
			{Field: "Delete", Proposed: "true", Source: "scan"},
			{Field: "last_checked", Proposed: "2026-04-01", Source: "scan"},
		},
	}}

	result := ConservativeAutoApprove(reports)

	if result.Modified != 1 {
		t.Errorf("modified = %d, want 1", result.Modified)
	}
	if result.Approved != 0 {
		t.Errorf("approved = %d, want 0 (Delete should block)", result.Approved)
	}
	if reports[0].ReviewStatus != "" {
		t.Errorf("status = %q, want empty (Delete blocks approval)", reports[0].ReviewStatus)
	}
}

func TestConservativeAutoApprove_MMProjVision(t *testing.T) {
	reports := []*PersistentReport{{
		Name: "vlm-model",
		OriginalEntry: &GalleryEntry{
			Overrides: map[string]any{
				"backend": "llama-cpp",
				"mmproj":  "llama-cpp/mmproj/model.gguf",
			},
		},
		Findings: []Finding{
			{Field: "known_usecases", Proposed: "[chat]", Source: "LLM"},
		},
	}}

	result := ConservativeAutoApprove(reports)

	if result.Approved != 1 {
		t.Errorf("approved = %d, want 1", result.Approved)
	}
	f := reports[0].Findings[0]
	if f.Proposed != "[chat vision]" {
		t.Errorf("proposed = %q, want [chat vision] (mmproj adds vision)", f.Proposed)
	}
}

func TestConservativeAutoApprove_PreserveUserDecisions(t *testing.T) {
	reports := []*PersistentReport{{
		Name: "partially-reviewed",
		OriginalEntry: &GalleryEntry{
			Overrides: map[string]any{"backend": "llama-cpp"},
		},
		Findings: []Finding{
			{Field: "known_usecases", Proposed: "[chat]", Source: "LLM", Accepted: boolPtr(true)},
			{Field: "Description", Proposed: "A model", Source: "LLM", Accepted: boolPtr(false)},
			{Field: "last_checked", Proposed: "2026-04-01", Source: "scan"},
		},
	}}

	result := ConservativeAutoApprove(reports)

	// User rejected Description, so not all accepted
	if result.Modified != 1 {
		t.Errorf("modified = %d, want 1", result.Modified)
	}
	// User's existing decisions preserved
	if !*reports[0].Findings[0].Accepted {
		t.Error("user's accept of usecases should be preserved")
	}
	if *reports[0].Findings[1].Accepted {
		t.Error("user's reject of Description should be preserved")
	}
	// last_checked auto-accepted
	if reports[0].Findings[2].Accepted == nil || !*reports[0].Findings[2].Accepted {
		t.Error("last_checked should be auto-accepted")
	}
}
