package galcheck

import (
	"fmt"
	"io"
	"strings"
)

type Finding struct {
	Field    string
	Current  string
	Proposed string
	Source   string // e.g., "HF metadata", "model card", "file check"
}

type FileCheckResult struct {
	Filename    string
	URI         string
	SHAMatch    bool
	Accessible  bool
	ExpectedSHA string
	ActualSHA   string
	StatusCode  int
	Error       string
}

type ModelReport struct {
	Name          string
	EntryIndex    int
	HFRepo        string
	Findings      []Finding
	FileResults   []FileCheckResult
	SafetyOK      bool
	SafetyNote    string
	ProposedEntry *GalleryEntry // the updated entry if changes are needed
}

func (r *ModelReport) HasChanges() bool {
	return len(r.Findings) > 0
}

// WriteReport writes a markdown report for a single model.
func WriteReport(w io.Writer, report *ModelReport) {
	fmt.Fprintf(w, "## %s (entry #%d)\n\n", report.Name, report.EntryIndex)

	if report.HFRepo != "" {
		fmt.Fprintf(w, "HuggingFace repo: %s\n\n", report.HFRepo)
	}

	// Findings
	if len(report.Findings) > 0 {
		fmt.Fprintln(w, "### Findings")
		for _, f := range report.Findings {
			current := f.Current
			if current == "" {
				current = "MISSING"
			}
			fmt.Fprintf(w, "- **%s**: %s → %s", f.Field, current, f.Proposed)
			if f.Source != "" {
				fmt.Fprintf(w, " (from %s)", f.Source)
			}
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w)
	} else {
		fmt.Fprintln(w, "### Findings")
		fmt.Fprintln(w, "No metadata changes needed.")
		fmt.Fprintln(w)
	}

	// File check results
	if len(report.FileResults) > 0 {
		fmt.Fprintln(w, "### File Checks")
		shaOK := 0
		accessOK := 0
		for _, fr := range report.FileResults {
			if fr.SHAMatch {
				shaOK++
			}
			if fr.Accessible {
				accessOK++
			}
		}
		total := len(report.FileResults)
		fmt.Fprintf(w, "- SHA256: %d/%d match\n", shaOK, total)
		fmt.Fprintf(w, "- Accessible: %d/%d reachable\n", accessOK, total)

		if !report.SafetyOK {
			fmt.Fprintf(w, "- **Safety**: %s\n", report.SafetyNote)
		} else {
			fmt.Fprintln(w, "- Safety: OK")
		}

		// Detail any failures
		for _, fr := range report.FileResults {
			if !fr.SHAMatch || !fr.Accessible {
				fmt.Fprintf(w, "\n  **%s**:", fr.Filename)
				if !fr.SHAMatch {
					fmt.Fprintf(w, " SHA mismatch (expected %s, got %s)", truncSHA(fr.ExpectedSHA), truncSHA(fr.ActualSHA))
				}
				if !fr.Accessible {
					fmt.Fprintf(w, " inaccessible (HTTP %d)", fr.StatusCode)
				}
				if fr.Error != "" {
					fmt.Fprintf(w, " error: %s", fr.Error)
				}
				fmt.Fprintln(w)
			}
		}
		fmt.Fprintln(w)
	}

	// Safety
	fmt.Fprintln(w, "---")
	fmt.Fprintln(w)
}

// WriteSummary writes a summary header for the full report.
func WriteSummary(w io.Writer, reports []*ModelReport) {
	fmt.Fprintln(w, "# Gallery Check Report")
	fmt.Fprintln(w)

	changed := 0
	fileIssues := 0
	safetyIssues := 0
	for _, r := range reports {
		if r.HasChanges() {
			changed++
		}
		for _, fr := range r.FileResults {
			if !fr.SHAMatch || !fr.Accessible {
				fileIssues++
			}
		}
		if !r.SafetyOK {
			safetyIssues++
		}
	}

	fmt.Fprintf(w, "**Models checked**: %d\n", len(reports))
	fmt.Fprintf(w, "**With metadata changes**: %d\n", changed)
	fmt.Fprintf(w, "**File issues**: %d\n", fileIssues)
	fmt.Fprintf(w, "**Safety issues**: %d\n", safetyIssues)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "---")
	fmt.Fprintln(w)
}

func truncSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12] + "..."
	}
	return sha
}

// ExtractLicense tries to extract a SPDX license ID from HF model tags.
func ExtractLicense(tags []string) string {
	for _, tag := range tags {
		if after, ok := strings.CutPrefix(tag, "license:"); ok {
			return after
		}
	}
	return ""
}
