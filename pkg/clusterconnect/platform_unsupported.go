//go:build !linux

package clusterconnect

import (
	"fmt"
	"net"
)

func platformTransparentSupported() (bool, string) {
	return false, "transparent TCP connect is currently Linux-only"
}

type OriginalDestination struct {
	IP   net.IP
	Port int
}

func requireTransparentPrivileges() error {
	return fmt.Errorf("transparent TCP connect is currently Linux-only")
}

func applyTransparentPlan(plan *TransparentPlan) error {
	return fmt.Errorf("transparent TCP connect is currently Linux-only")
}

func cleanupTransparentPlan(plan *TransparentPlan) error {
	return nil
}

func originalDestination(conn net.Conn) (*OriginalDestination, error) {
	return nil, fmt.Errorf("transparent TCP connect is currently Linux-only")
}
