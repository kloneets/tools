# Repository Guidelines

## Project Structure & Module Organization
`main.go` is the entrypoint and starts the GTK4 application from `src/app`. Feature code is split by package under `src/`: `notes`, `password`, `pages`, `settings`, `gdrive`, `ui`, and shared helpers in `helpers`. GTK builder assets live alongside the code that uses them, for example [`src/app/menu.ui`](/home/koko/Projects/koko-tools/src/app/menu.ui). Keep new UI files close to their owning package.

## Build, Test, and Development Commands
- `go build -o koko-tools`: build the desktop binary at the repository root.
- `./koko-tools`: run the app locally after building.
- `go test ./...`: run all Go tests. There are currently few or no tests; add them with feature work.
- `gofmt -w main.go $(find src -name '*.go')`: format Go sources before committing.

GTK4 system libraries are required before building: `gtk4`, `glib2`, and `gobject-introspection-1.0`.

## Coding Style & Naming Conventions
Use standard Go formatting with tabs and let `gofmt` own layout. Keep packages lowercase (`src/settings`, `src/ui`) and exported identifiers in PascalCase. Prefer short, focused files inside the feature package instead of large cross-package helpers. Name embedded UI assets descriptively, such as `menu.ui`, and keep related Go and UI definitions together. Do not use emoji in code, comments, commit messages, pull requests, or repository documentation.

## Testing Guidelines
Every code change must include or update automated tests unless the change is purely non-functional documentation. Write table-driven tests with the Go `testing` package in `*_test.go` files next to the code they cover. Favor package-level unit tests for helpers and settings logic; isolate GTK-dependent behavior behind testable functions where possible. Run `go test ./...` before opening a pull request.

## Commit & Pull Request Guidelines
Recent history mixes short messages (`small changes to notes`) with clearer prefixes (`feat: finished about section`, `fix window decoration`). Prefer imperative, scoped commits such as `feat: add Google Drive sync toggle` or `fix: back up settings on read failure`. Keep pull requests focused and include:
- a short summary of behavior changes
- linked issue or task, if one exists
- screenshots or a brief UI note for visible GTK changes
- confirmation that `go build` and `go test ./...` were run

## Configuration Notes
The app creates its config directory under the user home directory at startup. Changes to settings, file paths, or Google Drive integration should handle missing directories and read failures safely.
