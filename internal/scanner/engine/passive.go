package engine

import (
	"context"
	"encoding/json"

	"github.com/yourname/nscan/pkg/models"
	"go.uber.org/zap"
)

// PassiveScanner runs alongside the main pipeline, inspecting results
// without generating additional network traffic.
type PassiveScanner struct {
	rules  []PassiveRule
	log    *zap.Logger
	input  chan *ScanResult
	output chan *ScanResult
}

// PassiveRule defines a single passive detection rule.
type PassiveRule struct {
	ID       string
	Name     string
	Severity string // "info"|"low"|"medium"|"high"|"critical"
	Check    func(data []byte) []PassiveFinding
}

// PassiveFinding represents a single finding from a passive rule.
type PassiveFinding struct {
	RuleName string
	Severity string
	Detail   string
	Match    string // the matched content snippet
}

// NewPassiveScanner creates a passive scanner with the given buffer size.
func NewPassiveScanner(log *zap.Logger, bufSize int) *PassiveScanner {
	return &PassiveScanner{
		log:    log,
		input:  make(chan *ScanResult, bufSize),
		output: make(chan *ScanResult, bufSize),
	}
}

// AddRule registers a passive detection rule.
func (ps *PassiveScanner) AddRule(rule PassiveRule) {
	ps.rules = append(ps.rules, rule)
}

// Start begins the processing loop. It reads from input, runs all rules,
// and sends findings to output. It stops when ctx is cancelled or input is closed.
func (ps *PassiveScanner) Start(ctx context.Context) {
	go func() {
		defer close(ps.output)
		for {
			select {
			case <-ctx.Done():
				return
			case result, ok := <-ps.input:
				if !ok {
					return
				}
				ps.process(result)
			}
		}
	}()
}

// Feed sends a result for passive inspection. It drops the result if the
// input buffer is full to avoid blocking the main pipeline.
func (ps *PassiveScanner) Feed(result *ScanResult) {
	select {
	case ps.input <- result:
	default:
		ps.log.Debug("passive scanner input buffer full, dropping result")
	}
}

// Results returns the channel of passive findings.
func (ps *PassiveScanner) Results() <-chan *ScanResult {
	return ps.output
}

// Close signals that no more results will be fed.
func (ps *PassiveScanner) Close() {
	close(ps.input)
}

func (ps *PassiveScanner) process(result *ScanResult) {
	for _, rule := range ps.rules {
		findings := rule.Check(result.Data)
		for _, f := range findings {
			// Extract URL from the original result for context
			var url string
			var ha models.HTTPAsset
			if err := json.Unmarshal(result.Data, &ha); err == nil {
				url = ha.URL
			}

			asset := models.PassiveAsset{
				URL:      url,
				RuleName: f.RuleName,
				Severity: f.Severity,
				Detail:   f.Detail,
				Match:    f.Match,
			}
			sr, err := NewResult("passive", asset)
			if err != nil {
				ps.log.Error("failed to marshal passive finding", zap.Error(err))
				continue
			}
			select {
			case ps.output <- sr:
			default:
				ps.log.Debug("passive scanner output buffer full, dropping finding")
			}
		}
	}
}
