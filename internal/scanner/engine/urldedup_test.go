package engine

import "testing"

func TestURLDedup_SameParamsDifferentValues(t *testing.T) {
	d := NewURLDedup()
	if !d.IsNew("http://example.com/api?id=1") {
		t.Fatal("first URL should be new")
	}
	if d.IsNew("http://example.com/api?id=2") {
		t.Fatal("same pattern with different param value should dedup")
	}
}

func TestURLDedup_MultipleParams(t *testing.T) {
	d := NewURLDedup()
	if !d.IsNew("http://example.com/api?id=1&name=a") {
		t.Fatal("first URL should be new")
	}
	if d.IsNew("http://example.com/api?id=2&name=b") {
		t.Fatal("same param names should dedup")
	}
}

func TestURLDedup_DifferentPaths(t *testing.T) {
	d := NewURLDedup()
	if !d.IsNew("http://example.com/api?id=1") {
		t.Fatal("first URL should be new")
	}
	if !d.IsNew("http://example.com/other?id=1") {
		t.Fatal("different paths should NOT dedup")
	}
}

func TestURLDedup_NumericPathSegments(t *testing.T) {
	d := NewURLDedup()
	if !d.IsNew("http://example.com/users/123/posts") {
		t.Fatal("first URL should be new")
	}
	if d.IsNew("http://example.com/users/456/posts") {
		t.Fatal("numeric path segments should dedup")
	}
}

func TestFilterURLs(t *testing.T) {
	urls := []string{
		"http://example.com/api?id=1",
		"http://example.com/api?id=2",
		"http://example.com/api?id=3",
		"http://example.com/other?id=1",
	}
	result := FilterURLs(urls)
	if len(result) != 2 {
		t.Fatalf("expected 2 unique URLs, got %d: %v", len(result), result)
	}
}
