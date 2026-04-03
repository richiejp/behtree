package galcheck

import (
	"fmt"
	"strings"
	"time"
)

// AutoApproveResult summarizes what ConservativeAutoApprove did.
type AutoApproveResult struct {
	Approved uint // reports fully auto-approved
	Modified uint // reports with some findings accepted but not fully approved
	Skipped  uint // reports left untouched (unknown backend, already reviewed, etc.)
}

// ConservativeAutoApprove processes pending reports with mechanical, safe changes:
//   - Validates known_usecases against the model's backend
//   - Accepts factual/mechanical findings (last_checked, License, Icon, SHA256)
//   - Leaves subjective findings pending (Description, Tags, Delete)
//   - For llama-cpp models with mmproj, ensures "vision" is included
//
// Reports with unknown backends or only subjective findings are skipped.
func ConservativeAutoApprove(reports []*PersistentReport) AutoApproveResult {
	var result AutoApproveResult

	for _, r := range reports {
		if r.ReviewStatus != "" {
			result.Skipped++
			continue
		}

		backend := ExtractBackend(r.OriginalEntry)
		if !IsKnownBackend(backend) {
			result.Skipped++
			continue
		}

		hasVisionHardware := hasMMProj(r.OriginalEntry)
		allAccepted := true
		anyModified := false

		for i := range r.Findings {
			f := &r.Findings[i]
			if f.Accepted != nil {
				// Already reviewed by user, don't override
				if !*f.Accepted {
					allAccepted = false
				}
				continue
			}

			switch {
			case f.Field == "known_usecases":
				proposed := autoApproveUsecases(f, backend, hasVisionHardware)
				f.Proposed = proposed
				accepted := true
				f.Accepted = &accepted
				anyModified = true

			case f.Field == "last_checked",
				f.Field == "License",
				f.Field == "Icon",
				strings.HasPrefix(f.Field, "SHA256:"):
				accepted := true
				f.Accepted = &accepted
				anyModified = true

			case f.Field == "Delete":
				// Destructive action — requires human review
				allAccepted = false

			case f.Field == "Description",
				f.Field == "Tags":
				// Cosmetic improvements — leave for human review but
				// don't block approval of the mechanical changes
			}
		}

		if !anyModified {
			result.Skipped++
			continue
		}

		if allAccepted {
			r.ReviewStatus = "approved"
			r.ReviewedAt = time.Now().Format("2006-01-02")
			result.Approved++
		} else {
			result.Modified++
		}
	}

	return result
}

// autoApproveUsecases validates and filters proposed usecases for a backend,
// falling back to defaults when nothing valid remains.
func autoApproveUsecases(f *Finding, backend string, hasVisionHardware bool) string {
	proposed := parseProposedUsecases(f.Proposed)
	valid := filterValidUsecases(proposed, backend)

	if len(valid) == 0 {
		valid = DefaultUsecasesForBackend(backend)
	}

	// For backends that support vision with hardware present, ensure it's included
	if hasVisionHardware {
		possible := ValidUsecasesForBackend(backend)
		hasVision := false
		visionPossible := false
		for _, u := range valid {
			if u == "vision" {
				hasVision = true
			}
		}
		for _, u := range possible {
			if u == "vision" {
				visionPossible = true
			}
		}
		if visionPossible && !hasVision {
			valid = append(valid, "vision")
		}
	}

	return fmt.Sprintf("%v", valid)
}

// hasMMProj checks if a gallery entry has an mmproj file configured,
// indicating vision/multimodal hardware support.
func hasMMProj(entry *GalleryEntry) bool {
	if entry == nil || entry.Overrides == nil {
		return false
	}
	mmproj, _ := entry.Overrides["mmproj"].(string)
	return mmproj != ""
}
