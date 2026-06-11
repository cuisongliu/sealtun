//go:build linux

package clusterconnect

import (
	"encoding/binary"
	"testing"
	"unsafe"
)

func TestSockaddrPortUsesNetworkByteOrder(t *testing.T) {
	buf := []byte{0x16, 0x2e} // 5678
	raw := *(*uint16)(unsafe.Pointer(&buf[0]))
	if got := sockaddrPort(raw); got != binary.BigEndian.Uint16(buf) {
		t.Fatalf("expected port 5678, got %d", got)
	}
}
