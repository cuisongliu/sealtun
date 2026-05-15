package protocol

import (
	"fmt"
	"strings"
)

const (
	HTTPS = "https"
	SSH   = "ssh"
)

func Normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func ValidateExpose(value string) error {
	value = Normalize(value)
	switch value {
	case HTTPS, SSH:
		return nil
	case "":
		return fmt.Errorf("protocol is required")
	default:
		return fmt.Errorf("unsupported protocol %q: supported protocols are https and ssh", value)
	}
}

func ValidateServer(value string) error {
	value = Normalize(value)
	switch value {
	case HTTPS, SSH:
		return nil
	case "":
		return fmt.Errorf("protocol is required")
	default:
		return fmt.Errorf("unsupported server protocol %q: supported protocols are https and ssh", value)
	}
}
