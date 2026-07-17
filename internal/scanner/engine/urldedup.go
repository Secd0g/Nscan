package engine

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/cespare/xxhash/v2"
)

var numericSegmentRe = regexp.MustCompile(`\d+`)

// URLDedup deduplicates URLs by their structural pattern:
// scheme + host + path (with numeric path segments normalized) + sorted parameter names.
type URLDedup struct {
	seen map[uint64]struct{}
	mu   sync.Mutex
}

func NewURLDedup() *URLDedup {
	return &URLDedup{seen: make(map[uint64]struct{})}
}

// IsNew returns true if the URL's structural pattern has not been seen before.
func (d *URLDedup) IsNew(rawURL string) bool {
	key := urlPattern(rawURL)
	h := xxhash.Sum64String(key)

	d.mu.Lock()
	defer d.mu.Unlock()
	if _, exists := d.seen[h]; exists {
		return false
	}
	d.seen[h] = struct{}{}
	return true
}

// Reset clears all recorded patterns.
func (d *URLDedup) Reset() {
	d.mu.Lock()
	d.seen = make(map[uint64]struct{})
	d.mu.Unlock()
}

// FilterURLs returns only structurally unique URLs from the input slice.
func FilterURLs(urls []string) []string {
	d := NewURLDedup()
	result := make([]string, 0, len(urls))
	for _, u := range urls {
		if d.IsNew(u) {
			result = append(result, u)
		}
	}
	return result
}

func urlPattern(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	// Normalize numeric path segments
	segments := strings.Split(u.Path, "/")
	for i, seg := range segments {
		if numericSegmentRe.MatchString(seg) {
			segments[i] = numericSegmentRe.ReplaceAllString(seg, "")
		}
	}
	normalizedPath := strings.Join(segments, "/")

	// Extract sorted parameter names (ignore values)
	params := make([]string, 0, len(u.Query()))
	for k := range u.Query() {
		params = append(params, k)
	}
	sort.Strings(params)

	return u.Scheme + u.Host + normalizedPath + strings.Join(params, ",")
}
