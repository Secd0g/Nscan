package engine

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/yourname/nscan/pkg/models"
)

// DefaultPassiveRules returns the built-in passive detection rules.
func DefaultPassiveRules() []PassiveRule {
	return []PassiveRule{
		ruleEmailAddresses(),
		ruleInternalIPs(),
		ruleAPIKeys(),
		ruleErrorMessages(),
		ruleVersionDisclosure(),
		ruleSensitivePaths(),
	}
}

// helper: unmarshal HTTPAsset and return concatenated searchable text from
// available fields (Title, Banner, URL, Tech).
func extractSearchableText(data []byte) (models.HTTPAsset, string, bool) {
	var ha models.HTTPAsset
	if err := json.Unmarshal(data, &ha); err != nil {
		return ha, "", false
	}
	var sb strings.Builder
	sb.WriteString(ha.Title)
	sb.WriteByte('\n')
	sb.WriteString(ha.Banner)
	sb.WriteByte('\n')
	sb.WriteString(ha.URL)
	sb.WriteByte('\n')
	for _, t := range ha.Tech {
		sb.WriteString(t)
		sb.WriteByte(' ')
	}
	return ha, sb.String(), true
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ── Rules ────────────────────────────────────────────────────────────────────

func ruleEmailAddresses() PassiveRule {
	re := regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	return PassiveRule{
		ID: "passive-email", Name: "Email Address Disclosure", Severity: "info",
		Check: func(data []byte) []PassiveFinding {
			_, text, ok := extractSearchableText(data)
			if !ok {
				return nil
			}
			matches := re.FindAllString(text, 5)
			if len(matches) == 0 {
				return nil
			}
			seen := make(map[string]struct{})
			var findings []PassiveFinding
			for _, m := range matches {
				if _, dup := seen[m]; dup {
					continue
				}
				seen[m] = struct{}{}
				findings = append(findings, PassiveFinding{
					RuleName: "Email Address Disclosure",
					Severity: "info",
					Detail:   "Email address found in HTTP response metadata",
					Match:    m,
				})
			}
			return findings
		},
	}
}

func ruleInternalIPs() PassiveRule {
	re := regexp.MustCompile(`(?:10\.\d{1,3}\.\d{1,3}\.\d{1,3}|172\.(?:1[6-9]|2\d|3[01])\.\d{1,3}\.\d{1,3}|192\.168\.\d{1,3}\.\d{1,3})`)
	return PassiveRule{
		ID: "passive-internal-ip", Name: "Internal IP Disclosure", Severity: "low",
		Check: func(data []byte) []PassiveFinding {
			_, text, ok := extractSearchableText(data)
			if !ok {
				return nil
			}
			matches := re.FindAllString(text, 5)
			if len(matches) == 0 {
				return nil
			}
			seen := make(map[string]struct{})
			var findings []PassiveFinding
			for _, m := range matches {
				if _, dup := seen[m]; dup {
					continue
				}
				seen[m] = struct{}{}
				findings = append(findings, PassiveFinding{
					RuleName: "Internal IP Disclosure",
					Severity: "low",
					Detail:   "Private/internal IP address found in HTTP response metadata",
					Match:    m,
				})
			}
			return findings
		},
	}
}

func ruleAPIKeys() PassiveRule {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`AKIA[0-9A-Z]{16}`),                                                                   // AWS Access Key
		regexp.MustCompile(`ghp_[a-zA-Z0-9]{36}`),                                                                // GitHub token
		regexp.MustCompile(`eyJ[a-zA-Z0-9_-]{10,}\.[a-zA-Z0-9_-]{10,}\.[a-zA-Z0-9_-]{10,}`),                     // JWT
		regexp.MustCompile(`(?i)(?:api[_-]?key|apikey|api[_-]?secret)\s*[:=]\s*["']?([a-zA-Z0-9_\-]{16,})["']?`), // Generic API key
	}
	names := []string{"AWS Access Key", "GitHub Token", "JWT Token", "Generic API Key"}
	return PassiveRule{
		ID: "passive-api-keys", Name: "API Key / Token Exposure", Severity: "high",
		Check: func(data []byte) []PassiveFinding {
			_, text, ok := extractSearchableText(data)
			if !ok {
				return nil
			}
			var findings []PassiveFinding
			for i, re := range patterns {
				if m := re.FindString(text); m != "" {
					findings = append(findings, PassiveFinding{
						RuleName: "API Key / Token Exposure",
						Severity: "high",
						Detail:   names[i] + " detected in HTTP response metadata",
						Match:    truncate(m, 64),
					})
				}
			}
			return findings
		},
	}
}

func ruleErrorMessages() PassiveRule {
	patterns := []string{
		"SQL syntax", "mysql_fetch", "ORA-", "PG::Error", "SQLSTATE",
		"stack trace", "Traceback (most recent call last)",
		"Exception in thread", "at java.", "panic:",
		"XDEBUG", "var_dump(", "print_r(",
	}
	return PassiveRule{
		ID: "passive-error-msg", Name: "Error Message / Debug Info", Severity: "medium",
		Check: func(data []byte) []PassiveFinding {
			_, text, ok := extractSearchableText(data)
			if !ok {
				return nil
			}
			lower := strings.ToLower(text)
			var findings []PassiveFinding
			for _, p := range patterns {
				if strings.Contains(lower, strings.ToLower(p)) {
					findings = append(findings, PassiveFinding{
						RuleName: "Error Message / Debug Info",
						Severity: "medium",
						Detail:   "Potential error/debug information disclosure",
						Match:    p,
					})
					break // one finding per result is enough
				}
			}
			return findings
		},
	}
}

func ruleVersionDisclosure() PassiveRule {
	re := regexp.MustCompile(`(?i)(?:Apache|nginx|IIS|Tomcat|Jetty|Express|PHP|ASP\.NET|OpenSSL|X-Powered-By)[/: ]+[\d]+\.[\d]+[.\d]*`)
	return PassiveRule{
		ID: "passive-version", Name: "Server Version Disclosure", Severity: "info",
		Check: func(data []byte) []PassiveFinding {
			_, text, ok := extractSearchableText(data)
			if !ok {
				return nil
			}
			matches := re.FindAllString(text, 3)
			if len(matches) == 0 {
				return nil
			}
			seen := make(map[string]struct{})
			var findings []PassiveFinding
			for _, m := range matches {
				if _, dup := seen[m]; dup {
					continue
				}
				seen[m] = struct{}{}
				findings = append(findings, PassiveFinding{
					RuleName: "Server Version Disclosure",
					Severity: "info",
					Detail:   "Server software version disclosed in response headers/banner",
					Match:    m,
				})
			}
			return findings
		},
	}
}

func ruleSensitivePaths() PassiveRule {
	sensitives := []string{
		"/admin", "/debug", "/phpinfo", "/.env", "/.git",
		"/wp-admin", "/actuator", "/swagger", "/.svn",
		"/server-status", "/elmah.axd", "/trace.axd",
	}
	return PassiveRule{
		ID: "passive-sensitive-path", Name: "Sensitive Path Reference", Severity: "low",
		Check: func(data []byte) []PassiveFinding {
			_, text, ok := extractSearchableText(data)
			if !ok {
				return nil
			}
			lower := strings.ToLower(text)
			var findings []PassiveFinding
			for _, p := range sensitives {
				if strings.Contains(lower, p) {
					findings = append(findings, PassiveFinding{
						RuleName: "Sensitive Path Reference",
						Severity: "low",
						Detail:   "Reference to sensitive path found in response metadata",
						Match:    p,
					})
				}
			}
			return findings
		},
	}
}
