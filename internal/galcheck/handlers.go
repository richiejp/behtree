package galcheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/mudler/cogito"
	"github.com/richiejp/behtree"
	"github.com/richiejp/behtree/internal/llmgen"
	openai "github.com/sashabaranov/go-openai"
)

// Config holds configuration for the gallery check handlers.
type Config struct {
	GalleryPath string
	OutputDir   string // per-model output directory (enables resume)
	HF          *HFClient
	LLM         cogito.LLM // optional: LLM for synthesizing descriptions/tags
	LLMModel    string     // model name for chat completion requests
	MaxAge      time.Duration
	Timeout     time.Duration // timeout for LLM and HF API calls
	Limit       int           // max models to check (0 = unlimited)
	DryRun      bool          // just scan, don't fetch/verify
	Verbose     bool
	Reports     []*ModelReport  // accumulated reports
	checked     int             // count of models processed
	processed   map[string]bool // model names already processed
	gallery     []GalleryEntry  // cached gallery entries (loaded once)
}

// RepoInfo holds fetched metadata for a single HuggingFace repo.
type RepoInfo struct {
	RepoID   string            `json:"repo_id"`
	Metadata *HFModelInfo      `json:"metadata,omitempty"`
	Readme   string            `json:"readme,omitempty"`
	FileSHAs map[string]string `json:"file_shas"`
	Error    string            `json:"error,omitempty"`
}

// RegisterHandlers registers all gallery check action handlers on the registry.
func RegisterHandlers(registry *behtree.ActionRegistry, cfg *Config) {
	cfg.processed = make(map[string]bool)
	registry.Register("ScanGallery", scanGalleryHandler(cfg))
	registry.Register("ObserveRateLimit", observeRateLimitHandler(cfg))
	registry.Register("WaitForRateLimit", waitForRateLimitHandler(cfg))
	registry.Register("FetchModelInfo", fetchModelInfoHandler(cfg))
	registry.Register("VerifyFiles", verifyFilesHandler(cfg))
	registry.Register("SynthesizeUpdate", synthesizeUpdateHandler(cfg))
	registry.Register("Idle", idleHandler(cfg))
}

// ResumeFromDir loads existing reports from the output directory and
// pre-populates the processed map so those models are skipped.
func ResumeFromDir(cfg *Config) error {
	reports, err := LoadReports(cfg.OutputDir)
	if err != nil {
		return fmt.Errorf("resume: %w", err)
	}
	for _, r := range reports {
		cfg.processed[r.Name] = true
		cfg.checked++
		// Add to in-memory reports for summary
		cfg.Reports = append(cfg.Reports, &ModelReport{
			Name:          r.Name,
			EntryIndex:    r.EntryIndex,
			HFRepo:        r.HFRepo,
			HFRepos:       r.HFRepos,
			Findings:      r.Findings,
			FileResults:   r.FileResults,
			SafetyOK:      r.SafetyOK,
			SafetyNote:    r.SafetyNote,
			ProposedEntry: r.ProposedEntry,
		})
	}
	if cfg.Verbose && len(reports) > 0 {
		log.Printf("Resumed: %d models already processed", len(reports))
	}
	return nil
}

func scanGalleryHandler(cfg *Config) behtree.Handler {
	return func(_ behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
		if req == behtree.RequestFailure {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
		}

		_ = s.Set("gallery", "scanned", "true")

		// If a model is already loaded, don't replace it
		loaded, _ := s.Get("model_check", "loaded")
		if loaded == "true" {
			return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
		}

		// Check limit
		if cfg.Limit > 0 && cfg.checked >= cfg.Limit {
			_ = s.Set("model_check", "loaded", "false")
			return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
		}

		// Load gallery (cached after first read)
		if cfg.gallery == nil {
			entries, err := LoadGallery(cfg.GalleryPath)
			if err != nil {
				log.Printf("ScanGallery: %v", err)
				return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
			}
			cfg.gallery = entries
		}
		entries := cfg.gallery

		// Find first entry needing check (skip already processed this session)
		for i, entry := range entries {
			if cfg.processed[entry.Name] {
				continue
			}
			if !NeedsCheck(&entry, cfg.MaxAge) {
				continue
			}

			repos, fileMappings := ExtractHFRepos(&entry)
			if len(repos) == 0 {
				continue // skip entries without HF URLs
			}

			if cfg.Verbose {
				log.Printf("ScanGallery: selected model %q (entry #%d, HF repos: %v)", entry.Name, i, repos)
			}

			// Store model data in state
			_ = s.Set("model_check", "loaded", "true")
			_ = s.Set("model_check", "info_fetched", "false")
			_ = s.Set("model_check", "files_verified", "false")
			_ = s.Set("model_check", "name", entry.Name)
			_ = s.Set("model_check", "entry_index", i)
			_ = s.Set("model_check", "hf_repo", repos[0])

			reposJSON, _ := json.Marshal(repos)
			_ = s.Set("model_check", "hf_repos", string(reposJSON))

			mappingsJSON, _ := json.Marshal(fileMappings)
			_ = s.Set("model_check", "file_repo_mappings", string(mappingsJSON))

			entryJSON, _ := json.Marshal(entry)
			_ = s.Set("model_check", "current_entry", string(entryJSON))

			return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
		}

		// No models need checking
		if cfg.Verbose {
			log.Println("ScanGallery: no models need checking")
		}
		_ = s.Set("model_check", "loaded", "false")
		return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
	}
}

func observeRateLimitHandler(cfg *Config) behtree.Handler {
	return func(_ behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
		if req == behtree.RequestFailure {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
		}

		_ = s.Set("rate_limit", "observed", "true")

		apiOK := cfg.HF.APILimitOK()
		resOK := cfg.HF.ResolversLimitOK()

		if apiOK {
			_ = s.Set("rate_limit", "api_ok", "true")
		} else {
			_ = s.Set("rate_limit", "api_ok", "false")
		}
		if resOK {
			_ = s.Set("rate_limit", "resolvers_ok", "true")
		} else {
			_ = s.Set("rate_limit", "resolvers_ok", "false")
		}

		if cfg.Verbose {
			log.Printf("ObserveRateLimit: api_ok=%v resolvers_ok=%v", apiOK, resOK)
		}

		return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
	}
}

func waitForRateLimitHandler(cfg *Config) behtree.Handler {
	return func(_ behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
		if req == behtree.RequestFailure {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
		}

		if cfg.HF.APILimitOK() && cfg.HF.ResolversLimitOK() {
			_ = s.Set("rate_limit", "api_ok", "true")
			_ = s.Set("rate_limit", "resolvers_ok", "true")
			return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
		}

		resetAt := cfg.HF.NextResetTime()
		remaining := time.Until(resetAt)

		if remaining <= 0 {
			_ = s.Set("rate_limit", "api_ok", "true")
			_ = s.Set("rate_limit", "resolvers_ok", "true")
			return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
		}

		sleepDuration := min(remaining, 5*time.Second)
		log.Printf("WaitForRateLimit: rate limited, sleeping %v (reset in %v)", sleepDuration, remaining)
		time.Sleep(sleepDuration)

		return behtree.HandlerResult{Status: behtree.Running, Compatible: true}
	}
}

func fetchModelInfoHandler(cfg *Config) behtree.Handler {
	return func(_ behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
		if req == behtree.RequestFailure {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
		}

		name, _ := s.Get("model_check", "name")

		if cfg.DryRun {
			_ = s.Set("model_check", "info_fetched", "true")
			return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
		}

		// Load repo list
		var repos []string
		if raw, _ := s.Get("model_check", "hf_repos"); raw != nil {
			_ = json.Unmarshal([]byte(raw.(string)), &repos)
		}
		if len(repos) == 0 {
			// Fallback to single repo for backwards compat
			if r, _ := s.Get("model_check", "hf_repo"); r != nil {
				if rid, ok := r.(string); ok && rid != "" {
					repos = []string{rid}
				}
			}
		}
		if len(repos) == 0 {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: false}
		}

		if cfg.Verbose {
			log.Printf("FetchModelInfo: fetching metadata for %s (HF repos: %v)", name, repos)
		}

		allRepoInfo := make(map[string]*RepoInfo, len(repos))
		allFailed := true
		for _, repoID := range repos {
			ri := fetchSingleRepoInfo(cfg, repoID)
			allRepoInfo[repoID] = ri
			if ri.Metadata != nil {
				allFailed = false
			}
		}

		// If all repos failed with 401, mark for deletion
		if allFailed {
			log.Printf("FetchModelInfo: %s all repos inaccessible, marking for deletion", name)
			markForDeletion(cfg, s, repos[0], "all HF repos returned errors")
			return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
		}

		allInfoJSON, _ := json.Marshal(allRepoInfo)
		_ = s.Set("model_check", "all_repo_info", string(allInfoJSON))
		_ = s.Set("model_check", "info_fetched", "true")

		if cfg.Verbose {
			log.Printf("FetchModelInfo: done for %s (%d repos fetched)", name, len(repos))
		}

		return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
	}
}

func fetchSingleRepoInfo(cfg *Config, repoID string) *RepoInfo {
	ri := &RepoInfo{RepoID: repoID, FileSHAs: make(map[string]string)}

	info, err := cfg.HF.GetModelInfo(repoID)
	if err != nil {
		var httpErr *HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == 401 {
			ri.Error = "HTTP 401 (private or removed)"
		} else {
			ri.Error = err.Error()
		}
		if cfg.Verbose {
			log.Printf("FetchModelInfo: %s: %v", repoID, err)
		}
		return ri
	}
	ri.Metadata = info

	readme, err := cfg.HF.GetReadme(repoID)
	if err != nil {
		if cfg.Verbose {
			log.Printf("FetchModelInfo: %s README not available: %v", repoID, err)
		}
	} else {
		if len(readme) > 8000 {
			readme = readme[:8000]
		}
		ri.Readme = readme
	}

	files, err := cfg.HF.ListFiles(repoID)
	if err != nil {
		if cfg.Verbose {
			log.Printf("FetchModelInfo: %s failed to list files: %v", repoID, err)
		}
	} else {
		for _, f := range files {
			if f.LFS != nil && f.LFS.Oid != "" {
				ri.FileSHAs[filepath.Base(f.Path)] = f.LFS.Oid
			}
		}
	}

	return ri
}

func verifyFilesHandler(cfg *Config) behtree.Handler {
	return func(_ behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
		if req == behtree.RequestFailure {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
		}

		name, _ := s.Get("model_check", "name")
		if cfg.Verbose {
			log.Printf("VerifyFiles: checking files for %s", name)
		}

		if cfg.DryRun {
			_ = s.Set("model_check", "files_verified", "true")
			return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
		}

		// Load current entry
		entryJSON, _ := s.Get("model_check", "current_entry")
		var entry GalleryEntry
		if err := json.Unmarshal([]byte(entryJSON.(string)), &entry); err != nil {
			log.Printf("VerifyFiles: parse entry: %v", err)
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
		}

		var fileMappings []FileRepoMapping
		if raw, _ := s.Get("model_check", "file_repo_mappings"); raw != nil {
			_ = json.Unmarshal([]byte(raw.(string)), &fileMappings)
		}
		fileToRepo := make(map[string]string, len(fileMappings))
		for _, fm := range fileMappings {
			fileToRepo[fm.Filename] = fm.Repo
		}

		allRepoInfo := make(map[string]*RepoInfo)
		if raw, _ := s.Get("model_check", "all_repo_info"); raw != nil {
			_ = json.Unmarshal([]byte(raw.(string)), &allRepoInfo)
		}

		results := verifyEntryFiles(cfg.HF, &entry, fileToRepo, allRepoInfo)
		resultsJSON, _ := json.Marshal(results)
		_ = s.Set("model_check", "file_results", string(resultsJSON))

		var repos []string
		if raw, _ := s.Get("model_check", "hf_repos"); raw != nil {
			_ = json.Unmarshal([]byte(raw.(string)), &repos)
		}
		ok, note := runSafetyScans(cfg, repos)
		_ = s.Set("model_check", "safety_ok", ok)
		if note != "" {
			_ = s.Set("model_check", "safety_note", note)
		}

		_ = s.Set("model_check", "files_verified", "true")

		if cfg.Verbose {
			log.Printf("VerifyFiles: done for %s (%d files checked)", name, len(results))
		}

		return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
	}
}

func verifyEntryFiles(
	hf *HFClient, entry *GalleryEntry,
	fileToRepo map[string]string, allRepoInfo map[string]*RepoInfo,
) []FileCheckResult {
	var results []FileCheckResult
	for _, f := range entry.Files {
		result := FileCheckResult{
			Filename:    f.Filename,
			URI:         f.URI,
			ExpectedSHA: f.SHA256,
		}

		repo := fileToRepo[f.Filename]
		if repo == "" {
			repo = extractRepoFromURI(f.URI)
		}
		result.SourceRepo = repo

		if repo != "" {
			if ri, ok := allRepoInfo[repo]; ok && ri.FileSHAs != nil {
				baseName := filepath.Base(f.Filename)
				if hfSHA, ok := ri.FileSHAs[baseName]; ok {
					result.ActualSHA = hfSHA
					result.SHAMatch = strings.EqualFold(f.SHA256, hfSHA)
				} else {
					result.Error = fmt.Sprintf("file not found in %s file listing", repo)
				}
			} else if ri != nil && ri.Error != "" {
				result.Error = fmt.Sprintf("repo %s: %s", repo, ri.Error)
			} else {
				result.Error = fmt.Sprintf("no metadata for repo %s", repo)
			}
		} else {
			result.Error = "no HF repo could be determined for this file"
		}

		accessible, statusCode, err := hf.CheckFileAccessible(f.URI)
		result.Accessible = accessible
		result.StatusCode = statusCode
		if err != nil {
			result.Error = fmt.Sprintf("accessibility: %v", err)
		}

		results = append(results, result)
	}
	return results
}

// runSafetyScans runs safety scans on all repos, returns (status, note).
func runSafetyScans(cfg *Config, repos []string) (string, string) {
	safetyOK := true
	var notes []string
	for _, repoID := range repos {
		scan, err := cfg.HF.SafetyScan(repoID)
		if err != nil {
			if cfg.Verbose {
				log.Printf("VerifyFiles: %s safety scan unavailable: %v", repoID, err)
			}
			notes = append(notes, fmt.Sprintf("%s: scan unavailable", repoID))
			continue
		}
		if scan.HasUnsafeFile {
			safetyOK = false
			notes = append(notes, fmt.Sprintf("%s: clamav=%v pickles=%v",
				repoID, scan.ClamAVInfectedFiles, scan.DangerousPickles))
		}
	}

	note := strings.Join(notes, "; ")
	if !safetyOK {
		return "false", note
	}
	if len(notes) > 0 {
		return "unknown", note
	}
	return "true", ""
}

type synthesisState struct {
	entry       GalleryEntry
	repos       []string
	allRepoInfo map[string]*RepoInfo
	hfInfo      HFModelInfo
	fileResults []FileCheckResult
}

func loadSynthesisState(s *behtree.State) synthesisState {
	var st synthesisState

	entryJSON, _ := s.Get("model_check", "current_entry")
	_ = json.Unmarshal([]byte(entryJSON.(string)), &st.entry)

	if raw, _ := s.Get("model_check", "hf_repos"); raw != nil {
		_ = json.Unmarshal([]byte(raw.(string)), &st.repos)
	}
	st.allRepoInfo = make(map[string]*RepoInfo)
	if raw, _ := s.Get("model_check", "all_repo_info"); raw != nil {
		_ = json.Unmarshal([]byte(raw.(string)), &st.allRepoInfo)
	}
	// Derive hfInfo from first successful repo in allRepoInfo
	for _, repoID := range st.repos {
		if ri, ok := st.allRepoInfo[repoID]; ok && ri.Metadata != nil {
			st.hfInfo = *ri.Metadata
			break
		}
	}
	if frJSON, _ := s.Get("model_check", "file_results"); frJSON != nil {
		if str, ok := frJSON.(string); ok {
			_ = json.Unmarshal([]byte(str), &st.fileResults)
		}
	}
	return st
}

func synthesizeUpdateHandler(cfg *Config) behtree.Handler {
	return func(_ behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
		if req == behtree.RequestFailure {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
		}

		name, _ := s.Get("model_check", "name")
		entryIdx, _ := s.Get("model_check", "entry_index")
		hfRepo, _ := s.Get("model_check", "hf_repo")

		if cfg.Verbose {
			log.Printf("SynthesizeUpdate: generating report for %s", name)
		}

		st := loadSynthesisState(s)

		report := &ModelReport{
			Name:        st.entry.Name,
			EntryIndex:  entryIdx.(int),
			HFRepo:      hfRepo.(string),
			HFRepos:     st.repos,
			FileResults: st.fileResults,
		}

		// Safety
		safetyOK, _ := s.Get("model_check", "safety_ok")
		switch safetyOK {
		case "true":
			report.SafetyOK = true
		case "false":
			report.SafetyOK = false
			note, _ := s.Get("model_check", "safety_note")
			if n, ok := note.(string); ok {
				report.SafetyNote = n
			}
		default:
			report.SafetyOK = true
			report.SafetyNote = "scan unavailable"
		}

		// Call LLM for tag/usecase/description suggestions (if configured)
		var llmSuggestion *LLMSuggestion
		if cfg.LLM != nil && !cfg.DryRun {
			suggestion, llmErr := callLLM(cfg, &st.entry, st.allRepoInfo, st.repos)
			if llmErr != nil {
				log.Printf("SynthesizeUpdate: LLM call failed: %v", llmErr)
				return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
			}
			llmSuggestion = suggestion
		}

		// Select primary repo's metadata for generateFindings
		primaryHF := &st.hfInfo
		if llmSuggestion != nil && llmSuggestion.PrimaryRepo != "" {
			for _, r := range st.repos {
				if r == llmSuggestion.PrimaryRepo {
					if ri, ok := st.allRepoInfo[r]; ok && ri.Metadata != nil {
						primaryHF = ri.Metadata
					}
					report.HFRepo = r
					break
				}
			}
		}

		report.Findings, report.ProposedEntry = generateFindings(&st.entry, primaryHF, cfg.DryRun, llmSuggestion)

		recordReport(cfg, report, &st.entry)

		if cfg.Verbose {
			log.Printf("SynthesizeUpdate: report generated for %s (%d findings)", name, len(report.Findings))
		}

		// Clear model_check state for next model
		_ = s.Set("model_check", "loaded", "false")
		_ = s.Set("model_check", "info_fetched", "false")
		_ = s.Set("model_check", "files_verified", "false")

		return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
	}
}

// markForDeletion creates a report recommending removal of the current model,
// then clears model_check state so the next model can be processed.
func markForDeletion(cfg *Config, s *behtree.State, hfRepo, reason string) {
	entryJSON, _ := s.Get("model_check", "current_entry")
	var entry GalleryEntry
	_ = json.Unmarshal([]byte(entryJSON.(string)), &entry)

	entryIdx, _ := s.Get("model_check", "entry_index")

	report := &ModelReport{
		Name:       entry.Name,
		EntryIndex: entryIdx.(int),
		HFRepo:     hfRepo,
		Findings: []Finding{{
			Field:    "Delete",
			Current:  "present",
			Proposed: "remove",
			Source:   reason,
		}},
		SafetyOK:      true,
		ProposedEntry: nil, // nil signals deletion
	}

	recordReport(cfg, report, &entry)

	_ = s.Set("model_check", "loaded", "false")
	_ = s.Set("model_check", "info_fetched", "false")
	_ = s.Set("model_check", "files_verified", "false")
}

func recordReport(cfg *Config, report *ModelReport, original *GalleryEntry) {
	cfg.Reports = append(cfg.Reports, report)
	cfg.checked++
	cfg.processed[report.Name] = true

	if cfg.OutputDir != "" {
		pr := &PersistentReport{
			Name:          report.Name,
			EntryIndex:    report.EntryIndex,
			HFRepo:        report.HFRepo,
			HFRepos:       report.HFRepos,
			Findings:      report.Findings,
			FileResults:   report.FileResults,
			SafetyOK:      report.SafetyOK,
			SafetyNote:    report.SafetyNote,
			OriginalEntry: original,
			ProposedEntry: report.ProposedEntry,
			CheckedAt:     time.Now().Format("2006-01-02"),
		}
		if err := WriteReportFiles(cfg.OutputDir, pr); err != nil {
			log.Printf("SynthesizeUpdate: write report files: %v", err)
		}
	}
}

func idleHandler(cfg *Config) behtree.Handler {
	return func(_ behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
		if req == behtree.RequestFailure {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
		}

		_ = s.Set("system", "idle", "true")

		if cfg.Verbose {
			log.Printf("Idle: all models processed (%d checked)", cfg.checked)
		}

		return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
	}
}

// generateFindings compares current entry metadata against HF metadata and
// builds a proposed entry with all changes applied. If llm is non-nil, uses
// LLM suggestions for tags, description, and usecases.
func generateFindings(entry *GalleryEntry, hf *HFModelInfo, dryRun bool, llm *LLMSuggestion) ([]Finding, *GalleryEntry) {
	var findings []Finding

	// Deep-copy entry via JSON round-trip
	proposed := cloneEntry(entry)

	if dryRun {
		proposed.LastChecked = time.Now().Format("2006-01-02")
		findings = append(findings, Finding{
			Field:    "last_checked",
			Current:  entry.LastChecked,
			Proposed: proposed.LastChecked,
			Source:   "scan",
		})
		return findings, proposed
	}

	// License
	hfLicense := ExtractLicense(hf.Tags)
	if hfLicense != "" && entry.License != hfLicense {
		proposed.License = hfLicense
		findings = append(findings, Finding{
			Field:    "License",
			Current:  entry.License,
			Proposed: hfLicense,
			Source:   "HF metadata",
		})
	}

	// Description
	desc := entry.Description
	if desc == "" {
		if ov, ok := entry.Overrides["description"]; ok {
			desc, _ = ov.(string)
		}
	}
	if desc == "" || strings.HasPrefix(desc, "Imported from") {
		proposedDesc := "(needs model card summary)"
		source := "model card"
		if llm != nil && llm.Description != "" {
			proposedDesc = llm.Description
			source = "LLM from model card"
		}
		proposed.Description = proposedDesc
		// Clear overrides description if we're setting top-level
		if proposed.Overrides != nil {
			delete(proposed.Overrides, "description")
		}
		findings = append(findings, Finding{
			Field:    "Description",
			Current:  truncateStr(desc, 60),
			Proposed: proposedDesc,
			Source:   source,
		})
	}

	// Tags — always review when LLM is available, fallback to HF for empty/default
	if llm != nil && len(llm.Tags) > 0 {
		if !sameStringSet(entry.Tags, llm.Tags) {
			proposed.Tags = llm.Tags
			findings = append(findings, Finding{
				Field:    "Tags",
				Current:  fmt.Sprintf("%v", entry.Tags),
				Proposed: fmt.Sprintf("%v", llm.Tags),
				Source:   "LLM from model card",
			})
		}
	} else if len(entry.Tags) == 0 || hasOnlyDefaultTags(entry.Tags) {
		suggestedTags := suggestTags(hf)
		if len(suggestedTags) > 0 {
			proposed.Tags = suggestedTags
			findings = append(findings, Finding{
				Field:    "Tags",
				Current:  fmt.Sprintf("%v", entry.Tags),
				Proposed: fmt.Sprintf("%v", suggestedTags),
				Source:   "HF metadata",
			})
		}
	}

	// Known usecases — always review when LLM is available
	if llm != nil && len(llm.KnownUsecases) > 0 {
		var current []string
		if ov, ok := entry.Overrides["known_usecases"]; ok {
			if usecases, ok := ov.([]any); ok {
				for _, u := range usecases {
					current = append(current, fmt.Sprint(u))
				}
			}
		}
		if !sameStringSet(current, llm.KnownUsecases) {
			if proposed.Overrides == nil {
				proposed.Overrides = make(map[string]any)
			}
			// Store as []any for YAML compatibility
			usecaseAny := make([]any, len(llm.KnownUsecases))
			for i, u := range llm.KnownUsecases {
				usecaseAny[i] = u
			}
			proposed.Overrides["known_usecases"] = usecaseAny
			findings = append(findings, Finding{
				Field:    "known_usecases",
				Current:  fmt.Sprintf("%v", current),
				Proposed: fmt.Sprintf("%v", llm.KnownUsecases),
				Source:   "LLM from model card",
			})
		}
	}

	// Icon
	if entry.Icon == "" && hf.Author != "" {
		iconURL := fmt.Sprintf("https://cdn-avatars.huggingface.co/v1/production/uploads/%s", hf.Author)
		proposed.Icon = iconURL
		findings = append(findings, Finding{
			Field:    "Icon",
			Current:  "",
			Proposed: iconURL,
			Source:   "HF author avatar",
		})
	}

	// Last checked
	proposed.LastChecked = time.Now().Format("2006-01-02")
	findings = append(findings, Finding{
		Field:    "last_checked",
		Current:  entry.LastChecked,
		Proposed: proposed.LastChecked,
		Source:   "scan",
	})

	return findings, proposed
}

// cloneEntry deep-copies a GalleryEntry via JSON round-trip.
func cloneEntry(entry *GalleryEntry) *GalleryEntry {
	data, _ := json.Marshal(entry)
	var clone GalleryEntry
	_ = json.Unmarshal(data, &clone)
	return &clone
}

// sameStringSet returns true if a and b contain the same elements (order-independent).
func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]int, len(a))
	for _, s := range a {
		set[s]++
	}
	for _, s := range b {
		set[s]--
		if set[s] < 0 {
			return false
		}
	}
	return true
}

func hasOnlyDefaultTags(tags []string) bool {
	for _, t := range tags {
		if t != "default" {
			return false
		}
	}
	return true
}

func suggestTags(hf *HFModelInfo) []string {
	var tags []string
	seen := make(map[string]bool)

	for _, t := range hf.Tags {
		// Skip HF metadata tags like "license:..." or "region:..."
		if strings.Contains(t, ":") {
			continue
		}
		lower := strings.ToLower(t)
		if !seen[lower] {
			tags = append(tags, lower)
			seen[lower] = true
		}
	}

	if hf.PipelineTag != "" && !seen[strings.ToLower(hf.PipelineTag)] {
		tags = append(tags, strings.ToLower(hf.PipelineTag))
	}

	return tags
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

const metadataSystemPrompt = `You are an expert at classifying AI models for a local model gallery (LocalAI).
Given a model card (README) excerpt, the model's current gallery entry, and HuggingFace metadata,
produce updated metadata in JSON format.

## Output Format

Output a JSON object with these fields:
- "tags": array of descriptive tags for the model (see valid categories below)
- "known_usecases": array of LocalAI usecase strings (see valid values below)
- "description": a concise 1-3 sentence description of the model suitable for a gallery listing

## Valid known_usecases

These map to LocalAI capabilities. Pick all that apply:
- "chat" — conversational/instruction-following LLM
- "completion" — text completion
- "edit" — text editing
- "embeddings" — embedding/vector generation
- "rerank" — reranking/retrieval
- "image" — image generation (diffusers, stable diffusion, flux, etc.)
- "transcript" — speech-to-text / transcription
- "tts" — text-to-speech
- "sound_generation" — music/sound generation
- "video" — video generation
- "detection" — object detection
- "vad" — voice activity detection
- "tokenize" — tokenization only

Most GGUF LLMs should get ["chat"]. Multimodal LLMs that also do vision still get ["chat"].

## Tag categories

Tags are free-form but should be drawn from these categories:
- **Model family**: qwen, llama, mistral, phi, deepseek, gemma, falcon, granite, etc.
- **Task/capability**: chat, reasoning, multimodal, vision, code, math, function-calling, agent
- **Format**: gguf, quantized, diffusers, vllm, transformers
- **Size**: 0.8b, 3b, 7b, 13b, 27b, 70b, etc. (use the parameter count)
- **Type**: llm, text-to-image, text-to-speech, image-to-video, speech-recognition
- **Specialty**: instruction-tuned, distilled, moe (mixture of experts), multilingual

Keep tags lowercase. Include 5-12 relevant tags. Do NOT include HuggingFace-internal tags
like "transformers", "safetensors", "pytorch" unless they indicate the LocalAI backend.
Do NOT include the license as a tag.

## Primary Repo Selection

When the entry references files from multiple HuggingFace repos, identify which repo is
the PRIMARY model — the one that defines the entry's core purpose. Output its repo ID
in the "primary_repo" field.

Heuristics:
- The entry name and description describe the primary model's purpose
- Auxiliary components (text encoders, VAEs, tokenizers, autoencoders) are NOT the primary
- The primary repo's pipeline_tag should match the entry's purpose (e.g., image model → text-to-image)
- If there is only one repo, use that

Always output "primary_repo" even for single-repo entries.
Base your tags, usecases, and description on the PRIMARY model, not auxiliary components.`

// LLMSuggestion is the JSON structure expected from the LLM.
type LLMSuggestion struct {
	PrimaryRepo   string   `json:"primary_repo"`
	Tags          []string `json:"tags"`
	KnownUsecases []string `json:"known_usecases"`
	Description   string   `json:"description"`
}

func formatRepoSections(allRepoInfo map[string]*RepoInfo, repos []string) string {
	var b strings.Builder
	maxCardChars := 8000 / max(len(repos), 1)
	for _, repoID := range repos {
		ri := allRepoInfo[repoID]
		if ri == nil {
			continue
		}
		fmt.Fprintf(&b, "\n### Repo: %s\n", repoID)
		if ri.Error != "" {
			fmt.Fprintf(&b, "Error: %s\n", ri.Error)
			continue
		}
		if ri.Metadata != nil {
			fmt.Fprintf(&b, "Pipeline: %s\n", ri.Metadata.PipelineTag)
			fmt.Fprintf(&b, "Tags: %s\n", strings.Join(ri.Metadata.Tags, ", "))
		}
		if ri.Readme != "" {
			excerpt := ri.Readme
			if len(excerpt) > maxCardChars {
				excerpt = excerpt[:maxCardChars]
			}
			lines := strings.Split(excerpt, "\n")
			if len(lines) > 80 {
				lines = lines[:80]
			}
			fmt.Fprintf(&b, "Model card excerpt:\n%s\n", strings.Join(lines, "\n"))
		}
	}
	return b.String()
}

// buildLLMUserPrompt constructs the user prompt for model metadata synthesis.
func buildLLMUserPrompt(entry *GalleryEntry, allRepoInfo map[string]*RepoInfo, repos []string) string {
	const none = "none"

	currentTags := none
	if len(entry.Tags) > 0 {
		currentTags = strings.Join(entry.Tags, ", ")
	}
	currentDesc := entry.Description
	if currentDesc == "" {
		if ov, ok := entry.Overrides["description"]; ok {
			currentDesc, _ = ov.(string)
		}
	}
	if currentDesc == "" {
		currentDesc = none
	}

	var currentUsecases string
	if ov, ok := entry.Overrides["known_usecases"]; ok {
		if usecases, ok := ov.([]any); ok {
			parts := make([]string, len(usecases))
			for i, u := range usecases {
				parts[i] = fmt.Sprint(u)
			}
			currentUsecases = strings.Join(parts, ", ")
		}
	}
	if currentUsecases == "" {
		currentUsecases = none
	}

	var fileMappings strings.Builder
	for _, f := range entry.Files {
		repo := extractRepoFromURI(f.URI)
		if repo == "" {
			repo = "(non-HF)"
		}
		fmt.Fprintf(&fileMappings, "- %s → %s\n", f.Filename, repo)
	}

	return fmt.Sprintf(`## Model: %s

## Current gallery entry
Tags: %s
Description: %s
Known usecases: %s
Backend: %s

## HuggingFace Repos (%d repos referenced)
%s
### File → Repo mapping
%s`,
		entry.Name,
		currentTags,
		truncateStr(currentDesc, 200),
		currentUsecases,
		fmt.Sprint(entry.Overrides["backend"]),
		len(repos),
		formatRepoSections(allRepoInfo, repos),
		fileMappings.String(),
	)
}

// callLLM asks the LLM to suggest primary repo, tags, usecases, and description.
func callLLM(cfg *Config, entry *GalleryEntry, allRepoInfo map[string]*RepoInfo, repos []string) (*LLMSuggestion, error) {
	if cfg.LLM == nil {
		return nil, nil
	}

	userPrompt := buildLLMUserPrompt(entry, allRepoInfo, repos)

	req := openai.ChatCompletionRequest{
		Model:     cfg.LLMModel,
		MaxTokens: 1024,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: metadataSystemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: userPrompt},
		},
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	reply, usage, err := cfg.LLM.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM call: %w", err)
	}

	if cfg.Verbose {
		log.Printf("SynthesizeUpdate LLM: tokens prompt=%d completion=%d",
			usage.PromptTokens, usage.CompletionTokens)
	}

	choices := reply.ChatCompletionResponse.Choices
	if len(choices) == 0 {
		return nil, fmt.Errorf("LLM returned no choices")
	}

	content := llmgen.ExtractJSON(choices[0].Message.Content)

	var suggestion LLMSuggestion
	if err := json.Unmarshal([]byte(content), &suggestion); err != nil {
		return nil, fmt.Errorf("parse LLM response: %w (raw: %s)", err, truncateStr(content, 200))
	}

	return &suggestion, nil
}
