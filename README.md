# Koko Tools

This repository is for experiments in Go with a terminal UI.

There are a few everyday desktop tools here: page conversion, password generation, notes, and Google Drive sync.

## Free technologies used

This project is built with free and open technologies:

* Go: application language and build toolchain. Reference: https://go.dev/
* tcell and tview: terminal UI libraries used by the application shell. Reference: https://github.com/gdamore/tcell and https://github.com/rivo/tview
* Tree-sitter: parsing and syntax-highlighting foundation for the notes editor, together with free language grammars for Bash, CSS, Go, HTML, Java, JavaScript, JSON, Lua, PHP, Python, Rust, and TypeScript. Reference: https://tree-sitter.github.io/tree-sitter/
* Google OAuth 2.0 and Google Drive API client libraries: used for optional Drive sync. Reference: https://developers.google.com/identity/protocols/oauth2 and https://developers.google.com/drive/api
* Font Awesome Free: bundled icon set used for app assets and generated outputs. Reference: https://fontawesome.com/

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

*First build will take more time than others*

```sh
go build -o koko-tools
```

### Run

```sh
./koko-tools
```
