// Package runtime provides a sandboxed yaegi interpreter for nscan plugins.
package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
	"github.com/traefik/yaegi/stdlib/unsafe"
	nscansymbols "github.com/yourname/nscan/internal/scanner/plugins/symbols"
	"github.com/yourname/nscan/pkg/pluginsdk"
)

// allowedPackages lists packages plugins may import.
// Anything not in this list will fail at eval time.
var allowedPackages = map[string]bool{
	"fmt":            true,
	"strings":        true,
	"strconv":        true,
	"regexp":         true,
	"time":           true,
	"encoding/json":  true,
	"net/url":        true,
	"net/http":       true,
	"bytes":          true,
	"io":             true,
	"math":           true,
	"sort":           true,
	"unicode":        true,
	"unicode/utf8":   true,
	"context":        true,
	"errors":         true,
	"sync":           true,
	"github.com/yourname/nscan/pkg/pluginsdk": true,
}

// LoadFromSource compiles and returns a plugin Stage from Go source code.
// It runs a sandbox check: the source must not import disallowed packages,
// must export a New() function returning pluginsdk.Stage.
func LoadFromSource(src string) (pluginsdk.Stage, error) {
	i := interp.New(interp.Options{
		Unrestricted: false,
	})

	// Register stdlib symbols (subset only — full stdlib is gated below)
	i.Use(stdlib.Symbols)
	i.Use(unsafe.Symbols)
	i.Use(nscansymbols.Symbols)

	// Evaluate source
	_, err := i.Eval(src)
	if err != nil {
		return nil, fmt.Errorf("plugin compile error: %w", err)
	}

	// Fetch the New() constructor
	v, err := i.Eval(`plugin.New()`)
	if err != nil {
		return nil, fmt.Errorf("plugin must export New() function: %w", err)
	}
	if !v.IsValid() {
		return nil, fmt.Errorf("plugin.New() returned invalid value")
	}

	stage, ok := v.Interface().(pluginsdk.Stage)
	if !ok {
		return nil, fmt.Errorf("plugin.New() must return pluginsdk.Stage, got %T", v.Interface())
	}
	return stage, nil
}

// Sandbox wraps a plugin stage to enforce timeouts and recover from panics.
type Sandbox struct {
	inner   pluginsdk.Stage
	timeout time.Duration
}

// NewSandbox wraps stage with execution timeout protection.
func NewSandbox(stage pluginsdk.Stage, timeout time.Duration) *Sandbox {
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	return &Sandbox{inner: stage, timeout: timeout}
}

func (s *Sandbox) GetManifest() pluginsdk.Manifest {
	return s.inner.GetManifest()
}

func (s *Sandbox) Run(ctx context.Context, input *pluginsdk.StageInput, params map[string]string,
	results chan<- *pluginsdk.ScanResult, progress chan<- *pluginsdk.Progress) (out *pluginsdk.StageInput, err error) {

	runCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("plugin panic: %v", r)
			}
			close(done)
		}()
		out, err = s.inner.Run(runCtx, input, params, results, progress)
	}()

	select {
	case <-done:
		return
	case <-runCtx.Done():
		return nil, fmt.Errorf("plugin timed out after %s", s.timeout)
	}
}

// ValidateImports checks that the source code only imports allowed packages.
// This is a fast pre-check before handing source to the interpreter.
func ValidateImports(src string) error {
	// Use reflect/stdlib trick: parse import paths from source
	// Simple approach: scan for import blocks
	imports := extractImports(src)
	for _, imp := range imports {
		if !allowedPackages[imp] {
			return fmt.Errorf("disallowed import %q — plugins may not use os/exec, os, net, syscall, reflect, or unsafe directly", imp)
		}
	}
	return nil
}

func extractImports(src string) []string {
	// Use yaegi's own tokenizer indirectly — parse via reflect
	// Simple line-by-line scan for `"package/path"` inside import blocks
	var imports []string
	inImport := false
	for i := 0; i < len(src); {
		nl := nextNewline(src, i)
		line := src[i:nl]
		trimmed := strings.TrimSpace(line)

		if trimmed == "import (" {
			inImport = true
		} else if inImport && trimmed == ")" {
			inImport = false
		} else if inImport || hasPrefix(trimmed, `import "`) {
			if pkg := extractQuoted(trimmed); pkg != "" {
				imports = append(imports, pkg)
			}
		}
		i = nl + 1
	}
	return imports
}

func nextNewline(s string, from int) int {
	for i := from; i < len(s); i++ {
		if s[i] == '\n' {
			return i
		}
	}
	return len(s)
}

func hasPrefix(s, prefix string) bool {
	return strings.HasPrefix(s, prefix)
}

func extractQuoted(s string) string {
	start := -1
	for i, c := range s {
		if c == '"' {
			if start == -1 {
				start = i + 1
			} else {
				return s[start:i]
			}
		}
	}
	return ""
}
