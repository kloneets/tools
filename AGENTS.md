# Repository Guidelines

## Project Structure & Module Organization
`main.go` is the entrypoint and starts the terminal application from `src/app`. Feature code is split by package under `src/`: `notes`, `password`, `pages`, `settings`, `gdrive`, and shared helpers in `helpers`.

## Build, Test, and Development Commands
- `go build -o koko-tools`: build the desktop binary at the repository root.
- `./koko-tools`: run the app locally after building.
- `go test ./...`: run all Go tests.
- `gofmt -w main.go $(find src -name '*.go')`: format Go sources before committing.
- `make build`: build the normal desktop binary.
- `make test`: run the full Go test suite.

## Agent Development Workflow
For feature work, behavioral changes, and non-trivial fixes, follow `docs/agent-development-workflow.md`. Use `.agents/planner.md` to create the accepted plan, `.agents/coder.md` to implement it, `.agents/reviewer.md` for iterative code review, and `.agents/done-auditor.md` to confirm the final work matches the plan. Every review finding must be fixed or explicitly waived before the task is considered done.

## Coding Style & Naming Conventions
Use standard Go formatting with tabs and let `gofmt` own layout. Keep packages lowercase (`src/settings`, `src/app`) and exported identifiers in PascalCase. Prefer short, focused files inside the feature package instead of large cross-package helpers. Code should remain human-readable and maintainable. When working in an existing file, refactor it as needed to improve readability, structure, and maintainability instead of only adding the minimum change. Do not use emoji in code, comments, commit messages, pull requests, or repository documentation.

## Testing Guidelines
Every code change must include or update automated tests unless the change is purely non-functional documentation. Write table-driven tests with the Go `testing` package in `*_test.go` files next to the code they cover. Favor package-level unit tests for helpers and settings logic. Run `go test ./...` before opening a pull request.

## Commit & Pull Request Guidelines
Recent history mixes short messages (`small changes to notes`) with clearer prefixes (`feat: finished about section`, `fix window decoration`). Prefer imperative, scoped commits such as `feat: add Google Drive sync toggle` or `fix: back up settings on read failure`. Keep pull requests focused and include:
- a short summary of behavior changes
- linked issue or task, if one exists
- screenshots or a brief UI note for visible terminal UI changes
- confirmation that `go build` and `go test ./...` were run

## Configuration Notes
The app creates its config directory under the user home directory at startup. Changes to settings, file paths, or Google Drive integration should handle missing directories and read failures safely.
