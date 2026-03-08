# Koko Tools

This repository is for experiments in Go with GTK4.

There are a few everyday desktop tools here: page conversion, password generation, notes, and Google Drive sync.

## Books

This tool just calculate how many pages would be read in another edition of the book.

## Password generator

Generate random passwords

## Notes

You can now save notes

## Building and running

### Build

Requirements

* Go 1.23+
* GTK4
* GLib
* gobject-introspection
* pkg-config / pkgconf

*First build will take more time than others*

```sh
go build -o koko-tools
```

### macOS (Apple Silicon)

Install the GTK4 toolchain with Homebrew:

```sh
brew install gtk4 gobject-introspection pkgconf
export PKG_CONFIG_PATH="$(brew --prefix)/lib/pkgconfig:$(brew --prefix)/share/pkgconfig"
make test
make build-macos-arm64
make package-macos-arm64
```

Available targets:

* `make build-macos-arm64`: build `dist/macos-arm64/koko-tools`
* `make package-macos-arm64`: build `dist/macos-arm64/Koko Tools.app`

Run the app bundle with:

```sh
open "dist/macos-arm64/Koko Tools.app"
```

Or run the binary directly:

```sh
./dist/macos-arm64/koko-tools
```

CI publishes the same output as the `koko-tools-macos-arm64` artifact from the `macos-14` workflow job.

### Run

```sh
./koko-tools
```
