package api

import "testing"

func TestSplitHostPort(t *testing.T) {
	tests := []struct {
		requestHost string
		grpcAddr    string
		wantHost    string
		wantPort    string
	}{
		{"scan.example.com:8080", ":9000", "scan.example.com", "9000"},
		{"scan.example.com", "0.0.0.0:9443", "scan.example.com", "9443"},
		{"[2001:db8::1]:8080", "[::]:9000", "2001:db8::1", "9000"},
		{"[2001:db8::1]", "invalid", "2001:db8::1", "9000"},
	}
	for _, tt := range tests {
		host, port := splitHostPort(tt.requestHost, tt.grpcAddr)
		if host != tt.wantHost || port != tt.wantPort {
			t.Fatalf("splitHostPort(%q, %q) = (%q, %q), want (%q, %q)", tt.requestHost, tt.grpcAddr, host, port, tt.wantHost, tt.wantPort)
		}
	}
}

func TestShellQuote(t *testing.T) {
	if got, want := shellQuote("key'part"), `'key'"'"'part'`; got != want {
		t.Fatalf("shellQuote() = %q, want %q", got, want)
	}
}
