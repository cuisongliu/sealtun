package tunnel

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// DialRawTCPOverWebSocket connects stdin/stdout to a remote raw TCP stream.
func DialRawTCPOverWebSocket(ctx context.Context, wsURL, secret string) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	headers := http.Header{}
	headers.Add("Authorization", fmt.Sprintf("Bearer %s", secret))

	conn, resp, err := dialer.DialContext(ctx, wsURL, headers)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return fmt.Errorf("failed to dial tcp tunnel endpoint %s: %w", wsURL, err)
	}
	defer conn.Close()

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()

	wsConn := NewWSConn(conn)
	defer wsConn.Close()

	errc := make(chan error, 2)
	go func() {
		_, err := io.Copy(wsConn, os.Stdin)
		errc <- err
	}()
	go func() {
		_, err := io.Copy(os.Stdout, wsConn)
		errc <- err
	}()

	err = <-errc
	if err == nil || err == io.EOF || ctx.Err() != nil {
		return nil
	}
	if strings.Contains(err.Error(), "use of closed network connection") {
		return nil
	}
	return err
}
