package galcheck

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// HTTPError is returned when an HF API call gets a non-OK status code.
type HTTPError struct {
	StatusCode int
	Action     string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s: HTTP %d", e.Action, e.StatusCode)
}

// RateLimitBucket tracks the state of one HF rate-limit bucket.
type RateLimitBucket struct {
	Remaining int
	ResetAt   time.Time
}

type HFClient struct {
	baseURL  string
	client   *http.Client
	mu       sync.Mutex
	apiLimit RateLimitBucket
	resLimit RateLimitBucket
}

func NewHFClient(timeout time.Duration) *HFClient {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &HFClient{
		baseURL: "https://huggingface.co",
		client:  &http.Client{Timeout: timeout},
	}
}

type HFModelInfo struct {
	ModelID      string     `json:"modelId"`
	Author       string     `json:"author"`
	AvatarURL    string     `json:"avatarUrl,omitempty"`
	Tags         []string   `json:"tags"`
	PipelineTag  string     `json:"pipelineTag"`
	Downloads    int        `json:"downloads"`
	Private      bool       `json:"private"`
	LastModified string     `json:"lastModified"`
	CardData     HFCardData `json:"cardData"`
}

type HFCardData struct {
	LicenseName string `json:"license_name"`
}

type HFFileInfo struct {
	Type string     `json:"type"`
	Path string     `json:"path"`
	Oid  string     `json:"oid"`
	Size int64      `json:"size"`
	LFS  *HFLFSInfo `json:"lfs,omitempty"`
}

// LFS OID is the SHA256 hash of the file content.
type HFLFSInfo struct {
	Oid  string `json:"oid"`
	Size int64  `json:"size"`
}

type HFScanResult struct {
	HasUnsafeFile       bool     `json:"hasUnsafeFile"`
	ClamAVInfectedFiles []string `json:"clamAVInfectedFiles"`
	DangerousPickles    []string `json:"dangerousPickles"`
	ScansDone           bool     `json:"scansDone"`
}

func (c *HFClient) GetModelInfo(repoID string) (*HFModelInfo, error) {
	url := fmt.Sprintf("%s/api/models/%s", c.baseURL, repoID)

	resp, err := c.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch model info: %w", err)
	}
	defer resp.Body.Close()
	c.updateRateLimits(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPError{StatusCode: resp.StatusCode, Action: "fetch model info"}
	}

	var info HFModelInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decode model info: %w", err)
	}

	return &info, nil
}

// ListFiles lists all files in a HuggingFace repo, recursing into subdirectories.
func (c *HFClient) ListFiles(repoID string) ([]HFFileInfo, error) {
	return c.listFilesInPath(repoID, "")
}

func (c *HFClient) listFilesInPath(repoID, path string) ([]HFFileInfo, error) {
	var url string
	if path == "" {
		url = fmt.Sprintf("%s/api/models/%s/tree/main", c.baseURL, repoID)
	} else {
		url = fmt.Sprintf("%s/api/models/%s/tree/main/%s", c.baseURL, repoID, path)
	}

	resp, err := c.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	defer resp.Body.Close()
	c.updateRateLimits(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list files: HTTP %d", resp.StatusCode)
	}

	var items []HFFileInfo
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("decode file list: %w", err)
	}

	var allFiles []HFFileInfo
	for _, item := range items {
		switch item.Type {
		case "directory", "folder":
			subFiles, err := c.listFilesInPath(repoID, item.Path)
			if err != nil {
				return nil, err
			}
			allFiles = append(allFiles, subFiles...)
		case "file":
			allFiles = append(allFiles, item)
		}
	}

	return allFiles, nil
}

func (c *HFClient) GetFileSHA(repoID, filename string) (string, error) {
	files, err := c.ListFiles(repoID)
	if err != nil {
		return "", err
	}

	for _, f := range files {
		if filepath.Base(f.Path) == filename {
			if f.LFS != nil && f.LFS.Oid != "" {
				return f.LFS.Oid, nil
			}
			return f.Oid, nil
		}
	}

	return "", fmt.Errorf("file %q not found in repo %s", filename, repoID)
}

func (c *HFClient) GetReadme(repoID string) (string, error) {
	url := fmt.Sprintf("%s/%s/raw/main/README.md", c.baseURL, repoID)

	resp, err := c.client.Get(url)
	if err != nil {
		return "", fmt.Errorf("fetch readme: %w", err)
	}
	defer resp.Body.Close()
	c.updateRateLimits(resp)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch readme: HTTP %d", resp.StatusCode)
	}

	// Limit read to 64KB to avoid unbounded memory on large READMEs.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", fmt.Errorf("read readme: %w", err)
	}

	return string(body), nil
}

func (c *HFClient) SafetyScan(repoID string) (*HFScanResult, error) {
	url := fmt.Sprintf("%s/api/models/%s/scan", c.baseURL, repoID)

	resp, err := c.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("safety scan: %w", err)
	}
	defer resp.Body.Close()
	c.updateRateLimits(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("safety scan: HTTP %d", resp.StatusCode)
	}

	var result HFScanResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode scan: %w", err)
	}

	return &result, nil
}

func (c *HFClient) CheckFileAccessible(uri string) (bool, int, error) {
	// Resolve huggingface:// URIs
	resolved := resolveHFURI(uri, c.baseURL)

	req, err := http.NewRequest("HEAD", resolved, nil)
	if err != nil {
		return false, 0, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return false, 0, fmt.Errorf("head request: %w", err)
	}
	defer resp.Body.Close()
	c.updateRateLimits(resp)

	return resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusFound, resp.StatusCode, nil
}

// parseRateLimits parses the HF RateLimit header.
// Format: "api";r=999;t=120, "resolvers";r=4999;t=120
func parseRateLimits(header string, now time.Time) (api, resolvers *RateLimitBucket) {
	for entry := range strings.SplitSeq(header, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		parts := strings.Split(entry, ";")
		if len(parts) < 1 {
			continue
		}

		bucket := strings.Trim(parts[0], `" `)
		var remaining int
		var resetSec int
		for _, kv := range parts[1:] {
			kv = strings.TrimSpace(kv)
			if after, ok := strings.CutPrefix(kv, "r="); ok {
				remaining, _ = strconv.Atoi(after)
			} else if after, ok := strings.CutPrefix(kv, "t="); ok {
				resetSec, _ = strconv.Atoi(after)
			}
		}

		b := &RateLimitBucket{
			Remaining: remaining,
			ResetAt:   now.Add(time.Duration(resetSec) * time.Second),
		}

		switch bucket {
		case "api":
			api = b
		case "resolvers":
			resolvers = b
		}
	}
	return api, resolvers
}

func (c *HFClient) updateRateLimits(resp *http.Response) {
	header := resp.Header.Get("RateLimit")
	if header == "" {
		return
	}

	now := time.Now()
	api, resolvers := parseRateLimits(header, now)

	c.mu.Lock()
	defer c.mu.Unlock()

	if api != nil {
		c.apiLimit = *api
	}
	if resolvers != nil {
		c.resLimit = *resolvers
	}
}

func (c *HFClient) bucketOK(b *RateLimitBucket) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return b.Remaining > 0 || time.Now().After(b.ResetAt)
}

// APILimitOK reports whether the API rate limit bucket has capacity.
func (c *HFClient) APILimitOK() bool { return c.bucketOK(&c.apiLimit) }

// ResolversLimitOK reports whether the resolvers rate limit bucket has capacity.
func (c *HFClient) ResolversLimitOK() bool { return c.bucketOK(&c.resLimit) }

// NextResetTime returns the latest reset time of any exhausted bucket.
func (c *HFClient) NextResetTime() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	var latest time.Time
	now := time.Now()
	if c.apiLimit.Remaining <= 0 && c.apiLimit.ResetAt.After(now) && c.apiLimit.ResetAt.After(latest) {
		latest = c.apiLimit.ResetAt
	}
	if c.resLimit.Remaining <= 0 && c.resLimit.ResetAt.After(now) && c.resLimit.ResetAt.After(latest) {
		latest = c.resLimit.ResetAt
	}
	return latest
}

// GetAvatarURL fetches the avatar URL for an HF author (org or user)
// and validates it with a HEAD request before returning.
func (c *HFClient) GetAvatarURL(author string) (string, error) {
	// Try organization first, then user
	for _, kind := range []string{"organizations", "users"} {
		apiURL := fmt.Sprintf("%s/api/%s/%s/avatar", c.baseURL, kind, author)

		avatarURL, ok := c.fetchAvatarJSON(apiURL)
		if !ok {
			continue
		}

		if c.headOK(avatarURL) {
			return avatarURL, nil
		}
	}

	return "", fmt.Errorf("no avatar found for %s", author)
}

func (c *HFClient) fetchAvatarJSON(url string) (string, bool) {
	resp, err := c.client.Get(url)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", false
	}

	var result struct {
		AvatarURL string `json:"avatarUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || result.AvatarURL == "" {
		return "", false
	}
	return result.AvatarURL, true
}

func (c *HFClient) headOK(url string) bool {
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return false
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func resolveHFURI(uri, baseURL string) string {
	var path string
	if p, ok := strings.CutPrefix(uri, "huggingface://"); ok {
		path = p
	} else if p, ok := strings.CutPrefix(uri, "hf://"); ok {
		path = p
	} else {
		return uri
	}
	parts := strings.SplitN(path, "/", 3)
	if len(parts) == 3 {
		return fmt.Sprintf("%s/%s/%s/resolve/main/%s", baseURL, parts[0], parts[1], parts[2])
	}
	return uri
}
