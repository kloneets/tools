# Lessons Learned

## Test Environment Checks
- Run `go test ./...` early, but expect sandboxed Go cache permission failures here and rerun with the approved `go test` escalation path.
- For GTK-backed tests, isolate with `go test ./src/ui -v -count=1 -timeout 30s` before assuming the whole suite is hanging.
- Use `gtk.InitCheck()` in tests and skip cleanly when GTK cannot initialize.

## Dependency and Toolchain Issues
- On Go 1.26, check `go4.org/unsafe/assume-no-moving-gc` immediately if GTK packages panic during test startup.
- Prefer fixing the dependency graph so plain `go test ./...` works, instead of relying on `ASSUME_NO_MOVING_GC_UNSAFE_RISK_IT_WITH`.

## Repository-Specific Code Traps
- `google.golang.org/api/drive/v2` uses `MaxResults`, `items`, and `title`, not `PageSize`, `files`, and `name`.
- Password letter generation must include `z` and `Z`; watch loop bounds on character pools.
- Config directory creation should use `os.MkdirAll`, not `os.Mkdir`, because parent directories may be missing.

## Testing Strategy
- Extract small pure helpers from GTK event handlers before writing tests.
- When asserting GTK container contents, prefer stable checks like child count or label text over pointer equality between wrapped objects.
