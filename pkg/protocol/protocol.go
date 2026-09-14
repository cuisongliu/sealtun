package protocol

import (
	"fmt"
	"strings"
)

const (
	HTTPS = "https"
	SSH   = "ssh"
	TCP   = "tcp"
)

const (
	ServerHTTPPort   int32 = 8080
	ServerRawTCPPort int32 = 2222
)

func Normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func ValidateExpose(value string) error {
	value = Normalize(value)
	switch value {
	case HTTPS, SSH, TCP, "http":
		return nil
	case "":
		return fmt.Errorf("protocol is required")
	default:
		return fmt.Errorf("unsupported protocol %q: supported protocols are https and tcp (ssh and http are accepted aliases)", value)
	}
}

// ResolveAlias maps user-facing aliases onto the canonical protocol set
// (https, tcp) and reports the alias, so callers can surface a compatibility
// note. SSH tunnels are plain TCP with SSH-flavored output; http is always
// TLS on the public side and is an alias for https.
func ResolveAlias(value string) (canonical, alias string) {
	switch Normalize(value) {
	case SSH:
		return TCP, SSH
	case "http":
		return HTTPS, "http"
	default:
		return Normalize(value), ""
	}
}

func ValidateServer(value string) error {
	value = Normalize(value)
	switch value {
	case HTTPS, SSH, TCP:
		return nil
	case "":
		return fmt.Errorf("protocol is required")
	default:
		return fmt.Errorf("unsupported server protocol %q: supported protocols are https, ssh, and tcp", value)
	}
}

func UsesRawTCP(value string) bool {
	switch Normalize(value) {
	case SSH, TCP:
		return true
	default:
		return false
	}
}

func IsHTTP(value string) bool {
	return Normalize(value) == HTTPS
}
