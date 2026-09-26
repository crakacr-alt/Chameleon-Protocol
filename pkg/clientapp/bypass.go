package clientapp

import (
	"net"
	"strings"
)

// MatchBypass matches an exact host, domain suffix or CIDR.
// A rule ".example.local" matches subdomains and the base domain.
func MatchBypass(destination string, rules []string) bool {
	host, _, err := net.SplitHostPort(destination)
	if err != nil {
		return false
	}
	host = strings.Trim(strings.ToLower(host), "[]")

	ip := net.ParseIP(host)
	for _, raw := range rules {
		rule := strings.TrimSpace(strings.ToLower(raw))
		if rule == "" {
			continue
		}
		if strings.Contains(rule, "/") {
			if ip == nil {
				continue
			}
			_, network, err := net.ParseCIDR(rule)
			if err == nil && network.Contains(ip) {
				return true
			}
			continue
		}
		if rule == "localhost" && (host == "localhost" || (ip != nil && ip.IsLoopback())) {
			return true
		}
		if strings.HasPrefix(rule, ".") {
			base := strings.TrimPrefix(rule, ".")
			if host == base || strings.HasSuffix(host, rule) {
				return true
			}
			continue
		}
		if host == rule {
			return true
		}
	}
	return false
}
