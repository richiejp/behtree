package galcheck

import (
	"context"
	"encoding/json"
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
	HF          *HFClient
	LLM         cogito.LLM // optional: LLM for synthesizing descriptions/tags
	LLMModel    string     // model name for chat completion requests
	MaxAge      time.Duration
	Limit       int  // max models to check (0 = unlimited)
	DryRun      bool // just scan, don't fetch/verify
	Verbose     bool
	Reports     []*ModelReport // accumulated reports
	checked     int            // count of models processed
	processed   map[int]bool   // entry indices already processed this session
	gallery     []GalleryEntry // cached gallery entries (loaded once)
}

// RegisterHandlers registers all gallery check action handlers on the registry.
func RegisterHandlers(registry *behtree.ActionRegistry, cfg *Config) {
	cfg.processed = make(map[int]bool)
	registry.Register("ScanGallery", scanGalleryHandler(cfg))
	registry.Register("FetchModelInfo", fetchModelInfoHandler(cfg))
	registry.Register("VerifyFiles", verifyFilesHandler(cfg))
	registry.Register("SynthesizeUpdate", synthesizeUpdateHandler(cfg))
	registry.Register("Idle", idleHandler(cfg))
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
				return behtree.HandlerResult{Status: behtree.Failure, Compatible: false}
			}
			cfg.gallery = entries
		}
		entries := cfg.gallery

		// Find first entry needing check (skip already processed this session)
		for i, entry := range entries {
			if cfg.processed[i] {
				continue
			}
			if !NeedsCheck(&entry, cfg.MaxAge) {
				continue
			}

			hfRepo := ExtractHFRepo(&entry)
			if hfRepo == "" {
				continue // skip entries without HF URLs
			}

			if cfg.Verbose {
				log.Printf("ScanGallery: selected model %q (entry #%d, HF: %s)", entry.Name, i, hfRepo)
			}

			// Store model data in state
			_ = s.Set("model_check", "loaded", "true")
			_ = s.Set("model_check", "info_fetched", "false")
			_ = s.Set("model_check", "files_verified", "false")
			_ = s.Set("model_check", "name", entry.Name)
			_ = s.Set("model_check", "entry_index", i)
			_ = s.Set("model_check", "hf_repo", hfRepo)

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

func fetchModelInfoHandler(cfg *Config) behtree.Handler {
	return func(_ behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
		if req == behtree.RequestFailure {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
		}

		hfRepo, _ := s.Get("model_check", "hf_repo")
		repoID, ok := hfRepo.(string)
		if !ok || repoID == "" {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: false}
		}

		name, _ := s.Get("model_check", "name")
		if cfg.Verbose {
			log.Printf("FetchModelInfo: fetching metadata for %s (HF: %s)", name, repoID)
		}

		if cfg.DryRun {
			_ = s.Set("model_check", "info_fetched", "true")
			return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
		}

		// Fetch model metadata from HF API
		info, err := cfg.HF.GetModelInfo(repoID)
		if err != nil {
			log.Printf("FetchModelInfo: %v", err)
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: false}
		}

		infoJSON, _ := json.Marshal(info)
		_ = s.Set("model_check", "hf_metadata", string(infoJSON))

		// Fetch README/model card
		readme, err := cfg.HF.GetReadme(repoID)
		if err != nil {
			if cfg.Verbose {
				log.Printf("FetchModelInfo: README not available: %v", err)
			}
			readme = ""
		}
		// Truncate to reasonable size for state storage
		if len(readme) > 8000 {
			readme = readme[:8000]
		}
		_ = s.Set("model_check", "hf_model_card", readme)

		// Fetch file list with SHA256 hashes
		files, err := cfg.HF.ListFiles(repoID)
		if err != nil {
			log.Printf("FetchModelInfo: failed to list files: %v", err)
			// Non-fatal: we can still check other metadata
			_ = s.Set("model_check", "hf_file_shas", "{}")
		} else {
			shaMap := make(map[string]string)
			for _, f := range files {
				if f.LFS != nil && f.LFS.Oid != "" {
					shaMap[filepath.Base(f.Path)] = f.LFS.Oid
				}
			}
			shaJSON, _ := json.Marshal(shaMap)
			_ = s.Set("model_check", "hf_file_shas", string(shaJSON))
		}

		_ = s.Set("model_check", "info_fetched", "true")

		if cfg.Verbose {
			log.Printf("FetchModelInfo: done for %s", name)
		}

		return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
	}
}

func verifyFilesHandler(cfg *Config) behtree.Handler {
	return func(_ behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
		if req == behtree.RequestFailure {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
		}

		name, _ := s.Get("model_check", "name")
		hfRepo, _ := s.Get("model_check", "hf_repo")
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
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: false}
		}

		// Load HF file SHAs
		shaJSON, _ := s.Get("model_check", "hf_file_shas")
		var hfSHAs map[string]string
		if str, ok := shaJSON.(string); ok && str != "" {
			_ = json.Unmarshal([]byte(str), &hfSHAs)
		}
		if hfSHAs == nil {
			hfSHAs = make(map[string]string)
		}

		var results []FileCheckResult
		for _, f := range entry.Files {
			result := FileCheckResult{
				Filename:    f.Filename,
				URI:         f.URI,
				ExpectedSHA: f.SHA256,
			}

			// Check SHA256 against HF metadata (fetched by FetchModelInfo)
			baseName := filepath.Base(f.Filename)
			if hfSHA, ok := hfSHAs[baseName]; ok {
				result.ActualSHA = hfSHA
				result.SHAMatch = strings.EqualFold(f.SHA256, hfSHA)
			} else {
				result.Error = "no HF SHA available for this file"
			}

			// Check URI accessibility
			accessible, statusCode, err := cfg.HF.CheckFileAccessible(f.URI)
			result.Accessible = accessible
			result.StatusCode = statusCode
			if err != nil {
				result.Error = fmt.Sprintf("accessibility: %v", err)
			}

			results = append(results, result)
		}

		resultsJSON, _ := json.Marshal(results)
		_ = s.Set("model_check", "file_results", string(resultsJSON))

		// Safety scan
		repoID := hfRepo.(string)
		scan, err := cfg.HF.SafetyScan(repoID)
		if err != nil {
			if cfg.Verbose {
				log.Printf("VerifyFiles: safety scan unavailable: %v", err)
			}
			_ = s.Set("model_check", "safety_ok", "unknown")
		} else if scan.HasUnsafeFile {
			_ = s.Set("model_check", "safety_ok", "false")
			_ = s.Set("model_check", "safety_note", fmt.Sprintf("unsafe files detected: clamav=%v pickles=%v",
				scan.ClamAVInfectedFiles, scan.DangerousPickles))
		} else {
			_ = s.Set("model_check", "safety_ok", "true")
		}

		_ = s.Set("model_check", "files_verified", "true")

		if cfg.Verbose {
			log.Printf("VerifyFiles: done for %s (%d files checked)", name, len(results))
		}

		return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
	}
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

		// Load current entry
		entryJSON, _ := s.Get("model_check", "current_entry")
		var entry GalleryEntry
		_ = json.Unmarshal([]byte(entryJSON.(string)), &entry)

		// Load HF metadata
		var hfInfo HFModelInfo
		if metaJSON, _ := s.Get("model_check", "hf_metadata"); metaJSON != nil {
			if str, ok := metaJSON.(string); ok {
				_ = json.Unmarshal([]byte(str), &hfInfo)
			}
		}

		// Load file results
		var fileResults []FileCheckResult
		if frJSON, _ := s.Get("model_check", "file_results"); frJSON != nil {
			if str, ok := frJSON.(string); ok {
				_ = json.Unmarshal([]byte(str), &fileResults)
			}
		}

		// Build report
		report := &ModelReport{
			Name:        entry.Name,
			EntryIndex:  entryIdx.(int),
			HFRepo:      hfRepo.(string),
			FileResults: fileResults,
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
			modelCard := ""
			if mc, _ := s.Get("model_check", "hf_model_card"); mc != nil {
				modelCard, _ = mc.(string)
			}
			suggestion, llmErr := callLLM(cfg, &entry, modelCard, &hfInfo)
			if llmErr != nil {
				log.Printf("SynthesizeUpdate: LLM call failed: %v", llmErr)
				// Non-fatal: fall back to automated checks
			} else {
				llmSuggestion = suggestion
			}
		}

		// Generate findings (automated + LLM-assisted if available)
		report.Findings = generateFindings(&entry, &hfInfo, cfg.DryRun, llmSuggestion)

		cfg.Reports = append(cfg.Reports, report)
		cfg.checked++
		cfg.processed[report.EntryIndex] = true

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

// generateFindings compares current entry metadata against HF metadata.
// If llm is non-nil, uses LLM suggestions for tags, description, and usecases.
func generateFindings(entry *GalleryEntry, hf *HFModelInfo, dryRun bool, llm *LLMSuggestion) []Finding {
	var findings []Finding

	if dryRun {
		findings = append(findings, Finding{
			Field:    "last_checked",
			Current:  entry.LastChecked,
			Proposed: time.Now().Format("2006-01-02"),
			Source:   "scan",
		})
		return findings
	}

	// License
	hfLicense := ExtractLicense(hf.Tags)
	if hfLicense != "" && entry.License != hfLicense {
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
		proposed := "(needs model card summary)"
		source := "model card"
		if llm != nil && llm.Description != "" {
			proposed = llm.Description
			source = "LLM from model card"
		}
		findings = append(findings, Finding{
			Field:    "Description",
			Current:  truncateStr(desc, 60),
			Proposed: proposed,
			Source:   source,
		})
	}

	// Tags — always review when LLM is available, fallback to HF for empty/default
	if llm != nil && len(llm.Tags) > 0 {
		if !sameStringSet(entry.Tags, llm.Tags) {
			findings = append(findings, Finding{
				Field:    "Tags",
				Current:  fmt.Sprintf("%v", entry.Tags),
				Proposed: fmt.Sprintf("%v", llm.Tags),
				Source:   "LLM from model card",
			})
		}
	} else if len(entry.Tags) == 0 || hasOnlyDefaultTags(entry.Tags) {
		proposed := suggestTags(hf)
		if len(proposed) > 0 {
			findings = append(findings, Finding{
				Field:    "Tags",
				Current:  fmt.Sprintf("%v", entry.Tags),
				Proposed: fmt.Sprintf("%v", proposed),
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
		findings = append(findings, Finding{
			Field:    "Icon",
			Current:  "",
			Proposed: fmt.Sprintf("https://cdn-avatars.huggingface.co/v1/production/uploads/%s", hf.Author),
			Source:   "HF author avatar",
		})
	}

	// Last checked
	findings = append(findings, Finding{
		Field:    "last_checked",
		Current:  entry.LastChecked,
		Proposed: time.Now().Format("2006-01-02"),
		Source:   "scan",
	})

	return findings
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
Do NOT include the license as a tag.`

// LLMSuggestion is the JSON structure expected from the LLM.
type LLMSuggestion struct {
	Tags          []string `json:"tags"`
	KnownUsecases []string `json:"known_usecases"`
	Description   string   `json:"description"`
}

// callLLM asks the LLM to suggest tags, usecases, and description from the model card.
func callLLM(cfg *Config, entry *GalleryEntry, modelCard string, hf *HFModelInfo) (*LLMSuggestion, error) {
	if cfg.LLM == nil {
		return nil, nil
	}

	// Take first ~100 lines of model card
	lines := strings.Split(modelCard, "\n")
	if len(lines) > 100 {
		lines = lines[:100]
	}
	cardExcerpt := strings.Join(lines, "\n")

	// Build user prompt with current entry info and model card
	currentTags := "none"
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
		currentDesc = "none"
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
		currentUsecases = "none"
	}

	userPrompt := fmt.Sprintf(`## Model: %s
HuggingFace repo: %s
HF tags: %s
HF pipeline: %s

## Current gallery entry
Tags: %s
Description: %s
Known usecases: %s
Backend: %s

## Model card excerpt
%s`,
		entry.Name,
		hf.ModelID,
		strings.Join(hf.Tags, ", "),
		hf.PipelineTag,
		currentTags,
		truncateStr(currentDesc, 200),
		currentUsecases,
		fmt.Sprint(entry.Overrides["backend"]),
		cardExcerpt,
	)

	req := openai.ChatCompletionRequest{
		Model:     cfg.LLMModel,
		MaxTokens: 1024,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: metadataSystemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: userPrompt},
		},
	}

	reply, usage, err := cfg.LLM.CreateChatCompletion(context.Background(), req)
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
