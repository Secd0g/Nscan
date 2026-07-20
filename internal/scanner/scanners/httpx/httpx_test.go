package httpx

import (
	"context"
	"net"
	"testing"

	"github.com/yourname/nscan/internal/scanner/engine"
)

type stubResolver struct {
	calls map[string]int
}

func (r *stubResolver) LookupHost(_ context.Context, host string) ([]string, error) {
	r.calls[host]++
	if host == "missing.example" {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	return []string{"192.0.2.1"}, nil
}

func TestFilterResolvableURLsChecksEachHostOnce(t *testing.T) {
	r := &stubResolver{calls: make(map[string]int)}
	progress := make(chan *engine.Progress, 4)
	got := filterResolvableURLs(context.Background(), []string{
		"http://missing.example",
		"https://missing.example",
		"http://live.example",
		"https://live.example",
		"http://192.0.2.2",
	}, r, progress)

	if r.calls["missing.example"] != 1 || r.calls["live.example"] != 1 {
		t.Fatalf("expected one lookup per hostname, got %#v", r.calls)
	}
	if len(got) != 5 {
		t.Fatalf("expected all URLs to remain probeable, got %#v", got)
	}
	if len(progress) != 0 {
		t.Fatalf("expected no preflight skip warning, got %d", len(progress))
	}
}
