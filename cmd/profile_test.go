package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/labring/sealtun/pkg/auth"
)

func TestCollectProfileItems(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if _, err := auth.SaveProfile("gzg-main", auth.AuthData{
		Region:          "https://gzg.sealos.run",
		SealosDomain:    "sealosgzg.site",
		AuthMethod:      "oauth2_device_grant",
		AuthenticatedAt: "2026-05-08T08:00:00Z",
		CurrentWorkspace: &auth.Workspace{
			ID:       "ns-demo",
			TeamName: "demo",
		},
	}, "kubeconfig-gzg"); err != nil {
		t.Fatalf("SaveProfile returned error: %v", err)
	}
	if err := auth.ActivateProfile("gzg-main"); err != nil {
		t.Fatalf("ActivateProfile returned error: %v", err)
	}

	items, err := collectProfileItems()
	if err != nil {
		t.Fatalf("collectProfileItems returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one profile item, got %#v", items)
	}
	item := items[0]
	if item.Name != "gzg-main" || !item.Current || item.Region != "https://gzg.sealos.run" || item.SealosDomain != "sealosgzg.site" {
		t.Fatalf("unexpected profile item: %#v", item)
	}
	if item.WorkspaceID != "ns-demo" || item.WorkspaceName != "demo" {
		t.Fatalf("unexpected workspace summary: %#v", item)
	}
	if !item.KubeconfigPresent {
		t.Fatal("expected kubeconfig to be present")
	}
}

func TestProfileCurrentCommandPrintsNoActiveProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	var out bytes.Buffer
	profileCurrentCmd.SetOut(&out)
	t.Cleanup(func() { profileCurrentCmd.SetOut(nil) })

	if err := profileCurrentCmd.RunE(profileCurrentCmd, nil); err != nil {
		t.Fatalf("RunE returned error: %v", err)
	}
	if !strings.Contains(out.String(), "No active named profile") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestProfileDeleteRequiresExistingProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := auth.DeleteProfile("missing"); err == nil {
		t.Fatal("expected deleting missing profile to fail")
	}
}
