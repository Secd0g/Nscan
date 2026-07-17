package sensitive

import (
	"context"
	"strings"

	ahocorasick "github.com/petar-dambovaliev/aho-corasick"
	"github.com/trufflesecurity/trufflehog/v3/pkg/detectors"
	"github.com/trufflesecurity/trufflehog/v3/pkg/engine/defaults"
	"go.uber.org/zap"
)

// TruffleHogResult 单条 TruffleHog 命中
type TruffleHogResult struct {
	DetectorName string
	DetectorID   string
	Raw          string
	Verified     *bool
}

// TruffleHogScanner 封装 TruffleHog 检测器 + Aho-Corasick 关键字预过滤
type TruffleHogScanner struct {
	detectors []detectors.Detector
	keywords  []string
	kwToIdx   map[string][]int // keyword -> detector indices
	ac        ahocorasick.AhoCorasick
	log       *zap.Logger
}

func NewTruffleHogScanner(log *zap.Logger) (*TruffleHogScanner, error) {
	allDetectors := defaults.DefaultDetectors()

	kwToIdx := make(map[string][]int)
	var allKeywords []string
	for i, d := range allDetectors {
		for _, kw := range d.Keywords() {
			lower := strings.ToLower(kw)
			if _, exists := kwToIdx[lower]; !exists {
				allKeywords = append(allKeywords, lower)
			}
			kwToIdx[lower] = append(kwToIdx[lower], i)
		}
	}

	builder := ahocorasick.NewAhoCorasickBuilder(ahocorasick.Opts{
		MatchKind: ahocorasick.StandardMatch,
		DFA:       true,
	})
	ac := builder.Build(allKeywords)

	log.Info("TruffleHog scanner initialized",
		zap.Int("detectors", len(allDetectors)),
		zap.Int("keywords", len(allKeywords)),
	)

	return &TruffleHogScanner{
		detectors: allDetectors,
		keywords:  allKeywords,
		kwToIdx:   kwToIdx,
		ac:        ac,
		log:       log,
	}, nil
}

func (s *TruffleHogScanner) DetectorCount() int {
	return len(s.detectors)
}

// Scan 对 data 做 TruffleHog 检测：先 Aho-Corasick 预过滤，再只跑命中关键字的 detector
func (s *TruffleHogScanner) Scan(ctx context.Context, data []byte, verify bool) []TruffleHogResult {
	if len(data) == 0 {
		return nil
	}

	lower := strings.ToLower(string(data))
	matches := s.ac.FindAll(lower)
	if len(matches) == 0 {
		return nil
	}

	// 收集需要运行的 detector 索引（去重）
	needed := make(map[int]struct{})
	for _, m := range matches {
		kw := s.keywords[m.Pattern()]
		for _, idx := range s.kwToIdx[kw] {
			needed[idx] = struct{}{}
		}
	}

	var out []TruffleHogResult
	for idx := range needed {
		if ctx.Err() != nil {
			break
		}
		d := s.detectors[idx]
		results, err := d.FromData(ctx, verify, data)
		if err != nil {
			continue
		}
		for _, r := range results {
			raw := string(r.Raw)
			if len(raw) > 200 {
				raw = raw[:200] + "..."
			}
			verified := &r.Verified
			result := TruffleHogResult{
				DetectorName: r.DetectorType.String(),
				DetectorID:   r.DetectorType.String(),
				Raw:          raw,
				Verified:     verified,
			}
			if r.DetectorName != "" {
				result.DetectorName = r.DetectorName
			}
			out = append(out, result)
		}
	}
	return out
}
