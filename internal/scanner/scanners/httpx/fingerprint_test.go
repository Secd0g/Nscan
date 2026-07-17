package httpx

import (
	"sort"
	"testing"
)

func TestFingerprintMatcher_WordPress(t *testing.T) {
	m := NewFingerprintMatcher(DefaultFingerprints)
	body := `<link rel="stylesheet" href="/wp-content/themes/flavor/style.css">`
	got := m.Match(body)
	if len(got) != 1 || got[0] != "WordPress" {
		t.Errorf("expected [WordPress], got %v", got)
	}
}

func TestFingerprintMatcher_MultipleMatches(t *testing.T) {
	m := NewFingerprintMatcher(DefaultFingerprints)
	body := `<html>
		<script src="/wp-content/plugins/foo.js"></script>
		<script src="jquery.min.js"></script>
		<div id="__next">hello</div>
		<link rel="stylesheet" href="bootstrap.min.css">
	</html>`
	got := m.Match(body)
	sort.Strings(got)
	expected := []string{"Bootstrap", "Next.js", "WordPress", "jQuery"}
	sort.Strings(expected)

	if len(got) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, got)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("mismatch at %d: expected %s, got %s", i, expected[i], got[i])
		}
	}
}

func TestFingerprintMatcher_NoFalsePositives(t *testing.T) {
	m := NewFingerprintMatcher(DefaultFingerprints)

	// Empty body
	got := m.Match("")
	if len(got) != 0 {
		t.Errorf("expected no matches on empty body, got %v", got)
	}

	// Unrelated content
	got = m.Match("Hello world, this is a plain website with no frameworks.")
	if len(got) != 0 {
		t.Errorf("expected no matches on unrelated body, got %v", got)
	}
}

func TestFingerprintMatcher_Deduplication(t *testing.T) {
	m := NewFingerprintMatcher(DefaultFingerprints)
	// Body contains multiple WordPress patterns - should only appear once
	body := `<link href="/wp-content/style.css"><script src="/wp-includes/js/foo.js"></script>`
	got := m.Match(body)
	if len(got) != 1 || got[0] != "WordPress" {
		t.Errorf("expected [WordPress] (deduplicated), got %v", got)
	}
}
