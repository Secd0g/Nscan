// Package pluginsdk is the public API for nscan plugins.
// Plugin authors import this package to implement a scanner stage
// that can be interpreted at runtime via yaegi.
package pluginsdk

import "context"

// Manifest describes a plugin's metadata.
type Manifest struct {
	Name          string            `json:"name"`
	Version       string            `json:"version"`
	Author        string            `json:"author"`
	Description   string            `json:"description"`
	Capability    string            `json:"capability"` // subdomain|port|http|vuln|dir|sensitive
	DefaultParams map[string]string `json:"default_params,omitempty"`
}

// ScanResult is one output item from a plugin run.
type ScanResult struct {
	Type string // asset type
	Data []byte // JSON-encoded payload
}

// Progress is a progress or log event from a plugin.
type Progress struct {
	Stage   string
	Percent int32
	Message string
	Log     string
	Level   string // info|warn|error|debug
}

// StageInput is passed to a plugin's Run method.
type StageInput struct {
	Targets     []string
	Subdomains  []string
	Hosts       []string
	HTTPURLs    []string
	HTTPTechMap map[string][]string
}

// Stage is the interface every plugin must implement.
// The concrete type must be exported as "Plugin" in the plugin source:
//
//	type Plugin struct{}
//	func (p *Plugin) GetManifest() pluginsdk.Manifest { ... }
//	func (p *Plugin) Run(ctx context.Context, input *pluginsdk.StageInput, ...) (*pluginsdk.StageInput, error) { ... }
type Stage interface {
	GetManifest() Manifest
	Run(ctx context.Context, input *StageInput, params map[string]string,
		results chan<- *ScanResult, progress chan<- *Progress) (*StageInput, error)
}

// New is a convenience constructor plugins can expose.
// Plugins must define:
//
//	func New() pluginsdk.Stage { return &Plugin{} }
type Constructor func() Stage

// Log writes an info-level log message to the progress channel.
func Log(progress chan<- *Progress, msg string) {
	progress <- &Progress{Log: msg, Level: "info"}
}

// Warn writes a warn-level log message to the progress channel.
func Warn(progress chan<- *Progress, msg string) {
	progress <- &Progress{Log: msg, Level: "warn"}
}

// Emit sends a scan result to the results channel.
func Emit(results chan<- *ScanResult, typ string, data []byte) {
	results <- &ScanResult{Type: typ, Data: data}
}

// ReportProgress sends a progress percentage.
func ReportProgress(progress chan<- *Progress, pct int32, msg string) {
	progress <- &Progress{Percent: pct, Message: msg}
}
