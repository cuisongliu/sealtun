package tunnel

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type Target struct {
	URL     string
	Address string
	Port    string
}

func LocalhostTarget(localPort string) (Target, error) {
	if strings.TrimSpace(localPort) == "" {
		return Target{}, fmt.Errorf("local port is required")
	}
	targetURL := (&url.URL{Scheme: "http", Host: net.JoinHostPort("localhost", localPort)}).String()
	return ParseTarget(targetURL)
}

func ParseTarget(raw string) (Target, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Target{}, fmt.Errorf("target is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return Target{}, fmt.Errorf("invalid target %q: %w", raw, err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return Target{}, fmt.Errorf("target scheme must be http or https")
	}
	if parsed.Host == "" || parsed.Hostname() == "" {
		return Target{}, fmt.Errorf("target host is required")
	}
	if parsed.User != nil {
		return Target{}, fmt.Errorf("target must not include userinfo")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return Target{}, fmt.Errorf("target must not include query or fragment")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return Target{}, fmt.Errorf("target path is not supported")
	}
	port := parsed.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	value, err := strconv.Atoi(port)
	if err != nil || value < 1 || value > 65535 {
		return Target{}, fmt.Errorf("target port must be between 1 and 65535")
	}
	parsed.Scheme = scheme
	parsed.Host = net.JoinHostPort(parsed.Hostname(), port)
	parsed.Path = ""
	return Target{
		URL:     parsed.String(),
		Address: parsed.Host,
		Port:    port,
	}, nil
}

func TargetFor(localPort, targetURL string) (Target, error) {
	if strings.TrimSpace(targetURL) != "" {
		return ParseTarget(targetURL)
	}
	return LocalhostTarget(localPort)
}
