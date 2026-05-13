package accesspolicy

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const TemporaryTokenQueryParam = "_sealtun_token"
const minTokenLength = 8

var ipMatcherCache sync.Map

type Policy struct {
	BearerTokenHashes []string         `json:"bearerTokenHashes,omitempty"`
	IPAllowlist       []string         `json:"ipAllowlist,omitempty"`
	IPDenylist        []string         `json:"ipDenylist,omitempty"`
	TemporaryTokens   []TemporaryToken `json:"temporaryTokens,omitempty"`
}

type TemporaryToken struct {
	Name      string `json:"name,omitempty"`
	TokenHash string `json:"tokenHash"`
	TTL       string `json:"ttl,omitempty"`
	ExpiresAt string `json:"expiresAt"`
}

func Empty(policy *Policy) bool {
	if policy == nil {
		return true
	}
	return len(policy.BearerTokenHashes) == 0 &&
		len(policy.IPAllowlist) == 0 &&
		len(policy.IPDenylist) == 0 &&
		len(policy.TemporaryTokens) == 0
}

func HasTokenAuth(policy *Policy) bool {
	return policy != nil && (len(policy.BearerTokenHashes) > 0 || len(policy.TemporaryTokens) > 0)
}

func HashToken(token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("token is required")
	}
	if len(token) < minTokenLength {
		return "", fmt.Errorf("token must be at least %d characters", minTokenLength)
	}
	sum := sha256.Sum256([]byte(token))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func Validate(policy *Policy) error {
	if policy == nil {
		return nil
	}
	for _, hash := range policy.BearerTokenHashes {
		if err := validateTokenHash(hash); err != nil {
			return fmt.Errorf("bearer token hash: %w", err)
		}
	}
	for _, entry := range policy.IPAllowlist {
		if _, err := parseIPMatcher(entry); err != nil {
			return fmt.Errorf("ip allowlist entry %q: %w", entry, err)
		}
	}
	for _, entry := range policy.IPDenylist {
		if _, err := parseIPMatcher(entry); err != nil {
			return fmt.Errorf("ip denylist entry %q: %w", entry, err)
		}
	}
	for _, token := range policy.TemporaryTokens {
		if err := validateTokenHash(token.TokenHash); err != nil {
			return fmt.Errorf("temporary token: %w", err)
		}
		expiresAt, err := time.Parse(time.RFC3339, token.ExpiresAt)
		if err != nil {
			return fmt.Errorf("temporary token expiresAt: %w", err)
		}
		if expiresAt.IsZero() {
			return fmt.Errorf("temporary token expiresAt is required")
		}
	}
	return nil
}

func NetworkAllowed(policy *Policy, r *http.Request) (bool, string) {
	if policy == nil || (len(policy.IPAllowlist) == 0 && len(policy.IPDenylist) == 0) {
		return true, ""
	}
	ip := ClientIP(r)
	if ip == nil {
		return false, "client IP could not be determined"
	}
	for _, entry := range policy.IPDenylist {
		matcher, err := cachedIPMatcher(entry)
		if err == nil && matcher.Contains(ip) {
			return false, "client IP is denied"
		}
	}
	if len(policy.IPAllowlist) == 0 {
		return true, ""
	}
	for _, entry := range policy.IPAllowlist {
		matcher, err := cachedIPMatcher(entry)
		if err == nil && matcher.Contains(ip) {
			return true, ""
		}
	}
	return false, "client IP is not allowed"
}

func TokenAuthorized(policy *Policy, r *http.Request, now time.Time) bool {
	if policy == nil {
		return false
	}
	if token := bearerTokenFromRequest(r); token != "" && tokenMatchesAny(token, policy.BearerTokenHashes) {
		return true
	}
	queryToken := ""
	if r != nil && r.URL != nil {
		queryToken = r.URL.Query().Get(TemporaryTokenQueryParam)
	}
	if queryToken == "" {
		return false
	}
	for _, token := range policy.TemporaryTokens {
		expiresAt, err := time.Parse(time.RFC3339, token.ExpiresAt)
		if err != nil || !now.Before(expiresAt) {
			continue
		}
		if tokenMatches(queryToken, token.TokenHash) {
			return true
		}
	}
	return false
}

func StripTemporaryTokenQuery(rawURL *url.URL) {
	if rawURL == nil || rawURL.RawQuery == "" {
		return
	}
	values := rawURL.Query()
	if _, ok := values[TemporaryTokenQueryParam]; !ok {
		return
	}
	values.Del(TemporaryTokenQueryParam)
	rawURL.RawQuery = values.Encode()
}

func ClientIP(r *http.Request) net.IP {
	if r == nil {
		return nil
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		if ip := net.ParseIP(realIP); ip != nil {
			return ip
		}
	}
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			part := parts[i]
			if ip := net.ParseIP(strings.TrimSpace(part)); ip != nil {
				return ip
			}
		}
	}
	host := r.RemoteAddr
	if parsedHost, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		host = parsedHost
	}
	return net.ParseIP(strings.TrimSpace(host))
}

type ipMatcher struct {
	ipNet *net.IPNet
	ip    net.IP
}

func (m ipMatcher) Contains(ip net.IP) bool {
	if m.ipNet != nil {
		return m.ipNet.Contains(ip)
	}
	return m.ip != nil && ip != nil && m.ip.Equal(ip)
}

func parseIPMatcher(value string) (ipMatcher, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return ipMatcher{}, fmt.Errorf("entry is empty")
	}
	if strings.Contains(value, "/") {
		_, ipNet, err := net.ParseCIDR(value)
		if err != nil {
			return ipMatcher{}, err
		}
		return ipMatcher{ipNet: ipNet}, nil
	}
	ip := net.ParseIP(value)
	if ip == nil {
		return ipMatcher{}, fmt.Errorf("entry must be an IP address or CIDR range")
	}
	return ipMatcher{ip: ip}, nil
}

func cachedIPMatcher(value string) (ipMatcher, error) {
	value = strings.TrimSpace(value)
	if cached, ok := ipMatcherCache.Load(value); ok {
		return cached.(ipMatcher), nil
	}
	matcher, err := parseIPMatcher(value)
	if err != nil {
		return ipMatcher{}, err
	}
	ipMatcherCache.Store(value, matcher)
	return matcher, nil
}

func validateTokenHash(hash string) error {
	hash = strings.TrimSpace(hash)
	if !strings.HasPrefix(hash, "sha256:") {
		return fmt.Errorf("must use sha256:<hex>")
	}
	value := strings.TrimPrefix(hash, "sha256:")
	if len(value) != sha256.Size*2 {
		return fmt.Errorf("invalid sha256 length")
	}
	if _, err := hex.DecodeString(value); err != nil {
		return fmt.Errorf("invalid sha256 hex")
	}
	return nil
}

func bearerTokenFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	const prefix = "Bearer "
	if len(authHeader) <= len(prefix) || !strings.EqualFold(authHeader[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(authHeader[len(prefix):])
}

func tokenMatchesAny(token string, hashes []string) bool {
	for _, hash := range hashes {
		if tokenMatches(token, hash) {
			return true
		}
	}
	return false
}

func tokenMatches(token, hash string) bool {
	tokenHash, err := HashToken(token)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(tokenHash), []byte(strings.TrimSpace(hash))) == 1
}
