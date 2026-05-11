package publicauth

import "testing"

func TestNewBasicAuthHashesPassword(t *testing.T) {
	t.Parallel()

	config, err := NewBasicAuth("admin", "secret")
	if err != nil {
		t.Fatalf("NewBasicAuth returned error: %v", err)
	}
	if config.PasswordHash == "secret" {
		t.Fatal("password must not be stored in plain text")
	}
	if config.PasswordHash == LegacySHA256Hash("secret") {
		t.Fatal("password must not be stored as an unsalted SHA-256 digest")
	}
	if !Check(*config, "admin", "secret") {
		t.Fatal("expected generated config to authorize matching credentials")
	}
	if Check(*config, "admin", "wrong") {
		t.Fatal("expected wrong password to be rejected")
	}
}

func TestCheckAcceptsLegacySHA256Hash(t *testing.T) {
	t.Parallel()

	config := BasicAuth{Username: "admin", PasswordHash: LegacySHA256Hash("secret")}
	if !Check(config, "admin", "secret") {
		t.Fatal("expected legacy SHA-256 hash to authorize matching credentials")
	}
	if Check(config, "admin", "wrong") {
		t.Fatal("expected wrong password to be rejected")
	}
}

func TestValidateUsernameRejectsUnsafeValues(t *testing.T) {
	t.Parallel()

	for _, username := range []string{"", "bad:name", "bad\nname"} {
		if err := ValidateUsername(username); err == nil {
			t.Fatalf("expected username %q to be rejected", username)
		}
	}
}
