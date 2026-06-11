package tunnel

import (
	"io"
	"net"
)

type closeWriter interface {
	CloseWrite() error
}

func relayBidirectional(a, b net.Conn, observeBytes func(int64)) error {
	errc := make(chan error, 2)
	go copyAndCloseWrite(a, b, observeBytes, errc)
	go copyAndCloseWrite(b, a, observeBytes, errc)

	var firstErr error
	for i := 0; i < 2; i++ {
		err := <-errc
		if err != nil && !expectedRelayClose(err) && firstErr == nil {
			firstErr = err
			_ = a.Close()
			_ = b.Close()
		}
	}
	return firstErr
}

func copyAndCloseWrite(dst, src net.Conn, observeBytes func(int64), errc chan<- error) {
	n, err := io.Copy(dst, src)
	if observeBytes != nil {
		observeBytes(n)
	}
	if closeErr := closeWrite(dst); err == nil {
		err = closeErr
	}
	errc <- err
}

func closeWrite(conn net.Conn) error {
	if conn == nil {
		return nil
	}
	if closer, ok := conn.(closeWriter); ok {
		return closer.CloseWrite()
	}
	return conn.Close()
}
