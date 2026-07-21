package engine

import (
	"encoding/json"
	"net"
	"path"
	"strings"

	"github.com/yourname/nscan/pkg/proto/scanv1"
)

type BlacklistChecker struct {
	ips       map[string]bool
	domains   map[string]bool
	cidrs     []*net.IPNet
	wildcards []string
}

func NewBlacklistChecker(rules []*scanv1.BlacklistRule) *BlacklistChecker {
	c := &BlacklistChecker{
		ips:     make(map[string]bool),
		domains: make(map[string]bool),
	}
	for _, r := range rules {
		val := strings.ToLower(strings.TrimSpace(r.Value))
		switch r.Type {
		case "ip":
			c.ips[val] = true
		case "domain":
			c.domains[val] = true
		case "cidr":
			_, ipnet, err := net.ParseCIDR(val)
			if err == nil {
				c.cidrs = append(c.cidrs, ipnet)
			}
		case "wildcard":
			c.wildcards = append(c.wildcards, val)
		}
	}
	return c
}

func (c *BlacklistChecker) IsBlocked(target string) bool {
	host := strings.ToLower(strings.TrimSpace(target))
	// Remove scheme if present (e.g., http://)
	if idx := strings.Index(host, "://"); idx != -1 {
		host = host[idx+3:]
	}
	// Remove path if present
	if idx := strings.Index(host, "/"); idx != -1 {
		host = host[:idx]
	}
	// Remove port if present
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	if c.ips[host] {
		return true
	}
	if c.domains[host] {
		return true
	}

	ip := net.ParseIP(host)
	if ip != nil {
		for _, cidr := range c.cidrs {
			if cidr.Contains(ip) {
				return true
			}
		}
	}

	for _, w := range c.wildcards {
		matched, _ := path.Match(w, host)
		if matched {
			return true
		}
		// Fallback suffix match for *.example.com
		if strings.HasPrefix(w, "*.") && strings.HasSuffix(host, w[1:]) {
			return true
		}
	}

	return false
}

// IsResultBlocked applies the same blacklist rules to the asset payloads
// emitted by every scanner. Scanner implementations do not all return the
// same asset shape, so inspect the common target-bearing fields here rather
// than requiring every tool to duplicate this policy.
func (c *BlacklistChecker) IsResultBlocked(result *ScanResult) bool {
	if result == nil || len(result.Data) == 0 {
		return false
	}
	var fields map[string]any
	if err := json.Unmarshal(result.Data, &fields); err != nil {
		return false
	}
	for _, key := range []string{"url", "domain", "ip", "host", "target", "matched_at"} {
		if value, ok := fields[key].(string); ok && value != "" && c.IsBlocked(value) {
			return true
		}
	}
	return false
}

func (c *BlacklistChecker) FilterInput(input *StageInput, skippedCallback func(string)) *StageInput {
	if input == nil {
		return nil
	}
	out := &StageInput{}

	filterList := func(list []string, outList *[]string) {
		for _, item := range list {
			if !c.IsBlocked(item) {
				*outList = append(*outList, item)
			} else if skippedCallback != nil {
				skippedCallback(item)
			}
		}
	}

	filterList(input.Targets, &out.Targets)
	filterList(input.Subdomains, &out.Subdomains)
	filterList(input.Hosts, &out.Hosts)
	filterList(input.HTTPURLs, &out.HTTPURLs)
	for _, page := range input.CrawledPages {
		if !c.IsBlocked(page.URL) {
			out.CrawledPages = append(out.CrawledPages, page)
		} else if skippedCallback != nil {
			skippedCallback(page.URL)
		}
	}

	if input.HTTPTechMap != nil {
		out.HTTPTechMap = make(map[string][]string, len(input.HTTPTechMap))
		for u, tech := range input.HTTPTechMap {
			if !c.IsBlocked(u) {
				out.HTTPTechMap[u] = tech
			}
		}
	}

	return out
}
