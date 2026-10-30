package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Exercise packaging independently of the host compiler and Homebrew installation.
func TestMacOSBuildPackaging(t *testing.T) {
	for _, bundle := range []bool{false, true} {
		name := "standalone"
		if bundle {
			name = "bundle"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			for _, dir := range []string{"scripts", "assets", "bin"} {
				if err := os.Mkdir(filepath.Join(root, dir), 0755); err != nil {
					t.Fatal(err)
				}
			}
			for _, file := range []string{"scripts/build-macos-arm64.sh", "assets/koko-tools-icon-120.png"} {
				data, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, file), data, 0755); err != nil {
					t.Fatal(err)
				}
			}
			shim := "#!/bin/sh\n[ \"$CGO_ENABLED\" = 1 ] || exit 1\nprintf 'test executable' > \"$3\"\n"
			if err := os.WriteFile(filepath.Join(root, "bin/go"), []byte(shim), 0755); err != nil {
				t.Fatal(err)
			}
			args := []string{filepath.Join(root, "scripts/build-macos-arm64.sh")}
			if bundle {
				args = append(args, "--bundle")
			}
			cmd := exec.Command("bash", args...)
			cmd.Env = append(os.Environ(), "PATH="+filepath.Join(root, "bin")+":"+os.Getenv("PATH"))
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("build: %v\n%s", err, out)
			}
			files := []string{"dist/macos-arm64/koko-tools", "dist/macos-arm64/assets/koko-tools-icon-120.png"}
			if bundle {
				files = append(files, "dist/macos-arm64/Koko Tools.app/Contents/Info.plist", "dist/macos-arm64/Koko Tools.app/Contents/MacOS/koko-tools")
				_, sipsErr := exec.LookPath("sips")
				_, iconErr := exec.LookPath("iconutil")
				if sipsErr == nil && iconErr == nil {
					files = append(files, "dist/macos-arm64/Koko Tools.app/Contents/Resources/koko-tools.icns")
				}
			}
			for _, file := range files {
				if _, err := os.Stat(filepath.Join(root, file)); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
