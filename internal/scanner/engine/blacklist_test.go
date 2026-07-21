package engine

import (
	"testing"

	"github.com/yourname/nscan/pkg/proto/scanv1"
)

func TestBlacklistChecker(t *testing.T) {
	rules := []*scanv1.BlacklistRule{
		{Type: "ip", Value: "1.1.1.1"},
		{Type: "domain", Value: "example.com"},
		{Type: "cidr", Value: "10.0.0.0/8"},
		{Type: "wildcard", Value: "*.gov.cn"},
	}

	checker := NewBlacklistChecker(rules)

	tests := []struct {
		target   string
		expected bool
	}{
		{"1.1.1.1", true},
		{"1.1.1.1:80", true},
		{"http://1.1.1.1", true},
		{"http://1.1.1.1:8080/path", true},
		{"2.2.2.2", false},
		{"example.com", true},
		{"sub.example.com", false}, // domain is exact match
		{"10.1.2.3", true},
		{"10.255.255.255", true},
		{"11.0.0.0", false},
		{"test.gov.cn", true},
		{"a.b.gov.cn", true},
		{"gov.cn", false}, // wildcard *.gov.cn doesn't match gov.cn directly usually, but let's check
	}

	for _, tt := range tests {
		result := checker.IsBlocked(tt.target)
		if result != tt.expected {
			t.Errorf("IsBlocked(%q) = %v; want %v", tt.target, result, tt.expected)
		}
	}
}

func TestBlacklistCheckerFiltersResults(t *testing.T) {
	checker := NewBlacklistChecker([]*scanv1.BlacklistRule{
		{Type: "domain", Value: "Blocked.Example"},
		{Type: "ip", Value: "1.1.1.1"},
		{Type: "wildcard", Value: "*.blocked.example"},
	})

	blocked := []struct {
		name string
		data string
	}{
		{"http url", `{"url":"https://blocked.example/login"}`},
		{"subdomain", `{"domain":"api.blocked.example"}`},
		{"port", `{"ip":"1.1.1.1","port":443}`},
		{"vulnerability target", `{"target":"http://1.1.1.1:8080"}`},
	}
	for _, tt := range blocked {
		t.Run(tt.name, func(t *testing.T) {
			if !checker.IsResultBlocked(&ScanResult{Data: []byte(tt.data)}) {
				t.Fatalf("expected result to be blocked: %s", tt.data)
			}
		})
	}

	if checker.IsResultBlocked(&ScanResult{Data: []byte(`{"url":"https://safe.example"}`)}) {
		t.Fatal("did not expect safe result to be blocked")
	}
}
