package subdomain

import (
	"context"
	"strings"

	dnsresolver "github.com/yourname/nscan/internal/scanner/dns"
)

// dnsRecordCollector 通过 DNS 记录（MX/NS/SOA/TXT/SRV/CNAME）发现关联子域名
type dnsRecordCollector struct{ resolverConfig string }

func (c *dnsRecordCollector) Name() string { return "dns-record" }

func (c *dnsRecordCollector) Collect(ctx context.Context, domain string) ([]string, error) {
	seen := make(map[string]struct{})
	var results []string

	addIfSub := func(name string) {
		name = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
		if name == "" || name == domain {
			return
		}
		if !strings.HasSuffix(name, "."+domain) {
			return
		}
		if _, ok := seen[name]; !ok {
			seen[name] = struct{}{}
			results = append(results, name)
		}
	}

	resolver := dnsresolver.Resolver(c.resolverConfig)

	// MX 记录
	if mxs, err := resolver.LookupMX(ctx, domain); err == nil {
		for _, mx := range mxs {
			addIfSub(mx.Host)
		}
	}

	// NS 记录
	if nss, err := resolver.LookupNS(ctx, domain); err == nil {
		for _, ns := range nss {
			addIfSub(ns.Host)
		}
	}

	// TXT 记录 — 提取其中包含的域名
	if txts, err := resolver.LookupTXT(ctx, domain); err == nil {
		for _, txt := range txts {
			for _, m := range domainRe.FindAllString(txt, -1) {
				addIfSub(m)
			}
		}
	}

	// CNAME
	if cname, err := resolver.LookupCNAME(ctx, domain); err == nil {
		addIfSub(cname)
	}

	// SRV — 常见服务记录
	srvServices := []string{
		"_sip._tcp", "_sip._udp", "_xmpp-server._tcp", "_xmpp-client._tcp",
		"_ldap._tcp", "_kerberos._tcp", "_http._tcp", "_https._tcp",
		"_autodiscover._tcp", "_caldav._tcp", "_carddav._tcp",
	}
	for _, srv := range srvServices {
		if ctx.Err() != nil {
			break
		}
		_, addrs, err := resolver.LookupSRV(ctx, "", "", srv+"."+domain)
		if err != nil {
			continue
		}
		for _, a := range addrs {
			addIfSub(a.Target)
		}
	}

	return results, nil
}
