package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kloneets/tools/src/helpers"
)

func TestRunCLIOpenLinksReadsStdinAndOpensUniqueLinks(t *testing.T) {
	var opened []string
	restore := helpers.SetURIOpenerForTesting(func(uri string) {
		opened = append(opened, uri)
	})
	defer restore()
	var stdout bytes.Buffer
	code, handled := RunCLI([]string{"ol"}, strings.NewReader("https://example.com\nhttps://example.com\nfile:///tmp/a.txt"), &stdout, nil)
	if !handled {
		t.Fatal("handled = false, want true")
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if len(opened) != 2 || opened[0] != "https://example.com" || opened[1] != "file:///tmp/a.txt" {
		t.Fatalf("opened = %v, want unique links in order", opened)
	}
	if !strings.Contains(stdout.String(), "opened 2 link(s)") {
		t.Fatalf("stdout = %q, want opened count", stdout.String())
	}
}

func TestRunCLIOpenLinksReadsFiles(t *testing.T) {
	var opened []string
	restore := helpers.SetURIOpenerForTesting(func(uri string) {
		opened = append(opened, uri)
	})
	defer restore()
	dir := t.TempDir()
	path := filepath.Join(dir, "links.md")
	if err := os.WriteFile(path, []byte("[docs](https://example.com/docs)\nftp://example.com/file"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, handled := RunCLI([]string{"ol", path}, nil, nil, nil)
	if !handled || code != 0 {
		t.Fatalf("RunCLI() = %d, %t; want handled success", code, handled)
	}
	if len(opened) != 2 || opened[0] != "https://example.com/docs" || opened[1] != "ftp://example.com/file" {
		t.Fatalf("opened = %v, want file links", opened)
	}
}

func TestRunCLIOpenLinksMissingFileFails(t *testing.T) {
	var stderr bytes.Buffer
	code, handled := RunCLI([]string{"ol", filepath.Join(t.TempDir(), "missing.md")}, nil, nil, &stderr)
	if !handled {
		t.Fatal("handled = false, want true")
	}
	if code == 0 {
		t.Fatal("code = 0, want failure")
	}
	if strings.TrimSpace(stderr.String()) == "" {
		t.Fatal("stderr is empty, want read error")
	}
}
