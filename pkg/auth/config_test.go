package auth

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestGetSealosDirCopiesLegacySealtunFilesOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	legacyDir := filepath.Join(home, legacyConfigDir)
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatalf("create legacy dir: %v", err)
	}
	legacyFile := filepath.Join(legacyDir, "auth.json")
	if err := os.WriteFile(legacyFile, []byte(`{"region":"test"}`), 0o600); err != nil {
		t.Fatalf("write legacy auth file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "unrelated.json"), []byte(`{"owner":"other"}`), 0o600); err != nil {
		t.Fatalf("write unrelated legacy file: %v", err)
	}
	legacySessionsDir := filepath.Join(legacyDir, "sessions")
	if err := os.MkdirAll(legacySessionsDir, 0o700); err != nil {
		t.Fatalf("create legacy sessions dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacySessionsDir, "old.json"), []byte(`{"tunnelId":"old"}`), 0o600); err != nil {
		t.Fatalf("write legacy session file: %v", err)
	}

	dir, err := GetSealosDir()
	if err != nil {
		t.Fatalf("GetSealosDir returned error: %v", err)
	}

	expectedDir := filepath.Join(home, currentConfigDir)
	if dir != expectedDir {
		t.Fatalf("expected dir %s, got %s", expectedDir, dir)
	}
	if _, err := os.Stat(filepath.Join(expectedDir, "auth.json")); err != nil {
		t.Fatalf("expected copied auth.json to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(expectedDir, "unrelated.json")); !os.IsNotExist(err) {
		t.Fatalf("expected unrelated legacy file not to be copied, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(expectedDir, "sessions", "old.json")); !os.IsNotExist(err) {
		t.Fatalf("expected legacy session files not to be copied, stat err=%v", err)
	}
	if _, err := os.Stat(legacyDir); err != nil {
		t.Fatalf("expected legacy dir to remain untouched, stat err=%v", err)
	}
}

func TestGetSealosDirDoesNotRecopyLegacyAfterCurrentDirExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	legacyDir := filepath.Join(home, legacyConfigDir)
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatalf("create legacy dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "auth.json"), []byte(`{"region":"test"}`), 0o600); err != nil {
		t.Fatalf("write legacy auth file: %v", err)
	}

	dir, err := GetSealosDir()
	if err != nil {
		t.Fatalf("GetSealosDir returned error: %v", err)
	}
	currentAuth := filepath.Join(dir, "auth.json")
	if err := os.Remove(currentAuth); err != nil {
		t.Fatalf("remove current auth: %v", err)
	}

	if _, err := GetSealosDir(); err != nil {
		t.Fatalf("GetSealosDir returned error: %v", err)
	}
	if _, err := os.Stat(currentAuth); !os.IsNotExist(err) {
		t.Fatalf("expected removed current auth not to be recopied, stat err=%v", err)
	}
}

func TestGetSealosDirDoesNotFollowLegacySymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires extra privileges on Windows")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	outside := filepath.Join(home, "outside-auth.json")
	if err := os.WriteFile(outside, []byte(`{"region":"outside"}`), 0o600); err != nil {
		t.Fatalf("write outside auth: %v", err)
	}
	legacyDir := filepath.Join(home, legacyConfigDir)
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatalf("create legacy dir: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(legacyDir, "auth.json")); err != nil {
		t.Fatalf("create legacy auth symlink: %v", err)
	}

	dir, err := GetSealosDir()
	if err != nil {
		t.Fatalf("GetSealosDir returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "auth.json")); !os.IsNotExist(err) {
		t.Fatalf("expected legacy auth symlink not to be copied, stat err=%v", err)
	}
}

func TestGetSealosDirRejectsCurrentConfigSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires extra privileges on Windows")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	outside := filepath.Join(home, "outside-config")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatalf("create outside config dir: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(home, currentConfigDir)); err != nil {
		t.Fatalf("create current config symlink: %v", err)
	}

	if _, err := GetSealosDir(); err == nil {
		t.Fatal("expected current config symlink to be rejected")
	}
}

func TestEnsurePrivateDirRestrictsExistingDirectoryPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod permission bits are not portable on Windows")
	}

	dir := filepath.Join(t.TempDir(), "config")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatalf("relax config dir permissions: %v", err)
	}

	created, err := EnsurePrivateDir(dir, "config directory")
	if err != nil {
		t.Fatalf("EnsurePrivateDir returned error: %v", err)
	}
	if created {
		t.Fatal("expected existing directory not to be reported as created")
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat config dir: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("expected permissions 0700, got %04o", got)
	}
}

func TestSaveAuthDataDoesNotCommitAuthWhenKubeconfigWriteFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir, err := GetSealosDir()
	if err != nil {
		t.Fatalf("GetSealosDir returned error: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "kubeconfig"), 0o700); err != nil {
		t.Fatalf("create kubeconfig directory: %v", err)
	}

	err = SaveAuthData(AuthData{
		Region:        "https://gzg.sealos.run",
		AccessToken:   "access",
		RegionalToken: "regional",
	}, "apiVersion: v1")
	if err == nil {
		t.Fatal("expected kubeconfig write failure")
	}
	if _, err := os.Stat(filepath.Join(dir, "auth.json")); !os.IsNotExist(err) {
		t.Fatalf("auth.json should not be written when kubeconfig fails, stat err=%v", err)
	}
}

func TestSaveAuthDataRestoresKubeconfigWhenAuthWriteFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := SaveAuthData(AuthData{
		Region:        "https://gzg.sealos.run",
		AccessToken:   "old-access",
		RegionalToken: "old-regional",
	}, "old-kubeconfig"); err != nil {
		t.Fatalf("initial SaveAuthData returned error: %v", err)
	}

	dir, err := GetSealosDir()
	if err != nil {
		t.Fatalf("GetSealosDir returned error: %v", err)
	}
	authPath := filepath.Join(dir, "auth.json")
	if err := os.Remove(authPath); err != nil {
		t.Fatalf("remove auth.json: %v", err)
	}
	if err := os.Mkdir(authPath, 0o700); err != nil {
		t.Fatalf("replace auth.json with directory: %v", err)
	}

	err = SaveAuthData(AuthData{
		Region:        "https://hzh.sealos.run",
		AccessToken:   "new-access",
		RegionalToken: "new-regional",
	}, "new-kubeconfig")
	if err == nil {
		t.Fatal("expected auth write failure")
	}

	got, err := os.ReadFile(filepath.Join(dir, "kubeconfig"))
	if err != nil {
		t.Fatalf("read kubeconfig: %v", err)
	}
	if string(got) != "old-kubeconfig" {
		t.Fatalf("expected old kubeconfig to be restored, got %q", string(got))
	}
}

func TestProfileLifecycleActivatesNamedCredentials(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	name, err := SaveProfile("Dev-GZG", AuthData{
		Region:          "https://gzg.sealos.run",
		SealosDomain:    "sealosgzg.site",
		AuthMethod:      "oauth2_device_grant",
		AuthenticatedAt: "2026-05-08T08:00:00Z",
		CurrentWorkspace: &Workspace{
			ID:       "ns-demo",
			TeamName: "demo",
		},
	}, "kubeconfig-gzg")
	if err != nil {
		t.Fatalf("SaveProfile returned error: %v", err)
	}
	if name != "dev-gzg" {
		t.Fatalf("expected normalized profile name, got %s", name)
	}

	if err := ActivateProfile("dev-gzg"); err != nil {
		t.Fatalf("ActivateProfile returned error: %v", err)
	}
	current, err := CurrentProfileName()
	if err != nil {
		t.Fatalf("CurrentProfileName returned error: %v", err)
	}
	if current != "dev-gzg" {
		t.Fatalf("expected current profile dev-gzg, got %s", current)
	}
	authData, err := LoadAuthData()
	if err != nil {
		t.Fatalf("LoadAuthData returned error: %v", err)
	}
	if authData.Region != "https://gzg.sealos.run" {
		t.Fatalf("expected active auth to use profile region, got %s", authData.Region)
	}
	kubeconfig, err := ActiveKubeconfig()
	if err != nil {
		t.Fatalf("ActiveKubeconfig returned error: %v", err)
	}
	if kubeconfig != "kubeconfig-gzg" {
		t.Fatalf("expected active kubeconfig to be restored, got %q", kubeconfig)
	}

	profiles, err := ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles returned error: %v", err)
	}
	if len(profiles) != 1 || profiles[0].Name != "dev-gzg" || !profiles[0].Current || !profiles[0].KubeconfigPresent {
		t.Fatalf("unexpected profiles: %#v", profiles)
	}

	if err := DeleteProfile("dev-gzg"); err != nil {
		t.Fatalf("DeleteProfile returned error: %v", err)
	}
	current, err = CurrentProfileName()
	if err != nil {
		t.Fatalf("CurrentProfileName returned error after delete: %v", err)
	}
	if current != "" {
		t.Fatalf("expected current profile marker to be cleared, got %s", current)
	}
}

func TestProfileOperationsRejectSymlinkDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires extra privileges on Windows")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	profilesDir, err := ProfilesDir()
	if err != nil {
		t.Fatalf("ProfilesDir returned error: %v", err)
	}
	outside := filepath.Join(home, "outside-profile")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatalf("create outside profile dir: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(profilesDir, "linked")); err != nil {
		t.Fatalf("create profile symlink: %v", err)
	}

	if _, err := SaveProfile("linked", AuthData{Region: "https://gzg.sealos.run"}, "kubeconfig"); err == nil {
		t.Fatal("expected SaveProfile to reject symlink profile directory")
	}
	if _, _, err := LoadProfile("linked"); err == nil {
		t.Fatal("expected LoadProfile to reject symlink profile directory")
	}
	if err := DeleteProfile("linked"); err == nil {
		t.Fatal("expected DeleteProfile to reject symlink profile directory")
	}
	if _, err := os.Stat(filepath.Join(outside, "auth.json")); !os.IsNotExist(err) {
		t.Fatalf("expected outside directory not to be written through symlink, stat err=%v", err)
	}
}

func TestProfileOperationsRestrictExistingDirectoryPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod permission bits are not portable on Windows")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	profilesDir, err := ProfilesDir()
	if err != nil {
		t.Fatalf("ProfilesDir returned error: %v", err)
	}
	profilePath := filepath.Join(profilesDir, "shared")
	if err := os.Mkdir(profilePath, 0o700); err != nil {
		t.Fatalf("create profile dir: %v", err)
	}
	if err := os.Chmod(profilePath, 0o755); err != nil {
		t.Fatalf("relax profile dir permissions: %v", err)
	}

	if _, err := SaveProfile("shared", AuthData{Region: "https://gzg.sealos.run"}, "kubeconfig"); err != nil {
		t.Fatalf("SaveProfile returned error: %v", err)
	}
	info, err := os.Stat(profilePath)
	if err != nil {
		t.Fatalf("stat profile dir: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("expected profile dir permissions 0700, got %04o", got)
	}
}

func TestProfilesDirRejectsSymlinkRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires extra privileges on Windows")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	root, err := GetSealosDir()
	if err != nil {
		t.Fatalf("GetSealosDir returned error: %v", err)
	}
	outside := filepath.Join(home, "outside-profiles")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatalf("create outside profiles dir: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, profilesDirName)); err != nil {
		t.Fatalf("create profiles symlink: %v", err)
	}

	if _, err := ProfilesDir(); err == nil {
		t.Fatal("expected profiles root symlink to be rejected")
	}
}

func TestValidateProfileNameRejectsUnsafeValues(t *testing.T) {
	tests := []string{"", "../auth", ".hidden", "bad/name", "bad name", "bad:profile"}
	for _, input := range tests {
		if _, err := ValidateProfileName(input); err == nil {
			t.Fatalf("expected %q to be rejected", input)
		}
	}
}

func TestListProfilesIncludesBrokenProfiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir, err := ProfilesDir()
	if err != nil {
		t.Fatalf("ProfilesDir returned error: %v", err)
	}
	brokenDir := filepath.Join(dir, "broken")
	if err := os.MkdirAll(brokenDir, 0o700); err != nil {
		t.Fatalf("mkdir broken profile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(brokenDir, "auth.json"), []byte("{"), 0o600); err != nil {
		t.Fatalf("write broken auth: %v", err)
	}
	if err := os.WriteFile(filepath.Join(brokenDir, "kubeconfig"), []byte("kubeconfig"), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}

	profiles, err := ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles returned error: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected one broken profile, got %#v", profiles)
	}
	if profiles[0].Name != "broken" || profiles[0].Error == "" || !profiles[0].KubeconfigPresent {
		t.Fatalf("unexpected broken profile summary: %#v", profiles[0])
	}
}

func TestListProfilesMarksMissingKubeconfigBroken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir, err := ProfilesDir()
	if err != nil {
		t.Fatalf("ProfilesDir returned error: %v", err)
	}
	profileDir := filepath.Join(dir, "missing-kubeconfig")
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		t.Fatalf("mkdir profile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "auth.json"), []byte(`{"region":"https://gzg.sealos.run"}`), 0o600); err != nil {
		t.Fatalf("write auth: %v", err)
	}

	profiles, err := ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles returned error: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected one profile, got %#v", profiles)
	}
	if profiles[0].Error != "kubeconfig is missing" || profiles[0].KubeconfigPresent {
		t.Fatalf("expected missing kubeconfig to be reported as broken, got %#v", profiles[0])
	}
}

func TestClearAuthDataClearsActiveProfileMarker(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if _, err := SaveProfile("dev", AuthData{
		Region:          "https://gzg.sealos.run",
		AuthMethod:      "oauth2_device_grant",
		AuthenticatedAt: "2026-05-08T08:00:00Z",
	}, "kubeconfig"); err != nil {
		t.Fatalf("SaveProfile returned error: %v", err)
	}
	if err := ActivateProfile("dev"); err != nil {
		t.Fatalf("ActivateProfile returned error: %v", err)
	}
	if err := ClearAuthData(); err != nil {
		t.Fatalf("ClearAuthData returned error: %v", err)
	}
	current, err := CurrentProfileName()
	if err != nil {
		t.Fatalf("CurrentProfileName returned error: %v", err)
	}
	if current != "" {
		t.Fatalf("expected active profile marker to be cleared, got %s", current)
	}
	if _, _, err := LoadProfile("dev"); err != nil {
		t.Fatalf("expected saved profile to remain after logout cleanup: %v", err)
	}
}

func TestResolveRegionRejectsPlainHTTP(t *testing.T) {
	if _, err := ResolveRegion("http://custom-region.example"); err == nil {
		t.Fatal("expected plain http region to be rejected")
	}
}

func TestResolveRegionRejectsUnsupportedCustomRegions(t *testing.T) {
	tests := []string{
		"https://custom-region.example",
		"https://custom-region.example/path",
		"https://custom-region.example?debug=true",
		"https://custom-region.example#fragment",
		"https://user:pass@custom-region.example",
		"https://127.0.0.1:8443",
		"https://[::1]:8443",
		"https://localhost:8443",
	}

	for _, input := range tests {
		if _, err := ResolveRegion(input); err == nil {
			t.Fatalf("expected %s to be rejected", input)
		}
	}
}

func TestResolveRegionAllowsKnownRegionURLWithTrailingSlash(t *testing.T) {
	got, err := ResolveRegion("https://hzh.sealos.run/")
	if err != nil {
		t.Fatalf("expected known region URL with trailing slash to be accepted: %v", err)
	}
	if got != "https://hzh.sealos.run" {
		t.Fatalf("unexpected normalized region: %s", got)
	}
}

func TestPollIntervalRejectsNonPositiveValues(t *testing.T) {
	for _, input := range []int{0, -1, -30} {
		if got := pollIntervalFromSeconds(input); got != 5*time.Second {
			t.Fatalf("expected default poll interval for %d, got %s", input, got)
		}
	}
	if got := pollIntervalFromSeconds(2); got != 2*time.Second {
		t.Fatalf("expected explicit poll interval, got %s", got)
	}
}

func TestReadLimitedResponseBodyCapsBodySize(t *testing.T) {
	body := strings.Repeat("x", maxAuthResponseBodyBytes+1024)
	got, err := readLimitedResponseBody(strings.NewReader(body))
	if err != nil {
		t.Fatalf("readLimitedResponseBody returned error: %v", err)
	}
	if len(got) != maxAuthResponseBodyBytes {
		t.Fatalf("expected body to be capped at %d bytes, got %d", maxAuthResponseBodyBytes, len(got))
	}
}
