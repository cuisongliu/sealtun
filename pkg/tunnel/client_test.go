package tunnel

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tunnelprotocol "github.com/labring/sealtun/pkg/protocol"
)

func TestUnavailableResponse(t *testing.T) {
	t.Parallel()

	response := unavailableResponse("http://localhost:3000")

	if !strings.HasPrefix(response, "HTTP/1.1 502 Bad Gateway\r\n") {
		t.Fatalf("unexpected status line: %q", response)
	}
	if !strings.Contains(response, "Content-Type: text/html; charset=utf-8\r\n") {
		t.Fatal("missing content type header")
	}
	if !strings.Contains(response, "Sealtun Tunnel Status") {
		t.Fatal("missing sealtun status shell")
	}
	if !strings.Contains(response, "localhost:3000") {
		t.Fatal("response should show expected local target")
	}
	if !strings.Contains(response, "Refresh this page after the target is ready.") {
		t.Fatal("response should explain recovery step")
	}
	if !strings.Contains(response, "http://localhost:3000") {
		t.Fatal("response should mention the target")
	}
}

func TestRawTCPLocalForwardingDoesNotWriteHTTPFallback(t *testing.T) {
	t.Parallel()

	server, client := net.Pipe()
	done := make(chan struct{})
	go func() {
		handleLocalForwarding(server, "1", tunnelprotocol.TCP)
		close(done)
	}()

	buffer := make([]byte, 1)
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	n, err := client.Read(buffer)
	_ = client.Close()
	<-done

	if n != 0 {
		t.Fatalf("raw TCP fallback wrote %d bytes", n)
	}
	if err == nil {
		t.Fatal("expected raw TCP fallback to close without HTTP response bytes")
	}
}

func TestRawTCPLocalForwardingKeepsResponseAfterClientHalfClose(t *testing.T) {
	t.Parallel()

	localListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer localListener.Close()

	localDone := make(chan struct{})
	go func() {
		defer close(localDone)
		conn, err := localListener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = conn.Write([]byte("ready\n"))
		data, _ := io.ReadAll(conn)
		if len(data) > 0 {
			_, _ = conn.Write(append([]byte("echo:"), data...))
		}
	}()

	_, port, err := net.SplitHostPort(localListener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	streamListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer streamListener.Close()

	acceptc := make(chan net.Conn, 1)
	go func() {
		conn, err := streamListener.Accept()
		if err == nil {
			acceptc <- conn
		}
	}()
	client, err := net.Dial("tcp", streamListener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server := <-acceptc

	forwardDone := make(chan struct{})
	go func() {
		defer close(forwardDone)
		handleLocalForwarding(server, port, tunnelprotocol.TCP)
	}()

	if _, err := client.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	if tcp, ok := client.(*net.TCPConn); ok {
		if err := tcp.CloseWrite(); err != nil {
			t.Fatal(err)
		}
	} else {
		t.Fatal("expected tcp client connection")
	}

	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	payload, err := io.ReadAll(client)
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	if !strings.Contains(text, "ready\n") || !strings.Contains(text, "echo:ping") {
		t.Fatalf("expected ready banner and echo response, got %q", text)
	}

	<-forwardDone
	<-localDone
}

func TestHTTPLocalForwardingKeepsRawRelayForImplicitTargets(t *testing.T) {
	t.Parallel()

	localListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer localListener.Close()

	upstreamDone := make(chan struct{})
	go func() {
		defer close(upstreamDone)
		conn, err := localListener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		var request strings.Builder
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			request.WriteString(line)
			if line == "\r\n" {
				break
			}
		}
		if strings.Contains(request.String(), "Host: public.example") {
			_, _ = conn.Write([]byte("HTTP/1.1 204 No Content\r\nConnection: close\r\n\r\n"))
		}
	}()

	_, port, err := net.SplitHostPort(localListener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	target, err := TargetFor(port, "")
	if err != nil {
		t.Fatal(err)
	}
	if target.Explicit {
		t.Fatal("expected implicit local target")
	}
	server, client := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		handleTargetForwarding(server, target, tunnelprotocol.HTTPS)
	}()

	request := "GET /local HTTP/1.1\r\n" +
		"Host: public.example\r\n" +
		"Connection: close\r\n\r\n"
	if _, err := client.Write([]byte(request)); err != nil {
		t.Fatal(err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	response, err := io.ReadAll(client)
	if err != nil {
		t.Fatal(err)
	}
	_ = client.Close()
	<-done
	<-upstreamDone

	if !strings.Contains(string(response), "HTTP/1.1 204 No Content") {
		t.Fatalf("expected raw upstream response, got %q", string(response))
	}
}

func TestHTTPTargetForwardingUsesReverseProxyForExplicitTargets(t *testing.T) {
	t.Parallel()

	type observedRequest struct {
		method string
		path   string
		query  string
		host   string
		body   string
	}
	observed := make(chan observedRequest, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		observed <- observedRequest{
			method: r.Method,
			path:   r.URL.Path,
			query:  r.URL.RawQuery,
			host:   r.Host,
			body:   string(body),
		}
		w.Header().Set("X-Upstream", "ok")
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(w, "proxied:%s:%s", r.Method, body)
	}))
	defer upstream.Close()

	target, err := TargetFor("", upstream.URL+"/base")
	if err != nil {
		t.Fatal(err)
	}
	server, client := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		handleTargetForwarding(server, target, tunnelprotocol.HTTPS)
	}()

	request := "POST /v1/items?debug=true HTTP/1.1\r\n" +
		"Host: public.example\r\n" +
		"Content-Length: 7\r\n" +
		"\r\n" +
		"payload"
	if _, err := client.Write([]byte(request)); err != nil {
		t.Fatal(err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	response, err := io.ReadAll(client)
	if err != nil {
		t.Fatal(err)
	}
	_ = client.Close()
	<-done

	text := string(response)
	if !strings.Contains(text, "HTTP/1.1 201 Created") {
		t.Fatalf("expected upstream status, got %q", text)
	}
	if !strings.Contains(text, "X-Upstream: ok") {
		t.Fatalf("expected upstream header, got %q", text)
	}
	if !bytes.Contains(response, []byte("proxied:POST:payload")) {
		t.Fatalf("expected upstream body, got %q", text)
	}

	select {
	case got := <-observed:
		if got.method != http.MethodPost {
			t.Fatalf("unexpected method: %s", got.method)
		}
		if got.path != "/base/v1/items" || got.query != "debug=true" {
			t.Fatalf("unexpected upstream route: path=%q query=%q", got.path, got.query)
		}
		if got.host != target.HostHeader {
			t.Fatalf("expected target host %q, got %q", target.HostHeader, got.host)
		}
		if got.body != "payload" {
			t.Fatalf("unexpected body: %q", got.body)
		}
	case <-time.After(time.Second):
		t.Fatal("upstream did not receive request")
	}
}

func TestHTTPTargetForwardingPreservesPUTMethodAndBody(t *testing.T) {
	t.Parallel()

	observed := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		observed <- r.Method + ":" + r.URL.Path + ":" + string(body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	target, err := TargetFor("", upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	server, client := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		handleTargetForwarding(server, target, tunnelprotocol.HTTPS)
	}()

	request := "PUT /settings HTTP/1.1\r\n" +
		"Host: public.example\r\n" +
		"Content-Length: 11\r\n" +
		"\r\n" +
		"new-setting"
	if _, err := client.Write([]byte(request)); err != nil {
		t.Fatal(err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	response, err := io.ReadAll(client)
	if err != nil {
		t.Fatal(err)
	}
	_ = client.Close()
	<-done

	if !strings.Contains(string(response), "HTTP/1.1 204 No Content") {
		t.Fatalf("expected upstream status, got %q", string(response))
	}
	select {
	case got := <-observed:
		if got != "PUT:/settings:new-setting" {
			t.Fatalf("unexpected upstream request: %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("upstream did not receive request")
	}
}

func TestHTTPTargetForwardingSupportsUpgradeResponses(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isHTTPUpgrade(r) {
			t.Fatalf("expected upgrade request")
		}
		conn, rw, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Errorf("hijack upstream: %v", err)
			return
		}
		defer conn.Close()
		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: echo\r\nConnection: Upgrade\r\n\r\n")
		if err := rw.Flush(); err != nil {
			t.Errorf("flush upgrade response: %v", err)
			return
		}
		line, err := rw.ReadString('\n')
		if err != nil {
			t.Errorf("read upgraded payload: %v", err)
			return
		}
		_, _ = rw.WriteString("echo:" + line)
		_ = rw.Flush()
	}))
	defer upstream.Close()

	target, err := TargetFor("", upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	server, client := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		handleTargetForwarding(server, target, tunnelprotocol.HTTPS)
	}()

	request := "GET /socket HTTP/1.1\r\n" +
		"Host: public.example\r\n" +
		"Connection: Upgrade\r\n" +
		"Upgrade: echo\r\n\r\n"
	if _, err := client.Write([]byte(request)); err != nil {
		t.Fatal(err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	header := make([]byte, 256)
	n, err := client.Read(header)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(header[:n]), "101 Switching Protocols") {
		t.Fatalf("expected 101 response, got %q", string(header[:n]))
	}
	if _, err := client.Write([]byte("ping\n")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 32)
	n, err = client.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "echo:ping\n" {
		t.Fatalf("unexpected upgraded payload: %q", string(buf[:n]))
	}
	_ = client.Close()
	<-done
}

func TestJoinTargetPathPreservesRouteShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		base string
		req  string
		want string
	}{
		{base: "/base", req: "/v1/items", want: "/base/v1/items"},
		{base: "/base/", req: "/v1/items", want: "/base/v1/items"},
		{base: "/base", req: "/", want: "/base/"},
		{base: "/base", req: "/v1//items", want: "/base/v1//items"},
		{base: "/base", req: "/v1/../items", want: "/base/v1/../items"},
	}

	for _, tt := range tests {
		got := joinTargetPath(tt.base, tt.req)
		if got != tt.want {
			t.Fatalf("joinTargetPath(%q, %q) = %q, want %q", tt.base, tt.req, got, tt.want)
		}
	}
}

func TestJoinTargetRawPathPreservesEncodedSegments(t *testing.T) {
	t.Parallel()

	decoded := joinTargetPath("/base", "/files/a/b")
	raw := joinTargetRawPath("/base", "/files/a%2Fb", decoded)

	if raw != "/base/files/a%2Fb" {
		t.Fatalf("unexpected raw path: %q", raw)
	}
}
