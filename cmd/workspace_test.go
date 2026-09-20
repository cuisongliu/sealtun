package cmd

import (
	"strings"
	"testing"

	"github.com/labring/sealtun/pkg/auth"
)

func TestMatchWorkspace(t *testing.T) {
	namespaces := []auth.Namespace{
		{UID: "u1", ID: "ns-aaa", TeamName: "private team"},
		{UID: "u2", ID: "ns-bbb", TeamName: "team-alpha"},
		{UID: "u3", ID: "ns-ccc", TeamName: "team-alpha"},
	}
	match, err := matchWorkspace(namespaces, "ns-bbb")
	if err != nil || match.ID != "ns-bbb" {
		t.Fatalf("id match failed: %v %#v", err, match)
	}
	match, err = matchWorkspace(namespaces, "PRIVATE TEAM")
	if err != nil || match.ID != "ns-aaa" {
		t.Fatalf("case-insensitive name match failed: %v %#v", err, match)
	}
	if _, err = matchWorkspace(namespaces, "team-alpha"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("duplicate names must be ambiguous, got %v", err)
	}
	_, err = matchWorkspace(namespaces, "ns-zzz")
	if err == nil || !strings.Contains(err.Error(), "ns-aaa") {
		t.Fatalf("missing workspace must list available IDs, got %v", err)
	}
}

func TestRewriteKubeconfigNamespace(t *testing.T) {
	kubeconfig := `apiVersion: v1
kind: Config
current-context: main
contexts:
- name: main
  context:
    cluster: c
    user: u
    namespace: ns-old
clusters:
- name: c
  cluster:
    server: https://example:6443
users:
- name: u
  user:
    token: t
`
	rewritten, err := auth.RewriteKubeconfigNamespace(kubeconfig, "ns-new")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rewritten, "namespace: ns-new") {
		t.Fatalf("namespace not rewritten:\n%s", rewritten)
	}
	if strings.Contains(rewritten, "ns-old") {
		t.Fatalf("old namespace left behind:\n%s", rewritten)
	}
	ns, err := auth.KubeconfigNamespace(rewritten)
	if err != nil || ns != "ns-new" {
		t.Fatalf("KubeconfigNamespace = %q, %v", ns, err)
	}
	if _, err := auth.RewriteKubeconfigNamespace(kubeconfig, ""); err == nil {
		t.Fatal("empty namespace must fail")
	}
	if _, err := auth.RewriteKubeconfigNamespace("not yaml", "ns-x"); err == nil {
		t.Fatal("bad kubeconfig must fail")
	}
}
