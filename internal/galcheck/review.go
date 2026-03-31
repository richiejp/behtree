package galcheck

import (
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

//go:embed templates/review.html
var reviewFS embed.FS

// RebuildProposedEntry constructs a GalleryEntry from OriginalEntry with only
// accepted findings applied (copying field values from ProposedEntry).
// Returns nil if a "Delete" finding is accepted.
func RebuildProposedEntry(report *PersistentReport) *GalleryEntry {
	if report.OriginalEntry == nil {
		return nil
	}
	if report.ProposedEntry == nil {
		return nil
	}

	entry := CloneEntry(report.OriginalEntry)
	proposed := report.ProposedEntry

	for _, f := range report.Findings {
		if f.Accepted == nil || !*f.Accepted {
			continue
		}
		switch f.Field {
		case "Delete":
			return nil
		case "License":
			entry.License = proposed.License
		case "Description":
			entry.Description = proposed.Description
			if entry.Overrides != nil {
				delete(entry.Overrides, "description")
			}
		case "Tags":
			entry.Tags = proposed.Tags
		case "known_usecases":
			// config_file-targeted findings are applied by ApplyConfigFileChanges
			if f.Target == TargetConfigFile {
				continue
			}
			if proposed.Overrides != nil {
				if entry.Overrides == nil {
					entry.Overrides = make(map[string]any)
				}
				if uc, ok := proposed.Overrides["known_usecases"]; ok {
					entry.Overrides["known_usecases"] = uc
				}
			}
		case "Icon":
			entry.Icon = proposed.Icon
		case "last_checked":
			entry.LastChecked = proposed.LastChecked
		default:
			if after, ok := strings.CutPrefix(f.Field, "SHA256:"); ok {
				for j := range entry.Files {
					if entry.Files[j].Filename == after {
						for _, pf := range proposed.Files {
							if pf.Filename == after {
								entry.Files[j].SHA256 = pf.SHA256
								break
							}
						}
						break
					}
				}
			}
		}
	}

	return entry
}

// hasReviewData checks if any finding has been reviewed.
func hasReviewData(report *PersistentReport) bool {
	for _, f := range report.Findings {
		if f.Accepted != nil {
			return true
		}
	}
	return report.ReviewStatus != ""
}

// ReportSummary is the slim JSON returned by the list endpoint.
type ReportSummary struct {
	Name           string `json:"name"`
	ReviewStatus   string `json:"review_status"`
	FindingCount   int    `json:"finding_count"`
	SafetyOK       bool   `json:"safety_ok"`
	HFRepo         string `json:"hf_repo"`
	CheckedAt      string `json:"checked_at"`
	Downloads      int    `json:"downloads"`
	HFLastModified string `json:"last_modified_hf"`
}

// ReviewUpdate is the JSON body for PUT /api/reports/{name}.
type ReviewUpdate struct {
	ReviewStatus string `json:"review_status"`
	Findings     []struct {
		Index    int  `json:"index"`
		Accepted bool `json:"accepted"`
	} `json:"findings"`
}

// ReviewServer serves the review web UI and API.
type ReviewServer struct {
	dir     string
	reports []*PersistentReport
	byName  map[string]*PersistentReport
	mu      sync.RWMutex
}

func NewReviewServer(dir string) (*ReviewServer, error) {
	reports, err := LoadReports(dir)
	if err != nil {
		return nil, fmt.Errorf("load reports: %w", err)
	}

	byName := make(map[string]*PersistentReport, len(reports))
	for _, r := range reports {
		byName[r.Name] = r
	}

	return &ReviewServer{
		dir:     dir,
		reports: reports,
		byName:  byName,
	}, nil
}

func (rs *ReviewServer) ReportCount() int {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return len(rs.reports)
}

func (rs *ReviewServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", rs.handleIndex)
	mux.HandleFunc("GET /api/reports", rs.handleListReports)
	mux.HandleFunc("GET /api/reports/{name}", rs.handleGetReport)
	mux.HandleFunc("PUT /api/reports/{name}", rs.handleSaveReview)
	return mux
}

func (rs *ReviewServer) handleIndex(w http.ResponseWriter, _ *http.Request) {
	data, err := reviewFS.ReadFile("templates/review.html")
	if err != nil {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func (rs *ReviewServer) handleListReports(w http.ResponseWriter, _ *http.Request) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	summaries := make([]ReportSummary, len(rs.reports))
	for i, r := range rs.reports {
		summaries[i] = ReportSummary{
			Name:           r.Name,
			ReviewStatus:   r.ReviewStatus,
			FindingCount:   len(r.Findings),
			SafetyOK:       r.SafetyOK,
			HFRepo:         r.HFRepo,
			CheckedAt:      r.CheckedAt,
			Downloads:      r.Downloads,
			HFLastModified: r.HFLastModified,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(summaries)
}

func (rs *ReviewServer) handleGetReport(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	rs.mu.RLock()
	report, ok := rs.byName[name]
	rs.mu.RUnlock()

	if !ok {
		http.Error(w, "report not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(report)
}

func (rs *ReviewServer) handleSaveReview(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var update ReviewUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	rs.mu.Lock()
	report, ok := rs.byName[name]
	if !ok {
		rs.mu.Unlock()
		http.Error(w, "report not found", http.StatusNotFound)
		return
	}

	report.ReviewStatus = update.ReviewStatus
	report.ReviewedAt = time.Now().Format("2006-01-02")

	for _, fu := range update.Findings {
		if fu.Index >= 0 && fu.Index < len(report.Findings) {
			accepted := fu.Accepted
			report.Findings[fu.Index].Accepted = &accepted
		}
	}
	rs.mu.Unlock()

	if err := WriteReportFiles(rs.dir, report); err != nil {
		log.Printf("save review %s: %v", name, err)
		http.Error(w, "save failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "saved"})
}
