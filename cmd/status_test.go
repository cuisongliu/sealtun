package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/labring/sealtun/pkg/auth"
)

func TestCollectStatusLoggedOut(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	status, err := collectStatus()
	if err != nil {
		t.Fatalf("collectStatus returned error: %v", err)
	}

	if status.LoggedIn {
		t.Fatal("expected logged out status")
	}
	if status.ConfigDir == "" {
		t.Fatal("expected config dir to be populated")
	}
	if status.AuthFile.Present {
		t.Fatal("expected auth file to be absent")
	}
}

func TestCollectStatusDoesNotCreateConfigDirWhenLoggedOut(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if _, err := collectStatus(); err != nil {
		t.Fatalf("collectStatus returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".sealtun")); !os.IsNotExist(err) {
		t.Fatalf("expected collectStatus not to create config dir, stat error: %v", err)
	}
}

func TestCollectStatusWithKubeconfigSummary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := auth.SaveAuthData(auth.AuthData{
		Region:          "https://gzg.sealos.run",
		AuthMethod:      "oauth2_device_grant",
		AuthenticatedAt: "2026-04-20T08:00:00Z",
		CurrentWorkspace: &auth.Workspace{
			ID:       "ns-test",
			TeamName: "demo",
		},
	}, `
apiVersion: v1
kind: Config
current-context: ctx-demo
contexts:
- name: ctx-demo
  context:
    cluster: cluster-demo
    namespace: ns-demo
clusters:
- name: cluster-demo
  cluster:
    server: https://example.com
users:
- name: user-demo
  user:
    token: abc
`); err != nil {
		t.Fatalf("SaveAuthData returned error: %v", err)
	}

	status, err := collectStatus()
	if err != nil {
		t.Fatalf("collectStatus returned error: %v", err)
	}

	if !status.LoggedIn {
		t.Fatal("expected logged in status")
	}
	if status.SealosDomain != "" {
		t.Fatalf("expected empty sealos domain, got %s", status.SealosDomain)
	}
	if status.Kubeconfig.CurrentContext != "ctx-demo" {
		t.Fatalf("unexpected current context: %s", status.Kubeconfig.CurrentContext)
	}
	if status.Kubeconfig.Cluster != "cluster-demo" {
		t.Fatalf("unexpected cluster: %s", status.Kubeconfig.Cluster)
	}
	if status.Kubeconfig.Namespace != "ns-demo" {
		t.Fatalf("unexpected namespace: %s", status.Kubeconfig.Namespace)
	}
	if status.WorkspaceID != "ns-test" {
		t.Fatalf("unexpected workspace id: %s", status.WorkspaceID)
	}
}

func TestCollectStatusIncludesSealosDomain(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := auth.SaveAuthData(auth.AuthData{
		Region:          "https://hzh.sealos.run",
		SealosDomain:    "sealoshzh.site",
		AuthMethod:      "oauth2_device_grant",
		AuthenticatedAt: "2026-05-06T08:00:00Z",
	}, `
apiVersion: v1
kind: Config
current-context: ctx-demo
contexts:
- name: ctx-demo
  context:
    cluster: cluster-demo
clusters:
- name: cluster-demo
  cluster:
    server: https://example.com
users:
- name: user-demo
  user:
    token: abc
`); err != nil {
		t.Fatalf("SaveAuthData returned error: %v", err)
	}

	status, err := collectStatus()
	if err != nil {
		t.Fatalf("collectStatus returned error: %v", err)
	}
	if status.SealosDomain != "sealoshzh.site" {
		t.Fatalf("expected sealos domain sealoshzh.site, got %s", status.SealosDomain)
	}
}

func TestCollectStatusIncludesActiveProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if _, err := auth.SaveProfile("gzg-main", auth.AuthData{
		Region:          "https://gzg.sealos.run",
		SealosDomain:    "sealosgzg.site",
		AuthMethod:      "oauth2_device_grant",
		AuthenticatedAt: "2026-05-08T08:00:00Z",
	}, `
apiVersion: v1
kind: Config
current-context: ctx-demo
contexts:
- name: ctx-demo
  context:
    cluster: cluster-demo
clusters:
- name: cluster-demo
  cluster:
    server: https://example.com
users:
- name: user-demo
  user:
    token: abc
`); err != nil {
		t.Fatalf("SaveProfile returned error: %v", err)
	}
	if err := auth.ActivateProfile("gzg-main"); err != nil {
		t.Fatalf("ActivateProfile returned error: %v", err)
	}

	status, err := collectStatus()
	if err != nil {
		t.Fatalf("collectStatus returned error: %v", err)
	}
	if status.ActiveProfile != "gzg-main" {
		t.Fatalf("expected active profile gzg-main, got %s", status.ActiveProfile)
	}
}

func TestStatusJSONOutput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir, err := auth.GetSealosDir()
	if err != nil {
		t.Fatalf("GetSealosDir returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "kubeconfig"), []byte("not yaml"), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}

	status, err := collectStatus()
	if err != nil {
		t.Fatalf("collectStatus returned error: %v", err)
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}

	jsonText := string(data)
	if !strings.Contains(jsonText, `"loggedIn":false`) {
		t.Fatalf("expected loggedIn field in json output: %s", jsonText)
	}
	if !strings.Contains(jsonText, `"kubeconfig"`) {
		t.Fatalf("expected kubeconfig field in json output: %s", jsonText)
	}
}

func TestCollectStatusDoesNotFollowKubeconfigSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires extra privileges on Windows")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	dir, err := auth.GetSealosDir()
	if err != nil {
		t.Fatalf("GetSealosDir returned error: %v", err)
	}
	outside := filepath.Join(home, "outside-kubeconfig")
	if err := os.WriteFile(outside, []byte(`
apiVersion: v1
kind: Config
current-context: outside
contexts:
- name: outside
  context:
    cluster: outside-cluster
clusters:
- name: outside-cluster
  cluster:
    server: https://outside.example.com
users:
- name: outside-user
  user:
    token: abc
`), 0o600); err != nil {
		t.Fatalf("write outside kubeconfig: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "kubeconfig")); err != nil {
		t.Fatalf("create kubeconfig symlink: %v", err)
	}

	status, err := collectStatus()
	if err != nil {
		t.Fatalf("collectStatus returned error: %v", err)
	}
	if status.Kubeconfig.CurrentContext != "" || status.Kubeconfig.Cluster != "" {
		t.Fatalf("status followed kubeconfig symlink: %#v", status.Kubeconfig)
	}
	if status.Kubeconfig.Error == "" || !strings.Contains(status.Kubeconfig.Error, "not a regular file") {
		t.Fatalf("expected symlink error, got %#v", status.Kubeconfig)
	}
}
