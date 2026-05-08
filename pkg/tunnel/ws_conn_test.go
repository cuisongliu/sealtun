package tunnel

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestWSConnReadSkipsEmptyMessagesIteratively(t *testing.T) {
	t.Parallel()

	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer conn.Close()

		for i := 0; i < 32; i++ {
			if err := conn.WriteMessage(websocket.BinaryMessage, nil); err != nil {
				t.Errorf("write empty message: %v", err)
				return
			}
		}
		if err := conn.WriteMessage(websocket.BinaryMessage, []byte("ok")); err != nil {
			t.Errorf("write payload: %v", err)
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	netConn := NewWSConn(conn)
	buf := make([]byte, 2)
	n, err := netConn.Read(buf)
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}
	if got := string(buf[:n]); got != "ok" {
		t.Fatalf("expected payload ok, got %q", got)
	}
}
