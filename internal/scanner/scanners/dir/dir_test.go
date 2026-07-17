package dir

import "testing"

func TestNormalizeExtensions(t *testing.T) {
	tests := map[string]string{
		"":                    "",
		"php,asp, .html":      ".php,.asp,.html",
		" .json, ,xml, .yaml": ".json,.xml,.yaml",
	}
	for input, want := range tests {
		if got := normalizeExtensions(input); got != want {
			t.Fatalf("normalizeExtensions(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestParseOptionSet(t *testing.T) {
	options := parseOptionSet("recursive, follow_redirects,recursive")
	if !options["recursive"] || !options["follow_redirects"] {
		t.Fatalf("unexpected options: %#v", options)
	}
}
