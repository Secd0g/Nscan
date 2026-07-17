// Package loader fetches user plugins from the nscan server and registers
// them with the PipelineEngine at startup and on a refresh ticker.
package loader

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/yourname/nscan/internal/scanner/engine"
	pluginruntime "github.com/yourname/nscan/internal/scanner/plugins/runtime"
	"github.com/yourname/nscan/pkg/models"
	"github.com/yourname/nscan/pkg/pluginsdk"
	"go.uber.org/zap"
)

// pluginStageAdapter wraps a pluginsdk.Stage as an engine.Stage.
type pluginStageAdapter struct {
	name  string
	inner pluginsdk.Stage
}

func (a *pluginStageAdapter) Name() string { return a.name }

func (a *pluginStageAdapter) Run(ctx context.Context, input *engine.StageInput, params map[string]string,
	results chan<- *engine.ScanResult, progress chan<- *engine.Progress) (*engine.StageInput, error) {

	sdkIn := &pluginsdk.StageInput{
		Targets:     input.Targets,
		Subdomains:  input.Subdomains,
		Hosts:       input.Hosts,
		HTTPURLs:    input.HTTPURLs,
		HTTPTechMap: input.HTTPTechMap,
	}
	sdkResults := make(chan *pluginsdk.ScanResult, 100)
	sdkProgress := make(chan *pluginsdk.Progress, 100)

	// Fan-out SDK channels to engine channels
	go func() {
		for r := range sdkResults {
			results <- &engine.ScanResult{Type: r.Type, Data: r.Data}
		}
	}()
	go func() {
		for p := range sdkProgress {
			progress <- &engine.Progress{
				Stage:   a.name,
				Percent: p.Percent,
				Message: p.Message,
				Log:     p.Log,
				Level:   p.Level,
			}
		}
	}()

	sdkOut, err := pluginruntime.NewSandbox(a.inner, 5*time.Minute).Run(ctx, sdkIn, params, sdkResults, sdkProgress)
	close(sdkResults)
	close(sdkProgress)
	if err != nil || sdkOut == nil {
		return nil, err
	}
	return &engine.StageInput{
		Targets:     sdkOut.Targets,
		Subdomains:  sdkOut.Subdomains,
		Hosts:       sdkOut.Hosts,
		HTTPURLs:    sdkOut.HTTPURLs,
		HTTPTechMap: sdkOut.HTTPTechMap,
	}, nil
}

// Loader fetches plugins from server HTTP API and registers them.
type Loader struct {
	serverHTTP string
	token      string
	eng        *engine.PipelineEngine
	log        *zap.Logger
	registered map[string]bool // plugin IDs already registered
}

func New(serverHTTP, token string, eng *engine.PipelineEngine, log *zap.Logger) *Loader {
	return &Loader{
		serverHTTP: strings.TrimRight(serverHTTP, "/"),
		token:      token,
		eng:        eng,
		log:        log,
		registered: make(map[string]bool),
	}
}

// LoadOnce fetches all enabled user plugins and registers new ones.
func (l *Loader) LoadOnce(ctx context.Context) error {
	if l.serverHTTP == "" {
		return nil
	}
	plugins, err := l.fetchPlugins(ctx)
	if err != nil {
		return fmt.Errorf("plugin loader: %w", err)
	}
	for _, p := range plugins {
		if p.SourceCode == "" || p.Builtin {
			continue
		}
		id := p.ID.Hex()
		if l.registered[id] {
			continue
		}
		stage, err := pluginruntime.LoadFromSource(p.SourceCode)
		if err != nil {
			l.log.Warn("failed to load plugin", zap.String("name", p.Name), zap.Error(err))
			continue
		}
		manifest := stage.GetManifest()
		stageName := manifest.Capability
		if stageName == "" {
			stageName = p.Module
		}
		adapter := &pluginStageAdapter{name: stageName + ":" + p.Name, inner: stage}
		l.eng.Register(adapter)
		l.registered[id] = true
		l.log.Info("plugin loaded", zap.String("name", p.Name), zap.String("stage", adapter.name))
	}
	return nil
}

// Start starts a background goroutine that refreshes plugins every interval.
func (l *Loader) Start(ctx context.Context, interval time.Duration) {
	if l.serverHTTP == "" {
		return
	}
	go func() {
		_ = l.LoadOnce(ctx)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := l.LoadOnce(ctx); err != nil {
					l.log.Warn("plugin refresh failed", zap.Error(err))
				}
			}
		}
	}()
}

func (l *Loader) fetchPlugins(ctx context.Context) ([]models.Plugin, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.serverHTTP+"/api/v1/plugins", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+l.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, body)
	}
	var result struct {
		Data []models.Plugin `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}
