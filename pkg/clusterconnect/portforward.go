package clusterconnect

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

type PodDialer interface {
	DialPod(ctx context.Context, namespace, podName string, port int32) (net.Conn, error)
}

type PortForwardDialer struct {
	RESTConfig *rest.Config
	Clientset  kubernetes.Interface
}

func (d *PortForwardDialer) DialPod(ctx context.Context, namespace, podName string, port int32) (net.Conn, error) {
	if d == nil || d.RESTConfig == nil || d.Clientset == nil {
		return nil, fmt.Errorf("pod port-forward dialer is not initialized")
	}
	if namespace == "" || podName == "" || port <= 0 {
		return nil, fmt.Errorf("namespace, pod name, and port are required")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	localPort := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	transport, upgrader, err := spdy.RoundTripperFor(d.RESTConfig)
	if err != nil {
		return nil, err
	}
	serverURL := &url.URL{
		Scheme: "https",
		Path:   fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", namespace, podName),
		Host:   stringsTrimScheme(d.RESTConfig.Host),
	}
	if parsed, parseErr := url.Parse(d.RESTConfig.Host); parseErr == nil && parsed.Scheme != "" {
		serverURL.Scheme = parsed.Scheme
		serverURL.Host = parsed.Host
	}
	spdyDialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, serverURL)

	stopChan := make(chan struct{})
	readyChan := make(chan struct{})
	errChan := make(chan error, 1)
	go func() {
		fw, err := portforward.New(spdyDialer, []string{fmt.Sprintf("%d:%d", localPort, port)}, stopChan, readyChan, io.Discard, io.Discard)
		if err != nil {
			errChan <- err
			return
		}
		errChan <- fw.ForwardPorts()
	}()

	select {
	case <-readyChan:
	case err := <-errChan:
		close(stopChan)
		return nil, err
	case <-ctx.Done():
		close(stopChan)
		return nil, ctx.Err()
	}

	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(localPort)))
	if err != nil {
		close(stopChan)
		return nil, err
	}
	return &forwardedConn{Conn: conn, stop: stopChan}, nil
}

type forwardedConn struct {
	net.Conn
	stop chan struct{}
	once sync.Once
}

func (c *forwardedConn) Close() error {
	c.once.Do(func() {
		close(c.stop)
	})
	return c.Conn.Close()
}

func stringsTrimScheme(host string) string {
	if u, err := url.Parse(host); err == nil && u.Host != "" {
		return u.Host
	}
	return host
}
