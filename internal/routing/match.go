package routing

import (
	"path"
	"strings"
)

func matchPatterns(m Match) []string {
	if len(m.Hostnames) > 0 {
		return m.Hostnames
	}
	if m.Hostname != "" {
		return []string{m.Hostname}
	}
	return nil
}

func canonicalHost(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, ".")
	s = strings.ToLower(s)
	return s
}

func hostnameMatches(pattern string, hostname string) bool {
	pattern = canonicalHost(pattern)
	if pattern == "" || hostname == "" {
		return false
	}
	if pattern == hostname {
		return true
	}
	ok, err := path.Match(pattern, hostname)
	if err != nil {
		return false
	}
	return ok
}
