package gdrive

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/oauth2"
)

func TestTokenFromFileRoundTrip(t *testing.T) {
	tok := &oauth2.Token{
		AccessToken:  "access",
		RefreshToken: "refresh",
		TokenType:    "Bearer",
	}
	path := filepath.Join(t.TempDir(), "token.json")

	saveToken(path, tok)

	got, err := tokenFromFile(path)
	if err != nil {
		t.Fatalf("tokenFromFile() error = %v", err)
	}
	if got.AccessToken != tok.AccessToken || got.RefreshToken != tok.RefreshToken || got.TokenType != tok.TokenType {
		t.Fatalf("token mismatch: got %#v want %#v", got, tok)
	}
}

func TestTokenFromFileMissingFile(t *testing.T) {
	_, err := tokenFromFile(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("expected not-exist error, got %v", err)
	}
}
