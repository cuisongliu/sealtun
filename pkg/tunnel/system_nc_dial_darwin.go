//go:build darwin

package tunnel

import (
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"sync"
	"time"
)

func dialSystemNCContext(ctx context.Context, network, address string, nativeErr error) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" {
		return nil, nativeErr
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, nativeErr
	}
	if host == "" || port == "" {
		return nil, nativeErr
	}
	if len(host) > 0 && host[0] == '-' {
		return nil, nativeErr
	}

	cmd := exec.CommandContext(ctx, "/usr/bin/nc", host, port)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("%w; system nc fallback setup failed: %v", nativeErr, err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("%w; system nc fallback setup failed: %v", nativeErr, err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("%w; system nc fallback setup failed: %v", nativeErr, err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%w; system nc fallback failed: %v", nativeErr, err)
	}
	go func() {
		_, _ = io.Copy(io.Discard, stderr)
	}()

	return &systemNCConn{
		stdout: stdout,
		stdin:  stdin,
		cmd:    cmd,
		local:  systemNCAddr("system-nc"),
		remote: systemNCAddr(address),
	}, nil
}

type systemNCConn struct {
	stdout io.ReadCloser
	stdin  io.WriteCloser
	cmd    *exec.Cmd
	once   sync.Once
	local  net.Addr
	remote net.Addr
	err    error
}

func (c *systemNCConn) Read(p []byte) (int, error) {
	return c.stdout.Read(p)
}

func (c *systemNCConn) Write(p []byte) (int, error) {
	return c.stdin.Write(p)
}

func (c *systemNCConn) Close() error {
	c.once.Do(func() {
		_ = c.stdin.Close()
		_ = c.stdout.Close()
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		c.err = c.cmd.Wait()
	})
	return c.err
}

func (c *systemNCConn) LocalAddr() net.Addr {
	return c.local
}

func (c *systemNCConn) RemoteAddr() net.Addr {
	return c.remote
}

func (c *systemNCConn) SetDeadline(deadline time.Time) error {
	if err := c.SetReadDeadline(deadline); err != nil {
		return err
	}
	return c.SetWriteDeadline(deadline)
}

func (c *systemNCConn) SetReadDeadline(deadline time.Time) error {
	if setter, ok := c.stdout.(interface{ SetReadDeadline(time.Time) error }); ok {
		return setter.SetReadDeadline(deadline)
	}
	return nil
}

func (c *systemNCConn) SetWriteDeadline(deadline time.Time) error {
	if setter, ok := c.stdin.(interface{ SetWriteDeadline(time.Time) error }); ok {
		return setter.SetWriteDeadline(deadline)
	}
	return nil
}

type systemNCAddr string

func (a systemNCAddr) Network() string {
	return "tcp"
}

func (a systemNCAddr) String() string {
	return string(a)
}
