package httpx

import (
	ahocorasick "github.com/petar-dambovaliev/aho-corasick"
)

// Fingerprint defines a single technology fingerprint rule.
type Fingerprint struct {
	Name     string   // Technology name (e.g. "WordPress")
	Patterns []string // Patterns to match in body/headers
}

// FingerprintMatcher uses Aho-Corasick for O(n) multi-pattern matching.
type FingerprintMatcher struct {
	matcher  ahocorasick.AhoCorasick
	patIndex []int // pattern index -> fingerprint index
	fps      []Fingerprint
}

// NewFingerprintMatcher builds an Aho-Corasick automaton from all patterns.
func NewFingerprintMatcher(fps []Fingerprint) *FingerprintMatcher {
	var allPatterns []string
	var patIndex []int

	for i, fp := range fps {
		for _, p := range fp.Patterns {
			allPatterns = append(allPatterns, p)
			patIndex = append(patIndex, i)
		}
	}

	builder := ahocorasick.NewAhoCorasickBuilder(ahocorasick.Opts{
		AsciiCaseInsensitive: false,
		MatchOnlyWholeWords:  false,
		MatchKind:            ahocorasick.StandardMatch,
		DFA:                  true,
	})
	matcher := builder.Build(allPatterns)

	return &FingerprintMatcher{
		matcher:  matcher,
		patIndex: patIndex,
		fps:      fps,
	}
}

// Match runs the automaton over text and returns deduplicated tech names.
func (m *FingerprintMatcher) Match(text string) []string {
	matches := m.matcher.FindAll(text)
	seen := make(map[int]struct{})
	var result []string

	for _, match := range matches {
		fpIdx := m.patIndex[match.Pattern()]
		if _, ok := seen[fpIdx]; !ok {
			seen[fpIdx] = struct{}{}
			result = append(result, m.fps[fpIdx].Name)
		}
	}
	return result
}

// DefaultFingerprints is the built-in fingerprint set.
var DefaultFingerprints = []Fingerprint{
	// CMS
	{Name: "WordPress", Patterns: []string{"wp-content", "wp-includes", "/wp-json/", "wp-login.php"}},
	{Name: "Joomla", Patterns: []string{"/media/jui/", "Joomla!", "/administrator/index.php", "com_content"}},
	{Name: "Drupal", Patterns: []string{"Drupal.settings", "/sites/default/files/", "drupal.js", "/misc/drupal.js"}},
	{Name: "Magento", Patterns: []string{"Mage.Cookies", "/skin/frontend/", "magento", "/mage/"}},
	{Name: "Shopify", Patterns: []string{"cdn.shopify.com", "Shopify.theme", "shopify-section"}},
	{Name: "Ghost", Patterns: []string{"ghost-url", "ghost/api/", "ghost.io"}},

	// Frameworks
	{Name: "Laravel", Patterns: []string{"laravel_session", "XSRF-TOKEN", "laravel"}},
	{Name: "Django", Patterns: []string{"csrfmiddlewaretoken", "django", "__admin_media_prefix__"}},
	{Name: "Flask", Patterns: []string{"Werkzeug", "flask"}},
	{Name: "Spring", Patterns: []string{"org.springframework", "spring-boot", "Whitelabel Error Page"}},
	{Name: "Express", Patterns: []string{"X-Powered-By: Express", "express-session"}},
	{Name: "Rails", Patterns: []string{"csrf-token", "action_dispatch", "ruby on rails", "Rails.application"}},
	{Name: "Next.js", Patterns: []string{"__next", "_next/static", "__NEXT_DATA__"}},
	{Name: "Nuxt.js", Patterns: []string{"__nuxt", "_nuxt/", "__NUXT__"}},

	// JS Libraries
	{Name: "React", Patterns: []string{"react.development", "react.production", "_reactRootContainer", "data-reactroot"}},
	{Name: "Vue.js", Patterns: []string{"__vue__", "vue.runtime", "Vue.component", "data-v-"}},
	{Name: "Angular", Patterns: []string{"ng-version", "ng-app", "angular.js", "angular.min.js"}},
	{Name: "jQuery", Patterns: []string{"jquery.min.js", "jquery.js", "jQuery v"}},
	{Name: "Bootstrap", Patterns: []string{"bootstrap.min.css", "bootstrap.min.js", "bootstrap.css"}},

	// Servers
	{Name: "Nginx", Patterns: []string{"nginx"}},
	{Name: "Apache", Patterns: []string{"Apache"}},
	{Name: "IIS", Patterns: []string{"Microsoft-IIS", "IIS"}},
	{Name: "Tomcat", Patterns: []string{"Apache Tomcat", "Coyote", "tomcat"}},
	{Name: "Caddy", Patterns: []string{"Caddy", "caddy"}},
	{Name: "LiteSpeed", Patterns: []string{"LiteSpeed", "litespeed"}},

	// Languages
	{Name: "PHP", Patterns: []string{"X-Powered-By: PHP", ".php", "PHPSESSID"}},
	{Name: "ASP.NET", Patterns: []string{"ASP.NET", "__VIEWSTATE", "__EVENTVALIDATION", "aspnet_"}},
	{Name: "Node.js", Patterns: []string{"node.js", "nodejs"}},
	{Name: "Java", Patterns: []string{"JSESSIONID", "java.lang", "javax.faces"}},

	// Other
	{Name: "Cloudflare", Patterns: []string{"cloudflare", "cf-ray", "__cfduid"}},
	{Name: "Varnish", Patterns: []string{"Varnish", "X-Varnish", "varnish"}},
	{Name: "Redis", Patterns: []string{"redis"}},
	{Name: "Elasticsearch", Patterns: []string{"elasticsearch", "X-elastic-product"}},
}
