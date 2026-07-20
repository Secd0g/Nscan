package dns

import (
	"context"
	"net"
	"strings"
	"time"
)

const DefaultResolver = "8.8.8.8"

// Resolver creates a resolver for the first configured nameserver. An empty
// value intentionally falls back to Google's public resolver.
func Resolver(configured string) *net.Resolver {
	address := DefaultResolver
	for _, value := range strings.FieldsFunc(configured, func(r rune) bool { return r == ',' || r == '\n' || r == ' ' || r == '\t' }) {
		if strings.TrimSpace(value) != "" {
			address = strings.TrimSpace(value)
			break
		}
	}
	return &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		dialer := net.Dialer{Timeout: 3 * time.Second}
		return dialer.DialContext(ctx, "udp", net.JoinHostPort(address, "53"))
	}}
}

func LookupHost(ctx context.Context, configured, host string) ([]string, error) {
	return Resolver(configured).LookupHost(ctx, host)
}
