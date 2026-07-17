package subdomain

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// crtshCollector 通过 crt.sh 证书透明日志查询子域名
type crtshCollector struct{}

func (c *crtshCollector) Name() string { return "crt.sh" }

func (c *crtshCollector) Collect(ctx context.Context, domain string) ([]string, error) {
	url := fmt.Sprintf("https://crt.sh/?q=%%25.%s&output=json", domain)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "nscan/0.1")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("crt.sh request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("crt.sh returned %d", resp.StatusCode)
	}

	var entries []struct {
		NameValue string `json:"name_value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("crt.sh decode: %w", err)
	}

	seen := make(map[string]struct{})
	var results []string
	for _, e := range entries {
		for _, name := range strings.Split(e.NameValue, "\n") {
			name = strings.TrimSpace(strings.ToLower(name))
			// 跳过通配符和非目标域名
			if name == "" || strings.HasPrefix(name, "*.") {
				name = strings.TrimPrefix(name, "*.")
			}
			if name == "" || !strings.HasSuffix(name, domain) {
				continue
			}
			if _, ok := seen[name]; !ok {
				seen[name] = struct{}{}
				results = append(results, name)
			}
		}
	}
	return results, nil
}
