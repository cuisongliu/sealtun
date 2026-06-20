//go:build !darwin

package tunnel

import (
	"context"
	"net"
)

func dialSystemNCContext(_ context.Context, _, _ string, nativeErr error) (net.Conn, error) {
	return nil, nativeErr
}
