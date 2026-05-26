package googleauth

import "testing"

func TestOAuthClientIDUsesEnvOverride(t *testing.T) {
	t.Setenv("KOKO_TOOLS_GOOGLE_CLIENT_ID", "client-id-1")

	if got := OAuthClientID(); got != "client-id-1" {
		t.Fatalf("OAuthClientID() = %q, want client-id-1", got)
	}
}

func TestHasCredentialsRequiresClientIDAndSecret(t *testing.T) {
	t.Setenv("KOKO_TOOLS_GOOGLE_CLIENT_ID", "client-id-1")
	t.Setenv("KOKO_TOOLS_GOOGLE_CLIENT_SECRET", "")

	if !HasCredentials() {
		t.Fatal("HasCredentials() = false, want true with bundled secret fallback")
	}

	t.Setenv("KOKO_TOOLS_GOOGLE_CLIENT_ID", " ")
	t.Setenv("KOKO_TOOLS_GOOGLE_CLIENT_SECRET", " ")
	if !HasCredentials() {
		t.Fatal("HasCredentials() = false, want true with bundled credentials")
	}
}
