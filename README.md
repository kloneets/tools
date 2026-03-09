# Koko Tools

This repository is for experiments in Go with GTK4.

There are a few everyday desktop tools here: page conversion, password generation, notes, and Google Drive sync.

## Free technologies used

This project is built with free and open technologies:

* Go: application language and build toolchain. Reference: https://go.dev/
* GTK 4: native desktop UI toolkit. Reference: https://www.gtk.org/
* GLib and GObject Introspection: GTK runtime and introspection support used by the app bindings. Reference: https://docs.gtk.org/glib/ and https://gi.readthedocs.io/
* gotk4: Go bindings for GTK 4 used by the application. Reference: https://github.com/diamondburned/gotk4
* Tree-sitter: parsing and syntax-highlighting foundation for the notes editor, together with free language grammars for Bash, CSS, Go, HTML, Java, JavaScript, JSON, Lua, PHP, Python, Rust, and TypeScript. Reference: https://tree-sitter.github.io/tree-sitter/
* Google OAuth 2.0 and Google Drive API client libraries: used for optional Drive sync. Reference: https://developers.google.com/identity/protocols/oauth2 and https://developers.google.com/drive/api
* Font Awesome Free: bundled icon set used for toolbar and action icons in the app. Reference: https://fontawesome.com/

## Development aid

AI coding agents are used as a development aid in this project for implementation support, refactoring, and test/debug iteration. They are used during development, not as part of the shipped application runtime.

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
