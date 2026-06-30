package cmd

import (
	"net"
	"testing"
)

func testKubeconfig(t *testing.T, namespace string) string {
	t.Helper()
	addr := "127.0.0.1:1"
	if listener, err := net.Listen("tcp", "127.0.0.1:0"); err == nil {
		addr = listener.Addr().String()
		_ = listener.Close()
	}
	return `apiVersion: v1
kind: Config
clusters:
- name: test
  cluster:
    server: https://` + addr + `
    insecure-skip-tls-verify: true
contexts:
- name: test
  context:
    cluster: test
    user: test
    namespace: ` + namespace + `
current-context: test
users:
- name: test
  user:
    token: test
`
}
