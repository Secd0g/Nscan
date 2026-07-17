// Package configsync pulls centralized configuration (POCs, dictionaries,
// fingerprints) from the nscan server HTTP API and materializes it to local
// disk so scanner tools can consume the files directly.
package configsync

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yourname/nscan/pkg/models"
	"go.uber.org/zap"
)

// ConfigSync pulls centralized configuration from the server HTTP API
// and materializes it to local disk for scanner tools to consume.
type ConfigSync struct {
	serverHTTP string // e.g. "http://localhost:8080"
	token      string
	dataDir    string // local dir to write configs
	log        *zap.Logger
	client     *http.Client
}

// New creates a ConfigSync. serverHTTP is the base URL of the nscan server,
// token is the bearer token for authentication, and dataDir is the local
// directory where synced files are written.
func New(serverHTTP, token, dataDir string, log *zap.Logger) *ConfigSync {
	return &ConfigSync{
		serverHTTP: strings.TrimRight(serverHTTP, "/"),
		token:      token,
		dataDir:    dataDir,
		log:        log,
		client:     &http.Client{Timeout: 2 * time.Minute},
	}
}

// DataDir returns the local data directory path.
func (s *ConfigSync) DataDir() string { return s.dataDir }

// POCDir returns the local directory where POC templates are written.
func (s *ConfigSync) POCDir() string { return filepath.Join(s.dataDir, "pocs") }

// DictDir returns the local directory where dictionaries are written.
func (s *ConfigSync) DictDir() string { return filepath.Join(s.dataDir, "dicts") }

// SyncAll pulls all config types from the server and writes to local disk.
func (s *ConfigSync) SyncAll(ctx context.Context) error {
	if s.serverHTTP == "" {
		s.log.Debug("configsync: no server_http configured, skipping")
		return nil
	}

	var errs []string

	if err := s.SyncPOCs(ctx); err != nil {
		errs = append(errs, "pocs: "+err.Error())
		s.log.Warn("configsync: SyncPOCs failed", zap.Error(err))
	}
	if err := s.SyncDictionaries(ctx); err != nil {
		errs = append(errs, "dicts: "+err.Error())
		s.log.Warn("configsync: SyncDictionaries failed", zap.Error(err))
	}
	if err := s.SyncFingerprints(ctx); err != nil {
		errs = append(errs, "fingerprints: "+err.Error())
		s.log.Warn("configsync: SyncFingerprints failed", zap.Error(err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("configsync partial failure: %s", strings.Join(errs, "; "))
	}
	return nil
}

// SyncPOCs pulls nuclei templates and custom POCs from the server and writes
// them as YAML files under dataDir/pocs/.
func (s *ConfigSync) SyncPOCs(ctx context.Context) error {
	dir := s.POCDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir pocs: %w", err)
	}

	written := 0

	// 1. Nuclei templates (paginated — list returns IDs without content,
	//    then fetch content individually)
	tplIDs, err := s.listTemplateIDs(ctx)
	if err != nil {
		return fmt.Errorf("list templates: %w", err)
	}
	for _, tid := range tplIDs {
		tpl, err := s.getTemplateContent(ctx, tid)
		if err != nil {
			s.log.Warn("configsync: fetch template content failed", zap.String("id", tid), zap.Error(err))
			continue
		}
		if tpl.Content == "" {
			continue
		}
		fname := sanitizeFilename(tid) + ".yaml"
		if err := os.WriteFile(filepath.Join(dir, fname), []byte(tpl.Content), 0o644); err != nil {
			s.log.Warn("configsync: write template failed", zap.String("id", tid), zap.Error(err))
			continue
		}
		written++
	}

	// 2. Custom POCs — use the export endpoint which returns a zip of all
	//    enabled custom POCs with their YAML content.
	if err := s.syncCustomPOCsFromExport(ctx, dir, &written); err != nil {
		s.log.Warn("configsync: sync custom POCs failed", zap.Error(err))
	}

	s.log.Info("configsync: POCs synced", zap.Int("written", written))
	return nil
}

// SyncDictionaries pulls wordlists from the server and writes them as text
// files under dataDir/dicts/.
func (s *ConfigSync) SyncDictionaries(ctx context.Context) error {
	dir := s.DictDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir dicts: %w", err)
	}

	dicts, err := s.listDicts(ctx)
	if err != nil {
		return fmt.Errorf("list dicts: %w", err)
	}

	written := 0
	for _, d := range dicts {
		content, err := s.getDictContent(ctx, d.ID.Hex())
		if err != nil {
			s.log.Warn("configsync: fetch dict content failed", zap.String("name", d.Name), zap.Error(err))
			continue
		}
		// Organize by category/service
		subdir := dir
		if d.Category != "" {
			subdir = filepath.Join(dir, d.Category)
		}
		if d.Service != "" {
			subdir = filepath.Join(subdir, d.Service)
		}
		if err := os.MkdirAll(subdir, 0o755); err != nil {
			continue
		}
		fname := sanitizeFilename(d.Name) + ".txt"
		if err := os.WriteFile(filepath.Join(subdir, fname), []byte(content), 0o644); err != nil {
			s.log.Warn("configsync: write dict failed", zap.String("name", d.Name), zap.Error(err))
			continue
		}
		written++
	}

	s.log.Info("configsync: dictionaries synced", zap.Int("written", written))
	return nil
}

// SyncFingerprints pulls fingerprint rules from the server and writes them
// as a single JSON file at dataDir/fingerprints.json.
func (s *ConfigSync) SyncFingerprints(ctx context.Context) error {
	if err := os.MkdirAll(s.dataDir, 0o755); err != nil {
		return fmt.Errorf("mkdir datadir: %w", err)
	}

	fps, err := s.listFingerprints(ctx)
	if err != nil {
		return fmt.Errorf("list fingerprints: %w", err)
	}

	data, err := json.MarshalIndent(fps, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal fingerprints: %w", err)
	}

	outPath := filepath.Join(s.dataDir, "fingerprints.json")
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return fmt.Errorf("write fingerprints: %w", err)
	}

	s.log.Info("configsync: fingerprints synced", zap.Int("count", len(fps)))
	return nil
}

// Start begins periodic sync at the given interval. The first sync runs
// immediately in the goroutine (non-blocking for the caller).
func (s *ConfigSync) Start(ctx context.Context, interval time.Duration) {
	if s.serverHTTP == "" {
		return
	}
	go func() {
		if err := s.SyncAll(ctx); err != nil {
			s.log.Warn("configsync: initial periodic sync failed", zap.Error(err))
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.SyncAll(ctx); err != nil {
					s.log.Warn("configsync: periodic sync failed", zap.Error(err))
				}
			}
		}
	}()
}

// ── HTTP helpers ─────────────────────────────────────────────────────────────

func (s *ConfigSync) doGet(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.serverHTTP+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// listTemplateIDs fetches all nuclei template IDs (paginated with limit/skip).
func (s *ConfigSync) listTemplateIDs(ctx context.Context) ([]string, error) {
	var allIDs []string
	const batchSize = 200
	var skip int64
	for {
		path := fmt.Sprintf("/api/v1/poc/templates?limit=%d&skip=%d", batchSize, skip)
		body, err := s.doGet(ctx, path)
		if err != nil {
			return nil, err
		}
		var result struct {
			Data []struct {
				TemplateID string `json:"template_id"`
			} `json:"data"`
			Total int64 `json:"total"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, err
		}
		for _, t := range result.Data {
			if t.TemplateID != "" {
				allIDs = append(allIDs, t.TemplateID)
			}
		}
		if len(result.Data) < batchSize {
			break
		}
		skip += int64(len(result.Data))
	}
	return allIDs, nil
}

func (s *ConfigSync) getTemplateContent(ctx context.Context, templateID string) (*models.NucleiTemplate, error) {
	body, err := s.doGet(ctx, "/api/v1/poc/templates/"+templateID+"/content")
	if err != nil {
		return nil, err
	}
	var tpl models.NucleiTemplate
	if err := json.Unmarshal(body, &tpl); err != nil {
		return nil, err
	}
	return &tpl, nil
}

func (s *ConfigSync) syncCustomPOCsFromExport(ctx context.Context, dir string, written *int) error {
	body, err := s.doGet(ctx, "/api/v1/poc/custom/export")
	if err != nil {
		return err
	}
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return fmt.Errorf("invalid zip: %w", err)
	}
	for _, f := range reader.File {
		if f.FileInfo().IsDir() || !strings.HasSuffix(f.Name, ".yaml") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}
		fname := "custom-" + sanitizeFilename(strings.TrimSuffix(filepath.Base(f.Name), ".yaml")) + ".yaml"
		if err := os.WriteFile(filepath.Join(dir, fname), content, 0o644); err != nil {
			continue
		}
		*written++
	}
	return nil
}

func (s *ConfigSync) listDicts(ctx context.Context) ([]models.Dict, error) {
	body, err := s.doGet(ctx, "/api/v1/dicts")
	if err != nil {
		return nil, err
	}
	var result struct {
		Data []models.Dict `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

func (s *ConfigSync) getDictContent(ctx context.Context, dictID string) (string, error) {
	body, err := s.doGet(ctx, "/api/v1/dicts/"+dictID+"/content")
	if err != nil {
		return "", err
	}
	var result struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	return result.Content, nil
}

func (s *ConfigSync) listFingerprints(ctx context.Context) ([]models.Fingerprint, error) {
	var all []models.Fingerprint
	const batchSize = 200
	var skip int64
	for {
		path := fmt.Sprintf("/api/v1/fingerprints?limit=%d&skip=%d", batchSize, skip)
		body, err := s.doGet(ctx, path)
		if err != nil {
			return nil, err
		}
		var result struct {
			Data []models.Fingerprint `json:"data"`
			Total int64               `json:"total"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, err
		}
		all = append(all, result.Data...)
		if len(result.Data) < batchSize {
			break
		}
		skip += int64(len(result.Data))
	}
	return all, nil
}

// sanitizeFilename replaces path-unsafe characters.
func sanitizeFilename(s string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_")
	return r.Replace(s)
}
