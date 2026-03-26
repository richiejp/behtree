package galcheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Finding struct {
	Field    string `json:"field"`
	Current  string `json:"current"`
	Proposed string `json:"proposed"`
	Source   string `json:"source"` // e.g., "HF metadata", "model card", "file check"
}

type FileCheckResult struct {
	Filename    string `json:"filename"`
	URI         string `json:"uri"`
	SourceRepo  string `json:"source_repo,omitempty"`
	SHAMatch    bool   `json:"sha_match"`
	Accessible  bool   `json:"accessible"`
	ExpectedSHA string `json:"expected_sha"`
	ActualSHA   string `json:"actual_sha"`
	StatusCode  int    `json:"status_code"`
	Error       string `json:"error,omitempty"`
}

type ModelReport struct {
	Name          string            `json:"name"`
	EntryIndex    int               `json:"entry_index"`
	HFRepo        string            `json:"hf_repo"`
	HFRepos       []string          `json:"hf_repos,omitempty"`
	Findings      []Finding         `json:"findings"`
	FileResults   []FileCheckResult `json:"file_results"`
	SafetyOK      bool              `json:"safety_ok"`
	SafetyNote    string            `json:"safety_note,omitempty"`
	ProposedEntry *GalleryEntry     `json:"proposed_entry,omitempty"`
}

func (r *ModelReport) HasChanges() bool {
	return len(r.Findings) > 0
}

// WriteReport writes a markdown report for a single model.
func WriteReport(w io.Writer, report *ModelReport) {
	fmt.Fprintf(w, "## %s (entry #%d)\n\n", report.Name, report.EntryIndex)

	if len(report.HFRepos) > 1 {
		fmt.Fprintf(w, "HuggingFace repos: %s (primary)", report.HFRepo)
		for _, r := range report.HFRepos {
			if r != report.HFRepo {
				fmt.Fprintf(w, ", %s", r)
			}
		}
		fmt.Fprintln(w)
		fmt.Fprintln(w)
	} else if report.HFRepo != "" {
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

// PersistentReport is the on-disk format for a per-model report.
type PersistentReport struct {
	Name          string            `json:"name"`
	EntryIndex    int               `json:"entry_index"`
	HFRepo        string            `json:"hf_repo"`
	HFRepos       []string          `json:"hf_repos,omitempty"`
	Findings      []Finding         `json:"findings"`
	FileResults   []FileCheckResult `json:"file_results"`
	SafetyOK      bool              `json:"safety_ok"`
	SafetyNote    string            `json:"safety_note,omitempty"`
	OriginalEntry *GalleryEntry     `json:"original_entry"`
	ProposedEntry *GalleryEntry     `json:"proposed_entry"`
	CheckedAt     string            `json:"checked_at"`
}

// SanitizeFilename makes a model name safe for use as a filename.
func SanitizeFilename(name string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", " ", "_", ":", "_")
	return r.Replace(name)
}

// WriteReportFiles writes a .json and .md report file atomically to dir.
func WriteReportFiles(dir string, report *PersistentReport) error {
	base := SanitizeFilename(report.Name)

	// Write JSON
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	if err := atomicWrite(filepath.Join(dir, base+".json"), data); err != nil {
		return fmt.Errorf("write json: %w", err)
	}

	// Write markdown
	var buf bytes.Buffer
	mr := &ModelReport{
		Name:          report.Name,
		EntryIndex:    report.EntryIndex,
		HFRepo:        report.HFRepo,
		HFRepos:       report.HFRepos,
		Findings:      report.Findings,
		FileResults:   report.FileResults,
		SafetyOK:      report.SafetyOK,
		SafetyNote:    report.SafetyNote,
		ProposedEntry: report.ProposedEntry,
	}
	WriteReport(&buf, mr)
	if err := atomicWrite(filepath.Join(dir, base+".md"), buf.Bytes()); err != nil {
		return fmt.Errorf("write md: %w", err)
	}

	return nil
}

func atomicWrite(path string, data []byte) error {
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

// LoadReports reads all .json report files from a directory.
func LoadReports(dir string) ([]*PersistentReport, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
	}

	var reports []*PersistentReport
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		var report PersistentReport
		if err := json.Unmarshal(data, &report); err != nil {
			return nil, fmt.Errorf("parse %s: %w", e.Name(), err)
		}
		reports = append(reports, &report)
	}

	return reports, nil
}
